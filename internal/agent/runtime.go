package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"time"

	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/harness"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/permission"
	"github.com/Autumn-27/norma/skill"
	"github.com/Autumn-27/norma/tool"

	"github.com/GOAT-Foundry/CORTEX/internal/config"
	"github.com/GOAT-Foundry/CORTEX/internal/i18n"
	"github.com/GOAT-Foundry/CORTEX/internal/store"
)

// ProviderPreset describes a built-in LLM provider template supported by CORTEX.
type ProviderPreset struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Format      string   `json:"format"`
	BaseURL     string   `json:"base_url"`
	AuthType    string   `json:"auth_type"` // "api_key", "copilot_device_flow", "none"
	Endpoints   []string `json:"endpoints"`
	Description string   `json:"description"`
}

// DefaultPresets returns the built-in LLM provider presets including GitHub Copilot.
func DefaultPresets() []ProviderPreset {
	return []ProviderPreset{
		{
			ID:          "github-copilot",
			Name:        "GitHub Copilot (Native)",
			Format:      string(llm.FormatOpenAI),
			BaseURL:     "https://api.githubcopilot.com",
			AuthType:    "copilot_device_flow",
			Endpoints:   []string{"/chat/completions", "/responses", "/v1/messages", "/models"},
			Description: "Native GitHub Copilot integration via OAuth Device Flow & automatic session token exchange.",
		},
		{
			ID:          "openai",
			Name:        "OpenAI",
			Format:      string(llm.FormatOpenAI),
			BaseURL:     "https://api.openai.com/v1",
			AuthType:    "api_key",
			Endpoints:   []string{"/chat/completions", "/responses"},
			Description: "OpenAI API (Chat Completions & Responses wire formats).",
		},
		{
			ID:          "anthropic",
			Name:        "Anthropic Claude",
			Format:      string(llm.FormatAnthropic),
			BaseURL:     "https://api.anthropic.com",
			AuthType:    "api_key",
			Endpoints:   []string{"/v1/messages"},
			Description: "Anthropic Messages API with extended thinking support.",
		},
		{
			ID:          "gemini",
			Name:        "Google Gemini (OpenAI Compatible)",
			Format:      string(llm.FormatOpenAI),
			BaseURL:     "https://generativelanguage.googleapis.com/v1beta/openai",
			AuthType:    "api_key",
			Endpoints:   []string{"/chat/completions"},
			Description: "Google Gemini API via OpenAI compatibility layer.",
		},
		{
			ID:          "openrouter",
			Name:        "OpenRouter",
			Format:      string(llm.FormatOpenAI),
			BaseURL:     "https://openrouter.ai/api/v1",
			AuthType:    "api_key",
			Endpoints:   []string{"/chat/completions"},
			Description: "Unified multi-model router for hundreds of foundation models.",
		},
		{
			ID:          "ollama",
			Name:        "Ollama / Local OpenAI-Compatible",
			Format:      string(llm.FormatOpenAI),
			BaseURL:     "http://localhost:11434/v1",
			AuthType:    "none",
			Endpoints:   []string{"/chat/completions"},
			Description: "Local LLM execution via Ollama, vLLM, or LM Studio.",
		},
	}
}

// Runtime coordinates Norma agent sessions, built-in CORTEX tools, and skills.
type Runtime struct {
	cfg    config.Config
	store  *store.Store
	i18n   *i18n.Manager
	skills *skill.Registry
}

// NewRuntime initializes the Norma-backed CORTEX agent runtime.
func NewRuntime(cfg config.Config, st *store.Store, i18nMgr *i18n.Manager) *Runtime {
	var skillsReg *skill.Registry
	if info, err := os.Stat(cfg.SkillsDir); err == nil && info.IsDir() {
		if reg, err := skill.LoadDir(cfg.SkillsDir); err == nil {
			skillsReg = reg
		}
	}
	return &Runtime{
		cfg:    cfg,
		store:  st,
		i18n:   i18nMgr,
		skills: skillsReg,
	}
}

