// Package scenario defines the faults the demo injects and the scripted
// playbook an offline investigator follows for each. Fault methods run with
// the world lock held, so they touch w.Hosts directly and never call the
// world's locking methods.
package scenario

import (
	"fmt"
	"strings"
	"time"

	"github.com/asitha-w/warden/internal/world"
)

// Step is one move in a scripted playbook: say something, call a tool, or both.
type Step struct {
	Say   string
	Tool  string
	Input map[string]any
}

// Definition ties a scenario id to its fault and its scripted playbook.
type Definition struct {
	ID        string
	Resource  string // the cell the fault lands on
	Responder string // the responder whose playbook this is
	NewFault  func() world.Fault
	Playbook  []Step
	// Chain is the causal chain the scripted investigator records.
	Chain []map[string]string
}

// Catalog returns every scenario by id.
func Catalog() map[string]Definition {
	return map[string]Definition{
		"disk-fill":      diskFill("web-01"),
		"disk-fill-edge": diskFill("edge-01"),
		"bad-release":    badRelease(),
		"service-crash":  serviceCrash(),
		"server-stopped": serverStopped(),
	}
}

func stamp(d time.Duration) string { return time.Now().Add(d).Format("15:04") }

// --- disk fill: a deploy turns nginx debug on while logrotate is stalled ---

type diskFillFault struct {
	host  string
	fixed bool
}

func (f *diskFillFault) ID() string       { return diskFillID(f.host) }
func (f *diskFillFault) Resource() string { return f.host }
func (f *diskFillFault) Fixed() bool      { return f.fixed }
func (f *diskFillFault) Start(w *world.World) {
	h := w.Hosts[f.host]
	h.Activity = append(h.Activity, world.ActivityEntry{At: time.Now().Add(-42 * time.Minute), Actor: "ci-deploy", Text: "Deploy: set NGINX_DEBUG=1 in /etc/nginx/env, nginx -s reload"})
	h.Files["/etc/nginx/env"] = "NGINX_DEBUG=1\n"
	h.Files["/var/lib/logrotate/status"] = "logrotate state -- version 2\n\"/var/log/nginx/access.log\" 2026-10-21-01:00:00\n# last run exited 1: error: stat of /var/log/nginx/access.log.1 failed\n"
	h.Metrics["log_growth_mb_per_min"] = 80
	h.Metrics["disk_used_percent"] = max(h.Metrics["disk_used_percent"], 86) // forty minutes of debug logging already landed
	for i := 0; i < 12; i++ {
		h.Logs["/var/log/nginx/access.log"] = append(h.Logs["/var/log/nginx/access.log"], fmt.Sprintf("10.0.1.%d - - [%s] \"GET /api/cart HTTP/1.1\" 200 1832 debug: upstream=10.0.2.4:8080 rt=0.012 ua=0.011 cs=hit vars={...3 KB of request variables...}", 20+i, time.Now().Add(-time.Duration(12-i)*time.Second).Format("02/Jan/2006:15:04:05 -0700")))
	}
	h.Logs["/var/log/syslog"] = append(h.Logs["/var/log/syslog"], time.Now().Add(-40*time.Minute).Format("Jan 2 15:04:05")+" "+f.host+" logrotate[2211]: error: stat of /var/log/nginx/access.log.1 failed: Permission denied")
}
func (f *diskFillFault) Tick(w *world.World) {
	h := w.Hosts[f.host]
	if !f.fixed {
		h.Metrics["disk_used_percent"] = min(94, h.Metrics["disk_used_percent"]+2.1)
	} else {
		h.Metrics["disk_used_percent"] = max(61, h.Metrics["disk_used_percent"]-2.3)
		h.Metrics["log_growth_mb_per_min"] = 2
	}
}
func (f *diskFillFault) Remedy(w *world.World, a world.Action) (bool, string) {
	if r, _ := a.Args["resource"].(string); r != f.host {
		return false, ""
	}
	cmd, _ := a.Args["command"].(string)
	if a.Name != "rotate_logs" && !(a.Name == "run_command" && strings.Contains(cmd, "logrotate")) {
		return false, ""
	}
	f.fixed = true
	h := w.Hosts[f.host]
	h.Files["/etc/nginx/env"] = "NGINX_DEBUG=0\n"
	h.Files["/var/lib/logrotate/status"] = "logrotate state -- version 2\n\"/var/log/nginx/access.log\" " + time.Now().Format("2006-01-02-15:04:05") + "\n"
	return true, "stdout: rotating /var/log/nginx/access.log ... done (3.1 GB compressed)\nnginx: signal process started"
}

