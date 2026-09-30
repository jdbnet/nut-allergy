package alert

import (
	"testing"
	"time"

	"nut-allergy/internal/snmp"
)

func TestTrackerDebouncesOnBattery(t *testing.T) {
	tr := NewTracker(30*time.Second, 0)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if out := tr.Observe("u1", snmp.StateOnline, now); len(out) != 0 {
		t.Fatalf("initial = %v", out)
	}
	if out := tr.Observe("u1", snmp.StateOnBattery, now.Add(10*time.Second)); len(out) != 0 {
		t.Fatalf("early = %v", out)
	}
	out := tr.Observe("u1", snmp.StateOnBattery, now.Add(45*time.Second))
	if len(out) != 1 || out[0].Kind != KindOnBattery {
		t.Fatalf("confirmed = %v", out)
	}
}

func TestTrackerCooldown(t *testing.T) {
	tr := NewTracker(5*time.Second, time.Hour)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tr.Observe("u1", snmp.StateOnline, now)
	tr.Observe("u1", snmp.StateOnBattery, now.Add(10*time.Second))
	out := tr.Observe("u1", snmp.StateOnBattery, now.Add(20*time.Second))
	if len(out) != 1 {
		t.Fatalf("first alert = %v", out)
	}
	tr.Observe("u1", snmp.StateOnline, now.Add(30*time.Second))
	tr.Observe("u1", snmp.StateOnBattery, now.Add(40*time.Second))
	out = tr.Observe("u1", snmp.StateOnBattery, now.Add(50*time.Second))
	if len(out) != 0 {
		t.Fatalf("cooldown blocked = %v", out)
	}
}

func TestClassifyLowBattery(t *testing.T) {
	k, ok := classifyTransition(snmp.StateOnBattery, snmp.StateLowBattery)
	if !ok || k != KindLowBattery {
		t.Fatalf("got %v %v", k, ok)
	}
}
