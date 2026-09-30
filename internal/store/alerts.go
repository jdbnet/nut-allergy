package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func alertsDDL() string {
	return `
CREATE TABLE IF NOT EXISTS alert_smtp (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  enabled INTEGER NOT NULL DEFAULT 0,
  host TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL DEFAULT 587,
  tls_mode TEXT NOT NULL DEFAULT 'starttls',
  username TEXT NOT NULL DEFAULT '',
  password BLOB NOT NULL DEFAULT '',
  from_addr TEXT NOT NULL DEFAULT '',
  to_addrs TEXT NOT NULL DEFAULT ''
);
INSERT OR IGNORE INTO alert_smtp(id) VALUES(1);
CREATE TABLE IF NOT EXISTS alert_webhooks (
  id TEXT PRIMARY KEY,
  enabled INTEGER NOT NULL DEFAULT 1,
  url_enc BLOB NOT NULL,
  format TEXT NOT NULL DEFAULT 'generic'
);
`
}

// SMTPSettings is outbound email configuration.
type SMTPSettings struct {
	Enabled      bool   `json:"enabled"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	TLSMode      string `json:"tls_mode"`
	Username     string `json:"username"`
	FromAddr     string `json:"from_addr"`
	ToAddrs      string `json:"to_addrs"`
	PasswordSet  bool   `json:"password_set"`
}

// SMTPSettingsSecret includes the decrypted password for sending.
type SMTPSettingsSecret struct {
	SMTPSettings
	Password string
}

// WebhookSettings is one webhook endpoint (URL is not returned).
type WebhookSettings struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Format  string `json:"format"`
	URLSet  bool   `json:"url_set"`
}

// WebhookSecret includes the decrypted URL.
type WebhookSecret struct {
	WebhookSettings
	URL string
}

// AlertTiming holds debounce and cooldown seconds.
type AlertTiming struct {
	DebounceSeconds int `json:"debounce_seconds"`
	CooldownSeconds int `json:"cooldown_seconds"`
}

const (
	metaAlertDebounce  = "alert_debounce_seconds"
	metaAlertCooldown  = "alert_cooldown_seconds"
	defaultDebounceSec = 30
	defaultCooldownSec = 300
)

// GetAlertTiming returns debounce and cooldown settings.
func (s *Store) GetAlertTiming() (AlertTiming, error) {
	out := AlertTiming{DebounceSeconds: defaultDebounceSec, CooldownSeconds: defaultCooldownSec}
	if v, ok, err := s.meta(metaAlertDebounce); err != nil {
		return out, err
	} else if ok {
		fmt.Sscanf(v, "%d", &out.DebounceSeconds)
	}
	if v, ok, err := s.meta(metaAlertCooldown); err != nil {
		return out, err
	} else if ok {
		fmt.Sscanf(v, "%d", &out.CooldownSeconds)
	}
	if out.DebounceSeconds < 1 {
		out.DebounceSeconds = defaultDebounceSec
	}
	if out.CooldownSeconds < 0 {
		out.CooldownSeconds = defaultCooldownSec
	}
	return out, nil
}

// SetAlertTiming stores debounce and cooldown.
func (s *Store) SetAlertTiming(t AlertTiming) error {
	if t.DebounceSeconds < 1 || t.DebounceSeconds > 3600 {
		return errors.New("debounce must be between 1 and 3600 seconds")
	}
	if t.CooldownSeconds < 0 || t.CooldownSeconds > 86400 {
		return errors.New("cooldown must be between 0 and 86400 seconds")
	}
	if err := s.setMeta(metaAlertDebounce, fmt.Sprintf("%d", t.DebounceSeconds)); err != nil {
		return err
	}
	return s.setMeta(metaAlertCooldown, fmt.Sprintf("%d", t.CooldownSeconds))
}

// GetSMTP returns SMTP settings without the password.
func (s *Store) GetSMTP() (SMTPSettings, error) {
	var out SMTPSettings
	var enabled int
	var pass []byte
	err := s.db.QueryRow(`SELECT enabled, host, port, tls_mode, username, password, from_addr, to_addrs FROM alert_smtp WHERE id = 1`).
		Scan(&enabled, &out.Host, &out.Port, &out.TLSMode, &out.Username, &pass, &out.FromAddr, &out.ToAddrs)
	if err != nil {
		return out, err
	}
	out.Enabled = enabled != 0
	out.PasswordSet = len(pass) > 0
	return out, nil
}

// GetSMTPSecret loads SMTP settings including the decrypted password.
func (s *Store) GetSMTPSecret() (SMTPSettingsSecret, error) {
	pub, err := s.GetSMTP()
	if err != nil {
		return SMTPSettingsSecret{}, err
	}
	var pass []byte
	err = s.db.QueryRow(`SELECT password FROM alert_smtp WHERE id = 1`).Scan(&pass)
	if err != nil {
		return SMTPSettingsSecret{}, err
	}
	plain := ""
	if len(pass) > 0 {
		p, err := s.box.Open(pass)
		if err != nil {
			return SMTPSettingsSecret{}, err
		}
		plain = string(p)
	}
	return SMTPSettingsSecret{SMTPSettings: pub, Password: plain}, nil
}

// PutSMTP stores SMTP settings. Empty password keeps the existing secret.
func (s *Store) PutSMTP(in SMTPSettings, password string) error {
	curPass := []byte{}
	_ = s.db.QueryRow(`SELECT password FROM alert_smtp WHERE id = 1`).Scan(&curPass)
	if password != "" {
		sealed, err := s.box.Seal([]byte(password))
		if err != nil {
			return err
		}
		curPass = sealed
	}
	enabled := 0
	if in.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(`UPDATE alert_smtp SET enabled=?, host=?, port=?, tls_mode=?, username=?, password=?, from_addr=?, to_addrs=? WHERE id = 1`,
		enabled, strings.TrimSpace(in.Host), in.Port, in.TLSMode, in.Username, curPass, strings.TrimSpace(in.FromAddr), strings.TrimSpace(in.ToAddrs))
	return err
}

// ListWebhooks returns webhook rows without URLs.
func (s *Store) ListWebhooks() ([]WebhookSettings, error) {
	rows, err := s.db.Query(`SELECT id, enabled, url_enc, format FROM alert_webhooks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WebhookSettings
	for rows.Next() {
		var w WebhookSettings
		var enabled int
		var enc []byte
		if err := rows.Scan(&w.ID, &enabled, &enc, &w.Format); err != nil {
			return nil, err
		}
		w.Enabled = enabled != 0
		w.URLSet = len(enc) > 0
		out = append(out, w)
	}
	if out == nil {
		out = []WebhookSettings{}
	}
	return out, rows.Err()
}

