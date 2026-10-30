# E43: Eigener SQL-Builder auf pgx

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E43
- **Architektur:** [`architektur.md`](../architektur.md) §2, §3

## Kontext

Typtabellen sind zur Bauzeit unbekannt; `sqlc` kann sie nicht erzeugen.

## Entscheidung

**Eigener kleiner SQL-Builder auf `pgx`** für die Typtabellen; `sqlc` bleibt für die Kerntabellen

## Verworfen

`goqu`; `squirrel`

## Folgen

Ein kleiner Builder für Auswahl, Filter, Rechte, Sortierung, Seiten und `COPY`. Kein Fremdcode zwischen uns und PostgreSQL.
