package server

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"nut-allergy/internal/policy"
	"nut-allergy/internal/store"
	"nut-allergy/internal/version"
)

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version": version.Version,
		"update":  s.updates.Available(version.Newer),
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		badJSON(w, err)
		return
	}
	if err := s.store.CheckPassword(body.Password); err != nil {
		writeErr(w, http.StatusUnauthorized, "incorrect password")
		return
	}
	id, err := s.store.CreateSession(time.Now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.setSession(w, r, id)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.store.DeleteSession(c.Value)
	}
	clearSession(w)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSetup()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": s.loggedIn(r),
		"setup":         st,
	})
}

func (s *Server) handleFleet(w http.ResponseWriter, r *http.Request) {
	ups, err := s.store.ListUPS()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	agents, err := s.fleetAgents()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	timeout, _ := s.store.TimeoutSeconds()
	writeJSON(w, http.StatusOK, map[string]any{
		"timeout_seconds": timeout,
		"ups":             ups,
		"agents":          agents,
	})
}

type fleetAgent struct {
	store.Agent
	UPSNames         []string   `json:"ups_names"`
	EffectiveTimeout int        `json:"effective_timeout_seconds"`
	OnBattery        bool       `json:"on_battery"`
	Shutdown         bool       `json:"shutdown"`
	Reason           string     `json:"reason"`
	Deadline         *time.Time `json:"deadline"`
}

func (s *Server) fleetAgents() ([]fleetAgent, error) {
	agents, err := s.store.ListAgents()
	if err != nil {
		return nil, err
	}
	ups, err := s.store.ListUPS()
	if err != nil {
		return nil, err
	}
	byID := map[string]store.UPS{}
	for _, u := range ups {
		byID[u.ID] = u
	}
	serverTimeout, err := s.store.TimeoutSeconds()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]fleetAgent, 0, len(agents))
	for _, a := range agents {
		fa := fleetAgent{Agent: a, UPSNames: []string{}}
		var supplies []policy.Supply
		for _, id := range a.UPSIDs {
			u, ok := byID[id]
			if !ok {
				continue
			}
			fa.UPSNames = append(fa.UPSNames, u.Name)
			supplies = append(supplies, policy.Supply{ID: u.ID, State: u.State, OnBatterySince: u.OnBatterySince})
		}
		wait := serverTimeout
		if a.TimeoutOverride != nil {
			wait = *a.TimeoutOverride
		}
		fa.EffectiveTimeout = wait
		d := policy.Decide(supplies, time.Duration(wait)*time.Second, now)
		fa.OnBattery = d.OnBattery
		fa.Shutdown = d.Shutdown
		fa.Reason = d.Reason
		fa.Deadline = d.Deadline
		out = append(out, fa)
	}
	return out, nil
}

func (s *Server) handleListUPS(w http.ResponseWriter, r *http.Request) {
	ups, err := s.store.ListUPS()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ups)
}

func (s *Server) handleCreateUPS(w http.ResponseWriter, r *http.Request) {
	u, err := s.createUPSFromRequest(w, r)
	if err != nil {
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) handleGetUPS(w http.ResponseWriter, r *http.Request) {
	u, err := s.store.GetUPS(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "ups not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleUpdateUPS(w http.ResponseWriter, r *http.Request) {
	var in store.UPSInput
	if err := readJSON(r, &in); err != nil {
		badJSON(w, err)
		return
	}
	if err := validateUPS(in, false); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := s.store.UpdateUPS(r.PathValue("id"), in)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "ups not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleDeleteUPS(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteUPS(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "ups not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.fleetAgents()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	agents, err := s.fleetAgents()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	id := r.PathValue("id")
	for _, a := range agents {
		if a.ID == id {
			writeJSON(w, http.StatusOK, a)
			return
		}
	}
	writeErr(w, http.StatusNotFound, "agent not found")
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteAgent(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePatchAgent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UPSIDs          []string `json:"ups_ids"`
		TimeoutOverride *int     `json:"timeout_override_seconds"`
	}
	if err := readJSON(r, &body); err != nil {
		badJSON(w, err)
		return
	}
	if body.UPSIDs == nil {
		body.UPSIDs = []string{}
	}
	if body.TimeoutOverride != nil && (*body.TimeoutOverride < 1 || *body.TimeoutOverride > 86400) {
		writeErr(w, http.StatusBadRequest, "timeout override must be between 1 and 86400 seconds")
		return
	}
	if err := s.store.SetAgentConfig(r.PathValue("id"), body.UPSIDs, body.TimeoutOverride); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.handleGetAgent(w, r)
}

func (s *Server) handleEnrollToken(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSetup()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !st.Complete || st.Hostname == "" {
		writeErr(w, http.StatusConflict, "finish setup first")
		return
	}
	token, exp, err := s.store.CreateEnrollToken(time.Now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	origin := s.publicHTTPS(st.Hostname)
	origin = origin[:len(origin)-1]
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":      token,
		"expires_at": exp,
		"command":    "curl -fsSL '" + origin + "/install/" + token + "' | sudo bash",
	})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSetup()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handlePatchSettings(w http.ResponseWriter, r *http.Request) {
	if err := s.writeTimeout(w, r); err != nil {
		return
	}
	s.handleGetSettings(w, r)
}
