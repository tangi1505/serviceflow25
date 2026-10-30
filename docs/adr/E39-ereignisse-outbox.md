# E39: Ereignisse über Outbox, Webhooks, Strom und SSE

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E39
- **Architektur:** [`architektur.md`](../architektur.md) §7

## Kontext

Fremd-Engines und Middleware dürfen nach einem Ausfall keine Ereignisse verlieren.

## Entscheidung

**Ereignisse** transaktional in einer Outbox; hinaus als signierter Webhook mit Wiederholung, als abrufbarer Strom ab Position und per SSE im Browser

## Verworfen

nur Webhooks; zusätzlich ein Message-Broker

## Folgen

Eine fortlaufende Position je Ereignis; Abonnenten lesen ab ihrer Position nach (E46).
