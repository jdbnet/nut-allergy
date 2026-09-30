package server

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"nut-allergy/internal/alert"
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
		if err := alert.SendMail(cfg, alert.EmailSubject(payload), alert.EmailBody(payload)); err != nil {
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
		body, err := alert.FormatWebhookBody(h.Format, payload)
		if err != nil {
			log.Printf("webhook %s body: %v", h.ID, err)
			continue
		}
		if err := alert.PostWebhook(ctx, h.URL, body); err != nil {
			log.Printf("webhook %s %s: %v", h.ID, upsName, err)
		}
	}
}

func (n *notifyEngine) sendTestEmail() error {
	smtpCfg, err := n.store.GetSMTPSecret()
	if err != nil {
		return err
	}
	if !smtpCfg.Enabled {
		return errNotEnabled("smtp")
	}
	now := time.Now()
	tr := alert.Transition{Kind: alert.KindOnBattery, FromState: snmp.StateOnline, ToState: snmp.StateOnBattery, OccurredAt: now}
	payload := alert.BuildPayload("test", "Test UPS", tr, snmp.Reading{}, now)
	cfg := alert.SMTPConfig{
		Host:     smtpCfg.Host,
		Port:     smtpCfg.Port,
		TLSMode:  smtpCfg.TLSMode,
		Username: smtpCfg.Username,
		Password: smtpCfg.Password,
		From:     smtpCfg.FromAddr,
		To:       alert.ParseRecipients(smtpCfg.ToAddrs),
	}
	return alert.SendMail(cfg, "UPS alert test: "+payload.UPSName, alert.EmailBody(payload))
}

func (n *notifyEngine) sendTestWebhook(id string) error {
	url, err := n.store.WebhookURLForTest(id)
	if err != nil {
		return err
	}
	var format string
	hooks, err := n.store.ListWebhooks()
	if err != nil {
		return err
	}
	for _, h := range hooks {
		if h.ID == id {
			format = h.Format
			break
		}
	}
	now := time.Now()
	tr := alert.Transition{Kind: alert.KindOnMains, FromState: snmp.StateOnBattery, ToState: snmp.StateOnline, OccurredAt: now}
	payload := alert.BuildPayload("test", "Test UPS", tr, snmp.Reading{}, now)
	body, err := alert.FormatWebhookBody(format, payload)
	if err != nil {
		return err
	}
	return alert.PostWebhook(context.Background(), url, body)
}

func errNotEnabled(channel string) error {
	return errString(channel + " alerts are disabled")
}
