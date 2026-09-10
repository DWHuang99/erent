package oauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	jwtservice "erent/internal/middleware/jwt"
	"erent/internal/modules/oauth/oidc"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
)

func TestLoginSelectsProviderAndStoresItInFlow(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	service := newTestOAuthService(client)
	service.oidcAuth["second"] = &oidc.OIDCAuth{OauthConfig: &oauth2.Config{ClientID: "second-client", Endpoint: oauth2.Endpoint{AuthURL: "https://second.example/authorize"}}}
	router := gin.New()
	router.GET("/login", func(c *gin.Context) { c.Set(jwtservice.UserIDContextKey, uint64(1)); NewOauthHandler(service).Login(c) })
	for _, provider := range []string{"oai", "second", "", "unknown"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/login?provider="+provider, nil))
		if provider == "" || provider == "unknown" {
			if w.Code != 400 {
				t.Fatalf("invalid provider: %d", w.Code)
			}
			continue
		}
		if w.Code != 302 {
			t.Fatalf("login: %d %s", w.Code, w.Body)
		}
		target, err := url.Parse(w.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		if provider == "second" && (target.Host != "second.example" || target.Query().Get("client_id") != "second-client") {
			t.Fatal("wrong provider config")
		}
		flow, err := service.PopFlow(target.Query().Get("state"), t.Context())
		if err != nil || flow.Provider != provider {
			t.Fatalf("wrong stored provider: %+v %v", flow, err)
		}
	}
	if len(server.Keys()) != 0 {
		t.Fatal("invalid provider created a flow")
	}
}

func TestCallbackUsesOnlyStoredProvider(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	service := newTestOAuthService(client)
	service.oidcAuth["second"] = service.oidcAuth["oai"]
	for _, provider := range []string{"second", "", "removed"} {
		stub := &exchangeStub{err: ErrExchangeRejected}
		service.directory = testDirectory(service, stub)
		flow := oidc.LoginFlow{Provider: provider, UserID: 1, Nonce: "nonce", Verifier: "verifier", ExpiresAt: time.Now().Add(time.Minute)}
		if err := service.StoreFlow("state", flow, t.Context()); err != nil {
			t.Fatal(err)
		}
		w := testOAuthCallback(service, "/callback?state=state&code=code&provider=oai")
		switch provider {
		case "second":
			if w.Code != 400 || stub.provider != "second" {
				t.Fatalf("request overrode flow provider: %d %+v", w.Code, stub)
			}
		case "":
			if w.Code != 400 || stub.provider != "" {
				t.Fatal("missing provider accepted")
			}
		case "removed":
			if w.Code != 503 || stub.provider != "" {
				t.Fatal("removed provider accepted")
			}
		}
	}
}
