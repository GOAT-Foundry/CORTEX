package llmhub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Autumn-27/norma/llm"

	"github.com/GOAT-Foundry/CORTEX/internal/store"
)

const (
	// CopilotClientID is the standard OAuth App Client ID for GitHub Copilot Device Flow.
	CopilotClientID = "Iv1.b507a08c87ecfe98"

	copilotEditorVersion       = "vscode/1.99.0"
	copilotEditorPluginVersion = "copilot-chat/0.26.2"
	copilotUserAgent           = "GitHubCopilotChat/0.26.2"
	copilotIntegrationID       = "vscode-chat"
	copilotAPIVersion          = "2025-04-01"
	copilotSettingKey          = "copilot_oauth_token"
)

// Preset describes a built-in LLM provider template.
type Preset struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Format          string   `json:"format"`
	BaseURL         string   `json:"base_url"`
	DefaultModel    string   `json:"default_model"`
	AuthType        string   `json:"auth_type"` // "copilot_device_flow", "api_key", "none"
	Endpoints       []string `json:"endpoints"`
	InputCostPer1M  float64  `json:"input_cost_per_1m"`
	OutputCostPer1M float64  `json:"output_cost_per_1m"`
	Description     string   `json:"description"`
}

// CopilotDeviceCode holds the initial response of the GitHub OAuth Device Flow.
type CopilotDeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// CopilotPollResult holds one poll check result for the GitHub OAuth Device Flow.
type CopilotPollResult struct {
	Status      string `json:"status"` // "pending" | "slow_down" | "authorized" | "error"
	AccessToken string `json:"access_token,omitempty"`
	Interval    int    `json:"interval,omitempty"`
	Error       string `json:"error,omitempty"`
}

// CopilotModel describes a model available via GitHub Copilot.
type CopilotModel struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Vendor      string   `json:"vendor"`
	WireFormats []string `json:"wire_formats"`
}

// CopilotStatus reports the current GitHub Copilot authentication state.
type CopilotStatus struct {
	Connected        bool   `json:"connected"`
	HasOAuthToken    bool   `json:"has_oauth_token"`
	SessionValid     bool   `json:"session_valid"`
	SessionExpiresAt int64  `json:"session_expires_at,omitempty"`
	APIEndpoint      string `json:"api_endpoint"`
}

type copilotSession struct {
	Token       string
	ExpiresAt   int64
	APIEndpoint string
}

// Hub manages LLM provider configurations, GitHub Copilot OAuth & token exchange,
// automatic failover, and token/cost telemetry.
type Hub struct {
	store      *store.Store
	httpClient *http.Client

	DeviceCodeURL  string
	AccessTokenURL string
	CopilotTokenURL string

	mu           sync.Mutex
	sessionCache map[string]copilotSession
}

// New constructs a Hub and seeds default provider configurations if the database is empty.
func New(st *store.Store) *Hub {
	h := &Hub{
		store:           st,
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		DeviceCodeURL:   "https://github.com/login/device/code",
		AccessTokenURL:  "https://github.com/login/oauth/access_token",
		CopilotTokenURL: "https://api.github.com/copilot_internal/v2/token",
		sessionCache:    map[string]copilotSession{},
	}
	if st != nil {
		_ = h.seedDefaults(context.Background())
	}
	return h
}

