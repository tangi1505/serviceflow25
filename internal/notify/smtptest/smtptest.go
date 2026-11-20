// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package smtptest is an SMTP server for tests (WF15): it accepts every mail on a local port
// and keeps it, decoded, so that a test can read what was sent. Mailpit does the same for
// people (Compose profile mail).
package smtptest

import (
	"bufio"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
)

// Mail is one received message.
type Mail struct {
	From    string
	To      []string
	Subject string
	Body    string
	Header  mail.Header
}

// Server is a running test server.
type Server struct {
	Host string
	Port int
	ln   net.Listener
	mu   sync.Mutex
	mail []Mail
	// refused are the recipients answered with 550, as a mail server that will not take them.
	refused map[string]bool
}

// Refuse makes the server answer RCPT for the address with 550 from now on.
func (s *Server) Refuse(addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refused == nil {
		s.refused = map[string]bool{}
	}
	s.refused[strings.ToLower(addr)] = true
}

// Start runs a server until the test ends.
func Start(t *testing.T) *Server {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	s := &Server{Host: "127.0.0.1", Port: addr.Port, ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

// Mail returns what arrived so far.
func (s *Server) Mail() []Mail {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Mail(nil), s.mail...)
}

func (s *Server) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	reply := func(line string) { _, _ = io.WriteString(conn, line+"\r\n") }
	reply("220 smtptest")
	var from string
	var to []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			reply("250 smtptest")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			from = strings.Trim(strings.TrimSpace(line)[10:], "<> ")
			reply("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			rcpt := strings.Trim(strings.TrimSpace(line)[8:], "<> ")
			s.mu.Lock()
			no := s.refused[strings.ToLower(rcpt)]
			s.mu.Unlock()
			if no {
				reply("550 refused")
				continue
			}
			to = append(to, rcpt)
			reply("250 ok")
		case cmd == "DATA":
			reply("354 go on")
			var data strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" || l == ".\n" {
					break
				}
				data.WriteString(strings.TrimPrefix(l, "."))
			}
			s.keep(from, to, data.String())
			from, to = "", nil
			reply("250 kept")
		case cmd == "RSET":
			from, to = "", nil
			reply("250 ok")
		case cmd == "QUIT":
			reply("221 bye")
			return
		default:
			reply("250 ok")
		}
	}
}

func (s *Server) keep(from string, to []string, raw string) {
	m := Mail{From: from, To: to}
	if msg, err := mail.ReadMessage(strings.NewReader(raw)); err == nil {
		m.Header = msg.Header
		m.Subject, _ = new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
		body, _ := io.ReadAll(quotedprintable.NewReader(msg.Body))
		m.Body = strings.ReplaceAll(string(body), "\r\n", "\n")
	}
	s.mu.Lock()
	s.mail = append(s.mail, m)
	s.mu.Unlock()
}
