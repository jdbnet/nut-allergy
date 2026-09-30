package alert

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
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

// SendMail delivers a multipart HTML and plain-text message.
func SendMail(cfg SMTPConfig, subject, plain, htmlBody string) error {
	if cfg.Host == "" || len(cfg.To) == 0 || cfg.From == "" {
		return fmt.Errorf("smtp is not configured")
	}
	msg := buildMIMEMessage(cfg.From, cfg.To, subject, plain, htmlBody)
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

func buildMIMEMessage(from string, to []string, subject, plain, htmlBody string) []byte {
	if htmlBody == "" {
		return []byte(strings.Join([]string{
			"From: " + from,
			"To: " + strings.Join(to, ", "),
			"Subject: " + mime.QEncoding.Encode("utf-8", subject),
			"MIME-Version: 1.0",
			"Content-Type: text/plain; charset=UTF-8",
			"",
			plain,
		}, "\r\n"))
	}
	boundary := "nut-allergy-" + base64.RawURLEncoding.EncodeToString([]byte(subject))[:16]
	var buf bytes.Buffer
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	buf.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n")
	buf.WriteString("\r\n")
	writePart(&buf, boundary, "text/plain; charset=UTF-8", plain)
	writePart(&buf, boundary, "text/html; charset=UTF-8", htmlBody)
	buf.WriteString("--" + boundary + "--\r\n")
	return buf.Bytes()
}

func writePart(buf *bytes.Buffer, boundary, contentType, body string) {
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: " + contentType + "\r\n")
	buf.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	buf.WriteString(body)
	buf.WriteString("\r\n")
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
