package server

import (
	"net"
	"net/http"
	"strings"
	"time"

	"nut-allergy/internal/acme"
	"nut-allergy/internal/pki"
	"nut-allergy/internal/snmp"
	"nut-allergy/internal/store"
)

func (s *Server) handleSetupState(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSetup()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleSetupPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		badJSON(w, err)
		return
	}
	if len(body.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if err := s.store.SetPassword(body.Password); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
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

func (s *Server) handleSetupHostname(w http.ResponseWriter, r *http.Request) {
	if s.setupDone(w) {
		return
	}
	var body struct {
		Hostname string `json:"hostname"`
	}
	if err := readJSON(r, &body); err != nil {
		badJSON(w, err)
		return
	}
	host := strings.TrimSpace(strings.ToLower(body.Hostname))
	if !validHostname(host) {
		writeErr(w, http.StatusBadRequest, "enter a DNS hostname such as ups.example.com")
		return
	}
	if err := s.store.SetHostname(host); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"hostname": host})
}

type certRequest struct {
	Mode     string `json:"mode"`
	Provider string `json:"provider"`
	Token    string `json:"token"`
	Email    string `json:"email"`
	CertPEM  string `json:"cert_pem"`
	KeyPEM   string `json:"key_pem"`
}

func (s *Server) handleSetupCertificate(w http.ResponseWriter, r *http.Request) {
	if s.setupDone(w) {
		return
	}
	s.applyCertificate(w, r, true)
}

func (s *Server) handleSettingsCertificate(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSetup()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !st.Complete {
		writeErr(w, http.StatusConflict, "finish setup first")
		return
	}
	s.applyCertificate(w, r, false)
}

func (s *Server) applyCertificate(w http.ResponseWriter, r *http.Request, wizard bool) {
	var body certRequest
	if err := readJSON(r, &body); err != nil {
		badJSON(w, err)
		return
	}
	st, err := s.store.GetSetup()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if st.Hostname == "" {
		writeErr(w, http.StatusBadRequest, "set a hostname first")
		return
	}
	switch body.Mode {
	case "upload":
		if err := pki.ParseCertAndKey(body.CertPEM, body.KeyPEM, st.Hostname); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.store.PutCert("upload", body.CertPEM, body.KeyPEM); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := s.useCert(body.CertPEM, body.KeyPEM); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	case "lego":
		if err := s.issueLego(st.Hostname, body); err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
	default:
		writeErr(w, http.StatusBadRequest, "mode must be upload or lego")
		return
	}
	resp := map[string]string{"ok": "true"}
	if wizard {
		token, err := s.store.CreateContinueToken(time.Now())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp["https_url"] = s.publicHTTPS(st.Hostname)
		resp["continue_token"] = token
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) issueLego(hostname string, body certRequest) error {
	provider := body.Provider
	token := body.Token
	email := strings.TrimSpace(body.Email)
	if provider == "" || token == "" {
		p, t, e, err := s.store.DNS()
		if err != nil {
			return err
		}
		if provider == "" {
			provider = p
		}
		if token == "" {
			token = t
		}
		if email == "" {
			email = e
		}
	}
	if !acme.KnownProvider(provider) {
		return errString("choose a supported dns provider")
	}
	if token == "" || !strings.Contains(email, "@") {
		return errString("dns token and an email address are required")
	}
	if err := s.store.SetDNS(provider, token, email); err != nil {
		return err
	}
	accountKey, reg, _ := s.store.ACME()
	resource, _ := s.store.ACMEResource()
	if resource != "" {
		cur, _, _, err := s.store.Cert()
		if err != nil || !pki.ValidForHostname(cur, hostname) {
			resource = ""
		}
	}
	mat, err := acme.Issue(email, hostname, provider, token, accountKey, reg, resource)
	if err != nil {
		return err
	}
	if err := s.store.SetACME(mat.AccountKey, mat.Registration); err != nil {
		return err
	}
	if err := s.store.SetACMEResource(mat.Resource); err != nil {
		return err
	}
	if err := s.store.PutCert("lego", mat.CertPEM, mat.KeyPEM); err != nil {
		return err
	}
	return s.useCert(mat.CertPEM, mat.KeyPEM)
}

func (s *Server) handleSetupTimeout(w http.ResponseWriter, r *http.Request) {
	if s.setupDone(w) {
		return
	}
	if err := s.writeTimeout(w, r); err != nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (s *Server) writeTimeout(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Seconds int `json:"seconds"`
	}
	if err := readJSON(r, &body); err != nil {
		badJSON(w, err)
		return err
	}
	if body.Seconds < 1 || body.Seconds > 86400 {
		writeErr(w, http.StatusBadRequest, "timeout must be between 1 and 86400 seconds")
		return errString("timeout")
	}
	if err := s.store.SetTimeout(body.Seconds); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return err
	}
	return nil
}