// ListWebhookSecrets returns enabled webhooks with decrypted URLs.
func (s *Store) ListWebhookSecrets() ([]WebhookSecret, error) {
	pub, err := s.ListWebhooks()
	if err != nil {
		return nil, err
	}
	out := make([]WebhookSecret, 0, len(pub))
	for _, w := range pub {
		var enc []byte
		if err := s.db.QueryRow(`SELECT url_enc FROM alert_webhooks WHERE id = ?`, w.ID).Scan(&enc); err != nil {
			return nil, err
		}
		url := ""
		if len(enc) > 0 {
			plain, err := s.box.Open(enc)
			if err != nil {
				return nil, err
			}
			url = string(plain)
		}
		out = append(out, WebhookSecret{WebhookSettings: w, URL: url})
	}
	return out, nil
}

// CreateWebhook inserts a webhook with an encrypted URL.
func (s *Store) CreateWebhook(id, url, format string, enabled bool) error {
	enc, err := s.box.Seal([]byte(strings.TrimSpace(url)))
	if err != nil {
		return err
	}
	en := 0
	if enabled {
		en = 1
	}
	_, err = s.db.Exec(`INSERT INTO alert_webhooks(id, enabled, url_enc, format) VALUES(?, ?, ?, ?)`, id, en, enc, format)
	return err
}

// UpdateWebhook updates enabled state, format, and optionally the URL.
func (s *Store) UpdateWebhook(id string, enabled bool, format string, url string) error {
	var enc []byte
	if err := s.db.QueryRow(`SELECT url_enc FROM alert_webhooks WHERE id = ?`, id).Scan(&enc); err != nil {
		return err
	}
	if url != "" {
		sealed, err := s.box.Seal([]byte(strings.TrimSpace(url)))
		if err != nil {
			return err
		}
		enc = sealed
	}
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.db.Exec(`UPDATE alert_webhooks SET enabled=?, url_enc=?, format=? WHERE id = ?`, en, enc, format, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteWebhook removes a webhook.
func (s *Store) DeleteWebhook(id string) error {
	res, err := s.db.Exec(`DELETE FROM alert_webhooks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// WebhookURLForTest returns the decrypted URL for one webhook.
func (s *Store) WebhookURLForTest(id string) (string, error) {
	var enc []byte
	if err := s.db.QueryRow(`SELECT url_enc FROM alert_webhooks WHERE id = ?`, id).Scan(&enc); err != nil {
		return "", err
	}
	if len(enc) == 0 {
		return "", errors.New("webhook url is not set")
	}
	plain, err := s.box.Open(enc)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// NewWebhookID returns a random webhook id.
func (s *Store) NewWebhookID() (string, error) {
	return randomToken()
}
