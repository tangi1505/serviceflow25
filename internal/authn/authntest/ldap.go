// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package authntest

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jimlambrt/gldap"
)

// LDAP is a small directory for tests: one bind DN with a password, and a search that answers
// the person filter with the persons and the group filter with the groups, as lldap would.
// Entries are set with SetPersons and SetGroups and may change between syncs.
type LDAP struct {
	URL      string
	BindDN   string
	Password string
	BaseDN   string
	// PersonFilter and GroupFilter are the filters the server answers.
	PersonFilter string
	GroupFilter  string

	server *gldap.Server
	mu     sync.Mutex
	people []*gldap.Entry
	groups []*gldap.Entry
}

// Person is an entry of ou=people.
type Person struct {
	ID        string
	UID       string
	FirstName string
	LastName  string
	Email     string
}

// Group is an entry of ou=groups with member DNs.
type Group struct {
	ID      string
	Name    string
	Members []string // uids
}

// StartLDAP runs the directory on a free port; it stops with the test.
func StartLDAP(t interface {
	Cleanup(func())
	Fatal(...any)
}) *LDAP {
	l := &LDAP{BindDN: "uid=admin,ou=people,dc=example,dc=org", Password: "admin-test", BaseDN: "dc=example,dc=org",
		PersonFilter: "(objectClass=person)", GroupFilter: "(objectClass=groupOfUniqueNames)"}
	s, err := gldap.NewServer()
	if err != nil {
		t.Fatal(err)
	}
	mux, err := gldap.NewMux()
	if err != nil {
		t.Fatal(err)
	}
	_ = mux.Bind(l.bind)
	_ = mux.Search(l.search)
	_ = s.Router(mux)
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	go func() { _ = s.Run(addr) }()
	for i := 0; i < 100 && !s.Ready(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	l.server = s
	l.URL = "ldap://" + addr
	t.Cleanup(func() { _ = s.Stop() })
	return l
}

// PersonDN is the DN of a person with the uid.
func (l *LDAP) PersonDN(uid string) string {
	return fmt.Sprintf("uid=%s,ou=people,%s", uid, l.BaseDN)
}

// SetPersons replaces the persons.
func (l *LDAP) SetPersons(people ...Person) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.people = l.people[:0]
	for _, p := range people {
		l.people = append(l.people, gldap.NewEntry(l.PersonDN(p.UID), map[string][]string{
			"objectClass": {"inetOrgPerson", "person"}, "entryUUID": {p.ID}, "uid": {p.UID},
			"givenName": {p.FirstName}, "sn": {p.LastName}, "mail": {p.Email}, "cn": {p.FirstName + " " + p.LastName},
		}))
	}
}

// SetGroups replaces the groups.
func (l *LDAP) SetGroups(groups ...Group) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.groups = l.groups[:0]
	for _, g := range groups {
		members := make([]string, 0, len(g.Members))
		for _, uid := range g.Members {
			members = append(members, l.PersonDN(uid))
		}
		l.groups = append(l.groups, gldap.NewEntry(fmt.Sprintf("cn=%s,ou=groups,%s", g.Name, l.BaseDN), map[string][]string{
			"objectClass": {"groupOfUniqueNames", "groupOfNames"}, "entryUUID": {g.ID}, "cn": {g.Name},
			"member": members, "uniqueMember": members,
		}))
	}
}

func (l *LDAP) bind(w *gldap.ResponseWriter, r *gldap.Request) {
	resp := r.NewBindResponse(gldap.WithResponseCode(gldap.ResultInvalidCredentials))
	defer func() { _ = w.Write(resp) }()
	m, err := r.GetSimpleBindMessage()
	if err != nil {
		return
	}
	if strings.EqualFold(m.UserName, l.BindDN) && string(m.Password) == l.Password {
		resp.SetResultCode(gldap.ResultSuccess)
	}
}

func (l *LDAP) search(w *gldap.ResponseWriter, r *gldap.Request) {
	done := r.NewSearchDoneResponse()
	defer func() { _ = w.Write(done) }()
	m, err := r.GetSearchMessage()
	if err != nil {
		done.SetResultCode(gldap.ResultOperationsError)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var entries []*gldap.Entry
	switch strings.ToLower(m.Filter) {
	case strings.ToLower(l.PersonFilter):
		entries = l.people
	case strings.ToLower(l.GroupFilter):
		entries = l.groups
	}
	for _, e := range entries {
		attrs := map[string][]string{}
		for _, a := range e.Attributes {
			attrs[a.Name] = a.Values
		}
		_ = w.Write(r.NewSearchResponseEntry(e.DN, gldap.WithAttributes(attrs)))
	}
	done.SetResultCode(gldap.ResultSuccess)
}
