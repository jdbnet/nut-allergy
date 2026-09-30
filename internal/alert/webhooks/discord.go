package webhooks

func buildDiscord(p Message) (Delivery, error) {
	fields := []map[string]string{
		{"name": "UPS", "value": p.UPSName},
		{"name": "Status", "value": p.State},
		{"name": "Battery", "value": factValue(p.ChargePercent, "%")},
		{"name": "Runtime", "value": factValue(p.MinutesRemaining, " min")},
		{"name": "Load", "value": factValue(p.LoadPercent, "%")},
	}
	body := map[string]any{
		"username": "NUT Allergy",
		"embeds": []any{
			map[string]any{
				"title":       eventTitle(p),
				"description": p.Text,
				"color":       discordColor(p.Event),
				"fields":      fields,
				"timestamp":   p.Timestamp,
			},
		},
	}
	return marshalDelivery(body)
}