func (s *Server) handleSetupUPS(w http.ResponseWriter, r *http.Request) {
	if s.setupDone(w) {
		return
	}
	u, err := s.createUPSFromRequest(w, r)
	if err != nil {
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) handleSetupFinish(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSetup()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if st.Complete {
		writeJSON(w, http.StatusOK, map[string]bool{"complete": true})
		return
	}
	if !st.HasPassword || st.Hostname == "" || !st.HasCert || st.TimeoutSeconds < 1 {
		writeErr(w, http.StatusBadRequest, "password, hostname, certificate, and timeout are required")
		return
	}
	if _, err := s.ensureCA(time.Now()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.MarkComplete(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"complete": true})
}

func (s *Server) handleSetupContinue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := readJSON(r, &body); err != nil {
		badJSON(w, err)
		return
	}
	if err := s.store.ConsumeContinueToken(body.Token, time.Now()); err != nil {
		writeErr(w, http.StatusUnauthorized, err.Error())
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

func (s *Server) handleDNSProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, acme.Providers())
}

func (s *Server) handleSNMPProtocols(w http.ResponseWriter, r *http.Request) {
	auth, priv := snmp.Protocols()
	writeJSON(w, http.StatusOK, map[string][]string{"auth": auth, "priv": priv})
}

func (s *Server) setupDone(w http.ResponseWriter) bool {
	st, err := s.store.GetSetup()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return true
	}
	if st.Complete {
		writeErr(w, http.StatusConflict, "setup is already complete")
		return true
	}
	return false
}

func (s *Server) publicHTTPS(hostname string) string {
	_, port, err := net.SplitHostPort(s.httpsAddr)
	if err != nil || port == "" || port == "443" {
		return "https://" + hostname + "/"
	}
	return "https://" + hostname + ":" + port + "/"
}

func validHostname(h string) bool {
	if len(h) < 1 || len(h) > 253 || strings.ContainsAny(h, " /\\") {
		return false
	}
	if !strings.Contains(h, ".") || strings.HasPrefix(h, ".") || strings.HasSuffix(h, ".") || strings.Contains(h, "..") {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return true
}

type errString string

func (e errString) Error() string { return string(e) }

func (s *Server) createUPSFromRequest(w http.ResponseWriter, r *http.Request) (store.UPS, error) {
	var in store.UPSInput
	if err := readJSON(r, &in); err != nil {
		badJSON(w, err)
		return store.UPS{}, err
	}
	if err := validateUPS(in, true); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return store.UPS{}, err
	}
	u, err := s.store.CreateUPS(in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return store.UPS{}, err
	}
	return u, nil
}

func validateUPS(in store.UPSInput, needSecret bool) error {
	t := snmp.Target{
		Host:          strings.TrimSpace(in.Host),
		User:          in.Username,
		SecurityLevel: in.SecLevel,
		AuthProtocol:  in.AuthProtocol,
		AuthPassword:  in.AuthPassword,
		PrivProtocol:  in.PrivProtocol,
		PrivPassword:  in.PrivPassword,
	}
	if !needSecret && t.AuthPassword == "" {
		t.AuthPassword = "placeholder-secret"
	}
	if !needSecret && t.SecurityLevel == "authPriv" && t.PrivPassword == "" {
		t.PrivPassword = "placeholder-secret"
	}
	if strings.TrimSpace(in.Name) == "" {
		return errString("name is required")
	}
	return snmp.ValidateTarget(t)
}
