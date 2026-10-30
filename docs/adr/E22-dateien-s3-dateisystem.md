# E22: Dateien in S3 oder im Dateisystem

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E22
- **Architektur:** [`architektur.md`](../architektur.md) §1, §6.4

## Kontext

Anhänge und Bulk-Ströme müssen im Cluster für alle Instanzen erreichbar sein.

## Entscheidung

Dateien über die **S3-API** im Cluster, **Dateisystem** in Compose und Entwicklung, beides hinter einer Schnittstelle; Metadaten in PostgreSQL. Welcher S3-Speicher, ist offen (V23). **Geändert am 2026-10-20;** vorher: „S3-kompatibel (MinIO)". Grund: MinIO ist archiviert und bekommt keine Sicherheitsupdates mehr (§6.1)

## Verworfen

nur Dateisystem; in der Datenbank

## Folgen

Im Cluster ein S3-Speicher, welcher, ist offen (Hauptplan V23, MinIO ist archiviert); das Dateisystem in Compose. Metadaten in PostgreSQL.
