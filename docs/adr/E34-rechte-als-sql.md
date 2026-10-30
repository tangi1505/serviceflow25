# E34: Rechte als SQL-Bedingung aus Go

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E34
- **Architektur:** [`architektur.md`](../architektur.md) §5

## Kontext

Rechte nach Organisationsbaum, Rolle, Datensatz und Feld müssen bei jeder Abfrage und jedem Bulk gelten.

## Entscheidung

**Rechte als SQL-Bedingung,** in Go aus Rollen, Reichweite, Organisationsbaum und Feldrechten erzeugt und gecacht

## Verworfen

Row-Level Security in PostgreSQL; beides

## Folgen

Keine Row-Level Security. Ein Linter verbietet SQL auf Typtabellen außerhalb von `query` und `records`.
