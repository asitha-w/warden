// Package setup is the one place that raises a tenant from nothing to a
// managed estate: onboarding catalogue entries as cells, issuing the grant,
// enabling API access and installing the probe. The API and the orchestrator
// both call it, so the operator's clicks and the agent's tools take the same
// path and leave the same activity.
package setup

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/asitha-w/warden/internal/domain"
	"github.com/asitha-w/warden/internal/store"
)

// Service raises cells. Simulated: each raise reports back after a short delay.
type Service struct {
	st *store.Store
	// Delay scales the simulated round trips; tests set it to zero.
	Delay time.Duration
}

// New returns a service on the store.
func New(st *store.Store) *Service { return &Service{st: st, Delay: time.Second} }

// RuntimeHost is the tenant hostname the grant and the probe dial.
func RuntimeHost(t *domain.Tenant) string {
	if t == nil {
		return "runtime.warden.example"
	}
	return t.Runtime + ".warden.example"
}

// GrantLine is the one line the operator runs on a VM as a user with sudo.
func GrantLine(t *domain.Tenant, c *domain.Cell) string {
	if c == nil || c.Grant == nil {
		return ""
	}
	line := fmt.Sprintf("curl -fsSL \"https://%s/grant/%s?t=%s\" | sudo sh", RuntimeHost(t), c.ID, c.Grant.Token)
	if c.Grant.Tunnel {
		line += " -s -- --tunnel"
	}
	return line
}

// SyncProbe recomputes which cells can carry the probe and which have it.
func SyncProbe(st *domain.State) {
	st.Probe.Expected = []string{}
	st.Probe.Hosts = []string{}
	waiting := false
	for _, c := range st.Cells {
		if c.ProbeCapable {
			st.Probe.Expected = append(st.Probe.Expected, c.ID)
		}
		if c.HasProbe() {
			st.Probe.Hosts = append(st.Probe.Hosts, c.ID)
		}
		if c.Channel == domain.ChannelWaiting {
			waiting = true
		}
	}
	switch {
	case waiting:
		st.Probe.Status = domain.ProbeWaiting
	case len(st.Probe.Hosts) > 0:
		st.Probe.Status = domain.ProbeOnline
	default:
		st.Probe.Status = domain.ProbeNone
	}
}

// Onboard turns catalogue entries of a connection into cells. With no ids,
// every supported entry that is not a cell yet is onboarded.
func (s *Service) Onboard(connID string, ids []string, by string) ([]string, error) {
	var errOut error
	var added []string
	s.st.Update(func(st *domain.State) []store.Event {
		c := st.Connection(connID)
		if c == nil || c.Status != domain.ConnConnected {
			errOut = errors.New("connection not found or not validated")
			return nil
		}
		want := ids
		if len(want) == 0 {
			for _, cand := range c.Catalogue {
				want = append(want, cand.ID)
			}
		}
		for _, id := range want {
			cand := c.Candidate(id)
			if cand == nil || !cand.Supported || st.Cell(id) != nil {
				continue
			}
			st.Cells = append(st.Cells, domain.NewCell(c, cand))
			added = append(added, id)
		}
		if len(added) == 0 {
			errOut = errors.New("nothing to onboard: pick supported entries that are not cells yet")
			return nil
		}
		SyncProbe(st)
		return []store.Event{{Type: "cells", Data: st.Cells}, {Type: "probe", Data: st.Probe}, store.Activity(st, "run", fmt.Sprintf("%s onboarded %s as cells through %s. Level 0: provider sensors and provider actions.", by, strings.Join(added, ", "), c.Name), "", false), store.Toast(fmt.Sprintf("%d cell(s) onboarded", len(added)))}
	})
	return added, errOut
}

// Grant issues the one-time grant token for a VM cell and waits, in the
// background, for the grant to report back. It returns the grant so the
// caller can hand the operator the line to run.
func (s *Service) Grant(id string, tunnel bool, by string) (*domain.Grant, error) {
	var err error
	var g *domain.Grant
	s.st.Update(func(st *domain.State) []store.Event {
		c := st.Cell(id)
		switch {
		case c == nil:
			err = errors.New("no such cell")
		case c.IsCluster():
			err = errors.New("a cluster takes API access, not the grant")
		case c.Level >= domain.LevelGranted:
			err = errors.New(id + " already has the grant")
		case c.Channel == domain.ChannelWaiting:
			err = errors.New(id + " is waiting for its grant to report back")
		}
		if err != nil {
			return nil
		}
		c.Grant = &domain.Grant{Token: fmt.Sprintf("wgt_%06x%06x", rand.Intn(0xffffff), rand.Intn(0xffffff)), IssuedAt: time.Now(), Tunnel: tunnel}
		g = &domain.Grant{}
		*g = *c.Grant
		c.Channel = domain.ChannelWaiting
		SyncProbe(st)
		return []store.Event{{Type: "cells", Data: st.Cells}, {Type: "probe", Data: st.Probe}, store.Activity(st, "run", fmt.Sprintf("%s issued the Warden grant for %s (%s). Waiting for the grant to report back.", by, id, map[bool]string{true: "reverse tunnel", false: "SSH 22 from the egress address"}[tunnel]), "", false)}
	})
	if err != nil {
		return nil, err
	}
	s.after(2500*time.Millisecond, func() {
		s.st.Update(func(st *domain.State) []store.Event {
			c := st.Cell(id)
			if c == nil || c.Grant == nil || c.Level >= domain.LevelGranted {
				return nil
			}
			now := time.Now()
			c.Grant.HostKey = fmt.Sprintf("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI%06x…", rand.Intn(0xffffff))
			c.Grant.InstalledAt = &now
			c.Level = domain.LevelGranted
			c.Hands = domain.HandsSSH
			if c.Grant.Tunnel {
				c.Hands = domain.HandsTunnel
			}
			c.Channel = domain.ChannelOnline
			SyncProbe(st)
			return []store.Event{{Type: "cells", Data: st.Cells}, {Type: "probe", Data: st.Probe}, store.Activity(st, "signal", fmt.Sprintf("Grant on %s reported back: host key pinned, instance identity matches the provider, probe dialled in over OpAMP. %s. Hands: %s.", id, c.LevelName(), map[string]string{domain.HandsSSH: "SSH from the egress address, Ansible catalogue", domain.HandsTunnel: "SSH through the reverse tunnel, Ansible catalogue"}[c.Hands]), "", false), store.Toast(id + " is at Level 1")}
		})
	})
	return g, nil
}

