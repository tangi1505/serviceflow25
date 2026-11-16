// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SMTP is the mail server (ITSM_SMTP_*); an empty Host means no mail.
type SMTP struct {
	Host     string
	Port     int
	From     string
	User     string
	Password string
	// StartTLS upgrades the connection before the login; required with a login.
	StartTLS bool
}

// Message is one mail as plain text.
type Message struct {
	To      string
	Subject string
	Body    string
	Date    time.Time
}

// Mailer sends over SMTP.
type Mailer struct {
	cfg SMTP
	// Timeout bounds one delivery.
	Timeout time.Duration
}

// NewMailer wires a mailer.
func NewMailer(cfg SMTP) *Mailer {
	if cfg.Port == 0 {
		cfg.Port = 25
	}
	return &Mailer{cfg: cfg, Timeout: 30 * time.Second}
}

// Enabled says whether a mail server is set; without one no mail goes out (WF19: the inbox
// still fills).
func (m *Mailer) Enabled() bool { return m != nil && m.cfg.Host != "" }

// Send delivers one message.
func (m *Mailer) Send(ctx context.Context, msg Message) error {
	if _, err := mail.ParseAddress(msg.To); err != nil {
		return fmt.Errorf("address %q: %w", msg.To, err)
	}
	raw, err := m.build(msg)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))
	ctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	c, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = c.Close() }()
	if err := c.Hello("itsm"); err != nil {
		return err
	}
	if m.cfg.StartTLS {
		if err := c.StartTLS(&tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if m.cfg.User != "" {
		if err := c.Auth(smtp.PlainAuth("", m.cfg.User, m.cfg.Password, m.cfg.Host)); err != nil {
			return err
		}
	}
	from, err := mail.ParseAddress(m.cfg.From)
	if err != nil {
		return fmt.Errorf("sender %q: %w", m.cfg.From, err)
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(msg.To); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// build writes the message: UTF-8 headers encoded as RFC 2047 where needed, the text
// quoted-printable.
func (m *Mailer) build(msg Message) ([]byte, error) {
	var b bytes.Buffer
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	domain := "itsm"
	if from, err := mail.ParseAddress(m.cfg.From); err == nil {
		if _, d, ok := strings.Cut(from.Address, "@"); ok {
			domain = d
		}
	}
	header := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	header("From", m.cfg.From)
	header("To", msg.To)
	header("Subject", mime.QEncoding.Encode("utf-8", strings.ReplaceAll(msg.Subject, "\n", " ")))
	header("Date", msg.Date.Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	header("Auto-Submitted", "auto-generated")
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(strings.ReplaceAll(msg.Body, "\n", "\r\n"))); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
