.PHONY: web agent server test dev

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
REPO ?= jdbnet/nut-allergy
LDFLAGS := -s -w -X nut-allergy/internal/version.Version=$(VERSION) -X nut-allergy/internal/version.Commit=$(COMMIT) -X nut-allergy/internal/version.Repo=$(REPO)

web:
	cd web && npm install && npm run build
	rm -rf internal/assets/dist
	mkdir -p internal/assets/dist
	cp -a web/dist/. internal/assets/dist/

agent:
	rm -f internal/assets/nut-allergy-agent
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o internal/assets/nut-allergy-agent ./cmd/agent

server: web agent
	go build -ldflags "$(LDFLAGS)" -o bin/nut-allergy-server ./cmd/server

test:
	go test ./...

dev: server
	./bin/nut-allergy-server -data /tmp/nut-allergy-dev -setup-addr :18080 -https-addr :18443
