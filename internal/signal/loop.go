// Package signal is the deterministic loop: it ticks the world, pushes
// metrics to the console, runs the tenant's monitoring workflows on their
// schedules, hands hits to a responder or to Warden, and re-reads the signal
// after a fix to resolve findings. No model is involved anywhere in this
// package; a model runs only when a workflow fires and the trigger wakes
// Warden.
package signal

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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

// Loop runs the tenant's workflows and the verification loop. The rules are
// the checks metric workflows refer to by name; the workflows themselves live
// in the tenant state, so the operator and Warden can add, pause and remove
// them.
type Loop struct {
	st     *store.Store
	w      *world.World
	orch   *agent.Orchestrator
	log    *slog.Logger
	tick   time.Duration
	rules  []Rule
	hold   map[string]int
	clear  map[string]int
	tries  map[string]int
	cursor map[string]map[string]int  // workflow/cell -> log path -> lines read
	seen   map[string]map[string]bool // workflow/cell -> error signatures already fired on
	last   map[string]time.Time
}

// New builds the loop with the default rules.
func New(st *store.Store, w *world.World, orch *agent.Orchestrator, tick time.Duration, log *slog.Logger) *Loop {
	return &Loop{st: st, w: w, orch: orch, log: log, tick: tick, rules: defaultRules(), hold: map[string]int{}, clear: map[string]int{}, tries: map[string]int{}, cursor: map[string]map[string]int{}, seen: map[string]map[string]bool{}, last: map[string]time.Time{}}
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
	l.runWorkflows()
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
		check := l.checkFor(f.ResponderID, f.Resource)
		if check == nil {
			l.orch.Resolve(f.ID, "The remediator reported success and no watch applies to re-check.")
			continue
		}
		if check() {
			l.clear[f.ID] = 0
			l.tries[f.ID]++
			if l.tries[f.ID] > 60 {
				l.orch.Escalate(f.ID, "The signal did not clear within the verification window after the action. Escalated to on-call.")
			}
			continue
		}
		l.clear[f.ID]++
		if l.clear[f.ID] >= 3 {
			l.orch.Resolve(f.ID, l.clearedText(f.Resource, f.ResponderID))
			delete(l.clear, f.ID)
			delete(l.tries, f.ID)
			for k := range l.hold {
				if strings.HasSuffix(k, "/"+f.Resource) {
					l.hold[k] = 0
				}
			}
		}
	}
}

