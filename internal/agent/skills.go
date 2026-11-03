package agent

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/asitha-w/warden/internal/domain"
	"github.com/asitha-w/warden/internal/store"
)

// HandleWorkflow is what happens when a workflow with the wake-warden trigger
// fires: the hit is recorded and Warden is asked to decide. The decision is a
// model turn in live mode and a keyword route in scripted mode; either way it
// ends in start_responder or start_investigation, which are deterministic.
func (o *Orchestrator) HandleWorkflow(wf domain.Workflow, resource, title, text string) {
	o.st.Update(func(s *domain.State) []store.Event {
		return []store.Event{store.Activity(s, "signal", fmt.Sprintf("Workflow %s (%s) fired on %s: %s", wf.ID, wf.Name, resource, text), "", false), store.Toast("Workflow fired: " + title)}
	})
	o.Announce(fmt.Sprintf("[announce] workflow %s fired on %s: %s\nDecide what to do. If a responder in the table fits, call start_responder with it and the resource; otherwise call start_investigation. Say in one sentence what fired and what you started.", wf.ID, resource, text))
}

// SkillTools are the orchestrator's tools for runbooks, workflows, agents and
// routing a workflow hit. None changes the customer's side; all are allowed.
func SkillTools() []ToolDef {
	cell := map[string]any{"type": "string", "description": "Cell id, for example edge-01"}
	return []ToolDef{
		{Name: "save_runbook", Description: "Create or update a runbook: tenant guidance the investigator follows on matching cells. Selector is labels like app=edge, tier=proxy. The body is Markdown with the sections: what runs here, applies when, health signals, known fixes in order, never, escalation.", Schema: map[string]any{"name": map[string]any{"type": "string"}, "selector": map[string]any{"type": "string", "description": "k=v, k=v"}, "body": map[string]any{"type": "string"}}, Required: []string{"name", "selector", "body"}},
		{Name: "create_workflow", Description: "Create a monitoring workflow: a deterministic loop in the runtime that costs no tokens. collect is log-digest (source: a log path or * for every log on the cell) or metric (source: metric name). It fires on a new error signature (log-digest) and triggers wake-warden, which asks you to decide, or responder:<id>.", Schema: map[string]any{"name": map[string]any{"type": "string"}, "cell": cell, "every_seconds": map[string]any{"type": "integer"}, "collect": map[string]any{"type": "string", "enum": []string{"log-digest", "metric"}}, "source": map[string]any{"type": "string"}, "trigger": map[string]any{"type": "string", "description": "wake-warden or responder:<id>"}}, Required: []string{"name", "cell", "collect", "source"}},
		{Name: "add_agent", Description: "Add a specialist agent from a template and bind it to a scope (labels like app=edge). The responders the template names start it instead of the investigator for cells in scope. Templates: see get_setup_state or list_templates.", Schema: map[string]any{"template": map[string]any{"type": "string"}, "scope": map[string]any{"type": "string", "description": "k=v, k=v; empty for every cell"}}, Required: []string{"template"}},
		{Name: "list_templates", Description: "The agent templates Warden can add, with what they do and which responders they take over.", Schema: map[string]any{}},
		{Name: "start_responder", Description: "Route a workflow hit to a responder from the table: opens a finding and starts the responder's agent on the resource under the responder's policy. Use after a workflow fired when a responder fits.", Schema: map[string]any{"responder": map[string]any{"type": "string"}, "resource": cell, "title": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}, Required: []string{"responder", "resource", "title"}},
	}
}

