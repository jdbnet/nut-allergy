package alert

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"
)

// PostWebhook sends a JSON body to url.
func PostWebhook(ctx context.Context, url string, body []byte) error {
	if url == "" {
		return fmt.Errorf("webhook url is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %s", resp.Status)
	}
	return nil
}
