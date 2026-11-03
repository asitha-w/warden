package agent

import (
	"github.com/asitha-w/warden/internal/domain"
	"github.com/asitha-w/warden/internal/store"
)

// Price per million tokens, input and output, by model id. Unknown models use the Opus 5.5 rate.
var prices = map[string][2]float64{
	"claude-opus-5-5":   {4, 20},
	"claude-sonnet-5-5": {2, 10},
	"claude-haiku-5-5":  {0.1, 0.5},
	"claude-fable-5-1":  {10, 50},
	"claude-opus-5":     {5, 25},
}

// Cost returns the dollar cost of a call.
func Cost(model string, in, out int) float64 {
	p, ok := prices[model]
	if !ok {
		p = prices["claude-opus-5-5"]
	}
	return float64(in)/1e6*p[0] + float64(out)/1e6*p[1]
}

// AddUsage records one model call against a run and its agent.
func AddUsage(st *store.Store, runID, agentID, model string, in, out int) {
	st.Update(func(s *domain.State) []store.Event {
		u := domain.Usage{Calls: 1, InputTokens: in, OutputTokens: out, CostUSD: Cost(model, in, out)}
		evs := []store.Event{}
		if r := s.Run(runID); r != nil {
			r.Usage.Add(u)
			evs = append(evs, store.RunEvent(r))
		}
		if au := s.Usage[agentID]; au != nil {
			au.Add(u)
		} else {
			s.Usage[agentID] = &u
		}
		return append(evs, store.UsageEvent(s))
	})
}
