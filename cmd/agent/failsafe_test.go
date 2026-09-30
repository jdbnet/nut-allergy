package main

import (
	"testing"
	"time"
)

func TestOfflineShutdown(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)
	if offlineShutdown(false, &past, now) {
		t.Fatal("utility must not shut down when the server is unreachable")
	}
	if offlineShutdown(true, nil, now) {
		t.Fatal("missing deadline must not shut down")
	}
	if offlineShutdown(true, &future, now) {
		t.Fatal("future deadline must wait")
	}
	if !offlineShutdown(true, &past, now) {
		t.Fatal("passed deadline on battery must shut down")
	}
}
