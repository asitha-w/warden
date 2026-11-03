// Command warden runs the Warden console and its simulated tenant: the world,
// the signal loop, the orchestrator and the API, in one binary.
package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/asitha-w/warden/internal/agent"
	"github.com/asitha-w/warden/internal/api"
	"github.com/asitha-w/warden/internal/domain"
	sigloop "github.com/asitha-w/warden/internal/signal"
	"github.com/asitha-w/warden/internal/store"
	"github.com/asitha-w/warden/internal/world"
	"github.com/asitha-w/warden/web"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envDuration(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if ms, err := strconv.Atoi(v); err == nil {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return def
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	addr := env("WARDEN_ADDR", ":8080")
	model := env("WARDEN_MODEL", "claude-opus-5-5")
	tick := envDuration("WARDEN_TICK_MS", time.Second)
	pace := envDuration("WARDEN_PACE_MS", 900*time.Millisecond)
	useClaude := os.Getenv("ANTHROPIC_API_KEY") != "" && env("WARDEN_REASONER", "claude") == "claude"

	reasonerName := "scripted"
	if useClaude {
		reasonerName = "claude"
	}
	st := store.New(domain.NewState(model, reasonerName), log)
	w := world.New()
	gate := agent.NewGate()

	var reasoner agent.Reasoner
	if useClaude {
		reasoner = agent.NewClaude(st, model, log)
	} else {
		reasoner = agent.NewScripted(st, pace, model)
	}
	orch := agent.NewOrchestrator(st, w, gate, reasoner, model, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	loopCtx, cancelLoop := context.WithCancel(ctx)
	loop := sigloop.New(st, w, orch, tick, log)
	go loop.Run(loopCtx)

	reset := func() {
		orch.CancelAll()
		cancelLoop()
		*w = *world.New()
		st.Replace(domain.NewState(model, reasonerName))
		loopCtx, cancelLoop = context.WithCancel(ctx)
		loop = sigloop.New(st, w, orch, tick, log)
		go loop.Run(loopCtx)
		log.Info("tenant reset")
	}

	// WARDEN_WEB_DIR serves the console from disk so a browser refresh shows
	// every change; unset, the embedded copy is used.
	var webFS fs.FS
	if dir := os.Getenv("WARDEN_WEB_DIR"); dir != "" {
		webFS = os.DirFS(dir)
		log.Info("dev mode: serving console from disk", "dir", dir)
	} else {
		sub, err := fs.Sub(web.FS, ".")
		if err != nil {
			log.Error("web fs", "err", err)
			os.Exit(1)
		}
		webFS = sub
	}
	srv := &http.Server{Addr: addr, Handler: api.New(st, w, orch, webFS, reset, log).Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Info("warden listening", "addr", addr, "reasoner", reasonerName, "model", model, "tick", tick)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Info("warden stopped")
}
