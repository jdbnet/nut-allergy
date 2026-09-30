package server

import (
	"database/sql"
	"errors"
	"net/http"

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
	if err := s.notify.sendTestEmail(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
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
	format := body.Format
	if format == "" {
		format = "generic"
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
		format = body.Format
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

func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	if err := s.notify.sendTestWebhook(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}
