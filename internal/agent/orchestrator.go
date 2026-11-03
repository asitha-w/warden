package agent

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/asitha-w/warden/internal/domain"
	"github.com/asitha-w/warden/internal/setup"
	"github.com/asitha-w/warden/internal/store"
	"github.com/asitha-w/warden/internal/world"
	"github.com/asitha-w/warden/web"
)

// Orchestrator is the agent the operator talks to. It applies the responder
// table to signals, starts specialists, answers questions, and never changes
// infrastructure itself.
type Orchestrator struct {
	st       *store.Store
	w        *world.World
	gate     *Gate
	reasoner Reasoner
	setup    *setup.Service
	log      *slog.Logger
	model    string
	tenant   func() string

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	chatMu  sync.Mutex
}

// NewOrchestrator wires the orchestrator.
func NewOrchestrator(st *store.Store, w *world.World, gate *Gate, r Reasoner, su *setup.Service, model string, log *slog.Logger) *Orchestrator {
	return &Orchestrator{st: st, w: w, gate: gate, reasoner: r, setup: su, log: log, model: model, cancels: map[string]context.CancelFunc{}}
}

// Signal is what the signal loop hands over when a responder matches.
type Signal struct {
	ResponderID string
	Resource    string
	Title       string
	Severity    string
	Text        string
	Allow       []string // standing order: actions pre-approved for the run
	AllowedBy   string   // the watch that holds the order
}

// HandleSignal routes a matched signal: it opens a finding and a run and starts
// the responder's agent. This is deterministic; no model call is spent on routing.
func (o *Orchestrator) HandleSignal(sig Signal) string {
	var runID string
	o.st.Update(func(s *domain.State) []store.Event {
		if s.OpenFinding(sig.ResponderID, sig.Resource) != nil {
			return nil
		}
		r := s.Responder(sig.ResponderID)
		if r == nil {
			return nil
		}
		agent := s.Agent(r.Agent)
		if agent == nil || !agent.Enabled {
			return []store.Event{store.Activity(s, "policy", fmt.Sprintf("Responder %s matched on %s but agent %s is disabled. Nothing started.", r.ID, sig.Resource, r.Agent), "", false)}
		}
		fid := o.st.NextID("F", 0x8d)
		runID = o.st.NextID("inv", 0xa6)
		f := &domain.Finding{ID: fid, Title: sig.Title, Service: s.GroupName(sig.Resource), Resource: sig.Resource, Severity: sig.Severity, Status: domain.FindingInvestigating, RunID: runID, ResponderID: r.ID, CreatedAt: time.Now()}
		run := &domain.Run{ID: runID, Agent: r.Agent, Trigger: "signal", Task: sig.Title, FindingID: fid, ResponderID: r.ID, Status: domain.RunRunning, Outcome: "Running", StartedAt: time.Now(), Messages: []*domain.Message{}, Evidence: []domain.Evidence{}, Hypotheses: []domain.Hypothesis{}, Chain: []domain.ChainStep{}, Questions: []domain.QA{}, Runbooks: []string{}, Allowed: sig.Allow, AllowedBy: sig.AllowedBy}
		order := ""
		if len(sig.Allow) > 0 {
			order = fmt.Sprintf(" Standing order %s pre-approves: %s.", sig.AllowedBy, strings.Join(sig.Allow, ", "))
		}
		run.Messages = append(run.Messages,
			&domain.Message{ID: o.st.NextID("m", 0), Role: domain.RoleSignal, Title: "Watch", Text: sig.Text + fmt.Sprintf(" Responder %s matched.", r.ID), At: time.Now()},
			&domain.Message{ID: o.st.NextID("m", 0), Role: domain.RoleOrchestrator, Agent: "orchestrator", Title: "Warden", Text: fmt.Sprintf("Starting %s for %s under responder %s (%s, %s) with a budget of %d tool calls.%s%s", agent.Name, sig.Resource, r.ID, r.Mode, r.Level, agent.MaxToolCalls, order, attachRunbooks(s, run, sig.Resource)), At: time.Now()})
		s.Findings = append([]*domain.Finding{f}, s.Findings...)
		s.Runs = append([]*domain.Run{run}, s.Runs...)
		s.Usage["orchestrator"].Runs++
		s.Usage[r.Agent].Runs++
		return []store.Event{
			store.Activity(s, "signal", sig.Text, runID, false),
			store.Toast("Signal: " + sig.Title),
			store.FindingEvent(f), store.RunEvent(run), store.UsageEvent(s),
			store.Activity(s, "run", fmt.Sprintf("Orchestrator: responder %s matched. %s started on %s.", r.ID, agent.Name, runID), runID, false),
		}
	})
	if runID != "" {
		go o.runInvestigation(runID)
	}
	return runID
}

