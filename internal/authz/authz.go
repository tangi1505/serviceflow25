// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package authz decides what a session may do (Plattform-Kern PL8 to PL10; Hauptplan E34;
// architektur.md §5). A role holds permissions (type, action, scope, condition in CEL, field
// rights); an assignment gives a role to a person or a group for a node of the organisation
// tree. For a request the grants of the account's person are loaded once (cached in the
// process and in Valkey, invalidated over Pub/Sub) and turned into a SQL condition for reads
// and a check in Go for writes. A session may narrow itself to one role and one node (PL10).
// Every session works in one tenant (PL44, E136): the filter keeps it inside, whatever the
// scope says.
package authz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/query"
	"group24.de/itsm/internal/rules"
)

// Actions every type has; packages may add special rights (any snake_case name).
const (
	ActionRead   = "read"
	ActionCreate = "create"
	ActionUpdate = "update"
	ActionDelete = "delete"
	// Any stands for every type or every action in a permission.
	Any = "*"
)

// Scope says which records of a type a permission reaches, always within the subtree of the
// assignment's node, except "all", which reaches every record of the tenant (E81, E136).
type Scope string

const (
	ScopeOwn   Scope = "own"   // records the person created
	ScopeGroup Scope = "group" // records created by someone who shares a group with the person
	// ScopeInvolved reaches the records where the person or one of their groups holds a role
	// (requester, observer, assigned, …; WF3).
	ScopeInvolved Scope = "involved"
	ScopeUnit     Scope = "unit"    // records of the assignment's node itself
	ScopeSubtree  Scope = "subtree" // records of the node and everything below
	ScopeAll      Scope = "all"     // every record of the tenant
)

// NoTenant is the tenant of a session that may enter none: an account of the installation
// without a role in any tenant. No node can have this path (node names are lower case), so
// such a session reaches no record.
const NoTenant = "root.NONE"

var (
	// ErrNotFound says there is no such role or assignment.
	ErrNotFound = errors.New("authz: not found")
	// ErrExists says the role name is taken.
	ErrExists = errors.New("authz: role exists")
	// ErrInvalid says a name, action, scope, condition or field list breaks the rules.
	ErrInvalid = errors.New("authz: invalid")
	// ErrNoPerson says the account is not linked to a person, so it cannot hold roles.
	ErrNoPerson = errors.New("authz: account has no person")
	// ErrInUse says a role is still assigned somewhere and cannot be deleted (PL47b).
	ErrInUse = errors.New("authz: role in use")
)

var (
	nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	scopes = map[Scope]bool{ScopeOwn: true, ScopeGroup: true, ScopeInvolved: true, ScopeUnit: true, ScopeSubtree: true, ScopeAll: true}
)

// Fields are the field rights of a permission (PL9): which fields may be read and which
// written. nil means every field. System fields are always readable and never writable here.
type Fields struct {
	Read  []string `json:"read,omitempty"`
	Write []string `json:"write,omitempty"`
}

// Grant is one permission as it reaches a person, with the node it was assigned at.
type Grant struct {
	TypeName  string  `json:"type_name"`
	Action    string  `json:"action"`
	Scope     Scope   `json:"scope"`
	OrgPath   string  `json:"org_path"`
	Role      string  `json:"role"`
	Condition string  `json:"condition,omitempty"`
	Fields    *Fields `json:"fields,omitempty"`
}

func (g Grant) matches(typeName, action string) bool {
	return (g.TypeName == Any || g.TypeName == typeName) && (g.Action == Any || g.Action == action)
}

// Access is what one session may do.
type Access struct {
	AccountID uuid.UUID  `json:"account_id"`
	PersonID  *uuid.UUID `json:"person_id,omitempty"`
	Grants    []Grant    `json:"grants"`
	// Mates are the accounts whose records the scope "group" reaches (the own included).
	Mates []uuid.UUID `json:"mates,omitempty"`
	// Groups are the groups the person is a member of: with the person they make the scope
	// "involved" and the filter "@my_groups" (WF3).
	Groups []uuid.UUID `json:"groups,omitempty"`
	// ActiveRole and ActiveOrg narrow the session (PL10): only grants of that role, only
	// records in that node's subtree. Empty means no narrowing.
	ActiveRole string `json:"active_role,omitempty"`
	ActiveOrg  string `json:"active_org,omitempty"`
	// Readers are the rules by which every signed-in person reads records of a type without a
	// grant, whatever the roles and the narrowing say: the select lists with one rule without
	// condition (PL16, E93; PL45), types with readers of their own (SD16a).
	Readers map[string][]meta.Reader `json:"readers,omitempty"`
	// Approvable are the types with the trait approvals (WF6): whoever is asked to approve a
	// record of one of them reads it, without a grant.
	Approvable map[string]bool `json:"approvable,omitempty"`
	// Represents are the persons the session's person stands in for at approvals (WF6, the
	// substitute of a person): while it lasts she decides for them and their groups.
	Represents []Represented `json:"represents,omitempty"`
	// Home is the tenant the account is bound to (PL44); empty for an account of the
	// installation.
	Home string `json:"home,omitempty"`
	// PersonNode is the node of the person's record: where a new record lands that names no
	// node, if the access may create there (PL46). Empty without a person.
	PersonNode string `json:"person_node,omitempty"`
	// Tenants are the tenants the account may work in: its own, or for an account of the
	// installation those where it holds a role (a role at the root reaches all); NoTenant for
	// none. Just the root while the installation has no tenants.
	Tenants []string `json:"tenants,omitempty"`
	// Tenant is the tenant of the request, set by Enter; empty means the first of Tenants, so
	// that work on behalf of an account (jobs, webhooks) never leaves its tenants. It is not
	// cached: the session chooses it.
	Tenant string `json:"-"`
	// system is the unrestricted access of the binary itself (migrations, CLI, tests).
	system bool
}

// Enter returns a copy of the access working in the tenant: the wanted one if the account
// may work there, else the first it may (PL44). The cached access stays whole.
func (a *Access) Enter(want string) *Access {
	c := *a
	switch {
	case want != "" && containsString(a.Tenants, want):
		c.Tenant = want
	case len(a.Tenants) > 0:
		c.Tenant = a.Tenants[0]
	}
	return &c
}

