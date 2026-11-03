package signal

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/asitha-w/warden/internal/agent"
	"github.com/asitha-w/warden/internal/domain"
	"github.com/asitha-w/warden/internal/setup"
	"github.com/asitha-w/warden/internal/store"
	"github.com/asitha-w/warden/internal/world"
)

func TestLogWorkflowFiresOncePerSignature(t *testing.T) {
	st := store.New(domain.NewState("test", "scripted"), slog.Default())
	w := world.New()
	su := setup.New(st)
	su.Delay = 0
	orch := agent.NewOrchestrator(st, w, agent.NewGate(), agent.NewScripted(st, 0, "test"), su, "test", slog.Default())
	st.Update(func(s *domain.State) []store.Event {
		s.Tenant = &domain.Tenant{ID: "t-1", Name: "Shop", Runtime: "rt-1", EgressIP: "1.2.3.4"}
		c := &domain.Connection{ID: "conn-1", Name: "UpCloud", Provider: "upcloud", Status: domain.ConnConnected}
		domain.ValidateUpCloud(c)
		domain.Discover(c)
		s.Connections = append(s.Connections, c)
		s.Workflows = []*domain.Workflow{{ID: "wf-1", Name: "Logs on edge-01", Every: 1, Selector: map[string]string{"app": "edge", "tier": "proxy"}, Collect: "log-digest", Source: "*", Trigger: "responder:service-down", Enabled: true}}
		return nil
	})
	if _, err := su.Onboard("conn-1", []string{"edge-01"}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := su.Grant("edge-01", false, "test"); err != nil {
		t.Fatal(err)
	}
	l := New(st, w, orch, time.Second, slog.Default())
	l.runWorkflows() // baseline: cursor set, nothing counts yet
	l.last = map[string]time.Time{}
	l.runWorkflows() // quiet: nothing new
	var fired int
	st.View(func(s *domain.State) { fired = s.Workflows[0].Fired })
	if fired != 0 {
		t.Fatalf("fired on a quiet log: %d", fired)
	}
	// an error appears
	w.Hosts["edge-01"].Logs["/var/log/syslog"] = append(w.Hosts["edge-01"].Logs["/var/log/syslog"], "Nov 3 07:00:00 edge-01 systemd[1]: nginx.service: Failed with result 'signal'.")
	l.last = map[string]time.Time{}
	l.runWorkflows()
	st.View(func(s *domain.State) {
		if s.Workflows[0].Fired != 1 {
			t.Fatalf("expected one fire: %+v", s.Workflows[0])
		}
		if len(s.Findings) != 1 || s.Findings[0].ResponderID != "service-down" {
			t.Fatalf("expected a service-down finding: %+v", s.Findings)
		}
		if !strings.Contains(s.Activity[len(s.Activity)-1].Text, "nginx.service: Failed") && !strings.Contains(s.Runs[0].Messages[0].Text, "nginx.service: Failed") {
			t.Fatalf("signature missing from the signal text: %q", s.Runs[0].Messages[0].Text)
		}
	})
	// the same signature again does not fire twice
	w.Hosts["edge-01"].Logs["/var/log/syslog"] = append(w.Hosts["edge-01"].Logs["/var/log/syslog"], "Nov 3 07:00:05 edge-01 systemd[1]: nginx.service: Failed with result 'signal'.")
	l.last = map[string]time.Time{}
	l.runWorkflows()
	st.View(func(s *domain.State) {
		if s.Workflows[0].Fired != 1 {
			t.Fatalf("fired again on a known signature: %+v", s.Workflows[0])
		}
	})
	orch.CancelAll()
}

func TestSignatureStripsNoise(t *testing.T) {
	got := world.Signature("edge-01", "Nov 3 07:00:00 edge-01 kernel: Out of memory: Killed process 812 (nginx) total-vm:412344kB.")
	if got != "kernel: Out of memory: Killed process N (nginx) total-vm:NkB" {
		t.Fatalf("signature: %q", got)
	}
}
