# E11: Eine REST-API mit OpenAPI, Webhooks und Ereignisstrom

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E11
- **Architektur:** [`architektur.md`](../architektur.md) §6

## Kontext

Frontend, Middleware, Fremd-Engines und später MCP brauchen dieselbe Schnittstelle.

## Entscheidung

**Eine REST-API mit OpenAPI**, die auch das eigene Frontend nutzt; dazu Webhooks und ein Ereignisstrom. Kein zweiter Weg am Frontend vorbei

## Verworfen

GraphQL; REST und GraphQL

## Folgen

Das eigene Frontend nutzt nur die öffentliche API; es gibt keinen zweiten Weg. Kein GraphQL.
