// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Command fakeidp runs the small OpenID Connect provider of the tests (internal/authn/authntest)
// as a process, for the browser test of the walk-through of milestone 1 (SD21, Pascal's decision
// 2026-11-20: the rebuilt provider, no Keycloak in the CI). Only for tests: it signs in whoever
// POST /next names, without a password.
//
//	fakeidp -addr 127.0.0.1:18083 [-issuer http://127.0.0.1:18083]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"group24.de/itsm/internal/authn/authntest"
)

type process struct{}

func (process) Cleanup(func())    {}
func (process) Fatal(args ...any) { log.Fatal(args...) }

func main() {
	addr := flag.String("addr", "127.0.0.1:18083", "the address to listen on")
	issuer := flag.String("issuer", "", "the issuer as browser and server reach it (default http://<addr>)")
	flag.Parse()
	idp := authntest.NewAt(process{}, *addr)
	if *issuer != "" {
		idp.Issuer = *issuer
	}
	fmt.Printf("fakeidp: issuer %s, client %s\n", idp.Issuer, idp.ClientID)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	idp.Server.Close()
}
