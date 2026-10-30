# E42: Deutsch und Englisch, barrierefrei nach BITV

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E42
- **Architektur:** [`architektur.md`](../architektur.md) §10

## Kontext

Behörden verlangen Barrierefreiheit; Nachrüsten von Übersetzung und Barrierefreiheit ist teuer.

## Entscheidung

**Deutsch und Englisch** von Anfang an (V6); **barrierefrei nach BITV 2.0 / WCAG 2.1 AA** mit automatischer Prüfung (`axe`) in den Tests (V7)

## Verworfen

nur Deutsch; beides später

## Folgen

Kein sichtbarer Text im Code; `axe` prüft in den Tests und bricht den Build.
