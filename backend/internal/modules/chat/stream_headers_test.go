package chat

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	upstreamdirectory "erent/internal/directory/upstream"
	"erent/internal/modules/chat/route"
	"erent/internal/modules/translator"
	"erent/internal/rpc/upstream"
	"erent/internal/upstreamserver"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// 使用真实 gRPC 序列化和本地 HTTP 上游验证整个 Service 调用链。
func TestStreamRouteHeadersReachUpstream(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	rpcServer := grpc.NewServer()
	upstream.RegisterUpstreamServiceServer(rpcServer, upstreamserver.NewServer(nil, time.Second))
	go func() { _ = rpcServer.Serve(listener) }()
	t.Cleanup(func() { rpcServer.Stop(); _ = listener.Close() })
	conn, err := grpc.NewClient("passthrough:///headers-test",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	service := ChatService{directory: upstreamdirectory.New(upstream.NewUpstreamServiceClient(conn), time.Second)}

	for _, tc := range []struct {
		name           string
		mode           route.AuthMode
		header, prefix string
	}{
		{"access-bearer", route.AuthModeAccessToken, "Authorization", "Bearer "},
		{"key-bearer", route.AuthModeAPIKey, "Authorization", "Bearer "},
		{"key-custom", route.AuthModeAPIKey, "X-Api-Key", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"test","stream":true}`)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got, _ := io.ReadAll(r.Body)
				if r.Method != http.MethodPost || string(got) != string(body) {
					t.Error("request method or body changed")
				}
				if r.Header.Get(tc.header) != tc.prefix+"test-secret" {
					t.Error("route authentication header was not applied")
				}
				if r.Header.Get("X-Route-Version") != "test-v1" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "text/event-stream" {
					t.Error("fixed or default headers lost")
				}
				if tc.header != "Authorization" && r.Header.Get("Authorization") != "" {
					t.Error("unexpected bearer authentication")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"type\":\"response.created\"}\n\ndata: {\"type\":\"response.completed\"}\n\n")
			}))
			t.Cleanup(provider.Close)
			fixed := map[string]string{"X-Route-Version": "test-v1", strings.ToLower(tc.header): "stale-value"}
			route.Register(tc.name, tc.mode, translator.FormatOpenAIResponse, route.Route{
				Endpoint: provider.URL, UpstreamFormat: translator.FormatOpenAIResponse,
				AuthHeader: tc.header, AuthPrefix: tc.prefix, Headers: fixed,
			})
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			stream, errs := service.ChatStream(ctx, body, Credential{Mode: tc.mode, Secret: "test-secret"}, translator.FormatOpenAIResponse, tc.name)
			var lines []string
			for chunk := range stream {
				lines = append(lines, string(chunk))
			}
			for err := range errs {
				t.Fatal(err)
			}
			if strings.Join(lines, "") != "data: {\"type\":\"response.created\"}\n\ndata: {\"type\":\"response.completed\"}\n\n" {
				t.Fatal("SSE framing changed")
			}
			if len(lines) != 4 || !strings.Contains(lines[2], "response.completed") {
				t.Fatalf("stream was truncated: %v", lines)
			}
			if fixed[strings.ToLower(tc.header)] != "stale-value" {
				t.Fatal("credential was written into route configuration")
			}
		})
	}
}