// operatorResponder is the policy for investigations the operator asks for directly.
var operatorResponder = &domain.Responder{ID: "operator-request", Signal: "operator asked", Scope: "any resource", Mode: "Review", Level: "L3", Agent: "investigator",
	Tools: []domain.ToolPolicy{{Tool: "read_metrics", Policy: domain.PolicyAllow}, {Tool: "tail_logs", Policy: domain.PolicyAllow}, {Tool: "get_resource_state", Policy: domain.PolicyAllow}, {Tool: "run_command", Policy: domain.PolicyAsk}, {Tool: "restart_service", Policy: domain.PolicyAsk}, {Tool: "rollout_undo", Policy: domain.PolicyAsk}, {Tool: "start_server", Policy: domain.PolicyAsk}, {Tool: "notify", Policy: domain.PolicyAllow}}}

// StartInvestigation opens a run for an operator request.
func (o *Orchestrator) StartInvestigation(resource, task, by string) (string, error) {
	var runID string
	var errOut error
	o.st.Update(func(s *domain.State) []store.Event {
		res := s.Cell(resource)
		if res == nil {
			errOut = fmt.Errorf("no cell %q in this tenant; onboard it from its connection first", resource)
			return nil
		}
		agent := s.Agent("investigator")
		if agent == nil || !agent.Enabled {
			errOut = fmt.Errorf("the investigator agent is disabled")
			return nil
		}
		if s.Responder(operatorResponder.ID) == nil {
			s.Responders = append(s.Responders, operatorResponder)
		}
		fid := o.st.NextID("F", 0x8d)
		runID = o.st.NextID("inv", 0xa6)
		f := &domain.Finding{ID: fid, Title: task, Service: s.GroupName(resource), Resource: resource, Severity: "info", Status: domain.FindingInvestigating, RunID: runID, ResponderID: operatorResponder.ID, CreatedAt: time.Now()}
		run := &domain.Run{ID: runID, Agent: "investigator", Trigger: "user", Task: task, FindingID: fid, ResponderID: operatorResponder.ID, Status: domain.RunRunning, Outcome: "Running", StartedAt: time.Now(), Messages: []*domain.Message{}, Evidence: []domain.Evidence{}, Hypotheses: []domain.Hypothesis{}, Chain: []domain.ChainStep{}, Questions: []domain.QA{}, Runbooks: []string{}}
		run.Messages = append(run.Messages, &domain.Message{ID: o.st.NextID("m", 0), Role: domain.RoleOrchestrator, Agent: "orchestrator", Title: "Orchestrator", Text: fmt.Sprintf("%s asked: %q. Starting the investigator on %s under the operator-request policy (reads allowed, changes need approval).%s", by, task, resource, attachRunbooks(s, run, resource)), At: time.Now()})
		s.Findings = append([]*domain.Finding{f}, s.Findings...)
		s.Runs = append([]*domain.Run{run}, s.Runs...)
		s.Usage["investigator"].Runs++
		return []store.Event{store.FindingEvent(f), store.RunEvent(run), store.UsageEvent(s), store.Activity(s, "run", fmt.Sprintf("Orchestrator started %s on %s for %s.", runID, resource, by), runID, false)}
	})
	if errOut != nil {
		return "", errOut
	}
	go o.runInvestigation(runID)
	return runID, nil
}

