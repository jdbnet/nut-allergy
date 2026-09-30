package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"

	"nut-allergy/internal/version"
)

func runDaemon(ctx context.Context, configPath, shutdownCmd string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	client, err := httpClient(cfg)
	if err != nil {
		return err
	}
	log.Printf("agent %s polling %s", version.Version, cfg.Server)
	var nextUpgrade time.Time
	for {
		var resp pollResponse
		err := postJSON(client, cfg.Server+"/api/agent/poll", map[string]string{"hostname": cfg.Hostname}, &resp)
		now := time.Now()
		if err != nil {
			log.Printf("poll: %v", err)
			if offlineShutdown(cfg.OnBattery, cfg.Deadline, now) {
				if shutErr := shutdown(shutdownCmd); shutErr != nil {
					log.Printf("shutdown: %v", shutErr)
				}
			}
		} else {
			cfg.UPSIDs = resp.UPSIDs
			cfg.TimeoutOverride = resp.TimeoutOverride
			cfg.OnBattery = resp.OnBattery
			cfg.Deadline = resp.Deadline
			if err := saveConfig(configPath, cfg); err != nil {
				log.Printf("save config: %v", err)
			}
			if resp.Shutdown {
				log.Printf("shutdown: %s", resp.Reason)
				if shutErr := shutdown(shutdownCmd); shutErr != nil {
					log.Printf("shutdown: %v", shutErr)
				}
			} else {
				nextUpgrade = maybeUpgrade(client, cfg.Server, resp.ServerVersion, false, nextUpgrade, now)
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(5 * time.Second):
		}
	}
}

func shutdown(command string) error {
	cmd := exec.Command("/bin/sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}
