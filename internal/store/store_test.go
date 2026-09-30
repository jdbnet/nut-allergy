package store

import (
	"path/filepath"
	"testing"
	"time"

	"nut-allergy/internal/secret"
	"nut-allergy/internal/snmp"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	box, err := secret.Open(filepath.Join(dir, "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(filepath.Join(dir, "server.db"), box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestDeleteAgentRemovesBindings(t *testing.T) {
	st := openTest(t)
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	a, err := st.UpsertAgent("host-a", "fp-a", now)
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUPS(UPSInput{
		Name: "rack", Host: "10.0.0.5", SecLevel: "authNoPriv",
		Username: "nut", AuthProtocol: "SHA", AuthPassword: "secretsecret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetAgentConfig(a.ID, []string{u.ID}, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAgent(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetAgent(a.ID); err == nil {
		t.Fatal("agent still present")
	}
	agents, err := st.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 0 {
		t.Fatalf("agents = %d", len(agents))
	}
}

func TestReadingKeepsClockAcrossUnreachable(t *testing.T) {
	st := openTest(t)
	u, err := st.CreateUPS(UPSInput{
		Name: "rack", Description: "top", Host: "10.0.0.5", SecLevel: "authNoPriv",
		Username: "nut", AuthProtocol: "SHA", AuthPassword: "secretsecret",
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	mins := 40
	if err := st.ApplyReading(u.ID, snmp.Reading{State: snmp.StateOnBattery, MinutesRemaining: &mins}, start); err != nil {
		t.Fatal(err)
	}
	if err := st.ApplyReading(u.ID, snmp.Reading{State: snmp.StateUnreachable, Error: "timeout"}, start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := st.ApplyReading(u.ID, snmp.Reading{State: snmp.StateOnBattery, MinutesRemaining: &mins}, start.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetUPS(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.OnBatterySince == nil || !got.OnBatterySince.Equal(start) {
		t.Fatalf("since = %v, want %v", got.OnBatterySince, start)
	}
	if err := st.ApplyReading(u.ID, snmp.Reading{State: snmp.StateOnline}, start.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetUPS(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.OnBatterySince != nil {
		t.Fatalf("utility should clear the clock, got %v", got.OnBatterySince)
	}
	target, err := st.Target(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if target.AuthPassword != "secretsecret" {
		t.Fatal("secret did not round-trip")
	}
}
