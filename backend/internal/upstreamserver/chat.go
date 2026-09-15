package upstreamserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"erent/internal/rpc/upstream"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *server) ChatNonStream(ctx context.Context, request *upstream.ChatRequest) (*upstream.ChatResponse, error) {
	if request == nil || len(bytes.TrimSpace(request.Data)) == 0 || strings.TrimSpace(request.Secret) == "" || strings.TrimSpace(request.AuthHeader) == "" {
		return nil, status.Error(codes.InvalidArgument, "data, secret and auth header are required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, request.Endpoint, bytes.NewReader(request.Data))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid upstream request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for key, value := range request.Headers {
		req.Header.Set(key, value)
	}
	req.Header.Set(request.AuthHeader, request.AuthPrefix+request.Secret)
	resp, err := chathttpClient.Do(req)
	if err != nil {
		return nil, status.Error(chatStreamErrorCode(err), "upstream request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, status.Error(chatStreamErrorCode(&HTTPError{StatusCode: resp.StatusCode}), "upstream request failed")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status.Error(chatStreamErrorCode(err), "upstream response failed")
	}
	if !json.Valid(body) || !bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) {
		return nil, status.Error(codes.Internal, "invalid upstream JSON response")
	}
	return &upstream.ChatResponse{Content: string(body)}, nil
}

var chathttpClient = func() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 15 * time.Second
	// 保留 Transport 的连接和 TLS 超时；流的生命周期由请求 context 控制。
	return &http.Client{Transport: transport}
}()

// HTTPError 保留上游状态码供 gRPC 层分类，不携带可能包含敏感信息的响应体。
type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("upstream status: %d", e.StatusCode)
}

type Chunk struct {
	Content string
}

func Stream(
	ctx context.Context,
	body []byte,
	secret string,
	_ string, // 凭证模式在路由选择时使用，请求头由路由配置决定。
	handle func(Chunk) error,
	endpoint string,
	authHeader, authPrefix string,
	headers map[string]string,
) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	// 最后注入本次凭证，覆盖固定头中的同名值，不修改共享配置。
	req.Header.Set(authHeader, authPrefix+secret)

	resp, err := chathttpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{StatusCode: resp.StatusCode}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	terminalSeen := false
	for scanner.Scan() {
		data := scanner.Text()
		if strings.HasPrefix(data, "data:") {
			var event struct {
				Type string `json:"type"`
			}
			if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(data, "data:"))), &event) == nil {
				switch event.Type {
				case "response.completed", "response.incomplete", "response.failed":
					terminalSeen = true
				}
			}
		}

		if err := handle(Chunk{
			Content: data,
		}); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	if !terminalSeen {
		return io.ErrUnexpectedEOF
	}
	return nil
}
