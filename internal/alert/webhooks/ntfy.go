package webhooks

import "fmt"

func buildNtfy(p Message) (Delivery, error) {
	msg := fmt.Sprintf(
		"%s | Battery %s | Runtime %s | Load %s",
		p.Text,
		factValue(p.ChargePercent, "%"),
		factValue(p.MinutesRemaining, " min"),
		factValue(p.LoadPercent, "%"),
	)
	body := map[string]any{
		"title":    eventTitle(p) + ": " + p.UPSName,
		"message":  msg,
		"priority": ntfyPriority(p.Event),
		"tags":     ntfyTags(p.Event),
	}
	return marshalDelivery(body)
}
