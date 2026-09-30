package server

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"nut-allergy/internal/alert"
	"nut-allergy/internal/alert/webhooks"
	"nut-allergy/internal/snmp"
	"nut-allergy/internal/store"
)

type notifyEngine struct {
	store   *store.Store
	mu      sync.Mutex
	tracker *alert.Tracker
}

func newNotifyEngine(st *store.Store) *notifyEngine {
	return &notifyEngine{store: st}
}

func (n *notifyEngine) reloadTracker() {
	timing, err := n.store.GetAlertTiming()
	if err != nil {
		log.Printf("alert timing: %v", err)
		timing = store.AlertTiming{DebounceSeconds: 30, CooldownSeconds: 300}
	}
	n.mu.Lock()
	n.tracker = alert.NewTracker(time.Duration(timing.DebounceSeconds)*time.Second, time.Duration(timing.CooldownSeconds)*time.Second)
	n.mu.Unlock()
}

func (n *notifyEngine) observe(upsID, upsName, prevState string, r snmp.Reading, now time.Time) {
	n.mu.Lock()
	tr := n.tracker
	n.mu.Unlock()
	if tr == nil {
		return
	}
	for _, transition := range tr.Observe(upsID, r.State, now) {
		msg := alert.BuildPayload(upsID, upsName, transition, r, now).Text
		eventType := "alert_" + string(transition.Kind)
		if err := n.store.InsertUPSEvent(upsID, eventType, transition.FromState, transition.ToState, msg, r, now, false); err != nil {
			log.Printf("ups %s event: %v", upsName, err)
		}
		n.deliver(upsID, upsName, transition, r, now)
	}
	_ = prevState
}

func (n *notifyEngine) deliver(upsID, upsName string, tr alert.Transition, r snmp.Reading, now time.Time) {
	payload := alert.BuildPayload(upsID, upsName, tr, r, now)
	ctx := context.Background()

	smtpCfg, err := n.store.GetSMTPSecret()
	if err != nil {
		log.Printf("smtp settings: %v", err)
	} else if smtpCfg.Enabled {
		cfg := alert.SMTPConfig{
			Host:     smtpCfg.Host,
			Port:     smtpCfg.Port,
			TLSMode:  smtpCfg.TLSMode,
			Username: smtpCfg.Username,
			Password: smtpCfg.Password,
			From:     smtpCfg.FromAddr,
			To:       alert.ParseRecipients(smtpCfg.ToAddrs),
		}
		plain, htmlBody := alert.EmailMIME(payload)
		if err := alert.SendMail(cfg, alert.EmailSubject(payload), plain, htmlBody); err != nil {
			log.Printf("smtp %s: %v", upsName, err)
		}
	}

	hooks, err := n.store.ListWebhookSecrets()
	if err != nil {
		log.Printf("webhooks: %v", err)
		return
	}
	for _, h := range hooks {
		if !h.Enabled || strings.TrimSpace(h.URL) == "" {
			continue
		}
		delivery, err := webhooks.Build(h.Format, alert.WebhookMessage(payload))
		if err != nil {
			log.Printf("webhook %s body: %v", h.ID, err)
			continue
		}
		if err := alert.PostWebhook(ctx, h.URL, toWebhookDelivery(delivery)); err != nil {
			log.Printf("webhook %s %s: %v", h.ID, upsName, err)
		}
	}
}

func (n *notifyEngine) sendTestEmail(cfg store.SMTPSettingsSecret) error {
	if !cfg.Enabled {
		return errNotEnabled("smtp")
	}
	payload := alert.SamplePayload("test")
	smtp := alert.SMTPConfig{
		Host:     cfg.Host,
		Port:     cfg.Port,
		TLSMode:  cfg.TLSMode,
		Username: cfg.Username,
		Password: cfg.Password,
		From:     cfg.FromAddr,
		To:       alert.ParseRecipients(cfg.ToAddrs),
	}
	plain, htmlBody := alert.EmailMIME(payload)
	return alert.SendMail(smtp, "UPS alert test: "+payload.UPSName, plain, htmlBody)
}

func (n *notifyEngine) resolveSMTPSecret(pub store.SMTPSettings, password string) (store.SMTPSettingsSecret, error) {
	out := store.SMTPSettingsSecret{SMTPSettings: pub}
	stored, err := n.store.GetSMTPSecret()
	if err == nil {
		if password == "" {
			out.Password = stored.Password
		} else {
			out.Password = password
		}
		return out, nil
	}
	out.Password = password
	return out, nil
}

func (n *notifyEngine) testWebhook(url, format, event string) (alert.WebhookResult, error) {
	if strings.TrimSpace(url) == "" {
		return alert.WebhookResult{}, fmt.Errorf("webhook url is required")
	}
	payload := alert.SamplePayload(event)
	if event == "" {
		payload = alert.SamplePayload("test")
	}
	delivery, err := webhooks.Build(format, alert.WebhookMessage(payload))
	if err != nil {
		return alert.WebhookResult{}, err
	}
	return alert.PostWebhookDetailed(context.Background(), url, toWebhookDelivery(delivery))
}

func toWebhookDelivery(d webhooks.Delivery) alert.WebhookDelivery {
	return alert.WebhookDelivery{Body: d.Body, Headers: d.Headers}
}

func errNotEnabled(channel string) error {
	return errString(channel + " alerts are disabled")
}
