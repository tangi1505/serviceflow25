# E6: PostgreSQL als einzige Datenbank

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E6
- **Architektur:** [`architektur.md`](../architektur.md) §3

## Kontext

Freie Felder, Volltext, große Tabellen und Jobqueue sollen ohne weitere Datenbank auskommen.

## Entscheidung

**PostgreSQL** als einzige Datenbank; ihre Stärken (JSONB, Volltext, Partitionierung, Row-Level Security) werden genutzt

## Verworfen

MariaDB; Abstraktion über mehrere Datenbanken

## Folgen

JSONB, `ltree`, `tsvector`, Partitionen und `COPY` stehen zur Verfügung und werden genutzt. Kein Betrieb mit MariaDB; ein Kunde braucht PostgreSQL.
