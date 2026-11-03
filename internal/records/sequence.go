// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

// --- running numbers (PL49, Kern-Plan E63) ---
//
// A sequence field gets its number from a PostgreSQL sequence of its own per type, field and
// tenant, in the transaction of the create: fast and without a lock, with a gap after a
// transaction that does not commit. A sequence is created on its first use, once, under an
// advisory lock; its name is a hash, so that it fits the 63 bytes of a name. Only an import
// with a source may bring a number; the sequence then goes on above it.

// sequenceName is the name of the sequence of a field in a tenant.
func sequenceName(t meta.Type, field, tenant string) string {
	h := sha256.Sum256([]byte(schema.TableName(t) + "." + field + "@" + tenant))
	return "seq_" + hex.EncodeToString(h[:12])
}

// numberFields are the sequence fields of a type.
func numberFields(t meta.Type) []meta.Field {
	var out []meta.Field
	for _, f := range t.Fields {
		if f.Type == meta.FieldSequence {
			out = append(out, f)
		}
	}
	return out
}

// tenantOf is the tenant of a node in the database of the request.
func (s *Service) tenantOf(ctx context.Context, q interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, node string) (string, error) {
	var tenant string
	if err := q.QueryRow(ctx, "SELECT tenant_of($1::ltree)::text", node).Scan(&tenant); err != nil {
		return "", fmt.Errorf("records: tenant of %s: %w", node, err)
	}
	return tenant, nil
}

// sequences are the sequences of a type's number fields in the tenant of a node, made sure to
// exist, by field.
func (s *Service) sequences(ctx context.Context, t meta.Type, node string) (map[string]string, error) {
	fields := numberFields(t)
	if len(fields) == 0 {
		return nil, nil
	}
	pool := s.db.Tenant(ctx)
	tenant, err := s.tenantOf(ctx, pool, node)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		name, err := s.ensureSequence(ctx, pool, t, f.Name, tenant)
		if err != nil {
			return nil, err
		}
		out[f.Name] = name
	}
	return out, nil
}

// ensureSequence makes sure the sequence of a field in a tenant exists and returns its name;
// two processes that start it at once wait for each other on the lock. A new sequence starts
// above the highest number the tenant has already: records a tenant adopted from the root
// (--adopt) or tables brought back without their sequences keep theirs.
func (s *Service) ensureSequence(ctx context.Context, pool *pgxpool.Pool, t meta.Type, field, tenant string) (string, error) {
	name := sequenceName(t, field, tenant)
	key := fmt.Sprintf("%p/%s", pool, name)
	if _, ok := s.seqs.Load(key); ok {
		return name, nil
	}
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", name); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", name).Scan(&exists); err != nil || exists {
			return err
		}
		if _, err := tx.Exec(ctx, "CREATE SEQUENCE "+pgx.Identifier{name}.Sanitize()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, fmt.Sprintf("SELECT setval($1::regclass, m) FROM (SELECT max(%s) AS m FROM %s WHERE tenant = $2::ltree) x WHERE m IS NOT NULL",
			pgx.Identifier{field}.Sanitize(), table(t)), name, tenant)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("records: sequence %s: %w", name, err)
	}
	s.seqs.Store(key, true)
	return name, nil
}

// nextNumbers draws n numbers from a sequence in the transaction of the write.
func (s *Service) nextNumbers(ctx context.Context, tx pgx.Tx, seq string, n int) ([]int64, error) {
	rows, err := tx.Query(ctx, "SELECT nextval($1::regclass) FROM generate_series(1, $2)", seq, n)
	if err == nil {
		var out []int64
		if out, err = pgx.CollectRows(rows, pgx.RowTo[int64]); err == nil {
			return out, nil
		}
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		// The sequence went away (a database rebuilt under a running server): the next write
		// makes it anew.
		s.seqs.Range(func(k, _ any) bool {
			if strings.HasSuffix(fmt.Sprint(k), "/"+seq) {
				s.seqs.Delete(k)
			}
			return true
		})
	}
	return nil, fmt.Errorf("records: number from %s: %w", seq, err)
}

// raiseSequence makes a sequence go on above an imported number.
func raiseSequence(ctx context.Context, tx pgx.Tx, seq string, n int64) error {
	_, err := tx.Exec(ctx, fmt.Sprintf(
		"SELECT setval($1::regclass, greatest($2::bigint, CASE WHEN is_called THEN last_value ELSE 0 END)) FROM %s",
		pgx.Identifier{seq}.Sanitize()), seq, n)
	if err != nil {
		return fmt.Errorf("records: raise %s: %w", seq, err)
	}
	return nil
}

// takeNumbers takes the numbers out of an input. An import with a source may bring them
// (Kern-Plan E63), whole numbers from 1, also as text with the prefix ("INC-24817"); every
// other write refuses them as calculated in validate.
func takeNumbers(t meta.Type, input map[string]any) (map[string]any, map[string]int64, []Problem) {
	fields := numberFields(t)
	if len(fields) == 0 {
		return input, nil, nil
	}
	rest := make(map[string]any, len(input))
	for k, v := range input {
		rest[k] = v
	}
	var given map[string]int64
	var ps []Problem
	for _, f := range fields {
		v, ok := rest[f.Name]
		if !ok {
			continue
		}
		delete(rest, f.Name)
		if v == nil {
			continue
		}
		n, ok := parseNumber(f, v)
		if !ok {
			ps = append(ps, Problem{Field: f.Name, Code: "type", Message: "must be a whole number from 1, with or without the prefix " + strconv.Quote(f.Prefix)})
			continue
		}
		if given == nil {
			given = map[string]int64{}
		}
		given[f.Name] = n
	}
	return rest, given, ps
}

