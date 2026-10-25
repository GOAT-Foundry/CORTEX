package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/harness"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/permission"
	"github.com/Autumn-27/norma/skill"
	"github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"

	"github.com/GOAT-Foundry/CORTEX/internal/config"
	"github.com/GOAT-Foundry/CORTEX/internal/i18n"
	"github.com/GOAT-Foundry/CORTEX/internal/llmhub"
	"github.com/GOAT-Foundry/CORTEX/internal/store"
)

// ProviderPreset is an alias for llmhub.Preset for backward compatibility.
type ProviderPreset = llmhub.Preset

// DefaultPresets returns the built-in LLM provider presets including GitHub Copilot.
func DefaultPresets() []ProviderPreset {
	return llmhub.DefaultPresets()
}

// DefaultAgents returns the built-in AI agent personas seeded on startup.
func DefaultAgents() []store.AgentProfile {
	return []store.AgentProfile{
		{
			ID:           "default",
			Name:         "CORTEX General Agent",
			Description:  "Balanced general-purpose agent with access to domain JSON collections, timers, skills, and web tools.",
			SystemPrompt: "You are CORTEX, a helpful and autonomous general-purpose AI Agent running inside the CORTEX application framework.",
		},
		{
			ID:           "coder",
			Name:         "Code & Architecture Specialist",
			Description:  "Focused on clean software architecture, debugging, code generation, and structured technical artifacts.",
			SystemPrompt: "You are the CORTEX Code & Architecture Specialist. Produce clean, modular, production-grade code and concise technical explanations.",
		},
		{
			ID:           "researcher",
			Name:         "Deep Research & Domain Analyst",
			Description:  "Specializes in multi-step investigation, data synthesis, collection querying, and structured report writing.",
			SystemPrompt: "You are the CORTEX Deep Research & Domain Analyst. Gather evidence methodically, cross-check facts, and organize findings into clear structured reports.",
		},
	}
}

// Runtime coordinates Norma agent sessions, LLM Hub routing, built-in CORTEX tools, and skills.
type Runtime struct {
	cfg         config.Config
	store       *store.Store
	i18n        *i18n.Manager
	hub         *llmhub.Hub
	skills      *skill.Registry
	transcripts *transcript.Store
}

// NewRuntime initializes the Norma-backed CORTEX agent runtime.
func NewRuntime(cfg config.Config, st *store.Store, i18nMgr *i18n.Manager) *Runtime {
	var skillsReg *skill.Registry
	if info, err := os.Stat(cfg.SkillsDir); err == nil && info.IsDir() {
		if reg, err := skill.LoadDir(cfg.SkillsDir); err == nil {
			skillsReg = reg
		}
	}
	transcriptDir := filepath.Join(os.TempDir(), "cortex-transcripts")
	if cfg.DBPath != "" && cfg.DBPath != ":memory:" {
		transcriptDir = filepath.Join(filepath.Dir(cfg.DBPath), "transcripts")
	}
	rt := &Runtime{
		cfg:         cfg,
		store:       st,
		i18n:        i18nMgr,
		hub:         llmhub.New(st),
		skills:      skillsReg,
		transcripts: transcript.NewStore(transcriptDir),
	}
	if st != nil {
		rt.seedDefaultAgents(context.Background())
	}
	return rt
}

func (r *Runtime) seedDefaultAgents(ctx context.Context) {
	existing, err := r.store.ListAgents(ctx)
	if err != nil || len(existing) > 0 {
		return
	}
	for _, a := range DefaultAgents() {
		_, _ = r.store.SaveAgent(ctx, a)
	}
}

// Hub returns the underlying LLM Provider & Copilot Hub.
func (r *Runtime) Hub() *llmhub.Hub {
	return r.hub
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
	return r.NewSessionWithOptions(context.Background(), SessionOptions{
		Locale: localeOverride,
	})
}

// SessionOptions configures a CORTEX agent session.
type SessionOptions struct {
	SessionID  string
	AgentID    string
	ProviderID string
	Model      string
	Locale     string
}

