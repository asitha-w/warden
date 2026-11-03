package setup

import (
	"fmt"
	"strings"

	"github.com/asitha-w/warden/internal/domain"
)

// Stages of setup, in order. The stage is the first thing still missing.
const (
	StageTenant     = "tenant"     // no tenant yet
	StageConnection = "connection" // no connection yet, or none validated
	StageOnboard    = "onboard"    // a connection found things that are not cells yet
	StageReach      = "reach"      // cells exist below their highest level
	StageReady      = "ready"      // every cell at its highest level
)

// Who does a step.
const (
	WhoOperator = "operator" // the human: a form, a consent, a line to run on their side
	WhoWarden   = "warden"   // the orchestrator can do it with a tool
	WhoBoth     = "both"     // Warden prepares it, the operator runs it
)

// Step is one thing to do next.
type Step struct {
	Kind   string `json:"kind"`             // tenant, connection, validating, onboard, grant, waiting, api-access, probe
	Target string `json:"target,omitempty"` // connection or cell id
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Who    string `json:"who"`
	Tool   string `json:"tool,omitempty"` // the orchestrator tool that does it
}

// ConnSummary is a connection as setup sees it.
type ConnSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	Status     string `json:"status"`
	Found      int    `json:"found"`
	Manageable int    `json:"manageable"`
	Onboarded  int    `json:"onboarded"`
	Write      bool   `json:"write"`
}

// CellSummary is a cell as setup sees it.
type CellSummary struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Provider  string `json:"provider"`
	Level     int    `json:"level"`
	LevelName string `json:"levelName"`
	MaxLevel  int    `json:"maxLevel"`
	Hands     string `json:"hands"`
	Channel   string `json:"channel"`
	Next      *Step  `json:"next,omitempty"`
}

// Overview is the whole setup in one value: the stage, what exists, and the
// ordered list of what to do next. The first step is the one to do now.
type Overview struct {
	Stage       string         `json:"stage"`
	Tenant      *domain.Tenant `json:"tenant"`
	EgressIP    string         `json:"egressIp,omitempty"`
	RuntimeHost string         `json:"runtimeHost,omitempty"`
	Connections []ConnSummary  `json:"connections"`
	Cells       []CellSummary  `json:"cells"`
	Next        []Step         `json:"next"`
}

// Overview computes the setup state from the tenant state.
func (s *Service) Overview() Overview {
	var o Overview
	s.st.View(func(st *domain.State) { o = Compute(st) })
	return o
}

// Compute derives the overview from a state. Pure; safe under the store lock.
func Compute(st *domain.State) Overview {
	o := Overview{Connections: []ConnSummary{}, Cells: []CellSummary{}, Next: []Step{}}
	if st.Tenant == nil {
		o.Stage = StageTenant
		o.Next = append(o.Next, Step{Kind: "tenant", Title: "Create the tenant", Detail: "A name is enough. The tenant gets its own runtime and one static egress address.", Who: WhoOperator})
		return o
	}
	o.Tenant = st.Tenant
	o.EgressIP = st.Tenant.EgressIP
	o.RuntimeHost = RuntimeHost(st.Tenant)
	validating := false
	for _, c := range st.Connections {
		cs := ConnSummary{ID: c.ID, Name: c.Name, Provider: c.Provider, Status: c.Status, Found: len(c.Catalogue), Write: c.WriteScope.Level != "" && c.WriteScope.Level != domain.ScopeNone}
		if c.Provider == "upcloud" {
			cs.Write = true
		}
		for _, cand := range c.Catalogue {
			if cand.Supported {
				cs.Manageable++
				if st.Cell(cand.ID) != nil {
					cs.Onboarded++
				}
			}
		}
		o.Connections = append(o.Connections, cs)
		switch {
		case c.Status == domain.ConnValidating:
			validating = true
			o.Next = append(o.Next, Step{Kind: "validating", Target: c.ID, Title: "Validating " + c.Name, Detail: "Checking the identity and discovering what is in scope.", Who: WhoWarden})
		case c.Status == domain.ConnConnected && cs.Onboarded < cs.Manageable:
			o.Next = append(o.Next, Step{Kind: "onboard", Target: c.ID, Title: fmt.Sprintf("Onboard %d of %d manageable things from %s", cs.Manageable-cs.Onboarded, cs.Manageable, c.Name), Detail: "Each becomes a cell at Level 0: provider state, provider metrics and provider actions under policy.", Who: WhoWarden, Tool: "onboard_cells"})
		}
	}
	for _, c := range st.Cells {
		cs := CellSummary{ID: c.ID, Kind: c.Kind, Provider: c.Provider, Level: c.Level, LevelName: c.LevelName(), MaxLevel: c.MaxLevel(), Hands: c.Hands, Channel: c.Channel}
		if step := nextForCell(st, c); step != nil {
			cs.Next = step
			o.Next = append(o.Next, *step)
		}
		o.Cells = append(o.Cells, cs)
	}
	switch {
	case len(st.Connections) == 0:
		o.Stage = StageConnection
		o.Next = append([]Step{{Kind: "connection", Title: "Add a cloud provider credential", Detail: "Azure: a consented service principal or your own app registration at a scope. UpCloud: a sub-account API token. Secrets go in the form, never in the chat.", Who: WhoOperator}}, o.Next...)
	case validating && len(st.Cells) == 0 && !hasKind(o.Next, "onboard"):
		o.Stage = StageConnection
	case hasKind(o.Next, "onboard"):
		o.Stage = StageOnboard
	case len(st.Cells) == 0:
		o.Stage = StageConnection
		o.Next = append(o.Next, Step{Kind: "connection", Title: "The connection found nothing Warden can manage", Detail: "Widen the scope or add another connection.", Who: WhoOperator})
	case len(o.Next) > 0:
		o.Stage = StageReach
	default:
		o.Stage = StageReady
	}
	return o
}

