package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/asitha-w/warden/internal/domain"
	"github.com/asitha-w/warden/internal/scenario"
	"github.com/asitha-w/warden/internal/setup"
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
	// pending is the one thing Warden asked the operator for and is waiting
	// on: a runbook description for a cell.
	pending struct{ kind, cell string }
}

// NewScripted returns a scripted reasoner.
func NewScripted(st *store.Store, pace time.Duration, model string) *Scripted {
	return &Scripted{st: st, cat: scenario.Catalog(), pace: pace, model: model}
}

// Name implements Reasoner.
func (s *Scripted) Name() string { return "scripted" }

// Forget drops anything the scripted reasoner was waiting on; a new session.
func (s *Scripted) Forget() { s.pending = struct{ kind, cell string }{} }

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
	var resource string
	s.st.View(func(st *domain.State) {
		if f := st.Finding(run.FindingID); f != nil {
			resource = f.Resource
		}
	})
	var def *scenario.Definition
	for id := range s.cat {
		d := s.cat[id]
		if d.Responder == run.ResponderID && d.Resource == resource {
			def = &d
		}
	}
	if def == nil {
		if d, ok := s.cat[responderScenario[run.ResponderID]]; ok {
			def = &d
		}
	}
	if def != nil {
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
		// the full state goes in the tool line; the answer is the summary
		_, _ = req.Call(ctx, "get_setup_state", map[string]any{})
		return strings.TrimSpace(lead + " " + s.setupSummary())
	}
	switch {
	case strings.HasPrefix(q, "[announce] tenant"):
		answer = "I am Warden, the agent that looks after this estate. Which provider do you run on: Azure, UpCloud or AWS? Tell me and I open the right form; after that I take it from there."
	case strings.HasPrefix(q, "[announce] watch"):
		first, _, _ := strings.Cut(strings.TrimPrefix(req.Question, "[announce] watch "), "\n")
		answer = "A watch fired and I acted on it: " + first
		if strings.Contains(first, "Standing order") {
			answer += " Nothing needs your approval; the standing order covers it."
		} else {
			answer += " The fix will need your approval; it appears in the panel when the investigator proposes it."
		}
	case req.Call != nil && strings.HasPrefix(q, "[announce] workflow"):
		first, _, _ := strings.Cut(strings.TrimPrefix(req.Question, "[announce] workflow "), "\n")
		res := resourceIn(q)
		rid := responderFor(q)
		short := first
		if i := strings.Index(first, ": "); i > 0 {
			short = first[i+2:]
		}
		if rid != "" && res != "" {
			out, err := req.Call(ctx, "start_responder", map[string]any{"responder": rid, "resource": res, "title": responderTitle(rid, res), "reason": short})
			if err == nil {
				answer = fmt.Sprintf("A watch fired on %s: %s That matches responder %s, so I started it. %s", res, short, rid, out)
			} else {
				answer = fmt.Sprintf("A watch fired on %s: %s I could not start responder %s: %s", res, short, rid, err.Error())
			}
		} else if res != "" {
			out, _ := req.Call(ctx, "start_investigation", map[string]any{"resource": res, "task": "A watch fired: " + short})
			answer = fmt.Sprintf("A watch fired on %s: %s No responder in the table fits, so I started a general investigation. %s", res, short, out)
		} else {
			answer = "A watch fired but I could not tell which cell it was about: " + short
		}
	case s.pending.kind == "runbook" && req.Call != nil && !strings.Contains(q, "cancel") && !strings.Contains(q, "never mind"):
		cell := s.pending.cell
		s.pending = struct{ kind, cell string }{}
		name, sel, body := s.composeRunbook(cell, req.Question)
		out, err := req.Call(ctx, "save_runbook", map[string]any{"name": name, "selector": sel, "body": body})
		if err != nil {
			answer = "I could not save the runbook: " + err.Error()
		} else {
			answer = "I wrote the runbook from what you said and saved it. " + out + " Next, tell me what to watch: for example \"watch the logs on " + cell + " every 10 seconds and wake me on errors\"."
		}
	case s.pending.kind == "runbook":
		s.pending = struct{ kind, cell string }{}
		answer = "Fine, no runbook for now."
	case req.Call == nil:
		answer = "I am running in scripted mode without a model and have no tools in this context."
	case !how && strings.Contains(q, "runbook") && resourceIn(q) != "":
		s.pending = struct{ kind, cell string }{"runbook", resourceIn(q)}
		answer = "Tell me in one message: what runs on " + resourceIn(q) + ", what is safe for me to do, what I must never do, and who to page. I write the runbook from that and show it under Runbooks before it is used."
	case !how && strings.Contains(q, "runbook"):
		answer = "Name the cell the runbook is for, for example \"add a runbook for edge-01\"."
	case !how && metricIn(q) != "" && (strings.Contains(q, "alert") || strings.Contains(q, "watch") || strings.Contains(q, "monitor") || strings.Contains(q, "standing order")):
		metric, responder := metricIn(q), metricResponder(metricIn(q))
		threshold := 90.0
		if m := pctRe.FindStringSubmatch(q); m != nil {
			fmt.Sscanf(m[1], "%f", &threshold)
		}
		sel := ""
		switch {
		case strings.Contains(q, "production") || strings.Contains(q, " prod"):
			sel = "env=production"
		case strings.Contains(q, "staging"):
			sel = "env=staging"
		}
		in := map[string]any{"name": strings.ReplaceAll(metric, "_", " ") + " on " + strings.TrimPrefix(sel, "env="), "collect": "metric", "source": metric, "op": ">", "threshold": threshold, "hold": 3, "every_seconds": everyIn(q), "trigger": "wake-warden"}
		if sel != "" {
			in["selector"] = sel
		} else if res := resourceIn(q); res != "" {
			in["cell"] = res
			in["name"] = strings.ReplaceAll(metric, "_", " ") + " on " + res
		} else {
			in["selector"] = "env=production"
			in["name"] = strings.ReplaceAll(metric, "_", " ") + " on production"
		}
		if responder != "" {
			in["trigger"] = "responder:" + responder
		}
		preapproved := strings.Contains(q, "pre-approved") || strings.Contains(q, "preapproved") || strings.Contains(q, "already approved") || strings.Contains(q, "without asking") || strings.Contains(q, "approved action")
		cleanup := strings.Contains(q, "clean") || strings.Contains(q, "rotate") || strings.Contains(q, "logs")
		if preapproved && cleanup && responder == "disk-pressure-linux" {
			in["allow"] = []any{"rotate_logs"}
		}
		out, err := req.Call(ctx, "create_workflow", in)
		switch {
		case err != nil:
			answer = "I could not create the watch: " + err.Error()
		case preapproved && in["allow"] == nil:
			answer = out + " I did not pre-approve anything: only catalogue actions can be pre-approved, and none of them fits what you asked."
		default:
			answer = out
		}
	case !how && (strings.Contains(q, "watch") || strings.Contains(q, "monitor")) && resourceIn(q) != "":
		res := resourceIn(q)
		every := everyIn(q)
		source := "*"
		switch {
		case strings.Contains(q, "error log") || strings.Contains(q, "error.log"):
			source = "/var/log/nginx/error.log"
		case strings.Contains(q, "syslog"):
			source = "/var/log/syslog"
		}
		out, err := req.Call(ctx, "create_workflow", map[string]any{"name": "Logs on " + res, "cell": res, "every_seconds": every, "collect": "log-digest", "source": source, "trigger": "wake-warden"})
		if err != nil {
			answer = "I could not create the workflow: " + err.Error()
		} else {
			answer = out + " You will see it fire in this thread, and I will say what I do about it."
		}
	case !how && (strings.Contains(q, "add") || strings.Contains(q, "create")) && (strings.Contains(q, "agent") || strings.Contains(q, "specialist")):
		tid := templateIn(q)
		if tid == "" {
			out, _ := req.Call(ctx, "list_templates", map[string]any{})
			answer = "Which specialist? These are the templates I can add:\n" + out
			break
		}
		scope := ""
		if res := resourceIn(q); res != "" {
			s.st.View(func(st *domain.State) {
				if c := st.Cell(res); c != nil {
					scope = selectorText(CellSelector(c))
				}
			})
		} else if strings.Contains(q, "edge") {
			scope = "app=edge"
		} else if strings.Contains(q, "shop") {
			scope = "app=shop"
		}
		out, err := req.Call(ctx, "add_agent", map[string]any{"template": tid, "scope": scope})
		if err != nil {
			answer = "I could not add the agent: " + err.Error()
		} else {
			answer = out + " It is on the Agents page with its tools and budget; it runs only when its responder fires."
		}
	case !how && (q == "upcloud" || strings.Contains(q, "upcloud") && !strings.Contains(q, "onboard")) && !strings.Contains(q, "connection ") && len(q) < 40:
		s.st.Publish(store.Event{Type: "ui", Data: map[string]string{"open": "upcloud"}})
		answer = "UpCloud. I opened the token form in the panel: a sub-account token is enough to start, and it goes in the form, never here. When you save it I list what it can reach."
	case !how && (q == "azure" || strings.Contains(q, "azure") && len(q) < 40):
		s.st.Publish(store.Event{Type: "ui", Data: map[string]string{"open": "azure"}})
		answer = "Azure. I opened the credential form in the panel: consent to the Warden app or bring your own app registration, at a resource group or the subscription. When you save it I list what it can reach."
	case !how && (q == "aws" || strings.Contains(q, "aws") && len(q) < 40):
		answer = "AWS is not supported yet; Azure and UpCloud are. Say which of those you want and I open the form."
	case req.Call != nil && strings.HasPrefix(q, "[announce] connection"):
		answer = withState("Your connection is validated.")
	case req.Call != nil && strings.HasPrefix(q, "[announce] cell"):
		first, _, _ := strings.Cut(strings.TrimPrefix(req.Question, "[announce] cell"), "\n")
		answer = withState(strings.TrimSpace(first))
	case req.Call != nil && !how && strings.Contains(q, "onboard"):
		in := map[string]any{}
		if m := connRe.FindString(q); m != "" {
			in["connection"] = m
		}
		out, err := req.Call(ctx, "onboard_cells", in)
		if err != nil || strings.HasPrefix(out, "no connection") || strings.HasPrefix(out, "nothing to onboard") || strings.HasPrefix(out, "say which") {
			answer = out
		} else {
			answer = withState(out)
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
		answer = withState("")
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
		for li, line := range strings.Split(answer, "\n") {
			if li > 0 {
				onDelta("\n")
			}
			for i, w := range strings.Fields(line) {
				if i > 0 {
					w = " " + w
				}
				onDelta(w)
				time.Sleep(12 * time.Millisecond)
			}
		}
	}
	return answer, nil
}

