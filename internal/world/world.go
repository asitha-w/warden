// Package world is the simulated infrastructure: hosts with metric streams,
// services with versions, logs, files and an activity log. Faults bend the
// generators; actions fix them. Everything an agent tool reads comes from here,
// so an investigation is grounded in state the agent did not write.
package world

import (
	"fmt"
	"math"
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Point is one metric sample.
type Point struct {
	T time.Time `json:"t"`
	V float64   `json:"v"`
}

// Service is a process or workload on a host.
type Service struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	PrevVersion string `json:"prevVersion,omitempty"`
	State       string `json:"state"` // running, stopped
}

// ActivityEntry is a provider-side change record.
type ActivityEntry struct {
	At    time.Time `json:"at"`
	Actor string    `json:"actor"`
	Text  string    `json:"text"`
}

// Host is one monitored machine or cluster.
type Host struct {
	ID       string
	Provider string
	Kind     string
	State    string // started, stopped
	Metrics  map[string]float64
	Series   map[string][]Point
	Services map[string]*Service
	Logs     map[string][]string
	Files    map[string]string
	Activity []ActivityEntry
	Health   string
}

// Action is a change request executed against the world.
type Action struct {
	Name string
	Args map[string]any
}

// Fault is an injected problem. Tick evolves it; Remedy says whether an action fixes it.
type Fault interface {
	ID() string
	Resource() string
	Start(w *World)
	Tick(w *World)
	Remedy(w *World, a Action) (fixed bool, msg string)
	Fixed() bool
}

// World holds the hosts and active faults. All methods are safe for concurrent use.
type World struct {
	mu     sync.Mutex
	Hosts  map[string]*Host
	faults map[string]Fault
	tick   int
	rnd    *rand.Rand
	start  time.Time
}

const seriesLen = 180

// New builds the estate the seed resources describe.
func New() *World {
	w := &World{Hosts: map[string]*Host{}, faults: map[string]Fault{}, rnd: rand.New(rand.NewSource(7)), start: time.Now()}
	w.Hosts["web-01"] = &Host{ID: "web-01", Provider: "azure", Kind: "vm", State: "started", Health: "Available",
		Metrics:  map[string]float64{"cpu_percent": 23, "memory_percent": 48, "disk_used_percent": 71, "requests_per_second": 310, "error_rate_percent": 0.2, "log_growth_mb_per_min": 2, "http_up": 1},
		Services: map[string]*Service{"nginx": {Name: "nginx", Version: "1.26.2", State: "running"}, "shop-web": {Name: "shop-web", Version: "v3.1.0", State: "running"}},
		Logs:     map[string][]string{"/var/log/nginx/access.log": {}, "/var/log/nginx/error.log": {}, "/var/log/syslog": {}},
		Files:    map[string]string{"/var/lib/logrotate/status": "logrotate state -- version 2\n\"/var/log/nginx/access.log\" 2026-10-30-01:00:00\n", "/etc/nginx/env": "NGINX_DEBUG=0\n"},
		Activity: []ActivityEntry{{At: time.Now().Add(-26 * time.Hour), Actor: "ci-deploy", Text: "Deployment shop-web v3.1.0 succeeded"}}}
	w.Hosts["aks-shop"] = &Host{ID: "aks-shop", Provider: "azure", Kind: "kubernetes", State: "started", Health: "Available",
		Metrics:  map[string]float64{"cpu_percent": 41, "memory_percent": 62, "requests_per_second": 820, "error_rate_percent": 0.3, "p95_latency_ms": 120, "pod_restarts_10m": 0, "http_up": 1},
		Services: map[string]*Service{"shop-api": {Name: "shop-api", Version: "v2.4.0", State: "running"}, "ingress-nginx": {Name: "ingress-nginx", Version: "1.11.2", State: "running"}},
		Logs:     map[string][]string{"shop-api": {}, "ingress-nginx": {}},
		Files:    map[string]string{},
		Activity: []ActivityEntry{{At: time.Now().Add(-40 * time.Hour), Actor: "ci-deploy", Text: "Rollout shop-api v2.4.0 completed, 3/3 replicas ready"}}}
	w.Hosts["edge-01"] = &Host{ID: "edge-01", Provider: "upcloud", Kind: "server", State: "started", Health: "n/a",
		Metrics:  map[string]float64{"cpu_percent": 18, "memory_percent": 61, "disk_used_percent": 44, "requests_per_second": 140, "error_rate_percent": 0.1, "http_up": 1},
		Services: map[string]*Service{"nginx": {Name: "nginx", Version: "1.26.2", State: "running"}},
		Logs:     map[string][]string{"/var/log/syslog": {}, "/var/log/nginx/error.log": {}},
		Files:    map[string]string{},
		Activity: []ActivityEntry{}}
	w.Hosts["db-01"] = &Host{ID: "db-01", Provider: "upcloud", Kind: "managed-db", State: "started", Health: "n/a",
		Metrics:  map[string]float64{"cpu_percent": 31, "memory_percent": 70, "disk_used_percent": 52, "connections": 42},
		Services: map[string]*Service{"postgresql": {Name: "postgresql", Version: "16.4", State: "running"}},
		Logs:     map[string][]string{}, Files: map[string]string{}, Activity: []ActivityEntry{}}
	for _, h := range w.Hosts {
		h.Series = map[string][]Point{}
	}
	return w
}