// boundary is the node the request stays within, or "" for none: the system, the root, and an
// access built without tenants (tests).
func (a *Access) boundary() string {
	if a == nil || a.system {
		return ""
	}
	t := a.Tenant
	if t == "" && len(a.Tenants) > 0 {
		t = a.Tenants[0]
	}
	if t == org.Root {
		return ""
	}
	return t
}

// Bounded reports whether the request stays within a tenant.
func (a *Access) Bounded() bool { return a.boundary() != "" }

// CurrentTenant is the tenant of the request: the root without tenants.
func (a *Access) CurrentTenant() string {
	if b := a.boundary(); b != "" {
		return b
	}
	return org.Root
}

// HomeNode is where something lands that names no node of its own: the tenant of the request,
// else the root.
func (a *Access) HomeNode() string {
	if b := a.boundary(); b != "" && b != NoTenant {
		return b
	}
	return org.Root
}

// InTenant reports whether a node lies in the tenant of the request (PL44): anything else is
// not there for the session (404).
func (a *Access) InTenant(path string) bool {
	b := a.boundary()
	return b == "" || within(path, b)
}

// reaches reports whether a grant counts in the tenant of the request: its node lies in the
// tenant or above it (a role at the root counts in every tenant). A role in another tenant
// does not count, not even with the scope "all".
func (a *Access) reaches(g Grant) bool {
	b := a.boundary()
	return b == "" || within(g.OrgPath, b) || within(b, g.OrgPath)
}

// Installation reports whether the access holds the action on the type from a role at the
// root: what belongs to the installation (roles, types, settings of the root) needs that,
// in whatever tenant the session works (PL44).
func (a *Access) Installation(typeName, action string) bool {
	if a.IsSystem() {
		return true
	}
	for _, g := range a.grantsFor(typeName, action) {
		if g.OrgPath == org.Root && (g.Scope == ScopeAll || g.Scope == ScopeSubtree) {
			return true
		}
	}
	return false
}

// The right to evaluate across tenants (PL48, Hauptplan E137, E146): the action report_tenants
// on the pseudo type core.tenant.
const (
	TenantType          = "core.tenant"
	ActionReportTenants = "report_tenants"
)

// ReportsAcrossTenants reports whether the access may run evaluations across tenants (PL48,
// your decision of 2026-10-30): an account of the installation that holds the right named in a
// role at the root. Every action of a role (the built-in admin) does not count, and an account
// of a tenant never has it, so that an administrator of one tenant cannot look past its border.
func (a *Access) ReportsAcrossTenants() bool {
	if a == nil || a.system || a.Home != "" {
		return false
	}
	for _, g := range a.Grants {
		if g.TypeName == TenantType && g.Action == ActionReportTenants && g.OrgPath == org.Root &&
			(g.Scope == ScopeAll || g.Scope == ScopeSubtree) && (a.ActiveRole == "" || g.Role == a.ActiveRole) {
			return true
		}
	}
	return false
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// readers lists the rules by which the access reads a type without a grant, or does another
// action a rule lets do (comment, SD18).
func (a *Access) readers(typeName, action string) []meta.Reader {
	if action == ActionRead {
		return a.Readers[typeName]
	}
	var out []meta.Reader
	for _, r := range a.Readers[typeName] {
		if r.Lets(action) {
			out = append(out, r)
		}
	}
	return out
}

// System is the access of the binary itself: everything, everywhere.
func System() *Access { return &Access{system: true} }

// SystemIn is the access of the binary inside one tenant (PL45): every right, but only there;
// the system itself without tenants (an empty tenant or the root).
func SystemIn(tenant string) *Access {
	if tenant == "" || tenant == org.Root {
		return System()
	}
	return &Access{Tenants: []string{tenant}, Tenant: tenant,
		Grants: []Grant{{TypeName: Any, Action: Any, Scope: ScopeAll, OrgPath: org.Root, Role: "system"}}}
}

// IsSystem reports whether the access is the unrestricted one.
func (a *Access) IsSystem() bool { return a == nil || a.system }

// grantsFor lists the grants that cover a type and action, within the active role and the
// tenant.
func (a *Access) grantsFor(typeName, action string) []Grant {
	var out []Grant
	for _, g := range a.Grants {
		if (a.ActiveRole != "" && g.Role != a.ActiveRole) || !a.reaches(g) {
			continue
		}
		if g.matches(typeName, action) {
			out = append(out, g)
		}
	}
	return out
}

// Me is the person of the session, or nil (an account without a person, the system).
func (a *Access) Me() *uuid.UUID {
	if a == nil {
		return nil
	}
	return a.PersonID
}

// MyGroups are the groups of the session's person; empty without one.
func (a *Access) MyGroups() []uuid.UUID {
	if a == nil {
		return nil
	}
	return a.Groups
}

// involved lists the person and their groups: the holders the scope "involved" looks for.
func (a *Access) involved() []uuid.UUID {
	out := make([]uuid.UUID, 0, len(a.Groups)+1)
	seen := map[uuid.UUID]bool{}
	if a.PersonID != nil {
		out = append(out, *a.PersonID)
		seen[*a.PersonID] = true
	}
	for _, g := range a.Groups {
		if !seen[g] {
			out = append(out, g)
			seen[g] = true
		}
	}
	return out
}

// Allowed reports whether the action on the type is granted at all (for some records).
func (a *Access) Allowed(typeName, action string) bool {
	return a.IsSystem() || len(a.readers(typeName, action)) > 0 || len(a.grantsFor(typeName, action)) > 0
}

// Roles lists the roles the access holds in the tenant, for the session to choose from (PL10).
func (a *Access) Roles() []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range a.Grants {
		if a.reaches(g) && !seen[g.Role] {
			seen[g.Role] = true
			out = append(out, g.Role)
		}
	}
	return out
}

// Nodes lists the nodes the access holds roles at in the tenant (PL10).
func (a *Access) Nodes() []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range a.Grants {
		if a.reaches(g) && !seen[g.OrgPath] {
			seen[g.OrgPath] = true
			out = append(out, g.OrgPath)
		}
	}
	return out
}

// Reach is a node where an action is granted: with the nodes below it, or that node alone
// (the scope unit).
type Reach struct {
	Path  string
	Below bool
}

// Covers reports whether the reach includes the node.
func (r Reach) Covers(path string) bool {
	if r.Below {
		return within(path, r.Path)
	}
	return path == r.Path
}

