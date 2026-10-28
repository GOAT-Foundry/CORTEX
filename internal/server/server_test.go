package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GOAT-Foundry/CORTEX/internal/agent"
	"github.com/GOAT-Foundry/CORTEX/internal/config"
	"github.com/GOAT-Foundry/CORTEX/internal/i18n"
	"github.com/GOAT-Foundry/CORTEX/internal/server"
	"github.com/GOAT-Foundry/CORTEX/internal/store"
)

func TestServerEndpoints(t *testing.T) {
	cfg := config.Load()
	cfg.DBPath = ":memory:"
	cfg.SkillsDir = t.TempDir()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	i18nMgr := i18n.NewManager("en")
	rt := agent.NewRuntime(cfg, st, i18nMgr)
	srv := server.New(cfg, st, i18nMgr, rt)

	// 1. Health check
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d", rec.Code)
	}

	// 2. Switch locale to Japanese and verify SQLite persistence
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/i18n", bytes.NewBufferString(`{"locale":"ja"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("i18n put status = %d", rec.Code)
	}
	if i18nMgr.Get() != "ja" {
		t.Fatalf("expected locale ja, got %s", i18nMgr.Get())
	}

	// Verify a newly constructed server restores persisted locale from SQLite
	i18nMgrRestored := i18n.NewManager("en")
	_ = server.New(cfg, st, i18nMgrRestored, rt)
	if i18nMgrRestored.Get() != "ja" {
		t.Fatalf("expected restored server locale ja from SQLite, got %s", i18nMgrRestored.Get())
	}

	// Reject unsupported locale on PUT /api/i18n
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/i18n", bytes.NewBufferString(`{"locale":"fr"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported locale, got %d", rec.Code)
	}

	// 3. Collection Put & Get via REST
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/collections/projects/proj-1", bytes.NewBufferString(`{"name":"CORTEX Demo"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("collection put status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/collections/projects/proj-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("collection get status = %d", rec.Code)
	}
	var got store.CollectionItem
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode item: %v", err)
	}
	if got.ID != "proj-1" {
		t.Fatalf("unexpected id: %s", got.ID)
	}

	// 4. Providers list, update, test, Copilot models, and usage summary
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/providers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("providers get status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/providers/openai", bytes.NewBufferString(`{"name":"OpenAI","format":"openai","base_url":"https://api.openai.com/v1","api_key":"sk-test-1234567890","default_model":"gpt-4o","auth_type":"api_key","enabled":true,"priority":1}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("provider put status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/providers/test", bytes.NewBufferString(`{"id":"openai"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("provider test status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/providers/copilot/models", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("copilot models status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/usage", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("usage get status = %d", rec.Code)
	}

	// 5. Agents & Multi-turn Chat Sessions CRUD + SSE with mock LLM provider
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"Analyzing attachment...\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello from CORTEX chat!\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":40,\"completion_tokens\":12}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer mockLLM.Close()

	// Point primary openai provider to mockLLM
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/providers/openai", bytes.NewBufferString(`{"name":"OpenAI","format":"openai","base_url":"`+mockLLM.URL+`","api_key":"sk-test-1234567890","default_model":"gpt-4o","auth_type":"api_key","enabled":true,"priority":1}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("configure mock LLM status = %d", rec.Code)
	}

	// List seeded agents
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("agents list status = %d", rec.Code)
	}

	// Create chat session with an initial model override
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/chat/sessions", bytes.NewBufferString(`{"id":"sess-test-1","title":"New Chat","agent_id":"coder","model":"gpt-4o-mini"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create chat session status = %d: %s", rec.Code, rec.Body.String())
	}

	// Send message with attachment and an explicitly cleared model override ("model":"") over SSE
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPost,
		"/api/chat/sessions/sess-test-1/messages",
		bytes.NewBufferString(`{"prompt":"Review this file","provider_id":"openai","model":"","attachments":[{"name":"spec.md","media_type":"text/markdown","content":"# Spec","size_bytes":6}]}`),
	))
	if rec.Code != http.StatusOK {
		t.Fatalf("session chat status = %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("Hello from CORTEX chat!")) {
		t.Fatalf("expected streamed assistant text in SSE body, got: %s", rec.Body.String())
	}

	// Fetch session history and verify both user and assistant messages were persisted, and cleared model override was saved
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/chat/sessions/sess-test-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get chat session status = %d", rec.Code)
	}
	var sessionDetail struct {
		Session  store.ChatSession   `json:"session"`
		Messages []store.ChatMessage `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sessionDetail); err != nil {
		t.Fatalf("decode session detail: %v", err)
	}
	if sessionDetail.Session.Model != "" {
		t.Fatalf("expected cleared session model override to persist as empty string, got %q", sessionDetail.Session.Model)
	}
	if len(sessionDetail.Messages) != 2 {
		t.Fatalf("expected 2 persisted messages, got %d", len(sessionDetail.Messages))
	}
	if sessionDetail.Messages[1].Thinking != "Analyzing attachment..." {
		t.Fatalf("expected persisted thinking block, got %q", sessionDetail.Messages[1].Thinking)
	}

	// Delete chat session
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/chat/sessions/sess-test-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete chat session status = %d", rec.Code)
	}

	// 6. Extensibility Suite: Agent CRUD, Skills, MCP, Custom Tools, and WebSearch
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/agents/default", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when deleting default agent, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/skills", bytes.NewBufferString(`{"name":"audit-skill","description":"Audit helper","instructions":"# Audit Steps"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create skill status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/skills/audit-skill", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete skill status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/mcp", bytes.NewBufferString(`{"id":"fs-mcp","name":"Filesystem","command":"npx","args":["-y","@modelcontextprotocol/server-filesystem"],"enabled":false}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("save mcp status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/mcp/fs-mcp", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete mcp status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/tools/custom", bytes.NewBufferString(`{"id":"ping_tool","name":"ping_tool","description":"Ping tool","runner":"bash","target":"echo pong","enabled":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("save custom tool status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/tools/custom/ping_tool", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete custom tool status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/websearch", bytes.NewBufferString(`{"backend":"ddgs"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("put websearch status = %d", rec.Code)
	}

	// 7. Timers & Scheduler CRUD + Manual Run Now + Run History
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/timers", bytes.NewBufferString(`{"id":"timer-test-1","name":"Hourly Check","schedule":"0 * * * *","prompt":"Run status check","agent_id":"default","enabled":true}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create timer status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/timers/timer-test-1", bytes.NewBufferString(`{"schedule":"15m","enabled":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("update timer status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/timers/timer-test-1/run", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("run timer now status = %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("Hello from CORTEX chat!")) {
		t.Fatalf("expected timer run output from mockLLM, got: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/timers/runs?timer_id=timer-test-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list timer runs status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/timers/timer-test-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete timer status = %d", rec.Code)
	}
}