func parseSelector(text string) map[string]string {
	out := map[string]string{}
	for _, p := range strings.Split(text, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
		if ok && strings.TrimSpace(k) != "" {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

func selectorText(sel map[string]string) string {
	keys := make([]string, 0, len(sel))
	for k := range sel {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+sel[k])
	}
	return strings.Join(parts, ", ")
}

// CellSelector is the selector Warden uses for a cell: its app and tier labels.
func CellSelector(c *domain.Cell) map[string]string {
	sel := map[string]string{}
	for _, k := range []string{"app", "tier"} {
		if v := c.Labels[k]; v != "" {
			sel[k] = v
		}
	}
	return sel
}

func (o *Orchestrator) skillTool(name string, in map[string]any, by string) (string, error) {
	switch name {
	case "save_runbook":
		nm := strings.TrimSpace(str(in, "name"))
		sel := parseSelector(str(in, "selector"))
		body := strings.TrimSpace(str(in, "body"))
		if nm == "" || len(sel) == 0 || body == "" {
			return "", fmt.Errorf("a runbook needs a name, a selector and a body")
		}
		id := strings.ToLower(strings.ReplaceAll(nm, " ", "-"))
		var n int
		o.st.Update(func(s *domain.State) []store.Event {
			rb := s.Runbook(id)
			if rb == nil {
				rb = &domain.Runbook{ID: id}
				s.Runbooks = append(s.Runbooks, rb)
			}
			rb.Name, rb.Selector, rb.Body, rb.UpdatedAt, rb.UpdatedBy = nm, sel, body, time.Now(), by
			for _, c := range s.Cells {
				if rb.Matches(c) {
					n++
				}
			}
			return []store.Event{{Type: "runbooks", Data: s.Runbooks}, store.Activity(s, "run", fmt.Sprintf("Warden saved runbook %q (%s) for %s.", nm, selectorText(sel), by), "", false)}
		})
		return fmt.Sprintf("Saved runbook %s (%s). It applies to %d cell(s) now. The operator can edit it under Runbooks.", id, selectorText(sel), n), nil
	case "create_workflow":
		cellID := str(in, "cell")
		every := num(in, "every_seconds", 60)
		if every < 5 {
			every = 5
		}
		collect := str(in, "collect")
		source := str(in, "source")
		trigger := str(in, "trigger")
		if trigger == "" {
			trigger = "wake-warden"
		}
		if collect != "log-digest" && collect != "metric" {
			return "", fmt.Errorf("collect must be log-digest or metric")
		}
		if collect == "metric" {
			return "", fmt.Errorf("metric workflows are the seeded ones for now; a log-digest workflow can be created")
		}
		var wf *domain.Workflow
		var err error
		o.st.Update(func(s *domain.State) []store.Event {
			c := s.Cell(cellID)
			if c == nil {
				err = fmt.Errorf("no cell %s", cellID)
				return nil
			}
			sel := CellSelector(c)
			if len(sel) == 0 {
				err = fmt.Errorf("%s has no app or tier label to select on", cellID)
				return nil
			}
			if trigger != "wake-warden" && (!strings.HasPrefix(trigger, "responder:") || s.Responder(strings.TrimPrefix(trigger, "responder:")) == nil) {
				err = fmt.Errorf("trigger must be wake-warden or responder:<id> from the table")
				return nil
			}
			wf = &domain.Workflow{ID: o.st.NextID("wf", 0), Name: strings.TrimSpace(str(in, "name")), Every: every, Selector: sel, Collect: collect, Source: source, Condition: "a new error signature in what was written since the last run", Trigger: trigger, Enabled: true, CreatedBy: by, CreatedAt: time.Now()}
			if wf.Name == "" {
				wf.Name = "Logs on " + cellID
			}
			s.Workflows = append(s.Workflows, wf)
			return []store.Event{{Type: "workflow", Data: wf}, store.Activity(s, "run", fmt.Sprintf("Warden created workflow %s (%s): every %ds, %s of %s on %s, trigger %s.", wf.ID, wf.Name, every, collect, source, selectorText(sel), trigger), "", false)}
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Created workflow %s (%s): every %d seconds, %s of %s on cells matching %s, fires on a new error signature, trigger %s. It runs in the runtime and costs no tokens; a model runs only when it fires.", wf.ID, wf.Name, every, collect, source, selectorText(wf.Selector), trigger), nil
	case "list_templates":
		var out []string
		o.st.View(func(s *domain.State) {
			for _, t := range s.Templates {
				out = append(out, fmt.Sprintf("%s: %s. %s Takes over responders: %s.", t.ID, t.Role, t.Description, strings.Join(t.Responders, ", ")))
			}
		})
		return strings.Join(out, "\n"), nil
	case "add_agent":
		tid := str(in, "template")
		scope := parseSelector(str(in, "scope"))
		var err error
		var text string
		o.st.Update(func(s *domain.State) []store.Event {
			t := s.Template(tid)
			if t == nil {
				err = fmt.Errorf("no template %s; use list_templates", tid)
				return nil
			}
			if s.Agent(tid) != nil && s.Agent(tid).Template != "" {
				err = fmt.Errorf("agent %s already exists", tid)
				return nil
			}
			a := &domain.AgentDef{ID: tid, Name: t.Name, Kind: domain.AgentSpecialist, Role: t.Role, Description: t.Description, Model: s.Model, Effort: t.Effort, Tools: t.Tools, Enabled: true, MaxToolCalls: t.MaxToolCalls, Template: t.ID, Scope: scope, AddedBy: by}
			replaced := false
			for i, x := range s.Agents {
				if x.ID == tid {
					s.Agents[i] = a
					replaced = true
				}
			}
			if !replaced {
				s.Agents = append(s.Agents, a)
			}
			if s.Usage[tid] == nil {
				s.Usage[tid] = &domain.Usage{}
			}
			var took []string
			evs := []store.Event{{Type: "agent", Data: a}, store.UsageEvent(s)}
			for _, rid := range t.Responders {
				if r := s.Responder(rid); r != nil {
					r.Agent = tid
					took = append(took, rid)
					evs = append(evs, store.Event{Type: "responder", Data: r})
				}
			}
			n := 0
			for _, c := range s.Cells {
				if len(scope) == 0 || selectorMatchesCell(scope, c) {
					n++
				}
			}
			text = fmt.Sprintf("Added %s from template %s with scope %s (%d cell(s)). Responders %s now start it instead of the investigator.", a.Name, t.ID, selectorOrAll(scope), n, strings.Join(took, ", "))
			evs = append(evs, store.Activity(s, "run", "Warden "+strings.ToLower(text[:1])+text[1:], "", false))
			return evs
		})
		if err != nil {
			return "", err
		}
		return text, nil
	case "start_responder":
		rid := str(in, "responder")
		res := str(in, "resource")
		title := str(in, "title")
		reason := str(in, "reason")
		var exists bool
		o.st.View(func(s *domain.State) { exists = s.Responder(rid) != nil && s.Cell(res) != nil })
		if !exists {
			return "", fmt.Errorf("responder %s or cell %s not found", rid, res)
		}
		id := o.HandleSignal(Signal{ResponderID: rid, Resource: res, Title: title, Severity: "critical", Text: "Warden: " + reason})
		if id == "" {
			return fmt.Sprintf("A finding for %s on %s is already open; nothing new started.", rid, res), nil
		}
		return fmt.Sprintf("Started run %s on %s under responder %s. Watch it under Investigations.", id, res, rid), nil
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

func selectorMatchesCell(sel map[string]string, c *domain.Cell) bool {
	for k, v := range sel {
		if c.Labels[k] != v {
			return false
		}
	}
	return true
}

func selectorOrAll(sel map[string]string) string {
	if len(sel) == 0 {
		return "every cell"
	}
	return selectorText(sel)
}
