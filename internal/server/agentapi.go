package server

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"nut-allergy/internal/pki"
	"nut-allergy/internal/policy"
	"nut-allergy/internal/store"
	"nut-allergy/internal/version"
)

type agentContextKey struct{}

func (s *Server) requireAgent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := s.agentFromTLS(r)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, err.Error())
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), agentContextKey{}, a)))
	}
}

func agentFrom(ctx context.Context) store.Agent {
	a, _ := ctx.Value(agentContextKey{}).(store.Agent)
	return a
}

func (s *Server) agentFromTLS(r *http.Request) (store.Agent, error) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return store.Agent{}, errors.New("client certificate required")
	}
	ca := s.ca.Load()
	if ca == nil {
		if err := s.loadCA(); err != nil {
			return store.Agent{}, errors.New("agent ca is not ready")
		}
		ca = s.ca.Load()
	}
	cert := r.TLS.PeerCertificates[0]
	if _, err := cert.Verify(x509.VerifyOptions{
		Roots:     ca.Pool(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		return store.Agent{}, errors.New("client certificate rejected")
	}
	a, err := s.store.AgentByFingerprint(pki.Fingerprint(cert))
	if err != nil {
		return store.Agent{}, errors.New("unknown agent")
	}
	return a, nil
}

func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSetup()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !st.Complete {
		writeErr(w, http.StatusConflict, "server setup is not complete")
		return
	}
	var body struct {
		Token    string `json:"token"`
		Hostname string `json:"hostname"`
		CSR      string `json:"csr_pem"`
	}
	if err := readJSON(r, &body); err != nil {
		badJSON(w, err)
		return
	}
	if body.Hostname == "" || body.CSR == "" || body.Token == "" {
		writeErr(w, http.StatusBadRequest, "token, hostname, and csr are required")
		return
	}
	now := time.Now()
	if err := s.store.ConsumeEnrollToken(body.Token, now); err != nil {
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	ca, err := s.ensureCA(now)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	certPEM, fp, err := ca.SignCSR(body.CSR, body.Hostname, now)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.store.UpsertAgent(body.Hostname, fp, now); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"cert_pem": certPEM})
}

func (s *Server) handlePoll(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r.Context())
	var body struct {
		Hostname string `json:"hostname"`
	}
	if err := readJSON(r, &body); err != nil {
		badJSON(w, err)
		return
	}
	if body.Hostname == "" {
		body.Hostname = a.Hostname
	}
	now := time.Now()
	if err := s.store.TouchAgent(a.ID, body.Hostname, now); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	fresh, err := s.store.GetAgent(a.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.decisionFor(fresh, now))
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	ups, err := s.store.ListUPS()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type item struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		State       string `json:"state"`
	}
	out := make([]item, 0, len(ups))
	for _, u := range ups {
		out = append(out, item{ID: u.ID, Name: u.Name, Description: u.Description, State: u.State})
	}
	a := agentFrom(r.Context())
	timeout, _ := s.store.TimeoutSeconds()
	writeJSON(w, http.StatusOK, map[string]any{
		"ups":                      out,
		"ups_ids":                  a.UPSIDs,
		"timeout_seconds":          timeout,
		"timeout_override_seconds": a.TimeoutOverride,
	})
}

func (s *Server) handleAgentConfig(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r.Context())
	var body struct {
		UPSIDs          []string        `json:"ups_ids"`
		TimeoutOverride json.RawMessage `json:"timeout_override_seconds"`
	}
	if err := readJSON(r, &body); err != nil {
		badJSON(w, err)
		return
	}
	if body.UPSIDs == nil {
		body.UPSIDs = []string{}
	}
	var override *int
	if len(body.TimeoutOverride) > 0 && string(body.TimeoutOverride) != "null" {
		var n int
		if err := json.Unmarshal(body.TimeoutOverride, &n); err != nil || n < 1 || n > 86400 {
			writeErr(w, http.StatusBadRequest, "timeout override must be between 1 and 86400 seconds")
			return
		}
		override = &n
	}
	if err := s.store.SetAgentConfig(a.ID, body.UPSIDs, override); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	fresh, err := s.store.GetAgent(a.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.decisionFor(fresh, time.Now()))
}

type supplyView struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	State          string     `json:"state"`
	OnBatterySince *time.Time `json:"on_battery_since"`
}

type decisionView struct {
	Hostname        string       `json:"hostname"`
	UPSIDs          []string     `json:"ups_ids"`
	TimeoutSeconds  int          `json:"timeout_seconds"`
	TimeoutOverride *int         `json:"timeout_override_seconds"`
	Supplies        []supplyView `json:"supplies"`
	OnBattery       bool         `json:"on_battery"`
	Shutdown        bool         `json:"shutdown"`
	Reason          string       `json:"reason"`
	Deadline        *time.Time   `json:"deadline"`
	ServerVersion   string       `json:"server_version"`
}

func (s *Server) decisionFor(a store.Agent, now time.Time) decisionView {
	serverTimeout, _ := s.store.TimeoutSeconds()
	wait := serverTimeout
	if a.TimeoutOverride != nil {
		wait = *a.TimeoutOverride
	}
	view := decisionView{
		Hostname:        a.Hostname,
		UPSIDs:          a.UPSIDs,
		TimeoutSeconds:  wait,
		TimeoutOverride: a.TimeoutOverride,
		Supplies:        []supplyView{},
		ServerVersion:   version.Version,
	}
	var supplies []policy.Supply
	for _, id := range a.UPSIDs {
		u, err := s.store.GetUPS(id)
		if err != nil {
			continue
		}
		view.Supplies = append(view.Supplies, supplyView{ID: u.ID, Name: u.Name, State: u.State, OnBatterySince: u.OnBatterySince})
		supplies = append(supplies, policy.Supply{ID: u.ID, State: u.State, OnBatterySince: u.OnBatterySince})
	}
	d := policy.Decide(supplies, time.Duration(wait)*time.Second, now)
	view.OnBattery = d.OnBattery
	view.Shutdown = d.Shutdown
	view.Reason = d.Reason
	view.Deadline = d.Deadline
	return view
}