// Tick advances the simulation one step and returns the current metrics per host.
func (w *World) Tick() map[string]map[string]float64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.tick++
	now := time.Now()
	for _, h := range w.Hosts {
		if h.State != "started" {
			continue
		}
		w.jitter(h, "cpu_percent", 3, 4, 95)
		w.jitter(h, "memory_percent", 1, 10, 99)
		if _, ok := h.Metrics["requests_per_second"]; ok {
			base := map[string]float64{"web-01": 310, "aks-shop": 820, "edge-01": 140}[h.ID]
			if h.Metrics["http_up"] > 0 {
				h.Metrics["requests_per_second"] = clamp(h.Metrics["requests_per_second"]+(w.rnd.Float64()-0.5)*base*0.08, base*0.8, base*1.2)
			} else {
				h.Metrics["requests_per_second"] = 0
			}
		}
		if _, ok := h.Metrics["connections"]; ok {
			w.jitter(h, "connections", 4, 20, 90)
		}
	}
	for _, f := range w.faults {
		f.Tick(w)
	}
	out := map[string]map[string]float64{}
	for _, h := range w.Hosts {
		if h.State != "started" {
			out[h.ID] = map[string]float64{"up": 0}
			continue
		}
		m := map[string]float64{"up": 1}
		for k, v := range h.Metrics {
			h.Series[k] = append(h.Series[k], Point{T: now, V: v})
			if len(h.Series[k]) > seriesLen {
				h.Series[k] = h.Series[k][1:]
			}
			m[k] = math.Round(v*10) / 10
		}
		out[h.ID] = m
	}
	return out
}

func (w *World) jitter(h *Host, k string, step, lo, hi float64) {
	if _, ok := h.Metrics[k]; !ok {
		return
	}
	h.Metrics[k] = clamp(h.Metrics[k]+(w.rnd.Float64()-0.5)*step, lo, hi)
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// Inject starts a fault.
func (w *World) Inject(f Fault) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.faults[f.ID()] = f
	f.Start(w)
}

// ClearFault forgets a fault once its run is over.
func (w *World) ClearFault(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.faults, id)
}

// Metric returns the current value of a metric, and whether it exists.
func (w *World) Metric(host, metric string) (float64, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	h := w.Hosts[host]
	if h == nil || h.State != "started" {
		return 0, false
	}
	v, ok := h.Metrics[metric]
	return v, ok
}

// HostState returns the provider state of a host.
func (w *World) HostState(host string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if h := w.Hosts[host]; h != nil {
		return h.State
	}
	return "unknown"
}