// SkillsList returns the names and descriptions of loaded skills.
func (r *Runtime) SkillsList() []map[string]string {
	if r.skills == nil {
		return []map[string]string{}
	}
	list := r.skills.List()
	out := make([]map[string]string, 0, len(list))
	for _, s := range list {
		out = append(out, map[string]string{
			"name":        s.Name,
			"description": s.Description,
		})
	}
	return out
}

// NewSession constructs a Norma agentcore.Session equipped with CORTEX domain
// collection tools, timer scheduling tools, and locale-aware system prompts.
func (r *Runtime) NewSession(localeOverride string) (*agentcore.Session, error) {
	format := llm.FormatOpenAI
	if r.cfg.DefaultProvider == "anthropic" {
		format = llm.FormatAnthropic
	} else if r.cfg.DefaultProvider == "openai-responses" {
		format = llm.FormatOpenAIResponses
	}

	prov, err := llm.NewProvider(llm.Config{
		Format: format,
		Model:  r.cfg.DefaultModel,
	})
	if err != nil {
		return nil, err
	}

	tools := append(tool.DefaultTools(), r.cortexTools()...)

	sysPrompt := []string{
		"You are CORTEX, an autonomous AI Agent running inside the CORTEX application framework.",
		"You have access to generic JSON domain collections (collection_list, collection_get, collection_put, collection_delete) and future task scheduling (schedule_timer).",
		r.i18n.AgentLanguageDirective(localeOverride),
	}

	sess := agentcore.NewSession(agentcore.Options{
		Provider:        prov,
		SystemPrompt:    sysPrompt,
		Tools:           tools,
		Skills:          r.skills,
		EnableWebFetch:  true,
		EnableWebSearch: true,
		PermissionMode:  permission.ModeAuto,
	})
	return sess, nil
}

// StreamPrompt runs a single prompt through a Norma session and yields harness events.
func (r *Runtime) StreamPrompt(ctx context.Context, prompt, locale string) (iter.Seq2[harness.Event, error], func(), error) {
	sess, err := r.NewSession(locale)
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = sess.Close() }
	return sess.Prompt(ctx, prompt), cleanup, nil
}

