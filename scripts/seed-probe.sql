-- Copyright (C) 2026 group24 AG
-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Test data for the Durchstich (UI2 Prüfweg): 100,000 records of the type probe.probe, written
-- straight into the table (no audit rows, no events). Usage in WSL, after `package install`:
--
--   docker compose exec -T postgres psql -U itsm -d itsm -v n=100000 < scripts/seed-probe.sql
--
-- Running it again adds another batch; titles carry the batch time so that they stay distinct.

INSERT INTO rec_probe_probe (id, title, priority, active, due_on, seen_at, kind, created_at, updated_at)
SELECT
    uuidv7(),
    format('Probe %s-%s', to_char(now(), 'HH24MISS'), lpad(i::text, 6, '0')),
    1 + (i % 5),
    (i % 7) <> 0,
    CASE WHEN i % 3 = 0 THEN current_date + (i % 60) ELSE NULL END,
    CASE WHEN i % 2 = 0 THEN now() - make_interval(mins => i % 10000) ELSE NULL END,
    (ARRAY['question', 'fault', 'wish'])[1 + i % 3],
    now() - make_interval(secs => (:n - i)),
    now() - make_interval(secs => (:n - i))
FROM generate_series(1, :n) AS s(i);

SELECT count(*) AS probe_records FROM rec_probe_probe;
