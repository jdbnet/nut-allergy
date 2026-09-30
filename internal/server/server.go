// Package server is the NUT Allergy HTTPS server, poller, and setup wizard.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nut-allergy/internal/assets"
	"nut-allergy/internal/pki"
	"nut-allergy/internal/store"
	"nut-allergy/internal/update"
	"nut-allergy/internal/version"
)

const sessionCookie = "nut_allergy_session"

// Server serves the UI, the agent API, and UPS polling.
type Server struct {
	store     *store.Store
	dataDir   string
	httpsAddr string
	setupAddr string

	cert atomic.Pointer[tls.Certificate]
	ca   atomic.Pointer[pki.CA]

	readyOnce sync.Once
	certReady chan struct{}
	updates   *update.Checker
	notify    *notifyEngine
}

// New returns a server. httpsAddr and setupAddr are listen addresses.
func New(st *store.Store, dataDir, httpsAddr, setupAddr string) *Server {
	srv := &Server{
		store:     st,
		dataDir:   dataDir,
		httpsAddr: httpsAddr,
		setupAddr: setupAddr,
		certReady: make(chan struct{}),
		updates:   update.New(version.Repo, version.Version),
	}
	srv.notify = newNotifyEngine(st)
	srv.notify.reloadTracker()
	return srv
}

// Handler is the HTTP handler, used by listeners and tests.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/setup", s.handleSetupState)
	mux.HandleFunc("POST /api/setup/password", s.handleSetupPassword)
	mux.HandleFunc("POST /api/setup/hostname", s.requireAuth(s.handleSetupHostname))
	mux.HandleFunc("POST /api/setup/certificate", s.requireAuth(s.handleSetupCertificate))
	mux.HandleFunc("POST /api/setup/timeout", s.requireAuth(s.handleSetupTimeout))
	mux.HandleFunc("POST /api/setup/ups", s.requireAuth(s.handleSetupUPS))
	mux.HandleFunc("POST /api/setup/finish", s.requireAuth(s.handleSetupFinish))
	mux.HandleFunc("POST /api/setup/continue", s.handleSetupContinue)
	mux.HandleFunc("GET /api/dns-providers", s.handleDNSProviders)
	mux.HandleFunc("GET /api/snmp-protocols", s.handleSNMPProtocols)

	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("GET /api/version", s.handleVersion)

	mux.HandleFunc("GET /api/fleet", s.requireAuth(s.handleFleet))
	mux.HandleFunc("GET /api/ups", s.requireAuth(s.handleListUPS))
	mux.HandleFunc("POST /api/ups", s.requireAuth(s.handleCreateUPS))
	mux.HandleFunc("GET /api/ups/{id}", s.requireAuth(s.handleGetUPS))
	mux.HandleFunc("PUT /api/ups/{id}", s.requireAuth(s.handleUpdateUPS))
	mux.HandleFunc("DELETE /api/ups/{id}", s.requireAuth(s.handleDeleteUPS))

	mux.HandleFunc("GET /api/agents", s.requireAuth(s.handleListAgents))
	mux.HandleFunc("GET /api/agents/{id}", s.requireAuth(s.handleGetAgent))
	mux.HandleFunc("PATCH /api/agents/{id}", s.requireAuth(s.handlePatchAgent))
	mux.HandleFunc("DELETE /api/agents/{id}", s.requireAuth(s.handleDeleteAgent))
	mux.HandleFunc("POST /api/enroll-tokens", s.requireAuth(s.handleEnrollToken))

	mux.HandleFunc("GET /api/settings", s.requireAuth(s.handleGetSettings))
	mux.HandleFunc("PATCH /api/settings", s.requireAuth(s.handlePatchSettings))
	mux.HandleFunc("POST /api/settings/certificate", s.requireAuth(s.handleSettingsCertificate))
	mux.HandleFunc("GET /api/settings/alerts", s.requireAuth(s.handleGetAlerts))
	mux.HandleFunc("PUT /api/settings/alerts", s.requireAuth(s.handlePutAlerts))
	mux.HandleFunc("POST /api/settings/alerts/test-email", s.requireAuth(s.handleTestAlertEmail))
	mux.HandleFunc("GET /api/settings/webhooks", s.requireAuth(s.handleListWebhooks))
	mux.HandleFunc("POST /api/settings/webhooks", s.requireAuth(s.handleCreateWebhook))
	mux.HandleFunc("PATCH /api/settings/webhooks/{id}", s.requireAuth(s.handlePatchWebhook))
	mux.HandleFunc("DELETE /api/settings/webhooks/{id}", s.requireAuth(s.handleDeleteWebhook))
	mux.HandleFunc("POST /api/settings/webhooks/{id}/test", s.requireAuth(s.handleTestWebhook))

	mux.HandleFunc("POST /api/agent/enroll", s.handleEnroll)
	mux.HandleFunc("POST /api/agent/poll", s.requireAgent(s.handlePoll))
	mux.HandleFunc("GET /api/agent/catalog", s.requireAgent(s.handleCatalog))
	mux.HandleFunc("PUT /api/agent/config", s.requireAgent(s.handleAgentConfig))

	mux.HandleFunc("GET /install/{token}", s.handleInstall)
	mux.HandleFunc("GET /agent/bin", s.handleAgentBin)

	web, err := fs.Sub(assets.Web, "dist")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", spa(web))
	return mux
}

