package domain

import (
	"strings"
	"time"
)

// Seed data for a fresh tenant. In the real product these come from discovery
// and the tenant's own configuration; here they describe the simulated estate.

// Inventory is what a provider returns for discovery. Supported kinds become
// cells when onboarded; the rest are listed so the catalogue is honest about
// what Warden saw and cannot manage yet.
var Inventory = map[string][]Candidate{
	"azure": {
		{ID: "web-01", Kind: "Azure VM", Detail: "Standard_B2s · westeurope", ResourceGroup: "rg-shop-prod", Supported: true, ProbeCapable: true, Labels: map[string]string{"app": "shop", "tier": "web", "env": "production", "os": "linux", "owner": "platform@shop.example", "criticality": "critical"}},
		{ID: "aks-shop", Kind: "AKS cluster", Detail: "3 nodes · 1.31", ResourceGroup: "rg-shop-prod", Supported: true, ProbeCapable: true, Labels: map[string]string{"app": "shop", "tier": "api", "env": "production", "platform": "kubernetes", "owner": "platform@shop.example", "criticality": "critical"}},
		{ID: "la-shop", Kind: "Log Analytics workspace", Detail: "workspace", ResourceGroup: "rg-shop-prod", Labels: map[string]string{"app": "shop", "tier": "observability", "env": "production"}},
		{ID: "stshopprod", Kind: "Storage account", Detail: "Standard_LRS", ResourceGroup: "rg-shop-prod", Labels: map[string]string{"app": "shop", "env": "production"}},
		{ID: "vm-staging-01", Kind: "Azure VM", Detail: "Standard_B1s · westeurope", ResourceGroup: "rg-shop-staging", Supported: true, ProbeCapable: true, Labels: map[string]string{"app": "shop", "tier": "web", "env": "staging", "os": "linux"}},
	},
	"upcloud": {
		{ID: "edge-01", Kind: "UpCloud server", Detail: "1 vCPU 1 GB · de-fra1", Supported: true, ProbeCapable: true, Labels: map[string]string{"app": "edge", "tier": "proxy", "env": "production", "os": "linux", "owner": "ops@shop.example", "criticality": "standard"}},
		{ID: "uks-edge", Kind: "UKS cluster", Detail: "2 nodes · 1.34 · fi-hel2", Supported: true, ProbeCapable: true, Labels: map[string]string{"app": "edge", "tier": "api", "env": "production", "platform": "kubernetes", "owner": "ops@shop.example", "criticality": "standard"}},
		{ID: "db-01", Kind: "UpCloud managed PostgreSQL", Detail: "2 vCPU · fi-hel2", Labels: map[string]string{"app": "shop", "tier": "data", "env": "production"}},
	},
}

// Discover fills the connection's catalogue from the provider inventory in scope.
func Discover(c *Connection) {
	c.Catalogue = nil
	for _, cand := range Inventory[c.Provider] {
		probe := &Cell{ID: cand.ID, Kind: cand.Kind, Provider: c.Provider, ResourceGroup: cand.ResourceGroup}
		access := c.AccessTo(probe)
		if access == "" {
			continue
		}
		cp := cand
		cp.Access = access
		cp.Labels = map[string]string{}
		for k, v := range cand.Labels {
			cp.Labels[k] = v
		}
		c.Catalogue = append(c.Catalogue, cp)
	}
}

// NewCell creates a cell from a catalogue entry.
func NewCell(c *Connection, cand *Candidate) *Cell {
	labels := map[string]string{}
	for k, v := range cand.Labels {
		labels[k] = v
	}
	return &Cell{ID: cand.ID, Kind: cand.Kind, Detail: cand.Detail, Provider: c.Provider, ResourceGroup: cand.ResourceGroup, ConnectionID: c.ID, Level: LevelConnected, Hands: HandsNone, Channel: ChannelConnected, ProbeCapable: cand.ProbeCapable, Labels: labels, Telemetry: DefaultTelemetry(cand.Kind), OnboardedAt: time.Now()}
}

