package agent

import (
	"testing"

	"github.com/asitha-w/warden/internal/domain"
)

func TestPolicyRespectsModeAndUnknownActions(t *testing.T) {
	r := &domain.Responder{ID: "r", Mode: "Review", Tools: []domain.ToolPolicy{{Tool: "run_command", Policy: domain.PolicyAsk}, {Tool: "restart_service", Policy: domain.PolicyDeny}}}
	tb := &Toolbox{responder: r}
	if got := tb.Policy("run_command"); got != domain.PolicyAsk {
		t.Fatalf("review mode should ask, got %s", got)
	}
	r.Mode = "Autonomous"
	if got := tb.Policy("run_command"); got != domain.PolicyAllow {
		t.Fatalf("autonomous mode should allow an ask action, got %s", got)
	}
	if got := tb.Policy("restart_service"); got != domain.PolicyDeny {
		t.Fatalf("deny stays deny in autonomous mode, got %s", got)
	}
	if got := tb.Policy("delete_everything"); got != domain.PolicyDeny {
		t.Fatalf("unknown actions are denied, got %s", got)
	}
}

func TestCostUsesModelPrices(t *testing.T) {
	if c := Cost("claude-opus-5-5", 1_000_000, 0); c != 4 {
		t.Fatalf("expected $4 per million input tokens, got %v", c)
	}
	if c := Cost("unknown-model", 0, 1_000_000); c != 20 {
		t.Fatalf("unknown models fall back to the Opus rate, got %v", c)
	}
}
