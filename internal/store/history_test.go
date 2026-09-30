package store

import (
	"testing"
	"time"

	"nut-allergy/internal/snmp"
)

func TestRecordPollHistorySamplesAndEvents(t *testing.T) {
	st := openTest(t)
	u, err := st.CreateUPS(UPSInput{
		Name: "rack", Host: "10.0.0.5", SecLevel: "authNoPriv",
		Username: "nut", AuthProtocol: "SHA", AuthPassword: "secretsecret",
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	load := 25
	if err := st.ApplyReading(u.ID, snmp.Reading{State: snmp.StateOnline, LoadPercent: &load}, start); err != nil {
		t.Fatal(err)
	}
	if err := st.ApplyReading(u.ID, snmp.Reading{State: snmp.StateOnline, LoadPercent: &load}, start.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	metrics, err := st.ListUPSMetrics(u.ID, start.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 {
		t.Fatalf("metrics after throttle = %d, want 1", len(metrics))
	}
	if err := st.ApplyReading(u.ID, snmp.Reading{State: snmp.StateOnBattery, LoadPercent: &load}, start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	events, err := st.ListUPSEvents(u.ID, start.Add(-time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	var batteryEvent *UPSEvent
	for i := range events {
		if events[i].ToState == snmp.StateOnBattery {
			batteryEvent = &events[i]
			break
		}
	}
	if batteryEvent == nil {
		t.Fatalf("no on_battery event in %#v", events)
	}
}
