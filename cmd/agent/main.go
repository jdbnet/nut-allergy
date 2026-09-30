package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		configPath := fs.String("config", configDefault(), "agent config path")
		shutdownCmd := fs.String("shutdown", envOr("/sbin/shutdown -h +0", "NUT_ALLERGY_SHUTDOWN_CMD"), "command run when the policy says to power off")
		_ = fs.Parse(os.Args[2:])
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := runDaemon(ctx, *configPath, *shutdownCmd); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "enroll":
		fs := flag.NewFlagSet("enroll", flag.ExitOnError)
		configPath := fs.String("config", configDefault(), "agent config path")
		serverURL := fs.String("server", "", "server base URL")
		token := fs.String("token", "", "one-time enroll token")
		_ = fs.Parse(os.Args[2:])
		if *serverURL == "" || *token == "" {
			fmt.Fprintln(os.Stderr, "enroll requires --server and --token")
			os.Exit(2)
		}
		if err := enroll(*configPath, *serverURL, *token); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "setup":
		fs := flag.NewFlagSet("setup", flag.ExitOnError)
		configPath := fs.String("config", configDefault(), "agent config path")
		_ = fs.Parse(os.Args[2:])
		if err := runSetup(*configPath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func configDefault() string {
	return envOr("/etc/nut-allergy/agent.json", "NUT_ALLERGY_CONFIG")
}

func envOr(fallback, key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: nut-allergy-agent run|enroll|setup")
}

func jsonRequest(method, url string, body any) (*http.Request, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return http.NewRequest(method, url, bytes.NewReader(b))
}
