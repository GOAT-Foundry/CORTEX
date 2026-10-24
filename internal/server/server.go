package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Autumn-27/norma/harness"

	"github.com/GOAT-Foundry/CORTEX/internal/agent"
	"github.com/GOAT-Foundry/CORTEX/internal/config"
	"github.com/GOAT-Foundry/CORTEX/internal/i18n"
	"github.com/GOAT-Foundry/CORTEX/internal/store"
	"github.com/GOAT-Foundry/CORTEX/internal/webui"
)

// Server wires HTTP routes for REST APIs, SSE agent streams, and the embedded Next.js UI.
type Server struct {
	cfg     config.Config
	store   *store.Store
	i18n    *i18n.Manager
	runtime *agent.Runtime
	mux     *http.ServeMux
}

// New constructs a configured CORTEX HTTP server.
func New(cfg config.Config, st *store.Store, i18nMgr *i18n.Manager, rt *agent.Runtime) *Server {
	s := &Server{
		cfg:     cfg,
		store:   st,
		i18n:    i18nMgr,
		runtime: rt,
		mux:     http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// ServeHTTP implements http.Handler with permissive CORS for local Next.js dev.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Cortex-Locale")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/i18n", s.handleGetI18n)
	s.mux.HandleFunc("PUT /api/i18n", s.handlePutI18n)

	s.mux.HandleFunc("GET /api/providers", s.handleProviders)
	s.mux.HandleFunc("GET /api/skills", s.handleSkills)

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

	// Chat SSE endpoint
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

func (s *Server) handleGetI18n(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"locale":    s.i18n.Get(),
		"supported": []string{i18n.LocaleEN, i18n.LocaleJA},
		"directive": s.i18n.AgentLanguageDirective(""),
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
	updated := s.i18n.Set(body.Locale)
	writeJSON(w, http.StatusOK, map[string]any{
		"locale":    updated,
		"directive": s.i18n.AgentLanguageDirective(updated),
	})
}

func (s *Server) handleProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"presets": agent.DefaultPresets(),
	})
}

func (s *Server) handleSkills(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"skills": s.runtime.SkillsList(),
	})
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
	writeJSON(w, http.StatusOK, map[string]any{"timers": timers})
}

func (s *Server) handleCreateTimer(w http.ResponseWriter, r *http.Request) {
	var in store.TimerEntry
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.ID == "" {
		in.ID = fmt.Sprintf("timer-%d", time.Now().UTC().UnixNano())
	}
	if in.Name == "" || in.Schedule == "" || in.Prompt == "" {
		writeError(w, http.StatusBadRequest, "name, schedule, and prompt are required")
		return
	}
	saved, err := s.store.SaveTimer(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
		Locale string `json:"locale"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	stream, cleanup, err := s.runtime.StreamPrompt(r.Context(), req.Prompt, req.Locale)
	if err != nil {
		sendSSE(w, flusher, "error", map[string]any{"error": err.Error()})
		return
	}
	defer cleanup()

	for ev, err := range stream {
		if err != nil {
			sendSSE(w, flusher, "error", map[string]any{"error": err.Error()})
			return
		}
		payload := map[string]any{
			"kind": string(ev.Kind),
			"text": ev.Text,
		}
		if ev.Kind == harness.KindToolUse && ev.ToolUse != nil {
			payload["tool_name"] = ev.ToolUse.Name
			payload["tool_id"] = ev.ToolUse.ID
		}
		if ev.Kind == harness.KindResult && ev.Terminal != nil {
			payload["reason"] = string(ev.Terminal.Reason)
			payload["final_text"] = ev.Terminal.Text
		}
		sendSSE(w, flusher, string(ev.Kind), payload)
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
