package world

import (
	"strings"
	"testing"
)

func TestQueryMetricsSummarisesSeries(t *testing.T) {
	w := New()
	for i := 0; i < 20; i++ {
		w.Tick()
	}
	out, err := w.QueryMetrics("web-01", "disk_used_percent", 60)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "now 71") || !strings.Contains(out, "trend flat") {
		t.Fatalf("unexpected summary: %s", out)
	}
	if _, err := w.QueryMetrics("web-01", "nope", 60); err == nil || !strings.Contains(err.Error(), "available") {
		t.Fatalf("expected a helpful error, got %v", err)
	}
}

func TestStoppedHostHasNoMetrics(t *testing.T) {
	w := New()
	w.Hosts["edge-01"].State = "stopped"
	w.Tick()
	if _, ok := w.Metric("edge-01", "cpu_percent"); ok {
		t.Fatal("stopped host should report no metrics")
	}
	if _, err := w.TailLogs("edge-01", "/var/log/syslog", 10); err == nil {
		t.Fatal("probe cannot read logs on a stopped host")
	}
}

func TestExecuteRollbackAndDeny(t *testing.T) {
	w := New()
	w.Hosts["aks-shop"].Services["shop-api"].PrevVersion = "v2.3.9"
	out, err := w.Execute(Action{Name: "rollout_undo", Args: map[string]any{"resource": "aks-shop", "service": "shop-api"}})
	if err != nil || !strings.Contains(out, "v2.3.9") {
		t.Fatalf("rollback failed: %v %s", err, out)
	}
	if _, err := w.Execute(Action{Name: "stop_server", Args: map[string]any{"resource": "edge-01"}}); err == nil {
		t.Fatal("stop_server must be refused")
	}
}
