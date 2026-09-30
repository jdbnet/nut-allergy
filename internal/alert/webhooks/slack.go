package webhooks

import "fmt"

func buildSlack(p Message) (Delivery, error) {
	fields := fmt.Sprintf(
		"*UPS:* %s\n*Status:* %s\n*Battery:* %s\n*Runtime:* %s\n*Load:* %s\n*Time:* %s",
		p.UPSName,
		p.State,
		factValue(p.ChargePercent, "%"),
		factValue(p.MinutesRemaining, " min"),
		factValue(p.LoadPercent, "%"),
		p.Timestamp,
	)
	body := map[string]any{
		"text": p.Text,
		"blocks": []any{
			map[string]any{
				"type": "header",
				"text": map[string]any{
					"type": "plain_text",
					"text": eventTitle(p),
				},
			},
			map[string]any{
				"type": "section",
				"text": map[string]any{
					"type": "mrkdwn",
					"text": fields,
				},
			},
		},
	}
	return marshalDelivery(body)
}
