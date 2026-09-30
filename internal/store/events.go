package store

import (
	"database/sql"
	"time"

	"nut-allergy/internal/snmp"
)

func eventsDDL() string {
	return `
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

// UPSEvent is a UPS history row for the dashboard and alert log.
type UPSEvent struct {
	ID               int64     `json:"id"`
	EventType        string    `json:"event_type"`
	FromState        string    `json:"from_state"`
	ToState          string    `json:"to_state"`
	Message          string    `json:"message"`
	LoadPercent      *int      `json:"load_percent"`
	ChargePercent    *int      `json:"charge_percent"`
	MinutesRemaining *int      `json:"minutes_remaining"`
	CreatedAt        time.Time `json:"created_at"`
	Notified         bool      `json:"notified"`
}

// InsertUPSEvent appends a history row.
func (s *Store) InsertUPSEvent(upsID, eventType, fromState, toState, message string, r snmp.Reading, now time.Time, notified bool) error {
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
