# E8: Eigene schlanke Engine hinter einem Workflow-Provider

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E8
- **Architektur:** [`architektur.md`](../architektur.md) §8

## Kontext

Statusmodell, Genehmigungen und Fristen müssen am Datensatz schnell und transaktional laufen; zugleich sollen Flowzilla und andere Engines anschließbar sein.

## Entscheidung

**Schlanke eigene Engine** für alles am Datensatz (Status, Übergänge, Genehmigung, Zuweisung, SLA, Eskalation, Regeln, Benachrichtigung) hinter einer Schnittstelle **Workflow-Provider**. Fremde Engines (Flowzilla, Camunda, n8n) hängen über Ereignisstrom, REST-API und den Aktivitätstyp „externer Prozess" an; die eingebaute Engine ist Provider 1, Flowzilla Provider 2

## Verworfen

Flowzilla als Kern (zweite Laufzeit PHP, Lizenz je Kunde, Abhängigkeit im VS-NfD-Rechenzentrum); Temporal darunter; vollständige grafische Flow-Engine im Kern; Engine kleiner als beschrieben

## Folgen

Die eingebaute Engine ist Provider 1, Flowzilla Provider 2 über den Aktivitätstyp „externer Prozess". Langlaufende, systemübergreifende Abläufe gehören in eine Fremd-Engine, nicht in den Kern.
