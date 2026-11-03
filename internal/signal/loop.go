// Package signal is the deterministic signal loop: it ticks the world, pushes
// metrics to the console, evaluates the responder rules, hands matches to the
// orchestrator, and re-reads the signal after a fix to resolve findings. No
// model is involved anywhere in this package.
package signal

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/asitha-w/warden/internal/agent"
	"github.com/asitha-w/warden/internal/domain"
	"github.com/asitha-w/warden/internal/store"
	"github.com/asitha-w/warden/internal/world"
)

// Rule is one responder condition on one resource.
type Rule struct {
	Responder string
	Resource  string
	Title     string
	Severity  string
	Hold      int // consecutive ticks the condition must hold
	Check     func(w *world.World) bool
	Text      func(w *world.World) string
}

// Loop runs the signal and verification loops.
type Loop struct {
	st    *store.Store
	w     *world.World
	orch  *agent.Orchestrator
	log   *slog.Logger
	tick  time.Duration
	rules []Rule
	hold  map[string]int
	clear map[string]int
	tries map[string]int
}

// New builds the loop with the default rules.
func New(st *store.Store, w *world.World, orch *agent.Orchestrator, tick time.Duration, log *slog.Logger) *Loop {
	return &Loop{st: st, w: w, orch: orch, log: log, tick: tick, rules: defaultRules(), hold: map[string]int{}, clear: map[string]int{}, tries: map[string]int{}}
}

func above(host, metric string, limit float64) func(*world.World) bool {
	return func(w *world.World) bool {
		v, ok := w.Metric(host, metric)
		return ok && v > limit
	}
}

func defaultRules() []Rule {
	return []Rule{
		{Responder: "disk-pressure-linux", Resource: "web-01", Title: "Disk filling on web-01", Severity: "warning", Hold: 3,
			Check: above("web-01", "disk_used_percent", 90),
			Text: func(w *world.World) string {
				v, _ := w.Metric("web-01", "disk_used_percent")
				return fmt.Sprintf("disk_used_percent on web-01 crossed 90%% and held (now %.0f%%).", v)
			}},
		{Responder: "release-regression", Resource: "aks-shop", Title: "Error rate on shop-api above 5% after rollout", Severity: "warning", Hold: 3,
			Check: above("aks-shop", "error_rate_percent", 5),
			Text: func(w *world.World) string {
				v, _ := w.Metric("aks-shop", "error_rate_percent")
				return fmt.Sprintf("error_rate_percent on aks-shop crossed 5%% and held (now %.1f%%).", v)
			}},
		{Responder: "service-down", Resource: "edge-01", Title: "nginx on edge-01 is not responding", Severity: "critical", Hold: 3,
			Check: func(w *world.World) bool {
				v, ok := w.Metric("edge-01", "http_up")
				return ok && v == 0 && w.HostState("edge-01") == "started"
			},
			Text: func(w *world.World) string {
				return "http_up on edge-01 has been 0 for 3 checks while the host is started."
			}},
		{Responder: "server-stopped", Resource: "edge-01", Title: "edge-01 is stopped without an operator event", Severity: "critical", Hold: 2,
			Check: func(w *world.World) bool { return w.HostState("edge-01") == "stopped" && !w.HasOperatorStop("edge-01") },
			Text: func(w *world.World) string {
				return "UpCloud reports edge-01 stopped and no operator stop request is recorded."
			}},
	}
}

// Run ticks until the context ends.
func (l *Loop) Run(ctx context.Context) {
	t := time.NewTicker(l.tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.step()
		}
	}
}

func (l *Loop) step() {
	metrics := l.w.Tick()
	online := false
	var probes int
	l.st.View(func(s *domain.State) {
		online = s.Probe.Status == domain.ProbeOnline
		probes = len(s.Probe.Hosts)
	})
	l.st.Publish(store.Event{Type: "metrics", Data: map[string]any{"hosts": metrics, "probes": probes, "online": online, "at": time.Now()}})
	if !online {
		return
	}
	// signal: open findings, only for onboarded cells
	cells := map[string]bool{}
	l.st.View(func(s *domain.State) {
		for _, c := range s.Cells {
			cells[c.ID] = true
		}
	})
	for _, r := range l.rules {
		key := r.Responder + "/" + r.Resource
		if !cells[r.Resource] {
			l.hold[key] = 0
			continue
		}
		if r.Check(l.w) {
			l.hold[key]++
		} else {
			l.hold[key] = 0
		}
		if l.hold[key] == r.Hold {
			l.orch.HandleSignal(agent.Signal{ResponderID: r.Responder, Resource: r.Resource, Title: r.Title, Severity: r.Severity, Text: "Signal loop: " + r.Text(l.w)})
		}
	}
	// verification: re-read the signal for findings whose fix ran
	var verifying []domain.Finding
	l.st.View(func(s *domain.State) {
		for _, f := range s.Findings {
			if f.Status == domain.FindingVerifying {
				verifying = append(verifying, *f)
			}
		}
	})
	for _, f := range verifying {
		rule := l.ruleFor(f.ResponderID, f.Resource)
		if rule == nil {
			l.orch.Resolve(f.ID, "The remediator reported success and no responder rule applies to re-check.")
			continue
		}
		if rule.Check(l.w) {
			l.clear[f.ID] = 0
			l.tries[f.ID]++
			if l.tries[f.ID] > 60 {
				l.orch.Escalate(f.ID, "The signal did not clear within the verification window after the action. Escalated to on-call.")
			}
			continue
		}
		l.clear[f.ID]++
		if l.clear[f.ID] >= 3 {
			l.orch.Resolve(f.ID, l.clearedText(f.Resource, rule.Responder))
			delete(l.clear, f.ID)
			delete(l.tries, f.ID)
			l.hold[rule.Responder+"/"+rule.Resource] = 0
		}
	}
}

func (l *Loop) ruleFor(responder, resource string) *Rule {
	for i := range l.rules {
		if l.rules[i].Responder == responder && l.rules[i].Resource == resource {
			return &l.rules[i]
		}
	}
	return nil
}

func (l *Loop) clearedText(resource, responder string) string {
	switch responder {
	case "disk-pressure-linux":
		v, _ := l.w.Metric(resource, "disk_used_percent")
		g, _ := l.w.Metric(resource, "log_growth_mb_per_min")
		return fmt.Sprintf("Disk on %s is at %.0f%% and falling. Access log growth is %.0f MB/min. Resource Health remains Available.", resource, v, g)
	case "release-regression":
		v, _ := l.w.Metric(resource, "error_rate_percent")
		p, _ := l.w.Metric(resource, "p95_latency_ms")
		return fmt.Sprintf("Error rate on %s is %.1f%% and p95 latency %.0f ms, back at baseline after the rollback.", resource, v, p)
	case "service-down":
		m, _ := l.w.Metric(resource, "memory_percent")
		return fmt.Sprintf("nginx on %s answers HTTP again; memory is at %.0f%% and falling.", resource, m)
	case "server-stopped":
		return fmt.Sprintf("UpCloud reports %s started and the probe is reporting again.", resource)
	}
	return "The signal has cleared on three consecutive reads."
}
