package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/harness"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/mcp"
	"github.com/Autumn-27/norma/permission"
	"github.com/Autumn-27/norma/skill"
	"github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"

	"github.com/GOAT-Foundry/CORTEX/internal/config"
	"github.com/GOAT-Foundry/CORTEX/internal/i18n"
	"github.com/GOAT-Foundry/CORTEX/internal/llmhub"
	"github.com/GOAT-Foundry/CORTEX/internal/scheduler"
	"github.com/GOAT-Foundry/CORTEX/internal/store"
	"github.com/GOAT-Foundry/CORTEX/internal/taskengine"
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
	sched       *scheduler.Engine
	tasks       *taskengine.Engine
	skillsMu    sync.RWMutex
	skills      *skill.Registry
	transcripts *transcript.Store
}

// NewRuntime initializes the Norma-backed CORTEX agent runtime.
func NewRuntime(cfg config.Config, st *store.Store, i18nMgr *i18n.Manager) *Runtime {
	if cfg.SkillsDir != "" {
		seedDefaultSkills(cfg.SkillsDir)
	}
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
	rt.sched = scheduler.New(st, rt.RunScheduledPrompt)
	rt.tasks = taskengine.New(st, rt.RunWorkerPrompt)
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

type builtinSkillDef struct {
	DirName      string
	Name         string
	Description  string
	Instructions string
}

// DefaultBuiltinSkills returns the standard SKILL.md definitions bundled with CORTEX.
func DefaultBuiltinSkills() []builtinSkillDef {
	return []builtinSkillDef{
		{
			DirName:     "example",
			Name:        "domain-collection-helper",
			Description: "Guide for reading and writing structured application state in CORTEX JSON Collections and scheduling follow-up timers.",
			Instructions: "# CORTEX Domain Collection & Timer Skill\n\n" +
				"1. Inspect existing collections first using `collection_list` with an empty `collection` parameter.\n" +
				"2. Read records using `collection_list` or `collection_get`.\n" +
				"3. Persist domain artifacts or records using `collection_put` so the Next.js frontend can render them via `/api/collections/{name}`.\n" +
				"4. Schedule follow-ups using `schedule_timer` when a task requires periodic monitoring or a deferred check (`schedule: \"once\"` with `run_at`).",
		},
		{
			DirName:     "web-research",
			Name:        "web-research",
			Description: "Methodical web search (ddgs) and page retrieval (WebFetch) workflow for gathering, verifying, and synthesizing factual information with citations.",
			Instructions: "# Web Research & Source Verification Skill\n\n" +
				"1. Issue concise, targeted queries via `web_search` (`ddgs` default).\n" +
				"2. Fetch authoritative primary URLs via `WebFetch` rather than relying on snippets alone.\n" +
				"3. Cross-verify critical claims across at least two sources and include URLs in your report or collection record.",
		},
		{
			DirName:     "data-analysis",
			Name:        "data-analysis",
			Description: "Structured data aggregation, JSON/CSV statistical processing, and Markdown/JSON report generation using CORTEX collections and Python/shell scripts.",
			Instructions: "# Data Analysis & Reporting Skill\n\n" +
				"1. Load domain records via `collection_list`/`collection_get` or inspect local files with `Read`.\n" +
				"2. Compute exact sums, averages, and groupings deterministically using Python (`python3 -c` / `python -c`).\n" +
				"3. Persist structured results with `collection_put` and present a Markdown summary table.",
		},
		{
			DirName:     "scheduled-monitoring",
			Name:        "scheduled-monitoring",
			Description: "Patterns for recurring Cron/interval checks, one-shot deferred follow-ups, and stateful alerting using schedule_timer and CORTEX collections.",
			Instructions: "# Scheduled Monitoring & Autonomous Follow-Up Skill\n\n" +
				"1. Check `list_timers` first to avoid duplicate schedules.\n" +
				"2. Use fixed intervals (`\"15m\"`, `\"1h\"`), 5-field UTC Cron (`\"0 * * * *\"`), or `schedule: \"once\"` with RFC3339 `run_at`.\n" +
				"3. Write self-contained timer prompts that read prior state via `collection_get` and persist updates via `collection_put`.",
		},
		{
			DirName:     "shell-automation",
			Name:        "shell-automation",
			Description: "Cross-platform shell and PowerShell execution guide for Norma's Bash tool and CORTEX Custom Tools across Linux/Docker (bash) and Windows (PowerShell).",
			Instructions: "# Cross-Platform Shell & PowerShell Automation Skill\n\n" +
				"Norma's `Bash` tool automatically selects `powershell -NoProfile -NonInteractive -Command` on Windows and `bash -c` on Linux/macOS/Docker.\n" +
				"- On Windows (`powershell`): Do NOT use POSIX heredocs (`<< 'EOF'`); use PowerShell here-strings (`@'... '@`) or `python -c`. Chain built-ins with `;`.\n" +
				"- On Linux/Docker (`bash`): Pass non-interactive flags (`--no-pager`, `-y`) and prefer Python scripts for complex parsing.",
		},
	}
}

func seedDefaultSkills(skillsDir string) {
	for _, s := range DefaultBuiltinSkills() {
		dir := filepath.Join(skillsDir, s.DirName)
		skillPath := filepath.Join(dir, "SKILL.md")
		if _, err := os.Stat(skillPath); err == nil {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		var b strings.Builder
		b.WriteString("---\n")
		b.WriteString("name: " + s.Name + "\n")
		b.WriteString("description: " + s.Description + "\n")
		b.WriteString("license: MIT\n")
		b.WriteString("---\n\n")
		b.WriteString(s.Instructions + "\n")
		_ = os.WriteFile(skillPath, []byte(b.String()), 0o644)
	}
}

// HostEnvironmentReminder returns a system reminder segment describing the host OS
// and the shell executable used by Norma's Bash tool.
func HostEnvironmentReminder() string {
	if runtime.GOOS == "windows" {
		return "# hostEnvironment\nOS: windows (Norma `Bash` tool executes via `powershell -NoProfile -NonInteractive -Command`; use PowerShell syntax or `python -c`, avoid POSIX heredocs)."
	}
	return fmt.Sprintf("# hostEnvironment\nOS: %s (Norma `Bash` tool executes via `bash -c`).", runtime.GOOS)
}

// Hub returns the underlying LLM Provider & Copilot Hub.
func (r *Runtime) Hub() *llmhub.Hub {
	return r.hub
}

// Scheduler returns the background Timer & Cron Scheduler engine.
func (r *Runtime) Scheduler() *scheduler.Engine {
	return r.sched
}

// TaskEngine returns the autonomous Planner/Worker DAG Task Engine.
func (r *Runtime) TaskEngine() *taskengine.Engine {
	return r.tasks
}

// RunScheduledPrompt executes a timer prompt in an isolated Norma session and returns the output text.
func (r *Runtime) RunScheduledPrompt(ctx context.Context, agentID, prompt string) (string, error) {
	return r.RunWorkerPrompt(ctx, agentID, prompt, "")
}

// RunWorkerPrompt executes an autonomous worker prompt in an isolated Norma session with locale sync.
func (r *Runtime) RunWorkerPrompt(ctx context.Context, agentID, prompt, locale string) (string, error) {
	sess, cleanup, err := r.NewSessionWithCleanup(ctx, SessionOptions{
		AgentID: agentID,
		Locale:  locale,
	})
	if err != nil {
		return "", err
	}
	defer cleanup()

	var text string
	var termErr error
	for ev, err := range sess.Prompt(ctx, prompt) {
		if err != nil {
			return text, err
		}
		if ev.Kind == harness.KindText {
			text += ev.Text
		} else if ev.Kind == harness.KindResult && ev.Terminal != nil {
			if text == "" && ev.Terminal.Text != "" {
				text = ev.Terminal.Text
			}
			if ev.Terminal.Err != nil {
				termErr = ev.Terminal.Err
			}
		}
	}
	return text, termErr
}

// SkillsList returns the names, descriptions, and instructions of loaded skills.
func (r *Runtime) SkillsList() []map[string]string {
	r.skillsMu.RLock()
	reg := r.skills
	r.skillsMu.RUnlock()
	if reg == nil {
		return []map[string]string{}
	}
	list := reg.List()
	out := make([]map[string]string, 0, len(list))
	for _, s := range list {
		out = append(out, map[string]string{
			"name":         s.Name,
			"description":  s.Description,
			"instructions": s.Instructions,
		})
	}
	return out
}

// ReloadSkills reloads the skill registry from r.cfg.SkillsDir.
func (r *Runtime) ReloadSkills() error {
	if r.cfg.SkillsDir == "" {
		return nil
	}
	if err := os.MkdirAll(r.cfg.SkillsDir, 0o755); err != nil {
		return err
	}
	reg, err := skill.LoadDir(r.cfg.SkillsDir)
	if err != nil {
		return err
	}
	r.skillsMu.Lock()
	r.skills = reg
	r.skillsMu.Unlock()
	return nil
}

// SaveSkill writes a SKILL.md package under SkillsDir/<name>/SKILL.md and hot-reloads the registry.
func (r *Runtime) SaveSkill(name, description, instructions string) error {
	if err := store.ValidateIdent(name); err != nil {
		return err
	}
	dir := filepath.Join(r.cfg.SkillsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: " + name + "\n")
	b.WriteString("description: " + strings.ReplaceAll(strings.TrimSpace(description), "\n", " ") + "\n")
	b.WriteString("license: MIT\n")
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(instructions) + "\n")

	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(b.String()), 0o644); err != nil {
		return err
	}
	return r.ReloadSkills()
}

// DeleteSkill removes a skill directory under SkillsDir/<name> (or its resolved Dir) and reloads the registry.
func (r *Runtime) DeleteSkill(name string) error {
	if err := store.ValidateIdent(name); err != nil {
		return err
	}
	r.skillsMu.RLock()
	reg := r.skills
	r.skillsMu.RUnlock()
	if reg != nil {
		if s, ok := reg.Get(name); ok && s.Dir != "" {
			_ = os.RemoveAll(s.Dir)
		}
	}
	dir := filepath.Join(r.cfg.SkillsDir, name)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return r.ReloadSkills()
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
	sess, _, err := r.NewSessionWithCleanup(ctx, opts)
	return sess, err
}

// NewSessionWithCleanup constructs a stateful Norma Session and returns a cleanup function
// that closes both the session and any spawned MCP stdio subprocesses.
func (r *Runtime) NewSessionWithCleanup(ctx context.Context, opts SessionOptions) (*agentcore.Session, func(), error) {
	if opts.SessionID != "" {
		if err := store.ValidateIdent(opts.SessionID); err != nil {
			return nil, func() {}, err
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
		return nil, func() {}, err
	}

	tools := append(tool.DefaultTools(), r.cortexTools()...)
	tools = append(tools, r.buildCustomTools(ctx)...)
	mcpTools, mcpCleanup := r.connectMCPTools(ctx)
	tools = append(tools, mcpTools...)

	sysPrompt := []string{
		personaPrompt,
		"You have access to generic JSON domain collections (collection_list, collection_get, collection_put, collection_delete) and future task scheduling (schedule_timer).",
		"Always follow the # activeLocale directive in <system-reminder> for your response and artifact language.",
	}

	r.skillsMu.RLock()
	skillsReg := r.skills
	r.skillsMu.RUnlock()

	wsBackend := "ddgs"
	var braveKey, tavilyKey string
	if r.store != nil {
		if b, err := r.store.GetSetting(ctx, "web_search_backend"); err == nil && b != "" {
			wsBackend = b
		}
		if k, err := r.store.GetSetting(ctx, "brave_api_key"); err == nil {
			braveKey = k
		}
		if k, err := r.store.GetSetting(ctx, "tavily_api_key"); err == nil {
			tavilyKey = k
		}
	}
	if wsBackend == "brave-free" && braveKey == "" && os.Getenv("BRAVE_SEARCH_API_KEY") == "" {
		wsBackend = "ddgs"
	}
	if wsBackend == "tavily" && tavilyKey == "" && os.Getenv("TAVILY_API_KEY") == "" {
		wsBackend = "ddgs"
	}

	acOpts := agentcore.Options{
		Provider:           prov,
		SystemPrompt:       sysPrompt,
		Tools:              tools,
		Skills:             skillsReg,
		EnableWebFetch:     true,
		EnableWebSearch:    true,
		WebSearchBackend:   wsBackend,
		BraveSearchAPIKey:  braveKey,
		TavilySearchAPIKey: tavilyKey,
		PermissionMode:     permission.ModeAuto,
		Transcript:         r.transcripts,
		SessionID:          opts.SessionID,
		SystemReminderFunc: func() string {
			return r.i18n.SystemReminder(opts.Locale) + "\n" + HostEnvironmentReminder()
		},
	}

	sess := agentcore.NewSession(acOpts)
	if opts.SessionID != "" && r.transcripts != nil {
		_ = sess.Resume(opts.SessionID)
	}
	cleanup := func() {
		_ = sess.Close()
		mcpCleanup()
	}
	return sess, cleanup, nil
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
	sess, cleanup, err := r.NewSessionWithCleanup(ctx, SessionOptions{
		Locale: locale,
	})
	if err != nil {
		return nil, func() {}, err
	}
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
		Description: "Schedule a future or recurring autonomous agent task (5-field cron expression, duration interval like '15m', or 'once' with an RFC3339 run_at timestamp).",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":     map[string]any{"type": "string", "description": "Human-readable timer name"},
				"schedule": map[string]any{"type": "string", "description": "5-field cron (e.g. '0 * * * *'), duration (e.g. '15m'), or 'once'"},
				"run_at":   map[string]any{"type": "string", "description": "RFC3339 timestamp when schedule is 'once'"},
				"prompt":   map[string]any{"type": "string", "description": "Prompt to execute when triggered"},
				"agent_id": map[string]any{"type": "string", "description": "Optional target agent persona ID (default 'default')"},
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
				AgentID  string `json:"agent_id"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return tool.Errorf(err.Error()), nil
			}
			nextRun, err := scheduler.ValidateSchedule(in.Schedule, in.RunAt, time.Now().UTC())
			if err != nil {
				return tool.Errorf(err.Error()), nil
			}
			id := fmt.Sprintf("timer-%d", time.Now().UTC().UnixNano())
			entry, err := r.store.SaveTimer(ctx, store.TimerEntry{
				ID:        id,
				Name:      in.Name,
				Schedule:  in.Schedule,
				RunAt:     in.RunAt,
				Prompt:    in.Prompt,
				AgentID:   in.AgentID,
				NextRunAt: nextRun.Format(time.RFC3339),
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

	listTimers := tool.Build(tool.Spec{
		Name:        "list_timers",
		Description: "List all scheduled recurring and one-shot timers in CORTEX.",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		ReadOnly:    func(json.RawMessage) bool { return true },
		Concurrent:  func(json.RawMessage) bool { return true },
		Permissions: allowAll,
		Run: func(ctx context.Context, _ json.RawMessage, _ *tool.ToolContext) (tool.Result, error) {
			timers, err := r.store.ListTimers(ctx)
			if err != nil {
				return tool.Errorf(err.Error()), nil
			}
			b, _ := json.Marshal(timers)
			return tool.Text(string(b)), nil
		},
	})

	cancelTimer := tool.Build(tool.Spec{
		Name:        "cancel_timer",
		Description: "Cancel and delete a scheduled timer by its ID.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "Timer ID to cancel"},
			},
			"required": []any{"id"},
		},
		Permissions: allowAll,
		Run: func(ctx context.Context, input json.RawMessage, _ *tool.ToolContext) (tool.Result, error) {
			var in struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return tool.Errorf(err.Error()), nil
			}
			if err := r.store.DeleteTimer(ctx, in.ID); err != nil {
				return tool.Errorf(err.Error()), nil
			}
			return tool.Text(`{"canceled":true}`), nil
		},
	})

	return []tool.CoreTool{colList, colGet, colPut, colDelete, scheduleTimer, listTimers, cancelTimer}
}

func (r *Runtime) buildCustomTools(ctx context.Context) []tool.CoreTool {
	if r.store == nil {
		return nil
	}
	configs, err := r.store.ListCustomTools(ctx)
	if err != nil {
		return nil
	}
	allowAll := func(context.Context, json.RawMessage, permission.Context) permission.Decision {
		return permission.Allowed()
	}
	out := make([]tool.CoreTool, 0, len(configs))
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		c := cfg
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string", "description": "Tool input"},
			},
		}
		if c.SchemaJSON != "" {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(c.SchemaJSON), &parsed); err == nil && len(parsed) > 0 {
				schema = parsed
			}
		}
		out = append(out, tool.Build(tool.Spec{
			Name:        c.Name,
			Description: c.Description,
			Schema:      schema,
			Permissions: allowAll,
			Run: func(runCtx context.Context, input json.RawMessage, _ *tool.ToolContext) (tool.Result, error) {
				return executeCustomTool(runCtx, c, input)
			},
		}))
	}
	return out
}

func executeCustomTool(ctx context.Context, c store.CustomToolConfig, input json.RawMessage) (tool.Result, error) {
	runCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	switch c.Runner {
	case "http":
		method := strings.ToUpper(strings.TrimSpace(c.Method))
		if method == "" {
			method = http.MethodPost
		}
		var bodyReader io.Reader
		if method != http.MethodGet && method != http.MethodHead {
			bodyReader = bytes.NewReader(input)
		}
		req, err := http.NewRequestWithContext(runCtx, method, c.Target, bodyReader)
		if err != nil {
			return tool.Errorf(err.Error()), nil
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return tool.Errorf(err.Error()), nil
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
		if resp.StatusCode >= 400 {
			return tool.Errorf(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(raw))), nil
		}
		return tool.Text(string(raw)), nil

	case "python":
		pyBin := "python3"
		if _, err := exec.LookPath(pyBin); err != nil {
			pyBin = "python"
		}
		cmd := exec.CommandContext(runCtx, pyBin, "-c", c.Target)
		cmd.Stdin = bytes.NewReader(input)
		cmd.Env = append(os.Environ(), "CORTEX_TOOL_INPUT="+string(input))
		out, err := cmd.CombinedOutput()
		if err != nil {
			return tool.Errorf(fmt.Sprintf("%v: %s", err, string(out))), nil
		}
		return tool.Text(string(out)), nil

	case "bash":
		shBin := "bash"
		if _, err := exec.LookPath(shBin); err != nil {
			shBin = "sh"
		}
		cmd := exec.CommandContext(runCtx, shBin, "-c", c.Target)
		cmd.Stdin = bytes.NewReader(input)
		cmd.Env = append(os.Environ(), "CORTEX_TOOL_INPUT="+string(input))
		out, err := cmd.CombinedOutput()
		if err != nil {
			return tool.Errorf(fmt.Sprintf("%v: %s", err, string(out))), nil
		}
		return tool.Text(string(out)), nil

	case "powershell":
		psBin, err := exec.LookPath("pwsh")
		if err != nil {
			psBin, err = exec.LookPath("powershell")
		}
		if err != nil {
			return tool.Errorf("powershell runner requires 'pwsh' or 'powershell' installed on the host (use 'bash' or 'python' on Linux/Alpine containers)"), nil
		}
		cmd := exec.CommandContext(runCtx, psBin, "-NoProfile", "-NonInteractive", "-Command", c.Target)
		cmd.Stdin = bytes.NewReader(input)
		cmd.Env = append(os.Environ(), "CORTEX_TOOL_INPUT="+string(input))
		out, err := cmd.CombinedOutput()
		if err != nil {
			return tool.Errorf(fmt.Sprintf("%v: %s", err, string(out))), nil
		}
		return tool.Text(string(out)), nil

	default:
		return tool.Errorf("unsupported runner: " + c.Runner), nil
	}
}

func (r *Runtime) connectMCPTools(ctx context.Context) ([]tool.CoreTool, func()) {
	if r.store == nil {
		return nil, func() {}
	}
	servers, err := r.store.ListMCPServers(ctx)
	if err != nil || len(servers) == 0 {
		return nil, func() {}
	}
	var (
		out     []tool.CoreTool
		clients []*mcp.Client
	)
	for _, srv := range servers {
		if !srv.Enabled || strings.TrimSpace(srv.Command) == "" {
			continue
		}
		initCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		client, err := mcp.NewStdioClient(initCtx, srv.ID, srv.Command, srv.Env, srv.Args...)
		if err != nil {
			cancel()
			continue
		}
		tools, err := client.Tools(initCtx)
		cancel()
		if err != nil {
			_ = client.Close()
			continue
		}
		clients = append(clients, client)
		out = append(out, tools...)
	}
	cleanup := func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}
	return out, cleanup
}
