package upstreamserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"erent/internal/modules/oauth/oidc"
	"erent/internal/rpc/upstream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func refreshRequest() *upstream.RefreshTokenRequest {
	return &upstream.RefreshTokenRequest{Provider: "oai", RefreshToken: "old-refresh"}
}

func TestRefreshValidatesRequestAndProvider(t *testing.T) {
	server := NewServer(nil, time.Second)
	if response, err := server.RefreshToken(t.Context(), nil); response != nil || status.Code(err) != codes.InvalidArgument {
		t.Fatalf("nil request: response=%v error=%v", response, err)
	}
	client := testRPC(t, nil, time.Second)
	for _, tc := range []struct {
		request *upstream.RefreshTokenRequest
		want    codes.Code
	}{
		{&upstream.RefreshTokenRequest{}, codes.InvalidArgument},
		{&upstream.RefreshTokenRequest{Provider: "oai", RefreshToken: " \t"}, codes.InvalidArgument},
		{&upstream.RefreshTokenRequest{Provider: " \t", RefreshToken: "refresh"}, codes.InvalidArgument},
		{refreshRequest(), codes.FailedPrecondition},
		{&upstream.RefreshTokenRequest{Provider: "unknown", RefreshToken: "refresh"}, codes.FailedPrecondition},
	} {
		response, err := client.RefreshToken(t.Context(), tc.request)
		if response != nil || status.Code(err) != tc.want {
			t.Fatalf("got %v, want %v", err, tc.want)
		}
	}
	client = testRPC(t, &oidc.OIDCAuth{}, time.Second)
	if _, err := client.RefreshToken(t.Context(), refreshRequest()); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("missing OAuth config: %v", err)
	}
}

func TestRefreshRoundTripPreservesCredentials(t *testing.T) {
	for _, rotated := range []bool{true, false} {
		t.Run(fmt.Sprintf("rotated=%t", rotated), func(t *testing.T) {
			var calls atomic.Int32
			auth := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Method != http.MethodPost || r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" || r.Form.Get("client_id") != "test-client" || r.Form.Get("client_secret") != "client-secret" {
					t.Error("incorrect refresh parameters")
				}
				for _, key := range []string{"code", "code_verifier", "code_challenge", "redirect_uri"} {
					if r.Form.Has(key) {
						t.Errorf("unexpected authorization-code parameter: %s", key)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				if rotated {
					_, _ = fmt.Fprint(w, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer","expires_in":3600,"id_token":"new-id"}`)
				} else {
					_, _ = fmt.Fprint(w, `{"access_token":"new-access","token_type":"Bearer"}`)
				}
			}, "client-secret")
			before := time.Now()
			response, err := testRPC(t, auth, time.Second).RefreshToken(t.Context(), refreshRequest())
			if err != nil {
				t.Fatal(err)
			}
			if response.AccessToken != "new-access" || response.TokenType != "Bearer" || calls.Load() != 1 {
				t.Fatal("refresh result incomplete or request repeated")
			}
			if rotated {
				if response.RefreshToken != "new-refresh" || response.IdToken != "new-id" || response.ExpiresAt == nil {
					t.Fatal("rotated fields missing")
				}
				expiry := response.ExpiresAt.AsTime()
				if expiry.Before(before.Add(3599*time.Second)) || expiry.After(time.Now().Add(3601*time.Second)) {
					t.Fatal("incorrect expiry")
				}
			} else if response.RefreshToken != "old-refresh" || response.IdToken != "" || response.ExpiresAt != nil {
				t.Fatal("original refresh token or optional fields not preserved")
			}
		})
	}
}

func TestRefreshErrorContract(t *testing.T) {
	for _, tc := range []struct {
		name       string
		httpStatus int
		body       string
		want       codes.Code
	}{
		{"invalid grant", 400, `{"error":"invalid_grant","error_description":"secret-refresh"}`, codes.Unauthenticated},
		{"unavailable", 503, `{"error":"temporarily_unavailable"}`, codes.Unavailable},
		{"rate limited", 429, `{"error":"slow_down"}`, codes.Unavailable},
		{"client configuration", 401, `{"error":"invalid_client","error_description":"secret-client"}`, codes.Internal},
		{"missing access token", 200, `{"refresh_token":"secret-refresh"}`, codes.Internal},
		{"malformed body", 502, `secret-provider-body`, codes.Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			auth := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.httpStatus)
				_, _ = fmt.Fprint(w, tc.body)
			}, "")
			response, err := testRPC(t, auth, time.Second).RefreshToken(t.Context(), refreshRequest())
			if response != nil || status.Code(err) != tc.want {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if status.Convert(err).Message() != "token refresh failed" || strings.Contains(err.Error(), "secret") {
				t.Fatal("provider details leaked")
			}
			if calls.Load() != 1 {
				t.Fatal("refresh token retried")
			}
		})
	}
}

func TestRefreshHonorsDeadlinesAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		clientTimeout, serverTimeout time.Duration
	}{
		{"server", 2 * time.Second, 50 * time.Millisecond},
		{"client", 50 * time.Millisecond, 2 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				select {
				case <-r.Context().Done():
				case <-time.After(3 * time.Second):
				}
			}, "")
			client := testRPC(t, auth, tc.serverTimeout)
			ctx, cancel := context.WithTimeout(t.Context(), tc.clientTimeout)
			defer cancel()
			started := time.Now()
			if _, err := client.RefreshToken(ctx, refreshRequest()); status.Code(err) != codes.DeadlineExceeded {
				t.Fatalf("deadline: %v", err)
			}
			if time.Since(started) > time.Second {
				t.Fatal("refresh exceeded deadline")
			}
			canceled, stop := context.WithCancel(t.Context())
			stop()
			// Direct call verifies server-side cancellation classification as well.
			server := NewServer(map[string]*oidc.OIDCAuth{"oai": auth}, time.Second)
			if response, err := server.RefreshToken(canceled, refreshRequest()); response != nil || status.Code(err) != codes.Canceled {
				t.Fatalf("cancellation: %v", err)
			}
		})
	}
}