// CreateReach lists where the access may create records of the type (PL46), narrowed to the
// tenant and the node of the session's context: a role above the tenant yields the tenant. A
// reach inside another one is left out. Conditions of grants are not weighed.
func (a *Access) CreateReach(typeName string) []Reach {
	b := a.boundary()
	if b == NoTenant {
		return nil
	}
	var all []Reach
	for _, g := range a.grantsFor(typeName, ActionCreate) {
		r := Reach{Path: g.OrgPath, Below: g.Scope != ScopeUnit}
		for _, narrow := range []string{b, a.ActiveOrg} {
			if narrow == "" || within(r.Path, narrow) {
				continue
			}
			if !r.Below || !within(narrow, r.Path) {
				r.Path = ""
				break
			}
			r.Path = narrow
		}
		if r.Path != "" {
			all = append(all, r)
		}
	}
	var out []Reach
	for i, r := range all {
		inner := false
		for j, o := range all {
			// o swallows r: a larger reach, or the same one listed earlier.
			if i != j && o.Below && o.Covers(r.Path) && (o != r || j < i) {
				inner = true
				break
			}
		}
		if !inner {
			out = append(out, r)
		}
	}
	return out
}

// CreateNode is where a new record of the type lands that names no node (PL46, E136): the node
// of the session's context; else the person's node if the access may create there; else the
// one node it may create in. ok is false when that leaves a choice. Without any right it is
// the tenant, and the check of the write refuses.
func (a *Access) CreateNode(typeName string) (node string, ok bool) {
	if a.IsSystem() {
		return a.HomeNode(), true
	}
	if a.ActiveOrg != "" {
		return a.ActiveOrg, true
	}
	reach := a.CreateReach(typeName)
	if a.PersonNode != "" {
		for _, r := range reach {
			if r.Covers(a.PersonNode) {
				return a.PersonNode, true
			}
		}
	}
	switch len(reach) {
	case 0:
		return a.HomeNode(), true
	case 1:
		return reach[0].Path, true
	}
	return "", false
}

// Filter returns the SQL condition that keeps the records the access may read (or act on
// with the given action), and the arguments appended to args. Placeholders continue after
// len(args). Without a grant the condition is FALSE. The condition of a grant (CEL) joins
// its scope with AND; untranslatable conditions were refused when the permission was saved.
// The tenant bounds every scope (PL44): org_path <@ tenant joins the whole condition. Whoever
// is asked to approve a record reads it (WF6).
func (a *Access) Filter(typeName, action string, args []any) (string, []any) {
	if a.IsSystem() {
		return "TRUE", args
	}
	f, args := a.filter(typeName, action, args)
	if action != ActionRead || !a.Approvable[typeName] {
		return f, args
	}
	asked := a.Asked(time.Now())
	if len(asked) == 0 {
		return f, args
	}
	args = append(args, typeName, asked)
	vis := fmt.Sprintf("id IN (SELECT aps.record_id FROM approval_step aps WHERE aps.type_name = $%d AND aps.state IN ('open', 'approved', 'rejected') AND aps.approver_id = ANY($%d::uuid[]))",
		len(args)-1, len(args))
	if b := a.boundary(); b != "" {
		args = append(args, b)
		vis = fmt.Sprintf("(%s AND org_path <@ $%d::ltree)", vis, len(args))
	}
	if f == "FALSE" {
		return vis, args
	}
	return "(" + f + " OR " + vis + ")", args
}

func (a *Access) filter(typeName, action string, args []any) (string, []any) {
	b := a.boundary()
	listVis := ""
	if rs := a.readers(typeName, action); len(rs) > 0 {
		listVis, args = a.readerFilter(typeName, rs, args)
		if len(a.grantsFor(typeName, action)) == 0 {
			if listVis == "" {
				return "FALSE", args
			}
			return listVis, args
		}
	}
	var parts []string
	// The grants "involved" without a condition become one condition over all their nodes
	// (PL57): each of them as an OR of its own made PostgreSQL hash every actor row of the
	// person's groups for every query (found with H14: the queue of a team, 3 s).
	var involvedAt []string
	for _, g := range a.grantsFor(typeName, action) {
		if g.Scope == ScopeInvolved && g.Condition == "" {
			if !slices.Contains(involvedAt, g.OrgPath) {
				involvedAt = append(involvedAt, g.OrgPath)
			}
			continue
		}
		var scope string
		switch g.Scope {
		case ScopeAll:
			scope = "TRUE"
		case ScopeSubtree:
			args = append(args, g.OrgPath)
			scope = fmt.Sprintf("org_path <@ $%d::ltree", len(args))
		case ScopeUnit:
			args = append(args, g.OrgPath)
			scope = fmt.Sprintf("org_path = $%d::ltree", len(args))
		case ScopeOwn:
			args = append(args, g.OrgPath, a.AccountID)
			scope = fmt.Sprintf("(org_path <@ $%d::ltree AND created_by = $%d)", len(args)-1, len(args))
		case ScopeGroup:
			args = append(args, g.OrgPath, a.Mates)
			scope = fmt.Sprintf("(org_path <@ $%d::ltree AND created_by = ANY($%d::uuid[]))", len(args)-1, len(args))
		case ScopeInvolved:
			// The actor rows of the type that name the person or one of their groups; the bare
			// id is the record's, also inside a nested query on another table.
			args = append(args, g.OrgPath, typeName, a.involved())
			scope = fmt.Sprintf("(org_path <@ $%d::ltree AND id IN (SELECT act.record_id FROM actor act WHERE act.type_name = $%d AND act.target_id = ANY($%d::uuid[])))",
				len(args)-2, len(args)-1, len(args))
		default:
			continue
		}
		if g.Condition != "" {
			cond, newArgs, err := Translate(g.Condition, args)
			if err != nil {
				// Refused at save time; a stored one that fails now grants nothing.
				continue
			}
			args = newArgs
			scope = "(" + scope + " AND " + cond + ")"
		}
		parts = append(parts, scope)
	}
	if len(involvedAt) > 0 {
		var at []string
		for _, p := range involvedAt {
			args = append(args, p)
			at = append(at, fmt.Sprintf("org_path <@ $%d::ltree", len(args)))
		}
		args = append(args, typeName, a.involved())
		parts = append(parts, fmt.Sprintf("((%s) AND id IN (SELECT act.record_id FROM actor act WHERE act.type_name = $%d AND act.target_id = ANY($%d::uuid[])))",
			strings.Join(at, " OR "), len(args)-1, len(args)))
	}
	if len(parts) == 0 {
		if listVis != "" {
			return listVis, args
		}
		return "FALSE", args
	}
	filter := "(" + strings.Join(parts, " OR ") + ")"
	if a.ActiveOrg != "" {
		args = append(args, a.ActiveOrg)
		filter = fmt.Sprintf("(%s AND org_path <@ $%d::ltree)", filter, len(args))
	}
	if b != "" {
		args = append(args, b)
		filter = fmt.Sprintf("(%s AND org_path <@ $%d::ltree)", filter, len(args))
	}
	if listVis != "" {
		filter = "(" + filter + " OR " + listVis + ")"
	}
	return filter, args
}

