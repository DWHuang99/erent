package upstreamserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	upstreamdirectory "erent/internal/directory/upstream"
	"erent/internal/modules/oauth"
	"erent/internal/modules/oauth/oidc"
	"erent/internal/rpc/upstream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Route the fixed OpenAI endpoints to the test provider; never send test credentials externally.
type deviceTestTransport struct{ target *url.URL }

func (transport deviceTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host != "auth.openai.com" {
		return nil, fmt.Errorf("unexpected device host")
	}
	cloned := request.Clone(request.Context())
	cloned.URL.Scheme = transport.target.Scheme
	cloned.URL.Host = transport.target.Host
	cloned.Host = transport.target.Host
	return http.DefaultTransport.RoundTrip(cloned)
}

func testDeviceRPC(t *testing.T, auth *oidc.OIDCAuth, timeout time.Duration) upstream.UpstreamServiceClient {
	t.Helper()
	target, err := url.Parse(auth.OauthConfig.Endpoint.TokenURL)
	if err != nil {
		t.Fatal(err)
	}
	original := httpClient
	httpClient = &http.Client{Transport: deviceTestTransport{target: target}, Timeout: 15 * time.Second}
	t.Cleanup(func() { httpClient = original })
	return testRPC(t, auth, timeout)
}

func TestDeviceFlowServiceRPCRoundTrip(t *testing.T) {
	var calls atomic.Int32
	var previous time.Time
	auth := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Error("invalid request headers")
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("request body: %v", err)
		}
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			if len(body) != 1 || body["client_id"] != "test-client" {
				t.Errorf("unexpected request: %v", body)
			}
			fmt.Fprint(w, `{"device_auth_id":"device","user_code":"ABCD-1234","interval":"1"}`)
		case "/api/accounts/deviceauth/token":
			if body["device_auth_id"] != "device" || body["user_code"] != "ABCD-1234" {
				t.Errorf("lost polling body: %v", body)
			}
			now := time.Now()
			if !previous.IsZero() && now.Sub(previous) < time.Second {
				t.Error("polling faster than requested interval")
			}
			previous = now
			switch calls.Add(1) {
			case 1:
				w.WriteHeader(http.StatusForbidden)
			case 2:
				w.WriteHeader(http.StatusNotFound)
			default:
				fmt.Fprint(w, `{"authorization_code":"code","code_verifier":"verifier","code_challenge":"challenge"}`)
			}
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}, "")
	// Both ordinary RPC and HTTP timeouts are shorter than the two-second approval wait.
	directory := upstreamdirectory.New(testDeviceRPC(t, auth, time.Second), time.Second)
	service := oauth.NewOauthService(nil, map[string]*oidc.OIDCAuth{"oai": auth}, directory, nil, nil)
	device, err := service.GetDeviceFlowCode(t.Context(), "oai")
	if err != nil {
		t.Fatal(err)
	}
	if device.Interval != 1 || !strings.HasSuffix(device.VerificationURL, "/codex/device") {
		t.Fatalf("invalid instructions: %+v", device)
	}
	result, err := service.Poll(t.Context(), device.DeviceAuthID, device.UserCode, device.Interval, "oai")
	if err != nil {
		t.Fatal(err)
	}
	if result.Code != "code" || result.CodeVerifier != "verifier" || calls.Load() != 3 {
		t.Fatalf("unexpected result: %+v calls=%d", result, calls.Load())
	}
}

func TestDeviceFlowResponseValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantErr    bool
	}{
		{"valid response", `{"device_auth_id":"d","user_code":"u","interval":"2"}`, false},
		{"missing id", `{"user_code":"u","interval":"2"}`, true},
		{"zero interval", `{"device_auth_id":"d","user_code":"u","interval":"0"}`, true},
		{"invalid interval", `{"device_auth_id":"d","user_code":"u","interval":"bad"}`, true},
		{"huge interval", `{"device_auth_id":"d","user_code":"u","interval":901}`, true},
		{"malformed JSON", `{"device_auth_id":`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := testProvider(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }, "")
			directory := upstreamdirectory.New(testDeviceRPC(t, auth, time.Second), time.Second)
			result, err := directory.GetDeviceFlowCode(t.Context(), "oai")
			if tc.wantErr {
				if !errors.Is(err, upstreamdirectory.ErrDeviceFlowFailed) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil || result.UserCode != "u" || result.IntervalSeconds != 2 {
				t.Fatalf("result=%v error=%v", result, err)
			}
		})
	}
}

