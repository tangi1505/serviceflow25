# E20: Valkey von Anfang an Pflicht

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E20
- **Architektur:** [`architektur.md`](../architektur.md) §1, §5, §7

## Kontext

Mehrere `api`-Instanzen brauchen gemeinsame Sitzungen, Ratenbegrenzung, Cache und Pub/Sub.

## Entscheidung

**Valkey von Anfang an Pflicht** für Cache, Sitzungen, Ratenbegrenzung und Pub/Sub zwischen den API-Instanzen; auch in der Entwicklung. Valkey statt Redis wegen der Lizenz (BSD)

## Verworfen

Valkey erst im Cluster; kein Valkey

## Folgen

`api` ist zustandslos (LZ12). Valkey läuft auch in der Entwicklung mit; Lizenz BSD statt Redis.
