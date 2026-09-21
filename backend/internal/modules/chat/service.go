package chat

import (
	"bytes"
	"context"
	"encoding/json"
	upstreamdirectory "erent/internal/directory/upstream"
	"erent/internal/modules/chat/route"
	"erent/internal/modules/translator"
	"erent/internal/rpc/upstream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"strings"

	"google.golang.org/grpc"
)

type ChatService struct {
	directory *upstreamdirectory.Directory
}

func NewChatService(directory *upstreamdirectory.Directory) *ChatService {
	return &ChatService{directory: directory}
}

type Credential struct {
	Mode   route.AuthMode // access_token / api_key
	Secret string
}

func (c *ChatService) ChatStream(ctx context.Context, body []byte, credential Credential, clientFormat translator.Format, model string) (<-chan []byte, <-chan error) {
	respchan := make(chan []byte)
	errchan := make(chan error, 1)
	selectedRoute, err := route.GetRoute(model, credential.Mode, clientFormat)
	if err != nil {
		errchan <- err
		close(respchan)
		close(errchan)
		return respchan, errchan
	}
	reqbody := translator.TranslateRequest(
		clientFormat,
		selectedRoute.UpstreamFormat,
		model,
		body,
		true,
	)
	go func() {
		defer close(respchan)
		defer close(errchan)
		rpcStream, err := chatStream(ctx, reqbody, credential, selectedRoute, c.directory)
		if err != nil {
			errchan <- err
			return
		}
		var state any
		for {
			chunk, err := rpcStream.Recv()
			if err == io.EOF {
				if clientFormat == translator.FormatOpenAI && selectedRoute.UpstreamFormat == translator.FormatCodex {
					select {
					case respchan <- []byte("data: [DONE]\n\n"):
					case <-ctx.Done():
					}
				}
				break
			}
			if err != nil {
				// 按是否已经开始写响应，处理 HTTP 错误或流内错误。
				errchan <- err
				return
			}

			var events [][]byte
			if clientFormat == selectedRoute.UpstreamFormat {
				// RPC 按行传输，Scanner 已去除换行；透传时逐行恢复。
				events = [][]byte{[]byte(chunk.Content + "\n")}
			} else {
				if chunk.Content == "" {
					continue
				}
				events = translator.TranslateStream(
					ctx,
					selectedRoute.UpstreamFormat,
					clientFormat,
					model,
					body,
					reqbody,
					[]byte(chunk.Content),
					&state,
				)
				for i, event := range events {
					if len(event) == 0 {
						continue
					}
					if clientFormat == translator.FormatOpenAI {
						events[i] = append(append([]byte("data: "), event...), '\n', '\n')
					} else if !bytes.HasSuffix(event, []byte("\n\n")) {
						events[i] = append(bytes.TrimRight(event, "\r\n"), '\n', '\n')
					}
				}
			}

			for _, event := range events {
				if len(event) == 0 {
					continue
				}
				select {
				case respchan <- event:
				// 正常发送
				case <-ctx.Done():
					return
				}

			}
		}
	}()
	return respchan, errchan
}

func chatStream(ctx context.Context, body []byte, credential Credential, route route.Route, directory *upstreamdirectory.Directory) (grpc.ServerStreamingClient[upstream.ChatResponse], error) {
	return directory.ChatStream(ctx, body, credential.Secret, string(credential.Mode), route.Endpoint,
		route.AuthHeader, route.AuthPrefix, route.Headers)
}

