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
}
