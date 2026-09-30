package webhooks

import "fmt"

func buildGotify(p Message) (Delivery, error) {
	msg := fmt.Sprintf(
		"%s\n\n**UPS:** %s\n**Status:** %s\n**Battery:** %s\n**Runtime:** %s\n**Load:** %s\n**Time:** %s",
		p.Text,
		p.UPSName,
		p.State,
		factValue(p.ChargePercent, "%"),
		factValue(p.MinutesRemaining, " min"),
		factValue(p.LoadPercent, "%"),
		p.Timestamp,
	)
	body := map[string]any{
		"title":    eventTitle(p),
		"message":  msg,
		"priority": gotifyPriority(p.Event),
		"extras": map[string]any{
			"client::display": map[string]any{
				"contentType": "text/markdown",
			},
		},
	}
	return marshalDelivery(body)
}