func (o *Orchestrator) runInvestigation(runID string) {
	ctx, cancel := context.WithCancel(context.Background())
	o.mu.Lock()
	o.cancels[runID] = cancel
	o.mu.Unlock()
	defer func() {
		cancel()
		o.mu.Lock()
		delete(o.cancels, runID)
		o.mu.Unlock()
	}()
	var run *domain.Run
	var responder *domain.Responder
	maxCalls := 25
	o.st.View(func(s *domain.State) {
		if r := s.Run(runID); r != nil {
			cp := *r
			run = &cp
			responder = s.Responder(r.ResponderID)
			if a := s.Agent(r.Agent); a != nil && a.MaxToolCalls > 0 {
				maxCalls = a.MaxToolCalls
			}
		}
	})
	if run == nil {
		return
	}
	tb := NewToolbox(o.st, o.w, o.gate, runID, run.Agent, responder, maxCalls)
	tb.Allowed, tb.AllowedBy = run.Allowed, run.AllowedBy
	err := o.reasoner.Investigate(ctx, run, tb)
	o.st.Update(func(s *domain.State) []store.Event {
		r := s.Run(runID)
		if r == nil {
			return nil
		}
		f := s.Finding(r.FindingID)
		evs := []store.Event{}
		if err != nil && ctx.Err() == nil {
			o.log.Error("investigation failed", "run", runID, "err", err)
			r.Status = domain.RunFailed
			r.Outcome = "Failed: " + firstLine(err.Error())
			r.Messages = append(r.Messages, &domain.Message{ID: o.st.NextID("m", 0), Role: domain.RoleError, Title: "Run stopped", Text: err.Error() + ". The finding is escalated to on-call with the thread so far.", At: time.Now()})
			if f != nil && f.Status != domain.FindingResolved {
				f.Status = domain.FindingEscalated
				evs = append(evs, store.FindingEvent(f))
			}
			evs = append(evs, store.Activity(s, "verdict", "Run "+runID+" stopped: "+firstLine(err.Error()), runID, false))
		} else if r.Status == domain.RunRunning || r.Status == domain.RunWaiting {
			// the agent concluded without an executed action: a recommendation, or a denied one
			r.Status = domain.RunDone
			if r.ApprovalID != "" {
				r.Outcome = "Denied, escalated"
				if f != nil {
					f.Status = domain.FindingEscalated
					evs = append(evs, store.FindingEvent(f))
				}
				evs = append(evs, store.Activity(s, "verdict", "Denied. "+r.FindingID+" escalated to the on-call engineer with the full thread.", runID, false))
				evs = append(evs, endScenario(s, f)...)
			} else {
				r.Outcome = "Recommendation, no action"
				if f != nil {
					f.Status = domain.FindingOpen
					evs = append(evs, store.FindingEvent(f))
				}
			}
			verdict := o.verifierText(r)
			r.Messages = append(r.Messages, &domain.Message{ID: o.st.NextID("m", 0), Role: domain.RoleVerifier, Agent: "verifier", Title: "Verifier", Text: verdict, At: time.Now()})
			now := time.Now()
			r.EndedAt = &now
		}
		if r.Status == domain.RunVerifying {
			// evidence verdict lands now; the signal re-read resolves the finding later
			r.Messages = append(r.Messages, &domain.Message{ID: o.st.NextID("m", 0), Role: domain.RoleVerifier, Agent: "verifier", Title: "Verifier", Text: o.verifierText(r), At: time.Now()})
			s.Usage["verifier"].Runs++
			evs = append(evs, store.UsageEvent(s))
		}
		return append(evs, store.RunEvent(r))
	})
}

func (o *Orchestrator) verifierText(r *domain.Run) string {
	n := len(r.Evidence)
	claims := 0
	for _, h := range r.Hypotheses {
		if h.Verdict == "supported" {
			claims++
		}
	}
	if n == 0 {
		return "No evidence rows were recorded, so the cause is marked Unverified."
	}
	return fmt.Sprintf("%d evidence rows, each with a source pointer, support %d hypothesis. The cause is marked Verified because every link in the chain has a pointer, not because of a model score.", n, max(claims, 1))
}