func diskFillID(host string) string {
	if host == "web-01" {
		return "disk-fill"
	}
	return "disk-fill-edge"
}

func diskFill(host string) Definition {
	platform := "Resource Health reports Available. No platform event."
	ptr := "Resource Health · " + host
	rps := "310"
	if host != "web-01" {
		platform = "UpCloud reports the server started and no maintenance event."
		ptr = "Server state · " + host
		rps = "140"
	}
	return Definition{
		ID:        diskFillID(host),
		Resource:  host,
		Responder: "disk-pressure-linux",
		NewFault:  func() world.Fault { return &diskFillFault{host: host} },
		Playbook: []Step{
			{Say: "Disk pressure on " + host + ". I will read the disk series, the nginx logs, the activity and the logrotate state before forming a cause."},
			{Tool: "read_metrics", Input: map[string]any{"resource": host, "metric": "disk_used_percent", "window_minutes": 120}},
			{Tool: "tail_logs", Input: map[string]any{"resource": host, "path": "/var/log/nginx/access.log", "lines": 20}},
			{Tool: "get_activity", Input: map[string]any{"resource": host}},
			{Tool: "read_file", Input: map[string]any{"resource": host, "path": "/var/lib/logrotate/status"}},
			{Tool: "read_metrics", Input: map[string]any{"resource": host, "metric": "requests_per_second", "window_minutes": 120}},
			{Tool: "get_resource_state", Input: map[string]any{"resource": host}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "metric", "claim": "Disk on / is above 90% and rising about 2 points a sample.", "pointer": "disk_used_percent · " + host}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "log", "claim": "Every access log line carries a debug variable dump of about 3 KB.", "pointer": "/var/log/nginx/access.log · " + host}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "activity", "claim": "A deploy by ci-deploy set NGINX_DEBUG=1 42 minutes ago.", "pointer": "Activity · " + host}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "config", "claim": "logrotate has not completed since 2026-10-21; last run exited 1.", "pointer": "/var/lib/logrotate/status · " + host}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "metric", "claim": "Request rate is flat around " + rps + " rps. No traffic spike.", "pointer": "requests_per_second · " + host}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "resource", "claim": platform, "pointer": ptr}},
			{Tool: "record_hypothesis", Input: map[string]any{"title": "Debug logging enabled by the deploy, and logrotate stalled", "detail": "Supported by the activity, the per-line debug dump in the access log and the logrotate status file.", "verdict": "supported"}},
			{Tool: "record_hypothesis", Input: map[string]any{"title": "Traffic spike", "detail": "Rejected: request rate is flat across the window.", "verdict": "rejected"}},
			{Say: "Cause: the deploy turned on NGINX_DEBUG and logrotate has been failing, so the access log is filling the disk. Rotating the logs is the catalogue action for this; it also turns debug off and reloads nginx."},
			{Tool: "propose_action", Input: map[string]any{"action": "rotate_logs", "resource": host, "summary": "Rotate the nginx logs now and turn debug logging off", "blast_radius": "Rotates and compresses the access log and turns debug logging off. nginx reloads without dropping connections. About 3 GB is compressed; nothing is deleted."}},
			{Tool: "conclude", Input: map[string]any{"summary": "Disk on " + host + " filled because a deploy enabled nginx debug logging while logrotate was failing. Fix rotates the logs and disables debug.", "cause_verified": true}},
		},
		Chain: []map[string]string{
			{"time": stamp(-42 * time.Minute), "text": "Deploy sets NGINX_DEBUG=1"},
			{"time": stamp(-40 * time.Minute), "text": "Access log grows 80 MB/min"},
			{"time": stamp(-12 * time.Minute), "text": "logrotate stalled, disk at 90%", "severity": "warning"},
			{"time": stamp(0), "text": "Disk at 94%, writes slowing", "severity": "critical"},
		},
	}
}

// --- bad release of shop-api on aks-shop ---

type badReleaseFault struct{ fixed bool }