// HasOperatorStop reports whether an operator stopped the host on purpose.
func (w *World) HasOperatorStop(host string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	h := w.Hosts[host]
	if h == nil {
		return false
	}
	for _, a := range h.Activity {
		if strings.Contains(a.Text, "stop") && a.Actor != "platform" {
			return true
		}
	}
	return false
}

// ServiceState returns the state of a service on a host.
func (w *World) ServiceState(host, svc string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if h := w.Hosts[host]; h != nil {
		if s := h.Services[svc]; s != nil {
			return s.State
		}
	}
	return "unknown"
}

// --- queries the agent tools use ---

// QueryMetrics summarises a metric over the last window.
func (w *World) QueryMetrics(host, metric string, windowMin int) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	h := w.Hosts[host]
	if h == nil {
		return "", fmt.Errorf("unknown resource %q", host)
	}
	if h.State != "started" {
		return fmt.Sprintf("%s is %s: the probe is offline and no metrics are arriving. Provider state is the only signal.", host, h.State), nil
	}
	series, ok := h.Series[metric]
	if !ok {
		keys := make([]string, 0, len(h.Metrics))
		for k := range h.Metrics {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return "", fmt.Errorf("no metric %q on %s; available: %s", metric, host, strings.Join(keys, ", "))
	}
	if windowMin <= 0 {
		windowMin = 60
	}
	n := windowMin * 60 / 1 // one sample per second in the simulation; window is clamped to what we have
	if n > len(series) {
		n = len(series)
	}
	if n == 0 {
		return fmt.Sprintf("%s on %s: no samples yet", metric, host), nil
	}
	s := series[len(series)-n:]
	minV, maxV, sum := s[0].V, s[0].V, 0.0
	for _, p := range s {
		minV = math.Min(minV, p.V)
		maxV = math.Max(maxV, p.V)
		sum += p.V
	}
	first, last := s[0].V, s[len(s)-1].V
	trend := "flat"
	if last-first > math.Max(1, math.Abs(first)*0.1) {
		trend = "rising"
	} else if first-last > math.Max(1, math.Abs(first)*0.1) {
		trend = "falling"
	}
	// a few evenly spaced samples so the model can see the shape
	var pts []string
	step := len(s) / 8
	if step < 1 {
		step = 1
	}
	for i := 0; i < len(s); i += step {
		pts = append(pts, fmt.Sprintf("%s=%.1f", s[i].T.Format("15:04:05"), s[i].V))
	}
	pts = append(pts, fmt.Sprintf("%s=%.1f", s[len(s)-1].T.Format("15:04:05"), last))
	return fmt.Sprintf("%s on %s over the last %d samples: now %.1f, min %.1f, max %.1f, mean %.1f, trend %s. Samples: %s", metric, host, n, last, minV, maxV, sum/float64(n), trend, strings.Join(pts, " ")), nil
}

// TailLogs returns the last n lines of a log.
func (w *World) TailLogs(host, path string, n int) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	h := w.Hosts[host]
	if h == nil {
		return "", fmt.Errorf("unknown resource %q", host)
	}
	if h.State != "started" {
		return "", fmt.Errorf("%s is %s; the probe cannot read logs", host, h.State)
	}
	lines, ok := h.Logs[path]
	if !ok {
		keys := make([]string, 0, len(h.Logs))
		for k := range h.Logs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return "", fmt.Errorf("no log %q on %s; available: %s", path, host, strings.Join(keys, ", "))
	}
	if n <= 0 || n > 200 {
		n = 50
	}
	if len(lines) == 0 {
		return fmt.Sprintf("%s on %s: no lines in the window (steady state, nothing unusual logged)", path, host), nil
	}
	if n > len(lines) {
		n = len(lines)
	}
	return strings.Join(lines[len(lines)-n:], "\n"), nil
}