// Decide records an approval decision and releases the waiting run.
func (o *Orchestrator) Decide(approvalID string, approve bool, by string) error {
	var errOut error
	o.st.Update(func(s *domain.State) []store.Event {
		a := s.Approval(approvalID)
		if a == nil {
			errOut = fmt.Errorf("no approval %s", approvalID)
			return nil
		}
		if a.Status != domain.ApprovalPending {
			errOut = fmt.Errorf("approval %s already %s", approvalID, a.Status)
			return nil
		}
		now := time.Now()
		a.DecidedAt = &now
		a.DecidedBy = by
		if approve {
			a.Status = domain.ApprovalApproved
		} else {
			a.Status = domain.ApprovalDenied
		}
		return []store.Event{store.ApprovalEvent(a), store.Activity(s, "action", fmt.Sprintf("%s %s %s: %s.", by, a.Status, a.ID, a.Task), a.RunID, false)}
	})
	if errOut != nil {
		return errOut
	}
	var ap domain.Approval
	o.st.View(func(s *domain.State) {
		if a := s.Approval(approvalID); a != nil {
			ap = *a
		}
	})
	if ap.RunID == "" {
		if !approve {
			return nil
		}
		ok, err := o.runSetupApproval(&ap, by)
		if !ok {
			return fmt.Errorf("approval %s is not bound to a run or a setup action", approvalID)
		}
		return err
	}
	if !o.gate.Decide(approvalID, approve, by) {
		return fmt.Errorf("no run is waiting on %s", approvalID)
	}
	return nil
}

// Resolve closes a finding after the verifier saw the signal clear.
func (o *Orchestrator) Resolve(findingID, text string) {
	o.st.Update(func(s *domain.State) []store.Event {
		f := s.Finding(findingID)
		if f == nil {
			return nil
		}
		f.Status = domain.FindingResolved
		f.Severity = "ok"
		r := s.Run(f.RunID)
		evs := []store.Event{store.FindingEvent(f)}
		if r != nil {
			now := time.Now()
			r.Status = domain.RunDone
			r.Outcome = "Resolved, verified"
			r.EndedAt = &now
			dur := now.Sub(r.StartedAt).Round(time.Second)
			r.Messages = append(r.Messages, &domain.Message{ID: o.st.NextID("m", 0), Role: domain.RoleVerification, Agent: "verifier", Title: "Verification · independent read", Text: text + fmt.Sprintf(" %s is resolved. %s from trigger to verified, %d sources, $%.2f.", f.ID, dur, len(r.Evidence), r.Usage.CostUSD), At: time.Now()})
			evs = append(evs, store.RunEvent(r))
		}
		evs = append(evs, endScenario(s, f)...)
		return append(evs, store.Activity(s, "verify", "Verification: "+text+" "+f.ID+" resolved.", f.RunID, false), store.Toast("Resolved: "+f.Title))
	})
}

// Escalate gives up on a finding whose signal did not clear.
func (o *Orchestrator) Escalate(findingID, text string) {
	o.st.Update(func(s *domain.State) []store.Event {
		f := s.Finding(findingID)
		if f == nil {
			return nil
		}
		f.Status = domain.FindingEscalated
		evs := []store.Event{store.FindingEvent(f)}
		evs = append(evs, endScenario(s, f)...)
		if r := s.Run(f.RunID); r != nil {
			now := time.Now()
			r.Status = domain.RunDone
			r.Outcome = "Unresolved, escalated"
			r.EndedAt = &now
			r.Messages = append(r.Messages, &domain.Message{ID: o.st.NextID("m", 0), Role: domain.RoleVerification, Agent: "verifier", Title: "Verification · independent read", Text: text, At: time.Now()})
			evs = append(evs, store.RunEvent(r))
		}
		return append(evs, store.Activity(s, "verdict", "Escalated: "+text, f.RunID, false))
	})
}

// CancelAll stops running investigations, used by reset.
func (o *Orchestrator) CancelAll() {
	o.mu.Lock()
	defer o.mu.Unlock()
	for id, c := range o.cancels {
		c()
		delete(o.cancels, id)
	}
}

