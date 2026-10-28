package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Autumn-27/norma/harness"

	"github.com/GOAT-Foundry/CORTEX/internal/agent"
	"github.com/GOAT-Foundry/CORTEX/internal/config"
	"github.com/GOAT-Foundry/CORTEX/internal/i18n"
	"github.com/GOAT-Foundry/CORTEX/internal/scheduler"
	"github.com/GOAT-Foundry/CORTEX/internal/store"
	"github.com/GOAT-Foundry/CORTEX/internal/webui"
)

// Server wires HTTP routes for REST APIs, SSE agent streams, and the embedded Next.js UI.
type Server struct {
	cfg            config.Config
	store          *store.Store
	i18n           *i18n.Manager
	runtime        *agent.Runtime
	mux            *http.ServeMux
	sessionMu      sync.Mutex
	activeSessions map[string]bool
}

// New constructs a configured CORTEX HTTP server.
func New(cfg config.Config, st *store.Store, i18nMgr *i18n.Manager, rt *agent.Runtime) *Server {
	if st != nil && i18nMgr != nil {
		if savedLocale, err := st.GetSetting(context.Background(), "locale"); err == nil && savedLocale != "" {
			i18nMgr.Set(savedLocale)
		}
	}
	s := &Server{
		cfg:            cfg,
		store:          st,
		i18n:           i18nMgr,
		runtime:        rt,
		mux:            http.NewServeMux(),
		activeSessions: map[string]bool{},
	}
	s.registerRoutes()
	return s
}

