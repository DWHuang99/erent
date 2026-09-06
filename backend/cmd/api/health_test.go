package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"erent/internal/config"
	"erent/internal/rpc/upstream"
	"erent/internal/testdatabase"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
)

func TestRegisterHealthRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database := testdatabase.Open(t)
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatalf("get database handle: %v", err)
	}
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	instances := &applicationInstances{
		sqlDatabase: sqlDatabase,
		redisClient: redisClient,
	}
	router := gin.New()
	registerHealthRoutes(router, config.Config{Environment: "test"}, instances)

	for _, path := range []string{"/", "/api/health", "/health/live", "/health/ready"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s returned %d, want %d", path, response.Code, http.StatusOK)
		}
		var body map[string]string
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode GET %s response: %v", path, err)
		}
		if body["status"] != "ok" || body["environment"] != "test" {
			t.Fatalf("unexpected GET %s response: %v", path, body)
		}
	}

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(func() { grpcServer.Stop(); _ = listener.Close() })
	connection, err := grpc.NewClient("passthrough:///upstream", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	instances.upstreamConnection = connection
	for _, test := range []struct {
		status healthpb.HealthCheckResponse_ServingStatus
		want   int
	}{
		{healthpb.HealthCheckResponse_NOT_SERVING, http.StatusServiceUnavailable},
		{healthpb.HealthCheckResponse_SERVING, http.StatusOK},
	} {
		healthServer.SetServingStatus(upstream.UpstreamService_ServiceDesc.ServiceName, test.status)
		probe := httptest.NewRecorder()
		router.ServeHTTP(probe, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
		if probe.Code != test.want {
			t.Fatalf("upstream status %v: readiness %d, want %d", test.status, probe.Code, test.want)
		}
	}
	grpcServer.Stop()
	for _, test := range []struct {
		path string
		want int
	}{{"/health/ready", 503}, {"/health/live", 200}} {
		probe := httptest.NewRecorder()
		router.ServeHTTP(probe, httptest.NewRequest(http.MethodGet, test.path, nil))
		if probe.Code != test.want {
			t.Fatalf("upstream offline: %s = %d", test.path, probe.Code)
		}
	}
	instances.upstreamConnection = nil
	redisServer.Close()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness returned %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