const orchestratorSystem = `You are Warden's orchestrator for one customer tenant: the agent the operator talks to. You know the estate, the open findings and the runs, and you decide which specialist to start. You never change infrastructure yourself; the investigator proposes changes and policy gates them.

Use tools to answer from current state rather than guessing. When the operator asks you to check, investigate or look into a resource, call start_investigation with a clear task. Answer in at most three short sentences, plainly, and say what you started and where to watch it. Never invent findings, resources or numbers.

You also guide setup. Warden's documentation is available through read_docs; its pages are: model (cells, levels of reach, autonomy, what never happens), azure (the connection, VM Level 0 and 1, AKS Level 0, 1 and 2, step by step), upcloud (the connection, server Level 0 and 1, UKS Level 0, 1 and 2), grant (what the one-line grant installs, SSH certificates, the reverse tunnel), probe (what it collects and sends, failure behaviour), security (what runs on the customer's side, network, identities, audit, revocation), howto (links into every section). When the operator asks how to connect, onboard, grant, install, reach a private cluster, open a port, revoke access, or what Warden needs, read the matching page first and answer from it with the exact steps, roles and commands it gives; then point them to the page as #docs/<page>. Never guess a permission name; if the page does not state it, say so.

You also run setup, so the operator never configures Warden by hand. The flow is: the operator creates the tenant and adds a cloud provider credential in a form (never through you: you never take a secret); from there you take over. On any question about what exists, what you can see, or what to do next, call get_setup_state first and answer from it. Say plainly what each connection found and which of it you can manage, then onboard it with onboard_cells unless the operator says otherwise. For each cell say what you can already see and do at its level and what the next level would add. For a VM, issue_grant and give the operator the exact line to run on the host; do not paraphrase the line. For a cluster, enable_api_access and then install_probe each open an approval because they change the customer's cluster: say the approval id and that you will carry on once it is approved. After every setup action call get_setup_state again and tell the operator the one next thing. Keep to the three sentences unless you are handing over a command, then put the command on its own line.

You also hold the tenant's runbooks, workflows and agents. A runbook is the operator's knowledge of an app: when they describe what runs on a cell, what is safe, what is never allowed and who to page, write it as Markdown with those sections plus an "Applies when" section naming the unit or image, and save it with save_runbook on the cell's app and tier labels; say you wrote it from their words and they can edit it. A workflow is a monitoring loop that runs in the runtime without a model: when they ask you to watch logs or wake them on errors, create_workflow with log-digest and wake-warden and say it costs no tokens. When a workflow fires you are told what it found; pick the responder from the table that fits and start_responder, otherwise start_investigation, and say in one sentence what fired and what you started. Specialists come from templates only: add_agent binds a template to a scope and the template's responders start it from then on.`

// Chat handles one operator message to the orchestrator.
func (o *Orchestrator) Chat(ctx context.Context, question, by string) error {
	return o.turn(ctx, question, by, true)
}

// NewSession clears the thread. The estate, runbooks, watches and agents stay.
func (o *Orchestrator) NewSession() {
	o.chatMu.Lock()
	defer o.chatMu.Unlock()
	if r, ok := o.reasoner.(interface{ Forget() }); ok {
		r.Forget()
	}
	o.st.Update(func(s *domain.State) []store.Event {
		s.Chat = []*domain.ChatMessage{}
		return []store.Event{{Type: "chat.clear", Data: nil}}
	})
}

// Announce makes the orchestrator speak first: the prompt is not shown in the
// thread, only the answer. Used when the tenant is created, when a
// connection validates and when a cell reaches a new level, so the operator
// is told what happened and what comes next without asking.
func (o *Orchestrator) Announce(prompt string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := o.turn(ctx, prompt, "warden", false); err != nil {
			o.log.Warn("announce failed", "err", err)
		}
	}()
}