// DefaultPresets returns the built-in LLM provider presets.
func DefaultPresets() []Preset {
	return []Preset{
		{
			ID:              "github-copilot",
			Name:            "GitHub Copilot (Native)",
			Format:          string(llm.FormatOpenAI),
			BaseURL:         "https://api.githubcopilot.com",
			DefaultModel:    "gpt-4o",
			AuthType:        "copilot_device_flow",
			Endpoints:       []string{"/chat/completions", "/responses", "/v1/messages", "/models"},
			InputCostPer1M:  0.0,
			OutputCostPer1M: 0.0,
			Description:     "Native GitHub Copilot integration via OAuth Device Flow & automatic session token exchange.",
		},
		{
			ID:              "openai",
			Name:            "OpenAI",
			Format:          string(llm.FormatOpenAI),
			BaseURL:         "https://api.openai.com/v1",
			DefaultModel:    "gpt-4o",
			AuthType:        "api_key",
			Endpoints:       []string{"/chat/completions", "/responses"},
			InputCostPer1M:  2.50,
			OutputCostPer1M: 10.00,
			Description:     "OpenAI API (Chat Completions & Responses wire formats).",
		},
		{
			ID:              "anthropic",
			Name:            "Anthropic Claude",
			Format:          string(llm.FormatAnthropic),
			BaseURL:         "https://api.anthropic.com",
			DefaultModel:    "claude-3-7-sonnet-latest",
			AuthType:        "api_key",
			Endpoints:       []string{"/v1/messages"},
			InputCostPer1M:  3.00,
			OutputCostPer1M: 15.00,
			Description:     "Anthropic Messages API with extended thinking support.",
		},
		{
			ID:              "gemini",
			Name:            "Google Gemini (OpenAI Compatible)",
			Format:          string(llm.FormatOpenAI),
			BaseURL:         "https://generativelanguage.googleapis.com/v1beta/openai",
			DefaultModel:    "gemini-2.5-pro",
			AuthType:        "api_key",
			Endpoints:       []string{"/chat/completions"},
			InputCostPer1M:  1.25,
			OutputCostPer1M: 10.00,
			Description:     "Google Gemini API via OpenAI compatibility layer.",
		},
		{
			ID:              "openrouter",
			Name:            "OpenRouter",
			Format:          string(llm.FormatOpenAI),
			BaseURL:         "https://openrouter.ai/api/v1",
			DefaultModel:    "openai/gpt-4o",
			AuthType:        "api_key",
			Endpoints:       []string{"/chat/completions"},
			InputCostPer1M:  2.50,
			OutputCostPer1M: 10.00,
			Description:     "Unified multi-model router for hundreds of foundation models.",
		},
		{
			ID:              "deepseek",
			Name:            "DeepSeek",
			Format:          string(llm.FormatOpenAI),
			BaseURL:         "https://api.deepseek.com/v1",
			DefaultModel:    "deepseek-chat",
			AuthType:        "api_key",
			Endpoints:       []string{"/chat/completions"},
			InputCostPer1M:  0.27,
			OutputCostPer1M: 1.10,
			Description:     "DeepSeek V3 / R1 API endpoint.",
		},
		{
			ID:              "azure-openai",
			Name:            "Azure OpenAI",
			Format:          string(llm.FormatOpenAI),
			BaseURL:         "https://your-resource.openai.azure.com/openai/v1",
			DefaultModel:    "gpt-4o",
			AuthType:        "api_key",
			Endpoints:       []string{"/chat/completions", "/responses"},
			InputCostPer1M:  2.50,
			OutputCostPer1M: 10.00,
			Description:     "Azure OpenAI Service endpoint.",
		},
		{
			ID:              "ollama",
			Name:            "Ollama / Local OpenAI-Compatible",
			Format:          string(llm.FormatOpenAI),
			BaseURL:         "http://localhost:11434/v1",
			DefaultModel:    "llama3.3",
			AuthType:        "none",
			Endpoints:       []string{"/chat/completions"},
			InputCostPer1M:  0.0,
			OutputCostPer1M: 0.0,
			Description:     "Local LLM execution via Ollama, vLLM, or LM Studio.",
		},
	}
}

func (h *Hub) seedDefaults(ctx context.Context) error {
	existing, err := h.store.ListProviderConfigs(ctx)
	if err != nil || len(existing) > 0 {
		return err
	}
	envProvider := strings.ToLower(strings.TrimSpace(os.Getenv("CORTEX_PROVIDER")))
	envModel := strings.TrimSpace(os.Getenv("CORTEX_MODEL"))

	presets := DefaultPresets()
	for i, p := range presets {
		apiKey := ""
		switch p.ID {
		case "openai":
			apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		case "anthropic":
			apiKey = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
		case "gemini":
			apiKey = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
		case "github-copilot":
			apiKey = strings.TrimSpace(os.Getenv("GITHUB_COPILOT_TOKEN"))
		}
		priority := i + 2
		defModel := p.DefaultModel
		enabled := i < 3 || apiKey != ""
		if envProvider != "" && (p.ID == envProvider || (envProvider == "openai-responses" && p.ID == "openai")) {
			priority = 1
			enabled = true
			if envProvider == "openai-responses" {
				p.Format = string(llm.FormatOpenAIResponses)
			}
			if envModel != "" {
				defModel = envModel
			}
		}
		_, _ = h.store.SaveProviderConfig(ctx, store.ProviderConfig{
			ID:              p.ID,
			Name:            p.Name,
			Format:          p.Format,
			BaseURL:         p.BaseURL,
			APIKey:          apiKey,
			DefaultModel:    defModel,
			AuthType:        p.AuthType,
			Enabled:         enabled,
			Priority:        priority,
			InputCostPer1M:  p.InputCostPer1M,
			OutputCostPer1M: p.OutputCostPer1M,
		}, false)
	}
	return nil
}