func (r *Runtime) cortexTools() []tool.CoreTool {
	allowAll := func(context.Context, json.RawMessage, permission.Context) permission.Decision {
		return permission.Allowed()
	}

	colList := tool.Build(tool.Spec{
		Name:        "collection_list",
		Description: "List all documents in a named CORTEX JSON collection (or list all collection names when collection is omitted).",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{
					"type":        "string",
					"description": "Optional collection name. Omit or leave empty to list available collections.",
				},
			},
		},
		ReadOnly:    func(json.RawMessage) bool { return true },
		Concurrent:  func(json.RawMessage) bool { return true },
		Permissions: allowAll,
		Run: func(ctx context.Context, input json.RawMessage, _ *tool.ToolContext) (tool.Result, error) {
			var in struct {
				Collection string `json:"collection"`
			}
			_ = json.Unmarshal(input, &in)
			if in.Collection == "" {
				names, err := r.store.ListCollections(ctx)
				if err != nil {
					return tool.Errorf(err.Error()), nil
				}
				b, _ := json.Marshal(names)
				return tool.Text(string(b)), nil
			}
			items, err := r.store.ListItems(ctx, in.Collection)
			if err != nil {
				return tool.Errorf(err.Error()), nil
			}
			b, _ := json.Marshal(items)
			return tool.Text(string(b)), nil
		},
	})

	colGet := tool.Build(tool.Spec{
		Name:        "collection_get",
		Description: "Fetch a single JSON document by collection name and document ID.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{"type": "string", "description": "Collection name"},
				"id":         map[string]any{"type": "string", "description": "Document ID"},
			},
			"required": []any{"collection", "id"},
		},
		ReadOnly:    func(json.RawMessage) bool { return true },
		Concurrent:  func(json.RawMessage) bool { return true },
		Permissions: allowAll,
		Run: func(ctx context.Context, input json.RawMessage, _ *tool.ToolContext) (tool.Result, error) {
			var in struct {
				Collection string `json:"collection"`
				ID         string `json:"id"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return tool.Errorf(err.Error()), nil
			}
			item, err := r.store.GetItem(ctx, in.Collection, in.ID)
			if err != nil {
				return tool.Errorf(err.Error()), nil
			}
			b, _ := json.Marshal(item)
			return tool.Text(string(b)), nil
		},
	})

	colPut := tool.Build(tool.Spec{
		Name:        "collection_put",
		Description: "Create or update a JSON document in a named CORTEX collection.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{"type": "string", "description": "Collection name"},
				"id":         map[string]any{"type": "string", "description": "Document ID"},
				"data":       map[string]any{"type": "object", "description": "JSON document body"},
			},
			"required": []any{"collection", "id", "data"},
		},
		Permissions: allowAll,
		Run: func(ctx context.Context, input json.RawMessage, _ *tool.ToolContext) (tool.Result, error) {
			var in struct {
				Collection string          `json:"collection"`
				ID         string          `json:"id"`
				Data       json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return tool.Errorf(err.Error()), nil
			}
			item, err := r.store.PutItem(ctx, in.Collection, in.ID, in.Data)
			if err != nil {
				return tool.Errorf(err.Error()), nil
			}
			b, _ := json.Marshal(item)
			return tool.Text(string(b)), nil
		},
	})

	colDelete := tool.Build(tool.Spec{
		Name:        "collection_delete",
		Description: "Delete a document from a named CORTEX collection.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{"type": "string", "description": "Collection name"},
				"id":         map[string]any{"type": "string", "description": "Document ID"},
			},
			"required": []any{"collection", "id"},
		},
		Permissions: allowAll,
		Run: func(ctx context.Context, input json.RawMessage, _ *tool.ToolContext) (tool.Result, error) {
			var in struct {
				Collection string `json:"collection"`
				ID         string `json:"id"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return tool.Errorf(err.Error()), nil
			}
			if err := r.store.DeleteItem(ctx, in.Collection, in.ID); err != nil {
				return tool.Errorf(err.Error()), nil
			}
			return tool.Text(`{"deleted":true}`), nil
		},
	})

	scheduleTimer := tool.Build(tool.Spec{
		Name:        "schedule_timer",
		Description: "Schedule a future or recurring autonomous agent task (cron, interval, or one-shot run_at timestamp).",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":     map[string]any{"type": "string", "description": "Human-readable timer name"},
				"schedule": map[string]any{"type": "string", "description": "Cron expression, duration (e.g. '15m'), or 'once'"},
				"run_at":   map[string]any{"type": "string", "description": "RFC3339 timestamp when schedule is 'once'"},
				"prompt":   map[string]any{"type": "string", "description": "Prompt to execute when triggered"},
			},
			"required": []any{"name", "schedule", "prompt"},
		},
		Permissions: allowAll,
		Run: func(ctx context.Context, input json.RawMessage, _ *tool.ToolContext) (tool.Result, error) {
			var in struct {
				Name     string `json:"name"`
				Schedule string `json:"schedule"`
				RunAt    string `json:"run_at"`
				Prompt   string `json:"prompt"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return tool.Errorf(err.Error()), nil
			}
			id := fmt.Sprintf("timer-%d", time.Now().UTC().UnixNano())
			entry, err := r.store.SaveTimer(ctx, store.TimerEntry{
				ID:        id,
				Name:      in.Name,
				Schedule:  in.Schedule,
				RunAt:     in.RunAt,
				Prompt:    in.Prompt,
				Enabled:   true,
				CreatedBy: "agent",
			})
			if err != nil {
				return tool.Errorf(err.Error()), nil
			}
			b, _ := json.Marshal(entry)
			return tool.Text(string(b)), nil
		},
	})

	return []tool.CoreTool{colList, colGet, colPut, colDelete, scheduleTimer}
}