// NewSessionWithProvider constructs a Norma agentcore.Session routed through the
// LLM Hub (with failover and token/cost accounting).
func (r *Runtime) NewSessionWithProvider(ctx context.Context, localeOverride, providerID, modelOverride string) (*agentcore.Session, error) {
	return r.NewSessionWithOptions(ctx, SessionOptions{
		ProviderID: providerID,
		Model:      modelOverride,
		Locale:     localeOverride,
	})
}

// NewSessionWithOptions constructs and optionally resumes a stateful Norma Session.
func (r *Runtime) NewSessionWithOptions(ctx context.Context, opts SessionOptions) (*agentcore.Session, error) {
	if opts.SessionID != "" {
		if err := store.ValidateIdent(opts.SessionID); err != nil {
			return nil, err
		}
	}
	personaPrompt := "You are CORTEX, an autonomous AI Agent running inside the CORTEX application framework."
	providerID := strings.TrimSpace(opts.ProviderID)
	modelOverride := strings.TrimSpace(opts.Model)

	if r.store != nil && opts.AgentID != "" {
		if ag, err := r.store.GetAgent(ctx, opts.AgentID); err == nil {
			if strings.TrimSpace(ag.SystemPrompt) != "" {
				personaPrompt = ag.SystemPrompt
			}
			if providerID == "" && ag.ProviderID != "" {
				providerID = ag.ProviderID
			}
			if modelOverride == "" && ag.Model != "" {
				modelOverride = ag.Model
			}
		}
	}

	var (
		prov llm.Provider
		err  error
	)
	if r.hub != nil {
		prov, err = r.hub.BuildProvider(ctx, providerID, modelOverride)
	} else {
		prov, err = llm.NewProvider(llm.Config{
			Format: llm.FormatOpenAI,
			Model:  r.cfg.DefaultModel,
		})
	}
	if err != nil {
		return nil, err
	}

	tools := append(tool.DefaultTools(), r.cortexTools()...)

	sysPrompt := []string{
		personaPrompt,
		"You have access to generic JSON domain collections (collection_list, collection_get, collection_put, collection_delete) and future task scheduling (schedule_timer).",
		"Always follow the # activeLocale directive in <system-reminder> for your response and artifact language.",
	}

	acOpts := agentcore.Options{
		Provider:        prov,
		SystemPrompt:    sysPrompt,
		Tools:           tools,
		Skills:          r.skills,
		EnableWebFetch:  true,
		EnableWebSearch: true,
		PermissionMode:  permission.ModeAuto,
		Transcript:      r.transcripts,
		SessionID:       opts.SessionID,
		SystemReminderFunc: func() string {
			return r.i18n.SystemReminder(opts.Locale)
		},
	}

	sess := agentcore.NewSession(acOpts)
	if opts.SessionID != "" && r.transcripts != nil {
		_ = sess.Resume(opts.SessionID)
	}
	return sess, nil
}

// DeleteSessionTranscript removes the on-disk Norma transcript file for sessionID.
func (r *Runtime) DeleteSessionTranscript(sessionID string) error {
	if err := store.ValidateIdent(sessionID); err != nil {
		return err
	}
	if r == nil || r.transcripts == nil {
		return nil
	}
	path := r.transcripts.MainPath(sessionID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
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

// ChatTurnInput holds the parameters for a multi-turn chat message execution.
type ChatTurnInput struct {
	SessionID   string
	AgentID     string
	ProviderID  string
	Model       string
	Prompt      string
	Locale      string
	Attachments []store.ChatAttachment
}

// FormatPromptWithAttachments combines the user prompt with any attached text/file blocks.
func FormatPromptWithAttachments(prompt string, attachments []store.ChatAttachment) string {
	if len(attachments) == 0 {
		return prompt
	}
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\n<attachments>\n")
	for _, att := range attachments {
		safeName := filepath.Base(att.Name)
		b.WriteString(fmt.Sprintf("<attachment name=%q media_type=%q size_bytes=\"%d\">\n", safeName, att.MediaType, att.SizeBytes))
		content := att.Content
		if len(content) > 32000 {
			content = content[:32000] + "\n...[truncated]"
		}
		b.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("</attachment>\n")
	}
	b.WriteString("</attachments>")
	return b.String()
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
