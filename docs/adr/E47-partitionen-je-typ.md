# E47: Partitionen je Typ nach Erstellungsmonat

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E47
- **Architektur:** [`architektur.md`](../architektur.md) §3.3

## Kontext

Zehn Jahre Tickets und Verlauf sollen nicht in einer ungeteilten Tabelle liegen.

## Entscheidung

**Partitionen je Typ einstellbar, nach Erstellungsmonat** über den Zeitanteil der UUIDv7; Audit und Outbox immer

## Verworfen

nur Audit und Ereignisse; später entscheiden

## Folgen

Das Paket entscheidet je Typ; der Zeitanteil der UUIDv7 ist der Partitionsschlüssel. Ein Wartungsjob legt an und räumt ab.
