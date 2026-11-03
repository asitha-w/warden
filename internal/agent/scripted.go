package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/asitha-w/warden/internal/domain"
	"github.com/asitha-w/warden/internal/scenario"
	"github.com/asitha-w/warden/internal/store"
)

// Scripted replays the scenario playbooks through the real toolbox, so policy,
// approvals, evidence and verification behave exactly as with the model. It is
// used when no API key is configured and for repeatable demos.
type Scripted struct {
	st    *store.Store
	cat   map[string]scenario.Definition
	pace  time.Duration
	model string
}

// NewScripted returns a scripted reasoner.
func NewScripted(st *store.Store, pace time.Duration, model string) *Scripted {
	return &Scripted{st: st, cat: scenario.Catalog(), pace: pace, model: model}
}

// Name implements Reasoner.
func (s *Scripted) Name() string { return "scripted" }

// responderScenario maps a responder to the playbook that handles it.
var responderScenario = map[string]string{
	"disk-pressure-linux": "disk-fill",
	"release-regression":  "bad-release",
	"service-down":        "service-crash",
	"server-stopped":      "server-stopped",
}

// Investigate implements Reasoner.
func (s *Scripted) Investigate(ctx context.Context, run *domain.Run, tb *Toolbox) error {
	var playbook []scenario.Step
	if def, ok := s.cat[responderScenario[run.ResponderID]]; ok {
		playbook = def.Playbook
		tb.Chain = def.Chain
	} else {
		playbook = healthCheck(run)
	}
	var rbNames []string
	s.st.View(func(st *domain.State) {
		for _, id := range run.Runbooks {
			if rb := st.Runbook(id); rb != nil {
				rbNames = append(rbNames, rb.Name)
			}
		}
	})
	for i, step := range playbook {
		if err := ctx.Err(); err != nil {
			return err
		}
		if i == 0 && step.Say != "" && len(rbNames) > 0 {
			step.Say = "Following runbook " + strings.Join(rbNames, " and ") + ". " + step.Say
		}
		if step.Say != "" {
			s.stream(ctx, tb, step.Say)
		}
		if step.Tool != "" {
			// simulated model usage so the consumption views have numbers; labelled as such in the UI
			AddUsage(s.st, run.ID, run.Agent, s.model, 2400+len(step.Tool)*40, 160)
			if _, err := tb.Call(ctx, step.Tool, step.Input); err != nil {
				return err
			}
			time.Sleep(s.pace)
		}
	}
	return nil
}

func (s *Scripted) stream(ctx context.Context, tb *Toolbox, text string) {
	id := tb.Say(domain.RoleAgent, "", "")
	words := strings.Fields(text)
	for i, w := range words {
		if ctx.Err() != nil {
			return
		}
		if i > 0 {
			w = " " + w
		}
		tb.Append(id, w)
		time.Sleep(s.pace / 40)
	}
}

// Chat implements Reasoner with keyword answers and keyword tool routing.
func (s *Scripted) Chat(ctx context.Context, req ChatRequest, onDelta func(string)) (string, error) {
	q := strings.ToLower(req.Question)
	answer := ""
	how := strings.Contains(q, "how ")
	withState := func(lead string) string {
		out, _ := req.Call(ctx, "get_setup_state", map[string]any{})
		return lead + "\n" + out
	}
	switch {
	case req.Call != nil && !how && strings.Contains(q, "onboard"):
		out, err := req.Call(ctx, "onboard_cells", map[string]any{})
		if err != nil || strings.HasPrefix(out, "no connection") || strings.HasPrefix(out, "nothing to onboard") || strings.HasPrefix(out, "say which") {
			answer = out
		} else {
			answer = withState(out + " Here is where setup stands now.")
		}
	case req.Call != nil && !how && strings.Contains(q, "grant") && resourceIn(q) != "":
		out, _ := req.Call(ctx, "issue_grant", map[string]any{"cell": resourceIn(q), "tunnel": strings.Contains(q, "tunnel")})
		answer = out
	case req.Call != nil && !how && strings.Contains(q, "api access") && resourceIn(q) != "":
		out, _ := req.Call(ctx, "enable_api_access", map[string]any{"cell": resourceIn(q)})
		answer = out
	case req.Call != nil && !how && strings.Contains(q, "probe") && resourceIn(q) != "":
		out, _ := req.Call(ctx, "install_probe", map[string]any{"cell": resourceIn(q)})
		answer = out
	case req.Call != nil && !how && (strings.Contains(q, "catalogue") || strings.Contains(q, "catalog") || strings.Contains(q, "discover")):
		out, _ := req.Call(ctx, "list_catalogue", map[string]any{})
		answer = "This is what the connection found:\n" + out
	case req.Call != nil && !how && (strings.Contains(q, "found") || strings.Contains(q, "find") || strings.Contains(q, "next") || strings.Contains(q, "set up") || strings.Contains(q, "setup") || strings.Contains(q, "what is there") || strings.Contains(q, "what's there") || strings.Contains(q, "what can you") || strings.Contains(q, "where are we")):
		answer = withState("Here is where setup stands.")
	case req.Call != nil && (strings.Contains(q, "investigate") || strings.Contains(q, "look into") || strings.Contains(q, "check")) && resourceIn(q) != "":
		res := resourceIn(q)
		out, _ := req.Call(ctx, "start_investigation", map[string]any{"resource": res, "task": req.Question})
		answer = out
	case req.Call != nil && (strings.Contains(q, "finding") || strings.Contains(q, "wrong") || strings.Contains(q, "status") || strings.Contains(q, "what is open")):
		out, _ := req.Call(ctx, "list_findings", map[string]any{})
		answer = "Here is the current list, newest first:\n" + out
	case req.Call != nil && resourceIn(q) != "":
		out, _ := req.Call(ctx, "get_resource_state", map[string]any{"resource": resourceIn(q)})
		answer = "From the provider and the probe: " + out
	case req.Call != nil && docPageFor(q) != "":
		page := docPageFor(q)
		out, _ := req.Call(ctx, "read_docs", map[string]any{"page": page})
		answer = "The steps are in the documentation, page " + page + " (#docs/" + page + "). " + docSummary(page, out)
	case strings.Contains(q, "why") || strings.Contains(q, "cause"):
		answer = "The cause is in the evidence table: every claim there has a source pointer, and the hypotheses section says which alternative was rejected and why."
	case strings.Contains(q, "risk") || strings.Contains(q, "safe") || strings.Contains(q, "downtime"):
		answer = "The proposed action lists its blast radius: what changes, what could go wrong, and what does not change. If it fails, nothing has changed and the finding escalates to on-call."
	case strings.Contains(q, "cost") || strings.Contains(q, "token"):
		answer = "Usage per agent and per run is on the Agents and Consumption pages. Deterministic agents, the remediator and the verifier, cost nothing."
	default:
		answer = "I am running in scripted mode without a model. I can tell you what setup found and what is next, onboard what a connection found, issue the grant for a VM, enable API access or install the probe on a cluster, list findings, describe a resource (name it, for example web-01), or start an investigation if you ask me to check a resource."
	}
	if onDelta != nil {
		for i, w := range strings.Fields(answer) {
			if i > 0 {
				w = " " + w
			}
			onDelta(w)
			time.Sleep(12 * time.Millisecond)
		}
	}
	return answer, nil
}

