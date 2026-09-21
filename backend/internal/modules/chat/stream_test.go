package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	upstreamdirectory "erent/internal/directory/upstream"
	"erent/internal/modules/chat/route"
	"erent/internal/modules/translator"
	"erent/internal/rpc/upstream"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

type streamClient struct {
	upstream.UpstreamServiceClient
	chat func(context.Context, *upstream.ChatRequest) (grpc.ServerStreamingClient[upstream.ChatResponse], error)
}

func (c streamClient) ChatStream(ctx context.Context, req *upstream.ChatRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[upstream.ChatResponse], error) {
	return c.chat(ctx, req)
}

type eventStream struct {
	grpc.ClientStream
	lines []string
}

func (s *eventStream) Recv() (*upstream.ChatResponse, error) {
	if len(s.lines) == 0 {
		return nil, io.EOF
	}
	line := s.lines[0]
	s.lines = s.lines[1:]
	return &upstream.ChatResponse{Content: line}, nil
}

func TestStreamConvertsCodexToEachClient(t *testing.T) {
	for _, format := range []translator.Format{translator.FormatClaude, translator.FormatOpenAI, translator.FormatOpenAIResponse} {
		t.Run(format.String(), func(t *testing.T) {
			client := streamClient{chat: func(_ context.Context, req *upstream.ChatRequest) (grpc.ServerStreamingClient[upstream.ChatResponse], error) {
				var body map[string]any
				if json.Unmarshal(req.Data, &body) != nil || body["input"] == nil || body["stream"] != true {
					t.Errorf("request not converted: %s", req.Data)
				}
				return &eventStream{lines: []string{
					`data: {"type":"response.created","response":{"id":"resp_test","model":"gpt-5.5","created_at":1}}`, "",
					`data: {"type":"response.content_part.added","part":{"type":"output_text","text":""}}`, "",
					`data: {"type":"response.output_text.delta","delta":"hello"}`, "",
					`data: {"type":"response.content_part.done"}`, "",
					`data: {"type":"response.completed","response":{"id":"resp_test","model":"gpt-5.5","status":"completed","usage":{"input_tokens":1,"output_tokens":1}}}`, "",
				}}, nil
			}}
			service := ChatService{directory: upstreamdirectory.New(client, time.Second)}
			body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"}],"stream":true}`)
			if format == translator.FormatOpenAIResponse {
				body = []byte(`{"model":"gpt-5.5","input":"hello","stream":true}`)
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			data, errs := service.ChatStream(ctx, body, Credential{Mode: route.AuthModeAccessToken, Secret: "test"}, format, "gpt-5.5")
			var result strings.Builder
			for event := range data {
				result.Write(event)
			}
			for err := range errs {
				t.Fatal(err)
			}
			text := result.String()
			if !strings.Contains(text, "hello") || !strings.HasSuffix(text, "\n\n") {
				t.Fatalf("invalid stream: %s", text)
			}
			for _, line := range strings.Split(text, "\n") {
				if strings.HasPrefix(line, "data: ") && line != "data: [DONE]" && !json.Valid([]byte(strings.TrimPrefix(line, "data: "))) {
					t.Fatalf("invalid SSE JSON: %s", line)
				}
			}
			switch format {
			case translator.FormatClaude:
				if strings.Count(text, "event: content_block_start") != 1 || !strings.Contains(text, "event: message_stop") {
					t.Fatalf("Claude state lost: %s", text)
				}
			case translator.FormatOpenAI:
				if !strings.Contains(text, "chat.completion.chunk") || !strings.HasSuffix(text, "data: [DONE]\n\n") {
					t.Fatalf("invalid OpenAI stream: %s", text)
				}
			case translator.FormatOpenAIResponse:
				if !strings.Contains(text, "response.completed") {
					t.Fatalf("missing Responses completion: %s", text)
				}
			}
		})
	}
}

func TestStreamErrorsReturnWithoutReceiver(t *testing.T) {
	want := errors.New("RPC creation failed")
	for _, model := range []string{"missing-route", "gpt-5.5"} {
		t.Run(model, func(t *testing.T) {
			service := ChatService{directory: upstreamdirectory.New(streamClient{chat: func(context.Context, *upstream.ChatRequest) (grpc.ServerStreamingClient[upstream.ChatResponse], error) {
				return nil, want
			}}, time.Second)}
			done := make(chan struct{})
			go func() {
				defer close(done)
				data, errs := service.ChatStream(t.Context(), []byte(`{}`), Credential{Mode: route.AuthModeAccessToken}, translator.FormatClaude, model)
				for range data {
				}
				err := <-errs
				if model == "missing-route" && !errors.Is(err, errors.ErrUnsupported) || model != "missing-route" && !errors.Is(err, want) {
					t.Errorf("unexpected error: %v", err)
				}
			}()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("error path blocked")
			}
		})
	}
}

func TestStreamCancellationUnblocksSender(t *testing.T) {
	service := ChatService{directory: upstreamdirectory.New(streamClient{chat: func(context.Context, *upstream.ChatRequest) (grpc.ServerStreamingClient[upstream.ChatResponse], error) {
		return &eventStream{lines: []string{`data: {"type":"response.created","response":{"id":"test"}}`}}, nil
	}}, time.Second)}
	ctx, cancel := context.WithCancel(t.Context())
	data, errs := service.ChatStream(ctx, []byte(`{}`), Credential{Mode: route.AuthModeAccessToken}, translator.FormatClaude, "gpt-5.5")
	cancel()
	done := make(chan struct{})
	go func() {
		for range data {
		}
		for range errs {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled stream leaked")
	}
}

func TestChatRoutesRejectInvalidInputAndUninitializedAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterChatRoutes(router.Group(""), NewChatHandler(NewChatService(nil)))
	for _, path := range []string{"/v1/messages", "/v1/responses", "/v1/chat/completions"} {
		for _, body := range []string{`{`, `{"stream":true}`, `{"model":"gpt-5.5","stream":true}`} {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer test")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			want := http.StatusBadRequest
			if body == `{"model":"gpt-5.5","stream":true}` {
				want = http.StatusInternalServerError
			}
			if w.Code != want || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("%s: status=%d, headers=%v", path, w.Code, w.Header())
			}
		}
	}
}
