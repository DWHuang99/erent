package translator

import (
	"bytes"
	"encoding/json"
	"github.com/tidwall/gjson"
	"testing"
)

func TestNonStreamClaudeJSONPreservesTextToolsAndUsage(t *testing.T) {
	raw := []byte(`{"id":"msg_test","type":"message","model":"test","content":[{"type":"text","text":"hello"},{"type":"tool_use","id":"call_test","name":"lookup","input":{"city":"Shanghai"}}],"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":4}}`)
	for _, to := range []Format{FormatOpenAI, FormatOpenAIResponse} {
		got := TranslateNonStream(t.Context(), FormatClaude, to, "test", []byte(`{}`), []byte(`{}`), raw)
		if !json.Valid(got) || !bytes.Contains(got, []byte("Shanghai")) || !bytes.Contains(got, []byte("hello")) || !bytes.Contains(got, []byte("lookup")) {
			t.Fatalf("content lost for %s: %s", to, got)
		}
		if gjson.GetBytes(got, "usage.total_tokens").Int() != 7 {
			t.Fatalf("usage lost for %s: %s", to, got)
		}
	}
}

func TestTranslateRequestToCodex(t *testing.T) {
	for _, from := range []Format{FormatClaude, FormatOpenAI} {
		t.Run(from.String(), func(t *testing.T) {
			raw := []byte(`{"model":"client-model","messages":[{"role":"user","content":"hello"}],"max_tokens":100}`)
			got := TranslateRequest(from, FormatCodex, "gpt-5.5", raw, true)
			var request struct {
				Model  string `json:"model"`
				Stream bool   `json:"stream"`
				Input  []struct {
					Role    string `json:"role"`
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"input"`
			}
			if err := json.Unmarshal(got, &request); err != nil {
				t.Fatal(err)
			}
			if request.Model != "gpt-5.5" || !request.Stream || len(request.Input) != 1 || request.Input[0].Role != "user" || len(request.Input[0].Content) != 1 || request.Input[0].Content[0].Text != "hello" {
				t.Fatalf("unexpected Codex request: %s", got)
			}
		})
	}
}

func TestClaudeMessageListSystemInstructionsPreservedAsCodexReminder(t *testing.T) {
	raw := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"},{"role":"system","content":[{"type":"text","text":"Follow project instructions."}]},{"role":"assistant","content":"OK"}]}`)
	for _, stream := range []bool{true, false} {
		got := TranslateRequest(FormatClaude, FormatCodex, "gpt-5.5", raw, stream)
		input := gjson.GetBytes(got, "input").Array()
		if len(input) != 3 || input[0].Get("role").String() != "user" || input[1].Get("role").String() != "user" || input[2].Get("role").String() != "assistant" {
			t.Fatalf("unexpected message roles or order: %s", got)
		}
		if input[1].Get("content.0.text").String() != "<system-reminder>\nFollow project instructions.\n</system-reminder>" {
			t.Fatalf("system instructions lost: %s", got)
		}
	}
}

func TestTranslateNonStreamFromCodex(t *testing.T) {
	raw := []byte(`{"type":"response.completed","response":{"id":"resp_test","model":"gpt-5.5","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":3,"output_tokens":1}}}`)
	for _, to := range []Format{FormatClaude, FormatOpenAI} {
		t.Run(to.String(), func(t *testing.T) {
			got := TranslateNonStream(t.Context(), FormatCodex, to, "gpt-5.5", []byte(`{}`), []byte(`{}`), raw)
			var response struct {
				Type    string `json:"type"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			}
			if err := json.Unmarshal(got, &response); err != nil {
				t.Fatal(err)
			}
			if to == FormatClaude {
				if response.Type != "message" || len(response.Content) != 1 || response.Content[0].Text != "hello" {
					t.Fatalf("unexpected Claude response: %s", got)
				}
			} else if len(response.Choices) != 1 || response.Choices[0].Message.Content != "hello" {
				t.Fatalf("unexpected OpenAI response: %s", got)
			}
		})
	}
}

func TestTranslateStreamPreservesRequestState(t *testing.T) {
	var state any
	translate := func(raw string, state *any) []byte {
		return bytes.Join(TranslateStream(t.Context(), FormatCodex, FormatClaude, "gpt-5.5", []byte(`{}`), []byte(`{}`), []byte(raw), state), nil)
	}
	start := translate(`data: {"type":"response.content_part.added","part":{"type":"output_text","text":""}}`, &state)
	first := translate(`data: {"type":"response.output_text.delta","delta":"hello"}`, &state)
	second := translate(`data: {"type":"response.output_text.delta","delta":" world"}`, &state)
	if !bytes.Contains(start, []byte("event: content_block_start")) || !bytes.Contains(first, []byte("hello")) || !bytes.Contains(second, []byte(" world")) {
		t.Fatalf("text block state lost: first=%s second=%s", first, second)
	}
	stop := translate(`data: {"type":"response.content_part.done","part":{"type":"output_text","text":"hello world"}}`, &state)
	doneEvent := `data: {"type":"response.output_item.done","item":{"type":"message","content":[{"type":"output_text","text":"hello world"}]}}`
	if done := translate(doneEvent, &state); len(done) != 0 {
		t.Fatalf("completed item duplicated streamed text: %s", done)
	}
	var otherState any
	other := translate(doneEvent, &otherState)
	if !bytes.Contains(other, []byte("event: content_block_start")) {
		t.Fatalf("new request did not start its own block: %s", other)
	}
	last := translate(`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2}}}`, &state)
	if !bytes.Contains(stop, []byte("event: content_block_stop")) || !bytes.Contains(last, []byte("event: message_stop")) {
		t.Fatalf("missing terminal events: %s", last)
	}
}
