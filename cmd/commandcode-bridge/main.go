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

	"github.com/yoyooyooo/commandcode-bridge/internal/catalog"
	"github.com/yoyooyooo/commandcode-bridge/internal/openai"
	"github.com/yoyooyooo/commandcode-bridge/internal/runtime"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := runtime.Load(ctx)
	if err != nil {
		log.Fatal(err)
	}
	models := catalog.New(catalog.MustStatic())
	if refreshErr := models.Refresh(ctx, http.DefaultClient, cfg.BaseURL, cfg.UpstreamKey); refreshErr != nil {
		log.Printf("catalog refresh degraded: %v", refreshErr)
	}

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           openai.New(cfg, models, nil).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown failed: %v", err)
		}
	}()

	log.Printf("commandcode-bridge listening on %s", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
