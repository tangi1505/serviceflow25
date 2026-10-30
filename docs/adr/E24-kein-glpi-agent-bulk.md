# E24: Kein GLPI Agent; Daten über Discovery oder Middleware per Bulk

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E24
- **Architektur:** [`architektur.md`](../architektur.md) §6.4

## Kontext

Massendaten kommen aus eigenen Quellen oder über eine Middleware wie Flowzilla; der GLPI Agent wird nicht mehr genutzt.

## Entscheidung

**Kein GLPI Agent**, auch nicht als Collector oder als Datenformat. Daten kommen auf zwei Wegen: über **eigene Discovery-Quellen im Werkzeug** oder über eine **Middleware wie Flowzilla**. Für den zweiten Weg hat die REST-API **Bulk-Verarbeitung** (viele Datensätze je Aufruf, anlegen, ändern, abgleichen), **asynchrone Jobs** mit abfragbarem Stand sowie **Import und Update** als eigene Vorgänge. Das ist Teil des Kerns (PL), kein späterer Zusatz

## Verworfen

Inventurformat des GLPI Agent als Quelle annehmen; später entscheiden

## Folgen

Bulk-Verarbeitung, asynchrone Jobs, Import und Update sind Teil des Kerns, nicht ein späterer Zusatz.
