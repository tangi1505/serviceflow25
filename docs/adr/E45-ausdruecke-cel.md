# E45: Ausdruckssprache CEL

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E45
- **Architektur:** [`architektur.md`](../architektur.md) §5, §8, §9

## Kontext

Bedingungen, Regeln, Formeln und Rechtebedingungen brauchen eine sichere, schnelle Sprache für Admins.

## Entscheidung

**Ausdruckssprache CEL** (`cel-go`) für Bedingungen, Regeln, Formeln, Identifikationsregeln und Rechtebedingungen; berechnete Felder werden beim Schreiben ausgewertet und gespeichert; schwere Logik in WASM

## Verworfen

`expr`; eigene Sprache; nur WASM

## Folgen

`cel-go`, typgeprüft beim Speichern. Berechnete Felder werden beim Schreiben gespeichert, damit Listen schnell bleiben. Schwere Logik bleibt in WASM.