// ActivityLog returns provider-side changes for a host.
func (w *World) ActivityLog(host string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	h := w.Hosts[host]
	if h == nil {
		return "", fmt.Errorf("unknown resource %q", host)
	}
	if h.Provider == "upcloud" {
		if len(h.Activity) == 0 {
			return fmt.Sprintf("UpCloud has no activity log. Operator actions Warden knows about for %s: none.", host), nil
		}
	}
	if len(h.Activity) == 0 {
		return "No changes recorded in the last 48 hours.", nil
	}
	var out []string
	for _, a := range h.Activity {
		out = append(out, fmt.Sprintf("%s  %s  %s", a.At.Format("2006-01-02 15:04"), a.Actor, a.Text))
	}
	return strings.Join(out, "\n"), nil
}

// ResourceState describes a host from the provider's point of view plus services.
func (w *World) ResourceState(host string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	h := w.Hosts[host]
	if h == nil {
		return "", fmt.Errorf("unknown resource %q", host)
	}
	var svcs []string
	names := make([]string, 0, len(h.Services))
	for n := range h.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := h.Services[n]
		line := fmt.Sprintf("%s %s %s", s.Name, s.Version, s.State)
		if s.PrevVersion != "" {
			line += " (previous " + s.PrevVersion + ")"
		}
		svcs = append(svcs, line)
	}
	health := h.Health
	if h.Provider == "upcloud" {
		health = "not offered by UpCloud"
	}
	return fmt.Sprintf("%s (%s, %s): provider state %s, resource health %s. Services: %s.", host, h.Provider, h.Kind, h.State, health, strings.Join(svcs, "; ")), nil
}

// ReadFile returns a file from a host via the probe.
func (w *World) ReadFile(host, path string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	h := w.Hosts[host]
	if h == nil {
		return "", fmt.Errorf("unknown resource %q", host)
	}
	if h.State != "started" {
		return "", fmt.Errorf("%s is %s; the probe cannot read files", host, h.State)
	}
	c, ok := h.Files[path]
	if !ok {
		return "", fmt.Errorf("no such file %s on %s", path, host)
	}
	return c, nil
}

// Execute applies an action. It returns the provider's response text.
func (w *World) Execute(a Action) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	host, _ := a.Args["resource"].(string)
	h := w.Hosts[host]
	if h == nil && a.Name != "notify" {
		return "", fmt.Errorf("unknown resource %q", host)
	}
	var out string
	switch a.Name {
	case "run_command":
		if h.State != "started" {
			return "", fmt.Errorf("%s is not running", host)
		}
		cmd, _ := a.Args["command"].(string)
		out = fmt.Sprintf("Run Command on %s: status Succeeded\n$ %s", host, cmd)
	case "restart_service":
		svc, _ := a.Args["service"].(string)
		s := h.Services[svc]
		if s == nil {
			return "", fmt.Errorf("no service %q on %s", svc, host)
		}
		s.State = "running"
		h.Metrics["http_up"] = 1
		h.Activity = append(h.Activity, ActivityEntry{At: time.Now(), Actor: "warden-remediator", Text: "systemctl restart " + svc})
		out = fmt.Sprintf("systemctl restart %s on %s: active (running) since %s", svc, host, time.Now().Format("15:04:05"))
	case "rollout_undo":
		svc, _ := a.Args["service"].(string)
		s := h.Services[svc]
		if s == nil {
			return "", fmt.Errorf("no workload %q on %s", svc, host)
		}
		if s.PrevVersion == "" {
			return "", fmt.Errorf("%s has no previous revision to roll back to", svc)
		}
		from := s.Version
		s.Version, s.PrevVersion = s.PrevVersion, ""
		h.Activity = append(h.Activity, ActivityEntry{At: time.Now(), Actor: "warden-remediator", Text: fmt.Sprintf("kubectl rollout undo deployment/%s (%s -> %s)", svc, from, s.Version)})
		out = fmt.Sprintf("deployment.apps/%s rolled back to %s. 3/3 replicas ready.", svc, s.Version)
	case "start_server":
		if h.State == "started" {
			out = host + " is already started"
		} else {
			h.State = "starting"
			h.Activity = append(h.Activity, ActivityEntry{At: time.Now(), Actor: "warden-remediator", Text: "POST /1.3/server/" + host + "/start"})
			out = fmt.Sprintf("POST /1.3/server/%s/start: 200 OK, state maintenance -> started", host)
		}
	case "stop_server", "delete_pod":
		return "", fmt.Errorf("%s is denied by policy", a.Name)
	case "notify":
		out = "Notification sent to the service owner."
	default:
		return "", fmt.Errorf("unknown action %q", a.Name)
	}
	for _, f := range w.faults {
		if ok, msg := f.Remedy(w, a); ok && msg != "" {
			out += "\n" + msg
		}
	}
	return out, nil
}

