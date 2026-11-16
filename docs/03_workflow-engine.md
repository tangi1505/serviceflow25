# Plan: Workflow-Engine (WF)

**Stand:** 2026-10-20, erste Fassung aus Hauptplan H8. Teil des [Hauptplans](0_maintask.md),
Etappe 2 (Meilenstein 1) bis 5. Grundlage: [Architektur](architektur.md),
[Funktionsinventar](funktionsinventar-glpi.md), Kernfähigkeiten im Hauptplan §2.1.1,
Leistungsziele im Hauptplan §2.5. Nichts davon ist gebaut.

**Status:** ✅ abgenommen · 🟢 fertig, wartet auf die Abnahme · 🔨 in Arbeit · ⬜ offen ·
🔒 blockiert

**Für jede Aufgabe gilt:** Sie ist erst fertig, wenn `scripts/ci.sh` grün ist und die
Leistungsmessung aus Hauptplan H11 keinen Zielwert verfehlt und nichts um mehr als 10 %
langsamer macht (E32). Der Prüfweg nennt nur, was dazukommt. Alle Befehle laufen in WSL in
`~/police/itsm`. „Inventar G…" verweist auf die Zeilen des Funktionsinventars, „trägt KF…" auf
die Kernfähigkeit, für die die Aufgabe die Verantwortung hat.

---

## Aufgaben

### A — Durchstich (Meilenstein 1, zuerst)

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **WF1** | Statusmodell je Typ: Status, Übergänge mit CEL-Bedingung, Übergang per API, jedes Übergang ist ein Ereignis (Architektur §8) (trägt KF3; Inventar G162) | ✅ | 2 d | Typ `probe` mit drei Status: erlaubter Übergang per `curl` → 200 und Ereignis im Strom; unerlaubter → 409 mit Grund; `docker compose run --rm tools scripts/ci.sh` grün |

