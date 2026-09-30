package webhooks

func buildTeams(p Message) (Delivery, error) {
	title := eventTitle(p)
	card := map[string]any{
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body": []any{
			map[string]any{
				"type":   "TextBlock",
				"text":   title,
				"weight": "Bolder",
				"size":   "Medium",
				"color":  teamsTitleColor(p.Event),
				"wrap":   true,
			},
			map[string]any{
				"type": "TextBlock",
				"text": p.Text,
				"wrap": true,
			},
			map[string]any{
				"type": "FactSet",
				"facts": []any{
					fact("UPS", p.UPSName),
					fact("Status", p.State),
					fact("Previous", p.PreviousState),
					fact("Battery", factValue(p.ChargePercent, "%")),
					fact("Runtime", factValue(p.MinutesRemaining, " min")),
					fact("Load", factValue(p.LoadPercent, "%")),
					fact("Time", p.Timestamp),
				},
			},
		},
	}
	body := map[string]any{
		"type": "message",
		"attachments": []any{
			map[string]any{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"contentUrl":  nil,
				"content":     card,
			},
		},
	}
	return marshalDelivery(body)
}

func fact(title, value string) map[string]string {
	return map[string]string{"title": title, "value": value}
}
