// Package agent holds the orchestrator, the specialist reasoners and the
// toolbox they act through. Every tool call, whichever reasoner made it, goes
// through the same policy gate and lands in the same run thread.
package agent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/asitha-w/warden/internal/domain"
	"github.com/asitha-w/warden/internal/store"
	"github.com/asitha-w/warden/internal/world"
)

// ToolDef describes a tool to a model.
type ToolDef struct {
	Name        string
	Description string
	Schema      map[string]any
	Required    []string
}

// ErrDenied is returned when policy forbids an action.
var ErrDenied = errors.New("denied by policy")

// Gate hands approval decisions to the run goroutine waiting on them.
type Gate struct {
	mu    sync.Mutex
	waits map[string]chan decision
}

type decision struct {
	approved bool
	by       string
}

// NewGate returns an empty gate.
func NewGate() *Gate { return &Gate{waits: map[string]chan decision{}} }

func (g *Gate) wait(id string) chan decision {
	g.mu.Lock()
	defer g.mu.Unlock()
	ch := make(chan decision, 1)
	g.waits[id] = ch
	return ch
}

// Decide resolves a pending approval. It reports whether anything was waiting.
func (g *Gate) Decide(id string, approved bool, by string) bool {
	g.mu.Lock()
	ch, ok := g.waits[id]
	delete(g.waits, id)
	g.mu.Unlock()
	if !ok {
		return false
	}
	ch <- decision{approved: approved, by: by}
	return true
}

// Toolbox executes tools for one run against the world and records them.
type Toolbox struct {
	st        *store.Store
	w         *world.World
	gate      *Gate
	runID     string
	agentID   string
	responder *domain.Responder
	calls     int
	maxCalls  int
	Chain     []map[string]string // optional chain the scripted reasoner supplies
	Allowed   []string            // actions a standing order pre-approved for this run
	AllowedBy string              // the watch that holds the standing order
}

// NewToolbox binds a toolbox to a run.
func NewToolbox(st *store.Store, w *world.World, gate *Gate, runID, agentID string, r *domain.Responder, maxCalls int) *Toolbox {
	if maxCalls <= 0 {
		maxCalls = 25
	}
	return &Toolbox{st: st, w: w, gate: gate, runID: runID, agentID: agentID, responder: r, maxCalls: maxCalls}
}

// Defs returns the tool definitions the investigator may use.
func InvestigatorTools() []ToolDef {
	res := map[string]any{"type": "string", "description": "Cell id, for example web-01, aks-shop, edge-01"}
	return []ToolDef{
		{Name: "read_metrics", Description: "Read a metric series for a resource over a window and get a summary with samples. Metrics include cpu_percent, memory_percent, disk_used_percent, requests_per_second, error_rate_percent, p95_latency_ms, http_up, log_growth_mb_per_min, pod_restarts_10m.",
			Schema: map[string]any{"resource": res, "metric": map[string]any{"type": "string"}, "window_minutes": map[string]any{"type": "integer"}}, Required: []string{"resource", "metric"}},
		{Name: "tail_logs", Description: "Tail a log on a resource through the probe. On a VM pass a file path such as /var/log/syslog; on Kubernetes pass the workload name.",
			Schema: map[string]any{"resource": res, "path": map[string]any{"type": "string"}, "lines": map[string]any{"type": "integer"}}, Required: []string{"resource", "path"}},
		{Name: "get_activity", Description: "Provider activity log for a resource: deployments, run commands, operator actions, who changed what and when.",
			Schema: map[string]any{"resource": res}, Required: []string{"resource"}},
		{Name: "get_resource_state", Description: "Provider state, resource health and the services with their versions on a resource.",
			Schema: map[string]any{"resource": res}, Required: []string{"resource"}},
		{Name: "read_file", Description: "Read a small file on a VM through the probe, for example /var/lib/logrotate/status or /etc/nginx/env.",
			Schema: map[string]any{"resource": res, "path": map[string]any{"type": "string"}}, Required: []string{"resource", "path"}},
		{Name: "record_evidence", Description: "Record one claim with its source pointer. Every material claim in your conclusion must have one.",
			Schema: map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"metric", "log", "activity", "config", "resource"}}, "claim": map[string]any{"type": "string"}, "pointer": map[string]any{"type": "string", "description": "Where the claim comes from, for example 'disk_used_percent · web-01' or '/var/log/syslog · edge-01'"}}, Required: []string{"kind", "claim", "pointer"}},
		{Name: "record_hypothesis", Description: "Record a candidate cause and whether the evidence supports or rejects it. Record at least one rejected alternative.",
			Schema: map[string]any{"title": map[string]any{"type": "string"}, "detail": map[string]any{"type": "string"}, "verdict": map[string]any{"type": "string", "enum": []string{"supported", "rejected"}}}, Required: []string{"title", "detail", "verdict"}},
		{Name: "propose_action", Description: "Propose exactly one remediation. Policy decides whether it runs now, waits for a human, or is refused. The result tells you what happened. Actions: rotate_logs (resource), run_command (resource, command), restart_service (resource, service), rollout_undo (resource, service), start_server (resource), notify (resource). Prefer a catalogue action over run_command.",
			Schema: map[string]any{"action": map[string]any{"type": "string", "enum": []string{"rotate_logs", "run_command", "restart_service", "rollout_undo", "start_server", "notify"}}, "resource": res, "command": map[string]any{"type": "string"}, "service": map[string]any{"type": "string"}, "summary": map[string]any{"type": "string", "description": "One line a human can approve"}, "blast_radius": map[string]any{"type": "string", "description": "What changes, what could go wrong, and what does not change"}}, Required: []string{"action", "resource", "summary", "blast_radius"}},
		{Name: "conclude", Description: "Finish the investigation with a two-sentence summary. Call this last, after the action result.",
			Schema: map[string]any{"summary": map[string]any{"type": "string"}, "cause_verified": map[string]any{"type": "boolean", "description": "true only if every material claim has an evidence row"}}, Required: []string{"summary", "cause_verified"}},
	}
}

