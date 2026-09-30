package policy

import (
	"testing"
	"time"
)

func ts(t time.Time) *time.Time { return &t }

func TestDecide(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	timeout := 30 * time.Minute
	leftEarly := now.Add(-40 * time.Minute)
	leftLate := now.Add(-10 * time.Minute)

	tests := []struct {
		name         string
		supplies     []Supply
		wantShutdown bool
		wantOnBatt   bool
		wantReason   string
		wantDeadline *time.Time
	}{
		{
			name:       "none bound",
			wantReason: "no supplies bound",
		},
		{
			name: "one still on utility",
			supplies: []Supply{
				{ID: "a", State: OnBattery, OnBatterySince: ts(leftLate)},
				{ID: "b", State: Online},
			},
			wantReason: "a supply is still on utility or not confirmed on battery",
		},
		{
			name: "unreachable does not count as on battery",
			supplies: []Supply{
				{ID: "a", State: OnBattery, OnBatterySince: ts(leftEarly)},
				{ID: "b", State: Unreachable},
			},
			wantReason: "a supply is still on utility or not confirmed on battery",
		},
		{
			name: "unknown does not start the timer",
			supplies: []Supply{
				{ID: "a", State: Unknown},
			},
			wantReason: "a supply is still on utility or not confirmed on battery",
		},
		{
			name: "single ups waiting",
			supplies: []Supply{
				{ID: "a", State: OnBattery, OnBatterySince: ts(leftLate)},
			},
			wantOnBatt:   true,
			wantReason:   "waiting for on-battery timeout",
			wantDeadline: ts(leftLate.Add(timeout)),
		},
		{
			name: "clock starts when the last ups leaves utility",
			supplies: []Supply{
				{ID: "a", State: OnBattery, OnBatterySince: ts(leftEarly)},
				{ID: "b", State: OnBattery, OnBatterySince: ts(leftLate)},
			},
			wantOnBatt:   true,
			wantReason:   "waiting for on-battery timeout",
			wantDeadline: ts(leftLate.Add(timeout)),
		},
		{
			name: "timeout reached",
			supplies: []Supply{
				{ID: "a", State: OnBattery, OnBatterySince: ts(leftEarly)},
				{ID: "b", State: OnBattery, OnBatterySince: ts(now.Add(-30 * time.Minute))},
			},
			wantShutdown: true,
			wantOnBatt:   true,
			wantReason:   "on battery timeout",
			wantDeadline: ts(now),
		},
		{
			name: "low battery shuts down immediately",
			supplies: []Supply{
				{ID: "a", State: OnBattery, OnBatterySince: ts(leftLate)},
				{ID: "b", State: LowBattery, OnBatterySince: ts(leftLate)},
			},
			wantShutdown: true,
			wantOnBatt:   true,
			wantReason:   "low battery",
			wantDeadline: ts(now),
		},
		{
			name: "low battery on one ups while the other is on utility stays up",
			supplies: []Supply{
				{ID: "a", State: LowBattery, OnBatterySince: ts(leftEarly)},
				{ID: "b", State: Online},
			},
			wantReason: "a supply is still on utility or not confirmed on battery",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(tt.supplies, timeout, now)
			if got.Shutdown != tt.wantShutdown || got.OnBattery != tt.wantOnBatt || got.Reason != tt.wantReason {
				t.Fatalf("Decide() = shutdown:%v onBattery:%v reason:%q, want shutdown:%v onBattery:%v reason:%q",
					got.Shutdown, got.OnBattery, got.Reason, tt.wantShutdown, tt.wantOnBatt, tt.wantReason)
			}
			if tt.wantDeadline == nil {
				if got.Deadline != nil {
					t.Fatalf("deadline = %v, want nil", got.Deadline)
				}
				return
			}
			if got.Deadline == nil || !got.Deadline.Equal(*tt.wantDeadline) {
				t.Fatalf("deadline = %v, want %v", got.Deadline, tt.wantDeadline)
			}
		})
	}
}