// ServeHTTP implements http.Handler with loopback-restricted CORS for local Next.js dev.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if isLoopbackOrigin(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Cortex-Locale")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func isLoopbackOrigin(origin string) bool {
	return origin != "" && (len(origin) >= 16 && (origin[:16] == "http://localhost" || origin[:16] == "http://127.0.0.1" ||
		(len(origin) >= 17 && (origin[:17] == "https://localhost" || origin[:17] == "https://127.0.0.1"))))
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/i18n", s.handleGetI18n)
	s.mux.HandleFunc("PUT /api/i18n", s.handlePutI18n)

	s.mux.HandleFunc("GET /api/providers", s.handleProviders)
	s.mux.HandleFunc("PUT /api/providers/{id}", s.handlePutProvider)
	s.mux.HandleFunc("POST /api/providers/test", s.handleTestProvider)
	s.mux.HandleFunc("GET /api/providers/copilot/status", s.handleCopilotStatus)
	s.mux.HandleFunc("POST /api/providers/copilot/device-code", s.handleCopilotDeviceCode)
	s.mux.HandleFunc("POST /api/providers/copilot/poll", s.handleCopilotPoll)
	s.mux.HandleFunc("GET /api/providers/copilot/models", s.handleCopilotModels)
	s.mux.HandleFunc("DELETE /api/providers/copilot/auth", s.handleCopilotDisconnect)
	s.mux.HandleFunc("GET /api/usage", s.handleUsage)
	s.mux.HandleFunc("GET /api/skills", s.handleSkills)
	s.mux.HandleFunc("POST /api/skills", s.handleSaveSkill)
	s.mux.HandleFunc("DELETE /api/skills/{name}", s.handleDeleteSkill)

	s.mux.HandleFunc("GET /api/mcp", s.handleListMCP)
	s.mux.HandleFunc("POST /api/mcp", s.handleSaveMCP)
	s.mux.HandleFunc("PUT /api/mcp/{id}", s.handleSaveMCP)
	s.mux.HandleFunc("DELETE /api/mcp/{id}", s.handleDeleteMCP)

	s.mux.HandleFunc("GET /api/tools/custom", s.handleListCustomTools)
	s.mux.HandleFunc("POST /api/tools/custom", s.handleSaveCustomTool)
	s.mux.HandleFunc("PUT /api/tools/custom/{id}", s.handleSaveCustomTool)
	s.mux.HandleFunc("DELETE /api/tools/custom/{id}", s.handleDeleteCustomTool)

	s.mux.HandleFunc("GET /api/websearch", s.handleGetWebSearch)
	s.mux.HandleFunc("PUT /api/websearch", s.handlePutWebSearch)

	// Generic JSON Collection Store API
	s.mux.HandleFunc("GET /api/collections", s.handleListCollections)
	s.mux.HandleFunc("GET /api/collections/{name}", s.handleListItems)
	s.mux.HandleFunc("POST /api/collections/{name}", s.handleCreateItem)
	s.mux.HandleFunc("GET /api/collections/{name}/{id}", s.handleGetItem)
	s.mux.HandleFunc("PUT /api/collections/{name}/{id}", s.handlePutItem)
	s.mux.HandleFunc("DELETE /api/collections/{name}/{id}", s.handleDeleteItem)

	// Timers / Scheduler API
	s.mux.HandleFunc("GET /api/timers", s.handleListTimers)
	s.mux.HandleFunc("POST /api/timers", s.handleCreateTimer)
	s.mux.HandleFunc("PUT /api/timers/{id}", s.handleUpdateTimer)
	s.mux.HandleFunc("DELETE /api/timers/{id}", s.handleDeleteTimer)
	s.mux.HandleFunc("POST /api/timers/{id}/run", s.handleRunTimerNow)
	s.mux.HandleFunc("GET /api/timers/runs", s.handleListTimerRuns)

	// Agents & Chat Session Management API
	s.mux.HandleFunc("GET /api/agents", s.handleListAgents)
	s.mux.HandleFunc("POST /api/agents", s.handleSaveAgent)
	s.mux.HandleFunc("PUT /api/agents/{id}", s.handleSaveAgent)
	s.mux.HandleFunc("DELETE /api/agents/{id}", s.handleDeleteAgent)

	s.mux.HandleFunc("GET /api/chat/sessions", s.handleListChatSessions)
	s.mux.HandleFunc("POST /api/chat/sessions", s.handleCreateChatSession)
	s.mux.HandleFunc("GET /api/chat/sessions/{id}", s.handleGetChatSession)
	s.mux.HandleFunc("PUT /api/chat/sessions/{id}", s.handleUpdateChatSession)
	s.mux.HandleFunc("DELETE /api/chat/sessions/{id}", s.handleDeleteChatSession)
	s.mux.HandleFunc("POST /api/chat/sessions/{id}/messages", s.handleSessionChat)

	// Chat SSE endpoint (supports both session_id in body and standalone calls)
	s.mux.HandleFunc("POST /api/chat", s.handleChat)

	// Embedded Next.js Static UI fallback
	s.mux.Handle("/", webui.Handler(s.cfg.WebDir))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"framework":      "CORTEX",
		"version":        "0.1.0",
		"sdk":            "github.com/h-morozumi/Norma v0.4.5",
		"locale":         s.i18n.Get(),
		"default_model":  s.cfg.DefaultModel,
		"server_time_ut": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleGetI18n(w http.ResponseWriter, r *http.Request) {
	effective := s.i18n.ResolveLocale(
		r.URL.Query().Get("locale"),
		r.Header.Get("X-Cortex-Locale"),
		"",
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"locale":          s.i18n.Get(),
		"effective":       effective,
		"supported":       []string{i18n.LocaleEN, i18n.LocaleJA},
		"directive":       s.i18n.AgentLanguageDirective(effective),
		"system_reminder": s.i18n.SystemReminder(effective),
	})
}

func (s *Server) handlePutI18n(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Locale string `json:"locale"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	norm, ok := i18n.ParseStrict(body.Locale)
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported locale (supported: en, ja)")
		return
	}
	if s.store != nil {
		if err := s.store.SetSetting(r.Context(), "locale", norm); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	updated := s.i18n.Set(norm)
	writeJSON(w, http.StatusOK, map[string]any{
		"locale":          updated,
		"directive":       s.i18n.AgentLanguageDirective(updated),
		"system_reminder": s.i18n.SystemReminder(updated),
	})
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	var configs []store.ProviderConfig
	var copilot any
	if s.runtime != nil && s.runtime.Hub() != nil {
		configs, _ = s.runtime.Hub().ListProviders(r.Context())
		copilot = s.runtime.Hub().GetCopilotStatus(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"presets": agent.DefaultPresets(),
		"configs": configs,
		"copilot": copilot,
	})
}

func (s *Server) handlePutProvider(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil || s.runtime.Hub() == nil {
		writeError(w, http.StatusServiceUnavailable, "LLM hub unavailable")
		return
	}
	id := r.PathValue("id")
	var body struct {
		store.ProviderConfig
		ClearAPIKey bool `json:"clear_api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.ProviderConfig.ID = id
	if body.ProviderConfig.Name == "" {
		body.ProviderConfig.Name = id
	}
	saved, err := s.runtime.Hub().SaveProvider(r.Context(), body.ProviderConfig, body.ClearAPIKey)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrInvalidName) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil || s.runtime.Hub() == nil {
		writeError(w, http.StatusServiceUnavailable, "LLM hub unavailable")
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		writeError(w, http.StatusBadRequest, "provider id is required")
		return
	}
	res, err := s.runtime.Hub().TestProvider(r.Context(), body.ID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, store.ErrInvalidName) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCopilotStatus(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil || s.runtime.Hub() == nil {
		writeError(w, http.StatusServiceUnavailable, "LLM hub unavailable")
		return
	}
	writeJSON(w, http.StatusOK, s.runtime.Hub().GetCopilotStatus(r.Context()))
}