func hasKind(steps []Step, kind string) bool {
	for _, s := range steps {
		if s.Kind == kind {
			return true
		}
	}
	return false
}

func nextForCell(st *domain.State, c *domain.Cell) *Step {
	if c.Channel == domain.ChannelWaiting {
		return &Step{Kind: "waiting", Target: c.ID, Title: c.ID + " is being raised", Detail: "Waiting for the cell to report back.", Who: WhoWarden}
	}
	if !c.ProbeCapable || c.Level >= c.MaxLevel() {
		return nil
	}
	switch {
	case !c.IsCluster():
		detail := "Warden issues a one-time token; you run one line on the host as a user with sudo. It installs the probe and the SSH trust together. Port 22 must allow the egress address " + st.Tenant.EgressIP + ", or choose the reverse tunnel."
		if c.Grant != nil {
			return &Step{Kind: "grant", Target: c.ID, Title: "Run the grant on " + c.ID, Detail: "The token is issued. Run on the host: " + GrantLine(st.Tenant, c), Who: WhoOperator}
		}
		return &Step{Kind: "grant", Target: c.ID, Title: "Grant Warden access to " + c.ID, Detail: detail, Who: WhoBoth, Tool: "issue_grant"}
	case c.Level < domain.LevelAPI:
		return &Step{Kind: "api-access", Target: c.ID, Title: "Enable API access on " + c.ID, Detail: "Warden adds the egress address to the API server's allowed ranges and creates its read and write identities in the cluster. Changes the cluster, so it asks first.", Who: WhoWarden, Tool: "enable_api_access"}
	default:
		return &Step{Kind: "probe", Target: c.ID, Title: "Install the probe on " + c.ID, Detail: "Applies the collector manifest in namespace warden-system. Changes the cluster, so it asks first.", Who: WhoWarden, Tool: "install_probe"}
	}
}

// Text renders the overview for the orchestrator.
func (o Overview) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Setup stage: %s.\n", o.Stage)
	if o.Tenant != nil {
		fmt.Fprintf(&b, "Tenant %s, runtime %s, static egress %s.\n", o.Tenant.Name, o.RuntimeHost, o.EgressIP)
	} else {
		b.WriteString("No tenant yet.\n")
	}
	if len(o.Connections) == 0 {
		b.WriteString("Connections: none.\n")
	}
	for _, c := range o.Connections {
		w := "read only"
		if c.Write {
			w = "read and write"
		}
		fmt.Fprintf(&b, "Connection %s (%s, %s, %s): found %d, manageable %d, onboarded %d.\n", c.ID, c.Name, c.Provider, c.Status+", "+w, c.Found, c.Manageable, c.Onboarded)
	}
	if len(o.Cells) == 0 {
		b.WriteString("Cells: none.\n")
	}
	for _, c := range o.Cells {
		next := "at its highest level"
		if c.Next != nil {
			next = "next: " + c.Next.Title
		}
		fmt.Fprintf(&b, "Cell %s (%s, %s): %s of %d, hands %s, channel %s; %s.\n", c.ID, c.Kind, c.Provider, c.LevelName, c.MaxLevel, c.Hands, c.Channel, next)
	}
	if len(o.Next) == 0 {
		b.WriteString("Nothing left to set up.")
		return b.String()
	}
	b.WriteString("Next steps, in order:\n")
	for i, s := range o.Next {
		who := map[string]string{WhoOperator: "the operator does this", WhoWarden: "you can do this with " + s.Tool, WhoBoth: "you prepare it with " + s.Tool + ", the operator runs it"}[s.Who]
		if s.Tool == "" && s.Who == WhoWarden {
			who = "in progress"
		}
		fmt.Fprintf(&b, "%d. %s (%s). %s\n", i+1, s.Title, who, s.Detail)
	}
	return strings.TrimRight(b.String(), "\n")
}
