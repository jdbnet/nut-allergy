package webhooks

import (
	"encoding/json"
	"fmt"
)

// Delivery is one outbound webhook request.
type Delivery struct {
	Body    []byte
	Headers map[string]string
}

// Build formats an outbound webhook for the given template.
func Build(template string, p Message) (Delivery, error) {
	switch Normalize(template) {
	case TemplateTeams:
		return buildTeams(p)
	case TemplateDiscord:
		return buildDiscord(p)
	case TemplateSlack:
		return buildSlack(p)
	case TemplateGotify:
		return buildGotify(p)
	case TemplateNtfy:
		return buildNtfy(p)
	default:
		body, err := json.Marshal(p)
		if err != nil {
			return Delivery{}, err
		}
		return jsonDelivery(body), nil
	}
}

func jsonDelivery(body []byte) Delivery {
	return Delivery{
		Body:    body,
		Headers: map[string]string{"Content-Type": "application/json"},
	}
}

func marshalDelivery(v any) (Delivery, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return Delivery{}, err
	}
	return jsonDelivery(body), nil
}

func eventTitle(p Message) string {
	switch p.Event {
	case "on_battery":
		return "UPS on battery"
	case "on_mains":
		return "Back on mains power"
	case "low_battery":
		return "Low battery"
	case "test":
		return "Test notification"
	default:
		return "UPS alert"
	}
}

func factValue(p *int, suffix string) string {
	if p == nil {
		return "—"
	}
	return fmt.Sprintf("%d%s", *p, suffix)
}
