# E28: Technische Logs nur an das Monitoring

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E28
- **Architektur:** [`architektur.md`](../architektur.md) §2

## Kontext

Der Betrieb hat eigene Werkzeuge für Logs; die Oberfläche soll sich auf Daten konzentrieren.

## Entscheidung

**Technische Logs nur an das Monitoring**, strukturiert als JSON (Loki, Graylog o. ä.). In der Oberfläche bleiben Audit und Verlauf der Daten sowie der Stand der Jobs

## Verworfen

zusätzlich eine Log-Seite in der Oberfläche

## Folgen

`slog` als JSON auf die Standardausgabe. In der Oberfläche bleiben Audit, Verlauf und der Stand der Jobs.
