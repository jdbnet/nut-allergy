package server

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strings"

	"nut-allergy/internal/alert/webhooks"
	"nut-allergy/internal/store"
)

func (s *Server) handleGetAlerts(w http.ResponseWriter, r *http.Request) {
	smtp, err := s.store.GetSMTP()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	timing, err := s.store.GetAlertTiming()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	webhooks, err := s.store.ListWebhooks()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"smtp":     smtp,
		"timing":   timing,
		"webhooks": webhooks,
	})
}

func (s *Server) handlePutAlerts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SMTP   store.SMTPSettings `json:"smtp"`
		Timing store.AlertTiming  `json:"timing"`
		Password string           `json:"smtp_password"`
	}
	if err := readJSON(r, &body); err != nil {
		badJSON(w, err)
		return
	}
	if body.SMTP.Port < 1 || body.SMTP.Port > 65535 {
		writeErr(w, http.StatusBadRequest, "smtp port must be between 1 and 65535")
		return
	}
	switch body.SMTP.TLSMode {
	case "none", "starttls", "tls", "":
		if body.SMTP.TLSMode == "" {
			body.SMTP.TLSMode = "starttls"
		}
	default:
		writeErr(w, http.StatusBadRequest, "tls_mode must be none, starttls, or tls")
		return
	}
	if err := s.store.PutSMTP(body.SMTP, body.Password); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.SetAlertTiming(body.Timing); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.notify.reloadTracker()
	s.handleGetAlerts(w, r)
}

func (s *Server) handleTestAlertEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SMTP         store.SMTPSettings `json:"smtp"`
		SMTPPassword string             `json:"smtp_password"`
	}
	if err := readJSON(r, &body); err != nil && !errors.Is(err, io.EOF) {
		badJSON(w, err)
		return
	}
	var cfg store.SMTPSettingsSecret
	var err error
	if body.SMTP.Host != "" || body.SMTP.FromAddr != "" || body.SMTP.ToAddrs != "" {
		cfg, err = s.notify.resolveSMTPSecret(body.SMTP, body.SMTPPassword)
		cfg.SMTPSettings = body.SMTP
	} else {
		cfg, err = s.store.GetSMTPSecret()
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
		return
	}
	if err := s.notify.sendTestEmail(cfg); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Test email sent."})
}

func (s *Server) handleWebhookTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, webhooks.List())
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := s.store.ListWebhooks()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, hooks)
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL     string `json:"url"`
		Format  string `json:"format"`
		Enabled *bool  `json:"enabled"`
	}
	if err := readJSON(r, &body); err != nil {
		badJSON(w, err)
		return
	}
	if body.URL == "" {
		writeErr(w, http.StatusBadRequest, "url is required")
		return
	}
	format := webhooks.Normalize(body.Format)
	if !validWebhookTemplate(format) {
		writeErr(w, http.StatusBadRequest, "unknown webhook template")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	id, err := s.store.NewWebhookID()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.CreateWebhook(id, body.URL, format, enabled); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	hooks, err := s.store.ListWebhooks()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, h := range hooks {
		if h.ID == id {
			writeJSON(w, http.StatusCreated, h)
			return
		}
	}
	writeErr(w, http.StatusInternalServerError, "webhook missing after create")
}

func (s *Server) handlePatchWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL     string `json:"url"`
		Format  string `json:"format"`
		Enabled *bool  `json:"enabled"`
	}
	if err := readJSON(r, &body); err != nil {
		badJSON(w, err)
		return
	}
	id := r.PathValue("id")
	hooks, err := s.store.ListWebhooks()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var cur store.WebhookSettings
	found := false
	for _, h := range hooks {
		if h.ID == id {
			cur = h
			found = true
			break
		}
	}
	if !found {
		writeErr(w, http.StatusNotFound, "webhook not found")
		return
	}
	enabled := cur.Enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	format := cur.Format
	if body.Format != "" {
		format = webhooks.Normalize(body.Format)
		if !validWebhookTemplate(format) {
			writeErr(w, http.StatusBadRequest, "unknown webhook template")
			return
		}
	}
	if err := s.store.UpdateWebhook(id, enabled, format, body.URL); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "webhook not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	hooks, err = s.store.ListWebhooks()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, h := range hooks {
		if h.ID == id {
			writeJSON(w, http.StatusOK, h)
			return
		}
	}
	writeErr(w, http.StatusNotFound, "webhook not found")
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteWebhook(r.PathValue("id")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "webhook not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestWebhookDraft(w http.ResponseWriter, r *http.Request) {
	s.executeWebhookTest(w, r, "")
}

func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	s.executeWebhookTest(w, r, r.PathValue("id"))
}

func (s *Server) executeWebhookTest(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		URL    string `json:"url"`
		Format string `json:"format"`
		Event  string `json:"event"`
	}
	if err := readJSON(r, &body); err != nil && !errors.Is(err, io.EOF) {
		badJSON(w, err)
		return
	}
	url := strings.TrimSpace(body.URL)
	format := webhooks.Normalize(body.Format)
	event := body.Event
	if event == "" {
		event = "test"
	}
	if url == "" && id != "" {
		stored, err := s.store.WebhookURLForTest(id)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
			return
		}
		url = stored
	}
	if body.Format == "" && id != "" {
		hooks, err := s.store.ListWebhooks()
		if err == nil {
			for _, h := range hooks {
				if h.ID == id {
					format = webhooks.Normalize(h.Format)
					break
				}
			}
		}
	}
	res, err := s.notify.testWebhook(url, format, event)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
		return
	}
	ok := res.StatusCode >= 200 && res.StatusCode < 300
	out := map[string]any{
		"success":     ok,
		"status_code": res.StatusCode,
		"response":    res.Body,
	}
	if !ok {
		out["error"] = "webhook returned " + http.StatusText(res.StatusCode)
	}
	writeJSON(w, http.StatusOK, out)
}

func validWebhookTemplate(format string) bool {
	for _, t := range webhooks.List() {
		if t.ID == format {
			return true
		}
	}
	return false
}