// Run listens for the wizard, then serves HTTPS until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	if err := s.loadCert(); err != nil {
		return err
	}
	_ = s.loadCA()
	go s.updates.Loop(ctx)
	log.Printf("nut-allergy %s", version.Version)
	handler := s.Handler()
	setup, err := s.store.GetSetup()
	if err != nil {
		return err
	}
	if !setup.HasCert {
		plain := &http.Server{
			Addr:              s.setupAddr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      5 * time.Minute,
		}
		go func() {
			log.Printf("setup wizard on http://%s", listenHost(s.setupAddr))
			if err := plain.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("setup listener: %v", err)
			}
		}()
		select {
		case <-ctx.Done():
			return shutdown(plain, context.Background())
		case <-s.certReady:
			_ = shutdown(plain, context.Background())
		}
	}
	go s.pollLoop(ctx)
	go s.renewLoop(ctx)

	tlsSrv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      5 * time.Minute,
	}
	ln, err := net.Listen("tcp", s.httpsAddr)
	if err != nil {
		return err
	}
	tlsLn := tls.NewListener(ln, &tls.Config{
		GetCertificate: s.getCertificate,
		ClientAuth:     tls.RequestClientCert,
		MinVersion:     tls.VersionTLS12,
	})
	log.Printf("https on https://%s", listenHost(s.httpsAddr))
	errCh := make(chan error, 1)
	go func() { errCh <- tlsSrv.Serve(tlsLn) }()
	select {
	case <-ctx.Done():
		return shutdown(tlsSrv, context.Background())
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func shutdown(srv *http.Server, ctx context.Context) error {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return srv.Shutdown(c)
}

func listenHost(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "0.0.0.0" + addr
	}
	return addr
}

func (s *Server) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c := s.cert.Load()
	if c == nil {
		return nil, errors.New("certificate is not configured")
	}
	return c, nil
}

func (s *Server) loadCert() error {
	pemCert, pemKey, _, err := s.store.Cert()
	if err != nil {
		if strings.Contains(err.Error(), "not configured") {
			return nil
		}
		return err
	}
	return s.useCert(pemCert, pemKey)
}

func (s *Server) useCert(certPEM, keyPEM string) error {
	pair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return err
	}
	s.cert.Store(&pair)
	if err := s.writeCertFiles(certPEM, keyPEM); err != nil {
		return err
	}
	s.readyOnce.Do(func() { close(s.certReady) })
	return nil
}

func (s *Server) writeCertFiles(certPEM, keyPEM string) error {
	dir := filepath.Join(s.dataDir, "tls")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), []byte(certPEM), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "key.pem"), []byte(keyPEM), 0o600)
}

func (s *Server) loadCA() error {
	certPEM, keyPEM, err := s.store.CA()
	if err != nil {
		return err
	}
	ca, err := pki.ParseCA(certPEM, keyPEM)
	if err != nil {
		return err
	}
	s.ca.Store(ca)
	return nil
}

func (s *Server) ensureCA(now time.Time) (*pki.CA, error) {
	if ca := s.ca.Load(); ca != nil {
		return ca, nil
	}
	if err := s.loadCA(); err == nil {
		return s.ca.Load(), nil
	}
	ca, keyPEM, err := pki.NewCA(now)
	if err != nil {
		return nil, err
	}
	if err := s.store.SetCA(ca.CertPEM, keyPEM); err != nil {
		return nil, err
	}
	s.ca.Store(ca)
	return ca, nil
}

func spa(web fs.FS) http.Handler {
	files := http.FileServer(http.FS(web))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/install/") || r.URL.Path == "/agent/bin" {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(web, path); err != nil {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}