func (s *Server) handleCopilotDeviceCode(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil || s.runtime.Hub() == nil {
		writeError(w, http.StatusServiceUnavailable, "LLM hub unavailable")
		return
	}
	dc, err := s.runtime.Hub().StartCopilotDeviceFlow(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dc)
}

func (s *Server) handleCopilotPoll(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil || s.runtime.Hub() == nil {
		writeError(w, http.StatusServiceUnavailable, "LLM hub unavailable")
		return
	}
	var body struct {
		DeviceCode string `json:"device_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DeviceCode == "" {
		writeError(w, http.StatusBadRequest, "device_code is required")
		return
	}
	res, err := s.runtime.Hub().PollCopilotDeviceFlow(r.Context(), body.DeviceCode)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCopilotModels(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil || s.runtime.Hub() == nil {
		writeError(w, http.StatusServiceUnavailable, "LLM hub unavailable")
		return
	}
	models, source, err := s.runtime.Hub().ListCopilotModels(r.Context())
	resp := map[string]any{
		"models": models,
		"source": source,
	}
	if err != nil {
		resp["warning"] = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCopilotDisconnect(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil || s.runtime.Hub() == nil {
		writeError(w, http.StatusServiceUnavailable, "LLM hub unavailable")
		return
	}
	if err := s.runtime.Hub().DisconnectCopilot(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.runtime.Hub().GetCopilotStatus(r.Context()))
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	summary, err := s.store.GetUsageSummary(r.Context(), 25)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleSkills(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"skills": s.runtime.SkillsList(),
	})
}

func (s *Server) handleSaveSkill(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime unavailable")
		return
	}
	var body struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		Instructions string `json:"instructions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Name == "" || body.Description == "" || body.Instructions == "" {
		writeError(w, http.StatusBadRequest, "name, description, and instructions are required")
		return
	}
	if err := s.runtime.SaveSkill(body.Name, body.Description, body.Instructions); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"saved":  body.Name,
		"skills": s.runtime.SkillsList(),
	})
}

func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime unavailable")
		return
	}
	name := r.PathValue("name")
	if err := s.runtime.DeleteSkill(name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted": name,
		"skills":  s.runtime.SkillsList(),
	})
}

func maskMCPEnv(m store.MCPServerConfig) store.MCPServerConfig {
	if len(m.Env) == 0 {
		return m
	}
	masked := make(map[string]string, len(m.Env))
	for k, v := range m.Env {
		if v != "" {
			masked[k] = "••••••••"
		} else {
			masked[k] = ""
		}
	}
	m.Env = masked
	return m
}