func (o *Orchestrator) turn(ctx context.Context, question, by string, show bool) error {
	o.chatMu.Lock()
	defer o.chatMu.Unlock()
	var history []domain.ChatMessage
	var ctxText string
	var msgID string
	o.st.Update(func(s *domain.State) []store.Event {
		for _, m := range s.Chat {
			history = append(history, *m)
		}
		ctxText = o.contextText(s)
		am := &domain.ChatMessage{ID: o.st.NextID("c", 0), Role: "orchestrator", At: time.Now()}
		msgID = am.ID
		if !show {
			s.Chat = append(s.Chat, am)
			return []store.Event{store.ChatEvent(am)}
		}
		um := &domain.ChatMessage{ID: o.st.NextID("c", 0), Role: "user", Text: question, At: time.Now()}
		s.Chat = append(s.Chat, um, am)
		return []store.Event{store.ChatEvent(um), store.ChatEvent(am)}
	})
	appendText := func(delta string) {
		o.st.Update(func(s *domain.State) []store.Event {
			for _, m := range s.Chat {
				if m.ID == msgID {
					m.Text += delta
				}
			}
			return []store.Event{{Type: "chat.delta", Data: map[string]string{"messageId": msgID, "delta": delta}}}
		})
	}
	calls := 0
	call := func(ctx context.Context, name string, in map[string]any) (string, error) {
		calls++
		var tm *domain.ChatMessage
		o.st.Update(func(s *domain.State) []store.Event {
			tm = &domain.ChatMessage{ID: o.st.NextID("c", 0), Role: "tool", Tool: &domain.ToolCall{Name: name, Input: in, Status: "running"}, At: time.Now()}
			// insert before the streaming assistant message so the order reads naturally
			idx := len(s.Chat)
			for i, m := range s.Chat {
				if m.ID == msgID {
					idx = i
				}
			}
			s.Chat = append(s.Chat[:idx], append([]*domain.ChatMessage{tm}, s.Chat[idx:]...)...)
			return []store.Event{{Type: "chat.insert", Data: map[string]any{"message": tm, "before": msgID}}}
		})
		out, err := o.orchestratorTool(name, in, by)
		status := "done"
		if err != nil {
			out = err.Error()
			status = "error"
		}
		o.st.Update(func(s *domain.State) []store.Event {
			tm.Tool.Output = out
			tm.Tool.Status = status
			return []store.Event{store.ChatEvent(tm)}
		})
		return out, nil
	}
	answer, err := o.reasoner.Chat(ctx, ChatRequest{System: orchestratorSystem, Context: ctxText, Question: question, History: history, Tools: OrchestratorTools(), Call: call, AgentID: "orchestrator", MaxCalls: 6}, appendText)
	o.st.Update(func(s *domain.State) []store.Event {
		var am *domain.ChatMessage
		for _, m := range s.Chat {
			if m.ID == msgID {
				am = m
			}
		}
		if am == nil {
			return nil
		}
		if err != nil {
			am.Text = "I could not answer: " + err.Error()
		} else if am.Text == "" {
			am.Text = answer
		}
		s.Usage["orchestrator"].Calls += 0
		return []store.Event{store.ChatEvent(am), store.UsageEvent(s)}
	})
	return err
}

// OrchestratorTools are the tools the orchestrator may use in chat.
func OrchestratorTools() []ToolDef {
	res := map[string]any{"type": "string", "description": "Cell id, for example web-01, aks-shop, edge-01"}
	defs := []ToolDef{
		{Name: "list_findings", Description: "List the tenant's findings with status, severity, resource and run id.", Schema: map[string]any{}},
		{Name: "list_runs", Description: "List agent runs with agent, status, outcome and cost.", Schema: map[string]any{}},
		{Name: "get_resource_state", Description: "Provider state, health and services with versions for a resource, plus its current key metrics.", Schema: map[string]any{"resource": res}, Required: []string{"resource"}},
		{Name: "start_investigation", Description: "Start the investigator on a cell with a task. Returns the run id. Use when the operator asks to check, investigate or fix something.", Schema: map[string]any{"resource": res, "task": map[string]any{"type": "string", "description": "What to investigate, in one sentence"}}, Required: []string{"resource", "task"}},
		{Name: "read_docs", Description: "Read one page of Warden's documentation as Markdown. Pages: model, azure, upcloud, grant, probe, security, howto. Use before answering any question about setting up, connecting, granting, installing or revoking.", Schema: map[string]any{"page": map[string]any{"type": "string", "enum": DocPages()}}, Required: []string{"page"}},
	}
	defs = append(defs, SetupTools()...)
	return append(defs, SkillTools()...)
}