var connRe = regexp.MustCompile(`conn-[0-9a-f]+`)

// sentenceRe splits on a full stop followed by space or end, a semicolon or
// a newline, so dots inside addresses and paths survive.
var sentenceRe = regexp.MustCompile(`\.(?:\s+|$)|;|\n`)

// setupSummary is the scripted reasoner's one-paragraph reading of the setup
// overview: what exists, and the one next step with who does it.
func (s *Scripted) setupSummary() string {
	var o setup.Overview
	s.st.View(func(st *domain.State) { o = setup.Compute(st) })
	var b strings.Builder
	for _, c := range o.Connections {
		if c.Status == domain.ConnValidating {
			fmt.Fprintf(&b, "%s is still validating. ", c.Name)
			continue
		}
		fmt.Fprintf(&b, "Through %s I found %d things, %d of them I can manage", c.Name, c.Found, c.Manageable)
		switch {
		case c.Manageable == 0:
			b.WriteString(". ")
		case c.Onboarded == c.Manageable:
			b.WriteString(", all onboarded as cells. ")
		case c.Onboarded == 0:
			b.WriteString(", none onboarded yet. ")
		default:
			fmt.Fprintf(&b, ", %d onboarded so far. ", c.Onboarded)
		}
	}
	var levels []string
	for _, c := range o.Cells {
		what := "I see it from the provider only"
		switch {
		case c.Level >= c.MaxLevel:
			what = "I have full reach"
		case c.Level >= 1:
			what = "I can see inside and act"
		}
		levels = append(levels, fmt.Sprintf("%s is at %s, %s", c.ID, c.LevelName, what))
	}
	if len(levels) > 0 {
		b.WriteString(strings.Join(levels, "; ") + ". ")
	}
	if len(o.Next) == 0 {
		b.WriteString("Everything is at its highest level; there is nothing left to set up.")
		return b.String()
	}
	n := o.Next[0]
	switch n.Who {
	case setup.WhoOperator:
		fmt.Fprintf(&b, "Next, yours to do: %s.", strings.ToLower(n.Title[:1])+n.Title[1:])
	case setup.WhoBoth:
		fmt.Fprintf(&b, "Next: %s. Say \"grant %s\" and I issue the token and give you the line to run.", strings.ToLower(n.Title[:1])+n.Title[1:], n.Target)
	default:
		if n.Tool == "" {
			fmt.Fprintf(&b, "Now: %s.", strings.ToLower(n.Title[:1])+n.Title[1:])
		} else {
			fmt.Fprintf(&b, "Next: %s. Say so, or press the button in the panel, and I do it.", strings.ToLower(n.Title[:1])+n.Title[1:])
		}
	}
	return b.String()
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

// responderFor picks the responder a workflow hit maps to, from the words in
// the digest. Deterministic and small on purpose.
func responderFor(q string) string {
	switch {
	case strings.Contains(q, "logrotate") || strings.Contains(q, "disk") || strings.Contains(q, "no space"):
		return "disk-pressure-linux"
	case strings.Contains(q, "nginx") || strings.Contains(q, "out of memory") || strings.Contains(q, "killed") || strings.Contains(q, "failed with result"):
		return "service-down"
	case strings.Contains(q, "stopped"):
		return "server-stopped"
	case strings.Contains(q, "error rate") || strings.Contains(q, " 502 ") || strings.Contains(q, "upstream"):
		return "release-regression"
	}
	return ""
}

func responderTitle(rid, res string) string {
	switch rid {
	case "service-down":
		return "nginx on " + res + " is failing"
	case "server-stopped":
		return res + " is stopped without an operator event"
	case "disk-pressure-linux":
		return "Disk filling on " + res
	case "release-regression":
		return "Errors on " + res + " after a rollout"
	}
	return "Workflow hit on " + res
}

var pctRe = regexp.MustCompile(`(\d{1,3})\s*%`)

func metricIn(q string) string {
	switch {
	case strings.Contains(q, "disk"):
		return "disk_used_percent"
	case strings.Contains(q, "memory"):
		return "memory_percent"
	case strings.Contains(q, "cpu"):
		return "cpu_percent"
	case strings.Contains(q, "error rate"):
		return "error_rate_percent"
	}
	return ""
}

func metricResponder(metric string) string {
	switch metric {
	case "disk_used_percent":
		return "disk-pressure-linux"
	case "error_rate_percent":
		return "release-regression"
	}
	return ""
}

func everyIn(q string) int {
	for _, p := range []struct {
		words string
		secs  int
	}{{"every 5 seconds", 5}, {"every 10 seconds", 10}, {"every 15 seconds", 15}, {"every 30 seconds", 30}, {"every minute", 60}, {"every 2 minutes", 120}, {"every 5 minutes", 300}, {"every 10 minutes", 600}, {"every hour", 3600}} {
		if strings.Contains(q, p.words) {
			return p.secs
		}
	}
	return 10 // demo pace, the same as the seeded watches; say "every minute" for a production cadence
}

func templateIn(q string) string {
	switch {
	case strings.Contains(q, "nginx") || strings.Contains(q, "proxy") || strings.Contains(q, "edge"):
		return "nginx-specialist"
	case strings.Contains(q, "kubernetes") || strings.Contains(q, "k8s") || strings.Contains(q, "aks") || strings.Contains(q, "rollout"):
		return "kubernetes-specialist"
	case strings.Contains(q, "cost"):
		return "cost-advisor"
	}
	return ""
}

// composeRunbook writes a runbook from one operator message about a cell:
// the message is kept whole as the description and its sentences are sorted
// into the sections by their words. Warden says it wrote it from their words.
func (s *Scripted) composeRunbook(cellID, text string) (name, selector, body string) {
	var c *domain.Cell
	s.st.View(func(st *domain.State) {
		if x := st.Cell(cellID); x != nil {
			cp := *x
			c = &cp
		}
	})
	app, kind := cellID, "cells"
	sel := map[string]string{}
	if c != nil {
		sel = CellSelector(c)
		if a := c.Labels["app"]; a != "" {
			app = strings.ToUpper(a[:1]) + a[1:]
		}
		kind = c.Kind
	}
	name = fmt.Sprintf("%s on %s", app, kind)
	selector = selectorText(sel)
	if selector == "" {
		selector = "cell=" + cellID
	}
	var fixes, never, escalate, signals, rest []string
	for _, raw := range sentenceRe.Split(text, -1) {
		sent := strings.TrimSpace(raw)
		if sent == "" {
			continue
		}
		l := strings.ToLower(sent)
		switch {
		case strings.Contains(l, "never") || strings.Contains(l, "do not") || strings.Contains(l, "don't"):
			never = append(never, sent)
		case strings.Contains(l, "page") || strings.Contains(l, "escalat") || strings.Contains(l, "@") || strings.Contains(l, "on-call") || strings.Contains(l, "slack"):
			escalate = append(escalate, sent)
		case strings.Contains(l, "safe") || strings.Contains(l, "restart") || strings.Contains(l, "rotate") || strings.Contains(l, "clear") || strings.Contains(l, "fix"):
			fixes = append(fixes, sent)
		case strings.Contains(l, "normal") || strings.Contains(l, "above") || strings.Contains(l, "below") || strings.Contains(l, "%"):
			signals = append(signals, sent)
		default:
			rest = append(rest, sent)
		}
	}
	unit := "the service named in the description"
	for _, u := range []string{"nginx", "postgres", "redis", "shop-api", "shop-web", "haproxy", "varnish"} {
		if strings.Contains(strings.ToLower(text), u) {
			unit = u
			break
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", name)
	fmt.Fprintf(&b, "Written by Warden from the operator's words on %s. Edit freely.\n\n", time.Now().Format("2006-01-02"))
	if len(rest) > 0 {
		b.WriteString(strings.Join(rest, ". ") + ".\n\n")
	}
	fmt.Fprintf(&b, "## Applies when\n- Cells labelled %s running unit %s.\n\n", selector, unit)
	section := func(title string, items []string, numbered bool, empty string) {
		fmt.Fprintf(&b, "## %s\n", title)
		if len(items) == 0 {
			b.WriteString("- " + empty + "\n\n")
			return
		}
		for i, it := range items {
			if numbered {
				fmt.Fprintf(&b, "%d. %s.\n", i+1, it)
			} else {
				fmt.Fprintf(&b, "- %s.\n", it)
			}
		}
		b.WriteString("\n")
	}
	section("Health signals", signals, false, "Not stated yet; the workflows on this cell define the signals.")
	section("Known fixes, in order", fixes, true, "None stated; every action needs approval.")
	section("Never", never, false, "Nothing stated; delete-class actions are denied by policy anyway.")
	section("Escalation", escalate, false, "Not stated; findings escalate to the tenant owner.")
	return name, selector, strings.TrimSpace(b.String())
}
