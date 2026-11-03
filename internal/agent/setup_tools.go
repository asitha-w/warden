package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/asitha-w/warden/internal/domain"
	"github.com/asitha-w/warden/internal/setup"
	"github.com/asitha-w/warden/internal/store"
)

// setupPolicy is the fixed gate for what the orchestrator may do during
// setup. Onboarding and the grant change nothing on the customer's side
// until the operator acts, so they run at once. API access and the probe
// change the customer's cluster, so a human approves the specific cell.
var setupPolicy = map[string]string{
	"onboard_cells":     domain.PolicyAllow,
	"issue_grant":       domain.PolicyAllow,
	"enable_api_access": domain.PolicyAsk,
	"install_probe":     domain.PolicyAsk,
}

// SetupPolicy returns the policy for a setup tool, deny for anything else.
func SetupPolicy(tool string) string {
	if p, ok := setupPolicy[tool]; ok {
		return p
	}
	return domain.PolicyDeny
}

// SetupTools are the orchestrator's tools for taking a tenant from a fresh
// credential to a managed estate.
func SetupTools() []ToolDef {
	cell := map[string]any{"type": "string", "description": "Cell id, for example web-01 or aks-shop"}
	conn := map[string]any{"type": "string", "description": "Connection id, for example conn-1. Omit to use the only connection."}
	return []ToolDef{
		{Name: "get_setup_state", Description: "The whole setup in one call: stage, tenant, each connection with what it found, each cell with its level, and the next steps in order with who does each. Call this first whenever the operator asks what is there, what to do next, or after any setup action.", Schema: map[string]any{}},
		{Name: "list_catalogue", Description: "What a connection discovered: every entry with kind, whether Warden can manage it, and whether it is a cell already.", Schema: map[string]any{"connection": conn}},
		{Name: "onboard_cells", Description: "Make cells out of catalogue entries. Changes nothing on the customer's side. Omit ids to onboard everything manageable.", Schema: map[string]any{"connection": conn, "ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}},
		{Name: "issue_grant", Description: "Issue the one-time grant token for a VM cell and get the exact line the operator runs on the host as a user with sudo. The level rises by itself when the grant reports back. Use tunnel when the operator cannot open port 22.", Schema: map[string]any{"cell": cell, "tunnel": map[string]any{"type": "boolean"}}, Required: []string{"cell"}},
		{Name: "enable_api_access", Description: "Raise a cluster cell to Level 1: Warden allows its egress address on the API server and creates its identities in the cluster. Changes the customer's cluster, so policy opens an approval; tell the operator to approve it and do not call again.", Schema: map[string]any{"cell": cell}, Required: []string{"cell"}},
		{Name: "install_probe", Description: "Raise a cluster cell from Level 1 to Level 2 by applying the probe manifest. Changes the customer's cluster, so policy opens an approval; tell the operator to approve it and do not call again.", Schema: map[string]any{"cell": cell}, Required: []string{"cell"}},
	}
}

func (o *Orchestrator) setupTool(name string, in map[string]any, by string) (string, error) {
	switch name {
	case "get_setup_state":
		return o.setup.Overview().Text(), nil
	case "list_catalogue":
		id, err := o.connectionID(str(in, "connection"))
		if err != nil {
			return "", err
		}
		var out []string
		o.st.View(func(s *domain.State) {
			c := s.Connection(id)
			if c == nil {
				return
			}
			for _, cand := range c.Catalogue {
				state := "not manageable yet"
				if cand.Supported {
					state = "manageable, not a cell"
					if s.Cell(cand.ID) != nil {
						state = "cell"
					}
				}
				out = append(out, fmt.Sprintf("%s (%s, %s, %s access): %s", cand.ID, cand.Kind, cand.Detail, cand.Access, state))
			}
		})
		if len(out) == 0 {
			return "The catalogue is empty.", nil
		}
		return strings.Join(out, "\n"), nil
	case "onboard_cells":
		id, err := o.connectionID(str(in, "connection"))
		if err != nil {
			return "", err
		}
		var ids []string
		if raw, ok := in["ids"].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					ids = append(ids, s)
				}
			}
		}
		added, err := o.setup.Onboard(id, ids, by+" via orchestrator")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Onboarded %s as cells at Level 0.", strings.Join(added, ", ")), nil
	case "issue_grant":
		id := str(in, "cell")
		tunnel, _ := in["tunnel"].(bool)
		if _, err := o.setup.Grant(id, tunnel, by+" via orchestrator"); err != nil {
			return "", err
		}
		var line, egress string
		o.st.View(func(s *domain.State) {
			line = setup.GrantLine(s.Tenant, s.Cell(id))
			if s.Tenant != nil {
				egress = s.Tenant.EgressIP
			}
		})
		port := "Port 22 on the host must allow " + egress + " first."
		if tunnel {
			port = "The host dials out on 443, no inbound port is needed."
		}
		return fmt.Sprintf("Grant issued for %s, valid one hour. %s Give the operator this exact line to run on the host as a user with sudo:\n%s\nThe level turns to 1 by itself when the grant reports back.", id, port, line), nil
	case "enable_api_access", "install_probe":
		return o.askSetup(name, str(in, "cell"), by)
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

// connectionID resolves an optional connection id: the only connection when
// there is one, an error when the choice is ambiguous.
func (o *Orchestrator) connectionID(id string) (string, error) {
	var ids []string
	o.st.View(func(s *domain.State) {
		for _, c := range s.Connections {
			ids = append(ids, c.ID)
		}
	})
	if id != "" {
		for _, x := range ids {
			if x == id {
				return id, nil
			}
		}
		return "", fmt.Errorf("no connection %s; connections are %s", id, strings.Join(ids, ", "))
	}
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("no connection yet: the operator adds a cloud provider credential first")
	case 1:
		return ids[0], nil
	}
	return "", fmt.Errorf("say which connection: %s", strings.Join(ids, ", "))
}

