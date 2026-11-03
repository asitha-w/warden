package setup

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/asitha-w/warden/internal/domain"
	"github.com/asitha-w/warden/internal/store"
)

func fresh(t *testing.T) (*store.Store, *Service) {
	t.Helper()
	st := store.New(domain.NewState("test", "scripted"), slog.Default())
	s := New(st)
	s.Delay = 0
	return st, s
}

func connect(st *store.Store) string {
	var id string
	st.Update(func(s *domain.State) []store.Event {
		s.Tenant = &domain.Tenant{ID: "t-1", Name: "Shop", Runtime: "rt-1", EgressIP: "94.237.1.2", CreatedAt: time.Now()}
		c := &domain.Connection{ID: "conn-1", Name: "Azure", Provider: "azure", Status: domain.ConnConnected, ReadScope: domain.Scope{Level: domain.ScopeSubscription, Target: "sub"}, WriteScope: domain.Scope{Level: domain.ScopeNone}}
		domain.ValidateAzure(c)
		domain.Discover(c)
		s.Connections = append(s.Connections, c)
		id = c.ID
		return nil
	})
	return id
}

func TestOverviewWalksTheStages(t *testing.T) {
	st, s := fresh(t)
	if o := s.Overview(); o.Stage != StageTenant || o.Next[0].Kind != "tenant" {
		t.Fatalf("fresh state: %+v", o)
	}
	st.Update(func(d *domain.State) []store.Event {
		d.Tenant = &domain.Tenant{ID: "t-1", Name: "Shop", Runtime: "rt-1", EgressIP: "94.237.1.2"}
		return nil
	})
	if o := s.Overview(); o.Stage != StageConnection || o.Next[0].Kind != "connection" {
		t.Fatalf("tenant only: %+v", o)
	}
	id := connect(st)
	o := s.Overview()
	if o.Stage != StageOnboard || o.Next[0].Tool != "onboard_cells" || o.Connections[0].Manageable != 3 {
		t.Fatalf("connected: %+v", o)
	}
	added, err := s.Onboard(id, nil, "test")
	if err != nil || len(added) != 3 {
		t.Fatalf("onboard: %v %v", added, err)
	}
	o = s.Overview()
	if o.Stage != StageReach || len(o.Next) != 3 {
		t.Fatalf("after onboard: %+v", o)
	}
	for _, step := range o.Next {
		switch step.Target {
		case "web-01", "vm-staging-01":
			if step.Kind != "grant" || step.Tool != "issue_grant" || step.Who != WhoBoth {
				t.Fatalf("vm step: %+v", step)
			}
		case "aks-shop":
			if step.Kind != "api-access" || step.Tool != "enable_api_access" {
				t.Fatalf("cluster step: %+v", step)
			}
		}
	}
	if !strings.Contains(o.Text(), "Next steps, in order:") {
		t.Fatalf("text: %s", o.Text())
	}
}

func TestRaisesEndAtReady(t *testing.T) {
	st, s := fresh(t)
	id := connect(st)
	if _, err := s.Onboard(id, []string{"web-01", "aks-shop"}, "test"); err != nil {
		t.Fatal(err)
	}
	if o := s.Overview(); o.Stage != StageOnboard || o.Connections[0].Onboarded != 2 {
		t.Fatalf("one left to onboard: %+v", o)
	}
	if _, err := s.Onboard(id, nil, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Grant("vm-staging-01", false, "test"); err != nil {
		t.Fatal(err)
	}
	g, err := s.Grant("web-01", true, "test")
	if err != nil || !strings.HasPrefix(g.Token, "wgt_") {
		t.Fatalf("grant: %+v %v", g, err)
	}
	var line string
	st.View(func(d *domain.State) { line = GrantLine(d.Tenant, d.Cell("web-01")) })
	if !strings.Contains(line, "rt-1.warden.example/grant/web-01?t="+g.Token) || !strings.HasSuffix(line, "--tunnel") {
		t.Fatalf("line: %s", line)
	}
	if err := s.Probe("aks-shop", "test"); err == nil {
		t.Fatal("probe before API access must fail")
	}
	if err := s.APIAccess("aks-shop", "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.Probe("aks-shop", "test"); err != nil {
		t.Fatal(err)
	}
	o := s.Overview()
	if o.Stage != StageReady || len(o.Next) != 0 {
		t.Fatalf("expected ready: %+v", o)
	}
	st.View(func(d *domain.State) {
		if c := d.Cell("web-01"); c.Level != domain.LevelGranted || c.Hands != domain.HandsTunnel {
			t.Fatalf("web-01: %+v", c)
		}
		if c := d.Cell("aks-shop"); c.Level != domain.LevelProbe || c.Hands != domain.HandsKubeAPI {
			t.Fatalf("aks-shop: %+v", c)
		}
		if len(d.Probe.Hosts) != 3 {
			t.Fatalf("probe hosts: %v", d.Probe.Hosts)
		}
	})
}
