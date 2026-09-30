package webhooks

import (
	"time"

	"nut-allergy/internal/snmp"
)

// Message is the normalized alert content used by webhook templates.
type Message struct {
	Event            string
	UPSID            string
	UPSName          string
	State            string
	PreviousState    string
	Timestamp        string
	LoadPercent      *int
	ChargePercent    *int
	MinutesRemaining *int
	Text             string
}

// SampleMessage builds a realistic notification for tests.
func SampleMessage(event string) Message {
	now := time.Now().UTC()
	load := 42
	charge := 88
	mins := 37
	switch event {
	case "test":
		return Message{
			Event:            "test",
			UPSID:            "sample",
			UPSName:          "Rack A (test)",
			State:            snmp.StateOnBattery,
			PreviousState:    snmp.StateOnline,
			Timestamp:        now.Format(time.RFC3339),
			LoadPercent:      &load,
			ChargePercent:    &charge,
			MinutesRemaining: &mins,
			Text:             "Test UPS is running on battery (sample notification)",
		}
	case "on_mains":
		return Message{
			Event:            "on_mains",
			UPSID:            "sample",
			UPSName:          "Rack A",
			State:            snmp.StateOnline,
			PreviousState:    snmp.StateOnBattery,
			Timestamp:        now.Format(time.RFC3339),
			LoadPercent:      &load,
			ChargePercent:    &charge,
			MinutesRemaining: &mins,
			Text:             "Rack A is back on mains power",
		}
	case "low_battery":
		return Message{
			Event:            "low_battery",
			UPSID:            "sample",
			UPSName:          "Rack A",
			State:            snmp.StateLowBattery,
			PreviousState:    snmp.StateOnBattery,
			Timestamp:        now.Format(time.RFC3339),
			LoadPercent:      &load,
			ChargePercent:    &charge,
			MinutesRemaining: &mins,
			Text:             "Rack A reports low battery",
		}
	default:
		return Message{
			Event:            "on_battery",
			UPSID:            "sample",
			UPSName:          "Rack A",
			State:            snmp.StateOnBattery,
			PreviousState:    snmp.StateOnline,
			Timestamp:        now.Format(time.RFC3339),
			LoadPercent:      &load,
			ChargePercent:    &charge,
			MinutesRemaining: &mins,
			Text:             "Rack A is running on battery",
		}
	}
}