func resourceIn(q string) string {
	for _, r := range []string{"web-01", "aks-shop", "edge-01", "db-01", "la-shop", "vm-staging-01", "uks-edge"} {
		if strings.Contains(q, r) {
			return r
		}
	}
	return ""
}

// healthCheck is the generic playbook for an operator request without a
// matching fault: read the resource, record what was seen, recommend.
func healthCheck(run *domain.Run) []scenario.Step {
	res := resourceIn(strings.ToLower(run.Task))
	if res == "" {
		res = "web-01"
	}
	in := func(m map[string]any) map[string]any { m["resource"] = res; return m }
	return []scenario.Step{
		{Say: fmt.Sprintf("Checking %s as asked. I will read its state and the main series before saying anything.", res)},
		{Tool: "get_resource_state", Input: in(map[string]any{})},
		{Tool: "read_metrics", Input: in(map[string]any{"metric": "cpu_percent", "window_minutes": 30})},
		{Tool: "read_metrics", Input: in(map[string]any{"metric": "memory_percent", "window_minutes": 30})},
		{Tool: "read_metrics", Input: in(map[string]any{"metric": "http_up", "window_minutes": 30})},
		{Tool: "get_activity", Input: in(map[string]any{})},
		{Tool: "record_evidence", Input: in(map[string]any{"kind": "resource", "claim": res + " is started and its services are running.", "pointer": "provider state · " + res})},
		{Tool: "record_evidence", Input: in(map[string]any{"kind": "metric", "claim": "CPU, memory and http_up are within their normal bands over the last 30 minutes.", "pointer": "hostmetrics · " + res})},
		{Tool: "record_hypothesis", Input: map[string]any{"title": "Nothing wrong on " + res + " right now", "detail": "No signal rule is near its threshold and no recent change is recorded.", "verdict": "supported"}},
		{Say: "Nothing is wrong on " + res + " from what I can read. If it still feels slow, name the service or the time window and I will look there."},
		{Tool: "conclude", Input: map[string]any{"summary": res + " looks healthy: state started, metrics in band, no recent changes. No action proposed.", "cause_verified": false}},
	}
}

// docPageFor routes a setup question to a documentation page, or "".
func docPageFor(q string) string {
	setup := strings.Contains(q, "how do i") || strings.Contains(q, "how to") || strings.Contains(q, "set up") || strings.Contains(q, "setup") || strings.Contains(q, "connect") || strings.Contains(q, "onboard") || strings.Contains(q, "grant") || strings.Contains(q, "install") || strings.Contains(q, "revoke") || strings.Contains(q, "level") || strings.Contains(q, "permission") || strings.Contains(q, "role")
	if !setup {
		return ""
	}
	switch {
	case strings.Contains(q, "azure") || strings.Contains(q, "aks") || strings.Contains(q, "entra"):
		return "azure"
	case strings.Contains(q, "upcloud") || strings.Contains(q, "uks"):
		return "upcloud"
	case strings.Contains(q, "grant") || strings.Contains(q, "ssh") || strings.Contains(q, "port 22") || strings.Contains(q, "tunnel"):
		return "grant"
	case strings.Contains(q, "probe") || strings.Contains(q, "collector") || strings.Contains(q, "telemetry"):
		return "probe"
	case strings.Contains(q, "revoke") || strings.Contains(q, "secur") || strings.Contains(q, "credential"):
		return "security"
	case strings.Contains(q, "level") || strings.Contains(q, "cell") || strings.Contains(q, "connect"):
		return "model"
	}
	return "howto"
}

// docSummary returns the page's headings so the scripted answer says what the page covers.
func docSummary(page, md string) string {
	var heads []string
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "## ") {
			heads = append(heads, strings.TrimPrefix(line, "## "))
		}
	}
	if len(heads) == 0 {
		return ""
	}
	return "It covers: " + strings.Join(heads, "; ") + "."
}
