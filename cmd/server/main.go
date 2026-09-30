package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"nut-allergy/internal/secret"
	"nut-allergy/internal/server"
	"nut-allergy/internal/store"
)

func main() {
	if len(os.Args) == 1 || os.Args[1] == "install" {
		if err := installServer(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if os.Args[1] == "run" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	dataDir := flag.String("data", "/var/lib/nut-allergy", "directory for the database, key, and certificates")
	httpsAddr := flag.String("https-addr", ":443", "HTTPS listen address")
	setupAddr := flag.String("setup-addr", ":8080", "temporary plain HTTP address for the first-run wizard")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		log.Fatal(err)
	}
	box, err := secret.Open(filepath.Join(*dataDir, "secret.key"))
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(filepath.Join(*dataDir, "server.db"), box)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.New(st, *dataDir, *httpsAddr, *setupAddr).Run(ctx); err != nil {
		log.Fatal(err)
	}
}
