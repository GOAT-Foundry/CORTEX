package llmhub_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Autumn-27/norma/llm"

	"github.com/GOAT-Foundry/CORTEX/internal/llmhub"
	"github.com/GOAT-Foundry/CORTEX/internal/store"
)

func TestResolveCopilotFormat(t *testing.T) {
	cases := []struct {
		reqFmt string
		model  string
		want   llm.Format
	}{
		{"openai", "gpt-4o", llm.FormatOpenAI},
		{"openai", "o3-mini", llm.FormatOpenAIResponses},
		{"openai", "gpt-5", llm.FormatOpenAIResponses},
		{"anthropic", "claude-3.7-sonnet", llm.FormatAnthropic},
		{"openai", "claude-3.7-sonnet", llm.FormatOpenAI},
		{"openai", "gemini-2.5-pro", llm.FormatOpenAI},
	}
	for _, tc := range cases {
		got := llmhub.ResolveCopilotFormat(tc.reqFmt, tc.model)
		if got != tc.want {
			t.Fatalf("ResolveCopilotFormat(%q, %q) = %q, want %q", tc.reqFmt, tc.model, got, tc.want)
		}
	}
}

func TestCopilotDeviceFlowAndTokenExchange(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	var mockSrv *httptest.Server
	mockSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login/device/code":
			_, _ = fmt.Fprint(w, `{"device_code":"dev-123","user_code":"ABCD-1234","verification_uri":"https://github.com/login/device","expires_in":900,"interval":5}`)
		case "/login/oauth/access_token":
			_, _ = fmt.Fprint(w, `{"access_token":"ghu_testoauth1234567890"}`)
		case "/copilot_internal/v2/token":
			_, _ = fmt.Fprintf(w, `{"token":"tid=session123","expires_at":4102444800,"endpoints":{"api":%q}}`, mockSrv.URL)
		case "/models":
			_, _ = fmt.Fprint(w, `{"data":[{"id":"gpt-4o","name":"GPT-4o","vendor":"OpenAI"},{"id":"claude-3.7-sonnet","name":"Claude 3.7 Sonnet","vendor":"Anthropic"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockSrv.Close()

	hub := llmhub.New(st)
	hub.DeviceCodeURL = mockSrv.URL + "/login/device/code"
	hub.AccessTokenURL = mockSrv.URL + "/login/oauth/access_token"
	hub.CopilotTokenURL = mockSrv.URL + "/copilot_internal/v2/token"

	ctx := context.Background()

	dc, err := hub.StartCopilotDeviceFlow(ctx)
	if err != nil {
		t.Fatalf("StartCopilotDeviceFlow: %v", err)
	}
	if dc.UserCode != "ABCD-1234" {
		t.Fatalf("unexpected user_code: %s", dc.UserCode)
	}

	poll, err := hub.PollCopilotDeviceFlow(ctx, dc.DeviceCode)
	if err != nil {
		t.Fatalf("PollCopilotDeviceFlow: %v", err)
	}
	if poll.Status != "authorized" {
		t.Fatalf("expected authorized, got %+v", poll)
	}

	status := hub.GetCopilotStatus(ctx)
	if !status.Connected || !status.SessionValid {
		t.Fatalf("expected connected and valid session, got %+v", status)
	}

	models, source, err := hub.ListCopilotModels(ctx)
	if err != nil {
		t.Fatalf("ListCopilotModels: %v", err)
	}
	if source != "live" || len(models) != 2 {
		t.Fatalf("expected 2 live models, got source=%s models=%+v", source, models)
	}
}

func TestFailoverAndCostAccounting(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"rate limit"}}`, http.StatusTooManyRequests)
	}))
	defer primarySrv.Close()

	fallbackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"id": "chatcmpl-1",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "Fallback succeeded!"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 1000, "completion_tokens": 500}
		}`)
	}))
	defer fallbackSrv.Close()

	hub := llmhub.New(st)
	ctx := context.Background()

	// Disable seeded providers first
	existing, _ := st.ListProviderConfigs(ctx)
	for _, c := range existing {
		c.Enabled = false
		_, _ = st.SaveProviderConfig(ctx, c, false)
	}

	// Primary (priority 1) -> fails with 429
	_, _ = hub.SaveProvider(ctx, store.ProviderConfig{
		ID:              "openai",
		Name:            "Primary OpenAI",
		Format:          "openai",
		BaseURL:         primarySrv.URL,
		APIKey:          "sk-primary-test-key",
		DefaultModel:    "gpt-4o",
		AuthType:        "api_key",
		Enabled:         true,
		Priority:        1,
		InputCostPer1M:  2.0,
		OutputCostPer1M: 10.0,
	}, false)

	// Fallback (priority 2) -> succeeds with 1000 input + 500 output tokens
	_, _ = hub.SaveProvider(ctx, store.ProviderConfig{
		ID:              "openrouter",
		Name:            "Fallback OpenRouter",
		Format:          "openai",
		BaseURL:         fallbackSrv.URL,
		APIKey:          "sk-fallback-test-key",
		DefaultModel:    "gpt-4o-mini",
		AuthType:        "api_key",
		Enabled:         true,
		Priority:        2,
		InputCostPer1M:  2.0,
		OutputCostPer1M: 10.0,
	}, false)

	prov, err := hub.BuildProvider(ctx, "", "")
	if err != nil {
		t.Fatalf("BuildProvider: %v", err)
	}

	msg, _, usage, err := prov.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{llm.UserText("ping")},
	})
	if err != nil {
		t.Fatalf("Complete with failover failed: %v", err)
	}
	if msg.Text() != "Fallback succeeded!" {
		t.Fatalf("unexpected response text: %q", msg.Text())
	}
	if usage.InputTokens != 1000 || usage.OutputTokens != 500 {
		t.Fatalf("unexpected usage: %+v", usage)
	}

	summary, err := st.GetUsageSummary(ctx, 10)
	if err != nil {
		t.Fatalf("GetUsageSummary: %v", err)
	}
	if summary.TotalCalls != 1 || summary.InputTokens != 1000 || summary.OutputTokens != 500 {
		t.Fatalf("unexpected summary totals: %+v", summary)
	}
	// Expected cost: (1000 * $2.0 + 500 * $10.0) / 1,000,000 = $0.007
	if summary.TotalCostUSD < 0.0069 || summary.TotalCostUSD > 0.0071 {
		t.Fatalf("unexpected cost_usd: %f (want 0.007)", summary.TotalCostUSD)
	}
}