// APIAccess raises a cluster cell to Level 1: the kube identity and the
// network path, done by Warden through the connection.
func (s *Service) APIAccess(id, by string) error {
	var err error
	var provider string
	s.st.Update(func(st *domain.State) []store.Event {
		c := st.Cell(id)
		switch {
		case c == nil:
			err = errors.New("no such cell")
		case !c.IsCluster():
			err = errors.New("a VM takes the grant, not API access")
		case c.Level >= domain.LevelAPI:
			err = errors.New(id + " already has API access")
		case c.Channel == domain.ChannelWaiting:
			err = errors.New(id + " is already being raised")
		}
		if err != nil {
			return nil
		}
		provider = c.Provider
		c.Channel = domain.ChannelWaiting
		SyncProbe(st)
		text := fmt.Sprintf("%s enabled API access on %s: checking the role assignments and appending the egress address to the authorized IP ranges.", by, id)
		if provider == "upcloud" {
			text = fmt.Sprintf("%s enabled API access on %s: appending the egress address to the control plane IP filter, fetching the admin kubeconfig once, creating the warden-read and warden-write ServiceAccounts.", by, id)
		}
		return []store.Event{{Type: "cells", Data: st.Cells}, {Type: "probe", Data: st.Probe}, store.Activity(st, "run", text, "", false)}
	})
	if err != nil {
		return err
	}
	s.after(2200*time.Millisecond, func() {
		s.st.Update(func(st *domain.State) []store.Event {
			c := st.Cell(id)
			if c == nil || c.Level >= domain.LevelAPI {
				return nil
			}
			c.Level = domain.LevelAPI
			c.Hands = domain.HandsKubeAPI
			c.Channel = domain.ChannelConnected
			SyncProbe(st)
			text := fmt.Sprintf("%s: Entra token exchanged, nodes and namespaces listed. %s. Hands: Kubernetes API in the permitted namespaces.", id, c.LevelName())
			if provider == "upcloud" {
				text = fmt.Sprintf("%s: IP filter updated, ServiceAccount tokens stored, admin kubeconfig discarded (fetched %s; rotation is through UpCloud support). %s. Hands: Kubernetes API in the permitted namespaces.", id, time.Now().Format("15:04"), c.LevelName())
			}
			return []store.Event{{Type: "cells", Data: st.Cells}, {Type: "probe", Data: st.Probe}, store.Activity(st, "signal", text, "", false), store.Toast(id + " is at Level 1")}
		})
	})
	return nil
}

// Probe installs the probe on a cluster at Level 1, raising it to Level 2.
func (s *Service) Probe(id, by string) error {
	var err error
	s.st.Update(func(st *domain.State) []store.Event {
		c := st.Cell(id)
		switch {
		case c == nil:
			err = errors.New("no such cell")
		case !c.IsCluster():
			err = errors.New("on a VM the grant installs the probe")
		case c.Level < domain.LevelAPI:
			err = errors.New(id + " needs API access (Level 1) before the probe")
		case c.HasProbe():
			err = errors.New(id + " already has the probe")
		case c.Channel == domain.ChannelWaiting:
			err = errors.New(id + " is already being raised")
		}
		if err != nil {
			return nil
		}
		c.Channel = domain.ChannelWaiting
		SyncProbe(st)
		return []store.Event{{Type: "cells", Data: st.Cells}, {Type: "probe", Data: st.Probe}, store.Activity(st, "run", fmt.Sprintf("%s applied the probe manifest on %s: namespace warden-system, DaemonSet and cluster collector. Waiting for the collectors to dial in.", by, id), "", false)}
	})
	if err != nil {
		return err
	}
	s.after(2500*time.Millisecond, func() {
		s.st.Update(func(st *domain.State) []store.Event {
			c := st.Cell(id)
			if c == nil || c.HasProbe() {
				return nil
			}
			c.Level = domain.LevelProbe
			c.Channel = domain.ChannelOnline
			SyncProbe(st)
			return []store.Event{{Type: "cells", Data: st.Cells}, {Type: "probe", Data: st.Probe}, store.Activity(st, "signal", fmt.Sprintf("Probe on %s dialled in over OpAMP from every node. %s.", id, c.LevelName()), "", false), store.Toast(id + " is at Level 2")}
		})
	})
	return nil
}

// after runs fn once the simulated round trip has passed. With Delay zero it
// runs inline so tests see the result at once.
func (s *Service) after(d time.Duration, fn func()) {
	if s.Delay == 0 {
		fn()
		return
	}
	go func() {
		time.Sleep(d * s.Delay / time.Second)
		fn()
	}()
}
