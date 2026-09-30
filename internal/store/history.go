package store

import (
	"database/sql"
	"errors"
	"time"

	"nut-allergy/internal/snmp"
)

const (
	metricsRetention   = 30 * 24 * time.Hour
	metricsMinInterval = 60 * time.Second
)

// UPSMetricSample is one stored poll snapshot for charts.
type UPSMetricSample struct {
	RecordedAt       time.Time `json:"recorded_at"`
	State            string    `json:"state"`
	LoadPercent      *int      `json:"load_percent"`
	ChargePercent    *int      `json:"charge_percent"`
	InputVoltage     *int      `json:"input_voltage"`
	MinutesRemaining *int      `json:"minutes_remaining"`
}

// UPSEvent is a UPS status or alert history row.
type UPSEvent struct {
	ID               int64      `json:"id"`
	EventType        string     `json:"event_type"`
	FromState        string     `json:"from_state"`
	ToState          string     `json:"to_state"`
	Message          string     `json:"message"`
	LoadPercent      *int       `json:"load_percent"`
	ChargePercent    *int       `json:"charge_percent"`
	MinutesRemaining *int       `json:"minutes_remaining"`
	CreatedAt        time.Time  `json:"created_at"`
	Notified         bool       `json:"notified"`
}

func historyDDL() string {
	return `
CREATE TABLE IF NOT EXISTS ups_metrics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ups_id TEXT NOT NULL,
  recorded_at TEXT NOT NULL,
  state TEXT NOT NULL,
  load_percent INTEGER,
  charge_percent INTEGER,
  input_voltage INTEGER,
  minutes_remaining INTEGER
);
CREATE INDEX IF NOT EXISTS ups_metrics_ups_time ON ups_metrics(ups_id, recorded_at);
CREATE TABLE IF NOT EXISTS ups_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ups_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  from_state TEXT NOT NULL DEFAULT '',
  to_state TEXT NOT NULL,
  message TEXT NOT NULL DEFAULT '',
  load_percent INTEGER,
  charge_percent INTEGER,
  minutes_remaining INTEGER,
  created_at TEXT NOT NULL,
  notified INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS ups_events_ups_time ON ups_events(ups_id, created_at);
`
}

// RecordPollHistory stores throttled metric samples and state-change events after a poll.
func (s *Store) RecordPollHistory(upsID string, prevState string, r snmp.Reading, now time.Time) error {
	if prevState != r.State {
		if err := s.insertUPSEvent(upsID, "state_change", prevState, r.State, stateChangeMessage(prevState, r.State), r, now, false); err != nil {
			return err
		}
	}
	if shouldSampleMetric(r) {
		if err := s.insertMetricSample(upsID, r, now); err != nil {
			return err
		}
	}
	return s.pruneOldMetrics(now)
}

func shouldSampleMetric(r snmp.Reading) bool {
	if r.State == snmp.StateUnreachable {
		return false
	}
	return true
}

func (s *Store) insertMetricSample(upsID string, r snmp.Reading, now time.Time) error {
	var last string
	err := s.db.QueryRow(`SELECT recorded_at FROM ups_metrics WHERE ups_id = ? ORDER BY recorded_at DESC LIMIT 1`, upsID).Scan(&last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		t, err := time.Parse(time.RFC3339, last)
		if err == nil && now.Sub(t) < metricsMinInterval {
			return nil
		}
	}
	_, err = s.db.Exec(`INSERT INTO ups_metrics(ups_id, recorded_at, state, load_percent, charge_percent, input_voltage, minutes_remaining)
		VALUES(?, ?, ?, ?, ?, ?, ?)`,
		upsID, now.UTC().Format(time.RFC3339), r.State,
		nullPtr(r.LoadPercent), nullPtr(r.ChargePercent), nullPtr(r.InputVoltage), nullPtr(r.MinutesRemaining))
	return err
}

