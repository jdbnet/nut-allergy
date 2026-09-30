// Package policy decides when a host should shut down from the UPSs it is
// plugged into. A host stays up while any bound UPS is still on utility.
package policy

import "time"

const (
	Online      = "online"
	OnBattery   = "on_battery"
	LowBattery  = "low_battery"
	Unreachable = "unreachable"
	Unknown     = "unknown"
)

// Supply is one UPS as last reported by the poller.
type Supply struct {
	ID             string
	State          string
	OnBatterySince *time.Time
}

// Decision is the shutdown outcome for one agent at time Now.
type Decision struct {
	OnBattery bool
	Shutdown  bool
	Reason    string
	Deadline  *time.Time
}

// Decide shuts down only when every supply is on battery. The deadline is the
// moment the last supply left utility plus timeout. Low battery on that same
// condition shuts down immediately. Unreachable is not on-battery.
func Decide(supplies []Supply, timeout time.Duration, now time.Time) Decision {
	if len(supplies) == 0 {
		return Decision{Reason: "no supplies bound"}
	}
	if timeout < 0 {
		timeout = 0
	}

	allOnBattery := true
	anyLow := false
	var lastLeft time.Time
	for _, s := range supplies {
		switch s.State {
		case LowBattery:
			anyLow = true
			if s.OnBatterySince != nil && s.OnBatterySince.After(lastLeft) {
				lastLeft = *s.OnBatterySince
			}
		case OnBattery:
			if s.OnBatterySince != nil && s.OnBatterySince.After(lastLeft) {
				lastLeft = *s.OnBatterySince
			}
		default:
			allOnBattery = false
		}
	}
	if !allOnBattery {
		return Decision{Reason: "a supply is still on utility or not confirmed on battery"}
	}

	if lastLeft.IsZero() {
		lastLeft = now
	}
	deadline := lastLeft.Add(timeout)
	if anyLow {
		d := now
		return Decision{
			OnBattery: true,
			Shutdown:  true,
			Reason:    "low battery",
			Deadline:  &d,
		}
	}
	if !now.Before(deadline) {
		return Decision{
			OnBattery: true,
			Shutdown:  true,
			Reason:    "on battery timeout",
			Deadline:  &deadline,
		}
	}
	return Decision{
		OnBattery: true,
		Reason:    "waiting for on-battery timeout",
		Deadline:  &deadline,
	}
}
