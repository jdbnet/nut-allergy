package server

import (
	"bytes"
	"net/http"
	"time"

	"nut-allergy/internal/assets"
)

func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	st, err := s.store.GetSetup()
	if err != nil || !st.Complete || st.Hostname == "" {
		writeErr(w, http.StatusConflict, "server setup is not complete")
		return
	}
	if err := s.store.EnrollTokenUsable(token, time.Now()); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	origin := s.publicHTTPS(st.Hostname)
	origin = origin[:len(origin)-1]
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	_, _ = w.Write([]byte(installScript(origin, token)))
}

func (s *Server) handleAgentBin(w http.ResponseWriter, r *http.Request) {
	if len(assets.Agent) < 4 || !bytes.HasPrefix(assets.Agent, []byte{0x7f, 'E', 'L', 'F'}) {
		http.Error(w, "agent binary is not embedded; build with make", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=nut-allergy-agent")
	_, _ = w.Write(assets.Agent)
}

func installScript(origin, token string) string {
	return `#!/bin/sh
set -eu
if [ "$(id -u)" -ne 0 ]; then
  echo "run this script as root" >&2
  exit 1
fi
SERVER='` + origin + `'
TOKEN='` + token + `'
install -d -m 755 /usr/local/bin
install -d -m 700 /etc/nut-allergy
curl -fsSL "$SERVER/agent/bin" -o /usr/local/bin/nut-allergy-agent
chmod 755 /usr/local/bin/nut-allergy-agent
cat > /etc/systemd/system/nut-allergy-agent.service <<'UNIT'
[Unit]
Description=NUT Allergy agent
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/nut-allergy-agent run
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
UNIT
/usr/local/bin/nut-allergy-agent enroll --server "$SERVER" --token "$TOKEN"
systemctl daemon-reload
systemctl enable --now nut-allergy-agent
if [ -t 0 ]; then
  /usr/local/bin/nut-allergy-agent setup
fi
`
}
