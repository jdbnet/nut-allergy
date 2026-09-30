package main

import "time"

// offlineShutdown is the local failsafe. It fires only when the server had
// already reported every bound UPS on battery and the saved deadline has passed.
func offlineShutdown(onBattery bool, deadline *time.Time, now time.Time) bool {
	return onBattery && deadline != nil && !now.Before(*deadline)
}
