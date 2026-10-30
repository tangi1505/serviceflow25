# E15: AGPL mit Open Core

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E15
- **Architektur:** [`architektur.md`](../architektur.md) §12

## Kontext

Das Werkzeug ist ein Produkt der group24 AG, Kern offen, Zusatzmodule und Support kommerziell.

## Entscheidung

**Open Core:** Kern unter **AGPL**, Support und Zusatzmodule kommerziell. Folge: nur permissive Abhängigkeiten (MIT, Apache 2.0, BSD, MPL) oder AGPL-verträgliche; Rechte an jedem Beitrag bei der group24 AG, sonst ist keine kommerzielle Zweitlizenz möglich (V4)

## Verworfen

proprietär; noch offen

## Folgen

Nur Abhängigkeiten mit AGPL-verträglicher Lizenz (MIT, Apache 2.0, BSD, MPL). Beiträge Dritter brauchen eine Rechteübertragung (V4). Lizenzkopf in jeder Quelldatei.