// readerFilter is the SQL condition of the readers of a type (SD16a): a select list (PL16)
// has one without condition, so an entry is there for the persons at its node and below, one
// at the root for everyone (SD-Plan N5); a reader with roles reaches the records where the
// person or one of their groups holds one of them. The tenant bounds it (PL45, V28); a grant
// of one's own adds what it gives. A condition that does not translate lets nobody read.
func (a *Access) readerFilter(typeName string, rs []meta.Reader, args []any) (string, []any) {
	var parts []string
	for _, r := range rs {
		cond := ""
		if r.When != "" {
			c, newArgs, err := Translate(r.When, args)
			if err != nil {
				continue
			}
			cond, args = c, newArgs
		}
		var part string
		if len(r.Roles) == 0 {
			args = append(args, a.listNode())
			part = fmt.Sprintf("$%d::ltree <@ org_path", len(args))
		} else {
			args = append(args, typeName, r.Roles, a.involved())
			part = fmt.Sprintf("id IN (SELECT act.record_id FROM actor act WHERE act.type_name = $%d AND act.role = ANY($%d::text[]) AND act.target_id = ANY($%d::uuid[]))",
				len(args)-2, len(args)-1, len(args))
		}
		if cond != "" {
			part = "(" + part + " AND " + cond + ")"
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "", args
	}
	vis := "(" + strings.Join(parts, " OR ") + ")"
	if b := a.boundary(); b != "" {
		args = append(args, b)
		vis = fmt.Sprintf("(%s AND org_path <@ $%d::ltree)", vis, len(args))
	}
	return vis, args
}

// reads reports whether one of the readers of the type reaches the record: the same rules as
// readerFilter, decided in Go.
func (a *Access) reads(rs []meta.Reader, rec map[string]any) bool {
	for _, r := range rs {
		if len(r.Roles) == 0 {
			orgPath, _ := rec["org_path"].(string)
			if !within(a.listNode(), orgPath) {
				continue
			}
		} else if !a.holdsRole(rec, r.Roles...) {
			continue
		}
		if r.When != "" {
			// Only what the filter would let through: a condition that does not translate
			// lets nobody read there either.
			if _, _, err := Translate(r.When, nil); err != nil {
				continue
			}
			c, err := rules.Compile(r.When)
			if err != nil {
				continue
			}
			if met, err := c.Eval(rec); err != nil || !met {
				continue
			}
		}
		return true
	}
	return false
}

// listNode is the node from which the entries of the select lists are seen (SD-Plan N5): the
// node of the person, without one the tenant or the root.
func (a *Access) listNode() string {
	if a.PersonNode != "" {
		return a.PersonNode
	}
	return a.HomeNode()
}

// Permits checks one record against the grants for an action: the same rules as Filter,
// decided in Go for single reads and writes. The record carries org_path and created_by, and
// actors for the scope involved.
func (a *Access) Permits(typeName, action string, rec map[string]any) bool {
	if a.IsSystem() {
		return true
	}
	orgPath, _ := rec["org_path"].(string)
	if rs := a.readers(typeName, action); len(rs) > 0 && a.InTenant(orgPath) && a.reads(rs, rec) {
		// An entry of a select list at the person's node or above (SD-Plan N5), a record one
		// of the type's readers reaches (SD16a).
		return true
	}
	if !a.InTenant(orgPath) {
		return false
	}
	createdBy := creatorOf(rec)
	if a.ActiveOrg != "" && !within(orgPath, a.ActiveOrg) {
		return false
	}
	for _, g := range a.grantsFor(typeName, action) {
		ok := false
		switch g.Scope {
		case ScopeAll:
			ok = true
		case ScopeSubtree:
			ok = within(orgPath, g.OrgPath)
		case ScopeUnit:
			ok = orgPath == g.OrgPath
		case ScopeOwn:
			ok = within(orgPath, g.OrgPath) && createdBy != nil && *createdBy == a.AccountID
		case ScopeGroup:
			if within(orgPath, g.OrgPath) && createdBy != nil {
				for _, m := range a.Mates {
					if m == *createdBy {
						ok = true
					}
				}
			}
		case ScopeInvolved:
			ok = within(orgPath, g.OrgPath) && a.holdsRole(rec)
		}
		if !ok {
			continue
		}
		if g.Condition != "" {
			c, err := rules.Compile(g.Condition)
			if err != nil {
				continue
			}
			if met, err := c.Eval(rec); err != nil || !met {
				continue
			}
		}
		return true
	}
	return false
}

// ReadableFields returns the fields of the type the access may read: all=true for every
// field, else the set. System fields are always readable.
func (a *Access) ReadableFields(typeName string) (all bool, fields map[string]bool) {
	return a.fieldRights(typeName, ActionRead, func(f *Fields) []string { return f.Read })
}

// WritableFields returns the fields the access may write with create or update.
func (a *Access) WritableFields(typeName, action string) (all bool, fields map[string]bool) {
	return a.fieldRights(typeName, action, func(f *Fields) []string { return f.Write })
}

func (a *Access) fieldRights(typeName, action string, pick func(*Fields) []string) (bool, map[string]bool) {
	if a.IsSystem() || len(a.readers(typeName, action)) > 0 {
		return true, nil
	}
	if action == ActionRead && a.Approvable[typeName] && len(a.grantsFor(typeName, action)) == 0 {
		// Asked to approve without a grant (WF6): the whole record, it reads no other.
		return true, nil
	}
	set := map[string]bool{}
	for _, g := range a.grantsFor(typeName, action) {
		if g.Fields == nil {
			return true, nil
		}
		list := pick(g.Fields)
		if list == nil {
			return true, nil
		}
		for _, f := range list {
			set[f] = true
		}
	}
	return false, set
}

// holdsRole reports whether the person or one of their groups holds a role at the record.
// The record carries actors as the API shows them: role → [{"type", "id"}].
// Involved says whether the person of the access or one of their groups holds a role at the
// record (WF3); the stream marks such events as mine (UI8).
func (a *Access) Involved(rec map[string]any) bool { return a != nil && a.holdsRole(rec) }

// HoldsAnyRole reports whether the person or one of their groups holds one of the named roles
// at the record (SD-Plan N8: a transition only a role may take).
func (a *Access) HoldsAnyRole(rec map[string]any, names []string) bool {
	if a == nil {
		return false
	}
	roles, _ := rec["actors"].(map[string]any)
	mine := map[string]bool{}
	for _, id := range a.involved() {
		mine[id.String()] = true
	}
	for _, name := range names {
		refs, _ := roles[name].([]any)
		for _, r := range refs {
			if ref, ok := r.(map[string]any); ok && mine[fmt.Sprint(ref["id"])] {
				return true
			}
		}
	}
	return false
}

func (a *Access) holdsRole(rec map[string]any, only ...string) bool {
	roles, _ := rec["actors"].(map[string]any)
	if len(roles) == 0 {
		return false
	}
	mine := map[string]bool{}
	for _, id := range a.involved() {
		mine[id.String()] = true
	}
	for role, list := range roles {
		if len(only) > 0 && !slices.Contains(only, role) {
			continue
		}
		refs, _ := list.([]any)
		for _, r := range refs {
			ref, _ := r.(map[string]any)
			if id, _ := ref["id"].(string); mine[id] {
				return true
			}
		}
	}
	return false
}

func within(path, node string) bool {
	return path == node || strings.HasPrefix(path, node+".")
}

func creatorOf(rec map[string]any) *uuid.UUID {
	switch v := rec["created_by"].(type) {
	case string:
		if u, err := uuid.Parse(v); err == nil {
			return &u
		}
	case uuid.UUID:
		return &v
	case *uuid.UUID:
		return v
	}
	return nil
}

type ctxKey struct{}

// WithAccess puts the access of the request into the context.
func WithAccess(ctx context.Context, a *Access) context.Context {
	return context.WithValue(ctx, ctxKey{}, a)
}

// From returns the access of the request; nil means the binary itself (System).
func From(ctx context.Context) *Access {
	a, _ := ctx.Value(ctxKey{}).(*Access)
	return a
}

// --- loading, cache and invalidation (PL9) ---

const (
	genKey    = "authz:gen"
	channel   = "authz.gen"
	entryTTL  = 5 * time.Second
	keyPrefix = "authz:"
)

// Service loads access and manages roles and assignments.
type Service struct {
	db     *db.Router
	groups *query.Groups
	org    *org.Service
	kv     *cache.Client
	logger *slog.Logger

	mu  sync.Mutex
	gen int64
	// local holds loaded access per account for entryTTL, within one generation.
	local map[uuid.UUID]localEntry
}

type localEntry struct {
	gen    int64
	at     time.Time
	access *Access
}

// New returns a service on the pool; kv may be nil (then nothing is cached or shared).
func New(rt *db.Router, kv *cache.Client, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: rt, groups: query.NewGroups(rt), org: org.New(rt), kv: kv, logger: logger,
		local: map[uuid.UUID]localEntry{}}
}

