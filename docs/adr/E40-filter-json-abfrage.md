# E40: Filtern über Parameter und JSON-Abfrage

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E40
- **Architektur:** [`architektur.md`](../architektur.md) §6.3

## Kontext

Einfache Abfragen sollen einfach sein, komplexe über Beziehungen und Aggregate möglich.

## Entscheidung

**Filtern** über einfache Parameter, Komplexes als JSON-Abfrage per POST; dieselbe Sprache für gespeicherte Suchen, Dashboards und Rechtebedingungen

## Verworfen

Filter-Ausdruck wie OData; nur Parameter

## Folgen

Eine Abfragesprache für API, gespeicherte Suchen, Dashboards und Rechtebedingungen.