// ListProviders returns all configured providers with API keys masked for safe UI display.
func (h *Hub) ListProviders(ctx context.Context) ([]store.ProviderConfig, error) {
	if h.store == nil {
		return []store.ProviderConfig{}, nil
	}
	list, err := h.store.ListProviderConfigs(ctx)
	if err != nil {
		return nil, err
	}
	copilotTok := h.getCopilotOAuthToken(ctx)
	for i := range list {
		if list[i].ID == "github-copilot" && copilotTok != "" {
			list[i].HasAPIKey = true
		}
		list[i].APIKey = maskSecret(list[i].APIKey)
	}
	return list, nil
}

// SaveProvider updates or creates a provider configuration and returns the masked view.
func (h *Hub) SaveProvider(ctx context.Context, in store.ProviderConfig, clearKey bool) (store.ProviderConfig, error) {
	if h.store == nil {
		return store.ProviderConfig{}, errors.New("llmhub: store not initialized")
	}
	in.Format = normalizeWireFormat(in.Format)
	in.BaseURL = NormalizeBaseURL(in.BaseURL)
	if in.ID == "github-copilot" || in.AuthType == "copilot_device_flow" {
		if in.BaseURL == "" || !isTrustedCopilotHost(in.BaseURL) {
			in.BaseURL = "https://api.githubcopilot.com"
		}
	}
	if strings.Contains(in.APIKey, "••") || strings.Contains(in.APIKey, "***") {
		in.APIKey = ""
	}
	saved, err := h.store.SaveProviderConfig(ctx, in, clearKey)
	if err != nil {
		return store.ProviderConfig{}, err
	}
	if saved.ID == "github-copilot" && in.APIKey != "" {
		_ = h.store.SetSetting(ctx, copilotSettingKey, in.APIKey)
	}
	saved.APIKey = maskSecret(saved.APIKey)
	return saved, nil
}

// ResolveCopilotFormat selects the wire format required by a GitHub Copilot model
// when targeting a Copilot endpoint:
//   - gpt-5*, gpt-6*, o1*, o3*, o4*, grok-* -> FormatOpenAIResponses (/responses)
//   - claude-* -> FormatAnthropic (/v1/messages) when requested, otherwise FormatOpenAI (/chat/completions)
//   - gemini-*, gpt-4* -> FormatOpenAI (/chat/completions)
func ResolveCopilotFormat(requestedFormat string, model string) llm.Format {
	reqFmt := llm.Format(normalizeWireFormat(requestedFormat))
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "gpt-5") ||
		strings.HasPrefix(m, "gpt-6") ||
		strings.HasPrefix(m, "grok-") ||
		strings.HasPrefix(m, "o1") ||
		strings.HasPrefix(m, "o3") ||
		strings.HasPrefix(m, "o4"):
		return llm.FormatOpenAIResponses
	case strings.HasPrefix(m, "claude-"):
		if reqFmt == llm.FormatAnthropic {
			return llm.FormatAnthropic
		}
		if reqFmt == llm.FormatOpenAIResponses {
			return llm.FormatOpenAI
		}
		return reqFmt
	case strings.HasPrefix(m, "gemini-") || strings.HasPrefix(m, "gpt-4"):
		if reqFmt == llm.FormatOpenAIResponses {
			return llm.FormatOpenAIResponses
		}
		return llm.FormatOpenAI
	default:
		return reqFmt
	}
}

// NormalizeBaseURL strips trailing slashes and endpoint suffixes (/chat/completions,
// /responses, /v1/messages) so Norma's adapters can append their canonical path.
func NormalizeBaseURL(raw string) string {
	u := strings.TrimRight(strings.TrimSpace(raw), "/")
	for _, suffix := range []string{"/chat/completions", "/responses", "/v1/messages"} {
		if strings.HasSuffix(u, suffix) {
			u = strings.TrimRight(strings.TrimSuffix(u, suffix), "/")
			break
		}
	}
	return u
}

