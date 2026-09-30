package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	serverBinPath  = "/usr/local/bin/nut-allergy-server"
	serverUnitPath = "/etc/systemd/system/nut-allergy-server.service"
	serverUnitName = "nut-allergy-server"
)

func serverUnit() string {
	return `[Unit]
Description=NUT Allergy server
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/nut-allergy-server run
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
`
}

// installServer copies this binary into place and starts or restarts its service.
func installServer() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("run as root: sudo %s", os.Args[0])
	}
	src, err := os.Executable()
	if err != nil {
		return err
	}
	src, err = filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}
	return installFrom(src, serverBinPath, serverUnitPath, exec.Command)
}

func installFrom(src, binPath, unitPath string, command func(name string, arg ...string) *exec.Cmd) error {
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		return err
	}
	same, err := sameFile(src, binPath)
	if err != nil {
		return err
	}
	if !same {
		if err := copyFile(src, binPath, 0o755); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, []byte(serverUnit()), 0o644); err != nil {
		return err
	}
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("systemctl not found: %w", err)
	}
	for _, args := range [][]string{
		{"daemon-reload"},
		{"enable", serverUnitName},
		{"restart", serverUnitName},
	} {
		cmd := command(systemctl, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("systemctl %s: %w", args[0], err)
		}
	}
	fmt.Printf("installed %s and restarted %s\n", binPath, serverUnitName)
	return nil
}

func sameFile(a, b string) (bool, error) {
	ia, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	ib, err := os.Stat(b)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return os.SameFile(ia, ib), nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".nut-allergy-server-*")
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
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	ok = true
	return nil
}
