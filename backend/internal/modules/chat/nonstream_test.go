package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	upstreamdirectory "erent/internal/directory/upstream"
	"erent/internal/modules/apikey"
	"erent/internal/modules/chat/route"
	"erent/internal/modules/translator"
	"erent/internal/rpc/upstream"
	"erent/internal/security"
	"erent/internal/testdatabase"
	"erent/internal/upstreamserver"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func nonStreamService(t *testing.T) *ChatService {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	upstream.RegisterUpstreamServiceServer(srv, upstreamserver.NewServer(nil, time.Second))
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() { srv.Stop(); _ = listener.Close() })
	conn, err := grpc.NewClient("passthrough:///nonstream-test", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &ChatService{directory: upstreamdirectory.New(upstream.NewUpstreamServiceClient(conn), time.Second)}
}

func TestNonStreamDirectJSON(t *testing.T) {
	service := nonStreamService(t)
	for _, format := range []translator.Format{translator.FormatClaude, translator.FormatOpenAI, translator.FormatOpenAIResponse} {
		t.Run(format.String(), func(t *testing.T) {
			want := `{"id":"test","usage":{"total_tokens":7},"result":"hello"}`
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if gjson.GetBytes(body, "stream").Bool() || !gjson.GetBytes(body, "stream").Exists() {
					t.Error("upstream request is not explicitly non-streaming")
				}
				if gjson.GetBytes(body, "metadata.keep").String() != "yes" {
					t.Error("original request fields lost")
				}
				if r.Method != "POST" || r.Header.Get("Accept") != "application/json" || r.Header.Get("X-Api-Key") != "test-secret" || r.Header.Get("X-Version") != "v1" {
					t.Error("nonstream headers lost")
				}
				fmt.Fprint(w, want)
			}))
			t.Cleanup(provider.Close)
			model := "nonstream-" + format.String()
			route.Register(model, route.AuthModeAPIKey, format, route.Route{Endpoint: provider.URL, UpstreamFormat: format, AuthHeader: "X-Api-Key", Headers: map[string]string{"X-Version": "v1"}})
			result, err := service.ChatNonStream(t.Context(), []byte(`{"model":"test","metadata":{"keep":"yes"}}`), Credential{Mode: route.AuthModeAPIKey, Secret: "test-secret"}, format, model)
			if err != nil || result.Content != want {
				t.Fatalf("got %s, %v", result.Content, err)
			}
		})
	}
}

const finalResponse = `{"id":"resp_test","object":"response","model":"gpt-5.5","status":"completed","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"city\":\"Shanghai\"}"}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}`

func TestNonStreamForcedCodexConversion(t *testing.T) {
	service := nonStreamService(t)
	for _, format := range []translator.Format{translator.FormatClaude, translator.FormatOpenAI, translator.FormatOpenAIResponse} {
		t.Run(format.String(), func(t *testing.T) {
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if !gjson.GetBytes(body, "stream").Bool() || !gjson.GetBytes(body, "input").Exists() {
					t.Errorf("invalid forced request: %s", body)
				}
				if r.Header.Get("Authorization") != "Bearer test-secret" {
					t.Error("missing credential")
				}
				fmt.Fprintf(w, "data: {\"type\":\"response.completed\",\"response\":%s}\n\n", finalResponse)
			}))
			t.Cleanup(provider.Close)
			model := "forced-" + format.String()
			route.Register(model, route.AuthModeAccessToken, format, route.Route{Endpoint: provider.URL, UpstreamFormat: translator.FormatCodex, ForceStream: true, AuthHeader: "Authorization", AuthPrefix: "Bearer "})
			body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"}],"stream":false}`)
			if format == translator.FormatOpenAIResponse {
				body = []byte(`{"model":"gpt-5.5","input":"hello","stream":false}`)
			}
			result, err := service.ChatNonStream(t.Context(), body, Credential{Mode: route.AuthModeAccessToken, Secret: "test-secret"}, format, model)
			if err != nil {
				t.Fatal(err)
			}
			if !json.Valid([]byte(result.Content)) || strings.Contains(result.Content, "data:") {
				t.Fatalf("not JSON: %s", result.Content)
			}
			if gjson.GetBytes(body, "stream").Bool() {
				t.Fatal("original request mutated")
			}
			switch format {
			case translator.FormatClaude:
				if gjson.Get(result.Content, "content.0.text").String() != "hello" || gjson.Get(result.Content, "content.1.name").String() != "lookup" || gjson.Get(result.Content, "usage.output_tokens").Int() != 4 {
					t.Fatalf("Claude fields lost: %s", result.Content)
				}
			case translator.FormatOpenAI:
				if gjson.Get(result.Content, "choices.0.message.content").String() != "hello" || gjson.Get(result.Content, "choices.0.message.tool_calls.0.function.name").String() != "lookup" || gjson.Get(result.Content, "usage.total_tokens").Int() != 7 {
					t.Fatalf("OpenAI fields lost: %s", result.Content)
				}
			case translator.FormatOpenAIResponse:
				if gjson.Get(result.Content, "output.1.name").String() != "lookup" || gjson.Get(result.Content, "usage.total_tokens").Int() != 7 {
					t.Fatalf("Responses fields lost: %s", result.Content)
				}
			}
		})
	}
}