// DefaultTelemetry is the probe configuration Warden proposes for a kind.
func DefaultTelemetry(kind string) Telemetry {
	switch {
	case strings.Contains(kind, "cluster"):
		return Telemetry{HostMetrics: true, Processes: false, OTLP: true,
			Logs:      []LogSource{{Name: "shop-api", Enabled: true}, {Name: "ingress-nginx", Enabled: true}},
			Receivers: []Receiver{{Name: "kubelet", Detail: "node and pod metrics", Enabled: true}, {Name: "kube-state", Detail: "deployments, rollouts, restarts", Enabled: true}, {Name: "nginx-ingress", Detail: "upstream status codes", Enabled: false}}}
	case kind == "Azure VM":
		return Telemetry{HostMetrics: true, Processes: true, OTLP: false,
			Logs:      []LogSource{{Name: "/var/log/syslog", Enabled: true}, {Name: "/var/log/nginx/access.log", Enabled: true}, {Name: "/var/log/nginx/error.log", Enabled: true}, {Name: "/var/log/auth.log", Enabled: false}},
			Receivers: []Receiver{{Name: "nginx", Detail: "stub_status: requests, connections", Enabled: true}, {Name: "filestats", Detail: "size and age of watched files", Enabled: true}}}
	default:
		return Telemetry{HostMetrics: true, Processes: true, OTLP: false,
			Logs:      []LogSource{{Name: "/var/log/syslog", Enabled: true}, {Name: "/var/log/nginx/error.log", Enabled: true}, {Name: "/var/log/auth.log", Enabled: false}},
			Receivers: []Receiver{{Name: "nginx", Detail: "stub_status: requests, connections", Enabled: true}}}
	}
}

// SeedResponders is the routing table from signal to agent and policy.
func SeedResponders() []*Responder {
	return []*Responder{
		{ID: "disk-pressure-linux", Signal: "disk_used_percent > 90 for 5m", Scope: "VMs with a probe", Mode: "Review", Level: "L3", Agent: "investigator",
			Tools: []ToolPolicy{{Tool: "read_metrics", Policy: PolicyAllow}, {Tool: "tail_logs", Policy: PolicyAllow}, {Tool: "run_command", Policy: PolicyAsk}, {Tool: "restart_service", Policy: PolicyDeny}}},
		{ID: "release-regression", Signal: "error_rate > 5% for 5m after a rollout", Scope: "Kubernetes services", Mode: "Review", Level: "L3", Agent: "investigator",
			Tools: []ToolPolicy{{Tool: "read_metrics", Policy: PolicyAllow}, {Tool: "tail_logs", Policy: PolicyAllow}, {Tool: "rollout_undo", Policy: PolicyAsk}, {Tool: "delete_pod", Policy: PolicyDeny}}},
		{ID: "service-down", Signal: "http_up == 0 for 3m on a running host", Scope: "Hosts with a probe", Mode: "Review", Level: "L3", Agent: "investigator",
			Tools: []ToolPolicy{{Tool: "read_metrics", Policy: PolicyAllow}, {Tool: "tail_logs", Policy: PolicyAllow}, {Tool: "restart_service", Policy: PolicyAsk}, {Tool: "run_command", Policy: PolicyDeny}}},
		{ID: "server-stopped", Signal: "state != started, no operator event", Scope: "UpCloud servers", Mode: "Autonomous", Level: "L4", Agent: "investigator",
			Tools: []ToolPolicy{{Tool: "get_resource_state", Policy: PolicyAllow}, {Tool: "start_server", Policy: PolicyAllow}, {Tool: "stop_server", Policy: PolicyDeny}}},
		{ID: "slo-burn-rate", Signal: "burn rate > 2x over 1h", Scope: "Services with an SLO", Mode: "Review", Level: "L2", Agent: "investigator",
			Tools: []ToolPolicy{{Tool: "read_metrics", Policy: PolicyAllow}, {Tool: "notify", Policy: PolicyAllow}}},
		{ID: "cert-expiry", Signal: "cert expires in < 14d", Scope: "Ingress, load balancers", Mode: "Review", Level: "L2", Agent: "investigator",
			Tools: []ToolPolicy{{Tool: "get_resource_state", Policy: PolicyAllow}, {Tool: "notify", Policy: PolicyAllow}}},
	}
}

