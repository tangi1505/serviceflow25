# E46: Ereignisse 7 Tage nachlesbar

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E46
- **Architektur:** [`architektur.md`](../architektur.md) §3.3, §7

## Kontext

Abonnenten sollen einen Ausfall über ein langes Wochenende überbrücken.

## Entscheidung

**Ereignisse sind 7 Tage nachlesbar,** einstellbar; das Audit ist davon getrennt

## Verworfen

30 Tage; 24 Stunden

## Folgen

Tagespartitionen der Outbox, danach gelöscht. Wer älter ist, gleicht neu ab.
