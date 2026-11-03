package agent

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/asitha-w/warden/internal/domain"
	"github.com/asitha-w/warden/internal/setup"
	"github.com/asitha-w/warden/internal/store"
	"github.com/asitha-w/warden/internal/world"
)

func setupOrchestrator(t *testing.T) (*store.Store, *Orchestrator) {
	t.Helper()
	st := store.New(domain.NewState("test", "scripted"), slog.Default())
	su := setup.New(st)
	su.Delay = 0
	o := NewOrchestrator(st, world.New(), NewGate(), NewScripted(st, 0, "test"), su, "test", slog.Default())
	st.Update(func(s *domain.State) []store.Event {
		s.Tenant = &domain.Tenant{ID: "t-1", Name: "Shop", Runtime: "rt-1", EgressIP: "94.237.1.2", CreatedAt: time.Now()}
		c := &domain.Connection{ID: "conn-1", Name: "Azure", Provider: "azure", Status: domain.ConnConnected, ReadScope: domain.Scope{Level: domain.ScopeSubscription, Target: "sub"}, WriteScope: domain.Scope{Level: domain.ScopeNone}}
		domain.ValidateAzure(c)
		domain.Discover(c)
		s.Connections = append(s.Connections, c)
		return nil
	})
	return st, o
}

func TestSetupToolsFollowThePolicyTable(t *testing.T) {
	st, o := setupOrchestrator(t)
	out, err := o.orchestratorTool("get_setup_state", nil, "ann")
	if err != nil || !strings.Contains(out, "Setup stage: onboard") {
		t.Fatalf("state: %q %v", out, err)
	}
	out, err = o.orchestratorTool("onboard_cells", map[string]any{}, "ann")
	if err != nil || !strings.Contains(out, "aks-shop") {
		t.Fatalf("onboard: %q %v", out, err)
	}
	out, err = o.orchestratorTool("issue_grant", map[string]any{"cell": "web-01"}, "ann")
	if err != nil || !strings.Contains(out, "curl -fsSL \"https://rt-1.warden.example/grant/web-01?t=wgt_") {
		t.Fatalf("grant: %q %v", out, err)
	}
	// a cluster change asks: an approval appears, nothing changes yet
	out, err = o.orchestratorTool("enable_api_access", map[string]any{"cell": "aks-shop"}, "ann")
	if err != nil || !strings.Contains(out, "Approval A-") {
		t.Fatalf("api access: %q %v", out, err)
	}
	if _, err := o.orchestratorTool("enable_api_access", map[string]any{"cell": "aks-shop"}, "ann"); err == nil || !strings.Contains(err.Error(), "already pending") {
		t.Fatalf("second ask should report the pending approval: %v", err)
	}
	var apID string
	st.View(func(s *domain.State) {
		if c := s.Cell("aks-shop"); c.Level != domain.LevelConnected {
			t.Fatalf("cluster changed before approval: %+v", c)
		}
		apID = s.Approvals[0].ID
		if s.Approvals[0].RunID != "" || s.Approvals[0].Action != "enable_api_access" {
			t.Fatalf("approval: %+v", s.Approvals[0])
		}
	})
	if err := o.Decide(apID, true, "ann"); err != nil {
		t.Fatal(err)
	}
	st.View(func(s *domain.State) {
		if c := s.Cell("aks-shop"); c.Level != domain.LevelAPI || c.Hands != domain.HandsKubeAPI {
			t.Fatalf("cluster after approval: %+v", c)
		}
		if s.Approvals[0].Status != domain.ApprovalApproved {
			t.Fatalf("approval not marked: %+v", s.Approvals[0])
		}
	})
	// denial leaves the cell alone
	if _, err := o.orchestratorTool("install_probe", map[string]any{"cell": "aks-shop"}, "ann"); err != nil {
		t.Fatal(err)
	}
	st.View(func(s *domain.State) { apID = s.Approvals[1].ID })
	if err := o.Decide(apID, false, "ann"); err != nil {
		t.Fatal(err)
	}
	st.View(func(s *domain.State) {
		if c := s.Cell("aks-shop"); c.HasProbe() {
			t.Fatalf("probe installed after denial: %+v", c)
		}
	})
	if p := SetupPolicy("run_command"); p != domain.PolicyDeny {
		t.Fatalf("unknown setup action must be denied, got %s", p)
	}
}