// SeedAgents is the agent roster the orchestrator manages.
func SeedAgents(model string) []*AgentDef {
	return []*AgentDef{
		{ID: "orchestrator", Name: "Orchestrator", Kind: AgentOrchestrator, Role: "Routes signals and requests to the right agent",
			Description: "The agent you talk to. It watches the signal loop, applies the responder table, starts specialists, and answers questions about the tenant. It never changes infrastructure itself.",
			Model:       model, Effort: "medium", Tools: []string{"list_findings", "get_resource_state", "start_investigation", "list_runs"}, Enabled: true, MaxToolCalls: 8},
		{ID: "investigator", Name: "Investigator", Kind: AgentSpecialist, Role: "Collects evidence, forms hypotheses, proposes one action",
			Description: "Started by the orchestrator when a responder matches. Reads metrics, logs, activity and files, records evidence with pointers, and proposes a single remediation under the responder's policy.",
			Model:       model, Effort: "medium", Tools: []string{"read_metrics", "tail_logs", "get_activity", "get_resource_state", "read_file", "record_evidence", "record_hypothesis", "propose_action", "conclude"}, Enabled: true, MaxToolCalls: 25},
		{ID: "remediator", Name: "Remediator", Kind: AgentDeterministic, Role: "Executes approved actions with the write identity",
			Description: "No model. Runs exactly the action that was approved, with the preconditions re-checked, and records the provider's response.",
			Tools:       []string{"run_command", "restart_service", "rollout_undo", "start_server"}, Enabled: true},
		{ID: "verifier", Name: "Verifier", Kind: AgentDeterministic, Role: "Checks claims have pointers and re-reads the signal after a fix",
			Description: "No model. Marks a cause Verified only when every material claim has an evidence row, and resolves a finding only when the triggering signal has cleared on an independent read.",
			Tools:       []string{"read_metrics", "get_resource_state"}, Enabled: true},
		{ID: "cost-advisor", Name: "Cost advisor", Kind: AgentSpecialist, Role: "Finds idle and oversized resources",
			Description: "Weekly review of utilisation against size. Disabled until the tenant opts in; shown here so the roster is complete.",
			Model:       model, Effort: "low", Tools: []string{"read_metrics", "get_resource_state", "notify"}, Enabled: false, MaxToolCalls: 15},
	}
}

// SeedRunbooks is the tenant's starting guidance, one per app tier.
func SeedRunbooks() []*Runbook {
	t := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	return []*Runbook{
		{ID: "shop-web-vms", Name: "Shop web on VMs", Selector: map[string]string{"app": "shop", "tier": "web"}, UpdatedAt: t, UpdatedBy: "platform@shop.example", Body: `# Shop web on VMs

nginx in front of the shop-web process on Ubuntu 22.04. Deploys come from ci-deploy through Run Command.

## Health signals
- disk_used_percent on / above 85% is the early warning; 90% is the signal.
- requests_per_second around 300 is normal at this hour; a spike is above 600.

## Known fixes, in order
1. Disk pressure: force a logrotate run first. If NGINX_DEBUG=1 is set in /etc/nginx/env, turn it off and reload nginx. Debug logging is never meant to stay on in production.
2. High memory: restart shop-web, not nginx.

## Never
- Never delete files under /var/log. Rotate and compress only.
- Never reboot during the 08:00 to 10:00 CET order window.

## Escalation
platform@shop.example, #ops on Slack. Page only for Critical findings.
`},
		{ID: "shop-api-aks", Name: "Shop API on AKS", Selector: map[string]string{"app": "shop", "tier": "api"}, UpdatedAt: t, UpdatedBy: "platform@shop.example", Body: `# Shop API on AKS

Deployment shop-api, three replicas, rolled out by ci-deploy. Checkout depends on it.

## Known fixes, in order
1. Error rate rises right after a rollout: roll back to the previous revision. Do not wait for a hotfix.
2. Pod restarts without a rollout: check node memory before touching the deployment.

## Never
- Never delete pods by hand; let the deployment replace them.
- Never scale below three replicas.

## Escalation
platform@shop.example. Attach the pod log excerpt and the rollout revision.
`},
		{ID: "edge-proxy", Name: "Edge proxy servers", Selector: map[string]string{"app": "edge"}, UpdatedAt: t, UpdatedBy: "ops@shop.example", Body: `# Edge proxy servers

Small UpCloud servers running nginx as a caching proxy. 1 GB of memory; the cache zone is sized at 400 MB.

## Known fixes, in order
1. nginx down, host up: restart nginx. Safe at any hour.
2. Server stopped with no operator event: start it. This is allowlisted (L4).
3. Repeated OOM kills: lower edge_cache in /etc/nginx/conf.d/cache.conf and tell ops.

## Escalation
ops@shop.example. Edge is Standard criticality; no paging.
`},
	}
}