func (s *Server) handleListMCP(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"servers": []store.MCPServerConfig{}})
		return
	}
	servers, err := s.store.ListMCPServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range servers {
		servers[i] = maskMCPEnv(servers[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": servers})
}

func (s *Server) handleSaveMCP(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	var in store.MCPServerConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if pathID := r.PathValue("id"); pathID != "" {
		in.ID = pathID
	}
	if in.ID == "" {
		in.ID = fmt.Sprintf("mcp-%d", time.Now().UTC().UnixNano())
	}
	if in.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	saved, err := s.store.SaveMCPServer(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, maskMCPEnv(saved))
}

func (s *Server) handleDeleteMCP(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	id := r.PathValue("id")
	if err := s.store.DeleteMCPServer(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

func (s *Server) handleListCustomTools(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"tools": []store.CustomToolConfig{}})
		return
	}
	tools, err := s.store.ListCustomTools(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

func (s *Server) handleSaveCustomTool(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	var in store.CustomToolConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if pathID := r.PathValue("id"); pathID != "" {
		in.ID = pathID
	}
	if in.ID == "" {
		in.ID = in.Name
	}
	if in.Name == "" || in.Description == "" || in.Target == "" {
		writeError(w, http.StatusBadRequest, "name, description, and target are required")
		return
	}
	saved, err := s.store.SaveCustomTool(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteCustomTool(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	id := r.PathValue("id")
	if err := s.store.DeleteCustomTool(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

func (s *Server) handleGetWebSearch(w http.ResponseWriter, r *http.Request) {
	backend := "ddgs"
	hasBrave := false
	hasTavily := false
	if s.store != nil {
		if b, err := s.store.GetSetting(r.Context(), "web_search_backend"); err == nil && b != "" {
			backend = b
		}
		if k, err := s.store.GetSetting(r.Context(), "brave_api_key"); err == nil && k != "" {
			hasBrave = true
		}
		if k, err := s.store.GetSetting(r.Context(), "tavily_api_key"); err == nil && k != "" {
			hasTavily = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"backend":          backend,
		"supported":        []string{"ddgs", "brave-free", "tavily"},
		"has_brave_key":    hasBrave,
		"has_tavily_key":   hasTavily,
		"default_keyless":  "ddgs",
	})
}

func (s *Server) handlePutWebSearch(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	var body struct {
		Backend      string `json:"backend"`
		BraveAPIKey  string `json:"brave_api_key"`
		TavilyAPIKey string `json:"tavily_api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	switch body.Backend {
	case "", "ddgs":
		body.Backend = "ddgs"
	case "brave-free", "tavily":
	default:
		writeError(w, http.StatusBadRequest, "unsupported web search backend (use ddgs, brave-free, or tavily)")
		return
	}
	_ = s.store.SetSetting(r.Context(), "web_search_backend", body.Backend)
	if body.BraveAPIKey != "" {
		_ = s.store.SetSetting(r.Context(), "brave_api_key", body.BraveAPIKey)
	}
	if body.TavilyAPIKey != "" {
		_ = s.store.SetSetting(r.Context(), "tavily_api_key", body.TavilyAPIKey)
	}
	s.handleGetWebSearch(w, r)
}

func (s *Server) handleListCollections(w http.ResponseWriter, r *http.Request) {
	names, err := s.store.ListCollections(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"collections": names})
}

func (s *Server) handleListItems(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	items, err := s.store.ListItems(r.Context(), name)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrInvalidName) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"collection": name,
		"items":      items,
	})
}

func (s *Server) handleCreateItem(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var envelope struct {
		ID   string          `json:"id"`
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(raw, &envelope)
	id := envelope.ID
	if id == "" {
		id = fmt.Sprintf("item-%d", time.Now().UTC().UnixNano())
	}
	payload := envelope.Data
	if len(payload) == 0 {
		payload = json.RawMessage(raw)
	}
	item, err := s.store.PutItem(r.Context(), name, id, payload)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrInvalidName) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleGetItem(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	item, err := s.store.GetItem(r.Context(), name, id)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, store.ErrInvalidName):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handlePutItem(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(raw, &envelope)
	payload := envelope.Data
	if len(payload) == 0 {
		payload = json.RawMessage(raw)
	}
	item, err := s.store.PutItem(r.Context(), name, id, payload)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrInvalidName) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	if err := s.store.DeleteItem(r.Context(), name, id); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, store.ErrInvalidName):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

func (s *Server) handleListTimers(w http.ResponseWriter, r *http.Request) {
	timers, err := s.store.ListTimers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	runs, _ := s.store.ListTimerRuns(r.Context(), "", 30)
	writeJSON(w, http.StatusOK, map[string]any{
		"timers": timers,
		"runs":   runs,
	})
}

func (s *Server) handleCreateTimer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Schedule string `json:"schedule"`
		RunAt    string `json:"run_at"`
		Prompt   string `json:"prompt"`
		AgentID  string `json:"agent_id"`
		Enabled  *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.ID == "" {
		body.ID = fmt.Sprintf("timer-%d", time.Now().UTC().UnixNano())
	}
	if body.Name == "" || body.Schedule == "" || body.Prompt == "" {
		writeError(w, http.StatusBadRequest, "name, schedule, and prompt are required")
		return
	}
	nextRun, err := scheduler.ValidateSchedule(body.Schedule, body.RunAt, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	nextRunStr := ""
	if enabled {
		nextRunStr = nextRun.Format(time.RFC3339)
	}
	saved, err := s.store.SaveTimer(r.Context(), store.TimerEntry{
		ID:        body.ID,
		Name:      body.Name,
		Schedule:  body.Schedule,
		RunAt:     body.RunAt,
		Prompt:    body.Prompt,
		AgentID:   body.AgentID,
		Enabled:   enabled,
		NextRunAt: nextRunStr,
		CreatedBy: "user",
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleUpdateTimer(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	id := r.PathValue("id")
	existing, err := s.store.GetTimer(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var patch struct {
		Name     *string `json:"name"`
		Schedule *string `json:"schedule"`
		RunAt    *string `json:"run_at"`
		Prompt   *string `json:"prompt"`
		AgentID  *string `json:"agent_id"`
		Enabled  *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if patch.Name != nil && *patch.Name != "" {
		existing.Name = *patch.Name
	}
	if patch.Schedule != nil && *patch.Schedule != "" {
		existing.Schedule = *patch.Schedule
	}
	if patch.RunAt != nil {
		existing.RunAt = *patch.RunAt
	}
	if patch.Prompt != nil && *patch.Prompt != "" {
		existing.Prompt = *patch.Prompt
	}
	if patch.AgentID != nil && *patch.AgentID != "" {
		existing.AgentID = *patch.AgentID
	}
	if patch.Enabled != nil {
		existing.Enabled = *patch.Enabled
	}

	nextRun, err := scheduler.ValidateSchedule(existing.Schedule, existing.RunAt, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if existing.Enabled {
		existing.NextRunAt = nextRun.Format(time.RFC3339)
	} else {
		existing.NextRunAt = ""
	}

	saved, err := s.store.SaveTimer(r.Context(), existing)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteTimer(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	id := r.PathValue("id")
	if err := s.store.DeleteTimer(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

func (s *Server) handleRunTimerNow(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil || s.runtime.Scheduler() == nil {
		writeError(w, http.StatusServiceUnavailable, "scheduler unavailable")
		return
	}
	id := r.PathValue("id")
	runRec, err := s.runtime.Scheduler().TriggerNow(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runRec)
}

func (s *Server) handleListTimerRuns(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	timerID := r.URL.Query().Get("timer_id")
	runs, err := s.store.ListTimerRuns(r.Context(), timerID, 50)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"agents": agent.DefaultAgents()})
		return
	}
	agents, err := s.store.ListAgents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

func (s *Server) handleSaveAgent(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	var in store.AgentProfile
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if pathID := r.PathValue("id"); pathID != "" {
		in.ID = pathID
	}
	if in.ID == "" {
		in.ID = fmt.Sprintf("agent-%d", time.Now().UTC().UnixNano())
	}
	if in.Name == "" || in.SystemPrompt == "" {
		writeError(w, http.StatusBadRequest, "name and system_prompt are required")
		return
	}
	saved, err := s.store.SaveAgent(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	id := r.PathValue("id")
	if err := s.store.DeleteAgent(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

func (s *Server) handleListChatSessions(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"sessions": []store.ChatSession{}})
		return
	}
	sessions, err := s.store.ListChatSessions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) handleCreateChatSession(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	var in store.ChatSession
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.ID == "" {
		in.ID = fmt.Sprintf("chat-%d", time.Now().UTC().UnixNano())
	}
	if in.Title == "" {
		in.Title = "New Chat"
	}
	if in.AgentID == "" {
		in.AgentID = "default"
	}
	saved, err := s.store.SaveChatSession(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleGetChatSession(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	id := r.PathValue("id")
	cs, err := s.store.GetChatSession(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	msgs, err := s.store.ListChatMessages(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session":  cs,
		"messages": msgs,
	})
}

func (s *Server) handleUpdateChatSession(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	id := r.PathValue("id")
	existing, err := s.store.GetChatSession(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var patch struct {
		Title      *string `json:"title"`
		AgentID    *string `json:"agent_id"`
		ProviderID *string `json:"provider_id"`
		Model      *string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if patch.Title != nil && *patch.Title != "" {
		existing.Title = *patch.Title
	}
	if patch.AgentID != nil && *patch.AgentID != "" {
		existing.AgentID = *patch.AgentID
	}
	if patch.ProviderID != nil {
		existing.ProviderID = *patch.ProviderID
	}
	if patch.Model != nil {
		existing.Model = *patch.Model
	}
	saved, err := s.store.SaveChatSession(r.Context(), existing)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteChatSession(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	id := r.PathValue("id")
	if err := s.store.DeleteChatSession(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.runtime != nil {
		_ = s.runtime.DeleteSessionTranscript(id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

func (s *Server) acquireSessionTurn(sessionID string) bool {
	if sessionID == "" {
		return true
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if s.activeSessions[sessionID] {
		return false
	}
	s.activeSessions[sessionID] = true
	return true
}

func (s *Server) releaseSessionTurn(sessionID string) {
	if sessionID == "" {
		return
	}
	s.sessionMu.Lock()
	delete(s.activeSessions, sessionID)
	s.sessionMu.Unlock()
}

func (s *Server) handleSessionChat(w http.ResponseWriter, r *http.Request) {
	s.streamChatTurn(w, r, r.PathValue("id"))
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	s.streamChatTurn(w, r, "")
}

func (s *Server) streamChatTurn(w http.ResponseWriter, r *http.Request, pathSessionID string) {
	var req struct {
		SessionID   string                 `json:"session_id"`
		AgentID     *string                `json:"agent_id"`
		ProviderID  *string                `json:"provider_id"`
		Model       *string                `json:"model"`
		Prompt      string                 `json:"prompt"`
		Locale      string                 `json:"locale"`
		Attachments []store.ChatAttachment `json:"attachments"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&req); err != nil || (req.Prompt == "" && len(req.Attachments) == 0) {
		writeError(w, http.StatusBadRequest, "prompt or attachment is required")
		return
	}
	if pathSessionID != "" {
		req.SessionID = pathSessionID
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("chat-%d", time.Now().UTC().UnixNano())
	}
	if err := store.ValidateIdent(sessionID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if !s.acquireSessionTurn(sessionID) {
		writeError(w, http.StatusConflict, "session already has an active turn in progress")
		return
	}
	defer s.releaseSessionTurn(sessionID)

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	effectiveLocale := s.i18n.ResolveLocale(
		req.Locale,
		r.Header.Get("X-Cortex-Locale"),
		r.Header.Get("Accept-Language"),
	)

	var (
		agentID    string
		providerID string
		model      string
	)
	if req.AgentID != nil {
		agentID = *req.AgentID
	}
	if req.ProviderID != nil {
		providerID = *req.ProviderID
	}
	if req.Model != nil {
		model = *req.Model
	}

	if s.store != nil {
		cs, err := s.store.GetChatSession(r.Context(), sessionID)
		if err != nil {
			title := req.Prompt
			if len([]rune(title)) > 36 {
				title = string([]rune(title)[:36]) + "…"
			}
			if title == "" {
				title = "New Chat"
			}
			if agentID == "" {
				agentID = "default"
			}
			cs, err = s.store.SaveChatSession(r.Context(), store.ChatSession{
				ID:         sessionID,
				Title:      title,
				AgentID:    agentID,
				ProviderID: providerID,
				Model:      model,
			})
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		} else {
			dirty := false
			if (cs.Title == "New Chat" || cs.Title == "新規チャット") && req.Prompt != "" {
				title := req.Prompt
				if len([]rune(title)) > 36 {
					title = string([]rune(title)[:36]) + "…"
				}
				cs.Title = title
				dirty = true
			}
			if req.AgentID != nil && *req.AgentID != "" && *req.AgentID != cs.AgentID {
				cs.AgentID = *req.AgentID
				dirty = true
			}
			if req.ProviderID != nil && *req.ProviderID != cs.ProviderID {
				cs.ProviderID = *req.ProviderID
				dirty = true
			}
			if req.Model != nil && *req.Model != cs.Model {
				cs.Model = *req.Model
				dirty = true
			}
			if dirty {
				_, _ = s.store.SaveChatSession(r.Context(), cs)
			}
			agentID = cs.AgentID
			providerID = cs.ProviderID
			model = cs.Model
		}

		if _, err := s.store.AppendChatMessage(r.Context(), store.ChatMessage{
			SessionID:   sessionID,
			Role:        "user",
			Content:     req.Prompt,
			Attachments: req.Attachments,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if sessionID != "" {
		sendSSE(w, flusher, "session", map[string]any{
			"session_id": sessionID,
			"agent_id":   agentID,
		})
	}

	sess, cleanup, err := s.runtime.NewSessionWithCleanup(r.Context(), agent.SessionOptions{
		SessionID:  sessionID,
		AgentID:    agentID,
		ProviderID: providerID,
		Model:      model,
		Locale:     effectiveLocale,
	})
	if err != nil {
		sendSSE(w, flusher, "error", map[string]any{"error": err.Error()})
		return
	}
	defer cleanup()

	fullPrompt := agent.FormatPromptWithAttachments(req.Prompt, req.Attachments)

	var (
		assembledText     string
		assembledThinking string
		toolCalls         []store.ChatToolCall
		inputTokens       int
		outputTokens      int
	)

	for ev, err := range sess.Prompt(r.Context(), fullPrompt) {
		if err != nil {
			sendSSE(w, flusher, "error", map[string]any{"error": err.Error()})
			break
		}
		payload := map[string]any{
			"kind": string(ev.Kind),
			"text": ev.Text,
		}
		switch ev.Kind {
		case harness.KindText:
			assembledText += ev.Text
		case harness.KindThinking:
			assembledThinking += ev.Text
		case harness.KindToolUse:
			if ev.ToolUse != nil {
				inStr := string(ev.ToolUse.Input)
				payload["tool_id"] = ev.ToolUse.ID
				payload["tool_name"] = ev.ToolUse.Name
				payload["input"] = inStr
				toolCalls = append(toolCalls, store.ChatToolCall{
					ID:    ev.ToolUse.ID,
					Name:  ev.ToolUse.Name,
					Input: inStr,
				})
			}
		case harness.KindToolResult:
			if ev.ToolResult != nil {
				var outText string
				for _, b := range ev.ToolResult.Content {
					outText += b.Text
				}
				payload["tool_id"] = ev.ToolResult.ToolUseID
				payload["output"] = outText
				payload["is_error"] = ev.ToolResult.IsError
				for i := range toolCalls {
					if toolCalls[i].ID == ev.ToolResult.ToolUseID {
						toolCalls[i].Output = outText
						toolCalls[i].IsError = ev.ToolResult.IsError
						break
					}
				}
			}
		case harness.KindUsage:
			if ev.Usage != nil {
				inputTokens = ev.Usage.InputTokens
				outputTokens = ev.Usage.OutputTokens
				payload["input_tokens"] = inputTokens
				payload["output_tokens"] = outputTokens
			}
		case harness.KindResult:
			if ev.Terminal != nil {
				payload["reason"] = string(ev.Terminal.Reason)
				payload["final_text"] = ev.Terminal.Text
				if assembledText == "" && ev.Terminal.Text != "" {
					assembledText = ev.Terminal.Text
				}
				if ev.Terminal.Usage.InputTokens > 0 || ev.Terminal.Usage.OutputTokens > 0 {
					inputTokens = ev.Terminal.Usage.InputTokens
					outputTokens = ev.Terminal.Usage.OutputTokens
				}
			}
		}
		sendSSE(w, flusher, string(ev.Kind), payload)
	}

	if s.store != nil && sessionID != "" && (assembledText != "" || assembledThinking != "" || len(toolCalls) > 0) {
		_, _ = s.store.AppendChatMessage(context.Background(), store.ChatMessage{
			SessionID:    sessionID,
			Role:         "assistant",
			Content:      assembledText,
			Thinking:     assembledThinking,
			ToolCalls:    toolCalls,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		})
	}
}

func sendSSE(w http.ResponseWriter, flusher http.Flusher, event string, data any) {
	b, _ := json.Marshal(data)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(b))
	flusher.Flush()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}
