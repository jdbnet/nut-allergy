package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type pollResponse struct {
	Hostname        string     `json:"hostname"`
	UPSIDs          []string   `json:"ups_ids"`
	TimeoutSeconds  int        `json:"timeout_seconds"`
	TimeoutOverride *int       `json:"timeout_override_seconds"`
	OnBattery       bool       `json:"on_battery"`
	Shutdown        bool       `json:"shutdown"`
	Reason          string     `json:"reason"`
	Deadline        *time.Time `json:"deadline"`
	ServerVersion   string     `json:"server_version"`
	Supplies        []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"supplies"`
}

type catalogResponse struct {
	UPS []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		State       string `json:"state"`
	} `json:"ups"`
	UPSIDs          []string `json:"ups_ids"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	TimeoutOverride *int     `json:"timeout_override_seconds"`
}

func httpClient(cfg agentConfig) (*http.Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		pair, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.Certificates = []tls.Certificate{pair}
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}, nil
}

func postJSON(client *http.Client, url string, body any, dst any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	res, err := client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", res.Status, bytes.TrimSpace(raw))
	}
	if dst == nil {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func getJSON(client *http.Client, url string, dst any) error {
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", res.Status, bytes.TrimSpace(raw))
	}
	return json.Unmarshal(raw, dst)
}