func normalizeWireFormat(f string) string {
	switch strings.ToLower(strings.TrimSpace(f)) {
	case "anthropic", "/v1/messages", "messages":
		return string(llm.FormatAnthropic)
	case "openai-responses", "responses", "/responses":
		return string(llm.FormatOpenAIResponses)
	default:
		return string(llm.FormatOpenAI)
	}
}

func maskSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "••••••••"
	}
	return s[:4] + "••••" + s[len(s)-4:]
}

func (h *Hub) getCopilotOAuthToken(ctx context.Context) string {
	if h.store != nil {
		if tok, err := h.store.GetSetting(ctx, copilotSettingKey); err == nil && strings.TrimSpace(tok) != "" {
			return strings.TrimSpace(tok)
		}
		if cfg, err := h.store.GetProviderConfig(ctx, "github-copilot"); err == nil && strings.TrimSpace(cfg.APIKey) != "" {
			return strings.TrimSpace(cfg.APIKey)
		}
	}
	return strings.TrimSpace(os.Getenv("GITHUB_COPILOT_TOKEN"))
}

// GetCopilotStatus returns the current GitHub Copilot connection and session token state.
func (h *Hub) GetCopilotStatus(ctx context.Context) CopilotStatus {
	oauth := h.getCopilotOAuthToken(ctx)
	status := CopilotStatus{
		Connected:     oauth != "",
		HasOAuthToken: oauth != "",
		APIEndpoint:   "https://api.githubcopilot.com",
	}
	if oauth == "" {
		return status
	}
	h.mu.Lock()
	sess, ok := h.sessionCache[oauth]
	h.mu.Unlock()
	if ok && sess.Token != "" && sess.ExpiresAt > time.Now().Unix() {
		status.SessionValid = true
		status.SessionExpiresAt = sess.ExpiresAt
		if sess.APIEndpoint != "" {
			status.APIEndpoint = sess.APIEndpoint
		}
	}
	return status
}

// DisconnectCopilot removes the stored GitHub Copilot OAuth token and clears cached session tokens.
func (h *Hub) DisconnectCopilot(ctx context.Context) error {
	h.mu.Lock()
	h.sessionCache = map[string]copilotSession{}
	h.mu.Unlock()
	if h.store == nil {
		return nil
	}
	_ = h.store.DeleteSetting(ctx, copilotSettingKey)
	if cfg, err := h.store.GetProviderConfig(ctx, "github-copilot"); err == nil {
		cfg.APIKey = ""
		_, _ = h.store.SaveProviderConfig(ctx, cfg, true)
	}
	return nil
}