func (f *badReleaseFault) ID() string       { return "bad-release" }
func (f *badReleaseFault) Resource() string { return "aks-shop" }
func (f *badReleaseFault) Fixed() bool      { return f.fixed }
func (f *badReleaseFault) Start(w *world.World) {
	h := w.Hosts["aks-shop"]
	s := h.Services["shop-api"]
	s.PrevVersion, s.Version = s.Version, "v2.4.1"
	h.Activity = append(h.Activity, world.ActivityEntry{At: time.Now().Add(-9 * time.Minute), Actor: "ci-deploy", Text: "Rollout shop-api v2.4.1 started (image shop-api:2.4.1, 3 replicas)"})
	h.Activity = append(h.Activity, world.ActivityEntry{At: time.Now().Add(-7 * time.Minute), Actor: "ci-deploy", Text: "Rollout shop-api v2.4.1 completed, 3/3 replicas ready"})
	for i := 0; i < 10; i++ {
		h.Logs["shop-api"] = append(h.Logs["shop-api"], fmt.Sprintf("%s shop-api-7d9f-%d ERROR CheckoutHandler: runtime error: invalid memory address or nil pointer dereference at internal/cart/price.go:214 (discount=nil) request_id=%06d", time.Now().Add(-time.Duration(10-i)*20*time.Second).Format("15:04:05"), i%3, 418200+i*7))
	}
	h.Logs["ingress-nginx"] = append(h.Logs["ingress-nginx"], time.Now().Add(-6*time.Minute).Format("15:04:05")+" upstream shop-api: 502 Bad Gateway rate 17.9% (last 1m)")
}
func (f *badReleaseFault) Tick(w *world.World) {
	h := w.Hosts["aks-shop"]
	if !f.fixed {
		h.Metrics["error_rate_percent"] = min(18, h.Metrics["error_rate_percent"]+1.5)
		h.Metrics["p95_latency_ms"] = min(900, h.Metrics["p95_latency_ms"]+60)
	} else {
		h.Metrics["error_rate_percent"] = max(0.3, h.Metrics["error_rate_percent"]-2.5)
		h.Metrics["p95_latency_ms"] = max(120, h.Metrics["p95_latency_ms"]-110)
	}
}
func (f *badReleaseFault) Remedy(w *world.World, a world.Action) (bool, string) {
	if a.Name != "rollout_undo" {
		return false, ""
	}
	if s, _ := a.Args["service"].(string); s != "shop-api" {
		return false, ""
	}
	f.fixed = true
	return true, "Error rate on the upstream is falling."
}

func badRelease() Definition {
	return Definition{
		ID:       "bad-release",
		NewFault: func() world.Fault { return &badReleaseFault{} },
		Playbook: []Step{
			{Say: "Error rate on aks-shop crossed 5%. I will check the error series against the rollout history and read the shop-api logs."},
			{Tool: "read_metrics", Input: map[string]any{"resource": "aks-shop", "metric": "error_rate_percent", "window_minutes": 60}},
			{Tool: "get_activity", Input: map[string]any{"resource": "aks-shop"}},
			{Tool: "tail_logs", Input: map[string]any{"resource": "aks-shop", "path": "shop-api", "lines": 20}},
			{Tool: "read_metrics", Input: map[string]any{"resource": "aks-shop", "metric": "p95_latency_ms", "window_minutes": 60}},
			{Tool: "get_resource_state", Input: map[string]any{"resource": "aks-shop"}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "metric", "claim": "Error rate rose from 0.3% to about 18% and is still rising.", "pointer": "error_rate_percent · aks-shop"}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "activity", "claim": "Rollout shop-api v2.4.1 completed 7 minutes ago; the previous revision is v2.4.0.", "pointer": "Activity Log · aks-shop"}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "log", "claim": "shop-api pods log a nil pointer dereference in CheckoutHandler at price.go:214 on every failing request.", "pointer": "shop-api · pod logs"}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "metric", "claim": "p95 latency rose from 120 ms to about 900 ms in the same window.", "pointer": "p95_latency_ms · aks-shop"}},
			{Tool: "record_hypothesis", Input: map[string]any{"title": "Regression in shop-api v2.4.1", "detail": "The error rate starts with the rollout and the stack trace points at new code. Rolling back to v2.4.0 removes the cause.", "verdict": "supported"}},
			{Tool: "record_hypothesis", Input: map[string]any{"title": "Node or ingress fault", "detail": "Rejected: node CPU and memory are normal and ingress reports the upstream, not itself, as failing.", "verdict": "rejected"}},
			{Say: "Cause: shop-api v2.4.1 crashes on checkout. The safe first-line fix is to roll the deployment back to v2.4.0 and hand the trace to the owning team."},
			{Tool: "propose_action", Input: map[string]any{"action": "rollout_undo", "resource": "aks-shop", "service": "shop-api", "summary": "Roll shop-api back to v2.4.0", "blast_radius": "Kubernetes replaces the three replicas one at a time with the previous image. No data change. Checkout recovers within a minute; v2.4.1 stays available to redeploy once fixed."}},
			{Tool: "conclude", Input: map[string]any{"summary": "shop-api v2.4.1 introduced a nil pointer in checkout pricing. Rolled back to v2.4.0.", "cause_verified": true}},
		},
		Chain: []map[string]string{
			{"time": stamp(-9 * time.Minute), "text": "Rollout shop-api v2.4.1"},
			{"time": stamp(-7 * time.Minute), "text": "Checkout requests hit nil discount", "severity": "warning"},
			{"time": stamp(-6 * time.Minute), "text": "Ingress sees 502 on upstream"},
			{"time": stamp(0), "text": "Error rate 18%, p95 900 ms", "severity": "critical"},
		},
	}
}

