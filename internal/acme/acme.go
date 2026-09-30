// Package acme issues certificates with Lego using the DNS-01 challenge.
// Nothing is served for HTTP-01. The process only makes outbound calls.
package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/digitalocean"
	"github.com/go-acme/lego/v4/providers/dns/dnsimple"
	"github.com/go-acme/lego/v4/providers/dns/gandi"
	"github.com/go-acme/lego/v4/providers/dns/hetzner"
	"github.com/go-acme/lego/v4/providers/dns/linode"
	"github.com/go-acme/lego/v4/registration"
)

// Provider is a DNS host that accepts a single API token.
type Provider struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Providers are the DNS-01 backends the wizard offers.
func Providers() []Provider {
	return []Provider{
		{ID: "cloudflare", Label: "Cloudflare"},
		{ID: "digitalocean", Label: "DigitalOcean"},
		{ID: "hetzner", Label: "Hetzner"},
		{ID: "gandi", Label: "Gandi"},
		{ID: "dnsimple", Label: "DNSimple"},
		{ID: "linode", Label: "Linode"},
	}
}

// KnownProvider reports whether id is supported.
func KnownProvider(id string) bool {
	for _, p := range Providers() {
		if p.ID == id {
			return true
		}
	}
	return false
}

// Material is a certificate plus the account data needed to renew it.
type Material struct {
	CertPEM      string
	KeyPEM       string
	AccountKey   string
	Registration string
	Resource     string
}

type account struct {
	email string
	reg   *registration.Resource
	key   crypto.PrivateKey
}

func (a *account) GetEmail() string                        { return a.email }
func (a *account) GetRegistration() *registration.Resource { return a.reg }
func (a *account) GetPrivateKey() crypto.PrivateKey        { return a.key }

// Issue obtains or renews a certificate for domain. accountKeyPEM and regJSON
// may be empty on the first issuance. resourceJSON may be empty until a
// certificate exists; when it is set, the certificate is renewed.
func Issue(email, domain, provider, token, accountKeyPEM, regJSON, resourceJSON string) (Material, error) {
	if !KnownProvider(provider) {
		return Material{}, fmt.Errorf("unsupported dns provider %q", provider)
	}
	acc, err := loadAccount(email, accountKeyPEM, regJSON)
	if err != nil {
		return Material{}, err
	}
	config := lego.NewConfig(acc)
	config.Certificate.KeyType = certcrypto.EC256
	client, err := lego.NewClient(config)
	if err != nil {
		return Material{}, err
	}
	dns, err := dnsProvider(provider, token)
	if err != nil {
		return Material{}, err
	}
	// The provider API writes the TXT record. Lego's pre-check otherwise uses
	// /etc/resolv.conf, which on a LAN is split DNS and will not see it.
	// Public resolvers find the real authoritative nameservers, and those are
	// what the check queries.
	if err := client.Challenge.SetDNS01Provider(dns, dns01.AddRecursiveNameservers([]string{"1.1.1.1:53", "1.0.0.1:53"})); err != nil {
		return Material{}, err
	}
	if acc.reg == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return Material{}, err
		}
		acc.reg = reg
	}
	var res *certificate.Resource
	if resourceJSON != "" {
		var existing certificate.Resource
		if err := json.Unmarshal([]byte(resourceJSON), &existing); err != nil {
			return Material{}, err
		}
		res, err = client.Certificate.Renew(existing, true, false, "")
		if err != nil {
			return Material{}, err
		}
	} else {
		res, err = client.Certificate.Obtain(certificate.ObtainRequest{
			Domains: []string{domain},
			Bundle:  true,
		})
		if err != nil {
			return Material{}, err
		}
	}
	keyPEM, err := marshalKey(acc.key)
	if err != nil {
		return Material{}, err
	}
	regOut, err := json.Marshal(acc.reg)
	if err != nil {
		return Material{}, err
	}
	resOut, err := json.Marshal(res)
	if err != nil {
		return Material{}, err
	}
	return Material{
		CertPEM:      string(res.Certificate),
		KeyPEM:       string(res.PrivateKey),
		AccountKey:   keyPEM,
		Registration: string(regOut),
		Resource:     string(resOut),
	}, nil
}

func loadAccount(email, accountKeyPEM, regJSON string) (*account, error) {
	acc := &account{email: email}
	if accountKeyPEM == "" {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		acc.key = key
		return acc, nil
	}
	block, _ := pem.Decode([]byte(accountKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("acme account key pem missing")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	acc.key = key
	if regJSON != "" {
		var reg registration.Resource
		if err := json.Unmarshal([]byte(regJSON), &reg); err != nil {
			return nil, err
		}
		acc.reg = &reg
	}
	return acc, nil
}

func marshalKey(key crypto.PrivateKey) (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}

func dnsProvider(name, token string) (interface {
	Present(domain, token, keyAuth string) error
	CleanUp(domain, token, keyAuth string) error
}, error) {
	switch name {
	case "cloudflare":
		cfg := cloudflare.NewDefaultConfig()
		cfg.AuthToken = token
		return cloudflare.NewDNSProviderConfig(cfg)
	case "digitalocean":
		cfg := digitalocean.NewDefaultConfig()
		cfg.AuthToken = token
		return digitalocean.NewDNSProviderConfig(cfg)
	case "hetzner":
		cfg := hetzner.NewDefaultConfig()
		cfg.APIKey = token
		return hetzner.NewDNSProviderConfig(cfg)
	case "gandi":
		cfg := gandi.NewDefaultConfig()
		cfg.APIKey = token
		return gandi.NewDNSProviderConfig(cfg)
	case "dnsimple":
		cfg := dnsimple.NewDefaultConfig()
		cfg.AccessToken = token
		return dnsimple.NewDNSProviderConfig(cfg)
	case "linode":
		cfg := linode.NewDefaultConfig()
		cfg.Token = token
		return linode.NewDNSProviderConfig(cfg)
	default:
		return nil, fmt.Errorf("unsupported dns provider %q", name)
	}
}