// SeedScenarios lists the faults the demo can inject.
func SeedScenarios() []*Scenario {
	return []*Scenario{
		{ID: "disk-fill", Name: "Fill the disk on web-01", Description: "A deploy turns nginx debug logging on and logrotate has stalled. Expect disk-pressure-linux, a Run Command fix after approval.", Resource: "web-01", Status: "idle"},
		{ID: "bad-release", Name: "Ship a bad version of shop-api", Description: "Rollout v2.4.1 on aks-shop raises the error rate to 18%. Expect release-regression and a rollout undo after approval.", Resource: "aks-shop", Status: "idle"},
		{ID: "service-crash", Name: "Crash nginx on edge-01", Description: "The kernel OOM-kills nginx. Expect service-down and a service restart after approval.", Resource: "edge-01", Status: "idle"},
		{ID: "server-stopped", Name: "Stop the UpCloud server edge-01", Description: "The server stops with no operator event. Expect server-stopped to start it autonomously (L4).", Resource: "edge-01", Status: "idle"},
	}
}

// ValidateAzure simulates validation against the reviewed access model: the
// role assignments the identity would hold at the requested scopes, and the
// capabilities those roles give. Reader covers every Level 0 read; AKS reads
// need the Kubernetes RBAC roles; writes come only from the custom roles.
func ValidateAzure(c *Connection) {
	c.Identity = "Service principal Warden, consented (" + c.CredentialKind + ")"
	if c.ObjectID == "" {
		c.ObjectID = "6f1e2d3c-" + short(c.Directory) + "-warden"
	}
	if c.AuthMode == "byo" && c.ClientID != "" {
		c.Identity = "App registration " + short(c.ClientID) + " (" + c.CredentialKind + ")"
	}
	c.Roles = []RoleAssignment{
		{Identity: "read", Role: "Reader", Scope: c.ReadScope},
		{Identity: "read", Role: "Azure Kubernetes Service RBAC Reader", Scope: c.ReadScope},
		{Identity: "read", Role: "Azure Kubernetes Service Cluster User Role", Scope: c.ReadScope},
		{Identity: "read", Role: "Warden AKS reader (custom: managedClusters/nodes/read)", Scope: c.ReadScope},
	}
	hasWrite := c.WriteScope.Level != ScopeNone && c.WriteScope.Level != ""
	if hasWrite {
		c.Roles = append(c.Roles,
			RoleAssignment{Identity: "write", Role: "Warden VM operator (custom: start, restart, deallocate, powerOff)", Scope: c.WriteScope},
			RoleAssignment{Identity: "write", Role: "Warden AKS operator (custom: agentPools/write, start, stop)", Scope: c.WriteScope},
			RoleAssignment{Identity: "write", Role: "Warden AKS namespace writer (custom data actions, no secrets)", Scope: c.WriteScope})
	}
	w := func(detail string) Capability {
		if hasWrite {
			return Capability{Name: "", Detail: detail + " · write identity at " + c.WriteScope.Level + " " + c.WriteScope.Target, State: "available"}
		}
		return Capability{Name: "", Detail: "no write identity on this connection", State: "missing"}
	}
	named := func(name string, cap Capability) Capability { cap.Name = name; return cap }
	c.Capabilities = []Capability{
		{Name: "Read identity carries no write actions", Detail: "checked at validation · Reader only", State: "available"},
		{Name: "Resource discovery", Detail: "Resource Graph · Reader", State: "available"},
		{Name: "Platform metrics", Detail: "Azure Monitor · Reader (Percentage CPU, Available Memory Bytes, network, disk, VM availability)", State: "available"},
		{Name: "Resource health verdict", Detail: "Resource Health · Reader", State: "available"},
		{Name: "Activity log, who changed what", Detail: "Activity Log · Reader · 90 days", State: "available"},
		{Name: "Log Analytics queries", Detail: "needs Log Analytics Reader on the workspace", State: "missing"},
		{Name: "Kubernetes API reads", Detail: "AKS RBAC Reader + Cluster User Role + nodes/read · Entra token", State: "available"},
		named("VM power actions", w("start, restart, deallocate, powerOff")),
		named("AKS node pool scale, start, stop", w("agentPools/write, start/action, stop/action")),
		named("AKS namespace writes", w("deployments write, pods delete, pods/exec")),
	}
}

