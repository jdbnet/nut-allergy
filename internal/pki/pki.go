// Package pki creates the internal CA that signs agent client certificates.
package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"
)

// CA is the internal authority for agent client certificates.
type CA struct {
	Cert    *x509.Certificate
	Key     *ecdsa.PrivateKey
	CertPEM string
}

// NewCA generates a new CA valid for ten years.
func NewCA(now time.Time) (*CA, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, "", err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "NUT Allergy Agent CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, "", err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, "", err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, "", err
	}
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	return &CA{Cert: cert, Key: key, CertPEM: certPEM}, keyPEM, nil
}

// ParseCA loads a CA from PEM.
func ParseCA(certPEM, keyPEM string) (*CA, error) {
	cb, _ := pem.Decode([]byte(certPEM))
	if cb == nil {
		return nil, errors.New("ca certificate pem missing")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, err
	}
	kb, _ := pem.Decode([]byte(keyPEM))
	if kb == nil {
		return nil, errors.New("ca key pem missing")
	}
	k, err := x509.ParsePKCS8PrivateKey(kb.Bytes)
	if err != nil {
		return nil, err
	}
	ek, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("ca key is not ecdsa")
	}
	return &CA{Cert: cert, Key: ek, CertPEM: certPEM}, nil
}

// SignCSR returns a client certificate PEM and its SHA-256 fingerprint.
func (c *CA) SignCSR(csrPEM, hostname string, now time.Time) (certPEM, fingerprint string, err error) {
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil {
		return "", "", errors.New("csr pem missing")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return "", "", err
	}
	if err := csr.CheckSignature(); err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: hostname},
		DNSNames:     []string{hostname},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(5 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.Cert, csr.PublicKey, c.Key)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(der)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), hex.EncodeToString(sum[:]), nil
}

// Fingerprint is the hex SHA-256 of a certificate.
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

// Pool returns a cert pool containing the CA.
func (c *CA) Pool() *x509.CertPool {
	p := x509.NewCertPool()
	p.AddCert(c.Cert)
	return p
}

// ValidForHostname reports whether certPEM is a certificate for hostname.
func ValidForHostname(certPEM, hostname string) bool {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	return cert.VerifyHostname(hostname) == nil
}

// ParseCertAndKey checks that a PEM certificate and private key match and
// that hostname is in the certificate.
func ParseCertAndKey(certPEM, keyPEM, hostname string) error {
	cb, _ := pem.Decode([]byte(certPEM))
	if cb == nil {
		return errors.New("certificate pem missing")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return fmt.Errorf("certificate: %w", err)
	}
	kb, _ := pem.Decode([]byte(keyPEM))
	if kb == nil {
		return errors.New("private key pem missing")
	}
	key, err := x509.ParsePKCS8PrivateKey(kb.Bytes)
	if err != nil {
		key, err = x509.ParsePKCS1PrivateKey(kb.Bytes)
		if err != nil {
			if ek, eerr := x509.ParseECPrivateKey(kb.Bytes); eerr == nil {
				key = ek
				err = nil
			}
		}
	}
	if err != nil {
		return fmt.Errorf("private key: %w", err)
	}
	if !pubMatches(cert, key) {
		return errors.New("certificate and private key do not match")
	}
	if hostname != "" && cert.VerifyHostname(hostname) != nil {
		return fmt.Errorf("certificate is not valid for %s", hostname)
	}
	return nil
}

func pubMatches(cert *x509.Certificate, key any) bool {
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
		return ok && pub.Equal(&k.PublicKey)
	default:
		priv, ok := key.(interface{ Public() any })
		if !ok {
			return false
		}
		type equaler interface{ Equal(x any) bool }
		if eq, ok := cert.PublicKey.(equaler); ok {
			return eq.Equal(priv.Public())
		}
		return false
	}
}
