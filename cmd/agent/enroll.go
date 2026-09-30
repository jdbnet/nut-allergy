package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func enroll(configPath, serverURL, token string) error {
	serverURL = strings.TrimRight(serverURL, "/")
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: hostname},
		DNSNames: []string{hostname},
	}, key)
	if err != nil {
		return err
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	dir := filepath.Dir(configPath)
	certFile, keyFile := defaultPaths(dir)
	cfg := agentConfig{Server: serverURL, Hostname: hostname, CertFile: certFile, KeyFile: keyFile, UPSIDs: []string{}}
	if existing, err := loadConfig(configPath); err == nil {
		cfg.UPSIDs = existing.UPSIDs
		cfg.TimeoutOverride = existing.TimeoutOverride
		cfg.OnBattery = existing.OnBattery
		cfg.Deadline = existing.Deadline
	}
	client, err := httpClient(agentConfig{Server: serverURL})
	if err != nil {
		return err
	}
	var resp struct {
		CertPEM string `json:"cert_pem"`
	}
	if err := postJSON(client, serverURL+"/api/agent/enroll", map[string]string{
		"token":    token,
		"hostname": hostname,
		"csr_pem":  string(csrPEM),
	}, &resp); err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(certFile, []byte(resp.CertPEM), 0o644); err != nil {
		return err
	}
	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}
	fmt.Printf("enrolled %s with %s\n", hostname, serverURL)
	return nil
}