func (s *Store) insertUPSEvent(upsID, eventType, fromState, toState, message string, r snmp.Reading, now time.Time, notified bool) error {
	n := 0
	if notified {
		n = 1
	}
	_, err := s.db.Exec(`INSERT INTO ups_events(ups_id, event_type, from_state, to_state, message, load_percent, charge_percent, minutes_remaining, created_at, notified)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		upsID, eventType, fromState, toState, message,
		nullPtr(r.LoadPercent), nullPtr(r.ChargePercent), nullPtr(r.MinutesRemaining),
		now.UTC().Format(time.RFC3339), n)
	return err
}

func stateChangeMessage(from, to string) string {
	labels := map[string]string{
		snmp.StateOnline:      "On mains",
		snmp.StateOnBattery:   "On battery",
		snmp.StateLowBattery:  "Low battery",
		snmp.StateUnreachable: "Unreachable",
		snmp.StateUnknown:     "Unknown",
	}
	f := labels[from]
	if f == "" {
		f = from
	}
	t := labels[to]
	if t == "" {
		t = to
	}
	if from == "" || from == snmp.StateUnknown {
		return t
	}
	return f + " → " + t
}

func (s *Store) pruneOldMetrics(now time.Time) error {
	cutoff := now.Add(-metricsRetention).UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`DELETE FROM ups_metrics WHERE recorded_at < ?`, cutoff)
	return err
}

// ListUPSMetrics returns samples for one UPS since the given time.
func (s *Store) ListUPSMetrics(upsID string, since time.Time) ([]UPSMetricSample, error) {
	rows, err := s.db.Query(`SELECT recorded_at, state, load_percent, charge_percent, input_voltage, minutes_remaining
		FROM ups_metrics WHERE ups_id = ? AND recorded_at >= ? ORDER BY recorded_at`,
		upsID, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UPSMetricSample
	for rows.Next() {
		var sample UPSMetricSample
		var recorded string
		var load, charge, volts, mins sql.NullInt64
		if err := rows.Scan(&recorded, &sample.State, &load, &charge, &volts, &mins); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, recorded)
		if err != nil {
			continue
		}
		sample.RecordedAt = t
		sample.LoadPercent = nullInt(load)
		sample.ChargePercent = nullInt(charge)
		sample.InputVoltage = nullInt(volts)
		sample.MinutesRemaining = nullInt(mins)
		out = append(out, sample)
	}
	if out == nil {
		out = []UPSMetricSample{}
	}
	return out, rows.Err()
}

// ListUPSEvents returns recent events for one UPS since the given time.
func (s *Store) ListUPSEvents(upsID string, since time.Time, limit int) ([]UPSEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.Query(`SELECT id, event_type, from_state, to_state, message, load_percent, charge_percent, minutes_remaining, created_at, notified
		FROM ups_events WHERE ups_id = ? AND created_at >= ? ORDER BY created_at DESC LIMIT ?`,
		upsID, since.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UPSEvent
	for rows.Next() {
		var ev UPSEvent
		var created string
		var load, charge, mins sql.NullInt64
		var notified int
		if err := rows.Scan(&ev.ID, &ev.EventType, &ev.FromState, &ev.ToState, &ev.Message, &load, &charge, &mins, &created, &notified); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, created)
		if err != nil {
			continue
		}
		ev.CreatedAt = t
		ev.LoadPercent = nullInt(load)
		ev.ChargePercent = nullInt(charge)
		ev.MinutesRemaining = nullInt(mins)
		ev.Notified = notified != 0
		out = append(out, ev)
	}
	if out == nil {
		out = []UPSEvent{}
	}
	return out, rows.Err()
}

// InsertUPSEvent records an arbitrary event (e.g. alert delivery) for alert integrations.
func (s *Store) InsertUPSEvent(upsID, eventType, fromState, toState, message string, r snmp.Reading, now time.Time, notified bool) error {
	return s.insertUPSEvent(upsID, eventType, fromState, toState, message, r, now, notified)
}
