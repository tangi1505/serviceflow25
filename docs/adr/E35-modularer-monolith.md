# E35: Modularer Monolith mit den Rollen api und worker

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E35
- **Architektur:** [`architektur.md`](../architektur.md) §1, §2

## Kontext

Ein kleines Team, ein Betrieb ohne viele Dienste, kein Netzwerk zwischen Bausteinen.

## Entscheidung

**Modularer Monolith:** ein Go-Binary mit den Rollen `api` und `worker`, getrennt skalierbar (V10)

## Verworfen

wenige Dienste mit gRPC

## Folgen

Grenzen zwischen Bausteinen über Go-Schnittstellen, geprüft per `depguard`. Ein Baustein lässt sich später abspalten.
