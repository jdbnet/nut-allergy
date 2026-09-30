package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCopiesBinaryAndRestartsService(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "downloaded")
	if err := os.WriteFile(src, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(dir, "bin", "nut-allergy-server")
	unitPath := filepath.Join(dir, "systemd", "nut-allergy-server.service")
	var calls []string
	err := installFrom(src, binPath, unitPath, func(name string, args ...string) *exec.Cmd {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return exec.Command("true")
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary" {
		t.Fatalf("installed binary = %q", got)
	}
	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	unit, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unit), "ExecStart=/usr/local/bin/nut-allergy-server run") {
		t.Fatalf("unit:\n%s", unit)
	}
	if len(calls) != 3 || !strings.HasSuffix(calls[2], "restart nut-allergy-server") {
		t.Fatalf("systemctl calls = %#v", calls)
	}

	calls = nil
	if err := installFrom(binPath, binPath, unitPath, func(name string, args ...string) *exec.Cmd {
		calls = append(calls, strings.Join(args, " "))
		return exec.Command("true")
	}); err != nil {
		t.Fatal(err)
	}
	if calls[len(calls)-1] != "restart nut-allergy-server" {
		t.Fatalf("second install calls = %#v", calls)
	}
}
