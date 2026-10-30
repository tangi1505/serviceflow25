# E37: Definitionen in drei Schichten

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E37
- **Architektur:** [`architektur.md`](../architektur.md) §4

## Kontext

Pakete sollen andere Pakete erweitern und Kunden in der Oberfläche anpassen können, ohne Updates zu verlieren.

## Entscheidung

**Drei Schichten:** Basispaket, Erweiterungspakete, Kundenschicht; der Kern führt sie zur wirksamen Definition zusammen; ein Update des Basispakets überschreibt nie die Kundenschicht, Konflikte werden angezeigt

## Verworfen

kopieren und ändern

## Folgen

Basispaket, Erweiterungen, Kundenschicht werden zur wirksamen Definition zusammengeführt und gecacht. Konflikte zeigt der Kern vor der Aktivierung.
