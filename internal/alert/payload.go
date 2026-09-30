package alert

import (
	"fmt"
	"strings"
	"time"

	"nut-allergy/internal/alert/webhooks"
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

// SamplePayload builds a realistic notification for tests and previews.
func SamplePayload(event string) Payload {
	return payloadFromMessage(webhooks.SampleMessage(event))
}

// WebhookMessage converts a payload for webhook template builders.
func WebhookMessage(p Payload) webhooks.Message {
	return webhooks.Message{
		Event:            p.Event,
		UPSID:            p.UPSID,
		UPSName:          p.UPSName,
		State:            p.State,
		PreviousState:    p.PreviousState,
		Timestamp:        p.Timestamp,
		LoadPercent:      p.LoadPercent,
		ChargePercent:    p.ChargePercent,
		MinutesRemaining: p.MinutesRemaining,
		Text:             p.Text,
	}
}

func payloadFromMessage(m webhooks.Message) Payload {
	return Payload{
		Event:            m.Event,
		UPSID:            m.UPSID,
		UPSName:          m.UPSName,
		State:            m.State,
		PreviousState:    m.PreviousState,
		Timestamp:        m.Timestamp,
		LoadPercent:      m.LoadPercent,
		ChargePercent:    m.ChargePercent,
		MinutesRemaining: m.MinutesRemaining,
		Text:             m.Text,
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
