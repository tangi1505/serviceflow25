# E17: WASM-Sandbox per wazero

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E17
- **Architektur:** [`architektur.md`](../architektur.md) §9

## Kontext

Kundenlogik, die über Ausdrücke hinausgeht, muss sicher im Prozess laufen, ohne C-Abhängigkeit.

## Entscheidung

Sandbox für Kundenlogik: **WASM**, im Go-Host per `wazero` (reines Go, keine C-Abhängigkeit), mit Speicher- und Zeitlimit; Module in jeder Sprache, die nach WASM kompiliert

## Verworfen

eingebettetes JavaScript (goja); beides; nur deklarative Regeln

## Folgen

Module in jeder Sprache, die nach WASM kompiliert; harte Grenzen für Speicher und Zeit (V21). Einfache Logik läuft in CEL (E45), nicht in WASM.