// --- nginx crash on edge-01 ---

type serviceCrashFault struct{ fixed bool }

func (f *serviceCrashFault) ID() string       { return "service-crash" }
func (f *serviceCrashFault) Resource() string { return "edge-01" }
func (f *serviceCrashFault) Fixed() bool      { return f.fixed }
func (f *serviceCrashFault) Start(w *world.World) {
	h := w.Hosts["edge-01"]
	h.Services["nginx"].State = "stopped"
	h.Metrics["http_up"] = 0
	h.Metrics["memory_percent"] = 97
	ts := time.Now().Add(-3 * time.Minute).Format("Jan 2 15:04:05")
	h.Logs["/var/log/syslog"] = append(h.Logs["/var/log/syslog"],
		ts+" edge-01 kernel: Out of memory: Killed process 812 (nginx) total-vm:412344kB, anon-rss:388120kB",
		ts+" edge-01 systemd[1]: nginx.service: Main process exited, code=killed, status=9/KILL",
		ts+" edge-01 systemd[1]: nginx.service: Failed with result 'signal'.")
	h.Logs["/var/log/nginx/error.log"] = append(h.Logs["/var/log/nginx/error.log"], time.Now().Add(-4*time.Minute).Format("2006/01/02 15:04:05")+" [warn] 812#812: *48213 an upstream response is buffered to a temporary file, cache zone edge_cache is 98% full")
}
func (f *serviceCrashFault) Tick(w *world.World) {
	h := w.Hosts["edge-01"]
	if !f.fixed {
		h.Metrics["http_up"] = 0
		h.Metrics["memory_percent"] = max(90, h.Metrics["memory_percent"]-0.2)
	} else {
		h.Metrics["memory_percent"] = max(61, h.Metrics["memory_percent"]-4)
	}
}
func (f *serviceCrashFault) Remedy(w *world.World, a world.Action) (bool, string) {
	if a.Name != "restart_service" {
		return false, ""
	}
	if r, _ := a.Args["resource"].(string); r != "edge-01" {
		return false, ""
	}
	f.fixed = true
	return true, "Health check GET / returned 200 in 11 ms."
}