func TestNonStreamCollector(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		code  codes.Code
	}{
		{"early EOF", []string{`data: {"type":"response.created"}`, ""}, codes.Unavailable},
		{"failed", []string{`data: {"type":"response.failed","response":{"error":{"message":"secret"}}}`, ""}, codes.Internal},
		{"malformed", []string{`data: {`, ""}, codes.Internal},
		{"missing output", []string{`data: {"type":"response.completed","response":{}}`, ""}, codes.Internal},
		{"empty output", []string{`data: {"type":"response.completed","response":{"output":[]}}`, ""}, codes.OK},
		{"incomplete", []string{`data: {"type":"response.incomplete","response":{"output":[],"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`, ""}, codes.OK},
		{"output fallback", []string{`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message","content":[{"type":"output_text","text":"hello"}]}}`, "", `data: {"type":"response.completed","response":{"usage":{"output_tokens":1}}}`}, codes.OK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := streamtoNonStream(&eventStream{lines: tc.lines})
			if status.Code(err) != tc.code {
				t.Fatalf("got %v, want %v", err, tc.code)
			}
			if err != nil && (result.Content != "" || strings.Contains(err.Error(), "secret")) {
				t.Fatal("failed response leaked content")
			}
			if tc.name == "output fallback" && gjson.Get(result.Content, "response.output.0.content.0.text").String() != "hello" {
				t.Fatal("output item lost")
			}
		})
	}
}

func TestNonStreamUpstreamFailures(t *testing.T) {
	service := nonStreamService(t)
	for _, tc := range []struct {
		name   string
		status int
		body   string
		code   codes.Code
	}{
		{"unauthorized", 401, "secret provider error", codes.Unauthenticated},
		{"rate limit", 429, "secret provider error", codes.ResourceExhausted},
		{"invalid JSON", 200, "data: not json", codes.Internal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			t.Cleanup(provider.Close)
			route.Register(tc.name, route.AuthModeAPIKey, translator.FormatOpenAI, route.Route{Endpoint: provider.URL, UpstreamFormat: translator.FormatOpenAI, AuthHeader: "Authorization", AuthPrefix: "Bearer "})
			result, err := service.ChatNonStream(t.Context(), []byte(`{"model":"test"}`), Credential{Mode: route.AuthModeAPIKey, Secret: "test"}, translator.FormatOpenAI, tc.name)
			if status.Code(err) != tc.code || result.Content != "" || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unexpected result %s, %v", result.Content, err)
			}
		})
	}
}

func TestNonStreamCancellationReachesHTTP(t *testing.T) {
	service := nonStreamService(t)
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprint(force), func(t *testing.T) {
			started, stopped := make(chan struct{}), make(chan struct{})
			release := make(chan struct{})
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				close(started)
				select {
				case <-r.Context().Done():
					close(stopped)
				case <-release:
				}
			}))
			t.Cleanup(func() { close(release); provider.Close() })
			model := "cancel-nonstream-" + fmt.Sprint(force)
			route.Register(model, route.AuthModeAPIKey, translator.FormatOpenAIResponse, route.Route{Endpoint: provider.URL, UpstreamFormat: translator.FormatOpenAIResponse, ForceStream: force, AuthHeader: "X-Api-Key"})
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := service.ChatNonStream(ctx, []byte(`{"model":"test","stream":false}`), Credential{Mode: route.AuthModeAPIKey, Secret: "test"}, translator.FormatOpenAIResponse, model)
				done <- err
			}()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("HTTP request not started")
			}
			cancel()
			select {
			case err := <-done:
				if status.Code(err) != codes.Canceled {
					t.Fatalf("got %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("service did not cancel")
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("HTTP request did not cancel")
			}
		})
	}
}

