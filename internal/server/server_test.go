package server

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"nut-allergy/internal/secret"
	"nut-allergy/internal/snmp"
	"nut-allergy/internal/store"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	box, err := secret.Open(filepath.Join(dir, "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "server.db"), box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st, dir, ":443", ":8080")
}

func TestWizardUploadAndAgentPoll(t *testing.T) {
	s := testServer(t)
	certPEM, keyPEM := serverCert(t, "ups.test")
	plain := httptest.NewServer(s.Handler())
	defer plain.Close()

	var session string
	post(t, plain.Client(), plain.URL+"/api/setup/password", map[string]string{"password": "correct-horse"}, &session)
	postAuth(t, plain.Client(), plain.URL+"/api/setup/hostname", session, map[string]string{"hostname": "ups.test"})
	postAuth(t, plain.Client(), plain.URL+"/api/setup/certificate", session, map[string]string{
		"mode": "upload", "cert_pem": certPEM, "key_pem": keyPEM,
	})
	postAuth(t, plain.Client(), plain.URL+"/api/setup/timeout", session, map[string]int{"seconds": 1800})
	postAuth(t, plain.Client(), plain.URL+"/api/setup/finish", session, map[string]any{})

	pair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		t.Fatal(err)
	}
	tlsSrv := httptest.NewUnstartedServer(s.Handler())
	tlsSrv.TLS = &tls.Config{
		Certificates: []tls.Certificate{pair},
		ClientAuth:   tls.RequestClientCert,
		MinVersion:   tls.VersionTLS12,
	}
	tlsSrv.StartTLS()
	defer tlsSrv.Close()

	pool := x509.NewCertPool()
	block, _ := pem.Decode([]byte(certPEM))
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pool.AddCert(parsed)
	admin := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs:    pool,
		ServerName: "ups.test",
		MinVersion: tls.VersionTLS12,
	}}}
	login := post(t, admin, tlsSrv.URL+"/api/login", map[string]string{"password": "correct-horse"}, nil)

	var created store.UPS
	doJSON(t, admin, http.MethodPost, tlsSrv.URL+"/api/ups", login, map[string]string{
		"name": "rack-a", "description": "top", "host": "10.0.0.8",
		"sec_level": "authNoPriv", "username": "nut", "auth_protocol": "SHA", "auth_password": "secretsecret",
	}, &created)
	var other store.UPS
	doJSON(t, admin, http.MethodPost, tlsSrv.URL+"/api/ups", login, map[string]string{
		"name": "rack-b", "description": "bottom", "host": "10.0.0.9",
		"sec_level": "authNoPriv", "username": "nut", "auth_protocol": "SHA", "auth_password": "secretsecret",
	}, &other)

	past := time.Now().Add(-time.Hour)
	if err := s.store.ApplyReading(created.ID, snmp.Reading{State: snmp.StateOnBattery}, past); err != nil {
		t.Fatal(err)
	}
	if err := s.store.ApplyReading(other.ID, snmp.Reading{State: snmp.StateOnline}, past); err != nil {
		t.Fatal(err)
	}

	var tokenResp struct {
		Token   string `json:"token"`
		Command string `json:"command"`
	}
	doJSON(t, admin, http.MethodPost, tlsSrv.URL+"/api/enroll-tokens", login, map[string]any{}, &tokenResp)
	if tokenResp.Command == "" || tokenResp.Token == "" {
		t.Fatal("missing install command")
	}
	res, err := admin.Get(tlsSrv.URL + "/install/" + tokenResp.Token)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !bytes.Contains(body, []byte(tokenResp.Token)) {
		t.Fatalf("install script status %d body %s", res.StatusCode, body)
	}

	agentKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: "host-a"},
		DNSNames: []string{"host-a"},
	}, agentKey)
	if err != nil {
		t.Fatal(err)
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	var enrolled struct {
		CertPEM string `json:"cert_pem"`
	}
	doJSON(t, admin, http.MethodPost, tlsSrv.URL+"/api/agent/enroll", "", map[string]string{
		"token": tokenResp.Token, "hostname": "host-a", "csr_pem": string(csrPEM),
	}, &enrolled)

	agentPair, err := tls.X509KeyPair([]byte(enrolled.CertPEM), pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: mustPKCS8(t, agentKey),
	}))
	if err != nil {
		t.Fatal(err)
	}
	agentClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs:      pool,
		ServerName:   "ups.test",
		Certificates: []tls.Certificate{agentPair},
		MinVersion:   tls.VersionTLS12,
	}}}
	doJSON(t, agentClient, http.MethodPut, tlsSrv.URL+"/api/agent/config", "", map[string]any{
		"ups_ids": []string{created.ID, other.ID}, "timeout_override_seconds": nil,
	}, nil)
	var decision map[string]any
	doJSON(t, agentClient, http.MethodPost, tlsSrv.URL+"/api/agent/poll", "", map[string]string{"hostname": "host-a"}, &decision)
	if decision["shutdown"] != false || decision["on_battery"] != false {
		t.Fatalf("one supply on utility should stay up: %#v", decision)
	}

	if err := s.store.ApplyReading(other.ID, snmp.Reading{State: snmp.StateLowBattery}, past); err != nil {
		t.Fatal(err)
	}
	doJSON(t, agentClient, http.MethodPost, tlsSrv.URL+"/api/agent/poll", "", map[string]string{"hostname": "host-a"}, &decision)
	if decision["shutdown"] != true || decision["reason"] != "low battery" {
		t.Fatalf("low battery with every supply on battery should shut down: %#v", decision)
	}

	var fleet struct {
		Agents []struct {
			Hostname string `json:"hostname"`
		} `json:"agents"`
	}
	doJSON(t, admin, http.MethodGet, tlsSrv.URL+"/api/fleet", login, nil, &fleet)
	if len(fleet.Agents) != 1 || fleet.Agents[0].Hostname != "host-a" {
		t.Fatalf("fleet agents: %+v", fleet.Agents)
	}
}

func serverCert(t *testing.T, host string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(7),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: mustPKCS8(t, key)})
	return string(certPEM), string(keyPEM)
}

func mustPKCS8(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func post(t *testing.T, c *http.Client, url string, body any, cookie *string) string {
	t.Helper()
	return doJSON(t, c, http.MethodPost, url, "", body, nil, cookie)
}

func postAuth(t *testing.T, c *http.Client, url, cookie string, body any) {
	t.Helper()
	doJSON(t, c, http.MethodPost, url, cookie, body, nil)
}

func doJSON(t *testing.T, c *http.Client, method, url, cookie string, body any, dst any, cookieOut ...*string) string {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		t.Fatalf("%s %s: %s %s", method, url, res.Status, raw)
	}
	if dst != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, dst); err != nil {
			t.Fatal(err)
		}
	}
	if len(cookieOut) > 0 && cookieOut[0] != nil {
		for _, ck := range res.Cookies() {
			if ck.Name == sessionCookie {
				*cookieOut[0] = ck.Value
			}
		}
	}
	for _, ck := range res.Cookies() {
		if ck.Name == sessionCookie {
			return ck.Value
		}
	}
	return cookie
}