// runWorkflows runs every enabled workflow that is due. A metric workflow
// evaluates its named rule on the rule's resource when that cell is onboarded.
// A log workflow digests what is new in the logs of every matching cell and
// fires on an error signature it has not fired on before.
func (l *Loop) runWorkflows() {
	now := time.Now()
	var wfs []domain.Workflow
	var cells []domain.Cell
	l.st.View(func(s *domain.State) {
		for _, w := range s.Workflows {
			if w.Enabled {
				wfs = append(wfs, *w)
			}
		}
		for _, c := range s.Cells {
			cells = append(cells, *c)
		}
	})
	onboarded := map[string]bool{}
	for _, c := range cells {
		onboarded[c.ID] = true
	}
	for _, wf := range wfs {
		if t, ok := l.last[wf.ID]; ok && now.Sub(t) < time.Duration(wf.Every)*time.Second {
			continue
		}
		l.last[wf.ID] = now
		result := ""
		fired := false
		switch {
		case wf.Metric != "":
			// generic metric watch: every matching onboarded cell, its own hold
			n := 0
			for _, c := range cells {
				if !wf.Matches(&c) {
					continue
				}
				n++
				key := wf.ID + "/" + c.ID
				v, ok := l.w.Metric(c.ID, wf.Metric)
				if !ok {
					result = fmt.Sprintf("%s has no metric %s", c.ID, wf.Metric)
					continue
				}
				if compare(wf.Op, v, wf.Threshold) {
					l.hold[key]++
				} else {
					l.hold[key] = 0
				}
				result = fmt.Sprintf("%s: %s=%.1f, condition %s (%d of %d)", c.ID, wf.Metric, v, map[bool]string{true: "holds", false: "clear"}[l.hold[key] > 0], l.hold[key], wf.Hold)
				if l.hold[key] == wf.Hold {
					fired = true
					l.fire(wf, c.ID, fmt.Sprintf("%s %s %.0f on %s", wf.Metric, wf.Op, wf.Threshold, c.ID), "warning", fmt.Sprintf("%s on %s is %.1f, %s %.0f on %d consecutive reads.", wf.Metric, c.ID, v, wf.Op, wf.Threshold, wf.Hold))
				}
			}
			if n == 0 {
				result = "no cell matches the selector"
			}
		case wf.Collect == "metric" || wf.Collect == "provider-state":
			r := l.ruleByID(wf.Rule)
			if r == nil {
				result = "no rule " + wf.Rule
				break
			}
			key := wf.ID
			if !onboarded[r.Resource] {
				l.hold[key] = 0
				result = r.Resource + " is not a cell"
				break
			}
			if r.Check(l.w) {
				l.hold[key]++
			} else {
				l.hold[key] = 0
			}
			result = fmt.Sprintf("read %s on %s: condition %s (%d of %d)", wf.Source, r.Resource, map[bool]string{true: "holds", false: "clear"}[l.hold[key] > 0], l.hold[key], wf.Hold)
			if l.hold[key] == wf.Hold {
				fired = true
				l.fire(wf, r.Resource, r.Title, r.Severity, r.Text(l.w))
			}
		case wf.Collect == "log-digest":
			n := 0
			for _, c := range cells {
				if !wf.Matches(&c) {
					continue
				}
				n++
				key := wf.ID + "/" + c.ID
				_, seenBefore := l.cursor[key]
				d, err := l.w.LogDigest(c.ID, wf.Source, l.cursor[key])
				if err != nil {
					result = err.Error()
					continue
				}
				l.cursor[key] = d.Cursor
				if !seenBefore {
					// first run is the baseline: only what is written from now on counts
					result = fmt.Sprintf("%s: baseline taken, %d existing lines ignored", c.ID, d.NewLines)
					continue
				}
				if l.seen[key] == nil {
					l.seen[key] = map[string]bool{}
				}
				var fresh []string
				for _, sig := range d.Signatures {
					if !l.seen[key][sig] {
						fresh = append(fresh, sig)
						l.seen[key][sig] = true
					}
				}
				result = fmt.Sprintf("%s: %d new lines, %d errors", c.ID, d.NewLines, d.Errors)
				if len(fresh) > 0 {
					fired = true
					where := wf.Source
					if where == "*" {
						where = "the logs"
					}
					text := fmt.Sprintf("%d new lines in %s on %s, %d of them errors. New signatures: %s.", d.NewLines, where, c.ID, d.Errors, strings.Join(fresh, " · "))
					l.fire(wf, c.ID, fmt.Sprintf("New errors in %s on %s", where, c.ID), "warning", text)
				}
			}
			if n == 0 {
				result = "no cell matches the selector"
			}
		}
		l.st.Update(func(s *domain.State) []store.Event {
			w := s.Workflow(wf.ID)
			if w == nil {
				return nil
			}
			t := now
			w.LastRun = &t
			w.LastResult = result
			if fired {
				w.Fired++
			}
			return []store.Event{{Type: "workflow", Data: w}}
		})
	}
}

// fire hands a watch hit to the responder it names, or to Warden. A
// standing order travels with the signal so the run may execute the
// pre-approved action without asking.
func (l *Loop) fire(wf domain.Workflow, resource, title, severity, text string) {
	if strings.HasPrefix(wf.Trigger, "responder:") {
		rid := strings.TrimPrefix(wf.Trigger, "responder:")
		id := l.orch.HandleSignal(agent.Signal{ResponderID: rid, Resource: resource, Title: title, Severity: severity, Text: "Watch " + wf.ID + " (" + wf.Name + "): " + text, Allow: wf.Allow, AllowedBy: wf.ID})
		if id != "" {
			l.orch.AnnounceWatch(wf, resource, rid, id, text)
		}
		return
	}
	l.orch.HandleWorkflow(wf, resource, title, text)
}

func compare(op string, v, limit float64) bool {
	switch op {
	case "<":
		return v < limit
	case "==":
		return v == limit
	default:
		return v > limit
	}
}

// checkFor is the condition to re-read when verifying a fix: the generic
// watch that triggers the responder on the cell, or the named rule.
func (l *Loop) checkFor(responder, resource string) func() bool {
	var wf *domain.Workflow
	var cell *domain.Cell
	l.st.View(func(s *domain.State) {
		cell = s.Cell(resource)
		for _, w := range s.Workflows {
			if w.Metric != "" && w.Trigger == "responder:"+responder && cell != nil && w.Matches(cell) {
				cp := *w
				wf = &cp
			}
		}
	})
	if wf != nil {
		return func() bool {
			v, ok := l.w.Metric(resource, wf.Metric)
			return ok && compare(wf.Op, v, wf.Threshold)
		}
	}
	if r := l.ruleFor(responder, resource); r != nil {
		return func() bool { return r.Check(l.w) }
	}
	return nil
}

func (l *Loop) ruleByID(id string) *Rule {
	for i := range l.rules {
		if l.rules[i].Responder+"/"+l.rules[i].Resource == id {
			return &l.rules[i]
		}
	}
	return nil
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