// Policy returns the effective policy for an action under the run's responder.
func (t *Toolbox) Policy(action string) string {
	if t.responder == nil {
		return domain.PolicyAsk
	}
	p := ""
	for _, tp := range t.responder.Tools {
		if tp.Tool == action {
			p = tp.Policy
		}
	}
	if p == "" {
		return domain.PolicyDeny
	}
	if p == domain.PolicyAsk && t.responder.Mode == "Autonomous" {
		return domain.PolicyAllow
	}
	if p == domain.PolicyAsk && t.preApproved(action) {
		return domain.PolicyAllow
	}
	return p
}

// preApproved reports whether a standing order allows the action for this
// run. A standing order can only lift ask to allow; deny stays deny.
func (t *Toolbox) preApproved(action string) bool {
	for _, a := range t.Allowed {
		if a == action {
			return true
		}
	}
	return false
}

// Calls is how many tool calls the run has made.
func (t *Toolbox) Calls() int { return t.calls }

func str(in map[string]any, k string) string {
	v, _ := in[k].(string)
	return v
}

func num(in map[string]any, k string, def int) int {
	switch v := in[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

// Call executes a tool, records it in the run thread and returns the text result.
// Errors are returned as text so the model can recover; a Go error means the run must stop.
func (t *Toolbox) Call(ctx context.Context, name string, in map[string]any) (string, error) {
	t.calls++
	if t.calls > t.maxCalls {
		return "", fmt.Errorf("tool budget of %d calls exhausted", t.maxCalls)
	}
	msgID := t.addTool(name, in)
	out, err := t.exec(ctx, name, in)
	status := "done"
	if err != nil {
		if errors.Is(err, ErrDenied) {
			status = "denied"
		} else {
			status = "error"
		}
		out = err.Error()
	}
	t.finishTool(msgID, out, status)
	if errors.Is(err, context.Canceled) {
		return "", err
	}
	return out, nil
}

func (t *Toolbox) exec(ctx context.Context, name string, in map[string]any) (string, error) {
	switch name {
	case "read_metrics":
		if err := t.needsProbe(str(in, "resource"), str(in, "metric")); err != nil {
			return "", err
		}
		return t.w.QueryMetrics(str(in, "resource"), str(in, "metric"), num(in, "window_minutes", 60))
	case "tail_logs":
		if err := t.logAllowed(str(in, "resource"), str(in, "path")); err != nil {
			return "", err
		}
		return t.w.TailLogs(str(in, "resource"), str(in, "path"), num(in, "lines", 50))
	case "get_activity":
		return t.w.ActivityLog(str(in, "resource"))
	case "get_resource_state":
		return t.w.ResourceState(str(in, "resource"))
	case "read_file":
		if err := t.needsProbe(str(in, "resource"), ""); err != nil {
			return "", err
		}
		return t.w.ReadFile(str(in, "resource"), str(in, "path"))
	case "record_evidence":
		e := domain.Evidence{Kind: str(in, "kind"), Claim: str(in, "claim"), Pointer: str(in, "pointer")}
		if e.Claim == "" || e.Pointer == "" {
			return "", errors.New("claim and pointer are required")
		}
		t.st.Update(func(s *domain.State) []store.Event {
			r := s.Run(t.runID)
			if r == nil {
				return nil
			}
			r.Evidence = append(r.Evidence, e)
			return []store.Event{store.RunEvent(r)}
		})
		return fmt.Sprintf("Recorded evidence row %d.", t.count(func(r *domain.Run) int { return len(r.Evidence) })), nil
	case "record_hypothesis":
		h := domain.Hypothesis{Title: str(in, "title"), Detail: str(in, "detail"), Verdict: str(in, "verdict")}
		t.st.Update(func(s *domain.State) []store.Event {
			r := s.Run(t.runID)
			if r == nil {
				return nil
			}
			r.Hypotheses = append(r.Hypotheses, h)
			return []store.Event{store.RunEvent(r)}
		})
		return "Recorded hypothesis: " + h.Title + " (" + h.Verdict + ").", nil
	case "propose_action":
		return t.propose(ctx, in)
	case "conclude":
		t.st.Update(func(s *domain.State) []store.Event {
			r := s.Run(t.runID)
			if r == nil {
				return nil
			}
			r.Summary = str(in, "summary")
			r.CauseVerified, _ = in["cause_verified"].(bool)
			if len(t.Chain) > 0 && len(r.Chain) == 0 {
				for _, c := range t.Chain {
					r.Chain = append(r.Chain, domain.ChainStep{Time: c["time"], Text: c["text"], Severity: c["severity"]})
				}
			}
			return []store.Event{store.RunEvent(r)}
		})
		return "Conclusion recorded. The verifier takes over from here.", nil
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

func (t *Toolbox) count(fn func(r *domain.Run) int) int {
	n := 0
	t.st.View(func(s *domain.State) { n = fn(s.Run(t.runID)) })
	return n
}

// propose applies the policy gate and executes through the remediator.
func (t *Toolbox) propose(ctx context.Context, in map[string]any) (string, error) {
	action := str(in, "action")
	policy := t.Policy(action)
	args := map[string]any{"resource": str(in, "resource")}
	if v := str(in, "command"); v != "" {
		args["command"] = v
	}
	if v := str(in, "service"); v != "" {
		args["service"] = v
	}
	cmd := describe(action, args)
	level := "L3"
	if t.responder != nil {
		level = t.responder.Level
	}
	switch policy {
	case domain.PolicyDeny:
		t.st.Update(func(s *domain.State) []store.Event {
			r := s.Run(t.runID)
			if r == nil {
				return nil
			}
			t.appendMsg(r, domain.RolePolicy, "policy", "denied", fmt.Sprintf("%s is denied for responder %s. Nothing was changed. Propose a different action or conclude with a recommendation.", action, t.responderID()))
			return []store.Event{store.RunEvent(r), store.Activity(s, "policy", action+" denied by policy on "+t.responderID()+".", t.runID, false)}
		})
		return "", fmt.Errorf("%w: %s is not allowed for responder %s", ErrDenied, action, t.responderID())
	case domain.PolicyAsk:
		var ap *domain.Approval
		t.st.Update(func(s *domain.State) []store.Event {
			r := s.Run(t.runID)
			if r == nil {
				return nil
			}
			f := s.Finding(r.FindingID)
			ap = &domain.Approval{ID: t.st.NextID("A", 0x20), RunID: r.ID, FindingID: r.FindingID, Task: str(in, "summary"), Level: level, Action: action, Args: args, Command: cmd, BlastRadius: str(in, "blast_radius"), Status: domain.ApprovalPending, RequestedAt: time.Now()}
			s.Approvals = append(s.Approvals, ap)
			r.ApprovalID = ap.ID
			r.Status = domain.RunWaiting
			r.Outcome = "Awaiting approval"
			evs := []store.Event{}
			if f != nil {
				f.Status = domain.FindingAwaitingApproval
				if f.Severity == "warning" {
					f.Severity = "critical"
				}
				evs = append(evs, store.FindingEvent(f))
			}
			t.appendMsg(r, domain.RolePolicy, "policy", "review mode · "+level, fmt.Sprintf("%s is allowlisted for %s but production is in review mode, so a human approves the specific action. Preconditions re-checked by the remediator before it runs.", action, t.responderID()))
			evs = append(evs, store.RunEvent(r), store.ApprovalEvent(ap), store.Activity(s, "task", fmt.Sprintf("Task %s needs approval: %s. Policy: review mode, %s.", ap.ID, ap.Task, level), t.runID, true), store.Toast("Approval requested: "+ap.Task), store.Activity(s, "notify", "Email sent to the service owner and webhook posted to #ops.", t.runID, false))
			return evs
		})
		ch := t.gate.wait(ap.ID)
		var d decision
		select {
		case d = <-ch:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		if !d.approved {
			t.st.Update(func(s *domain.State) []store.Event {
				r := s.Run(t.runID)
				if r == nil {
					return nil
				}
				t.appendMsg(r, domain.RoleUser, "", d.by+" · denied", "Denied. The finding is escalated to on-call with the full thread. No change was made.")
				return []store.Event{store.RunEvent(r)}
			})
			return "The operator denied the action. Do not propose it again; conclude with the recommendation and what the operator should do by hand.", nil
		}
		t.st.Update(func(s *domain.State) []store.Event {
			r := s.Run(t.runID)
			if r == nil {
				return nil
			}
			t.appendMsg(r, domain.RoleUser, "", d.by+" · approved", "Approved: "+str(in, "summary"))
			return []store.Event{store.RunEvent(r)}
		})
		return t.remediate(action, args, cmd, "approved by "+d.by)
	default:
		why := "allowlisted, " + level
		t.st.Update(func(s *domain.State) []store.Event {
			r := s.Run(t.runID)
			if r == nil {
				return nil
			}
			if t.preApproved(action) && (t.responder == nil || t.responder.Mode != "Autonomous") {
				why = "standing order " + t.AllowedBy
				t.appendMsg(r, domain.RolePolicy, "policy", "standing order · "+t.AllowedBy, fmt.Sprintf("%s was pre-approved by the operator in watch %s for exactly this case. Preconditions re-checked by the remediator. Executing now and reporting.", action, t.AllowedBy))
				return []store.Event{store.RunEvent(r), store.Activity(s, "policy", fmt.Sprintf("%s pre-approved by standing order %s. Executing without asking.", action, t.AllowedBy), t.runID, false)}
			}
			t.appendMsg(r, domain.RolePolicy, "policy", "autonomous · "+level, fmt.Sprintf("%s is allowlisted for %s in autonomous mode. Preconditions passed. Executing now and reporting.", action, t.responderID()))
			return []store.Event{store.RunEvent(r), store.Activity(s, "policy", fmt.Sprintf("%s allowlisted on %s (%s). Executing without approval.", action, t.responderID(), level), t.runID, false)}
		})
		return t.remediate(action, args, cmd, why)
	}
}

// remediate is the deterministic remediator: it runs exactly the approved action.
func (t *Toolbox) remediate(action string, args map[string]any, cmd, why string) (string, error) {
	// the remediator needs a write identity whose scope covers the resource
	var writeErr error
	var conn string
	t.st.View(func(s *domain.State) {
		res := s.Cell(str(args, "resource"))
		if res == nil {
			writeErr = fmt.Errorf("%q is not an onboarded cell", str(args, "resource"))
			return
		}
		if action == "notify" {
			return
		}
		c := s.WriteConnection(res)
		if c == nil {
			writeErr = fmt.Errorf("no %s connection has a write identity covering %s", res.Provider, res.ID)
			return
		}
		conn = c.Name
		inGuest := action == "run_command" || action == "restart_service" || action == "rotate_logs"
		inCluster := action == "rollout_undo" || action == "delete_pod"
		switch {
		case inGuest && !res.HasHands():
			writeErr = fmt.Errorf("no hands on %s: it is at %s. Run the Warden grant to raise it to Level 1", res.ID, res.LevelName())
		case inCluster && res.Level < domain.LevelAPI:
			writeErr = fmt.Errorf("%s is at %s; %s needs API access (Level 1)", res.ID, res.LevelName(), action)
		}
	})
	if writeErr != nil {
		t.st.Update(func(s *domain.State) []store.Event {
			r := s.Run(t.runID)
			if r == nil {
				return nil
			}
			r.Status = domain.RunDone
			r.Outcome = "Blocked: no write identity"
			t.appendMsg(r, domain.RoleExecutor, "remediator", "write identity", cmd+"\nstatus: Not run\n"+writeErr.Error()+". Add a write scope to the connection, or run this by hand.")
			evs := []store.Event{store.RunEvent(r)}
			if f := s.Finding(r.FindingID); f != nil {
				f.Status = domain.FindingEscalated
				evs = append(evs, store.FindingEvent(f))
				evs = append(evs, endScenario(s, f)...)
			}
			return append(evs, store.Activity(s, "action", "Remediator blocked: "+writeErr.Error(), t.runID, false))
		})
		return "The remediator could not run the action: " + writeErr.Error() + ". Conclude with what the operator should do by hand.", nil
	}
	if conn != "" {
		why += " via " + conn
	}
	t.st.Update(func(s *domain.State) []store.Event {
		r := s.Run(t.runID)
		if r == nil {
			return nil
		}
		r.Status = domain.RunExecuting
		r.Outcome = "Executing"
		evs := []store.Event{}
		if f := s.Finding(r.FindingID); f != nil {
			f.Status = domain.FindingExecuting
			evs = append(evs, store.FindingEvent(f))
		}
		s.Usage["remediator"].Runs++
		evs = append(evs, store.RunEvent(r), store.UsageEvent(s), store.Activity(s, "action", "Remediator running "+action+" on "+str(args, "resource")+" with the write identity ("+why+").", t.runID, false))
		return evs
	})
	time.Sleep(1500 * time.Millisecond) // the provider takes a moment
	out, err := t.w.Execute(world.Action{Name: action, Args: args})
	t.st.Update(func(s *domain.State) []store.Event {
		r := s.Run(t.runID)
		if r == nil {
			return nil
		}
		evs := []store.Event{}
		if err != nil {
			r.Status = domain.RunFailed
			r.Outcome = "Action failed"
			t.appendMsg(r, domain.RoleExecutor, "remediator", "write identity", cmd+"\nstatus: Failed\n"+err.Error())
			if f := s.Finding(r.FindingID); f != nil {
				f.Status = domain.FindingEscalated
				evs = append(evs, store.FindingEvent(f))
			}
			evs = append(evs, store.Activity(s, "action", "Action failed: "+err.Error(), t.runID, false))
		} else {
			r.Status = domain.RunVerifying
			r.Outcome = "Verifying"
			t.appendMsg(r, domain.RoleExecutor, "remediator", "write identity", cmd+"\nstatus: Succeeded\n"+out)
			if f := s.Finding(r.FindingID); f != nil {
				f.Status = domain.FindingVerifying
				evs = append(evs, store.FindingEvent(f))
			}
			evs = append(evs, store.Activity(s, "action", "Action finished. "+firstLine(out)+" Verifier re-reading the signal.", t.runID, false))
		}
		return append(evs, store.RunEvent(r))
	})
	if err != nil {
		return "The action failed: " + err.Error() + ". Conclude with what the operator should do.", nil
	}
	return "Executed. Provider response:\n" + out + "\nThe verifier now re-reads the signal independently. Call conclude.", nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func describe(action string, args map[string]any) string {
	res := str(args, "resource")
	switch action {
	case "rotate_logs":
		return "ansible " + res + " -m logrotate: logrotate --force /etc/logrotate.d/nginx && NGINX_DEBUG=0 && nginx -s reload"
	case "run_command":
		return "az vm run-command invoke --name " + res + " --command-id RunShellScript --scripts '" + str(args, "command") + "'"
	case "restart_service":
		return "ssh " + res + " systemctl restart " + str(args, "service")
	case "rollout_undo":
		return "kubectl --context " + res + " rollout undo deployment/" + str(args, "service")
	case "start_server":
		return "POST /1.3/server/" + res + "/start"
	case "notify":
		return "notify owner of " + res
	}
	return action
}

func (t *Toolbox) responderID() string {
	if t.responder == nil {
		return "none"
	}
	return t.responder.ID
}

// addTool appends a running tool message and returns its id.
func (t *Toolbox) addTool(name string, in map[string]any) string {
	var id string
	t.st.Update(func(s *domain.State) []store.Event {
		r := s.Run(t.runID)
		if r == nil {
			return nil
		}
		id = t.st.NextID("m", 0)
		r.Messages = append(r.Messages, &domain.Message{ID: id, Role: domain.RoleTool, Agent: t.agentID, Tool: &domain.ToolCall{Name: name, Input: in, Status: "running"}, At: time.Now()})
		return []store.Event{store.RunEvent(r)}
	})
	return id
}

func (t *Toolbox) finishTool(id, out, status string) {
	t.st.Update(func(s *domain.State) []store.Event {
		r := s.Run(t.runID)
		if r == nil {
			return nil
		}
		for _, m := range r.Messages {
			if m.ID == id && m.Tool != nil {
				m.Tool.Output = out
				m.Tool.Status = status
			}
		}
		return []store.Event{store.RunEvent(r)}
	})
}

func (t *Toolbox) appendMsg(r *domain.Run, role, agent, title, text string) *domain.Message {
	m := &domain.Message{ID: t.st.NextID("m", 0), Role: role, Agent: agent, Title: title, Text: text, At: time.Now()}
	r.Messages = append(r.Messages, m)
	return m
}

// Say appends an agent message to the run and returns its id (for streaming updates).
func (t *Toolbox) Say(role, title, text string) string {
	var id string
	t.st.Update(func(s *domain.State) []store.Event {
		r := s.Run(t.runID)
		if r == nil {
			return nil
		}
		m := t.appendMsg(r, role, t.agentID, title, text)
		id = m.ID
		return []store.Event{store.RunEvent(r)}
	})
	return id
}

// Append adds text to an existing message, used while streaming.
func (t *Toolbox) Append(id, delta string) {
	t.st.Update(func(s *domain.State) []store.Event {
		r := s.Run(t.runID)
		if r == nil {
			return nil
		}
		for _, m := range r.Messages {
			if m.ID == id {
				m.Text += delta
			}
		}
		return []store.Event{{Type: "run.delta", Data: map[string]string{"runId": t.runID, "messageId": id, "delta": delta}}}
	})
}

// SortedKeys is a small helper for deterministic output.
func SortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// inGuestMetrics are only visible through the probe; the rest come from the provider.
var inGuestMetrics = map[string]bool{"memory_percent": true, "disk_used_percent": true, "log_growth_mb_per_min": true, "http_up": true, "pod_restarts_10m": true, "p95_latency_ms": true}

// needsProbe refuses in-guest reads on cells without the probe, and any read on a non-cell.
func (t *Toolbox) needsProbe(cellID, metric string) error {
	var err error
	t.st.View(func(s *domain.State) {
		c := s.Cell(cellID)
		if c == nil {
			err = fmt.Errorf("%q is not an onboarded cell", cellID)
			return
		}
		if !c.HasProbe() && (metric == "" || inGuestMetrics[metric]) {
			err = fmt.Errorf("%s is at %s; %s needs the probe (Level %d). Provider sensors still give cpu_percent, requests_per_second, error_rate_percent and state", cellID, c.LevelName(), map[bool]string{true: "reading files", false: metric}[metric == ""], c.ProbeLevel())
			return
		}
		if metric != "" && metric != "cpu_percent" && metric != "requests_per_second" && metric != "error_rate_percent" && metric != "connections" && !c.Telemetry.HostMetrics {
			err = fmt.Errorf("host metrics are switched off in the probe telemetry of %s", cellID)
		}
	})
	return err
}

// logAllowed refuses logs the probe is not configured to tail.
func (t *Toolbox) logAllowed(cellID, path string) error {
	var err error
	t.st.View(func(s *domain.State) {
		c := s.Cell(cellID)
		if c == nil {
			err = fmt.Errorf("%q is not an onboarded cell", cellID)
			return
		}
		if !c.HasProbe() && !(c.IsCluster() && c.Level >= domain.LevelAPI) {
			err = fmt.Errorf("%s is at %s; logs need Level 1", cellID, c.LevelName())
			return
		}
		if !c.Telemetry.LogEnabled(path) {
			var on []string
			for _, l := range c.Telemetry.Logs {
				if l.Enabled {
					on = append(on, l.Name)
				}
			}
			err = fmt.Errorf("%s is not a log source in the probe telemetry of %s; enabled: %s", path, cellID, strings.Join(on, ", "))
		}
	})
	return err
}
