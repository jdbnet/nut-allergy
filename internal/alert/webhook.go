package alert

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WebhookResult is the HTTP outcome of a webhook POST.
type WebhookResult struct {
	StatusCode int
	Body       string
}

// PostWebhook sends a webhook delivery and returns an error on failure.
func PostWebhook(ctx context.Context, url string, delivery WebhookDelivery) error {
	res, err := PostWebhookDetailed(ctx, url, delivery)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d: %s", res.StatusCode, truncate(res.Body, 512))
	}
	return nil
}

// PostWebhookDetailed always returns the HTTP status and response body snippet.
func PostWebhookDetailed(ctx context.Context, url string, delivery WebhookDelivery) (WebhookResult, error) {
	if url == "" {
		return WebhookResult{}, fmt.Errorf("webhook url is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(delivery.Body))
	if err != nil {
		return WebhookResult{}, err
	}
	if delivery.Headers != nil {
		for k, v := range delivery.Headers {
			req.Header.Set(k, v)
		}
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return WebhookResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return WebhookResult{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(raw))}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