// cq is the query handle in the control database: what belongs to the installation (PL47).
func (s *Service) cq() *queries.Queries { return queries.New(s.db.Control()) }

// Run keeps the generation in step with the other instances: it subscribes to the channel
// and reads the counter at the start. It blocks until the context ends.
func (s *Service) Run(ctx context.Context) error {
	if s.kv == nil {
		<-ctx.Done()
		return nil
	}
	if g, err := s.kv.GetInt(ctx, genKey); err == nil {
		s.setGen(g)
	}
	for ctx.Err() == nil {
		err := s.kv.Subscribe(ctx, channel, func(message string) {
			var g int64
			if _, err := fmt.Sscanf(message, "%d", &g); err == nil {
				s.setGen(g)
			}
		})
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("authz: subscription lost, retrying", "error", err.Error())
			time.Sleep(time.Second)
		}
	}
	return nil
}

func (s *Service) setGen(g int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if g > s.gen {
		s.gen = g
		s.local = map[uuid.UUID]localEntry{}
	}
}

// Generation is the current cache generation of this instance.
func (s *Service) Generation() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen
}

// Invalidate bumps the generation everywhere: after a change of roles, permissions or
// assignments every instance loads access afresh.
func (s *Service) Invalidate(ctx context.Context) {
	if s.kv == nil {
		s.mu.Lock()
		s.gen++
		s.local = map[uuid.UUID]localEntry{}
		s.mu.Unlock()
		return
	}
	g, err := s.kv.Incr(ctx, genKey)
	if err != nil {
		s.logger.Warn("authz: invalidate", "error", err.Error())
		s.setGen(s.Generation() + 1)
		return
	}
	s.setGen(g)
	if err := s.kv.Publish(ctx, channel, fmt.Sprint(g)); err != nil {
		s.logger.Warn("authz: publish", "error", err.Error())
	}
}

// Load gathers the access of an account, from the process cache, Valkey or the database.
func (s *Service) Load(ctx context.Context, accountID uuid.UUID) (*Access, error) {
	gen := s.Generation()
	s.mu.Lock()
	e, ok := s.local[accountID]
	s.mu.Unlock()
	if ok && e.gen == gen && time.Since(e.at) < entryTTL {
		return e.access, nil
	}
	key := fmt.Sprintf("%s%d:%s", keyPrefix, gen, accountID)
	if s.kv != nil {
		if b, err := s.kv.Get(ctx, key); err == nil {
			var a Access
			if json.Unmarshal(b, &a) == nil {
				s.remember(accountID, gen, &a)
				return &a, nil
			}
		}
	}
	a, err := s.load(ctx, accountID)
	if err != nil {
		return nil, err
	}
	s.remember(accountID, gen, a)
	if s.kv != nil {
		if b, err := json.Marshal(a); err == nil {
			_ = s.kv.Set(ctx, key, b, entryTTL)
		}
	}
	return a, nil
}