func (c *ChatService) ChatNonStream(ctx context.Context, body []byte, credential Credential, clientFormat translator.Format, model string) (upstream.ChatResponse, error) {
	selectedRoute, err := route.GetRoute(model, credential.Mode, clientFormat)
	if err != nil {
		return upstream.ChatResponse{}, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if selectedRoute.ForceStream && selectedRoute.UpstreamFormat != translator.FormatCodex && selectedRoute.UpstreamFormat != translator.FormatOpenAIResponse {
		return upstream.ChatResponse{}, status.Error(codes.InvalidArgument, "stream aggregation is unsupported for this upstream format")
	}
	reqbody := translator.TranslateRequest(clientFormat, selectedRoute.UpstreamFormat, model, body, selectedRoute.ForceStream)
	// 同协议转换会透传原文，这里也必须显式设置实际发送给上游的 stream。
	var fields map[string]json.RawMessage
	if json.Unmarshal(reqbody, &fields) != nil || fields == nil {
		return upstream.ChatResponse{}, status.Error(codes.InvalidArgument, "invalid request JSON")
	}
	fields["stream"], _ = json.Marshal(selectedRoute.ForceStream)
	reqbody, _ = json.Marshal(fields)
	var raw []byte
	if selectedRoute.ForceStream {
		stream, err := chatStream(ctx, reqbody, credential, selectedRoute, c.directory)
		if err != nil {
			return upstream.ChatResponse{}, err
		}
		result, err := streamtoNonStream(stream)
		if err != nil {
			return upstream.ChatResponse{}, err
		}
		raw = []byte(result.Content)
		if selectedRoute.UpstreamFormat == translator.FormatOpenAIResponse {
			var event struct {
				Response json.RawMessage `json:"response"`
			}
			_ = json.Unmarshal(raw, &event)
			raw = event.Response
		}
	} else {
		result, err := c.directory.ChatNonStream(ctx, reqbody, credential.Secret, string(credential.Mode), selectedRoute.Endpoint,
			selectedRoute.AuthHeader, selectedRoute.AuthPrefix, selectedRoute.Headers)
		if err != nil {
			return upstream.ChatResponse{}, err
		}
		raw = []byte(result.Content)
	}
	if clientFormat != selectedRoute.UpstreamFormat {
		raw = translator.TranslateNonStream(ctx, selectedRoute.UpstreamFormat, clientFormat, model, body, reqbody, raw)
	}
	if !json.Valid(raw) || !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		return upstream.ChatResponse{}, status.Error(codes.Internal, "invalid translated response")
	}
	return upstream.ChatResponse{Content: string(raw)}, nil
}

func streamtoNonStream(stream grpc.ServerStreamingClient[upstream.ChatResponse]) (upstream.ChatResponse, error) {
	var data []string
	items := make(map[int]json.RawMessage)
	consume := func() ([]byte, error) {
		if len(data) == 0 {
			return nil, nil
		}
		raw := []byte(strings.Join(data, "\n"))
		data = nil
		var event struct {
			Type        string                     `json:"type"`
			Response    map[string]json.RawMessage `json:"response"`
			OutputIndex int                        `json:"output_index"`
			Item        json.RawMessage            `json:"item"`
		}
		if json.Unmarshal(raw, &event) != nil {
			return nil, status.Error(codes.Internal, "invalid upstream stream event")
		}
		switch event.Type {
		case "response.failed", "error":
			return nil, status.Error(codes.Internal, "upstream generation failed")
		case "response.output_item.done":
			if event.OutputIndex < 0 || !json.Valid(event.Item) {
				return nil, status.Error(codes.Internal, "invalid output item")
			}
			items[event.OutputIndex] = event.Item
		case "response.completed", "response.incomplete":
			if event.Response == nil {
				return nil, status.Error(codes.Internal, "missing final response")
			}
			output, exists := event.Response["output"]
			var finalItems []json.RawMessage
			if exists && (json.Unmarshal(output, &finalItems) != nil || string(output) == "null") {
				return nil, status.Error(codes.Internal, "invalid final output")
			}
			if !exists || len(finalItems) == 0 && len(items) > 0 {
				list := make([]json.RawMessage, len(items))
				for index := range list {
					item, ok := items[index]
					if !ok {
						return nil, status.Error(codes.Internal, "incomplete output items")
					}
					list[index] = item
				}
				if len(list) == 0 {
					return nil, status.Error(codes.Internal, "missing final output")
				}
				event.Response["output"], _ = json.Marshal(list)
			}
			return json.Marshal(struct {
				Type     string                     `json:"type"`
				Response map[string]json.RawMessage `json:"response"`
			}{event.Type, event.Response})
		}
		return nil, nil
	}
	for {
		chunk, err := stream.Recv()
		if err != nil && err != io.EOF {
			return upstream.ChatResponse{}, err
		}
		if err == io.EOF {
			result, parseErr := consume()
			if parseErr != nil {
				return upstream.ChatResponse{}, parseErr
			}
			if result != nil {
				return upstream.ChatResponse{Content: string(result)}, nil
			}
			return upstream.ChatResponse{}, status.Error(codes.Unavailable, "upstream stream ended before final response")
		}
		if chunk.Content == "" {
			result, err := consume()
			if err != nil {
				return upstream.ChatResponse{}, err
			}
			if result != nil {
				return upstream.ChatResponse{Content: string(result)}, nil
			}
		} else if strings.HasPrefix(chunk.Content, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(chunk.Content, "data:"), " "))
		}
	}
}