func TestDeviceFlowErrorsDoNotRetryOrLeak(t *testing.T) {
	for _, tc := range []struct {
		name       string
		poll       bool
		httpStatus int
		body       string
		want       error
	}{
		{"disabled", false, 404, "secret-device", upstreamdirectory.ErrProviderUnavailable},
		{"forbidden start", false, 403, "secret-device", upstreamdirectory.ErrProviderUnavailable},
		{"bad request", true, 400, "secret-device", upstreamdirectory.ErrDeviceFlowRejected},
		{"rate limit", true, 429, "secret-device", upstreamdirectory.ErrUpstreamUnavailable},
		{"server failure", true, 503, "secret-device", upstreamdirectory.ErrUpstreamUnavailable},
		{"bad JSON", true, 200, "secret-device", upstreamdirectory.ErrDeviceFlowFailed},
		{"missing verifier", true, 200, `{"authorization_code":"secret-device"}`, upstreamdirectory.ErrDeviceFlowFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			auth := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.httpStatus)
				fmt.Fprint(w, tc.body)
			}, "")
			directory := upstreamdirectory.New(testDeviceRPC(t, auth, time.Second), time.Second)
			var err error
			if tc.poll {
				_, err = directory.PollDeviceFlow(t.Context(), "oai", "d", "u", 1)
			} else {
				_, err = directory.GetDeviceFlowCode(t.Context(), "oai")
			}
			if !errors.Is(err, tc.want) || strings.Contains(fmt.Sprint(err), "secret-device") || calls.Load() != 1 {
				t.Fatalf("error=%v calls=%d", err, calls.Load())
			}
		})
	}
}

func TestDeviceFlowCancellationAndHTTPTimeout(t *testing.T) {
	for _, duringRequest := range []bool{false, true} {
		t.Run(fmt.Sprint(duringRequest), func(t *testing.T) {
			entered := make(chan struct{}, 1)
			release := make(chan struct{})
			auth := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				entered <- struct{}{}
				if duringRequest {
					select {
					case <-r.Context().Done():
					case <-release:
					}
					return
				}
				w.WriteHeader(403)
			}, "")
			t.Cleanup(func() { close(release) })
			directory := upstreamdirectory.New(testDeviceRPC(t, auth, 100*time.Millisecond), time.Second)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := directory.PollDeviceFlow(ctx, "oai", "d", "u", 900); done <- err }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("request never started")
			}
			want := upstreamdirectory.ErrDeviceFlowTimeout
			if !duringRequest {
				cancel()
				want = context.Canceled
			}
			select {
			case err := <-done:
				if !errors.Is(err, want) {
					t.Fatalf("error=%v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("poll did not stop")
			}
		})
	}
}

func TestDeviceFlowRejectsInvalidArguments(t *testing.T) {
	s := NewServer(nil, time.Second)
	for _, request := range []*upstream.PollDeviceFlowRequest{nil, {}, {Provider: "oai", DeviceAuthId: "d", UserCode: "u"}, {Provider: "oai", DeviceAuthId: "d", UserCode: "u", IntervalSeconds: 901}} {
		if _, err := s.PollDeviceFlow(t.Context(), request); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("error=%v", err)
		}
	}
	if _, err := s.GetDeviceFlowCode(t.Context(), &upstream.DeviceFlowRequest{Provider: "unknown"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("error=%v", err)
	}
}

func TestDeviceExchangeUsesPrivateConfigCopy(t *testing.T) {
	auth := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		expected := "http://localhost/oauth/callback"
		if r.Form.Get("code") == "device-code" {
			expected = "https://auth.openai.com/deviceauth/callback"
		}
		if r.Form.Get("redirect_uri") != expected || r.Form.Get("code_verifier") != "verifier" {
			t.Errorf("wrong exchange parameters: %v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"access","token_type":"Bearer"}`)
	}, "")
	directory := upstreamdirectory.New(testRPC(t, auth, time.Second), time.Second)
	for _, flowType := range []string{"device", "browser"} {
		if _, err := directory.Exchange(t.Context(), flowType+"-code", "verifier", "oai", flowType); err != nil {
			t.Fatal(err)
		}
	}
	if auth.OauthConfig.RedirectURL != "http://localhost/oauth/callback" {
		t.Fatal("shared redirect changed")
	}
	if _, err := directory.Exchange(t.Context(), "code", "verifier", "oai", "typo"); !errors.Is(err, upstreamdirectory.ErrInvalidExchange) {
		t.Fatalf("invalid flow: %v", err)
	}
}