func (s *Service) remember(accountID uuid.UUID, gen int64, a *Access) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gen == s.gen {
		s.local[accountID] = localEntry{gen: gen, at: time.Now(), access: a}
	}
}

func (s *Service) load(ctx context.Context, accountID uuid.UUID) (*Access, error) {
	// The account, its person and its assignments live in its home database (PL47b): the
	// tenant's, or the control database for an account of the installation, whose assignments
	// are all central (E141). The permissions of the roles are central.
	home, _, err := s.db.Account(ctx, accountID)
	if err != nil {
		return nil, err
	}
	hq := queries.New(home)
	acc, err := hq.GetAccount(ctx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &Access{AccountID: accountID}, nil
		}
		return nil, fmt.Errorf("authz: account: %w", err)
	}
	a := &Access{AccountID: accountID}
	if acc.Tenant.Valid {
		a.Home = acc.Tenant.String
	}
	tenants, err := s.cq().ListTenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("authz: tenants: %w", err)
	}
	// The tenants follow from the grants, which come below; every return passes here.
	defer func() { a.Tenants = enterable(a, tenants) }()
	// Select lists are readable by everyone who is signed in (PL16, E93), other types by
	// their readers (SD16a).
	types, err := meta.Types(ctx, s.cq())
	if err != nil {
		return nil, err
	}
	for _, t := range types {
		if t.HasApprovals() {
			if a.Approvable == nil {
				a.Approvable = map[string]bool{}
			}
			a.Approvable[t.QualifiedName()] = true
		}
		var rs []meta.Reader
		if t.IsList() {
			rs = append(rs, meta.Reader{})
		}
		rs = append(rs, t.Readers...)
		if len(rs) > 0 {
			if a.Readers == nil {
				a.Readers = map[string][]meta.Reader{}
			}
			a.Readers[t.QualifiedName()] = rs
		}
	}
	if !acc.PersonID.Valid {
		return a, nil
	}
	person := acc.PersonID.UUID
	a.PersonID = &person
	persons := query.NewGroups(db.Single(home))
	groups, err := persons.GroupsOfPerson(ctx, person)
	if err != nil {
		return nil, err
	}
	if groups == nil {
		groups = []uuid.UUID{}
	}
	a.Groups = groups
	if a.PersonNode, err = persons.NodeOfPerson(ctx, person); err != nil {
		return nil, err
	}
	if len(a.Approvable) > 0 {
		if a.Represents, err = represents(ctx, home, persons, person); err != nil {
			return nil, err
		}
	}
	rows, err := s.grants(ctx, hq, person, groups)
	if err != nil {
		return nil, err
	}
	needMates := false
	// The same grant through several assignments counts once (PL57).
	seenGrant := map[string]bool{}
	for _, r := range rows {
		key := strings.Join([]string{r.TypeName, r.Action, r.Scope, r.OrgPath, r.RoleName, r.Condition, string(r.Fields)}, "\x00")
		if seenGrant[key] {
			continue
		}
		seenGrant[key] = true
		g := Grant{TypeName: r.TypeName, Action: r.Action, Scope: Scope(r.Scope), OrgPath: r.OrgPath, Role: r.RoleName, Condition: r.Condition}
		if len(r.Fields) > 0 {
			var f Fields
			if json.Unmarshal(r.Fields, &f) == nil {
				g.Fields = &f
			}
		}
		if g.Scope == ScopeGroup {
			needMates = true
		}
		a.Grants = append(a.Grants, g)
	}
	if needMates {
		if a.Mates, err = persons.AccountsOfGroupMates(ctx, person); err != nil {
			return nil, err
		}
	}
	return a, nil
}

// grant is one permission reaching a person through one assignment.
type grant struct {
	TypeName  string
	Action    string
	Scope     string
	Condition string
	Fields    []byte
	OrgPath   string
	RoleName  string
}

// grants joins the assignments of a person and the person's groups (in the home database) with
// the permissions of their roles (in the control database), in the order of the roles' names,
// type names and actions, like the single query before PL47b.
func (s *Service) grants(ctx context.Context, hq *queries.Queries, person uuid.UUID, groups []uuid.UUID) ([]grant, error) {
	as, err := hq.AssignmentsOfHolders(ctx, queries.AssignmentsOfHoldersParams{PersonID: uuid.NullUUID{UUID: person, Valid: true}, GroupIds: groups})
	if err != nil {
		return nil, fmt.Errorf("authz: assignments: %w", err)
	}
	if len(as) == 0 {
		return nil, nil
	}
	byRole := map[uuid.UUID][]string{}
	var ids []uuid.UUID
	for _, x := range as {
		if _, ok := byRole[x.RoleID]; !ok {
			ids = append(ids, x.RoleID)
		}
		byRole[x.RoleID] = append(byRole[x.RoleID], x.OrgPath)
	}
	ps, err := s.cq().PermissionsOfRoles(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("authz: permissions: %w", err)
	}
	var out []grant
	for _, p := range ps {
		for _, path := range byRole[p.RoleID] {
			out = append(out, grant{TypeName: p.TypeName, Action: p.Action, Scope: p.Scope, Condition: p.Condition,
				Fields: p.Fields, OrgPath: path, RoleName: p.RoleName})
		}
	}
	return out, nil
}

// enterable lists the tenants an access may work in (PL44): just the root while there are
// none; the own one for an account bound to a tenant; else every tenant where a role reaches,
// or NoTenant.
func enterable(a *Access, tenants []queries.ListTenantsRow) []string {
	if len(tenants) == 0 {
		return []string{org.Root}
	}
	if a.Home != "" {
		return []string{a.Home}
	}
	var out []string
	for _, t := range tenants {
		for _, g := range a.Grants {
			if within(g.OrgPath, t.Path) || within(t.Path, g.OrgPath) {
				out = append(out, t.Path)
				break
			}
		}
	}
	if len(out) == 0 {
		return []string{NoTenant}
	}
	return out
}

// --- roles, permissions, assignments ---

