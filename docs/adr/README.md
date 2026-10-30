# Architekturentscheidungen

Je Architekturentscheidung aus dem [Hauptplan](../0_maintask.md) §1 eine Datei (E44). Erzeugt aus
§1; ändert sich dort eine Entscheidung, wird die Datei neu erzeugt und die alte Fassung bleibt im
Hauptplan sichtbar („geändert am …; vorher: …").

| Nr | Entscheidung | Seit |
| --- | --- | --- |
| [E4](E04-backend-go.md) | Backend in Go | 2026-10-20 |
| [E5](E05-frontend-react.md) | Frontend in React und TypeScript | 2026-10-20 |
| [E6](E06-datenbank-postgresql.md) | PostgreSQL als einzige Datenbank | 2026-10-20 |
| [E7](E07-datengetrieben-plus-sandbox.md) | Datengetriebene Plattform plus Sandbox | 2026-10-20 |
| [E8](E08-workflow-provider.md) | Eigene schlanke Engine hinter einem Workflow-Provider | 2026-10-20 |
| [E9](E09-auslieferung-helm-compose.md) | Auslieferung per Helm und Docker Compose | 2026-10-20 |
| [E10](E10-anmeldung-oidc-ldap-lokal.md) | Anmeldung über OIDC/SAML, LDAP-Sync und lokale Konten | 2026-10-20 |
| [E11](E11-api-rest-openapi.md) | Eine REST-API mit OpenAPI, Webhooks und Ereignisstrom | 2026-10-20 |
| [E15](E15-lizenz-agpl-open-core.md) | AGPL mit Open Core | 2026-10-20 |
| [E16](E16-instanz-je-kunde.md) | Eine Installation je Kunde, Organisationsbaum innen | 2026-10-20 |
| [E17](E17-sandbox-wasm.md) | WASM-Sandbox per wazero | 2026-10-20 |
| [E18](E18-go-schlank.md) | Go schlank: chi, pgx, sqlc, goose | 2026-10-20 |
| [E19](E19-jobs-river.md) | Hintergrundjobs mit River in PostgreSQL | 2026-10-20 |
| [E20](E20-valkey-pflicht.md) | Valkey von Anfang an Pflicht | 2026-10-20 |
| [E21](E21-suche-postgresql.md) | Volltextsuche in PostgreSQL hinter einer Schnittstelle | 2026-10-20 |
| [E22](E22-dateien-s3-dateisystem.md) | Dateien in S3 oder im Dateisystem | 2026-10-20 |
| [E24](E24-kein-glpi-agent-bulk.md) | Kein GLPI Agent; Daten über Discovery oder Middleware per Bulk | 2026-10-20 |
| [E25](E25-schlanke-pakete.md) | Schlankes Paket je Practice, erweiterbar | 2026-10-20 |
| [E28](E28-logs-ins-monitoring.md) | Technische Logs nur an das Monitoring | 2026-10-20 |
| [E29](E29-konflikte-statt-sperren.md) | Konflikterkennung statt Sperren | 2026-10-20 |
| [E31](E31-leistungsziele.md) | Leistungsziele LZ1 bis LZ12 als Muss | 2026-10-20 |
| [E32](E32-leistung-als-gate.md) | Leistung ab dem ersten Code als Gate | 2026-10-20 |
| [E33](E33-tabelle-je-typ.md) | Eine echte Tabelle je Entitätstyp | 2026-10-20 |
| [E34](E34-rechte-als-sql.md) | Rechte als SQL-Bedingung aus Go | 2026-10-20 |
| [E35](E35-modularer-monolith.md) | Modularer Monolith mit den Rollen api und worker | 2026-10-20 |
| [E36](E36-ids-uuidv7.md) | IDs als UUIDv7 | 2026-10-20 |
| [E37](E37-drei-schichten.md) | Definitionen in drei Schichten | 2026-10-20 |
| [E38](E38-bulk-api.md) | Bulk-API synchron bis 1.000, sonst asynchron | 2026-10-20 |
| [E39](E39-ereignisse-outbox.md) | Ereignisse über Outbox, Webhooks, Strom und SSE | 2026-10-20 |
| [E40](E40-filter-json-abfrage.md) | Filtern über Parameter und JSON-Abfrage | 2026-10-20 |
| [E41](E41-frontend-bibliotheken.md) | Frontend-Bibliotheken | 2026-10-20 |
| [E42](E42-sprache-und-bitv.md) | Deutsch und Englisch, barrierefrei nach BITV | 2026-10-20 |
| [E43](E43-sql-builder.md) | Eigener SQL-Builder auf pgx | 2026-10-20 |
| [E45](E45-ausdruecke-cel.md) | Ausdruckssprache CEL | 2026-10-20 |
| [E46](E46-ereignisse-7-tage.md) | Ereignisse 7 Tage nachlesbar | 2026-10-20 |
| [E47](E47-partitionen-je-typ.md) | Partitionen je Typ nach Erstellungsmonat | 2026-10-20 |
