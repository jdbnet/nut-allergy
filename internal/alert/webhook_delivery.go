package alert

// WebhookDelivery is one outbound webhook request.
type WebhookDelivery struct {
	Body    []byte
	Headers map[string]string
}
