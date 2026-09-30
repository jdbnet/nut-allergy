package alert

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nut-allergy/internal/snmp"
)

// Payload is the JSON body sent to webhooks.
type Payload struct {
	Event          string `json:"event"`
	UPSID          string `json:"ups_id"`
	UPSName        string `json:"ups_name"`
	State          string `json:"state"`
	PreviousState  string `json:"previous_state"`
	Timestamp      string `json:"timestamp"`
	LoadPercent    *int   `json:"load_percent,omitempty"`
	ChargePercent  *int   `json:"charge_percent,omitempty"`
	MinutesRemaining *int `json:"minutes_remaining,omitempty"`
	Text           string `json:"text"`
}

// BuildPayload formats a webhook/email-friendly payload.
func BuildPayload(upsID, upsName string, tr Transition, r snmp.Reading, now time.Time) Payload {
	text := humanMessage(upsName, tr)
	return Payload{
		Event:            string(tr.Kind),
		UPSID:            upsID,
		UPSName:          upsName,
		State:            tr.ToState,
		PreviousState:    tr.FromState,
		Timestamp:        now.UTC().Format(time.RFC3339),
		LoadPercent:      r.LoadPercent,
		ChargePercent:    r.ChargePercent,
		MinutesRemaining: r.MinutesRemaining,
		Text:             text,
	}
}

func humanMessage(upsName string, tr Transition) string {
	switch tr.Kind {
	case KindOnBattery:
		return upsName + " is running on battery"
	case KindOnMains:
		return upsName + " is back on mains power"
	case KindLowBattery:
		return upsName + " reports low battery"
	default:
		return upsName + " power state changed"
	}
}

// FormatWebhookBody encodes the payload for a receiver style.
func FormatWebhookBody(format string, p Payload) ([]byte, error) {
	switch format {
	case "slack":
		body := map[string]any{"text": p.Text}
		return json.Marshal(body)
	case "discord":
		body := map[string]any{"content": p.Text}
		return json.Marshal(body)
	default:
		return json.Marshal(p)
	}
}

// EmailSubject returns a short subject line.
func EmailSubject(p Payload) string {
	return "UPS alert: " + p.UPSName
}

// EmailBody returns plain text for SMTP.
func EmailBody(p Payload) string {
	lines := []string{
		p.Text,
		"",
		"UPS: " + p.UPSName,
		"Event: " + p.Event,
		"State: " + p.State,
		"Previous: " + p.PreviousState,
		"Time: " + p.Timestamp,
	}
	if p.LoadPercent != nil {
		lines = append(lines, fmt.Sprintf("Load: %d%%", *p.LoadPercent))
	}
	if p.ChargePercent != nil {
		lines = append(lines, fmt.Sprintf("Battery: %d%%", *p.ChargePercent))
	}
	if p.MinutesRemaining != nil {
		lines = append(lines, fmt.Sprintf("Runtime: %d min", *p.MinutesRemaining))
	}
	return strings.Join(lines, "\n")
}
