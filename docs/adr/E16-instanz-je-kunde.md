# E16: Eine Installation je Kunde, Organisationsbaum innen

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E16
- **Architektur:** [`architektur.md`](../architektur.md) §3, §5

## Kontext

VS-NfD-Kunden teilen keine Installation; innerhalb eines Kunden gibt es Behörden und Dienststellen mit eigener Sicht.

## Entscheidung

**Eine Installation je Kunde** mit eigener Datenbank; **innerhalb** ein Baum aus Organisationseinheiten mit Sichtbarkeit und Rechtevererbung (wie GLPIs Entitäten, sauberer)

## Verworfen

eine Instanz für viele Kunden; Mandantenspalte „auf Vorrat"

## Folgen

Keine Mandantenspalte. Jeder Datensatz hat einen Ort im Organisationsbaum (`org_path`), über den Sichtbarkeit und Rechte vererbt werden.
