package main

import (
	"os"
	"path/filepath"
	"testing"

	"nut-allergy/internal/version"
)

func TestShouldUpgrade(t *testing.T) {
	t.Cleanup(func() { version.Version = "dev" })
	version.Version = "1.0.0"
	if shouldUpgrade("1.0.0", false) {
		t.Fatal("same version should stay")
	}
	if shouldUpgrade("dev", false) || shouldUpgrade("", false) || shouldUpgrade("1.1.0", true) {
		t.Fatal("dev, empty, or shutdown should stay")
	}
	if !shouldUpgrade("1.1.0", false) {
		t.Fatal("newer server should upgrade the agent")
	}
	version.Version = "dev"
	if !shouldUpgrade("1.1.0", false) {
		t.Fatal("dev agent should match a released server")
	}
}

func TestInstallBinaryRejectsText(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "nut-allergy-agent")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installBinary(dest, []byte("not an elf")); err == nil {
		t.Fatal("expected reject")
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("dest changed to %q", got)
	}
	elf := append([]byte{0x7f, 'E', 'L', 'F'}, []byte("rest")...)
	if err := installBinary(dest, elf); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(dest)
	if err != nil || string(got) != string(elf) {
		t.Fatalf("installed %q err %v", got, err)
	}
}
