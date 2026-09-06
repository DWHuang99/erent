package upstreamserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"erent/internal/config"
	upstreamdirectory "erent/internal/directory/upstream"
	"erent/internal/modules/oauth"
	"erent/internal/modules/oauth/oidc"
	"erent/internal/rpc/upstream"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func testProvider(t *testing.T, handler http.HandlerFunc, clientSecret string) *oidc.OIDCAuth {
	t.Helper()
	var provider *httptest.Server
	provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": provider.URL, "authorization_endpoint": provider.URL + "/authorize", "token_endpoint": provider.URL + "/token", "token_endpoint_auth_methods_supported": []string{"client_secret_post"}})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(provider.Close)
	auth, err := oidc.NewOIDCAuth(t.Context(), config.OIDCConfig{Issuer: provider.URL, ClientID: "test-client", ClientSecret: clientSecret, RedirectURL: "http://localhost/oai/callback"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return auth
}
func testRPC(t *testing.T, auth *oidc.OIDCAuth, timeout time.Duration) upstream.UpstreamServiceClient {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	upstream.RegisterUpstreamServiceServer(server, NewServer(map[string]*oidc.OIDCAuth{"oai": auth}, timeout))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	connection, err := grpc.NewClient("passthrough:///upstream", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return upstream.NewUpstreamServiceClient(connection)
}
func validRequest() *upstream.ExchangeCodeRequest {
	return &upstream.ExchangeCodeRequest{Provider: "oai", Code: "authorization-code", CodeVerifier: "pkce-verifier"}
}
func TestExchangeRoundTripPreservesTokenAndPKCE(t *testing.T) {
	var calls atomic.Int32
	auth := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("code_verifier") != "pkce-verifier" || r.Form.Get("code_challenge") != "" || r.Form.Get("code") != "authorization-code" || r.Form.Get("redirect_uri") != "http://localhost/oai/callback" || r.Form.Get("client_id") != "test-client" || r.Form.Get("client_secret") != "client-secret" {
			t.Errorf("unexpected token request parameters")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600}`)
	}, "client-secret")
	directory := upstreamdirectory.New(testRPC(t, auth, time.Second), time.Second)
	before := time.Now()
	token, err := directory.Exchange(t.Context(), "authorization-code", "pkce-verifier", "oai")
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "access" || token.RefreshToken != "refresh" || token.TokenType != "Bearer" || token.Expiry.Before(before.Add(3599*time.Second)) || token.Expiry.After(time.Now().Add(3601*time.Second)) {
		t.Fatalf("token fields were lost: %+v", token)
	}
	if calls.Load() != 1 {
		t.Fatalf("exchange count = %d", calls.Load())
	}
}
func TestExchangeErrorContract(t *testing.T) {
	tests := []struct {
		name       string
		httpStatus int
		body       string
		want       error
	}{
		{"invalid grant", 400, `{"error":"invalid_grant","error_description":"secret-code"}`, oauth.ErrExchangeRejected},
		{"provider unavailable", 503, `{"error":"temporarily_unavailable"}`, oauth.ErrUpstreamUnavailable},
		{"rate limited", 429, `{"error":"slow_down"}`, oauth.ErrUpstreamUnavailable},
		{"provider configuration", 401, `{"error":"invalid_client","error_description":"secret-value"}`, oauth.ErrExchangeFailed},
		{"invalid response", 200, `{"refresh_token":"no-access"}`, oauth.ErrExchangeFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			auth := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.httpStatus)
				_, _ = fmt.Fprint(w, tt.body)
			}, "")
			client := testRPC(t, auth, time.Second)
			directory := upstreamdirectory.New(client, time.Second)
			token, err := directory.Exchange(t.Context(), "authorization-code", "pkce-verifier", "oai")
			if token != nil || !errors.Is(err, tt.want) {
				t.Fatalf("got token %v error %v; want %v", token, err, tt.want)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("provider response leaked")
			}
			if calls.Load() != 1 {
				t.Fatalf("authorization code was retried %d times", calls.Load())
			}
		})
	}
}
func TestExchangeValidatesRequestAndDisabledProvider(t *testing.T) {
	client := testRPC(t, nil, time.Second)
	for _, test := range []struct {
		request *upstream.ExchangeCodeRequest
		want    codes.Code
	}{
		{&upstream.ExchangeCodeRequest{}, codes.InvalidArgument},
		{validRequest(), codes.FailedPrecondition},
		{&upstream.ExchangeCodeRequest{Provider: "unknown", Code: "code", CodeVerifier: "verifier"}, codes.FailedPrecondition},
	} {
		_, err := client.ExchangeCode(t.Context(), test.request)
		if status.Code(err) != test.want {
			t.Fatalf("code = %v, want %v", status.Code(err), test.want)
		}
	}
	directory := upstreamdirectory.New(client, time.Second)
	if _, err := directory.Exchange(t.Context(), "code", "verifier", "oai"); !errors.Is(err, oauth.ErrProviderUnavailable) {
		t.Fatalf("disabled provider: %v", err)
	}
	if _, err := directory.Exchange(t.Context(), "", "verifier", "oai"); !errors.Is(err, oauth.ErrInvalidExchange) {
		t.Fatalf("invalid code: %v", err)
	}
	_, err := client.RefreshToken(t.Context(), &upstream.RefreshTokenRequest{})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("refresh placeholder = %v", err)
	}
}
func TestExchangeDeadlines(t *testing.T) {
	for _, test := range []struct {
		name                         string
		clientTimeout, serverTimeout time.Duration
	}{
		{"directory deadline", 100 * time.Millisecond, 5 * time.Second},
		{"server deadline", 5 * time.Second, 100 * time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			auth := testProvider(t, func(w http.ResponseWriter, r *http.Request) { _ = r.ParseForm(); <-r.Context().Done() }, "")
			directory := upstreamdirectory.New(testRPC(t, auth, test.serverTimeout), test.clientTimeout)
			started := time.Now()
			_, err := directory.Exchange(t.Context(), "code", "verifier", "oai")
			if !errors.Is(err, oauth.ErrExchangeTimeout) {
				t.Fatalf("deadline error = %v", err)
			}
			if time.Since(started) > 2*time.Second {
				t.Fatal("deadline did not bound request")
			}
		})
	}
}
func TestCallbackUsesDirectoryAndConsumesState(t *testing.T) {
	var calls atomic.Int32
	auth := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("code_verifier") != "saved-verifier" {
			t.Error("stored PKCE verifier was not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"access","token_type":"Bearer"}`)
	}, "")
	directory := upstreamdirectory.New(testRPC(t, auth, time.Second), time.Second)
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	service := oauth.NewOauthService(redisClient, auth, directory, "oai")
	if err := service.StoreFlow("state", oidc.LoginFlow{Verifier: "saved-verifier", ExpiresAt: time.Now().Add(time.Minute)}, t.Context()); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	oauth.RegisterOauthRoutes(router.Group("/oai"), oauth.NewOauthHandler(service))
	for i, want := range []int{http.StatusOK, http.StatusBadRequest} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/oai/callback?state=state&code=authorization-code", nil))
		if response.Code != want {
			t.Fatalf("callback %d = %d: %s", i, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "access") {
			t.Fatal("token leaked to browser")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("provider calls = %d", calls.Load())
	}
}
func TestServeStopsOnCancellationAndReportsBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := config.UpstreamServerConfig{Address: listener.Addr().String(), RequestTimeout: time.Second, ShutdownTimeout: time.Second}
	if err := Serve(t.Context(), cfg, nil); err == nil {
		t.Fatal("bind conflict ignored")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cfg.Address = "127.0.0.1:0"
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg, nil) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not exit")
	}
}
