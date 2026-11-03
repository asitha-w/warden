package scenario

import (
	"testing"

	"github.com/asitha-w/warden/internal/world"
)

func TestDiskFillRampsAndRemedies(t *testing.T) {
	w := world.New()
	w.Inject(Catalog()["disk-fill"].NewFault())
	for i := 0; i < 15; i++ {
		w.Tick()
	}
	if v, _ := w.Metric("web-01", "disk_used_percent"); v < 90 {
		t.Fatalf("disk should be above 90 after the ramp, got %.1f", v)
	}
	if _, err := w.Execute(world.Action{Name: "run_command", Args: map[string]any{"resource": "web-01", "command": "logrotate --force /etc/logrotate.d/nginx"}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		w.Tick()
	}
	if v, _ := w.Metric("web-01", "disk_used_percent"); v >= 90 {
		t.Fatalf("disk should fall after the fix, got %.1f", v)
	}
}

func TestEveryScenarioHasAPlaybookEndingInConclude(t *testing.T) {
	for id, def := range Catalog() {
		if len(def.Playbook) == 0 {
			t.Fatalf("%s has no playbook", id)
		}
		last := def.Playbook[len(def.Playbook)-1]
		if last.Tool != "conclude" {
			t.Fatalf("%s playbook must end with conclude, ends with %q", id, last.Tool)
		}
	}
}
