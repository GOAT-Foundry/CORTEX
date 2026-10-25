package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GOAT-Foundry/CORTEX/internal/agent"
	"github.com/GOAT-Foundry/CORTEX/internal/config"
	"github.com/GOAT-Foundry/CORTEX/internal/i18n"
	"github.com/GOAT-Foundry/CORTEX/internal/server"
	"github.com/GOAT-Foundry/CORTEX/internal/store"
)

func main() {
	cfg := config.Load()

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("cortex: failed to open SQLite store (%s): %v", cfg.DBPath, err)
	}
	defer st.Close()

	i18nMgr := i18n.NewManager(cfg.DefaultLocale)
	rt := agent.NewRuntime(cfg, st, i18nMgr)
	srv := server.New(cfg, st, i18nMgr, rt)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("CORTEX server listening on http://localhost:%s (locale=%s, db=%s)", cfg.Port, i18nMgr.Get(), cfg.DBPath)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("cortex: server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("CORTEX shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}
