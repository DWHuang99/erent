package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGatewayHealthIsLocalAndOtherRoutesAreProxied(t *testing.T) {
	upstreamCalls := 0
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.WriteHeader(http.StatusCreated)
	})
	router := newGatewayRouter(upstream)

	live := httptest.NewRecorder()
	router.ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if live.Code != http.StatusOK || upstreamCalls != 0 {
		t.Fatalf("live status = %d, upstream calls = %d", live.Code, upstreamCalls)
	}

	ping := httptest.NewRecorder()
	router.ServeHTTP(ping, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if ping.Code != http.StatusOK || upstreamCalls != 0 {
		t.Fatalf("ping status = %d, upstream calls = %d", ping.Code, upstreamCalls)
	}

	api := httptest.NewRecorder()
	router.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify", nil))
	if api.Code != http.StatusCreated || upstreamCalls != 1 {
		t.Fatalf("api status = %d, upstream calls = %d", api.Code, upstreamCalls)
	}
}

func TestLoadGatewayConfigRejectsInvalidValues(t *testing.T) {
	_, err := loadGatewayConfig(func(key string) (string, bool) {
		if key == "GATEWAY_ADDR" {
			return "invalid", true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("expected invalid address error")
	}
}
