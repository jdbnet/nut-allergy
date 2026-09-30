package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"nut-allergy/internal/version"
)

// shouldUpgrade reports whether this agent should replace itself with the server's binary.
// A development server never pushes an upgrade. Any other mismatch, including a downgrade, matches the server.
func shouldUpgrade(serverVersion string, shuttingDown bool) bool {
	if shuttingDown || serverVersion == "" || serverVersion == "dev" || serverVersion == version.Version {
		return false
	}
	return true
}

func maybeUpgrade(client *http.Client, server, serverVersion string, shuttingDown bool, notBefore time.Time, now time.Time) time.Time {
	if now.Before(notBefore) || !shouldUpgrade(serverVersion, shuttingDown) {
		return notBefore
	}
	if err := upgrade(client, server); err != nil {
		log.Printf("upgrade: %v", err)
		return now.Add(15 * time.Minute)
	}
	return notBefore
}

func upgrade(client *http.Client, server string) error {
	dl := *client
	dl.Timeout = 2 * time.Minute
	res, err := dl.Get(strings.TrimRight(server, "/") + "/agent/bin")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("download agent: %s", res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 80<<20))
	if err != nil {
		return err
	}
	dest, err := os.Executable()
	if err != nil {
		return err
	}
	if dest, err = filepath.EvalSymlinks(dest); err != nil {
		return err
	}
	if err := installBinary(dest, body); err != nil {
		return err
	}
	return syscall.Exec(dest, os.Args, os.Environ())
}

func installBinary(dest string, body []byte) error {
	if len(body) < 4 || body[0] != 0x7f || body[1] != 'E' || body[2] != 'L' || body[3] != 'F' {
		return fmt.Errorf("download is not a linux executable")
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".nut-allergy-agent-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	ok = true
	return nil
}
