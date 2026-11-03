// Package store is the in-memory system of record for one tenant plus the
// event bus the console subscribes to. Every mutation goes through Update so
// readers always see a consistent state and every change becomes an event.
package store

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/asitha-w/warden/internal/domain"
)

// Event is one change pushed to subscribers.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Store guards the tenant state and fans out events.
type Store struct {
	mu    sync.Mutex
	s     *domain.State
	seq   map[string]int
	subs  map[chan []byte]struct{}
	subMu sync.Mutex
	log   *slog.Logger
}

// New returns a store around the state.
func New(s *domain.State, log *slog.Logger) *Store {
	return &Store{s: s, seq: map[string]int{}, subs: map[chan []byte]struct{}{}, log: log}
}

// View runs fn with the state under the lock. Do not retain pointers.
func (st *Store) View(fn func(s *domain.State)) {
	st.mu.Lock()
	defer st.mu.Unlock()
	fn(st.s)
}

// Update runs fn under the lock and publishes the events it returns.
func (st *Store) Update(fn func(s *domain.State) []Event) {
	st.mu.Lock()
	evs := fn(st.s)
	st.mu.Unlock()
	st.Publish(evs...)
}

// Replace swaps the whole state, used by reset.
func (st *Store) Replace(s *domain.State) {
	st.mu.Lock()
	st.s = s
	st.seq = map[string]int{}
	st.mu.Unlock()
	st.Publish(Event{Type: "snapshot", Data: st.SnapshotValue()})
}

// Snapshot returns the state as JSON.
func (st *Store) Snapshot() []byte {
	st.mu.Lock()
	defer st.mu.Unlock()
	b, _ := json.Marshal(st.s)
	return b
}

// SnapshotValue returns a detached copy of the state.
func (st *Store) SnapshotValue() *domain.State {
	var out domain.State
	_ = json.Unmarshal(st.Snapshot(), &out)
	return &out
}

// NextID returns a short sequential id with the prefix. Call under the lock.
func (st *Store) NextID(prefix string, start int) string {
	st.seq[prefix]++
	return fmt.Sprintf("%s-%04x", prefix, start+st.seq[prefix])
}

// Subscribe returns a channel of encoded events and a cancel func.
func (st *Store) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 256)
	st.subMu.Lock()
	st.subs[ch] = struct{}{}
	st.subMu.Unlock()
	return ch, func() {
		st.subMu.Lock()
		delete(st.subs, ch)
		st.subMu.Unlock()
	}
}

// Publish encodes and fans out events. Slow subscribers drop events; they
// recover on the next full snapshot.
func (st *Store) Publish(evs ...Event) {
	for _, ev := range evs {
		if ev.Type == "" {
			continue
		}
		b, err := json.Marshal(ev)
		if err != nil {
			st.log.Error("encode event", "type", ev.Type, "err", err)
			continue
		}
		st.subMu.Lock()
		for ch := range st.subs {
			select {
			case ch <- b:
			default:
				st.log.Warn("subscriber slow, dropping event", "type", ev.Type)
			}
		}
		st.subMu.Unlock()
	}
}

// Helpers that build common events. They take the state so callers inside
// Update can use them without re-locking.

// Activity appends an activity line and returns its event.
func Activity(s *domain.State, kind, text, runID string, task bool) Event {
	a := domain.Activity{Kind: kind, Text: text, RunID: runID, Task: task, At: time.Now()}
	s.Activity = append([]domain.Activity{a}, s.Activity...)
	if len(s.Activity) > 60 {
		s.Activity = s.Activity[:60]
	}
	return Event{Type: "activity", Data: a}
}

// Toast returns a toast event.
func Toast(text string) Event { return Event{Type: "toast", Data: text} }

// RunEvent returns the full-run event.
func RunEvent(r *domain.Run) Event { return Event{Type: "run", Data: r} }

// FindingEvent returns the finding event.
func FindingEvent(f *domain.Finding) Event { return Event{Type: "finding", Data: f} }

// ApprovalEvent returns the approval event.
func ApprovalEvent(a *domain.Approval) Event { return Event{Type: "approval", Data: a} }

// UsageEvent returns the usage-by-agent event.
func UsageEvent(s *domain.State) Event { return Event{Type: "usage", Data: s.Usage} }

// ChatEvent returns a chat message event.
func ChatEvent(m *domain.ChatMessage) Event { return Event{Type: "chat", Data: m} }

// ScenarioEvent returns a scenario event.
func ScenarioEvent(sc *domain.Scenario) Event { return Event{Type: "scenario", Data: sc} }