func serviceCrash() Definition {
	return Definition{
		ID:       "service-crash",
		NewFault: func() world.Fault { return &serviceCrashFault{} },
		Playbook: []Step{
			{Say: "edge-01 stopped answering HTTP while the host itself is up. I will check the service state, memory and the system log."},
			{Tool: "get_resource_state", Input: map[string]any{"resource": "edge-01"}},
			{Tool: "read_metrics", Input: map[string]any{"resource": "edge-01", "metric": "memory_percent", "window_minutes": 30}},
			{Tool: "tail_logs", Input: map[string]any{"resource": "edge-01", "path": "/var/log/syslog", "lines": 20}},
			{Tool: "tail_logs", Input: map[string]any{"resource": "edge-01", "path": "/var/log/nginx/error.log", "lines": 20}},
			{Tool: "read_metrics", Input: map[string]any{"resource": "edge-01", "metric": "http_up", "window_minutes": 30}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "resource", "claim": "nginx is stopped on edge-01; the server itself is started.", "pointer": "systemd · edge-01"}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "log", "claim": "The kernel OOM killer terminated nginx (pid 812) three minutes ago.", "pointer": "/var/log/syslog · edge-01"}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "metric", "claim": "Memory was at 97% when nginx died and is still above 90%.", "pointer": "memory_percent · edge-01"}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "log", "claim": "nginx warned that the cache zone was 98% full and buffering upstream responses to disk.", "pointer": "/var/log/nginx/error.log · edge-01"}},
			{Tool: "record_hypothesis", Input: map[string]any{"title": "OOM kill driven by a full proxy cache on a 1 GB host", "detail": "Supported by the kernel log and the memory series. A restart restores service; the cache size needs a follow-up.", "verdict": "supported"}},
			{Tool: "record_hypothesis", Input: map[string]any{"title": "Provider outage", "detail": "Rejected: the server is started and the probe is reporting.", "verdict": "rejected"}},
			{Say: "Cause: nginx was OOM-killed. Restarting it restores the edge now; I will note the cache sizing for the owner."},
			{Tool: "propose_action", Input: map[string]any{"action": "restart_service", "resource": "edge-01", "service": "nginx", "summary": "Restart nginx on edge-01", "blast_radius": "Starts the nginx service. Edge traffic resumes within seconds. No configuration or data changes."}},
			{Tool: "conclude", Input: map[string]any{"summary": "nginx on edge-01 was OOM-killed with the proxy cache full. Restarted; recommend lowering edge_cache size.", "cause_verified": true}},
		},
		Chain: []map[string]string{
			{"time": stamp(-4 * time.Minute), "text": "Cache zone 98% full, buffering to disk"},
			{"time": stamp(-3 * time.Minute), "text": "Memory 97%, kernel OOM kills nginx", "severity": "critical"},
			{"time": stamp(0), "text": "http_up 0 for 3 minutes", "severity": "critical"},
		},
	}
}

// --- UpCloud server edge-01 stopped ---

type serverStoppedFault struct {
	fixed    bool
	starting int
}

func (f *serverStoppedFault) ID() string       { return "server-stopped" }
func (f *serverStoppedFault) Resource() string { return "edge-01" }
func (f *serverStoppedFault) Fixed() bool      { return f.fixed }
func (f *serverStoppedFault) Start(w *world.World) {
	h := w.Hosts["edge-01"]
	h.State = "stopped"
}
func (f *serverStoppedFault) Tick(w *world.World) {
	h := w.Hosts["edge-01"]
	if h.State == "starting" {
		f.starting++
		if f.starting >= 3 {
			h.State = "started"
			h.Metrics["http_up"] = 1
			h.Services["nginx"].State = "running"
			f.fixed = true
		}
	}
}
func (f *serverStoppedFault) Remedy(w *world.World, a world.Action) (bool, string) {
	if a.Name != "start_server" {
		return false, ""
	}
	if r, _ := a.Args["resource"].(string); r != "edge-01" {
		return false, ""
	}
	return true, ""
}

func serverStopped() Definition {
	return Definition{
		ID:       "server-stopped",
		NewFault: func() world.Fault { return &serverStoppedFault{} },
		Playbook: []Step{
			{Say: "edge-01 reports stopped and nobody stopped it. I will confirm the provider state and look for an operator action before starting it."},
			{Tool: "get_resource_state", Input: map[string]any{"resource": "edge-01"}},
			{Tool: "get_activity", Input: map[string]any{"resource": "edge-01"}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "resource", "claim": "UpCloud reports edge-01 in state stopped.", "pointer": "GET /1.3/server · edge-01"}},
			{Tool: "record_evidence", Input: map[string]any{"kind": "activity", "claim": "No operator stop action is recorded; UpCloud offers no activity log, and Warden has seen no stop request.", "pointer": "Operator actions · edge-01"}},
			{Tool: "record_hypothesis", Input: map[string]any{"title": "Unplanned stop", "detail": "No intent to stop is recorded. Starting the server is the allowlisted L4 action for this responder.", "verdict": "supported"}},
			{Say: "Cause: unplanned stop. Starting the server is allowlisted for this responder, so I am doing it now and will report."},
			{Tool: "propose_action", Input: map[string]any{"action": "start_server", "resource": "edge-01", "summary": "Start edge-01", "blast_radius": "Powers on the server. Edge traffic returns once nginx starts with the host. No data change."}},
			{Tool: "conclude", Input: map[string]any{"summary": "edge-01 stopped without an operator event. Started autonomously under server-stopped (L4).", "cause_verified": true}},
		},
		Chain: []map[string]string{
			{"time": stamp(0), "text": "edge-01 state stopped, no operator event", "severity": "critical"},
		},
	}
}