// askSetup opens an approval for a setup action that changes the customer's
// side, and returns at once. Decide runs the action when the operator approves.
func (o *Orchestrator) askSetup(tool, cellID, by string) (string, error) {
	if SetupPolicy(tool) != domain.PolicyAsk {
		return "", fmt.Errorf("%s is not an approvable setup action", tool)
	}
	var err error
	var ap *domain.Approval
	o.st.Update(func(s *domain.State) []store.Event {
		c := s.Cell(cellID)
		switch {
		case c == nil:
			err = fmt.Errorf("no such cell %s", cellID)
		case !c.IsCluster():
			err = fmt.Errorf("%s is a VM: it takes the grant, use issue_grant", cellID)
		case tool == "enable_api_access" && c.Level >= domain.LevelAPI:
			err = fmt.Errorf("%s already has API access", cellID)
		case tool == "install_probe" && c.Level < domain.LevelAPI:
			err = fmt.Errorf("%s needs API access before the probe", cellID)
		case tool == "install_probe" && c.HasProbe():
			err = fmt.Errorf("%s already has the probe", cellID)
		case c.Channel == domain.ChannelWaiting:
			err = fmt.Errorf("%s is already being raised", cellID)
		}
		if err != nil {
			return nil
		}
		for _, a := range s.Approvals {
			if a.Status == domain.ApprovalPending && a.Action == tool && a.Args["cell"] == cellID {
				err = fmt.Errorf("approval %s for %s on %s is already pending", a.ID, tool, cellID)
				return nil
			}
		}
		task := map[string]string{"enable_api_access": "Enable API access on " + cellID, "install_probe": "Install the probe on " + cellID}[tool]
		blast := map[string]string{
			"enable_api_access": "Adds the tenant egress address to the API server's allowed ranges and creates namespace warden-system with read and write ServiceAccounts. Workloads are not touched. Revoke by deleting the namespace and the range.",
			"install_probe":     "Applies a DaemonSet and one Deployment in namespace warden-system that read node, pod and event data and send telemetry out on 443. Workloads are not touched. Revoke by deleting the namespace.",
		}[tool]
		ap = &domain.Approval{ID: o.st.NextID("A", 0x20), Task: task, Level: "setup", Action: tool, Args: map[string]any{"cell": cellID}, Command: tool + " " + cellID, BlastRadius: blast, Status: domain.ApprovalPending, RequestedAt: time.Now()}
		s.Approvals = append(s.Approvals, ap)
		return []store.Event{store.ApprovalEvent(ap), store.Activity(s, "task", fmt.Sprintf("Setup %s needs approval: %s. Requested by the orchestrator for %s.", ap.ID, ap.Task, by), "", true), store.Toast("Approval requested: " + ap.Task)}
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Approval %s opened: %s. It changes the customer's cluster, so the operator approves it (Approvals, or say approve). When approved it runs and the level rises by itself.", ap.ID, ap.Task), nil
}

// runSetupApproval executes an approved setup action. It returns false when
// the approval is not a setup one.
func (o *Orchestrator) runSetupApproval(a *domain.Approval, by string) (bool, error) {
	if a.RunID != "" || SetupPolicy(a.Action) != domain.PolicyAsk {
		return false, nil
	}
	cellID, _ := a.Args["cell"].(string)
	who := by + " approved " + a.ID
	switch a.Action {
	case "enable_api_access":
		return true, o.setup.APIAccess(cellID, who)
	case "install_probe":
		return true, o.setup.Probe(cellID, who)
	}
	return false, nil
}
