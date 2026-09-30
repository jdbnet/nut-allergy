package server

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"log"
	"time"

	"nut-allergy/internal/acme"
	"nut-allergy/internal/snmp"
)

func (s *Server) pollLoop(ctx context.Context) {
	s.pollOnce()
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.pollOnce()
		}
	}
}

func (s *Server) pollOnce() {
	ups, err := s.store.ListUPS()
	if err != nil {
		log.Printf("list ups: %v", err)
		return
	}
	now := time.Now()
	for _, u := range ups {
		target, err := s.store.Target(u.ID)
		if err != nil {
			log.Printf("ups %s credentials: %v", u.Name, err)
			continue
		}
		reading := snmp.Poll(target)
		if reading.Error != "" {
			log.Printf("ups %s: %s", u.Name, reading.Error)
		}
		if err := s.store.ApplyReading(u.ID, reading, now); err != nil {
			log.Printf("ups %s reading: %v", u.Name, err)
		}
	}
}

func (s *Server) renewLoop(ctx context.Context) {
	t := time.NewTicker(12 * time.Hour)
	defer t.Stop()
	s.renewOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.renewOnce()
		}
	}
}

func (s *Server) renewOnce() {
	st, err := s.store.GetSetup()
	if err != nil || st.TLSMode != "lego" || !st.HasCert {
		return
	}
	pemCert, _, _, err := s.store.Cert()
	if err != nil {
		return
	}
	block, _ := pem.Decode([]byte(pemCert))
	if block == nil {
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return
	}
	if time.Until(cert.NotAfter) > 30*24*time.Hour {
		return
	}
	provider, token, email, err := s.store.DNS()
	if err != nil {
		log.Printf("renew: %v", err)
		return
	}
	accountKey, reg, err := s.store.ACME()
	if err != nil {
		log.Printf("renew: %v", err)
		return
	}
	resource, err := s.store.ACMEResource()
	if err != nil {
		log.Printf("renew: %v", err)
		return
	}
	mat, err := acme.Issue(email, st.Hostname, provider, token, accountKey, reg, resource)
	if err != nil {
		log.Printf("renew: %v", err)
		return
	}
	if err := s.store.SetACME(mat.AccountKey, mat.Registration); err != nil {
		log.Printf("renew store account: %v", err)
		return
	}
	if err := s.store.SetACMEResource(mat.Resource); err != nil {
		log.Printf("renew store resource: %v", err)
		return
	}
	if err := s.store.PutCert("lego", mat.CertPEM, mat.KeyPEM); err != nil {
		log.Printf("renew store cert: %v", err)
		return
	}
	if err := s.useCert(mat.CertPEM, mat.KeyPEM); err != nil {
		log.Printf("renew load cert: %v", err)
		return
	}
	log.Printf("renewed certificate for %s", st.Hostname)
}
