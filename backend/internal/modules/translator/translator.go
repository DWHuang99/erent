// Package translator 封装不同协议之间的请求和响应转换。
package translator

import (
	"context"
	"encoding/json"

	sdktr "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	_ "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator/builtin"
)

// Format 表示协议格式；Codex 与标准 OpenAI Responses 分别使用不同格式。
type Format = sdktr.Format

const (
	FormatOpenAI         = sdktr.FormatOpenAI
	FormatOpenAIResponse = sdktr.FormatOpenAIResponse
	FormatClaude         = sdktr.FormatClaude
	FormatGemini         = sdktr.FormatGemini
	FormatCodex          = sdktr.FormatCodex
	FormatAntigravity    = sdktr.FormatAntigravity
)

// TranslateRequest 将客户端 from 协议的请求转换为上游 to 协议。
// model 为实际请求的上游模型。未注册的转换沿用 SDK 回退行为：保留正文并更新 model。
func TranslateRequest(from, to Format, model string, raw []byte, stream bool) []byte {
	if from == to {
		return raw
	}
	return sdktr.TranslateRequest(
		from,
		to,
		model,
		raw,
		stream,
	)
}

// TranslateStream 将上游 from 协议的流片段转换为客户端 to 协议，方向与请求相反。
// originalReq、translatedReq 分别为客户端原始请求和发往上游的转换后请求。
// 调用方为每个请求声明 var state any，所有片段顺序传入同一个非 nil 的 &state；
// 不同请求或不同协议对不能共享 state。
// raw 和返回片段的封装格式遵循对应 SDK 转换器，不能统一视为 JSON 或完整 SSE。
// 例如 Codex -> Claude 的 raw 需带 data: 前缀，返回值已包含 SSE 事件格式。
// 未注册的转换原样返回 raw。
func TranslateStream(
	ctx context.Context,
	from, to Format,
	model string,
	originalReq, translatedReq, raw []byte,
	state *any,
) [][]byte {
	return sdktr.TranslateStream(
		ctx,
		from,
		to,
		model,
		originalReq,
		translatedReq,
		raw,
		state,
	)
}

// TranslateNonStream 将上游 from 协议的完整响应转换为客户端 to 协议，方向与请求相反。
// originalReq、translatedReq 分别为客户端原始请求和发往上游的转换后请求。
// raw 遵循对应 SDK 转换器的输入格式。例如 Codex -> Claude 需要包含 response 字段的
// response.completed 或 response.incomplete 事件 JSON，不带 data: 前缀。
// 本函数不负责聚合流片段；未注册的转换原样返回 raw。
func TranslateNonStream(
	ctx context.Context,
	from, to Format,
	model string,
	originalReq, translatedReq, raw []byte,
) []byte {
	// SDK 的 Claude -> OpenAI 非流式转换器接收完整 SSE，而 HTTP 非流式返回 Message JSON。
	if from == FormatClaude && (to == FormatOpenAI || to == FormatOpenAIResponse) && json.Valid(raw) {
		raw = claudeMessageEvents(raw)
		if len(raw) == 0 {
			return nil
		}
	}
	var state any
	return sdktr.TranslateNonStream(
		ctx,
		from,
		to,
		model,
		originalReq,
		translatedReq,
		raw,
		&state,
	)
}

func claudeMessageEvents(raw []byte) []byte {
	var message map[string]json.RawMessage
	if json.Unmarshal(raw, &message) != nil || string(message["type"]) != `"message"` {
		return nil
	}
	var blocks []map[string]json.RawMessage
	if json.Unmarshal(message["content"], &blocks) != nil {
		return nil
	}
	var result []byte
	appendEvent := func(event any) {
		data, _ := json.Marshal(event)
		result = append(result, []byte("data: ")...)
		result = append(result, data...)
		result = append(result, '\n', '\n')
	}
	message["content"] = json.RawMessage(`[]`)
	appendEvent(map[string]any{"type": "message_start", "message": message})
	for index, block := range blocks {
		var kind string
		_ = json.Unmarshal(block["type"], &kind)
		var delta map[string]any
		switch kind {
		case "text":
			delta = map[string]any{"type": "text_delta", "text": block["text"]}
		case "thinking":
			delta = map[string]any{"type": "thinking_delta", "thinking": block["thinking"]}
		case "tool_use":
			delta = map[string]any{"type": "input_json_delta", "partial_json": string(block["input"])}
		default:
			return nil // 不将 SDK 无法表示的内容静默转换为空成功响应。
		}
		start := map[string]any{"type": kind}
		if kind == "text" {
			start["text"] = ""
		}
		if kind == "thinking" {
			start["thinking"] = ""
		}
		if kind == "tool_use" {
			start["id"] = block["id"]
			start["name"] = block["name"]
			start["input"] = map[string]any{}
		}
		appendEvent(map[string]any{"type": "content_block_start", "index": index, "content_block": start})
		appendEvent(map[string]any{"type": "content_block_delta", "index": index, "delta": delta})
		appendEvent(map[string]any{"type": "content_block_stop", "index": index})
	}
	appendEvent(map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": message["stop_reason"], "stop_sequence": message["stop_sequence"]}, "usage": message["usage"]})
	appendEvent(map[string]any{"type": "message_stop"})
	return result
}