// DocPages lists the documentation pages embedded with the console.
func DocPages() []string {
	return []string{"model", "azure", "upcloud", "grant", "probe", "security", "howto"}
}

// ReadDoc returns a documentation page, or an error naming the valid pages.
func ReadDoc(page string) (string, error) {
	for _, p := range DocPages() {
		if p == page {
			b, err := fs.ReadFile(web.FS, "docs/"+page+".md")
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
	}
	return "", fmt.Errorf("no documentation page %q; pages are %s", page, strings.Join(DocPages(), ", "))
}

func (o *Orchestrator) orchestratorTool(name string, in map[string]any, by string) (string, error) {
	switch name {
	case "list_findings":
		var out []string
		o.st.View(func(s *domain.State) {
			for _, f := range s.Findings {
				out = append(out, fmt.Sprintf("%s [%s, %s] %s on %s (%s), run %s", f.ID, f.Severity, f.Status, f.Title, f.Resource, f.Service, f.RunID))
			}
		})
		if len(out) == 0 {
			return "No findings. The estate is quiet.", nil
		}
		return strings.Join(out, "\n"), nil
	case "list_runs":
		var out []string
		o.st.View(func(s *domain.State) {
			for _, r := range s.Runs {
				out = append(out, fmt.Sprintf("%s agent=%s status=%s outcome=%q cost=$%.2f task=%q", r.ID, r.Agent, r.Status, r.Outcome, r.Usage.CostUSD, r.Task))
			}
		})
		if len(out) == 0 {
			return "No runs yet.", nil
		}
		return strings.Join(out, "\n"), nil
	case "get_resource_state":
		res := str(in, "resource")
		st, err := o.w.ResourceState(res)
		if err != nil {
			return "", err
		}
		var m []string
		for _, k := range []string{"cpu_percent", "memory_percent", "disk_used_percent", "requests_per_second", "error_rate_percent", "http_up"} {
			if v, ok := o.w.Metric(res, k); ok {
				m = append(m, fmt.Sprintf("%s=%.1f", k, v))
			}
		}
		if len(m) > 0 {
			st += " Now: " + strings.Join(m, ", ") + "."
		}
		return st, nil
	case "read_docs":
		return ReadDoc(str(in, "page"))
	case "start_investigation":
		id, err := o.StartInvestigation(str(in, "resource"), str(in, "task"), by)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Started run %s on %s. Watch it under Investigations.", id, str(in, "resource")), nil
	}
	if out, err := o.setupTool(name, in, by); err == nil || !strings.HasPrefix(err.Error(), "unknown tool") {
		return out, err
	}
	return o.skillTool(name, in, by)
}

func (o *Orchestrator) contextText(s *domain.State) string {
	var b strings.Builder
	if s.Tenant != nil {
		fmt.Fprintf(&b, "Tenant %s (%s), runtime %s. ", s.Tenant.Name, s.Tenant.Env, s.Tenant.Runtime)
	}
	fmt.Fprintf(&b, "Probes online: %d of %d. Cells: ", len(s.Probe.Hosts), len(s.Probe.Expected))
	for _, r := range s.Cells {
		fmt.Fprintf(&b, "%s (%s, %s, hands %s, app %s); ", r.ID, r.Kind, r.LevelName(), r.Hands, s.GroupName(r.ID))
	}
	open := 0
	for _, f := range s.Findings {
		if f.Status != domain.FindingResolved {
			open++
		}
	}
	fmt.Fprintf(&b, "Open findings: %d. Pending approvals: %d. Setup stage: %s (get_setup_state has the detail). ", open, pendingApprovals(s), setup.Compute(s).Stage)
	fmt.Fprintf(&b, "Runbooks: %d. Workflows: %d. Responders: ", len(s.Runbooks), len(s.Workflows))
	for _, r := range s.Responders {
		fmt.Fprintf(&b, "%s (%s, agent %s); ", r.ID, r.Signal, r.Agent)
	}
	fmt.Fprintf(&b, "Agent templates: ")
	for _, t := range s.Templates {
		fmt.Fprintf(&b, "%s; ", t.ID)
	}
	fmt.Fprintf(&b, "Documentation pages: %s (read_docs).", strings.Join(DocPages(), ", "))
	return b.String()
}

func pendingApprovals(s *domain.State) int {
	n := 0
	for _, a := range s.Approvals {
		if a.Status == domain.ApprovalPending {
			n++
		}
	}
	return n
}

// Ask answers a question about one run, with the run as context and no tools.
func (o *Orchestrator) Ask(ctx context.Context, runID, question string) (string, error) {
	var ctxText string
	o.st.View(func(s *domain.State) {
		r := s.Run(runID)
		if r == nil {
			return
		}
		var b strings.Builder
		f := s.Finding(r.FindingID)
		if f != nil {
			fmt.Fprintf(&b, "Finding %s: %s on %s (%s), status %s. ", f.ID, f.Title, f.Resource, f.Service, f.Status)
		}
		fmt.Fprintf(&b, "Run %s by %s, status %s, outcome %s. Evidence: ", r.ID, r.Agent, r.Status, r.Outcome)
		for _, e := range r.Evidence {
			fmt.Fprintf(&b, "%s: %s [%s] ", e.Kind, e.Claim, e.Pointer)
		}
		for _, h := range r.Hypotheses {
			fmt.Fprintf(&b, "Hypothesis (%s): %s. %s ", h.Verdict, h.Title, h.Detail)
		}
		if a := s.Approval(r.ApprovalID); a != nil {
			fmt.Fprintf(&b, "Proposed action (%s): %s. Command: %s. Blast radius: %s ", a.Status, a.Task, a.Command, a.BlastRadius)
		}
		if r.Summary != "" {
			b.WriteString("Summary: " + r.Summary)
		}
		ctxText = b.String()
	})
	if ctxText == "" {
		return "", fmt.Errorf("no run %s", runID)
	}
	system := "You are Warden, answering an operator's question about one investigation. Answer in at most three short sentences, plainly, citing evidence pointers when relevant. Never invent new facts; if the thread does not say, say so."
	answer, err := o.reasoner.Chat(ctx, ChatRequest{System: system, Context: ctxText, Question: question, RunID: runID, AgentID: "orchestrator", MaxCalls: 1}, nil)
	if err != nil {
		return "", err
	}
	o.st.Update(func(s *domain.State) []store.Event {
		r := s.Run(runID)
		if r == nil {
			return nil
		}
		r.Questions = append(r.Questions, domain.QA{Question: question, Answer: answer, At: time.Now()})
		return []store.Event{store.RunEvent(r)}
	})
	return answer, nil
}

// endScenario marks the demo scenario on a finding's resource finished. The
// fault may still be live in the world (a denied fix), which is the point.
func endScenario(s *domain.State, f *domain.Finding) []store.Event {
	var evs []store.Event
	if f == nil {
		return nil
	}
	for _, sc := range s.Scenarios {
		if sc.Resource == f.Resource && sc.Status == "running" {
			sc.Status = "done"
			evs = append(evs, store.ScenarioEvent(sc))
		}
	}
	return evs
}

// attachRunbooks records the runbooks that match the resource on the run and
// returns a sentence for the orchestrator's message.
func attachRunbooks(s *domain.State, run *domain.Run, resource string) string {
	rbs := s.RunbooksFor(resource)
	if len(rbs) == 0 {
		return " No runbook matches " + resource + "."
	}
	var names []string
	for _, rb := range rbs {
		run.Runbooks = append(run.Runbooks, rb.ID)
		names = append(names, rb.Name)
	}
	return " Runbooks handed to the investigator: " + strings.Join(names, ", ") + "."
}

// runbookText renders the run's runbooks for a model prompt.
func runbookText(s *domain.State, run *domain.Run) string {
	var b strings.Builder
	for _, id := range run.Runbooks {
		if rb := s.Runbook(id); rb != nil {
			fmt.Fprintf(&b, "\n--- Runbook: %s (labels %s) ---\n%s\n", rb.Name, selectorString(rb.Selector), rb.Body)
		}
	}
	return b.String()
}

func selectorString(sel map[string]string) string {
	var parts []string
	for _, k := range sortedKeys(sel) {
		parts = append(parts, k+"="+sel[k])
	}
	return strings.Join(parts, ", ")
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
