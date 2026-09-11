package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	upstreamdirectory "erent/internal/directory/upstream"
	jwtservice "erent/internal/middleware/jwt"
	"erent/internal/modules/oauth/oidc"
	"erent/internal/rpc/upstream"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type deviceHandlerRPC struct {
	*serviceRPCClient
	startErr, pollErr error
	polls             int
	lastPoll          *upstream.PollDeviceFlowRequest
	flowType          string
}

func (c *deviceHandlerRPC) GetDeviceFlowCode(context.Context, *upstream.DeviceFlowRequest, ...grpc.CallOption) (*upstream.DeviceFlowResponse, error) {
	if c.startErr != nil {
		return nil, c.startErr
	}
	return &upstream.DeviceFlowResponse{DeviceAuthId: "device", UserCode: "user-code", IntervalSeconds: 5, VerificationUrl: "https://auth.openai.com/codex/device"}, nil
}
func (c *deviceHandlerRPC) PollDeviceFlow(_ context.Context, r *upstream.PollDeviceFlowRequest, _ ...grpc.CallOption) (*upstream.DeviceAuthorizationResponse, error) {
	c.polls++
	c.lastPoll = r
	if c.pollErr != nil {
		return nil, c.pollErr
	}
	return &upstream.DeviceAuthorizationResponse{AuthorizationCode: "code", CodeVerifier: "verifier"}, nil
}
func (c *deviceHandlerRPC) ExchangeCode(ctx context.Context, r *upstream.ExchangeCodeRequest, opts ...grpc.CallOption) (*upstream.TokenResponse, error) {
	c.flowType = r.FlowType
	return c.serviceRPCClient.ExchangeCode(ctx, r, opts...)
}

func TestDeviceHandlersBindOwnerAndSaveVerifiedIdentity(t *testing.T) {
	service, db, key := persistenceService(t)
	redisServer := miniredis.RunT(t)
	service.redisClient = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { service.redisClient.Close() })
	token := signedToken(t, key, jwt.MapClaims{"nonce": ""})
	rpc := &deviceHandlerRPC{serviceRPCClient: &serviceRPCClient{service: service, source: &tokenExchange{token: token}}}
	service.directory = upstreamdirectory.New(rpc, time.Second)
	handler := NewOauthHandler(service)
	router := gin.New()
	owner := uint64(1)
	router.Use(func(c *gin.Context) { c.Set(jwtservice.UserIDContextKey, owner); c.Next() })
	router.POST("/login", handler.LoginDeviceFlow)
	router.POST("/complete", handler.CallbackDeviceFlow)
	send := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	login := send("/login?provider=oai", "")
	if login.Code != 200 || !strings.Contains(login.Body.String(), "verification_url") {
		t.Fatalf("login: %s", login.Body.String())
	}
	owner = 2
	if w := send("/complete?provider=oai", `{"device_auth_id":"device"}`); w.Code != 400 || rpc.polls != 0 {
		t.Fatalf("cross-owner: %d polls=%d", w.Code, rpc.polls)
	}
	owner = 1
	// Browser-supplied code and interval cannot override the stored values.
	complete := send("/complete?provider=oai", `{"device_auth_id":"device","user_code":"tampered","interval":1}`)
	if complete.Code != 200 {
		t.Fatalf("complete: %d %s", complete.Code, complete.Body.String())
	}
	if rpc.flowType != "device" || rpc.lastPoll.UserCode != "user-code" || rpc.lastPoll.IntervalSeconds != 5 {
		t.Fatal("flow parameters not preserved")
	}
	var model OAuthInfo
	if err := db.First(&model).Error; err != nil || model.UserID != 1 {
		t.Fatalf("saved account: %+v %v", model, err)
	}
	if w := send("/complete?provider=oai", `{"device_auth_id":"device"}`); w.Code != 400 || rpc.polls != 1 {
		t.Fatal("device flow replay accepted")
	}
	// The browser still requires nonce; device mode still rejects invalid signatures.
	if err := service.SaveToken(t.Context(), token, oidc.LoginFlow{Provider: "oai", UserID: 1}, true); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("browser nonce: %v", err)
	}
	badToken := token.WithExtra(map[string]any{"id_token": "not-a-signed-token"})
	if err := service.SaveToken(t.Context(), badToken, oidc.LoginFlow{Provider: "oai", UserID: 1}, false); !errors.Is(err, ErrInvalidIDToken) {
		t.Fatalf("device identity: %v", err)
	}
	// Upstream failures return a single error response, never dereference nil results.
	rpc.startErr = status.Error(codes.Unavailable, "private-detail")
	if w := send("/login?provider=oai", ""); w.Code != 503 || !json.Valid(w.Body.Bytes()) {
		t.Fatalf("start failure: %s", w.Body.String())
	}
	rpc.startErr = nil
	send("/login?provider=oai", "")
	rpc.pollErr = status.Error(codes.DeadlineExceeded, "private-detail")
	if w := send("/complete?provider=oai", `{"device_auth_id":"device"}`); w.Code != 504 || !json.Valid(w.Body.Bytes()) {
		t.Fatalf("poll failure: %s", w.Body.String())
	}
}

func TestDeviceFlowExpiresAndConsumesOnce(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { client.Close() })
	service := newTestOAuthService(client)
	flow := deviceLoginFlow{Provider: "oai", UserID: 1, DeviceAuthID: "device", UserCode: "code", Interval: 5, ExpiresAt: time.Now().Add(time.Minute)}
	if err := service.storeDeviceFlow(t.Context(), flow); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := service.popDeviceFlow(t.Context(), "device", "oai", 1); results <- err }()
	}
	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, ErrInvalidOAuthState)) || (second == nil && errors.Is(first, ErrInvalidOAuthState))) {
		t.Fatalf("concurrent consume: %v %v", first, second)
	}
	if err := service.storeDeviceFlow(t.Context(), flow); err != nil {
		t.Fatal(err)
	}
	redisServer.FastForward(2 * time.Minute)
	if _, err := service.popDeviceFlow(t.Context(), "device", "oai", 1); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("expired flow: %v", err)
	}
}