type nonStreamClient struct {
	upstream.UpstreamServiceClient
	call func(context.Context, *upstream.ChatRequest) (*upstream.ChatResponse, error)
}

func (c nonStreamClient) ChatNonStream(ctx context.Context, req *upstream.ChatRequest, _ ...grpc.CallOption) (*upstream.ChatResponse, error) {
	return c.call(ctx, req)
}

func TestNonStreamHandlerWritesJSON(t *testing.T) {
	db := testdatabase.Open(t)
	if err := db.Exec("CREATE TABLE oauth_infos (id INTEGER PRIMARY KEY, type TEXT, disabled BOOLEAN, access_token TEXT, expired DATETIME)").Error; err != nil {
		t.Fatal(err)
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	cipher, err := security.Encrypt(key, []byte("handler-token"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO oauth_infos (id, type, disabled, access_token) VALUES (10, 'codex', FALSE, ?)", string(cipher)).Error; err != nil {
		t.Fatal(err)
	}
	apikey.CredentialInject(key)
	t.Cleanup(func() { apikey.CredentialInject(nil) })
	model := "handler-nonstream-test"
	route.Register(model, route.AuthModeAccessToken, translator.FormatOpenAI, route.Route{UpstreamFormat: translator.FormatOpenAI, AuthHeader: "Authorization"})
	for _, failure := range []bool{false, true} {
		client := nonStreamClient{call: func(_ context.Context, req *upstream.ChatRequest) (*upstream.ChatResponse, error) {
			if gjson.GetBytes(req.Data, "messages.0.content").String() != "hello" {
				t.Error("handler lost request body")
			}
			if failure {
				return nil, status.Error(codes.ResourceExhausted, "rate limited")
			}
			return &upstream.ChatResponse{Content: `{"id":"test","choices":[]}`}, nil
		}}
		handler := NewChatHandler(NewChatService(upstreamdirectory.New(client, time.Second)))
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("apikey_accounts", []apikey.ApikeyAccountItem{{ID: 10, Type: "codex", AccessToken: string(cipher)}})
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"handler-nonstream-test","messages":[{"role":"user","content":"hello"}],"stream":false}`))
		handler.Chat(c, w, translator.FormatOpenAI)
		if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
			t.Fatal("handler emitted SSE")
		}
		if failure {
			if w.Code != 429 {
				t.Fatalf("got %d", w.Code)
			}
		} else if w.Code != 200 || w.Body.String() != `{"id":"test","choices":[]}` {
			t.Fatalf("unexpected response %d %s", w.Code, w.Body.String())
		}
	}
}

func TestNonStreamDirectProtocolConversion(t *testing.T) {
	service := nonStreamService(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if gjson.GetBytes(body, "stream").Bool() || gjson.GetBytes(body, "messages.0.role").String() != "user" {
			t.Errorf("invalid request: %s", body)
		}
		fmt.Fprint(w, `{"id":"msg_test","type":"message","role":"assistant","model":"test","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":4}}`)
	}))
	t.Cleanup(provider.Close)
	route.Register("direct-convert", route.AuthModeAPIKey, translator.FormatOpenAI, route.Route{Endpoint: provider.URL, UpstreamFormat: translator.FormatClaude, AuthHeader: "X-Api-Key"})
	result, err := service.ChatNonStream(t.Context(), []byte(`{"model":"test","messages":[{"role":"user","content":"hello"}],"stream":false}`), Credential{Mode: route.AuthModeAPIKey, Secret: "test"}, translator.FormatOpenAI, "direct-convert")
	if err != nil || gjson.Get(result.Content, "choices.0.message.content").String() != "hello" || gjson.Get(result.Content, "usage.total_tokens").Int() != 7 {
		t.Fatalf("conversion failed: %s %v", result.Content, err)
	}
}