// Now is the simulation clock, used for scenario timestamps.
func (w *World) Now() time.Time { return time.Now() }

// Digest is the small payload a log workflow collects: what is new since the
// cursor, how much of it is an error, and the distinct error signatures.
type Digest struct {
	Host       string         `json:"host"`
	Path       string         `json:"path"`
	NewLines   int            `json:"newLines"`
	Errors     int            `json:"errors"`
	Signatures []string       `json:"signatures"`
	Sample     []string       `json:"sample"`
	Cursor     map[string]int `json:"-"`
}

var (
	digestNum  = regexp.MustCompile(`\d+`)
	digestTime = regexp.MustCompile(`^(\w{3} +\d+ \d\d:\d\d:\d\d|\d{4}/\d\d/\d\d \d\d:\d\d:\d\d|\S+ - - \[[^\]]+\])\s*`)
)

func isErrorLine(line string) bool {
	l := strings.ToLower(line)
	for _, k := range []string{"error", "[crit]", "[emerg]", "killed", "failed", "denied", "out of memory"} {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

// Signature strips the timestamp, the host name and numbers so repeated
// lines collapse into one.
func Signature(host, line string) string {
	s := digestTime.ReplaceAllString(line, "")
	s = strings.TrimPrefix(s, host+" ")
	s = digestNum.ReplaceAllString(s, "N")
	s = strings.TrimRight(s, ".")
	if len(s) > 100 {
		s = s[:100]
	}
	return strings.TrimSpace(s)
}

// LogDigest aggregates the lines written since the cursor on the host. path
// "*" covers every log on the host. The cursor maps path to line count and
// is returned updated, so the caller keeps it per workflow and cell.
func (w *World) LogDigest(host, path string, cursor map[string]int) (Digest, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	h := w.Hosts[host]
	if h == nil {
		return Digest{}, fmt.Errorf("unknown resource %q", host)
	}
	if h.State != "started" {
		return Digest{}, fmt.Errorf("%s is %s; nothing to read", host, h.State)
	}
	d := Digest{Host: host, Path: path, Cursor: map[string]int{}}
	for k, v := range cursor {
		d.Cursor[k] = v
	}
	seen := map[string]bool{}
	paths := []string{path}
	if path == "*" {
		paths = paths[:0]
		for k := range h.Logs {
			paths = append(paths, k)
		}
		sort.Strings(paths)
	}
	for _, p := range paths {
		lines, ok := h.Logs[p]
		if !ok {
			if path != "*" {
				return Digest{}, fmt.Errorf("no log %q on %s", p, host)
			}
			continue
		}
		from := d.Cursor[p]
		if from > len(lines) {
			from = 0
		}
		for _, line := range lines[from:] {
			d.NewLines++
			if isErrorLine(line) {
				d.Errors++
				sig := Signature(host, line)
				if !seen[sig] {
					seen[sig] = true
					d.Signatures = append(d.Signatures, sig)
				}
				if len(d.Sample) < 3 {
					d.Sample = append(d.Sample, line)
				}
			}
		}
		d.Cursor[p] = len(lines)
	}
	return d, nil
}