// StartCopilotDeviceFlow initiates the GitHub OAuth 2.0 Device Flow.
func (h *Hub) StartCopilotDeviceFlow(ctx context.Context) (*CopilotDeviceCode, error) {
	payload, _ := json.Marshal(map[string]string{
		"client_id": CopilotClientID,
		"scope":     "read:user",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.DeviceCodeURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", copilotUserAgent)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub device flow HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out CopilotDeviceCode
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.DeviceCode == "" || out.UserCode == "" {
		return nil, fmt.Errorf("invalid device code response: %s", strings.TrimSpace(string(body)))
	}
	if out.Interval <= 0 {
		out.Interval = 5
	}
	return &out, nil
}

// PollCopilotDeviceFlow polls GitHub OAuth once and, if authorized, persists the OAuth token
// and exchanges it for an initial Copilot session token.
func (h *Hub) PollCopilotDeviceFlow(ctx context.Context, deviceCode string) (*CopilotPollResult, error) {
	payload, _ := json.Marshal(map[string]string{
		"client_id":   CopilotClientID,
		"device_code": strings.TrimSpace(deviceCode),
		"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.AccessTokenURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", copilotUserAgent)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var raw struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Interval         int    `json:"interval"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if raw.AccessToken != "" {
		if h.store != nil {
			_ = h.store.SetSetting(ctx, copilotSettingKey, raw.AccessToken)
			if cfg, err := h.store.GetProviderConfig(ctx, "github-copilot"); err == nil {
				cfg.APIKey = raw.AccessToken
				cfg.Enabled = true
				_, _ = h.store.SaveProviderConfig(ctx, cfg, false)
			}
		}
		_, _ = h.getOrExchangeCopilotToken(ctx, raw.AccessToken)
		return &CopilotPollResult{
			Status:      "authorized",
			AccessToken: maskSecret(raw.AccessToken),
		}, nil
	}
	switch raw.Error {
	case "authorization_pending":
		return &CopilotPollResult{Status: "pending"}, nil
	case "slow_down":
		return &CopilotPollResult{Status: "slow_down", Interval: raw.Interval}, nil
	default:
		msg := raw.ErrorDescription
		if msg == "" {
			msg = raw.Error
		}
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return &CopilotPollResult{Status: "error", Error: msg}, nil
	}
}

func (h *Hub) getOrExchangeCopilotToken(ctx context.Context, oauthToken string) (copilotSession, error) {
	oauthToken = strings.TrimSpace(oauthToken)
	if oauthToken == "" {
		return copilotSession{}, errors.New("copilot: missing OAuth token")
	}
	if strings.Contains(oauthToken, "tid=") {
		return copilotSession{
			Token:       oauthToken,
			ExpiresAt:   time.Now().Add(25 * time.Minute).Unix(),
			APIEndpoint: "https://api.githubcopilot.com",
		}, nil
	}

	now := time.Now().Unix()
	h.mu.Lock()
	if cached, ok := h.sessionCache[oauthToken]; ok && cached.Token != "" && cached.ExpiresAt-60 > now {
		h.mu.Unlock()
		return cached, nil
	}
	h.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.CopilotTokenURL, nil)
	if err != nil {
		return copilotSession{}, err
	}
	req.Header.Set("Authorization", "token "+oauthToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Editor-Version", copilotEditorVersion)
	req.Header.Set("Editor-Plugin-Version", copilotEditorPluginVersion)
	req.Header.Set("User-Agent", copilotUserAgent)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return copilotSession{}, fmt.Errorf("copilot token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return copilotSession{}, fmt.Errorf("copilot token exchange HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
		Endpoints struct {
			API string `json:"api"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return copilotSession{}, fmt.Errorf("copilot token exchange parse error: %w", err)
	}
	if strings.TrimSpace(parsed.Token) == "" {
		return copilotSession{}, errors.New("copilot token exchange returned empty token")
	}
	if parsed.ExpiresAt <= 0 {
		parsed.ExpiresAt = time.Now().Add(25 * time.Minute).Unix()
	}
	ep := strings.TrimRight(strings.TrimSpace(parsed.Endpoints.API), "/")
	if ep == "" {
		ep = "https://api.githubcopilot.com"
	}
	sess := copilotSession{
		Token:       parsed.Token,
		ExpiresAt:   parsed.ExpiresAt,
		APIEndpoint: ep,
	}

	h.mu.Lock()
	h.sessionCache[oauthToken] = sess
	h.mu.Unlock()
	return sess, nil
}

// ListCopilotModels fetches available models from GitHub Copilot's /models endpoint
// when authenticated, or returns the default curated Copilot model catalog.
func (h *Hub) ListCopilotModels(ctx context.Context) ([]CopilotModel, string, error) {
	fallback := defaultCopilotModels()
	oauth := h.getCopilotOAuthToken(ctx)
	if oauth == "" {
		return fallback, "catalog", nil
	}
	sess, err := h.getOrExchangeCopilotToken(ctx, oauth)
	if err != nil {
		return fallback, "catalog_fallback", err
	}
	modelsURL := strings.TrimRight(sess.APIEndpoint, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return fallback, "catalog_fallback", err
	}
	req.Header.Set("Authorization", "Bearer "+sess.Token)
	req.Header.Set("Accept", "application/json")
	applyCopilotHeaders(req.Header)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fallback, "catalog_fallback", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != http.StatusOK {
		return fallback, "catalog_fallback", fmt.Errorf("copilot /models HTTP %d", resp.StatusCode)
	}

	var parsed struct {
		Data []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Vendor string `json:"vendor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Data) == 0 {
		return fallback, "catalog", nil
	}
	out := make([]CopilotModel, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if m.ID == "" {
			continue
		}
		name := m.Name
		if name == "" {
			name = m.ID
		}
		vendor := m.Vendor
		if vendor == "" {
			vendor = inferVendor(m.ID)
		}
		out = append(out, CopilotModel{
			ID:          m.ID,
			Name:        name,
			Vendor:      vendor,
			WireFormats: supportedWireFormatsForModel(m.ID),
		})
	}
	if len(out) == 0 {
		return fallback, "catalog", nil
	}
	return out, "live", nil
}

func defaultCopilotModels() []CopilotModel {
	return []CopilotModel{
		{ID: "gpt-4o", Name: "GPT-4o", Vendor: "OpenAI", WireFormats: []string{"/chat/completions", "/responses"}},
		{ID: "gpt-4.1", Name: "GPT-4.1", Vendor: "OpenAI", WireFormats: []string{"/chat/completions", "/responses"}},
		{ID: "o3-mini", Name: "o3-mini", Vendor: "OpenAI", WireFormats: []string{"/responses"}},
		{ID: "claude-3.7-sonnet", Name: "Claude 3.7 Sonnet", Vendor: "Anthropic", WireFormats: []string{"/chat/completions", "/v1/messages"}},
		{ID: "claude-sonnet-4", Name: "Claude Sonnet 4", Vendor: "Anthropic", WireFormats: []string{"/chat/completions", "/v1/messages"}},
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Vendor: "Google", WireFormats: []string{"/chat/completions"}},
	}
}

func inferVendor(modelID string) string {
	m := strings.ToLower(modelID)
	switch {
	case strings.HasPrefix(m, "claude"):
		return "Anthropic"
	case strings.HasPrefix(m, "gemini"):
		return "Google"
	default:
		return "OpenAI"
	}
}

func supportedWireFormatsForModel(modelID string) []string {
	m := strings.ToLower(modelID)
	switch {
	case strings.HasPrefix(m, "gpt-5") || strings.HasPrefix(m, "gpt-6") || strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4"):
		return []string{"/responses"}
	case strings.HasPrefix(m, "claude"):
		return []string{"/chat/completions", "/v1/messages"}
	default:
		return []string{"/chat/completions"}
	}
}

func applyCopilotHeaders(h http.Header) {
	h.Set("Editor-Version", copilotEditorVersion)
	h.Set("Editor-Plugin-Version", copilotEditorPluginVersion)
	h.Set("Copilot-Integration-Id", copilotIntegrationID)
	h.Set("User-Agent", copilotUserAgent)
	h.Set("Openai-Intent", "conversation-panel")
	h.Set("X-GitHub-Api-Version", copilotAPIVersion)
}

func isTrustedCopilotHost(rawURLOrHost string) bool {
	h := strings.ToLower(strings.TrimSpace(rawURLOrHost))
	if strings.Contains(h, "://") {
		if u, err := url.Parse(h); err == nil && u.Hostname() != "" {
			h = strings.ToLower(u.Hostname())
		}
	}
	if i := strings.IndexByte(h, ':'); i >= 0 {
		h = h[:i]
	}
	return h == "api.githubcopilot.com" ||
		strings.HasSuffix(h, ".githubcopilot.com") ||
		h == "127.0.0.1" ||
		h == "localhost"
}

type copilotRoundTripper struct {
	hub        *Hub
	oauthToken string
	base       http.RoundTripper
}

func (rt *copilotRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	if req == nil || req.URL == nil || !isTrustedCopilotHost(req.URL.Host) {
		return nil, errors.New("copilot: refusing to send credentials to untrusted host")
	}
	sess, err := rt.hub.getOrExchangeCopilotToken(req.Context(), rt.oauthToken)
	if err != nil {
		return nil, err
	}
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", "Bearer "+sess.Token)
	if cloned.Header.Get("x-api-key") != "" {
		cloned.Header.Set("x-api-key", sess.Token)
	}
	if sess.APIEndpoint != "" && strings.EqualFold(cloned.URL.Hostname(), "api.githubcopilot.com") {
		if epURL, err := url.Parse(sess.APIEndpoint); err == nil && epURL.Host != "" && isTrustedCopilotHost(epURL.Host) {
			cloned.URL.Scheme = epURL.Scheme
			cloned.URL.Host = epURL.Host
			cloned.Host = epURL.Host
		}
	}
	applyCopilotHeaders(cloned.Header)
	return base.RoundTrip(cloned)
}

// TestProvider verifies connectivity and authentication for a provider configuration.
func (h *Hub) TestProvider(ctx context.Context, id string) (map[string]any, error) {
	if h.store == nil {
		return nil, errors.New("store unavailable")
	}
	cfg, err := h.store.GetProviderConfig(ctx, id)
	if err != nil {
		return nil, err
	}
	if cfg.ID == "github-copilot" || cfg.AuthType == "copilot_device_flow" {
		oauth := h.getCopilotOAuthToken(ctx)
		if oauth == "" {
			return map[string]any{
				"ok":          false,
				"provider_id": cfg.ID,
				"message":     "GitHub Copilot OAuth token is not configured. Connect via Device Flow first.",
			}, nil
		}
		sess, err := h.getOrExchangeCopilotToken(ctx, oauth)
		if err != nil {
			return map[string]any{
				"ok":          false,
				"provider_id": cfg.ID,
				"message":     err.Error(),
			}, nil
		}
		return map[string]any{
			"ok":           true,
			"provider_id":  cfg.ID,
			"api_endpoint": sess.APIEndpoint,
			"expires_at":   sess.ExpiresAt,
			"message":      "GitHub Copilot session token exchanged successfully.",
		}, nil
	}

	if cfg.AuthType == "api_key" && strings.TrimSpace(cfg.APIKey) == "" {
		return map[string]any{
			"ok":           false,
			"config_valid": false,
			"provider_id":  cfg.ID,
			"message":      "API key is not configured for this provider.",
		}, nil
	}

	probeURL := strings.TrimRight(cfg.BaseURL, "/") + "/models"
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, probeURL, nil)
	probeReachable := false
	probeStatus := 0
	if err == nil {
		if cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
			req.Header.Set("x-api-key", cfg.APIKey)
		}
		if resp, err := h.httpClient.Do(req); err == nil {
			probeStatus = resp.StatusCode
			probeReachable = resp.StatusCode >= 200 && resp.StatusCode < 500
			_ = resp.Body.Close()
		}
	}

	return map[string]any{
		"ok":              true,
		"config_valid":    true,
		"probe_reachable": probeReachable,
		"probe_status":    probeStatus,
		"provider_id":     cfg.ID,
		"format":          cfg.Format,
		"base_url":        cfg.BaseURL,
		"model":           cfg.DefaultModel,
		"message":         "Provider configuration is valid and ready.",
	}, nil
}

type candidateProvider struct {
	cfg      store.ProviderConfig
	model    string
	format   llm.Format
	provider llm.Provider
}

// BuildProvider constructs a Failover & Usage-Tracking llm.Provider from the
// configured providers in SQLite (with preferredID placed first when specified).
func (h *Hub) BuildProvider(ctx context.Context, preferredID, modelOverride string) (llm.Provider, error) {
	var configs []store.ProviderConfig
	if h.store != nil {
		configs, _ = h.store.ListProviderConfigs(ctx)
	}
	if len(configs) == 0 {
		for i, p := range DefaultPresets() {
			configs = append(configs, store.ProviderConfig{
				ID:              p.ID,
				Name:            p.Name,
				Format:          p.Format,
				BaseURL:         p.BaseURL,
				DefaultModel:    p.DefaultModel,
				AuthType:        p.AuthType,
				Enabled:         i == 0,
				Priority:        i + 1,
				InputCostPer1M:  p.InputCostPer1M,
				OutputCostPer1M: p.OutputCostPer1M,
			})
		}
	}

	ordered := make([]store.ProviderConfig, 0, len(configs))
	if preferredID != "" {
		for _, c := range configs {
			if c.ID == preferredID {
				ordered = append(ordered, c)
				break
			}
		}
	}
	for _, c := range configs {
		if c.ID == preferredID {
			continue
		}
		if c.Enabled {
			ordered = append(ordered, c)
		}
	}
	if len(ordered) == 0 && len(configs) > 0 {
		ordered = append(ordered, configs[0])
	}

	candidates := make([]candidateProvider, 0, len(ordered))
	for idx, c := range ordered {
		override := ""
		// Apply explicit modelOverride only to the primary/preferred candidate
		// so fallback providers use their own native DefaultModel.
		if idx == 0 {
			override = modelOverride
		}
		cand, err := h.buildSingleCandidate(ctx, c, override)
		if err == nil {
			candidates = append(candidates, cand)
		}
	}
	if len(candidates) == 0 {
		return nil, errors.New("llmhub: no valid provider candidates available")
	}
	return &failoverProvider{
		hub:        h,
		candidates: candidates,
	}, nil
}

func (h *Hub) buildSingleCandidate(ctx context.Context, c store.ProviderConfig, modelOverride string) (candidateProvider, error) {
	model := strings.TrimSpace(modelOverride)
	if model == "" {
		model = strings.TrimSpace(c.DefaultModel)
	}
	if model == "" {
		model = "gpt-4o"
	}

	format := llm.Format(normalizeWireFormat(c.Format))
	baseURL := NormalizeBaseURL(c.BaseURL)
	apiKey := strings.TrimSpace(c.APIKey)
	var customClient *http.Client

	if c.ID == "github-copilot" || c.AuthType == "copilot_device_flow" {
		oauth := h.getCopilotOAuthToken(ctx)
		if oauth != "" {
			apiKey = oauth
		}
		format = ResolveCopilotFormat(c.Format, model)
		if baseURL == "" {
			baseURL = "https://api.githubcopilot.com"
		}
		customClient = &http.Client{
			Timeout: 120 * time.Second,
			Transport: &copilotRoundTripper{
				hub:        h,
				oauthToken: apiKey,
				base:       h.httpClient.Transport,
			},
		}
	}

	prov, err := llm.NewProvider(llm.Config{
		Format:     format,
		BaseURL:    baseURL,
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: customClient,
		MaxRetries: 1,
	})
	if err != nil {
		return candidateProvider{}, err
	}
	return candidateProvider{
		cfg:      c,
		model:    model,
		format:   format,
		provider: prov,
	}, nil
}

type failoverProvider struct {
	hub        *Hub
	candidates []candidateProvider
}

func (fp *failoverProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		var lastErr error
		for idx, cand := range fp.candidates {
			var usage llm.Usage
			var bufferedStarts []llm.StreamEvent
			committed := false
			var streamErr error

			for ev, err := range cand.provider.Stream(ctx, req) {
				if err != nil {
					streamErr = err
					break
				}
				if ev.Type == llm.SEMessageStart || ev.Type == llm.SEMessageDelta {
					usage.MergeEcho(ev.Usage)
				}
				if !committed && ev.Type == llm.SEMessageStart {
					bufferedStarts = append(bufferedStarts, ev)
					continue
				}
				if !committed {
					committed = true
					for _, st := range bufferedStarts {
						if !yield(st, nil) {
							fp.recordUsage(cand, usage)
							return
						}
					}
					bufferedStarts = nil
				}
				if !yield(ev, nil) {
					fp.recordUsage(cand, usage)
					return
				}
			}

			if streamErr == nil {
				for _, st := range bufferedStarts {
					if !yield(st, nil) {
						break
					}
				}
				fp.recordUsage(cand, usage)
				return
			}
			lastErr = streamErr
			// If events were already yielded to the caller, or this is the last candidate, surface the error.
			if committed || idx == len(fp.candidates)-1 {
				yield(llm.StreamEvent{}, lastErr)
				return
			}
		}
		if lastErr != nil {
			yield(llm.StreamEvent{}, lastErr)
		}
	}
}

func (fp *failoverProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	var lastErr error
	for _, cand := range fp.candidates {
		msg, stop, usage, err := cand.provider.Complete(ctx, req)
		if err == nil {
			fp.recordUsage(cand, usage)
			return msg, stop, usage, nil
		}
		lastErr = err
	}
	return llm.Message{}, "", llm.Usage{}, lastErr
}

func (fp *failoverProvider) recordUsage(cand candidateProvider, u llm.Usage) {
	if fp.hub == nil || fp.hub.store == nil {
		return
	}
	cost := (float64(u.InputTokens)*cand.cfg.InputCostPer1M + float64(u.OutputTokens)*cand.cfg.OutputCostPer1M) / 1_000_000.0
	_, _ = fp.hub.store.RecordUsage(context.Background(), store.UsageLogEntry{
		ProviderID:       cand.cfg.ID,
		Model:            cand.model,
		WireFormat:       string(cand.format),
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens,
		CostUSD:          cost,
	})
}
