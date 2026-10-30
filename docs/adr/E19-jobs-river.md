# E19: Hintergrundjobs mit River in PostgreSQL

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E19
- **Architektur:** [`architektur.md`](../architektur.md) §2, §8, §11

## Kontext

Jobs sollen in derselben Transaktion entstehen wie die Änderung, die sie auslöst.

## Entscheidung

Hintergrundjobs mit **River in PostgreSQL**, transaktional mit den Fachdaten

## Verworfen

Valkey/Asynq mit Outbox; NATS JetStream

## Folgen

Kein eigener Jobdienst. Warteschlangen mit Vorrang trennen interaktive Folgearbeit von Massenläufen (LZ10).