// ValidateUpCloud simulates validation of a sub-account API token: the
// permissions it carries and what the API gives through them. UpCloud has no
// read and write split and no server metrics endpoint; both are said plainly.
func ValidateUpCloud(c *Connection) {
	if c.SubAccount == "" {
		c.SubAccount = "warden"
	}
	c.Identity = "API token for sub-account " + c.SubAccount
	if len(c.Permissions) == 0 {
		c.Permissions = []Permission{{TargetType: "server", TargetIdentifier: "*"}, {TargetType: "managed_kubernetes", TargetIdentifier: "*"}}
	}
	if c.TokenExpiresAt == nil {
		t := time.Now().Add(365 * 24 * time.Hour)
		c.TokenExpiresAt = &t
	}
	has := func(tt string) bool {
		for _, p := range c.Permissions {
			if p.TargetType == tt {
				return true
			}
		}
		return false
	}
	state := func(ok bool) string {
		if ok {
			return "available"
		}
		return "missing"
	}
	c.Capabilities = []Capability{
		{Name: "No read and write split", Detail: "a permission allows every operation on its target; the Warden policy gate is the only split", State: "note"},
		{Name: "Server inventory and state", Detail: "GET /1.3/server/{uuid} · server permission", State: state(has("server"))},
		{Name: "Audit log, who did what", Detail: "GET /1.3/audit-logs · actor and source IP · 180 days", State: "available"},
		{Name: "Server power and firewall", Detail: "POST start, stop, restart, firewall_rule · same server permission, gated by policy", State: state(has("server"))},
		{Name: "Managed Kubernetes clusters", Detail: "GET /1.3/kubernetes/{uuid}, kubeconfig, IP filter · managed_kubernetes permission", State: state(has("managed_kubernetes"))},
		{Name: "Managed database metrics", Detail: "GET /1.3/database/{uuid}/metrics · managed_database permission", State: state(has("managed_database"))},
		{Name: "Server metrics", Detail: "no endpoint in the UpCloud API; the probe at Level 1 provides them", State: "none"},
		{Name: "Resource health verdict", Detail: "not offered by UpCloud", State: "none"},
	}
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8] + "…"
	}
	return s
}

// NewState returns an empty tenant state with seed data.
func NewState(model, reasoner string) *State {
	s := &State{
		Connections: []*Connection{},
		Probe:       Probe{Status: ProbeNone, Hosts: []string{}, Expected: []string{}},
		Cells:       []*Cell{},
		Runbooks:    SeedRunbooks(),
		Responders:  SeedResponders(),
		Agents:      SeedAgents(model),
		Findings:    []*Finding{},
		Runs:        []*Run{},
		Approvals:   []*Approval{},
		Activity:    []Activity{},
		Chat:        []*ChatMessage{},
		Scenarios:   SeedScenarios(),
		Usage:       map[string]*Usage{},
		Reasoner:    reasoner,
		Model:       model,
	}
	for _, a := range s.Agents {
		s.Usage[a.ID] = &Usage{}
	}
	return s
}
