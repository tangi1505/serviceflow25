# E33: Eine echte Tabelle je Entitätstyp

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E33
- **Architektur:** [`architektur.md`](../architektur.md) §3

## Kontext

Frei definierte Typen mit Millionen Zeilen müssen schnell filterbar und massenhaft schreibbar sein.

## Entscheidung

**Eine echte Tabelle je Entitätstyp,** vom Kern angelegt und online geändert; echte Spalten, Indexe, Partitionen, `COPY`

## Verworfen

eine Tabelle mit JSONB für alles; Mischform

## Folgen

Der Kern ändert das Schema zur Laufzeit (§3.2). Für diese Tabellen gibt es kein `sqlc`, sondern den eigenen Builder (E43).
