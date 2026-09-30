// Package store persists server state in SQLite. SNMP and DNS secrets are
// encrypted with the process key before they are written.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"nut-allergy/internal/secret"
	"nut-allergy/internal/snmp"
)

const (
	sessionTTL  = 7 * 24 * time.Hour
	enrollTTL   = time.Hour
	continueTTL = 15 * time.Minute
)

// Store is the server database.
type Store struct {
	db  *sql.DB
	box *secret.Box
}

// Open creates the schema and returns a store. dbPath is the sqlite file.
func Open(dbPath string, box *secret.Box) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	s := &Store{db: db, box: box}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS meta (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS ups (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  host TEXT NOT NULL,
  sec_level TEXT NOT NULL,
  username TEXT NOT NULL,
  auth_protocol TEXT NOT NULL,
  auth_password BLOB NOT NULL,
  priv_protocol TEXT NOT NULL DEFAULT '',
  priv_password BLOB NOT NULL,
  state TEXT NOT NULL DEFAULT 'unknown',
  minutes_remaining INTEGER,
  seconds_on_battery INTEGER,
  input_voltage INTEGER,
  load_percent INTEGER,
  charge_percent INTEGER,
  on_battery_since TEXT,
  last_error TEXT NOT NULL DEFAULT '',
  updated_at TEXT
);
CREATE TABLE IF NOT EXISTS agents (
  id TEXT PRIMARY KEY,
  hostname TEXT NOT NULL,
  cert_fingerprint TEXT NOT NULL UNIQUE,
  timeout_override INTEGER,
  last_seen TEXT,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS agent_ups (
  agent_id TEXT NOT NULL,
  ups_id TEXT NOT NULL,
  PRIMARY KEY (agent_id, ups_id)
);
CREATE TABLE IF NOT EXISTS enroll_tokens (
  token TEXT PRIMARY KEY,
  expires_at TEXT NOT NULL,
  used_at TEXT
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS continue_tokens (
  token TEXT PRIMARY KEY,
  expires_at TEXT NOT NULL,
  used_at TEXT
);
` + eventsDDL() + alertsDDL())
	return err
}

func (s *Store) meta(k string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT v FROM meta WHERE k = ?`, k).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

func (s *Store) setMeta(k, v string) error {
	_, err := s.db.Exec(`INSERT INTO meta(k, v) VALUES(?, ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v`, k, v)
	return err
}

// Setup is the first-run progress the wizard reads.
type Setup struct {
	Complete       bool   `json:"complete"`
	HasPassword    bool   `json:"has_password"`
	Hostname       string `json:"hostname"`
	HasCert        bool   `json:"has_cert"`
	TLSMode        string `json:"tls_mode"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// GetSetup returns wizard progress.
func (s *Store) GetSetup() (Setup, error) {
	var out Setup
	if v, ok, err := s.meta("setup_complete"); err != nil {
		return out, err
	} else {
		out.Complete = ok && v == "1"
	}
	if _, ok, err := s.meta("admin_hash"); err != nil {
		return out, err
	} else {
		out.HasPassword = ok
	}
	h, _, err := s.meta("hostname")
	if err != nil {
		return out, err
	}
	out.Hostname = h
	if v, ok, err := s.meta("cert_pem"); err != nil {
		return out, err
	} else {
		out.HasCert = ok && v != ""
	}
	mode, _, err := s.meta("tls_mode")
	if err != nil {
		return out, err
	}
	out.TLSMode = mode
	if v, ok, err := s.meta("timeout_seconds"); err != nil {
		return out, err
	} else if ok {
		fmt.Sscanf(v, "%d", &out.TimeoutSeconds)
	}
	return out, nil
}

// SetPassword stores a bcrypt hash. It fails if a password already exists.
func (s *Store) SetPassword(plain string) error {
	if _, ok, err := s.meta("admin_hash"); err != nil {
		return err
	} else if ok {
		return errors.New("password already set")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.setMeta("admin_hash", string(hash))
}

// CheckPassword reports whether plain matches the admin password.
func (s *Store) CheckPassword(plain string) error {
	hash, ok, err := s.meta("admin_hash")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("password is not set")
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}

// ChangePassword replaces the admin password after the current one matches.
func (s *Store) ChangePassword(current, next string) error {
	if err := s.CheckPassword(current); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.setMeta("admin_hash", string(hash))
}

// CreateSession returns a new session id.
func (s *Store) CreateSession(now time.Time) (string, error) {
	id, err := randomToken()
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(`INSERT INTO sessions(id, expires_at) VALUES(?, ?)`, id, now.Add(sessionTTL).UTC().Format(time.RFC3339))
	return id, err
}

// SessionValid reports whether id is an unexpired session.
func (s *Store) SessionValid(id string, now time.Time) (bool, error) {
	var exp string
	err := s.db.QueryRow(`SELECT expires_at FROM sessions WHERE id = ?`, id).Scan(&exp)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	t, err := time.Parse(time.RFC3339, exp)
	if err != nil {
		return false, err
	}
	return now.Before(t), nil
}

// DeleteSession removes a session.
func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// CreateContinueToken is a one-time bridge from the HTTP wizard to HTTPS.
func (s *Store) CreateContinueToken(now time.Time) (string, error) {
	id, err := randomToken()
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(`INSERT INTO continue_tokens(token, expires_at) VALUES(?, ?)`, id, now.Add(continueTTL).UTC().Format(time.RFC3339))
	return id, err
}

// ConsumeContinueToken marks a bridge token used.
func (s *Store) ConsumeContinueToken(token string, now time.Time) error {
	var exp string
	var used sql.NullString
	err := s.db.QueryRow(`SELECT expires_at, used_at FROM continue_tokens WHERE token = ?`, token).Scan(&exp, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("unknown continuation")
	}
	if err != nil {
		return err
	}
	if used.Valid {
		return errors.New("continuation already used")
	}
	t, err := time.Parse(time.RFC3339, exp)
	if err != nil {
		return err
	}
	if !now.Before(t) {
		return errors.New("continuation expired")
	}
	_, err = s.db.Exec(`UPDATE continue_tokens SET used_at = ? WHERE token = ?`, now.UTC().Format(time.RFC3339), token)
	return err
}

// SetHostname stores the DNS name used for certificates and install scripts.
func (s *Store) SetHostname(host string) error { return s.setMeta("hostname", host) }

// SetTimeout stores the server-wide on-battery wait in seconds.
func (s *Store) SetTimeout(seconds int) error {
	return s.setMeta("timeout_seconds", fmt.Sprintf("%d", seconds))
}

// TimeoutSeconds returns the server-wide wait. Zero means unset.
func (s *Store) TimeoutSeconds() (int, error) {
	v, ok, err := s.meta("timeout_seconds")
	if err != nil || !ok {
		return 0, err
	}
	var n int
	_, err = fmt.Sscanf(v, "%d", &n)
	return n, err
}

// PutCert stores the server certificate and key PEM and the issuance mode.
func (s *Store) PutCert(mode, certPEM, keyPEM string) error {
	if err := s.setMeta("tls_mode", mode); err != nil {
		return err
	}
	if err := s.setMeta("cert_pem", certPEM); err != nil {
		return err
	}
	return s.setMeta("key_pem", keyPEM)
}

// Cert returns the server certificate and key PEM.
func (s *Store) Cert() (certPEM, keyPEM, mode string, err error) {
	var ok bool
	certPEM, ok, err = s.meta("cert_pem")
	if err != nil || !ok {
		if err == nil {
			err = errors.New("certificate is not configured")
		}
		return
	}
	keyPEM, _, err = s.meta("key_pem")
	if err != nil {
		return
	}
	mode, _, err = s.meta("tls_mode")
	return
}

// SetDNS stores Let's Encrypt DNS-01 settings. The token is encrypted.
func (s *Store) SetDNS(provider, token, email string) error {
	blob, err := s.box.Seal([]byte(token))
	if err != nil {
		return err
	}
	if err := s.setMeta("dns_provider", provider); err != nil {
		return err
	}
	if err := s.setMeta("dns_token", hex.EncodeToString(blob)); err != nil {
		return err
	}
	return s.setMeta("acme_email", email)
}

// DNS returns the provider, decrypted token, and account email.
func (s *Store) DNS() (provider, token, email string, err error) {
	var ok bool
	provider, ok, err = s.meta("dns_provider")
	if err != nil || !ok {
		if err == nil {
			err = errors.New("dns provider is not configured")
		}
		return
	}
	enc, _, err := s.meta("dns_token")
	if err != nil {
		return
	}
	raw, err := hex.DecodeString(enc)
	if err != nil {
		return
	}
	plain, err := s.box.Open(raw)
	if err != nil {
		return
	}
	token = string(plain)
	email, _, err = s.meta("acme_email")
	return
}

// SetACME stores the Lego account key and registration JSON.
func (s *Store) SetACME(accountKeyPEM, regJSON string) error {
	blob, err := s.box.Seal([]byte(accountKeyPEM))
	if err != nil {
		return err
	}
	if err := s.setMeta("acme_account_key", hex.EncodeToString(blob)); err != nil {
		return err
	}
	return s.setMeta("acme_account_reg", regJSON)
}

// SetACMEResource stores the last Lego certificate resource JSON.
func (s *Store) SetACMEResource(resourceJSON string) error {
	return s.setMeta("acme_cert_resource", resourceJSON)
}

// ACMEResource returns the last Lego certificate resource JSON.
func (s *Store) ACMEResource() (string, error) {
	v, ok, err := s.meta("acme_cert_resource")
	if err != nil || !ok {
		if err == nil {
			err = errors.New("acme certificate resource is not configured")
		}
		return "", err
	}
	return v, nil
}

// ACME returns the account key PEM and registration JSON.
func (s *Store) ACME() (accountKeyPEM, regJSON string, err error) {
	enc, ok, err := s.meta("acme_account_key")
	if err != nil || !ok {
		if err == nil {
			err = errors.New("acme account is not configured")
		}
		return
	}
	raw, err := hex.DecodeString(enc)
	if err != nil {
		return
	}
	plain, err := s.box.Open(raw)
	if err != nil {
		return
	}
	accountKeyPEM = string(plain)
	regJSON, _, err = s.meta("acme_account_reg")
	return
}

// SetCA stores the internal agent CA.
func (s *Store) SetCA(certPEM, keyPEM string) error {
	blob, err := s.box.Seal([]byte(keyPEM))
	if err != nil {
		return err
	}
	if err := s.setMeta("ca_cert", certPEM); err != nil {
		return err
	}
	return s.setMeta("ca_key", hex.EncodeToString(blob))
}

// CA returns the internal CA certificate and decrypted key PEM.
func (s *Store) CA() (certPEM, keyPEM string, err error) {
	var ok bool
	certPEM, ok, err = s.meta("ca_cert")
	if err != nil || !ok {
		if err == nil {
			err = errors.New("ca is not configured")
		}
		return
	}
	enc, _, err := s.meta("ca_key")
	if err != nil {
		return
	}
	raw, err := hex.DecodeString(enc)
	if err != nil {
		return
	}
	plain, err := s.box.Open(raw)
	if err != nil {
		return
	}
	keyPEM = string(plain)
	return
}

// MarkComplete finishes the wizard.
func (s *Store) MarkComplete() error { return s.setMeta("setup_complete", "1") }

// UPS is a device without its SNMP secrets.
type UPS struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	Host             string     `json:"host"`
	SecLevel         string     `json:"sec_level"`
	Username         string     `json:"username"`
	AuthProtocol     string     `json:"auth_protocol"`
	PrivProtocol     string     `json:"priv_protocol"`
	State            string     `json:"state"`
	MinutesRemaining *int       `json:"minutes_remaining"`
	SecondsOnBattery *int       `json:"seconds_on_battery"`
	InputVoltage     *int       `json:"input_voltage"`
	LoadPercent      *int       `json:"load_percent"`
	ChargePercent    *int       `json:"charge_percent"`
	OnBatterySince   *time.Time `json:"on_battery_since"`
	LastError        string     `json:"last_error"`
	UpdatedAt        *time.Time `json:"updated_at"`
}

// UPSInput is a create or update payload. Empty passwords keep the stored secret on update.
type UPSInput struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Host         string `json:"host"`
	SecLevel     string `json:"sec_level"`
	Username     string `json:"username"`
	AuthProtocol string `json:"auth_protocol"`
	AuthPassword string `json:"auth_password"`
	PrivProtocol string `json:"priv_protocol"`
	PrivPassword string `json:"priv_password"`
}

// CreateUPS inserts a UPS.
func (s *Store) CreateUPS(in UPSInput) (UPS, error) {
	auth, err := s.box.Seal([]byte(in.AuthPassword))
	if err != nil {
		return UPS{}, err
	}
	priv, err := s.box.Seal([]byte(in.PrivPassword))
	if err != nil {
		return UPS{}, err
	}
	id, err := randomToken()
	if err != nil {
		return UPS{}, err
	}
	_, err = s.db.Exec(`INSERT INTO ups(id, name, description, host, sec_level, username, auth_protocol, auth_password, priv_protocol, priv_password)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, strings.TrimSpace(in.Name), in.Description, strings.TrimSpace(in.Host), in.SecLevel, in.Username, in.AuthProtocol, auth, in.PrivProtocol, priv)
	if err != nil {
		return UPS{}, err
	}
	return s.GetUPS(id)
}

// UpdateUPS updates a UPS. Blank passwords keep the current secrets.
func (s *Store) UpdateUPS(id string, in UPSInput) (UPS, error) {
	curAuth, curPriv, err := s.secretBlobs(id)
	if err != nil {
		return UPS{}, err
	}
	if in.AuthPassword != "" {
		curAuth, err = s.box.Seal([]byte(in.AuthPassword))
		if err != nil {
			return UPS{}, err
		}
	}
	if in.PrivPassword != "" {
		curPriv, err = s.box.Seal([]byte(in.PrivPassword))
		if err != nil {
			return UPS{}, err
		}
	}
	res, err := s.db.Exec(`UPDATE ups SET name=?, description=?, host=?, sec_level=?, username=?, auth_protocol=?, auth_password=?, priv_protocol=?, priv_password=? WHERE id=?`,
		strings.TrimSpace(in.Name), in.Description, strings.TrimSpace(in.Host), in.SecLevel, in.Username, in.AuthProtocol, curAuth, in.PrivProtocol, curPriv, id)
	if err != nil {
		return UPS{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return UPS{}, sql.ErrNoRows
	}
	return s.GetUPS(id)
}

func (s *Store) secretBlobs(id string) (auth, priv []byte, err error) {
	err = s.db.QueryRow(`SELECT auth_password, priv_password FROM ups WHERE id = ?`, id).Scan(&auth, &priv)
	return
}

// DeleteUPS removes a UPS and any agent bindings to it.
func (s *Store) DeleteUPS(id string) error {
	if _, err := s.db.Exec(`DELETE FROM agent_ups WHERE ups_id = ?`, id); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM ups_events WHERE ups_id = ?`, id); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM ups WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetUPS returns one UPS.
func (s *Store) GetUPS(id string) (UPS, error) {
	row := s.db.QueryRow(`SELECT id, name, description, host, sec_level, username, auth_protocol, priv_protocol, state,
		minutes_remaining, seconds_on_battery, input_voltage, load_percent, charge_percent, on_battery_since, last_error, updated_at
		FROM ups WHERE id = ?`, id)
	return scanUPS(row)
}

// ListUPS returns every UPS.
func (s *Store) ListUPS() ([]UPS, error) {
	rows, err := s.db.Query(`SELECT id, name, description, host, sec_level, username, auth_protocol, priv_protocol, state,
		minutes_remaining, seconds_on_battery, input_voltage, load_percent, charge_percent, on_battery_since, last_error, updated_at
		FROM ups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UPS
	for rows.Next() {
		u, err := scanUPS(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if out == nil {
		out = []UPS{}
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUPS(sc scanner) (UPS, error) {
	var u UPS
	var mins, secs, volts, load, charge sql.NullInt64
	var since, updated sql.NullString
	err := sc.Scan(&u.ID, &u.Name, &u.Description, &u.Host, &u.SecLevel, &u.Username, &u.AuthProtocol, &u.PrivProtocol, &u.State,
		&mins, &secs, &volts, &load, &charge, &since, &u.LastError, &updated)
	if err != nil {
		return u, err
	}
	u.MinutesRemaining = nullInt(mins)
	u.SecondsOnBattery = nullInt(secs)
	u.InputVoltage = nullInt(volts)
	u.LoadPercent = nullInt(load)
	u.ChargePercent = nullInt(charge)
	u.OnBatterySince = nullTime(since)
	u.UpdatedAt = nullTime(updated)
	return u, nil
}

func nullInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func nullTime(n sql.NullString) *time.Time {
	if !n.Valid || n.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, n.String)
	if err != nil {
		return nil
	}
	return &t
}

// Target loads SNMP credentials for a poll.
func (s *Store) Target(id string) (snmp.Target, error) {
	var t snmp.Target
	var auth, priv []byte
	err := s.db.QueryRow(`SELECT host, sec_level, username, auth_protocol, auth_password, priv_protocol, priv_password FROM ups WHERE id = ?`, id).
		Scan(&t.Host, &t.SecurityLevel, &t.User, &t.AuthProtocol, &auth, &t.PrivProtocol, &priv)
	if err != nil {
		return t, err
	}
	ap, err := s.box.Open(auth)
	if err != nil {
		return t, err
	}
	pp, err := s.box.Open(priv)
	if err != nil {
		return t, err
	}
	t.AuthPassword = string(ap)
	t.PrivPassword = string(pp)
	return t, nil
}

// ApplyReading stores a poll and maintains the on-battery clock.
// Unreachable keeps the previous clock so a blip does not look like a return to utility.
func (s *Store) ApplyReading(id string, r snmp.Reading, now time.Time) error {
	var prevState string
	var prevSince sql.NullString
	err := s.db.QueryRow(`SELECT state, on_battery_since FROM ups WHERE id = ?`, id).Scan(&prevState, &prevSince)
	if err != nil {
		return err
	}
	since := onBatterySince(prevState, prevSince.String, r.State, now)
	var sinceVal any
	if since != nil {
		sinceVal = since.UTC().Format(time.RFC3339)
	}
	_, err = s.db.Exec(`UPDATE ups SET state=?, minutes_remaining=?, seconds_on_battery=?, input_voltage=?, load_percent=?, charge_percent=?, on_battery_since=?, last_error=?, updated_at=? WHERE id=?`,
		r.State, nullPtr(r.MinutesRemaining), nullPtr(r.SecondsOnBattery), nullPtr(r.InputVoltage), nullPtr(r.LoadPercent), nullPtr(r.ChargePercent),
		sinceVal, r.Error, now.UTC().Format(time.RFC3339), id)
	return err
}

func nullPtr(n *int) any {
	if n == nil {
		return nil
	}
	return *n
}

func onBatterySince(prevState, prevSince, next string, now time.Time) *time.Time {
	_ = prevState
	battery := next == snmp.StateOnBattery || next == snmp.StateLowBattery
	if battery {
		if t := nullTime(sql.NullString{String: prevSince, Valid: prevSince != ""}); t != nil {
			return t
		}
		return &now
	}
	if next == snmp.StateUnreachable || next == snmp.StateUnknown {
		return nullTime(sql.NullString{String: prevSince, Valid: prevSince != ""})
	}
	return nil
}

// Agent is a connected host without its private key.
type Agent struct {
	ID              string     `json:"id"`
	Hostname        string     `json:"hostname"`
	Fingerprint     string     `json:"-"`
	TimeoutOverride *int       `json:"timeout_override_seconds"`
	LastSeen        *time.Time `json:"last_seen"`
	UPSIDs          []string   `json:"ups_ids"`
}

// CreateEnrollToken returns a single-use enroll token.
func (s *Store) CreateEnrollToken(now time.Time) (string, time.Time, error) {
	token, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	exp := now.Add(enrollTTL)
	_, err = s.db.Exec(`INSERT INTO enroll_tokens(token, expires_at) VALUES(?, ?)`, token, exp.UTC().Format(time.RFC3339))
	return token, exp, err
}

// EnrollTokenUsable reports whether token can still be used to enroll.
func (s *Store) EnrollTokenUsable(token string, now time.Time) error {
	_, err := s.lookupEnrollToken(token, now)
	return err
}

func (s *Store) lookupEnrollToken(token string, now time.Time) (string, error) {
	var exp string
	var used sql.NullString
	err := s.db.QueryRow(`SELECT expires_at, used_at FROM enroll_tokens WHERE token = ?`, token).Scan(&exp, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("unknown enroll token")
	}
	if err != nil {
		return "", err
	}
	if used.Valid {
		return "", errors.New("enroll token already used")
	}
	t, err := time.Parse(time.RFC3339, exp)
	if err != nil {
		return "", err
	}
	if !now.Before(t) {
		return "", errors.New("enroll token expired")
	}
	return exp, nil
}

// ConsumeEnrollToken marks a token used.
func (s *Store) ConsumeEnrollToken(token string, now time.Time) error {
	if _, err := s.lookupEnrollToken(token, now); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE enroll_tokens SET used_at = ? WHERE token = ?`, now.UTC().Format(time.RFC3339), token)
	return err
}

// UpsertAgent creates an agent or replaces the certificate of an existing hostname.
func (s *Store) UpsertAgent(hostname, fingerprint string, now time.Time) (Agent, error) {
	var id string
	err := s.db.QueryRow(`SELECT id FROM agents WHERE hostname = ?`, hostname).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		id, err = randomToken()
		if err != nil {
			return Agent{}, err
		}
		_, err = s.db.Exec(`INSERT INTO agents(id, hostname, cert_fingerprint, created_at) VALUES(?, ?, ?, ?)`,
			id, hostname, fingerprint, now.UTC().Format(time.RFC3339))
		if err != nil {
			return Agent{}, err
		}
	} else if err != nil {
		return Agent{}, err
	} else {
		_, err = s.db.Exec(`UPDATE agents SET cert_fingerprint = ? WHERE id = ?`, fingerprint, id)
		if err != nil {
			return Agent{}, err
		}
	}
	return s.GetAgent(id)
}

// GetAgent returns an agent and its UPS bindings.
func (s *Store) GetAgent(id string) (Agent, error) {
	var a Agent
	var override sql.NullInt64
	var seen sql.NullString
	err := s.db.QueryRow(`SELECT id, hostname, cert_fingerprint, timeout_override, last_seen FROM agents WHERE id = ?`, id).
		Scan(&a.ID, &a.Hostname, &a.Fingerprint, &override, &seen)
	if err != nil {
		return a, err
	}
	if override.Valid {
		v := int(override.Int64)
		a.TimeoutOverride = &v
	}
	a.LastSeen = nullTime(seen)
	a.UPSIDs, err = s.agentUPS(id)
	return a, err
}

// AgentByFingerprint finds the agent for a client certificate.
func (s *Store) AgentByFingerprint(fp string) (Agent, error) {
	var id string
	err := s.db.QueryRow(`SELECT id FROM agents WHERE cert_fingerprint = ?`, fp).Scan(&id)
	if err != nil {
		return Agent{}, err
	}
	return s.GetAgent(id)
}

func (s *Store) agentUPS(id string) ([]string, error) {
	rows, err := s.db.Query(`SELECT ups_id FROM agent_ups WHERE agent_id = ? ORDER BY ups_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		ids = append(ids, u)
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, rows.Err()
}

// ListAgents returns every enrolled agent.
func (s *Store) ListAgents() ([]Agent, error) {
	rows, err := s.db.Query(`SELECT id FROM agents ORDER BY hostname`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	out := make([]Agent, 0, len(ids))
	for _, id := range ids {
		a, err := s.GetAgent(id)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// DeleteAgent removes an enrolled host and its UPS bindings.
func (s *Store) DeleteAgent(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM agent_ups WHERE agent_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM agents WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

// SetAgentConfig replaces bindings and the optional timeout override.
// overrideSet distinguishes "leave it" from "clear it" — callers always pass the new value.
func (s *Store) SetAgentConfig(id string, upsIDs []string, override *int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM agents WHERE id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(`DELETE FROM agent_ups WHERE agent_id = ?`, id); err != nil {
		return err
	}
	for _, upsID := range upsIDs {
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM ups WHERE id = ?`, upsID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return fmt.Errorf("unknown ups %s", upsID)
		}
		if _, err := tx.Exec(`INSERT INTO agent_ups(agent_id, ups_id) VALUES(?, ?)`, id, upsID); err != nil {
			return err
		}
	}
	var ov any
	if override != nil {
		ov = *override
	}
	if _, err := tx.Exec(`UPDATE agents SET timeout_override = ? WHERE id = ?`, ov, id); err != nil {
		return err
	}
	return tx.Commit()
}

// TouchAgent updates hostname and last seen time from a poll.
func (s *Store) TouchAgent(id, hostname string, now time.Time) error {
	_, err := s.db.Exec(`UPDATE agents SET hostname = ?, last_seen = ? WHERE id = ?`, hostname, now.UTC().Format(time.RFC3339), id)
	return err
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
