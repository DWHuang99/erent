package route

import (
	"errors"
	"reflect"
	"testing"

	"erent/internal/modules/translator"
)

func TestRouteSelection(t *testing.T) {
	registry := NewRegistry()
	cases := []struct {
		model  string
		mode   AuthMode
		format translator.Format
		route  Route
	}{
		{"model-a", AuthModeAPIKey, translator.FormatOpenAI, Route{Endpoint: "https://example.com/v1/chat/completions", UpstreamFormat: translator.FormatOpenAI}},
		{"model-a", AuthModeAPIKey, translator.FormatOpenAIResponse, Route{Endpoint: "https://example.com/v1/responses", UpstreamFormat: translator.FormatOpenAIResponse}},
		{"model-a", AuthModeAccessToken, translator.FormatOpenAI, Route{Endpoint: "https://example.com/codex/responses", UpstreamFormat: translator.FormatCodex, ForceStream: true}},
		{"model-b", AuthModeAPIKey, translator.FormatOpenAI, Route{Endpoint: "https://other.example.com/v1/chat/completions", UpstreamFormat: translator.FormatOpenAI}},
	}
	for _, tc := range cases {
		registry.register(tc.model, tc.mode, tc.format, tc.route)
	}
	for _, tc := range cases {
		got, err := registry.getRoute(tc.model, tc.mode, tc.format)
		if err != nil || !reflect.DeepEqual(got, tc.route) {
			t.Fatalf("getRoute(%q, %q, %q) = %+v, %v; want %+v", tc.model, tc.mode, tc.format, got, err, tc.route)
		}
	}
	updated := Route{Endpoint: "https://updated.example.com/v1/chat/completions", UpstreamFormat: translator.FormatOpenAI}
	registry.register(cases[0].model, cases[0].mode, cases[0].format, updated)
	cases[0].route = updated
	for _, tc := range cases {
		got, err := registry.getRoute(tc.model, tc.mode, tc.format)
		if err != nil || !reflect.DeepEqual(got, tc.route) {
			t.Fatalf("re-register affected another route: got %+v, %v; want %+v", got, err, tc.route)
		}
	}
}

func TestUnknownRouteDoesNotFallBack(t *testing.T) {
	registry := NewRegistry()
	registry.register("model-a", AuthModeAccessToken, translator.FormatOpenAI, Route{Endpoint: "https://example.com"})
	for _, tc := range []struct {
		model  string
		mode   AuthMode
		format translator.Format
	}{
		{"missing", AuthModeAccessToken, translator.FormatOpenAI},
		{"model-a", AuthModeAPIKey, translator.FormatOpenAI},
		{"model-a", AuthModeAccessToken, translator.FormatClaude},
	} {
		got, err := registry.getRoute(tc.model, tc.mode, tc.format)
		if !errors.Is(err, errors.ErrUnsupported) || !reflect.DeepEqual(got, Route{}) {
			t.Fatalf("expected unsupported route for %+v; got %+v, %v", tc, got, err)
		}
	}
}

func TestCodexRoutesCoverClientFormats(t *testing.T) {
	for _, model := range []string{"gpt-6-astra", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.3-codex-spark"} {
		for _, format := range []translator.Format{translator.FormatClaude, translator.FormatOpenAIResponse, translator.FormatOpenAI} {
			got, err := GetRoute(model, AuthModeAccessToken, format)
			if err != nil || got.Endpoint != "https://chatgpt.com/backend-api/codex/responses" || !got.ForceStream {
				t.Fatalf("missing Codex route for %q, %q: %+v, %v", model, format, got, err)
			}
		}
	}
}

func TestRouteHeadersAreIsolated(t *testing.T) {
	registry := NewRegistry()
	headers := map[string]string{"X-Version": "v1"}
	registry.register("headers-test", AuthModeAccessToken, translator.FormatOpenAI, Route{
		AuthHeader: "Authorization", AuthPrefix: "Bearer ", Headers: headers,
	})
	headers["X-Version"] = "changed-after-register"
	first, err := registry.getRoute("headers-test", AuthModeAccessToken, translator.FormatOpenAI)
	if err != nil || first.Headers["X-Version"] != "v1" || first.AuthHeader != "Authorization" || first.AuthPrefix != "Bearer " {
		t.Fatalf("registered headers changed: %+v, %v", first, err)
	}
	first.Headers["X-Version"] = "changed-by-request"
	second, err := registry.getRoute("headers-test", AuthModeAccessToken, translator.FormatOpenAI)
	if err != nil || second.Headers["X-Version"] != "v1" {
		t.Fatalf("request modified shared route headers: %+v, %v", second, err)
	}
}