// parseNumber reads a running number: a whole number from 1, or text with or without the
// prefix of the field.
func parseNumber(f meta.Field, v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		if x >= 1 && x == math.Trunc(x) && x <= math.MaxInt64 {
			return int64(x), true
		}
	case int64:
		return x, x >= 1
	case int:
		return int64(x), x >= 1
	case json.Number:
		n, err := x.Int64()
		return n, err == nil && n >= 1
	case string:
		s := strings.TrimSpace(x)
		if f.Prefix != "" && len(s) > len(f.Prefix) && strings.EqualFold(s[:len(f.Prefix)], f.Prefix) {
			s = s[len(f.Prefix):]
		}
		n, err := strconv.ParseInt(s, 10, 64)
		return n, err == nil && n >= 1
	}
	return 0, false
}

// quoteText is a text literal for SQL.
func quoteText(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// numberBatch gives the new rows of a bulk batch their running numbers, in the order of the
// rows and in the transaction of the batch; imported numbers move their sequence above them.
func (s *Service) numberBatch(ctx context.Context, tx pgx.Tx, t meta.Type, todo []pending) error {
	fields := numberFields(t)
	if len(fields) == 0 {
		return nil
	}
	var paths []string
	seen := map[string]bool{}
	for _, p := range todo {
		if p.op == "c" && !seen[p.path] {
			seen[p.path] = true
			paths = append(paths, p.path)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, "SELECT p, tenant_of(p::ltree)::text FROM unnest($1::text[]) p", paths)
	if err != nil {
		return fmt.Errorf("records: tenants of a batch: %w", err)
	}
	tenants := map[string]string{}
	for rows.Next() {
		var p, tenant string
		if err := rows.Scan(&p, &tenant); err != nil {
			rows.Close()
			return err
		}
		tenants[p] = tenant
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	pool := s.db.Tenant(ctx)
	for _, f := range fields {
		missing := map[string][]int{} // sequence: index in todo
		highest := map[string]int64{}
		for i, p := range todo {
			if p.op != "c" {
				continue
			}
			seq, err := s.ensureSequence(ctx, pool, t, f.Name, tenants[p.path])
			if err != nil {
				return err
			}
			if n, ok := p.values[f.Name].(int64); ok {
				highest[seq] = max(highest[seq], n)
				continue
			}
			missing[seq] = append(missing[seq], i)
		}
		for seq, n := range highest {
			if err := raiseSequence(ctx, tx, seq, n); err != nil {
				return err
			}
		}
		for seq, idx := range missing {
			ns, err := s.nextNumbers(ctx, tx, seq, len(idx))
			if err != nil {
				return err
			}
			for k, i := range idx {
				todo[i].values[f.Name] = ns[k]
			}
		}
	}
	return nil
}

// numberBatchSize is the number of records numbered in one transaction.
const numberBatchSize = 500

// Number gives the records of a type that have no running number yet one, the trash
// included, in the order of their creation and per tenant (PL49): for a type that got its
// sequence field after records were there. Like a recomputation it makes no version and no
// audit row; the caller recomputes the search column after. It returns how many it numbered.
func (s *Service) Number(ctx context.Context, typeName string) (int, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, f := range numberFields(t) {
		col := pgx.Identifier{f.Name}.Sanitize()
		for {
			n := 0
			err := s.tx(ctx, func(tx pgx.Tx) error {
				sql := fmt.Sprintf("SELECT id, tenant::text FROM %s WHERE %s IS NULL", table(t), col)
				if c := listScope(t, ""); c != "" {
					sql += " AND " + c
				}
				rows, err := tx.Query(ctx, sql+fmt.Sprintf(" ORDER BY created_at, id LIMIT %d FOR UPDATE", numberBatchSize))
				if err != nil {
					return err
				}
				type row struct {
					id     [16]byte
					tenant string
				}
				var list []row
				for rows.Next() {
					var r row
					if err := rows.Scan(&r.id, &r.tenant); err != nil {
						rows.Close()
						return err
					}
					list = append(list, r)
				}
				rows.Close()
				if err := rows.Err(); err != nil {
					return err
				}
				for _, r := range list {
					seq, err := s.ensureSequence(ctx, s.db.Tenant(ctx), t, f.Name, r.tenant)
					if err != nil {
						return err
					}
					ns, err := s.nextNumbers(ctx, tx, seq, 1)
					if err != nil {
						return err
					}
					if _, err := tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET %s = $2 WHERE id = $1", table(t), col), r.id, ns[0]); err != nil {
						return fmt.Errorf("records: number %s: %w", typeName, err)
					}
				}
				n = len(list)
				return nil
			})
			if err != nil {
				return total, err
			}
			total += n
			if n < numberBatchSize {
				break
			}
		}
	}
	return total, nil
}
