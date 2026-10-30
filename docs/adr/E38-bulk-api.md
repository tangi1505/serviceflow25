# E38: Bulk-API synchron bis 1.000, sonst asynchron

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E38
- **Architektur:** [`architektur.md`](../architektur.md) §6.4

## Kontext

Kleine Mengen sollen sofort ein Ergebnis bekommen, große ohne Zeitüberschreitung durchlaufen.

## Entscheidung

**Bulk-API:** synchron bis 1.000 Datensätze mit Ergebnis je Zeile, darüber ein NDJSON-Strom in einen asynchronen Job; Abgleich über Quelle und externen Schlüssel, idempotent; Vollstand einer Quelle markiert Fehlendes als verschwunden

## Verworfen

nur asynchron; nur synchron

## Folgen

Abgleich über Quelle und Schlüssel, nur Änderungen schreiben, Vollstand je Quelle, idempotent.