// Role is a named set of permissions.
type Role struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Label       map[string]string `json:"label"`
	Description map[string]string `json:"description"`
	Permissions []Permission      `json:"permissions"`
}

// Permission is one line of a role.
type Permission struct {
	TypeName  string  `json:"type_name"`
	Action    string  `json:"action"`
	Scope     Scope   `json:"scope"`
	Condition string  `json:"condition,omitempty"`
	Fields    *Fields `json:"fields,omitempty"`
}

// Assignment gives a role to a person or a group at a node.
type Assignment struct {
	ID       uuid.UUID  `json:"id"`
	Role     string     `json:"role"`
	PersonID *uuid.UUID `json:"person_id,omitempty"`
	GroupID  *uuid.UUID `json:"group_id,omitempty"`
	OrgPath  string     `json:"org_path"`
}

// Roles lists every role with its permissions.
func (s *Service) Roles(ctx context.Context) ([]Role, error) {
	rows, err := s.cq().ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("authz: roles: %w", err)
	}
	out := make([]Role, 0, len(rows))
	for _, r := range rows {
		role, err := s.role(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, nil
}

// Role returns one role by name.
func (s *Service) Role(ctx context.Context, name string) (Role, error) {
	r, err := s.cq().GetRoleByName(ctx, name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Role{}, ErrNotFound
		}
		return Role{}, fmt.Errorf("authz: role: %w", err)
	}
	return s.role(ctx, r)
}

func (s *Service) role(ctx context.Context, r queries.Role) (Role, error) {
	role := Role{ID: r.ID, Name: r.Name, Label: map[string]string{}, Description: map[string]string{}, Permissions: []Permission{}}
	_ = json.Unmarshal(r.Label, &role.Label)
	_ = json.Unmarshal(r.Description, &role.Description)
	ps, err := s.cq().ListPermissions(ctx, r.ID)
	if err != nil {
		return Role{}, fmt.Errorf("authz: permissions: %w", err)
	}
	for _, p := range ps {
		perm := Permission{TypeName: p.TypeName, Action: p.Action, Scope: Scope(p.Scope), Condition: p.Condition}
		if len(p.Fields) > 0 {
			var f Fields
			if json.Unmarshal(p.Fields, &f) == nil {
				perm.Fields = &f
			}
		}
		role.Permissions = append(role.Permissions, perm)
	}
	return role, nil
}

// CreateRole adds a role without permissions.
func (s *Service) CreateRole(ctx context.Context, name string, label, description map[string]string) (Role, error) {
	if !nameRe.MatchString(name) {
		return Role{}, fmt.Errorf("%w: role name %q must be snake_case", ErrInvalid, name)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Role{}, err
	}
	if label == nil {
		label = map[string]string{}
	}
	if description == nil {
		description = map[string]string{}
	}
	lb, _ := json.Marshal(label)
	db, _ := json.Marshal(description)
	r, err := s.cq().InsertRole(ctx, queries.InsertRoleParams{ID: id, Name: name, Label: lb, Description: db})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Role{}, fmt.Errorf("%w: %s", ErrExists, name)
		}
		return Role{}, fmt.Errorf("authz: create role: %w", err)
	}
	return s.role(ctx, r)
}

// DeleteRole removes a role with its permissions and assignments; "admin" stays.
func (s *Service) DeleteRole(ctx context.Context, name string) error {
	r, err := s.cq().GetRoleByName(ctx, name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("authz: role: %w", err)
	}
	if r.Name == "admin" {
		return fmt.Errorf("%w: the role admin is built in", ErrInvalid)
	}
	// A role in use is not deleted (PL47b): its assignments lie in every database, and no
	// transaction spans them. The caller removes the assignments first.
	for _, pool := range s.db.Pools() {
		n, err := queries.New(pool).CountAssignmentsOfRole(ctx, r.ID)
		if err != nil {
			return fmt.Errorf("authz: assignments of role: %w", err)
		}
		if n > 0 {
			return fmt.Errorf("%w: %s has %d assignments; remove them first", ErrInUse, name, n)
		}
	}
	n, err := s.cq().DeleteRole(ctx, r.ID)
	if err != nil {
		return fmt.Errorf("authz: delete role: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: the role admin is built in", ErrInvalid)
	}
	s.Invalidate(ctx)
	return nil
}

