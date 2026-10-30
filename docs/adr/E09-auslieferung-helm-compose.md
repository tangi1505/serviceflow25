# E9: Auslieferung per Helm und Docker Compose

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E9
- **Architektur:** [`architektur.md`](../architektur.md) §1, §12

## Kontext

Produktion beim Kunden im eigenen Rechenzentrum mit Skalierung; dazu Entwicklung, Demo und kleine Kunden.

## Entscheidung

**Kubernetes per Helm** für Produktion, **Docker Compose** für Entwicklung, Demo und kleine Kunden; dieselben Container in beiden

## Verworfen

nur Compose; nur Kubernetes

## Folgen

Dieselben Container in beiden Wegen. Compose startet eine Instanz mit beiden Rollen, Helm skaliert `api` und `worker` getrennt.
