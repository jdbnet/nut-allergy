package alert

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
)

// SMTPConfig is used to send mail.
type SMTPConfig struct {
	Host     string
	Port     int
	TLSMode  string // none, starttls, tls
	Username string
	Password string
	From     string
	To       []string
}

// SendMail delivers a plain-text message.
func SendMail(cfg SMTPConfig, subject, body string) error {
	if cfg.Host == "" || len(cfg.To) == 0 || cfg.From == "" {
		return fmt.Errorf("smtp is not configured")
	}
	msg := strings.Join([]string{
		"From: " + cfg.From,
		"To: " + strings.Join(cfg.To, ", "),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	auth := smtpAuth(cfg)
	switch cfg.TLSMode {
	case "tls":
		return sendTLS(addr, cfg.Host, auth, cfg.From, cfg.To, []byte(msg))
	case "starttls", "":
		return sendSTARTTLS(addr, cfg.Host, auth, cfg.From, cfg.To, []byte(msg))
	case "none":
		return smtp.SendMail(addr, auth, cfg.From, cfg.To, []byte(msg))
	default:
		return fmt.Errorf("unknown tls mode %q", cfg.TLSMode)
	}
}

func smtpAuth(cfg SMTPConfig) smtp.Auth {
	if cfg.Username == "" {
		return nil
	}
	return smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
}

func sendTLS(addr, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	tlsCfg := &tls.Config{ServerName: host}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer client.Close()
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	return sendClient(client, from, to, msg)
}

func sendSTARTTLS(addr, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return err
		}
	}
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	return sendClient(client, from, to, msg)
}

func sendClient(client *smtp.Client, from string, to []string, msg []byte) error {
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// ParseRecipients splits comma or newline separated addresses.
func ParseRecipients(raw string) []string {
	raw = strings.ReplaceAll(raw, "\n", ",")
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