// SetPermission adds or replaces one line of a role: a line is a type, an action and a scope,
// and a role may hold several scopes for one type and action, which count with OR (UI4). The
// condition must translate to SQL, the field lists must be snake_case names.
func (s *Service) SetPermission(ctx context.Context, roleName string, p Permission) error {
	if p.TypeName != Any && !strings.Contains(p.TypeName, ".") {
		return fmt.Errorf("%w: type %q must be package.type or *", ErrInvalid, p.TypeName)
	}
	if p.Action != Any && !nameRe.MatchString(p.Action) {
		return fmt.Errorf("%w: action %q must be snake_case or *", ErrInvalid, p.Action)
	}
	if !scopes[p.Scope] {
		return fmt.Errorf("%w: scope %q must be own, group, involved, unit, subtree or all", ErrInvalid, p.Scope)
	}
	if _, _, err := Translate(p.Condition, nil); err != nil {
		return err
	}
	var fields []byte
	if p.Fields != nil {
		for _, f := range append(append([]string{}, p.Fields.Read...), p.Fields.Write...) {
			if !nameRe.MatchString(f) {
				return fmt.Errorf("%w: field %q must be snake_case", ErrInvalid, f)
			}
		}
		fields, _ = json.Marshal(p.Fields)
	}
	r, err := s.cq().GetRoleByName(ctx, roleName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("authz: role: %w", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	_, err = s.cq().UpsertPermission(ctx, queries.UpsertPermissionParams{
		ID: id, RoleID: r.ID, TypeName: p.TypeName, Action: p.Action, Scope: string(p.Scope), Condition: p.Condition, Fields: fields,
	})
	if err != nil {
		return fmt.Errorf("authz: set permission: %w", err)
	}
	s.Invalidate(ctx)
	return nil
}

// UnsetPermission removes the line of a role with that scope, or with scope "" every line of
// the type and action.
func (s *Service) UnsetPermission(ctx context.Context, roleName, typeName, action string, scope Scope) error {
	r, err := s.cq().GetRoleByName(ctx, roleName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("authz: role: %w", err)
	}
	p := queries.DeletePermissionParams{RoleID: r.ID, TypeName: typeName, Action: action}
	if scope != "" {
		p.Scope = pgtype.Text{String: string(scope), Valid: true}
	}
	n, err := s.cq().DeletePermission(ctx, p)
	if err != nil {
		return fmt.Errorf("authz: unset permission: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	s.Invalidate(ctx)
	return nil
}

// Assign gives a role to a person or a group (exactly one of them) at a node.
func (s *Service) Assign(ctx context.Context, roleName string, personID, groupID *uuid.UUID, orgPath string, by *uuid.UUID) (Assignment, error) {
	if (personID == nil) == (groupID == nil) {
		return Assignment{}, fmt.Errorf("%w: an assignment needs a person or a group, not both", ErrInvalid)
	}
	if orgPath == "" {
		orgPath = org.Root
	}
	if ok, err := s.org.Exists(ctx, orgPath); err != nil {
		return Assignment{}, err
	} else if !ok {
		return Assignment{}, fmt.Errorf("%w: node %q", ErrNotFound, orgPath)
	}
	r, err := s.cq().GetRoleByName(ctx, roleName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Assignment{}, fmt.Errorf("%w: role %q", ErrNotFound, roleName)
		}
		return Assignment{}, fmt.Errorf("authz: role: %w", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Assignment{}, err
	}
	target, err := s.assignDB(ctx, personID)
	if err != nil {
		return Assignment{}, err
	}
	row, err := queries.New(target).InsertAssignment(ctx, queries.InsertAssignmentParams{
		ID: id, RoleID: r.ID, PersonID: nullUUID(personID), GroupID: nullUUID(groupID), OrgPath: orgPath, CreatedBy: nullUUID(by),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// The same role for the same holder at the same node once (PL57).
			return Assignment{}, fmt.Errorf("%w: the role %s is assigned there already", ErrExists, roleName)
		}
		return Assignment{}, fmt.Errorf("authz: assign: %w", err)
	}
	s.Invalidate(ctx)
	return Assignment{ID: row.ID, Role: roleName, PersonID: personID, GroupID: groupID, OrgPath: row.OrgPath}, nil
}

// Unassign removes an assignment.
// assignDB is where an assignment lives (PL47b): with the person of an account of the
// installation in the control database, else in the tenant's.
func (s *Service) assignDB(ctx context.Context, personID *uuid.UUID) (*pgxpool.Pool, error) {
	if !s.db.Separate() || personID == nil {
		return s.db.Tenant(ctx), nil
	}
	accs, err := s.cq().AccountsOfPerson(ctx, uuid.NullUUID{UUID: *personID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("authz: account of person: %w", err)
	}
	if len(accs) > 0 {
		return s.db.Control(), nil
	}
	return s.db.Tenant(ctx), nil
}

// InstallationPerson reports whether a person is the person of an account of the installation
// (PL48): such a person lives at the root, outside every tenant, and its assignments lie
// centrally (PL47b, E142).
func (s *Service) InstallationPerson(ctx context.Context, personID uuid.UUID) (bool, error) {
	accs, err := s.cq().AccountsOfPerson(ctx, uuid.NullUUID{UUID: personID, Valid: true})
	if err != nil {
		return false, fmt.Errorf("authz: account of person: %w", err)
	}
	for _, a := range accs {
		if !a.Tenant.Valid {
			return true, nil
		}
	}
	return false, nil
}

// assignmentPools are the databases with assignments the session may see: the tenant's, and
// with a database per tenant the central ones of the accounts of the installation.
func (s *Service) assignmentPools(ctx context.Context) []*pgxpool.Pool {
	pools := []*pgxpool.Pool{s.db.Tenant(ctx)}
	if s.db.Separate() && s.db.Tenant(ctx) != s.db.Control() {
		pools = append(pools, s.db.Control())
	}
	return pools
}

func (s *Service) Unassign(ctx context.Context, id uuid.UUID) error {
	var n int64
	for _, pool := range s.assignmentPools(ctx) {
		m, err := queries.New(pool).DeleteAssignment(ctx, id)
		if err != nil {
			return fmt.Errorf("authz: unassign: %w", err)
		}
		n += m
	}
	if n == 0 {
		return ErrNotFound
	}
	s.Invalidate(ctx)
	return nil
}

// Assignments lists every assignment.
func (s *Service) Assignments(ctx context.Context) ([]Assignment, error) {
	roles, err := s.cq().ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("authz: roles: %w", err)
	}
	names := make(map[uuid.UUID]string, len(roles))
	for _, r := range roles {
		names[r.ID] = r.Name
	}
	var rows []queries.ListAssignmentsRow
	for _, pool := range s.assignmentPools(ctx) {
		rs, err := queries.New(pool).ListAssignments(ctx)
		if err != nil {
			return nil, fmt.Errorf("authz: assignments: %w", err)
		}
		rows = append(rows, rs...)
	}
	out := make([]Assignment, 0, len(rows))
	for _, r := range rows {
		a := Assignment{ID: r.ID, Role: names[r.RoleID], OrgPath: r.OrgPath}
		if r.PersonID.Valid {
			p := r.PersonID.UUID
			a.PersonID = &p
		}
		if r.GroupID.Valid {
			g := r.GroupID.UUID
			a.GroupID = &g
		}
		out = append(out, a)
	}
	// By role, then node, as before PL47b.
	slices.SortStableFunc(out, func(a, b Assignment) int {
		if c := strings.Compare(a.Role, b.Role); c != 0 {
			return c
		}
		return strings.Compare(a.OrgPath, b.OrgPath)
	})
	return out, nil
}

// LinkPerson ties an account to a person; nil removes the link.
func (s *Service) LinkPerson(ctx context.Context, accountID uuid.UUID, personID *uuid.UUID) error {
	home, _, err := s.db.Account(ctx, accountID)
	if err != nil {
		return err
	}
	if err := queries.New(home).SetAccountPerson(ctx, queries.SetAccountPersonParams{ID: accountID, PersonID: nullUUID(personID)}); err != nil {
		return fmt.Errorf("authz: link person: %w", err)
	}
	s.Invalidate(ctx)
	return nil
}

func nullUUID(p *uuid.UUID) uuid.NullUUID {
	if p == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *p, Valid: true}
}
