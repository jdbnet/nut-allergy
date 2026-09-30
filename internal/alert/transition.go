package alert

import (
	"time"

	"nut-allergy/internal/snmp"
)

// Kind is a notification class.
type Kind string

const (
	KindOnBattery  Kind = "on_battery"
	KindOnMains    Kind = "on_mains"
	KindLowBattery Kind = "low_battery"
)

// Transition is a confirmed UPS state change worth notifying.
type Transition struct {
	Kind       Kind
	FromState  string
	ToState    string
	OccurredAt time.Time
}

// Tracker debounces SNMP state flapping before emitting transitions.
type Tracker struct {
	Debounce time.Duration
	Cooldown time.Duration

	stable   map[string]string
	pending  map[string]pendingState
	lastSent map[string]map[Kind]time.Time
}

type pendingState struct {
	state string
	since time.Time
}

// NewTracker returns a tracker with the given debounce and cooldown windows.
func NewTracker(debounce, cooldown time.Duration) *Tracker {
	return &Tracker{
		Debounce: debounce,
		Cooldown: cooldown,
		stable:   map[string]string{},
		pending:  map[string]pendingState{},
		lastSent: map[string]map[Kind]time.Time{},
	}
}

// Observe ingests a poll reading and returns transitions that cleared debounce and cooldown.
func (t *Tracker) Observe(upsID, state string, now time.Time) []Transition {
	if state == snmp.StateUnreachable || state == snmp.StateUnknown {
		return nil
	}
	stable := t.stable[upsID]
	if stable == "" {
		t.stable[upsID] = state
		return nil
	}
	if state == stable {
		delete(t.pending, upsID)
		return nil
	}
	p := t.pending[upsID]
	if p.state != state {
		t.pending[upsID] = pendingState{state: state, since: now}
		return nil
	}
	if now.Sub(p.since) < t.Debounce {
		return nil
	}
	kind, ok := classifyTransition(stable, state)
	if !ok {
		t.stable[upsID] = state
		delete(t.pending, upsID)
		return nil
	}
	if t.inCooldown(upsID, kind, now) {
		t.stable[upsID] = state
		delete(t.pending, upsID)
		return nil
	}
	tr := Transition{Kind: kind, FromState: stable, ToState: state, OccurredAt: now}
	t.stable[upsID] = state
	delete(t.pending, upsID)
	t.markSent(upsID, kind, now)
	return []Transition{tr}
}

func classifyTransition(from, to string) (Kind, bool) {
	switch {
	case to == snmp.StateLowBattery:
		return KindLowBattery, true
	case to == snmp.StateOnBattery && from == snmp.StateOnline:
		return KindOnBattery, true
	case to == snmp.StateOnline && (from == snmp.StateOnBattery || from == snmp.StateLowBattery):
		return KindOnMains, true
	default:
		return "", false
	}
}

func (t *Tracker) inCooldown(upsID string, kind Kind, now time.Time) bool {
	if t.Cooldown <= 0 {
		return false
	}
	m := t.lastSent[upsID]
	if m == nil {
		return false
	}
	last, ok := m[kind]
	return ok && now.Sub(last) < t.Cooldown
}

func (t *Tracker) markSent(upsID string, kind Kind, now time.Time) {
	if t.lastSent[upsID] == nil {
		t.lastSent[upsID] = map[Kind]time.Time{}
	}
	t.lastSent[upsID][kind] = now
}
