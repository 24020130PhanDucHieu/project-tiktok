package service

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"

	"github.com/tiktoday/kocflow-api/internal/config"
)

// ErrEmailNotConfigured is returned when SMTP settings are missing.
var ErrEmailNotConfigured = errors.New("Email chưa được cấu hình SMTP trên server")

// Mailer sends plain-text emails to KOCs through SMTP.
type Mailer interface {
	SendKocEmail(to, displayName, tiktokID, text string) error
	Send(to, subject, text string) error
}

// SMTPMailer is the SMTP implementation of Mailer.
type SMTPMailer struct{ cfg config.Config }

// NewSMTPMailer builds a Mailer from the SMTP configuration.
func NewSMTPMailer(cfg config.Config) *SMTPMailer { return &SMTPMailer{cfg: cfg} }

// Configured reports whether SMTP credentials are present.
func (m *SMTPMailer) Configured() bool {
	return m.cfg.SMTPHost != "" && m.cfg.SMTPUser != "" && m.cfg.SMTPPassword != ""
}

// SendKocEmail sends the invitation / reminder email.
func (m *SMTPMailer) SendKocEmail(to, displayName, tiktokID, text string) error {
	if !m.Configured() {
		return ErrEmailNotConfigured
	}
	if text == "" {
		text = fmt.Sprintf("Chào %s,\n\nMình là KocFlow. Mình thấy kênh TikTok @%s phù hợp với một chiến dịch bên mình. Bạn cho mình xin phép trao đổi brief và mức phí hợp tác nhé?\n\nTrân trọng,\nKocFlow Studio", displayName, tiktokID)
	}
	return m.Send(to, "Cơ hội hợp tác TikTok dành cho "+displayName, text)
}

// Send delivers a plain-text email through SMTP.
func (m *SMTPMailer) Send(to, subject, text string) error {
	if !m.Configured() {
		return ErrEmailNotConfigured
	}
	msg := strings.Join([]string{
		"From: " + m.cfg.MailFrom,
		"To: " + to,
		"Subject: =?UTF-8?B?" + b64(subject) + "?=",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		text,
	}, "\r\n")

	addr := net.JoinHostPort(m.cfg.SMTPHost, strconv.Itoa(m.cfg.SMTPPort))
	auth := smtp.PlainAuth("", m.cfg.SMTPUser, m.cfg.SMTPPassword, m.cfg.SMTPHost)
	from := extractAddress(m.cfg.MailFrom)

	if m.cfg.SMTPPort == 465 { // implicit TLS
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: m.cfg.SMTPHost})
		if err != nil {
			return err
		}
		client, err := smtp.NewClient(conn, m.cfg.SMTPHost)
		if err != nil {
			return err
		}
		defer client.Close()
		return deliver(client, auth, from, to, msg)
	}
	conn, err := net.DialTimeout("tcp", addr, 15e9)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, m.cfg.SMTPHost)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: m.cfg.SMTPHost}); err != nil {
			return err
		}
	}
	return deliver(client, auth, from, to, msg)
}

func deliver(c *smtp.Client, auth smtp.Auth, from, to, msg string) error {
	if err := c.Auth(auth); err != nil {
		return err
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// extractAddress turns "Name <a@b.c>" into "a@b.c".
func extractAddress(v string) string {
	if i := strings.LastIndex(v, "<"); i >= 0 {
		if j := strings.Index(v[i:], ">"); j > 0 {
			return v[i+1 : i+j]
		}
	}
	return strings.TrimSpace(v)
}
