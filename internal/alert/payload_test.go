package alert

import (
	"encoding/json"
	"testing"
	"time"

	"nut-allergy/internal/snmp"
)

func TestBuildPayloadAndSlackFormat(t *testing.T) {
	load := 40
	tr := Transition{Kind: KindOnBattery, FromState: snmp.StateOnline, ToState: snmp.StateOnBattery, OccurredAt: time.Now()}
	p := BuildPayload("id1", "Rack A", tr, snmp.Reading{LoadPercent: &load}, tr.OccurredAt)
	if p.Event != "on_battery" {
		t.Fatalf("event %q", p.Event)
	}
	body, err := FormatWebhookBody("slack", p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m["text"] == "" {
		t.Fatal("missing text")
	}
}
