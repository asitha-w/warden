package agent

import (
	"strings"
	"testing"

	"github.com/asitha-w/warden/internal/domain"
)

func TestSkillsWriteRunbooksWorkflowsAndAgents(t *testing.T) {
	st, o := setupOrchestrator(t)
	if _, err := o.orchestratorTool("onboard_cells", map[string]any{}, "ann"); err != nil {
		t.Fatal(err)
	}
	sc := NewScripted(st, 0, "test")
	name, sel, body := sc.composeRunbook("web-01", "web-01 runs nginx for the shop. Restarting nginx is safe. Never delete /var/log. Page platform@shop.example.")
	if name != "Shop on Azure VM" || sel != "app=shop, tier=web" {
		t.Fatalf("name %q selector %q", name, sel)
	}
	for _, want := range []string{"## Applies when", "unit nginx", "1. Restarting nginx is safe.", "- Never delete /var/log.", "- Page platform@shop.example."} {
		if !strings.Contains(body, want) {
			t.Fatalf("body misses %q:\n%s", want, body)
		}
	}
	out, err := o.orchestratorTool("save_runbook", map[string]any{"name": name, "selector": sel, "body": body}, "ann")
	if err != nil || !strings.Contains(out, "applies to 2 cell") {
		t.Fatalf("save_runbook: %q %v", out, err)
	}
	out, err = o.orchestratorTool("create_workflow", map[string]any{"name": "Logs on web-01", "cell": "web-01", "every_seconds": 10, "collect": "log-digest", "source": "*"}, "ann")
	if err != nil || !strings.Contains(out, "wake-warden") {
		t.Fatalf("create_workflow: %q %v", out, err)
	}
	if _, err := o.orchestratorTool("create_workflow", map[string]any{"cell": "web-01", "collect": "log-digest", "source": "*", "trigger": "responder:nope"}, "ann"); err == nil {
		t.Fatal("unknown responder must be refused")
	}
	out, err = o.orchestratorTool("add_agent", map[string]any{"template": "kubernetes-specialist", "scope": "app=shop"}, "ann")
	if err != nil || !strings.Contains(out, "release-regression") {
		t.Fatalf("add_agent: %q %v", out, err)
	}
	if _, err := o.orchestratorTool("add_agent", map[string]any{"template": "nope"}, "ann"); err == nil {
		t.Fatal("unknown template must be refused")
	}
	st.View(func(s *domain.State) {
		if r := s.Responder("release-regression"); r.Agent != "kubernetes-specialist" {
			t.Fatalf("responder not rebound: %+v", r)
		}
		if a := s.Agent("kubernetes-specialist"); a == nil || a.Scope["app"] != "shop" || s.Usage[a.ID] == nil {
			t.Fatalf("agent: %+v", a)
		}
		if len(s.Workflows) != 5 || s.Workflows[4].Trigger != "wake-warden" || s.Workflows[4].Every != 10 {
			t.Fatalf("workflow: %+v", s.Workflows[4])
		}
	})
	out, err = o.orchestratorTool("start_responder", map[string]any{"responder": "service-down", "resource": "web-01", "title": "nginx on web-01 is failing", "reason": "test"}, "ann")
	if err != nil || !strings.HasPrefix(out, "Started run") {
		t.Fatalf("start_responder: %q %v", out, err)
	}
	o.CancelAll()
}
