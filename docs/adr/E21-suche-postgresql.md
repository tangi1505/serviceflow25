# E21: Volltextsuche in PostgreSQL hinter einer Schnittstelle

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E21
- **Architektur:** [`architektur.md`](../architektur.md) §3, §11

## Kontext

Suche über Tickets, Assets und Wissen ohne weiteren Dienst und ohne doppelte Rechteprüfung.

## Entscheidung

Suche mit **PostgreSQL-Volltext** (tsvector, deutsches Wörterbuch, Trigram), hinter einer Schnittstelle, damit Meilisearch oder OpenSearch nachrüstbar bleiben

## Verworfen

Meilisearch; OpenSearch

## Folgen

Eine erzeugte `tsvector`-Spalte je Typtabelle mit GIN-Index. Meilisearch oder OpenSearch bleiben über die Schnittstelle nachrüstbar.