### B — Verbreitern bis Meilenstein 1

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **WF2** | Feldzustände je Status und Formularvariante: Pflicht, versteckt, vorbelegt, schreibgeschützt (B5) (Inventar G53; Hauptplan E110) | 🟢 | 1,5 d | Übergang nach „gelöst" ohne Pflichtfeld „Lösung" → 422 mit Feldname; im Status „geschlossen" ist das Formular schreibgeschützt; `docker compose run --rm tools scripts/ci.sh` grün |
| **WF3** | Rollen am Datensatz: Anforderer, Beobachter, Bearbeiter, je als Person, Gruppe oder Lieferant; Team je Vorgang; je Typ definierbar (Inventar G37, G79; Hauptplan E109) | 🟢 | 2 d | Vorgang mit Bearbeitergruppe → jedes Mitglied findet ihn unter „meine Gruppen"; Rollen eines anderen Typs sind dort nicht wählbar; `docker compose run --rm tools scripts/ci.sh` grün |
| **WF4** | Regel-Engine: Auslöser Anlegen, Ändern, Zeit; Bedingung in CEL; Aktionen setzen, zuweisen, anlegen, benachrichtigen; synchron vor dem Speichern oder danach als Job (Architektur §6.2) (trägt KF7; Inventar G57; Hauptplan E111) | 🟢 | 2 d | Regel „Kategorie Netz → Gruppe Netz" setzt die Gruppe beim Anlegen in derselben Antwort; eine Folgeregel läuft als Job; LZ4 hält in H11; `docker compose run --rm tools scripts/ci.sh` grün |
| **WF6** | Genehmigungen: Unterdatensatz mit Stufen in fester Reihenfolge oder parallel, Mindestanteil, Vertretung (trägt KF4; Inventar G43, G44); dazu Reiter „Genehmigung“, „Zu genehmigen“ im Arbeitsplatz, Bereich „Genehmigungen“ im Portal und Vertretung unter „Meine Einstellungen“ (E17) | 🟢 | 3 d (vorher 2 d) | Zwei Stufen, Stufe 2 erst nach Stufe 1; parallele Stufe mit 2 von 3 ist nach der zweiten Zustimmung erfüllt; Vertretung darf zustimmen; `docker compose run --rm tools scripts/ci.sh` grün |
| **WF7** | Genehmigungsvorlagen und Erinnerung an offene Genehmigungen (Inventar G45, G67); für die Anfrage nennt seit SD6 die Erfüllungsvorlage die Stufen (Regelaktion `approval`, SD-Plan N18) | 🟢 | 2,5 d (vorher 1 d) | Vorlage setzt beide Stufen in einem Schritt; nach der eingestellten Zeit geht eine Erinnerung (Testmail); `docker compose run --rm tools scripts/ci.sh` grün |
| **WF8** | Kalender mit Arbeitszeiten und Feiertagen (Inventar G169; Hauptplan E113) | 🟢 | 1 d | Frist von 8 Stunden ab Freitag 15 Uhr endet am Montag 15 Uhr bei 8–16 Uhr Arbeitszeit; ein Feiertag verschiebt sie um einen Tag; `docker compose run --rm tools scripts/ci.sh` grün |
| **WF9** | Fristen und SLA-Uhren: Uhr als Datensatz mit Start, Pause und Ziel; River-Job auf den nächsten Schwellwert, kein minütlicher Lauf (Architektur §8) (trägt KF5; Inventar G167; Hauptplan E114) | 🟢 | 2 d | Uhr startet beim Anlegen, pausiert im Status „wartet", Restzeit stimmt nach Fortsetzen; bei 100.000 offenen Uhren gibt es keinen Lauf über alle (Messung in H11); `docker compose run --rm tools scripts/ci.sh` grün |
| **WF10** | Eskalationsstufen mit Aktionen (Inventar G66, G168; Hauptplan E115) | 🟢 | 1,5 d | Bei 80 % der Frist setzt Stufe 1 die Priorität hoch, bei 100 % benachrichtigt Stufe 2 die Leitung; `docker compose run --rm tools scripts/ci.sh` grün |
| **WF11** | Wartegründe mit automatischer Folgemeldung und Lösung nach Zeit (Inventar G54; Hauptplan E116) | 🟢 | 1 d | Wartegrund „Rückmeldung Nutzer" mit zwei Erinnerungen und Lösung nach 7 Tagen läuft in der Testzeit ab wie eingestellt; `docker compose run --rm tools scripts/ci.sh` grün |
| **WF12** | Zeitregeln im Statusmodell: automatisch schließen, Warnung bei zu langer Laufzeit (Inventar G63, G64; Hauptplan E117) | 🟢 | 1 d | Ein gelöster Vorgang ist nach der eingestellten Zeit geschlossen; ein zu lange offener erzeugt eine Warnung; `docker compose run --rm tools scripts/ci.sh` grün |
| **WF13** | Untervorgänge (Aufgaben) mit Zeit, Planung und Bearbeiter (Inventar G41; Hauptplan E118) | 🟢 | 1,5 d | Drei Aufgaben an einem Vorgang, die Summe der Zeiten erscheint am Vorgang; eine Aufgabe mit Termin erscheint im Kalender (UI15); `docker compose run --rm tools scripts/ci.sh` grün |
| **WF14** | Zeitpläne, die Datensätze anlegen (wiederkehrende Vorgänge) (Inventar G61, G62; Hauptplan E119) | 🟢 | 1 d | Zeitplan „jeden Montag" legt in der Testzeit genau einen Vorgang je Montag an; `docker compose run --rm tools scripts/ci.sh` grün |
| **WF15** | Benachrichtigungen: Ereignis, Empfänger aus Rollen am Datensatz, Vorlage mit Platzhaltern je Sprache, Mail hinaus über SMTP; Mailpit als Compose-Profil zum Testen (trägt KF14; Inventar G160, G161; Hauptplan E120) | 🟢 | 2 d | Neuer Vorgang → Mail an die Bearbeitergruppe in Mailpit (http://localhost:8025), auf Deutsch oder Englisch nach Einstellung der Person; `docker compose run --rm tools scripts/ci.sh` grün |
| **WF19** | Posteingang: Benachrichtigungen auch im Werkzeug (E19). `core.notification` bekommt die Wege `mail` und `inbox` als zwei Schalter, beide als Vorgabe an, bestehende per Umzug auch; je Person ein Eintrag mit Betreff und Text in ihrer Sprache, Link zum Datensatz, Auslöser, Zeit und gelesen; der Abnehmer `notify` läuft auch ohne Mailserver (dann nur der Posteingang); API `GET /api/v1/me/inbox` (Seite und Zahl der ungelesenen), `POST /api/v1/me/inbox:read` (einzelne oder alle), `DELETE /api/v1/me/inbox/{id}`; ein neuer Eintrag meldet sich im Ereignisstrom der Sitzung als `inbox` mit der Zahl der ungelesenen; ein Job löscht Einträge nach der Frist (Einstellung `inbox_retention_days` am Organisationsbaum, Vorgabe 90). Die Glocke baut UI35 | 🔨 | 2 d | Ohne Mailserver: Benachrichtigung bei `create` an `actors.assigned`, neue Störung mit Annas Gruppe als Bearbeiter → `GET /api/v1/me/inbox` als Anna zeigt einen ungelesenen Eintrag mit deutschem Betreff und Link, `:read` setzt die Zahl auf 0, `DELETE` entfernt ihn; ein Eintrag älter als die Frist ist nach dem Job weg; nur `mail` an → kein Eintrag; mit Mailpit (`--profile mail`) kommt die Mail dazu; eine offene Verbindung zu `/api/v1/events/stream` bekommt `event: inbox`; `docker compose run --rm tools scripts/ci.sh` grün, LZ4 und LZ13 nicht mehr als 10 % langsamer |

### C — Meilenstein 2

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **WF5** | Regeln für weitere Typen; Regeln testen, exportieren, importieren (Inventar G58, G59, G123) | ⬜ | 1,5 d | Regeltest mit Beispieldaten zeigt, welche Regel greift, ohne zu speichern; Export und Import ergeben dieselbe Regel; `docker compose run --rm tools scripts/ci.sh` grün |
| **WF16** | Workflow-Provider: Schnittstelle `Start`, `Signal`, `Cancel`; Aktivität „externer Prozess" mit Rückruf (E8); Flowzilla als Provider 2 folgt in IN | ⬜ | 2 d | Ein Testprovider im Test bekommt den Start, ruft zurück, der Vorgang geht in den nächsten Status; `docker compose run --rm tools scripts/ci.sh` grün |

### D — Meilenstein 3 und 4

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **WF17** | Erinnerungen an geplante Termine (Inventar G75) | ⬜ | 0,5 d | Termin mit Erinnerung 15 Minuten vorher → Benachrichtigung im Browser zur richtigen Zeit (Testzeit); `docker compose run --rm tools scripts/ci.sh` grün |
| **WF18** | Regel auf eine gespeicherte Suche: benachrichtigen, wenn sie Treffer hat (Inventar G113) | ⬜ | 1 d | Gespeicherte Suche „Vorgänge ohne Bearbeiter" mit Regel → Mail, sobald ein Treffer entsteht; `docker compose run --rm tools scripts/ci.sh` grün |

**Summe:** 27,5 d, geschätzt vor dem Bau (Block A: 2 d); WF6 am 2026-11-10 von 2 d auf 3 d für Oberfläche, Bereich im Portal und Vertretung (E17). WF7 am 2026-11-12 von 1 d auf 2,5 d für Kerntypen, Rollen, Erinnerung als Job und den Umzug der Anfrage (E18): 29 d. WF19 am 2026-11-16 neu mit 2 d für den Posteingang (E19): 31 d.

**Stand der Reihenfolge (2026-10-28, Hauptplan E108):** Block B von WF kommt jetzt, vor Block B der Oberfläche; Block B ist gebaut: WF3, WF2, WF4 und WF8 bis WF15; dazwischen kam PL41 (Verteiler in Stapeln, Kern-Plan) aus dem Befund von WF4. Danach kam PL42 (Zeitleiste, Kern-Plan, Hauptplan E121) mit dem Ereignis `comment` für Benachrichtigungen. Als Nächstes Block B der Oberfläche (UI-Plan), dann SD; WF5 bis WF7 und WF16 folgen in Block C.

**Reihenfolge:** Block A mit dem Durchstich (Hauptplan E59): WF1, sobald PL3 steht. Danach Block B: WF3 und WF2 (Rollen, Feldzustände), WF4 (Regeln), WF8 und WF9 (Kalender, Uhren), dann der Rest. WF15 vor dem Service Desk (SD), weil der Mail braucht. WF16 in Block C, damit IN Flowzilla anbinden kann. WF19 (Posteingang) kommt vor UI35, weil die Glocke der neuen Kopfzeile seine Zahl zeigt (Hauptplan E189).

---

## 1. Entscheidungen

Die übergreifenden Entscheidungen stehen im Hauptplan §1 (E8, E19, E45).

| # | Entscheidung | Seit | Quelle |
| --- | --- | --- | --- |
| **E1** | Mailpit (MIT) als Testserver für Mail im Compose-Profil; nur zum Testen (V1) | 2026-10-20 | Pascal |
| **E2** | Das Statusmodell steht in der Typdefinition (`states` mit genau einem `initial`, `transitions` mit `from`, `to`, `when`); der Status liegt in der Systemspalte `state`, ein neuer Datensatz startet im Anfangsstatus, PATCH auf `state` wird abgelehnt (422), nur ein Übergang ändert ihn | 2026-10-20 | Umsetzung |
| **E3** | Übergang per `POST …/{id}:transition` mit `{"transition": name}`: Zeile gesperrt, Status und CEL-Bedingung geprüft, Status mit neuer Version geschrieben, Audit-Zeile `transition` und Ereignis in derselben Transaktion; nicht erlaubt → 409 mit Grund (Status oder Bedingung), unbekannt → 422. `GET …/{id}/transitions` sagt je Übergang, ob er gerade geht und warum nicht | 2026-10-20 | Umsetzung |
| **E4** | CEL (`internal/rules`, cel.dev/cel-go) sieht den Datensatz als `record` (Felder und Systemfelder) und muss `true`/`false` liefern; die Definition prüft Bedingungen beim Installieren, ein fehlendes Feld zur Laufzeit ist ein Fehler, kein stilles `false` | 2026-10-20 | Umsetzung |
| **E19** | Posteingang (WF19, deine Entscheidungen 2026-11-16 für die Glocke der Kopfzeile, UI35): Einträge entstehen nur aus den Benachrichtigungen (`core.notification`), nicht automatisch aus jeder Änderung an meinen Vorgängen; jede Benachrichtigung hat die Wege Mail und Posteingang, bestehende gehen nach dem Umzug an beide; Einträge bleiben 90 Tage, einstellbar, gelesen oder nicht; die Person markiert als gelesen und löscht selbst. Zum Bau, von dir am 2026-11-16 freigegeben: die Einträge liegen in einer Kerntabelle `inbox_entry` in der Datenbank des Mandanten, nicht als Typ, weil ein Typ je Eintrag Verlauf und Ereignis schreibt (bei 70.000 Personen zu teuer) und niemand außer der Person sie liest; die offenen Tabs erfahren davon über Valkey wie beim Ereignisstrom, ohne Abfragen im Takt. Verworfen: alles aus meinen Vorgängen (laut, nicht steuerbar); beides; nur Mail für bestehende; ohne Frist; 30 Tage | 2026-11-16 | Pascal |
| **E18** | Genehmigungsvorlagen und Erinnerung (WF7, deine Entscheidungen 2026-11-12 nach dem Mockup „Genehmigungsvorlagen und Erinnerung“, [Artifact](https://claude.ai/artifact/6QqiAzj1PyfcLmL7j4ikZH), Hauptplan E179): **Vorlage als Datensatz** des Kerns (`core.approval_template`: Name, Text an die Genehmiger, Erinnerung alle n Arbeitstage, aktiv; Stufen `core.approval_template_stage` mit Name, nötigen Zustimmungen, Reihenfolge) für jeden Typ mit Genehmigungen. **Genehmiger** sind feste Personen und Gruppen und Rollen des Vorgangs (`actors.<Rolle>`, `actors.<Rolle>.responsible` für die Leitung der Gruppen), aufgelöst beim Anfordern; eine Stufe ohne Genehmiger fällt weg. **Anfordern:** im Reiter „Genehmigung“ füllt die gewählte Vorlage Text und Stufen, danach ist alles änderbar; die Regelaktion `approval` nennt eine Vorlage (`template`: Id oder Weg über Verweisfelder). **Erinnerung** wiederholt, bis die Stufe entschieden ist, an alle in der laufenden Stufe, die noch nicht entschieden haben (Gruppen: alle Mitglieder), und ihre Vertretungen; der Abstand kommt aus der Vorlage (0: keine), sonst aus der Einstellung `approvals.remind_days` am Organisationsbaum (Vorgabe 2), gezählt in Arbeitstagen des Kalenders der Einheit; jede Erinnerung steht im Verlauf, die Mail ist eine Benachrichtigung der Startdaten (Ereignis `approval_reminder`). **Anfrage ganz umgestellt:** die Erfüllungsvorlage nennt eine Genehmigungsvorlage statt eigener Stufen (`request.template_stage` entfällt); vorhandene Stufen ziehen beim Update in eine Genehmigungsvorlage um. Verworfen: Vorlagen je Typ; nur beim Anfordern; nur feste Genehmiger; einmalige Erinnerung; Erinnerung mit Höchstzahl; nur an der Vorlage; Erfüllungsvorlage behält eigene Stufen | 2026-11-12 | Pascal |
| **E17** | Genehmigungen (WF6, deine Entscheidungen 2026-11-10 nach dem Mockup „Genehmigungen“, [Artifact](https://claude.ai/artifact/Mez7bLzqU887wXw9s9Mnwk), Hauptplan E175): **Merkmal `approvals` mit Kerntabellen** (`approval`, `approval_stage`, `approval_step`, Migration 00044), wie Bewertung und Zeitleiste; jeder Typ kann es anziehen. **Stufen** laufen nacheinander, in einer Stufe sind Personen und Gruppen gleichzeitig gefragt, die Stufe braucht „mindestens n“ Zustimmungen (0: alle); sie ist erteilt, sobald genug zugestimmt haben, abgelehnt, sobald das nicht mehr erreichbar ist; dann entfallen die offenen Fragen. **Eine Gruppe** ist einmal gefragt, das erste Mitglied entscheidet für sie. **Wirkung:** das berechnete Feld `approval` (offen, erteilt, abgelehnt, zurückgezogen) und die Übergänge, die der Typ unter `approval` nennt (`requested`, `approved`, `rejected`); sie laufen nach dem Schreiben mit den Rechten des Systems und dem Entscheider als Auslöser, der Grund einer Ablehnung geht als Text mit. **Vertretung** an der Person mit „von“ und „bis“ (Tabelle `substitute`), eingetragen von der Person selbst oder von der Verwaltung an der Person (Recht zum Ändern der Person); in der Zeit entscheidet die Vertretung für die Person und ihre Gruppen, im Verlauf steht „für“. **Lesen:** wer gefragt ist oder entschieden hat, liest den Datensatz ohne eigenes Recht (`authz.Access.Asked`). **Ablehnen** braucht immer einen Grund. **Oberfläche:** Reiter „Genehmigung“ nach „Bezüge“, links „Zu genehmigen“ mit Anzahl, im Portal ein eigener Bereich „Genehmigungen“ im Kopf (Variante 3B) mit Seite zum Lesen und Entscheiden, Vertretungen unter „Meine Einstellungen“ und als Reiter an der Person. Benachrichtigungen kennen die Ereignisse `approval_request`, `approval_decision`, `approval` und den Empfänger `approvers`. Verworfen: Kerntyp „Genehmigung“ als Datensatz; nur Feld und Ereignis ohne Übergänge; jedes Gruppenmitglied einzeln; Weiterleiten je Genehmigung; „Zu genehmigen“ auf der Startseite des Portals (3A); Abschnitt über dem Formular; nur die Person selbst trägt Vertretungen ein | 2026-11-10 | Pascal |
| **E16** | `internal/notify`: `HandleBatch` (Abnehmer `notify`, nur wo ein Mailserver gesetzt ist) liest die aktiven Benachrichtigungen einmal je Stapel über `records.Query` und legt je passendem Ereignis einen Job `notify.fanout` an, je notify-Ereignis einer Regel ebenso. `notify.fanout` liest Ereignis und Datensatz (wie er jetzt ist), prüft die Bedingung, löst die Empfänger auf (Rollen aus `actors`, Gruppen über `core.group_member`, Personen), zieht doppelte und den Auslöser ab (Konto → Person über `authz`) und legt je Person einen Job `notify.send` an, eindeutig nach seinen Argumenten (River `UniqueOpts.ByArgs`), damit ein wiederholter Lauf keine Mail doppelt legt. `notify.send` liest Person (Adresse, Sprache, sonst Deutsch), Benachrichtigung und Datensatz mit `records.Localize` in ihrer Sprache, rendert und schickt mit `net/smtp` (STARTTLS und Anmeldung wählbar, Anmeldung nur mit STARTTLS); 8 Versuche, danach `delivery_failure` mit der Art `mail`. Platzhalter prüft die Prüfung beim Speichern (`CheckRecord`); ein unbekannter in einer Übersetzung bleibt im Text stehen. Für Tests gibt es `internal/notify/smtptest`, einen SMTP-Empfänger im Prozess | 2026-10-29 | Umsetzung |
| **E15** | `internal/recurrence`: `Run(ctx, now)` fragt über `records.Query` nur die aktiven Vorgänge mit leerem oder erreichtem `next_run` ab (kein Lauf über alle), legt den Datensatz des Termins mit `records.Sync` und dem Schlüssel `recurrence:<id>` / Termin an (der eindeutige Index `(source, external_key)` aus PL31 macht es genau einmal; eine zweite Instanz bekommt einen Konflikt oder „unverändert“) und schreibt `next_run` (nächster Termin nach jetzt, in der Zeitzone, auch über die Zeitumstellung), `last_run` und `last_error` über `records.Update`, also mit Verlauf. Ein leeres `next_run` wird nur gesetzt, ohne anzulegen. Die Prüfung beim Speichern (`CheckRecord`, mehrere über `records.Checks`) füllt die Vorlage für einen Beispieltermin und prüft sie mit `CheckValues` gegen den Zieltyp. Der Job `recurrence.run` läuft jede Minute auf der Standard-Warteschlange | 2026-10-29 | Umsetzung |
| **E14** | Ein Summenfeld ist ein Zahlfeld mit `rollup: {from, via, op, field, where}`; `from` muss ein Typ desselben Pakets sein, `via` ein Referenzfeld von `from` auf diesen Typ, `sum` braucht ein Zahlfeld, `count` keines. Der Lader setzt es auf `computed` wie die Kostensummen (Spalte mit Vorgabe 0, nicht schreibbar, ohne Version). `internal/records/rollup.go`: Nach jedem Schreiben eines gezählten Datensatzes (Anlegen, Ändern, Übergang, Papierkorb, Wiederherstellen) sperrt `rollupsWritten` die Zeilen der Vorgänge vorher und nachher in der Reihenfolge der Ids und zählt danach in einer eigenen Anweisung (READ COMMITTED sieht dann, was vorher festgeschrieben wurde), schreibt geänderte Summen mit Verlaufszeile und Ereignis `update` zur aktuellen Version und gibt den Uhren des Vorgangs Bescheid. `where` wird mit `authz.Translate` nach SQL übersetzt; geht das nicht, wird das Schreiben der Aufgabe abgelehnt (`meta` prüft beim Laden nur, dass CEL es versteht, weil `authz` von `meta` abhängt). Bulk-Zeilen eines gezählten Typs gehen den Weg von Sync | 2026-10-29 | Umsetzung |
| **E13** | `core.sla` hat das Feld `counts` (`calendar` als Vorgabe, `round_the_clock`); bei `round_the_clock` bekommt eine neue Uhr keinen Kalender, auch wenn die SLA oder die Einheit einen nennt. Das Feld wirkt beim Start einer Uhr wie Ziel und Kalender (E10); eine Änderung trifft nur neue Uhren | 2026-10-29 | Umsetzung |
| **E12** | Ein Wartegrund ist eine SLA mit `start_from: applies`, `applies` auf Status und Grund (etwa `record.state == "waiting" && record.wait_reason == "<id>"`) und `stop: false`. Weil `applies` beim Ende der Wartezeit falsch wird, wird die Uhr abgebrochen; beim nächsten Warten startet sie mit `start_from: applies` neu ab jetzt, mit leeren Stufen (sonst rückwirkend ab Anlage, E10). Stufen haben `percent` oder `minutes` (Spalte `rule.minutes`, genau eines von beiden); die Uhr sortiert die Stufen nach ihrem Zeitpunkt. Die Regelaktion `transition` ruft `records.Transition` mit den Feldern; ein Übergang, den der Datensatz inzwischen nicht mehr erlaubt, bricht den Job ab und steht bei den Fehlschlägen (PL41). Im Probe-Typ das Feld `wait_reason` und `resolve` auch aus „Wartet“ | 2026-10-29 | Umsetzung |
| **E11** | Eine Stufe ist eine Regel mit `trigger: clock`, `sla` und `percent` (neue Spalten der Tabelle `rule`, Prüfung: Modus „after“, kein Zeitplan, die SLA gibt es und sie gilt für den Typ der Regel, 1 bis 1000 %). Die Uhr hält die erreichten Stufen in `clock.levels` und fragt die Engine nach den Stufen ihrer SLA (`Engine.Levels`, aus dem Zwischenspeicher der Regeln). `settle` vermerkt nach jeder Bewegung und in jedem Job die erreichten Stufen und den Verstoß und plant den Job auf die kleinste offene Schwelle (Stufe oder Frist); eine Stufe, die vor dem Job überschritten wurde (Pause, Lösung, später Start), vermerkt der Übergang selbst. Das Ereignis `escalation` nennt die Regel; der Regel-Abnehmer aus PL41 legt daraus genau einen Job dieser einen Regel an (`ApplyArgs.Rule`), so laufen die Aktionen genau einmal und mit dem Regelnamen im Verlauf. Eine neu gestartete Uhr (Umstufung) erreicht ihre Stufen neu | 2026-10-29 | Umsetzung |
| **E10** | Die Uhren liegen in `internal/sla`. records ruft zwei Haken: `Clocks.Written` in der Transaktion jedes Schreibens (Anlegen, Ändern, Übergang, Papierkorb, Wiederherstellen; nach dem endgültigen Löschen ohne Datensatz, dann gehen die Uhren mit) und `CheckRecord` vor dem Schreiben (eine SLA nennt einen installierten Typ, ihre Bedingungen lassen sich übersetzen; sonst 422). Eine Uhr merkt sich die bis zum Beginn des laufenden Abschnitts gezählte Arbeitszeit (`used_seconds`, `resumed_at`) und daraus die Frist (`due_at`); das Ziel und der Kalender werden beim Start kopiert, eine spätere Änderung der SLA trifft nur neue Uhren. Jede Änderung zählt `generation` hoch; der Job `sla.due` trägt sie und tut nichts, wenn sie nicht mehr stimmt. Kommt der Job und ist die Frist nach dem Kalender noch nicht erreicht (ein Feiertag kam dazu), plant er sich neu. Geht eine Uhr über die Frist, ohne dass der Job vorher kam (Pause oder Lösung kurz danach), merkt das der Übergang selbst. Der Verstoß steht im Verlauf als Aktion `breach` (Feld `clocks`) und als Ereignis `breach` mit SLA und Zeitpunkt. Die SLAs hält jede Instanz 30 Sekunden im Speicher; eine Änderung an `core.sla` leert ihn hier und über Valkey auf den anderen. Ein Kalender, der nicht rechnen kann, zählt rund um die Uhr und steht im Log, damit ein kaputter Kalender nicht jedes Ticket blockiert. Bulk-Zeilen eines Typs mit SLAs gehen den Weg von Sync | 2026-10-29 | Umsetzung |
| **E9** | Das Rechnen liegt in `internal/calendar`: `Calendar.Add` und `Between` gehen Tag für Tag durch die Spannen in der Zeitzone des Kalenders (Spannen eines Tags sortiert und zusammengefasst, Feiertage ohne Spannen, höchstens zehn Jahre, ohne jede Arbeitszeit `ErrNoHours`); `Service.Load` liest Kalender, Arbeitszeiten und Feiertage mit den Rechten des Aufrufers über `records`, `ForNode` die Einstellung `calendar.default`, `GenerateHolidays` legt fehlende Tage über `records` an. `GermanHolidays` rechnet Ostern nach Meeus/Jones/Butcher und kennt die landesweiten Feiertage der 16 Länder; Tage, die nur einzelne Gemeinden halten (Mariä Himmelfahrt in Bayern, Fronleichnam in Teilen von Sachsen und Thüringen, Augsburger Friedensfest), fehlen bewusst. Die Zeitzonen reisen mit dem Programm (`time/tzdata`), weil das Image keine hat | 2026-10-28 | Umsetzung |
| **E8** | Die Engine liegt in `internal/rules/engine` (CEL bleibt in `internal/rules`, mit `CompileRule` für `record` und `changed`). Vor dem Schreiben ruft `records` den Haken `Before` mit dem Datensatz, wie er wird, und den Namen der Änderungen; was die Regeln setzen, prüft `records` wie eine Eingabe und schreibt es in derselben Version, ohne Feldrechte und schreibgeschützte Feldzustände des Aufrufers. Die Regeln werden je Instanz zwischengespeichert und bei jeder Änderung über Valkey verworfen (spätestens nach 30 s). Nach dem Schreiben stellt der Verteiler (PL24) je Ereignis einen eindeutigen Job `rules.apply` ein; der Job schreibt mit `records.WithRule` (Regel und Tiefe in Verlauf und Ereignis). Der Job `rules.time` läuft jede Minute und führt fällige Regeln nach Zeitplan aus (`RunSchedules`). Eine Schreibung, die der Datensatz ablehnt, beendet den Job ohne Wiederholung. Bulk-Zeilen, an denen eine Regel vor dem Schreiben etwas ändert, gehen über den Zeilenweg. LZ4 misst jetzt mit zehn Regeln für `probe.probe` (acht vorher, eine trifft; zwei danach) | 2026-10-28 | Umsetzung |
| **E7** | Feldzustände (`internal/records/fieldstates.go`): `effective` addiert die zutreffenden Regeln für einen Status und einen Datensatz in API-Form (Bedingungen kompiliert und je Text zwischengespeichert; eine Bedingung, die an einem leeren Feld scheitert, gilt als nicht zutreffend); `createStates` und `changeStates` prüfen beim Anlegen und Ändern, auch im Mengenweg des Bulk. Ein Übergang läuft jetzt über denselben Schreibweg wie PATCH (`update` mit `move`): Zeile gesperrt, Version geprüft (ohne `If-Match` die aktuelle), Status und Bedingung gegen den Datensatz mit den neuen Feldern, Feldzustände, Verlaufszeilen `update` je Feld und `transition` für `state`, ein Ereignis der Art `transition`, das alle geänderten Felder nennt. Das Formular holt die Feldzustände über `:field-states`, blendet Versteckte aus, markiert Pflichtfelder, zeigt Schreibgeschützte als Text und schickt sie nicht mit, belegt einen neuen Datensatz vor und zeigt einen gesperrten ohne Speichern. `probe.probe` hat dafür das Feld `solution`, die Status `resolved` und `closed` mit `resolve` und `close` und vier Regeln | 2026-10-28 | Umsetzung |
| **E6** | Die Halter einer Rolle liest `records` als abgeleitete Spalte `actors` per Unterabfrage auf `actor` (ein Rundlauf je Abfrage, auch in Listen und in der JSON-Abfrage) und schreibt sie in derselben Transaktion wie den Datensatz: je geänderter Rolle löschen und neu einfügen (`internal/records/actors.go`). Die Prüfung braucht den Haltertyp in `to` der Rolle und einen vorhandenen Datensatz; `single` erlaubt einen Halter. Bulk-Zeilen mit `actors` gehen über den Zeilenweg. Die Reichweite `involved` sucht in SQL über `id IN (SELECT record_id FROM actor …)` und in Go über die Halter am gelesenen Datensatz; `authz.Access` kennt dafür die Gruppen der Person (`Groups`). Das Paket `probe` hat die Rollen `requester`, `observer`, `assigned` und hängt deshalb von `core` ab (Installationsreihenfolge core, probe in CI und Tests) | 2026-10-28 | Umsetzung |
| **E5** | Die Outbox (Hauptplan E39) beginnt mit WF1: Tabelle `event` (Position, Zeit, Art, Typ, Datensatz, Akteur, Nutzlast) und `GET /api/v1/events?after=&limit=`; Verteiler, Aufbewahrung und SSE kommen mit PL24 | 2026-10-20 | Umsetzung |

## 2. Begründung

- **Was WF baut,** steht in der [Architektur](architektur.md) §8.
- **Abgedeckt:** alle 26 Zeilen des Funktionsinventars mit Ziel WF und die Kernfähigkeiten KF3,
  KF4, KF5, KF7, KF14 als Träger.
- **Mail hinein** (IMAP) gehört zum Service Desk (SD), Mail hinaus zu WF15.

## 3. Offene Fragen

| # | Frage | Hängt daran |
| --- | --- | --- |
Keine offen.

**Beantwortet am 2026-10-20:** V1 (E1).

Die Fragen an die Fachseite stehen im Hauptplan.

## 4. Umsetzungsstand

### WF1 — Statusmodell je Typ, 2026-10-20

- **Grundlage:** Hauptplan E8, E16, E29, E39; Architektur §8; E2 bis E5 dieses Plans; PL1 bis
  PL5, UI2.
- **Gebaut:** `internal/meta` kennt `states` und `transitions` je Typ und prüft sie
  (snake_case, genau ein Anfangsstatus, bekannte Status, Bedingungen kompilieren);
  `internal/rules` wertet CEL aus; `internal/records/transition.go` mit `Transition`,
  `Transitions` und `Events`; Migration 00006 (Tabelle `event`, Audit-Aktion `transition`);
  API `:transition`, `/transitions`, `/events` in `api/openapi.yaml` und `internal/httpapi/
  transitions.go`; das Paket `probe` hat die Status `new`, `in_progress`, `done` mit `start`,
  `finish` und `reopen` (nur bei `record.priority <= 3`). Oberfläche: `web/src/render/
  Transitions.tsx` zeigt Status und je Übergang einen Knopf, nicht mögliche sind gesperrt und
  nennen den Grund; ein Klick wechselt den Status und frischt Datensatz und Liste auf.
- **Belegt, 2026-10-20 im Werkzeug-Container:**
  - `go test` (`rules`, `meta`, `records`, `httpapi`) grün, Linter ohne Befund. Prüfweg per
    HTTP: neuer Probe-Datensatz steht in `new`; PATCH auf `state` → 422; `/transitions` nennt
    `start` erlaubt, `finish` und `reopen` nicht (Grund `state`); `finish` aus `new` → 409 mit
    Grund; unbekannter Übergang → 422; `start`, `finish`, `reopen`, `start`, `finish` → je 200
    mit neuem Status, ETag und Version 2 bis 6; nach `priority: 5` ist `reopen` 409 und die
    Liste sagt `condition`; `/api/v1/events` liefert die fünf Übergänge in Reihenfolge mit
    Akteur; der Verlauf zeigt fünf Audit-Zeilen `transition` am Feld `state`.
  - Vitest, 20 Tests grün: Statuskasten mit „Neu", „Beginnen" aktiv, „Erledigen" gesperrt mit
    Grund, Klick → „In Arbeit" und Version 2, danach „Erledigt", `reopen` bei Priorität 5
    gesperrt mit „Die Bedingung ist nicht erfüllt."; eine Ablehnung des Servers (409) erscheint
    als Hinweis und die Knöpfe werden neu geladen.
  - Playwright gegen den Server: Datensatz anlegen, „Erledigen" gesperrt, „Beginnen" → „In
    Arbeit", Version 2, danach der Konfliktfall aus UI2 bis Version 4; axe ohne Befund.
    Unterwegs gefunden: axe maß eine Farbe mitten in einer CSS-Transition (2,37:1); die
    Browser-Tests laufen jetzt mit `reducedMotion`, und `tokens.css` schaltet Transitionen bei
    `prefers-reduced-motion` ab. Gesperrte Knöpfe nutzen Tokens statt Transparenz.
  - `scripts/ci.sh` komplett grün (Go, Last mit Gate, Web mit 20 Vitest- und 2 Browser-Tests, LZ9 dort 89 ms).
- **Nicht geprüft:** Aktionen beim Übergang (Feld setzen, benachrichtigen) kommen mit WF4,
  Feldzustände je Status mit WF2, Rechte je Übergang mit PL8; der Ereignisstrom hat weder
  Aufbewahrung noch SSE noch Verteiler (PL24); Übergänge laufen nur über die API, nicht per
  Bulk (PL31); die Bedingung sieht nur den Datensatz, nicht die Person (kommt mit PL8).

### WF7 — Genehmigungsvorlagen und Erinnerung, 2026-11-12

- **Grundlage:** Hauptplan E179, KF4; Inventar G45, G67; E18 dieses Plans; deine Entscheidungen
  vom 2026-11-12 und das Mockup „Genehmigungsvorlagen und Erinnerung“ (Zeichenflächen 1 bis 4).
- **Gebaut:**
  - Kern: Kerntypen `core.approval_template` und `core.approval_template_stage` (Paket `core`
    0.5.0); `internal/records/approval_template.go` füllt eine Vorlage für einen Datensatz aus
    (Rollen aufgelöst, leere Stufen weg) und prüft die Rollen der Stufen;
    `internal/records/approval_remind.go` mit dem Job `approval.remind` jede Minute; die
    Genehmigung trägt Vorlage und Abstand, jede Stufe Anzahl, letzte und nächste Erinnerung
    (Migration 00046); `calendar.AddDays` zählt Arbeitstage; Benachrichtigungen kennen
    `approval_reminder`, und der Empfänger `approvers` schließt die Vertretungen ein.
  - Regelaktion `approval` mit `template` (`internal/rules/engine/each.go`); API
    `GET …/approval-templates`, `ApprovalRequest.template`.
  - Paket `request` 0.3.0: Feld „Genehmigungsvorlage“ an der Erfüllungsvorlage, die Regeln
    fragen über `template.approval_template`, Mail „Erinnerung an Genehmigung“; der Umzug
    (`cmd/itsm/upgrade.go`) macht beim Update von einer Version unter 0.3.0 aus den Stufen jeder
    Erfüllungsvorlage eine Genehmigungsvorlage und stellt die zwei Regeln der Startdaten um,
    solange sie noch die alten Stufen nennen (`seed.Refresh`). Paket `knowledge` 0.7.0 mit der
    Mail „Erinnerung an Freigabe“.
  - Oberfläche: im Reiter „Genehmigung“ die Auswahl „Vorlage“ beim Anfordern, die Herkunft aus
    einer Rolle unter den Genehmigern, der Hinweis zur Erinnerung; an der Stufe „Erinnert …,
    nächste Erinnerung …“.
- **Belegt:**
  - Go `TestApprovalTemplatesOverHTTP` (Prüfweg): Entwürfe nach Namen, inaktive und leere Stufen
    weg, `actors.group.responsible` wird zur Leitung der Gruppe; ohne Änderungsrecht kein
    Entwurf; angefordert aus dem Entwurf trägt die Genehmigung Vorlage und Tage; der Job
    erinnert erst, wenn es so weit ist, zählt, setzt die nächste Erinnerung und erinnert nicht
    zweimal; zurückgezogen keine Erinnerung mehr, eine Vorlage mit 0 Tagen nie; eine Rolle, die
    keine ist, und eine unbekannte Vorlage werden abgelehnt. `TestAddDays`.
  - Vitest `Approvals.test.tsx`: Vorlage wählen füllt Text und Stufen, Herkunft aus der Rolle,
    Hinweis zur Erinnerung, die Anfrage nennt die Vorlage, `axe` ohne Befund; die Stufe zeigt,
    wie oft sie erinnert hat.
  - Alte Tests angepasst, weil die Erfüllungsvorlage seit WF7 eine Genehmigungsvorlage nennt
    statt eigener Stufen: `TestRequestOverHTTP` und `TestCatalogOverHTTP` legen
    `core.approval_template` mit Stufen an, die Regeln mit falscher Angabe prüfen
    `core.approval_template_stage` und drei neue Fälle mit `template`; die Zahl der Startdaten
    ist bei `request` 12 statt 11 und bei `knowledge` 10 statt 9 (die Erinnerungsmails).
  - `scripts/ci.sh` im zweiten Lauf 18 von 19 Schritten grün (126 Vitest, 18 Playwright); rot nur
    LZ3 gegen die alte Basislinie (+15 %), im A/B gegen SD17 (`73035ab`) 0 % (10,58 gegen
    10,59 ms), LZ1, LZ2 und LZ4 gleich auf; die Basislinie erneuerst du. Im ersten Lauf: Lint
    (`nilerr` in der Regelaktion), E34 (der Umzug las die alte Tabelle in `cmd/itsm`, jetzt
    `records.RemovedRows`), zwei Tests mit Zufallstext im Feld „Rollen des Vorgangs“
    (Muster in `coverage_test.go` wie bei SD13), Lasttor ungültig unter fremder Last.
- **Nicht geprüft:** Der Umzug beim Update hat keinen automatischen Test, weil es für die
  Paketinstallation keine Datenbanktests gibt; geprüft auf 8092 beim Einspielen. Die Mail der
  Erinnerung nur über den Empfänger `approvers` wie bei WF6, nicht in Mailpit. Ein Feiertag
  mitten im Abstand zählt über den Kalender der Einheit, ohne Kalender zählen Kalendertage.

### WF6 — Genehmigungen, 2026-11-10

- **Grundlage:** Hauptplan E175, KF4; Inventar G43, G44; E17 dieses Plans; deine Entscheidungen
  vom 2026-11-10 (Merkmal mit Kerntabellen, der Typ nennt die Übergänge, ein Mitglied für die
  Gruppe, Vertretung an der Person; nach dem Mockup: eigener Bereich im Portal, Reiter, „Zu
  genehmigen“ mit Anzahl, Vertretung durch die Person und die Verwaltung).
- **Gebaut:**
  - Kern: Merkmal `approvals` mit dem berechneten Feld `approval` und Typ-Option `approval`
    (`internal/meta/approval.go`); Migration `00044_approval.sql` (Genehmigung, Stufe, Frage,
    Vertretung, Verlaufsaktion `approval`); `internal/records/approval.go` (anfordern, entscheiden,
    zurückziehen, „Zu genehmigen“, Übergang danach) und `substitute.go`; in `internal/authz`
    liest, wer gefragt ist (`Asked`, auch als Vertretung), in Listen und einzeln; beim
    endgültigen Löschen gehen die Genehmigungen mit.
  - API: `GET`/`POST …/{id}/approvals`, `POST …/approvals/{approvalId}:decide` und `:withdraw`,
    `GET /api/v1/me/approvals`, `GET`/`POST`/`DELETE /api/v1/persons/{personId}/substitutes`.
  - Benachrichtigungen: Ereignisse `approval_request`, `approval_decision`, `approval`, Empfänger
    `approvers` (wen die Stufe fragt); Paket `core` 0.3.0 mit den Beschreibungen dazu.
  - Paket `probe` 0.2.0: Typ `probe.order` (Bestellung) mit Genehmigung, Übergänge `ask`,
    `grant`, `refuse` (mit Grund).
  - Oberfläche: `render/Approvals.tsx` (Reiter mit Stufen, Entscheidung, Anfordern,
    Zurückziehen), `render/ApprovalList.tsx` („Zu genehmigen“ und Bereich im Portal),
    `portal/Approval.tsx` (lesen und entscheiden im Portal), `render/Substitutes.tsx` (unter
    „Meine Einstellungen“ und als Reiter „Vertretungen“ an der Person).
- **Alte Tests angepasst:** `portal/Portal.test.tsx` klickt „Meine Vorgänge“ jetzt im Inhalt:
  der Kopf des Portals zeigt die Bereiche immer, weil „Genehmigungen“ für jeden dort steht, und
  hat damit einen zweiten Link „Meine Vorgänge“.
- **Belegt:**
  - Go `TestApprovalsOverHTTP` (Prüfweg): zwei Stufen; die Bestellung wartet nach dem Anfordern;
    eine zweite offene Genehmigung ist 409; Stufe 2 fragt erst nach Stufe 1 (vorher 404 beim
    Lesen und Entscheiden); die Leitung trägt eine Vertretung ein, die Vertretung entscheidet
    „für“ sie; Ablehnen ohne Grund ist 422; in „mindestens 2 von 3“ lehnt die Gruppe ab, zwei
    Personen stimmen zu, dann ist die Bestellung genehmigt; eine Ablehnung schließt die zweite
    Bestellung mit dem Grund in der Zeitleiste; Zurückziehen lässt die Fragen entfallen und das
    Lesen enden; falsche Anfragen 422; ein Typ ohne Merkmal 404; Verlauf und Ereignisse.
  - Go `TestApprovalSpec`: Feld, Prüfung des Blocks `approval`.
  - Vitest `render/Approvals.test.tsx`: Reiter mit Stufen und „für“, Ablehnen nur mit Grund,
    Zustimmen schickt die Frage; Anfordern in Stufen; „Zu genehmigen“ mit Anzahl und im Portal
    als Bereich mit Seite zum Entscheiden; Vertretung an der Person eintragen und entfernen; mit
    axe.
  - Gefunden im ersten CI-Lauf, beides aus WF6: der Abdeckungstest (PL30) fand bei einer neuen
    Bestellung im Feld `approval` den Wert „0“. Der Kern gab jedem berechneten Feld die Vorgabe 0
    und beschrieb es als nie leer, gedacht für Summen (PL20). Jetzt gilt das nur für Zahlenfelder
    (`internal/schema`, Beschreibung in `internal/httpapi/api.go`); eine berechnete Auswahl ist
    leer, bis der Kern sie setzt. Der Prüfweg prüft das jetzt auch. Der Browser-Test des Portals
    fand „Meine Vorgänge“ zweimal (wie oben bei Vitest) und klickt jetzt den Pfad.
  - Ganze CI grün am 2026-11-10 im zweiten Lauf (der erste fand die Befunde oben): Go und Web (121 Vitest-Fälle, 18 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,10 ms (−3 % zur Basislinie im selben Lauf), LZ2 5,15 ms (+2 %), LZ3 7,51 ms (+9 %), LZ4 11,46 ms (+6 %), LZ5 9.489 Zeilen/s, LZ13 8.502 Ereignisse/s.
- **Nicht geprüft:** Kein eigener Browser-Test. Kommt der Übergang nach der Entscheidung nicht
  durch (der Datensatz steht inzwischen woanders), bleibt die Genehmigung stehen und der Verlauf
  sagt es; zwischen dem Schreiben der Entscheidung und dem Übergang liegt ein eigener Schritt.
  Benachrichtigt werden nur die Gefragten, nicht ihre Vertretungen. Namen von Personen zeigt das
  Portal nur, wo die Person sie lesen darf. Vorlagen und Erinnerungen kommen mit WF7.

### WF15 — Benachrichtigungen als Mail, 2026-10-29

- **Grundlage:** Hauptplan E120, KF14; Inventar G160, G161; E16 dieses Plans; deine
  Entscheidungen vom 2026-10-29 (Typ `core.notification`, einfache Platzhalter, Abnehmer mit
  Job je Mail, je Person eine Mail und keine an den Auslöser).
- **Gebaut:** `packages/core/types/notification.yaml`; `internal/notify` (`notify.go`
  Prüfung, Abnehmer, Jobs; `render.go` Platzhalter; `mail.go` SMTP; `smtptest` für Tests);
  Migration `00029_mail.sql` (Fehlschlagart `mail`); `ITSM_SMTP_*` in
  `internal/platform/config.go`; Abnehmer `notify` in `internal/events`; Mailpit als
  Compose-Profil `mail` (Port 8026), `compose.yaml` stellt die Mail an Mailpit zu.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Prüfweg (`TestNotificationsOverHTTP`, Mails an einen SMTP-Empfänger im Test): Gruppe mit
    Anna (Deutsch), Ben (Englisch), einer Person ohne Adresse und der Person des Testkontos.
    Benachrichtigung bei `create` an `actors.assigned` und zusätzlich an Anna, Betreff und
    Text deutsch, englisch als Übersetzung. Ein neuer Vorgang mit der Gruppe als Bearbeiter:
    genau zwei Mails. Anna auf Deutsch („Neuer Vorgang: …“, „Art Störung“, Name des
    Auslösers, Link `…/types/probe.probe/<id>`), Ben auf Englisch („New record: …“, „kind
    Fault“); keine an den Auslöser, keine ohne Adresse, Anna trotz zweier Angaben einmal.
    Die notify-Aktion einer Regel geht als Mail mit ihrem Text („Bitte prüfen“) an die
    genannte Person.
  - Abgelehnt mit 422 am richtigen Feld: unbekanntes Ereignis, Empfänger „jemand“, Rolle, die
    der Typ nicht hat, Platzhalter auf ein fehlendes Feld, unbekannter Platzhalter.
  - Einzeln (`internal/notify`): Platzhalter mit Auswahl-Text je Sprache und Rückfall auf
    Deutsch; Prüfung von Platzhaltern und Empfängern; Mail mit Umlauten und Zeichen im
    Betreff, langen Zeilen und einer Zeile nur mit Punkt, Kopfzeilen, falsche Adresse, kein
    Server. Konfiguration (`TestLoadConfigReadsSMTP`): ohne Host aus, Host ohne Absender,
    falscher Port, Anmeldung ohne STARTTLS.
  - Angepasst: der Abdeckungstest (PL30) legt `core.notification` mit gültigen Werten und
    inaktiv an (`checkedSamples`).
  - Gefunden in der CI, beides meine Fehler: Lauf 1 fand den erzeugten Server-Code nicht
    nachgezogen (ich hatte nach der Änderung an `openapi.yaml` nur die Web-Typen erzeugt);
    Lauf 2 fand eine Beschreibung mit Komma in der YAML-Kurzschreibweise (wie schon bei WF4),
    die die OpenAPI ungültig machte; sie steht jetzt in Anführungszeichen.
  - Ganze CI grün am 2026-10-29 im dritten Lauf (die ersten beiden scheiterten an den Funden unten): Go und Web (23 Vitest-Fälle, 2 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,06 ms, LZ2 7,40 ms, LZ3 9,20 ms, LZ4 8,11 ms, LZ5 11.554 Zeilen/s, LZ13 8.996 Ereignisse/s, LZ9 92 ms; die Basislinie (Build vom 2026-10-20) ist für LZ1 bis LZ13 weiter nicht messbar (kein Login).
- **Nicht geprüft:** Mailpit selbst (Prüfweg unten von Hand); Versand gegen einen echten
  Mailserver mit STARTTLS und Anmeldung. Keine HTML-Mails, keine Mail hinein (IMAP, SD), keine
  Benachrichtigung im Browser. Gruppen in Gruppen werden nicht aufgelöst. Ohne
  `ITSM_SMTP_HOST` läuft kein Abnehmer: Ereignisse dieser Zeit werden nie nachgeschickt. Die
  Mail zeigt den Datensatz, wie er beim Versand ist, nicht wie beim Ereignis. Die Rechte
  der Empfänger werden nicht geprüft: wer benachrichtigt wird, bekommt Titel und Felder aus
  der Vorlage, auch wenn er den Datensatz nicht lesen dürfte (Vorlagen entsprechend
  schreiben; offen für SD).

### WF14 — Wiederkehrende Vorgänge, 2026-10-29

- **Grundlage:** Hauptplan E119; Inventar G61, G62; E15 dieses Plans; deine Entscheidungen vom
  2026-10-29 (Typ `core.recurrence`, einmal nachholen, wenige feste Platzhalter).
- **Gebaut:** `packages/core/types/recurrence.yaml`; `internal/recurrence` (Lauf, Prüfung,
  Vorlage); Job `recurrence.run` jede Minute; `records.Checks` für mehrere Prüfungen.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Prüfweg (`TestRecurrenceOverHTTP`, die Zeit setzt der Test): „jeden Montag um 7“ (`0 7 * * 1`,
    Europe/Berlin), Vorlage mit Titel „… KW {{week}}“ und Fälligkeit `+3d`. Sonntag, 8.
    November 2026: der erste Lauf setzt nur den nächsten Termin (Montag, 9. November, 7 Uhr
    Berlin = 6 Uhr UTC). Montag 6:59: nichts. Montag 7:00: genau ein Vorgang „… KW 46“, fällig
    am 12. November, mit Quelle `recurrence:<id>`; 7:01: nichts, nächster Termin der 16.
    Montag, 16.: zwei Läufe gleichzeitig legen zusammen genau einen an. Am 23. lief nichts;
    am Mittwoch, 25., wird er einmal nachgeholt („… KW 48“), danach steht der 30. an. Am Ende
    genau drei Vorgänge für drei Montage; der Verlauf zeigt die Termine.
  - Abgelehnt mit 422 am richtigen Feld: Zeitplan „jeden Montag“, Zeitzone „Mars/Base“,
    unbekannter Zieltyp, Vorlage ohne JSON, Vorlage ohne Pflichtfeld Titel, Vorlage mit
    unbekanntem Feld.
  - Vorlage und Zeitplan (`internal/recurrence`): alle Platzhalter, `+3d` im Datumsfeld,
    `+4h` im Zeitfeld, `+3d` in einem Textfeld bleibt Text, `+1w` über die Zeitumstellung;
    der nächste Montag 7 Uhr nach der Umstellung auf Winterzeit; falsche Zeitpläne und Zonen.
  - Angepasst: der Abdeckungstest (PL30) setzt für `core.recurrence` gültige Werte und legt
    den Vorgang inaktiv an (`checkedSamples`), wie für die SLA in WF9.
  - Ganze CI grün am 2026-10-29 im ersten Lauf: Go und Web (23 Vitest-Fälle, 2 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 2,94 ms, LZ2 7,44 ms, LZ3 8,89 ms, LZ4 10,26 ms, LZ5 11.860 Zeilen/s, LZ13 8.621 Ereignisse/s; die Basislinie (Build vom 2026-10-20) ist für LZ1 bis LZ13 weiter nicht messbar (kein Login).
- **Nicht geprüft:** Ein geänderter Zeitplan wirkt erst nach dem schon gesetzten nächsten
  Termin (der bleibt stehen). Die Vorlage kennt keine Rollen-Platzhalter und keine Felder
  anderer Datensätze. Der Job läuft jede Minute auf allen Instanzen; River sorgt dafür, dass
  ein periodischer Job einmal läuft, der Schlüssel fängt den Rest. Der laufende Container auf
  derselben Datenbank könnte zwischen Anlegen und erstem Lauf des Tests den nächsten Termin
  nach echter Zeit setzen (Fenster von Millisekunden, einmal je Minute). Keine Oberfläche
  außer Liste und Formular des Typs.

### WF13 — Aufgaben mit Zeit, Planung und Bearbeiter, 2026-10-29

- **Grundlage:** Hauptplan E118; Inventar G41; E14 dieses Plans; deine Entscheidungen vom
  2026-10-29 (eigener Typ je Paket, Summenfeld beim Schreiben gepflegt).
- **Gebaut:** Feldart `rollup` (`internal/meta/model.go`, `rollup.go`, Lader in `load.go`);
  Pflege in `internal/records/rollup.go` und in den Schreibwegen; im Probe-Paket der Typ
  `probe.task` (Titel, Vorgang, Minuten, Termin von/bis, Status offen/erledigt, Rolle
  Bearbeiter) und an `probe.probe` die Summen `task_minutes` und `tasks_open`.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Prüfweg (`TestTasksOverHTTP`): drei Aufgaben mit 30, 45 und 60 Minuten, am Vorgang 135
    Minuten und 3 offene; eine erledigt: 2 offen; Zeit von 45 auf 50: 140; eine Aufgabe an
    einen anderen Vorgang: dort 60 und 1, hier 80 und 1; eine im Papierkorb: 30 und 0,
    wiederhergestellt: 80 und 1. Der Vorgang bleibt bei Version 1, sein Verlauf zeigt beide
    Summen; eine Summe zu schreiben gibt 422. Die Aufgabe mit Termin am 10. November findet
    eine Abfrage auf die Woche (`:query` mit `planned_start` gte/lt), so liest UI15 den
    Kalender. Zehn Aufgaben gleichzeitig: 70 Minuten und 11 offen, keine geht verloren.
  - Prüfung beim Laden (`TestRollupValidation`): fremdes Paket, fehlender Typ, `via` ohne
    Referenz auf den Typ, Summe über ein Textfeld, Anzahl mit Feld, unbekannte Operation,
    Bedingung, die CEL nicht versteht; das Summenfeld ist berechnet und nicht schreibbar.
  - Der Abdeckungstest (PL30) prüft `probe.task` gegen die OpenAPI.
  - Gefunden im ersten CI-Lauf, ein Leistungsfehler meines Codes: `TestBulkStreamOverHTTP` brach
    nach 60 Sekunden ab (rund 1.600 statt 16.000 Zeilen/s). Die erste Fassung fragte bei jeder
    Bulk-Zeile und jedem Schreiben jedes Typs alle Typdefinitionen aus der Datenbank ab, um
    zu wissen, ob ein Summenfeld den Typ zählt. Jetzt schreibt der Lader an den gezählten Typ,
    wer ihn zählt (`Type.CountedBy`); die Frage kostet nichts mehr, Typen werden nur beim
    Schreiben eines gezählten Datensatzes geladen. Danach 100.000 Zeilen in 6,3 Sekunden.
  - Ganze CI grün am 2026-10-29 im zweiten Lauf (der erste fand den Leistungsfehler oben): Go und Web (23 Vitest-Fälle, 2 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,00 ms, LZ2 7,28 ms, LZ3 8,37 ms, LZ4 7,58 ms, LZ5 11.228 Zeilen/s, LZ13 9.856 Ereignisse/s; die Basislinie (Build vom 2026-10-20) ist für LZ1 bis LZ13 weiter nicht messbar (kein Login).
- **Nicht geprüft:** Ein Summenfeld über einen Typ eines anderen Pakets geht nicht (bewusst,
  E14). Eine Bedingung, die CEL versteht, aber nicht nach SQL geht, fällt erst beim ersten
  Schreiben einer Aufgabe auf. Wird ein Summenfeld neu in einen Typ aufgenommen, stehen die
  vorhandenen Vorgänge auf 0, bis eine ihrer Aufgaben geschrieben wird (kein Nachrechnen beim
  Installieren). Die Kalenderansicht selbst ist UI15; „Meine Aufgaben“ über `@me` und
  `@my_groups` (WF3) ist nicht eigens getestet. Kein Aufgabentyp im Kernpaket; jeder
  Fachplan (SD, Change, Problem) legt seinen an.

### WF12 — Zeitregeln im Statusmodell, 2026-10-29

- **Grundlage:** Hauptplan E117; Inventar G63, G64; E13 dieses Plans; deine Entscheidungen vom
  2026-10-29 (als SLA mit Stufen, Zeitbasis wählbar je SLA, Warnung als Benachrichtigung an
  die Bearbeiter).
- **Gebaut:** Feld `counts` in `packages/core/types/sla.yaml`; in `internal/sla` keine
  Kalender bei `round_the_clock`; Prüfweg `TestTimeRulesOverHTTP`.
- **Anleitung** (zwei SLAs, zwei Stufen; Werte für den Probe-Typ):
  - Warnung nach 3 Tagen Laufzeit: SLA `target_type` = Typ, `target_minutes` 4320,
    `counts` `round_the_clock`, `stop` `record.state in ["resolved", "closed"]`; Regel
    `{"trigger": "clock", "mode": "after", "sla": "<id>", "percent": 100, "actions":
    [{"notify": {"to": ["actors.assigned"], "message": "Vorgang läuft zu lange"}}]}`.
  - Schließen 7 Tage nach der Lösung: SLA `target_minutes` 10080, `counts`
    `round_the_clock`, `start_from` `applies`, `applies` `record.state == "resolved"`, `stop`
    `false`; Regel mit `"percent": 100` und `[{"transition": {"name": "close"}}]`. Wird der
    Datensatz vorher wieder geöffnet, endet die Uhr; die nächste Lösung zählt neu.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Prüfweg (die Zeit der Uhren setzt der Test): Die Einheit `root.wf12` hat einen Kalender
    Montag bis Freitag 8–16 Uhr. Angelegt am Freitag, 6. November 2026, 15 Uhr: die Uhr der
    Warnung hat keinen Kalender, ihr Job steht auf Montag 15 Uhr (drei Tage, nicht drei
    Arbeitstage). Danach benachrichtigt die Stufe die Bearbeitergruppe mit „Vorgang läuft zu
    lange“. Gelöst an Tag 4: die Uhr der Warnung ist gestoppt, die Uhr zum Schließen zählt ab
    der Lösung, ihr Job steht sieben Tage später; danach ist der Datensatz geschlossen und die
    Uhr abgebrochen.
  - Ganze CI grün am 2026-10-29 im ersten Lauf: Go und Web (23 Vitest-Fälle, 2 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 2,64 ms, LZ2 5,70 ms, LZ3 8,34 ms, LZ4 7,49 ms, LZ5 13.207 Zeilen/s, LZ13 9.932 Ereignisse/s; die Basislinie (Build vom 2026-10-20) ist für LZ1 bis LZ13 weiter nicht messbar (kein Login).
- **Nicht geprüft:** Keine Oberfläche, die beides in einem Schritt anlegt (UI Block B). Eine
  Warnung an jemand anderen als die Bearbeiter ist eine andere Empfängerangabe an der Stufe,
  nicht eigens getestet. Kein Kennzeichen „zu lange offen“ am Datensatz; Listen können nur
  über den Verlauf oder `…/clocks` danach sehen.

### WF11 — Wartegründe mit Erinnerung und Lösung nach Zeit, 2026-10-29

- **Grundlage:** Hauptplan E116; Inventar G54; E12 dieses Plans; deine Entscheidungen vom
  2026-10-29 (Uhren und Stufen nutzen, Typ `core.wait_reason`, Erinnerung als
  Benachrichtigung an den Anforderer).
- **Gebaut:** `packages/core/types/wait_reason.yaml`; Feld `start_from` in
  `packages/core/types/sla.yaml`; Migration `00028_wait.sql` (`rule.minutes`); in
  `internal/rules/engine` Stufen in Minuten und die Aktion `transition`; in `internal/sla`
  der Start ab Geltung und die Sortierung der Stufen; im Probe-Typ `wait_reason` und
  `resolve` aus „Wartet“; API: `minutes` an der Regel, `transition` als Aktion.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Prüfweg (`TestWaitReasonOverHTTP`, die Zeit der Uhren setzt der Test): Wartegrund
    „Rückmeldung Nutzer“, SLA 7 Tage ab Beginn des Wartens, Erinnerungen nach 2 und 5 Tagen
    an den Anforderer, Lösung bei 100 % mit „Ohne Rückmeldung nach 7 Tagen gelöst“. Angelegt
    um T0, wartet ab T0+1h: die Uhr zählt ab T0+1h, der erste Job steht zwei Tage später.
    Nach 2 Tagen steht „Bitte melden Sie sich (1)“ im Verlauf, an die Person des Anforderers,
    der nächste Job steht auf 5 Tage; nach 5 Tagen die zweite Erinnerung, der nächste Job auf
    7 Tage; nach 7 Tagen ist der Datensatz gelöst mit dem Text, die Uhr abgebrochen.
  - Neu beginnen: drei Tage gewartet (eine Erinnerung), fortgesetzt (Uhr abgebrochen), einen
    Tag später wieder gewartet: die Uhr zählt ab dann, ohne Stufen, die erste Erinnerung
    wieder zwei Tage später.
  - Abgelehnt mit 422: Stufe mit Prozent und Minuten zugleich, ohne beides, Übergang, den der
    Typ nicht hat.
  - Rechnen (`TestClockLevelsInMinutes`): Stufen nach 2 und 5 Tagen neben 100 % einer Woche.
  - Der Abdeckungstest (PL30) prüft `core.wait_reason` gegen die OpenAPI.
  - Angepasst, gefunden im ersten CI-Lauf: `TestLoadProbePackageFromRepository` prüfte den
    Probe-Typ gegen eine feste Liste installierter Kerntypen (Person, Gruppe); das neue Feld
    `wait_reason` zeigt auf `core.wait_reason`. Der Test lädt jetzt das Kernpaket aus dem
    Repository als installierte Typen, statt die Liste nachzuführen.
  - CI am 2026-10-29 in drei Teilen grün für den Endstand: Lauf 1 scheiterte am Meta-Test (oben). Lauf 2: alle Go-Tests grün, aber das Leistungsgate fiel (LZ4 p95 41 ms, LZ13 3.791 Ereignisse/s, innerhalb des Laufs immer schlechter) unter fremder Last auf dem Rechner (Load um 8, viele andere Container); die Laststufe allein mit demselben Stand war grün: LZ1 p95 2,67 ms, LZ2 10,87 ms, LZ3 8,38 ms, LZ4 7,76 ms, LZ5 10.124 Zeilen/s, LZ13 9.887 Ereignisse/s, Gate bestanden. Die Web-Schritte danach allein: der Browser-Test `records.spec.ts` fand mit `getByLabel("Art")` auch „Wartegrund“ (Playwright sucht ohne exact nach Teilwörtern); er sucht jetzt exakt. Danach Web grün (23 Vitest-Fälle, 2 Playwright-Fälle mit axe).
- **Nicht geprüft:** Die Erinnerung ist eine Benachrichtigung (Verlauf, Ereignis), keine
  Antwort am Ticket und noch keine Mail (WF15, SD). Ein Wartegrund ohne SLA tut nichts; die
  Verbindung von Grund und Erinnerungen steht in der Bedingung der SLA mit der Id des Grundes
  (eine Oberfläche, die das zusammen anlegt, fehlt). Erinnerungen hängen am Kalender der SLA
  (ohne Kalender der Einheit, sonst rund um die Uhr).

### WF10 — Eskalationsstufen, 2026-10-29

- **Grundlage:** Hauptplan E115, KF5, KF7; Inventar G66, G168; E11 dieses Plans; deine
  Entscheidungen vom 2026-10-29 (Stufen als Regeln, 1–1000 % einmal je Uhr, Nachweis in
  Verlauf und Ereignis).
- **Gebaut:** Migration `00027_escalation.sql` (`rule.sla`, `rule.percent`, Auslöser
  `clock`, `clock.levels`, Verlaufsaktion `escalation`); in `internal/rules/engine`
  Prüfung, `Levels` und der Job aus dem Ereignis; in `internal/sla` Stufen erreichen und den
  Job auf die nächste Schwelle planen (`clock.go`, `settle` in `sla.go`, `Due` in `jobs.go`);
  API: `sla` und `percent` an der Regel, `levels` an der Uhr.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Prüfweg (`TestEscalationOverHTTP`, die Zeit der Uhren setzt der Test): SLA 60 Minuten,
    Stufe 1 bei 80 % setzt die Priorität auf 1, Stufe 2 bei 100 % benachrichtigt die Gruppe
    „Leitung“, Stufe 3 bei 150 % ebenso. Der erste Job der neuen Uhr steht auf T0+48 (80 %),
    nicht auf der Frist. Um T0+49 feuert Stufe 1, der Job der Regel setzt Priorität 1, der
    nächste Job steht auf T0+60. Um T0+61 ist die Uhr verletzt, Stufe 2 feuert und die
    Benachrichtigung „Frist verletzt“ steht im Verlauf, der nächste Job wartet auf T0+90
    (150 %). Im Verlauf genau zwei Einträge `escalation` (je Stufe einer) und ein `breach`;
    `…/clocks` zeigt `levels: [wf10_s1, wf10_s2]`. Ein alter Job tut nichts.
  - Abgelehnt mit 422: Anteil 0 und 1001, Stufe „before“, SLA eines anderen Typs, SLA an
    einer Regel ohne Auslöser `clock`.
  - Rechnen (`TestClockLevels`): erste Schwelle 80 %, jede Stufe einmal, nach Verstoß und
    Pause nur noch Stufe 3 (nach der Pause später), nichts mehr offen ohne Job, Neustart
    erreicht die Stufen neu.
  - Ganze CI grün am 2026-10-29 im ersten Lauf: Go und Web (23 Vitest-Fälle, 2 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 2,58 ms, LZ2 5,39 ms, LZ3 7,35 ms, LZ4 7,37 ms (mit zehn Regeln), LZ5 11.325 Zeilen/s, LZ13 8.792 Ereignisse/s; die Basislinie (Build vom 2026-10-20) ist für LZ1 bis LZ13 weiter nicht messbar (kein Login).
- **Nicht geprüft:** Eine Stufe, die angelegt wird, während Uhren schon laufen, wird bei
  diesen erst zum nächsten schon geplanten Job (spätestens der Frist) bemerkt und dann
  nachgeholt. Stufen mehrerer Instanzen: die Regeln sind bis zu 30 Sekunden im Speicher, über
  Valkey geleert. Die Benachrichtigung ist ein Eintrag im Verlauf und ein Ereignis; die Mail
  kommt mit WF15. „Die Leitung“ ist eine Gruppe oder eine Rolle am Datensatz, eine Hierarchie
  (Vorgesetzte der Bearbeiter) gibt es nicht. Keine Oberfläche für Stufen außer der Regel-API.

### WF9 — Fristen und SLA-Uhren, 2026-10-29

- **Grundlage:** Hauptplan E114, KF5; Inventar G167; E10 dieses Plans; Architektur §8; deine
  Entscheidungen vom 2026-10-28 und 2026-10-29 (SLA als Typ `core.sla`, Uhren in einer
  eigenen Tabelle, Bewegung beim Schreiben, CEL-Bedingungen; Umstufung zählt ab Anlage;
  im Papierkorb halten die Uhren an).
- **Gebaut:** `packages/core/types/sla.yaml`; Migration `00026_clock.sql` (Tabelle `clock`,
  Aktion `breach` im Verlauf); `internal/sla` (`clock.go` Rechnen, `sla.go` SLAs und Haken,
  `jobs.go` Job `sla.due` und Lesen); Haken in `internal/records/clockhook.go` und in den
  Schreibwegen; API `GET /api/v1/records/{type}/{id}/clocks`; im Probe-Typ der Status
  „Wartet“ mit den Übergängen `wait` und `resume`.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Prüfweg (`TestSLAOverHTTP`, die Zeit der Uhren setzt der Test): SLA „A“ mit 60 Minuten
    rund um die Uhr. Die Uhr startet beim Anlegen (Frist T0+60), pausiert im Status „Wartet“
    von T0+20 bis T0+50 (20 Minuten gezählt, 40 übrig, keine Frist), nach dem Fortsetzen
    sind um T0+60 noch 30 Minuten übrig, die Frist ist T0+90. Der Job des ersten Laufs tut
    nichts, der aktuelle markiert um T0+91 den Verstoß zum Zeitpunkt T0+90 (−1 Minute übrig),
    der Verlauf zeigt ihn. Die Lösung um T0+95 stoppt die Uhr nach 65 Minuten, sie bleibt
    verletzt. Zwei Jobs für zwei Läufe.
  - Umstufung: P3 mit 480 Minuten, P1 mit 60 Minuten; nach 90 Minuten auf P1 gesetzt, wird
    die P3-Uhr abgebrochen (90 Minuten), die P1-Uhr zählt ab Anlage und ist sofort verletzt
    (Zeitpunkt Anlage + 60). Papierkorb von +100 bis +130: um +140 sind es 110 Minuten.
  - Kalender Montag bis Freitag 8–16 Uhr: 120 Minuten ab Freitag, 6. November 2026, 15 Uhr
    enden am Montag um 9 Uhr; am Samstagmittag sind noch 60 Minuten übrig.
  - Eine SLA mit einer Bedingung, die sich nicht übersetzen lässt, oder für einen fehlenden
    Typ: 422 mit dem Feld. Nach dem endgültigen Löschen sind die Uhren weg.
  - Rechnen (`internal/sla`): Pause und Fortsetzen, gleicher Zustand ohne Änderung, Stopp
    nach der Frist vor dem Job (Verstoß zur Frist), Wiederöffnen läuft weiter, später Start
    sofort verletzt, Start in der Pause, Kalender mit Feiertag und Pause übers Wochenende.
  - Angepasst, gefunden im ersten CI-Lauf: `TestStateModelOverHTTP` erwartet 7 statt 5
    Übergänge am neuen Datensatz und findet „reopen“ an fünfter statt dritter Stelle, weil der
    Probe-Typ für den Prüfweg „Warten“ und „Fortsetzen“ bekommen hat. Der Abdeckungstest
    (PL30) füllte Textfelder mit Zufallstext; die neue Prüfung von `core.sla` lehnt das zu
    Recht ab. Er setzt für diese Felder jetzt gültige Werte (`checkedSamples`: Zieltyp
    probe.probe, Bedingungen, die nie gelten).
  - Ganze CI grün am 2026-10-29 (zweiter Lauf; der erste scheiterte an den zwei Tests oben): Go und Web (23 Vitest-Fälle, 2 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 2,43 ms, LZ2 6,38 ms, LZ3 6,79 ms, LZ4 6,64 ms (mit zehn Regeln; der Haken der Uhren kostet bei Typen ohne SLA nichts Messbares), LZ5 10.107 Zeilen/s, LZ13 9.726 Ereignisse/s; die Basislinie (Build vom 2026-10-20) ist für LZ1 bis LZ13 weiter nicht messbar (kein Login).
- **Nicht geprüft:** 100.000 offene Uhren sind nicht gemessen (Messung in H11); es gibt
  keinen periodischen Job über alle Uhren, je laufender Uhr einen Job zur Frist. Pausen vor
  dem späten Start einer SLA sind nicht bekannt und zählen mit. Ein Feiertag, der nach dem
  Planen eine Frist früher macht (Feiertag entfernt), wird erst zur alten Frist bemerkt. Bulk
  mit SLAs geht Zeile für Zeile (nicht gemessen). Kein Zwischenspeicher für Kalender: jede
  Bewegung einer Uhr lädt ihren Kalender. Keine Oberfläche für Uhren; SLAs über Liste und
  Formular von `core.sla`. Eskalation erst mit WF10.

### WF8 — Kalender mit Arbeitszeiten und Feiertagen, 2026-10-28

- **Grundlage:** Hauptplan E113, E16; Inventar G169; E9 dieses Plans; deine Entscheidungen vom
  2026-10-28 (Typen im Kernpaket, Feiertage von Hand und aus dem Generator, Kalender je Knoten
  über eine Einstellung).
- **Gebaut:** `packages/core/types/calendar.yaml`, `calendar_hours.yaml`,
  `calendar_holiday.yaml`; `internal/calendar` (`calendar.go`, `holidays.go`, `service.go`);
  API `POST /api/v1/calendars/{id}:add`, `:between`, `:holidays`
  (`internal/httpapi/calendars.go`); Befehl `itsm calendar holidays <id> <Land> <Jahr>`.
- **Belegt, 2026-10-28 im Werkzeug-Container:**
  - Prüfweg (`TestCalendarsOverHTTP`): Kalender „Büro“ Montag bis Freitag 8–16 Uhr, Zeitzone
    Europe/Berlin von selbst; 8 Stunden ab Freitag, 6. November 2026, 15 Uhr enden am Montag
    um 15 Uhr; mit einem Feiertag am Montag am Dienstag um 15 Uhr. Zwischen Freitag 15 Uhr und
    Dienstag 9:30 Uhr liegen über den Feiertag 150 Minuten. Die Feiertage von NRW 2026 kommen
    mit einem Aufruf (11 Tage), der zweite Aufruf fügt nichts hinzu; danach enden 2 Stunden ab
    Heiligabend 15 Uhr am Montag, 28. Dezember, um 9 Uhr. Die Einstellung `calendar.default`
    an `root.wf8` gibt der Einheit den Kalender, ohne Einstellung keinen. Eine Arbeitszeit
    „8“ statt „08:00“ führt beim Rechnen zu 422, ein unbekannter Kalender zu 404.
  - Rechnen (`internal/calendar`): Beginn vor Arbeitsbeginn, Ende genau zum Feierabend, eine
    Minute mehr am nächsten Tag, Sonntag, Dauer null, Beginn in UTC, Mittagspause (auch
    unsortiert eingetragen), Umstellung auf Winterzeit, kein Arbeitstag, verkehrte Spanne;
    `Between` rückwärts null; Uhrzeiten wie „8:30“ und „24:00“ gültig, „25:00“, „08:60“, „8“
    nicht; Ostern 2024 bis 2027 und 2038, NRW 2026 mit 11 Feiertagen (Karfreitag,
    Fronleichnam, Allerheiligen), Sachsen mit Buß- und Bettag und Reformationstag, Berlin mit
    Frauentag, 2017 der Reformationstag überall, ein unbekanntes Land abgelehnt.
  - Der Abdeckungstest (PL30) prüft die drei neuen Typen gegen die OpenAPI.
  - Gefunden in der CI: Der Linter wollte den Import der eingebetteten Zeitzonen nicht in der
    Gruppe der Standardpakete; er steht jetzt in `internal/calendar/tzdata.go`. Und
    `TestWebhooksOverHTTP` erwartete eine Zustellung im Zustand „available“; seit PL41 nimmt der
    Verteiler des laufenden Containers (gemeinsame Datenbank) das Testereignis selbst auf und
    stellt schon zu. Der Test prüft jetzt genau eine Zustellung mit 8 Versuchen zum Ereignis,
    gleich in welchem Zustand.
  - Doku nachgezogen auf deine Frage vom 2026-10-28: `architektur.md` (Bausteine
    `internal/rules/engine` und `internal/calendar`, §7 Verteiler in Stapeln, §8 Rollen, Regeln,
    Feldzustände, Kalender), README (Stand, Befehle), UI-Plan (Formular mit Feldzuständen aus
    WF2, offene Oberflächen für WF3, WF4, WF8).
  - Ganze CI grün am 2026-10-28 (vierter Lauf; Lauf 1 Lint, Lauf 2 und 3 der Webhook-Test, beides oben): Go und Web (23 Vitest-Fälle, 2 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,22 ms, LZ2 6,24 ms, LZ3 9,04 ms, LZ4 9,01 ms (mit zehn Regeln), LZ5 10.164 Zeilen/s, LZ13 8.966 Ereignisse/s; die Basislinie (Build vom 2026-10-20) ist für LZ1 bis LZ13 weiter nicht messbar (kein Login).
- **Nicht geprüft:** Eine Arbeitszeit wie „8“ wird erst beim Rechnen abgelehnt, nicht beim
  Speichern (Textfelder haben kein Muster). Doppelte Feiertage von Hand (zwei Einträge am selben
  Tag) stören nicht, werden aber nicht verhindert. Kalender werden bei jeder Rechnung neu
  geladen, ohne Zwischenspeicher (für viele Uhren in WF9 zu prüfen). Keine Oberfläche außer
  Liste und Formular der drei Typen; der Generator ist nur über API und Befehl erreichbar.
  Tage, die nur einzelne Gemeinden halten, kommen nicht aus dem Generator.

### WF4 — Regel-Engine, 2026-10-28

- **Grundlage:** Hauptplan E111, KF7, LZ4; Architektur B4, §6.2; Inventar G57; E8 dieses Plans;
  deine Entscheidungen vom 2026-10-28 (Kerntabelle mit API, benachrichtigen als Ereignis,
  Zeitplan mit Bedingung in SQL, Systemrechte mit der Regel im Verlauf).
- **Gebaut:** Migration 00024 (Tabelle `rule`, Spalte `audit.rule`, Aktion `notify`),
  `db/queries/rule.sql`; `rules.CompileRule`/`EvalRule`; `internal/rules/engine` (`engine.go`:
  Regeln, Prüfung, Zwischenspeicher, `Before`; `jobs.go`: `Handle`, `rules.apply`,
  `rules.time`, `RunSchedules`); in `records` der Haken `Before`, `WithRule`, `CheckValues`,
  `Matching`, Regelname im Verlauf und Regel und Tiefe im Ereignis; API `/api/v1/rules` und
  `rule` im Verlauf; Verdrahtung in `main.go` und im Testserver (eigene Queue, eigener
  Verteiler); LZ4 mit zehn Regeln im Lastwerkzeug.
- **Belegt, 2026-10-28 im Werkzeug-Container:**
  - Prüfweg (`TestRulesOverHTTP`): Regel „Art Störung → Gruppe Netz, Priorität 2“ vor dem
    Schreiben: Das Anlegen antwortet mit der Gruppe als Bearbeiter und Priorität 2; ein Wunsch
    bleibt unberührt. Eine Regel danach (Gruppe gesetzt → Kennzeichen „NETZ“ und
    Benachrichtigung an die Bearbeiter) läuft als Job, ihre Änderung löst die nächste Regel
    aus (Fälligkeit); der Datensatz steht danach auf Version 3. Der Verlauf nennt je Änderung
    die Regel (`wf4_netz`, `wf4_follow`, `wf4_chain`) und die Benachrichtigung mit Text, die
    Änderungen der Person tragen keine. Ein Ereignis in Tiefe 5 stellt keinen Job ein. Eine
    Regel nach Zeitplan findet den Wunsch über ihre Bedingung in SQL und setzt die Priorität.
    Falsche Regeln: Bedingung ohne gültige Syntax, benachrichtigen vor dem Schreiben,
    unbekanntes Feld, Aktion ohne Inhalt, Zeitplan fehlt → 422 mit allen Problemen; Lesen,
    Ersetzen (Version 2, abgeschaltet), Liste, unbekannte Regel 404.
  - CEL (`TestRuleConditionSeesChanged`): `changed` ist in Regeln bekannt, in Übergängen nicht.
  - LZ4 mit zehn Regeln hält das Ziel; Zahlen in der CI-Zeile.
  - `scripts/ci.sh` komplett grün (Go, Lizenzen, Last mit Gate: LZ1 p95 2,40 ms, LZ2 4,57 ms, LZ3 7,30 ms, LZ4 mit zehn Regeln 9,45 ms, vorher ohne Regeln um 6 ms (WF2: 6,10 ms), Ziel 100 ms; LZ5 11.298 Zeilen/s; Web mit 23 Vitest- und 2 Browser-Tests, LZ9 115 ms). Der erste Lauf scheiterte am Abdeckungstest: eine Beschreibung mit Komma in der YAML-Kurzschreibweise (wie bei PL30), jetzt in Anführungszeichen.
- **Nicht geprüft:** Keine Oberfläche für Regeln (Regel-Editor in Block B der Oberfläche
  oder später). Werte können nur fest gesetzt werden, nicht aus anderen Feldern berechnet;
  „anlegen“ verknüpft den neuen Datensatz nicht mit dem auslösenden. Benachrichtigungen gehen
  noch nirgends hin außer Ereignisstrom und Webhooks (WF15). Die Rechte `read` und
  `manage_rules` auf `core.rule` sind nur mit Admin belegt, nicht mit einer eingeschränkten
  Rolle. Läuft eine Regel nach Zeitplan auf zwei Instanzen gleichzeitig (der Job ist ein
  River-Zeitplan, also eine Instanz), ist nicht geprüft. Bulk mit Regeln vor dem Schreiben
  läuft zeilenweise. Mehrere Instanzen mit unterschiedlichem Zwischenspeicher während einer
  Regeländerung (bis 30 s ohne Valkey) nicht gemessen.
- **Befund beim Rauchtest (2026-10-28):** Im Container wirkte die Regel danach erst nach Minuten,
  weil sein Verteiler rund 37.000 Ereignisse der CI-Läufe (gemeinsame Datenbank) aufzuholen
  hatte; er schafft rund 375 Ereignisse/s. Ein Bulk-Lauf mit 100.000 Datensätzen verzögert
  Webhooks und Regeln danach also um gut vier Minuten. Aufgeholt wirkte die Regel nach 1 s. Der
  Verteiler arbeitet jedes Ereignis einzeln ab (Valkey-Publish, Webhooks mit eigener Abfrage,
  Regeln); ob er schneller werden muss, ist eine Frage an dich. **Entschieden am 2026-10-28:**
  PL41 im Kern-Plan, gebaut vor WF8 (Hauptplan E112).
- **Nachtrag SD16d (2026-11-04, Hauptplan E172):** Bedingungen kennen `today()` und `now()` mit
  Abstand, auch die von Regeln nach Zeitplan in SQL; „Rolle zuweisen“ nimmt mit `actor: true`
  die Person, die schreibt, dazu (vor dem Schreiben aus der Anfrage). Belegt im SD-Plan, §4
  SD16d.

### WF2 — Feldzustände je Status und Formularvariante, 2026-10-28

- **Grundlage:** Hauptplan E110, E29; Architektur B5, §8; Inventar G53; E7 dieses Plans; deine
  Entscheidungen vom 2026-10-28 (Regeln mit Status und CEL, versteckt nur im Formular, Felder
  im selben Übergang, Formular schon jetzt minimal).
- **Gebaut:** `meta.FieldStateRule` und `Type.FieldStates` mit Prüfung; `internal/records/
  fieldstates.go`; Anlegen, Ändern und Bulk prüfen die Regeln; `Transition` mit Feldern und
  Version über `update`; API: `fields` und `If-Match` am Übergang, `POST …:field-states`
  (auch im Schema je Typ); Oberfläche: `fieldStatesQuery`, `Form` mit Feldzuständen, Texte
  `form.locked` und `form.rule.read_only`; `probe.probe` mit Lösung, „Gelöst“, „Geschlossen“
  und vier Regeln (gelöst und geschlossen brauchen die Lösung, geschlossen ist gesperrt, ein
  Wunsch blendet Auswirkung und Dringlichkeit aus und startet mit Priorität 4).
- **Belegt, 2026-10-28 im Werkzeug-Container:**
  - Prüfweg (`TestFieldStatesOverHTTP`): `resolve` ohne Lösung → 422 mit `solution` und
    `required`; mit `fields` ohne `If-Match` → 428; mit `If-Match: "2"` → 200, Status
    `resolved`, Lösung gesetzt, Version 3, ETag `"3"`; nach `close` ist ein PATCH 422
    `read_only`, `:field-states` meldet `locked` und alle Felder schreibgeschützt; ein
    Wunsch-Entwurf blendet drei Felder aus und belegt Priorität 4 vor, ein angelegter Wunsch hat
    sie; der Verlauf zeigt in Version 3 `transition` an `state` und `update` an `solution`.
  - Kern (`TestFieldStates`, eigener Testtyp): schreibgeschützt beim Anlegen (außer mit dem
    vorbelegten Wert), Pflichtfeld je Variante, Vorbelegung nur bei Wunsch; Änderung eines
    schreibgeschützten Felds abgelehnt, sein unveränderter Wert darf mit; Übergang ohne
    Pflichtfeld 422, mit veralteter Version 409 (Konflikt), Bedingung sieht die neuen Felder;
    Pflichtfeld im Zielstatus nicht leerbar; geschlossen sperrt Felder und Rollen, Felder mit
    einem Übergang heraus ebenso, der Übergang allein geht; Feldzustände für Entwürfe je
    Variante; im Mengenweg des Bulk gelten dieselben Regeln.
  - Definition (`TestFieldStatesInTheDefinition`): unbekannter Status oder Feld, `*` außerhalb
    von `read_only` oder nicht allein, Pflicht und versteckt zugleich, Bedingung ohne gültige
    Syntax, falsche Vorbelegung, Regel ohne Aussage werden abgelehnt.
  - Oberfläche (Vitest): neuer Datensatz mit verstecktem, Pflicht-, schreibgeschütztem und
    vorbelegtem Feld, Pflichtmeldung, das schreibgeschützte Feld wird nicht geschickt; ein
    geschlossener Datensatz zeigt den Hinweis, Felder nur lesbar und keinen Knopf „Speichern“;
    axe ohne Befund. Die bestehenden Übergangs-Tests (WF1) laufen unverändert über den neuen
    Schreibweg.
  - Gefunden in der CI: Nach vielen Läufen hintereinander scheiterten 14 HTTP-Tests an der
    Anmeldung (401). Das Ereignisprotokoll (PL27) nannte den Grund `locked`: Die Fehlversuche
    aller Tests von 127.0.0.1 zählen in derselben Valkey-Instanz, die Tests des Pakets `auth`
    mit eingeschalteter Sperre; nach fünf im Zeitfenster war die Adresse gesperrt. Der
    Testserver hebt jetzt beim Start die Sperre seines Kontos und der Adresse auf; dafür hat
    `auth` neben `Unlock` ein `UnlockAddress`.
  - `scripts/ci.sh` komplett grün (Go, Lizenzen, Last mit Gate: LZ1 p95 2,46 ms, LZ2 6,57 ms, LZ3 6,13 ms, LZ4 6,10 ms, LZ5 11.696 Zeilen/s; Web mit 23 Vitest- und 2 Browser-Tests, LZ9 131 ms).
- **Nicht geprüft:** Das Formular fragt die Feldzustände beim Laden; wechselt man im neuen
  Formular die Art, zieht es die Variante nicht nach (der Server prüft beim Speichern). Die
  Übergangsknöpfe fragen nicht nach Pflichtfeldern des Zielstatus; der Server antwortet mit 422,
  der Knopf zeigt die Meldung. Rollen können nicht Pflicht sein und nicht einzeln
  schreibgeschützt (nur mit `*`). Vorbelegung gilt nur beim Anlegen, nicht beim Eintritt in
  einen Status. Ein Feld, das der Typ als Pflicht führt und eine Regel versteckt, kann im
  Formular nicht gefüllt werden; die Prüfung der Definition fängt nur den Widerspruch innerhalb
  einer Regel ab.

### WF3 — Rollen am Datensatz, 2026-10-28

- **Grundlage:** Hauptplan E109, E81, E108; Architektur B1, §5; Inventar G37, G79; E6 dieses
  Plans; deine Entscheidungen vom 2026-10-28 (Kerntabelle mit Merkmal, Haltertypen je Rolle,
  neue Reichweite `involved`, Rollen im Datensatz mit Version).
- **Gebaut:** Migration 00023 (Tabelle `actor`, Reichweite `involved` in `permission`),
  Abfragen `db/queries/actor.sql`; `meta.ActorRole`, `Type.Actors` und das Kern-Merkmal
  `actors` mit Prüfung; `internal/records/actors.go` (Lesen als Spalte, Prüfen, Schreiben,
  Filter), eingebunden in Anlegen, Ändern, Löschen, Liste, JSON-Abfrage, Versionen,
  Feldrechte und Bulk; `authz` mit `ScopeInvolved`, `Access.Groups`, `Me`, `MyGroups`; das
  Schema je Typ in der OpenAPI hat `actors`; `probe.probe` mit drei Rollen, `probe` hängt von
  `core` ab; der Abdeckungstest (PL30) setzt je Rolle einen Halter.
- **Belegt, 2026-10-28 im Werkzeug-Container:**
  - Prüfweg (`TestRolesAtTheRecordOverHTTP`): Ein Probe-Datensatz mit der Gruppe „Netz“ als
    Bearbeiter und Ben als Anforderer. Anna (Mitglied von „Netz“, Rolle mit `involved`) findet
    ihn unter `filter[actors.assigned]=@my_groups` und per JSON-Abfrage; Ben findet ihn dort
    nicht, liest ihn aber als Anforderer. Anna leert den Anforderer (Version 2), danach ist der
    Datensatz für Ben 404. Die Rolle `vendor` (gibt es an `probe.probe` nicht) und
    `probe.color` als Beobachter → 422 mit `actors.vendor` und `actors.observer`. Der Verlauf
    nennt `actors.requester` und `actors.assigned`; Typ und OpenAPI zeigen die Rollen und
    `involved`.
  - Kern (`TestRolesAtTheRecord`, Testpaket mit Person, Team und Lieferant als Haltern): jede
    Rolle in der Antwort, Halter sortiert, doppelte entfernt, Ereignis nennt die gesetzten
    Rollen. Abgelehnt werden die Rolle eines anderen Typs (`unknown`), ein falscher Haltertyp
    (`type`), eine unbekannte Id (`reference`), zwei Halter bei `single` und `actors` ohne
    Merkmal. Ändern: neue Version mit Verlaufszeile alt/neu; dieselben Halter noch einmal ändern
    nichts; `null` leert; der Lieferant kommt dazu. Version 1 zeigt die ersten Halter, der
    Vergleich 1..3 nennt die drei Rollen, Wiederherstellen von Version 1 bringt sie als Version
    4 zurück. Filter je Rolle, über jede Rolle mit Komma, JSON-Abfrage mit `null` und `in`;
    `involved` und `@me`/`@my_groups` mit einer Person und einer Gruppe; ohne Feldrecht
    `actors` sind die Rollen unsichtbar und nicht schreibbar; nach dem endgültigen Löschen ist
    `actor` leer.
  - Definition (`TestRolesInTheDefinition`): Merkmal ohne Rolle, Rolle ohne Merkmal, unbekannter
    Haltertyp, leeres `to`, doppelte Rolle werden abgelehnt.
  - Der Abdeckungstest (PL30) legt `probe.probe` mit je einem Halter pro Rolle an und prüft die
    Antwort gegen das Schema mit `actors`.
  - Gefunden in der CI: Der lokale Container auf derselben Datenbank kennt seit PL32
    `bulk.apply` und nahm die LZ5-Jobs der Laststufe, ohne deren Strom in seinem Dateispeicher;
    die Jobs scheiterten. Neu ist `ITSM_BULK_QUEUE` (Vorgabe `bulk`, deine Entscheidung vom
    2026-10-28); die Laststufe nimmt `bulkload` (`TestLoadConfigBulkQueue`, README).
  - Ebenfalls gefunden: LZ5 wurde je Build nur einmal gemessen; derselbe Build gegen sich
    selbst lag im Selbsttest 12 % auseinander und das Gate schlug an. LZ5 läuft jetzt wie die
    anderen Szenarien mehrmals im Wechsel, der beste Lauf zählt (E76).
  - `scripts/ci.sh` komplett grün (Go, Lizenzen, Last mit Gate: LZ1 p95 2,43 ms, LZ2 4,87 ms, LZ3 14,85 ms bei streuenden Läufen, LZ4 7,37 ms, LZ5 12.509 Zeilen/s; Web mit 21 Vitest- und 2 Browser-Tests, LZ9 131 ms).
- **Nicht geprüft:** Die Regel „nicht mehr als 10 % langsamer“ (E32) lässt sich derzeit nicht
  prüfen: Die Basislinie unter `/var/lib/itsm-load` stammt vom 2026-10-20, vor PL1, und kennt
  keine API; das Gate vergleicht deshalb nur mit den Zielwerten. Die Werte liegen im Bereich
  der Läufe von PL30 bis PL32 (LZ1 p95 2,43 ms, vorher 1,91 bis 2,36 ms). Die Oberfläche zeigt und setzt die Rollen noch nicht (Block B der
  Oberfläche); die Antwort nennt Typ und Id der Halter, nicht ihren Namen. Gruppen zählen nur
  direkt: Wer in einer Untergruppe ist, gehört für `@my_groups` und `involved` nicht zur
  Obergruppe. Nach dem endgültigen Löschen trägt das Ereignis keine Halter, `involved` zeigt es
  dann nicht mehr. Eine Rolle kann nicht Pflicht sein. Bulk-Zeilen mit Rollen laufen zeilenweise
  (langsamer als der Mengenweg). CEL-Bedingungen sehen weiterhin nur den Datensatz, nicht die
  Person (`person.…`); `record.actors` steht in Bedingungen zur Verfügung, ist aber nicht
  eigens getestet. Der Lieferant ist nur mit einem Testtyp belegt.
