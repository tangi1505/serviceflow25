# Plan: Service Desk (SD)

**Stand:** 2026-10-30, erste Fassung aus Hauptplan H10. Teil des [Hauptplans](0_maintask.md),
Etappe 2 (Meilenstein 1), abgenommen am 2026-10-31. Geprüft gegen den Arbeitsstand auf `main` vom 2026-10-30: Commit
`62ae664` und die noch nicht committeten Änderungen bis PL47e. Grundlage:
[Architektur](architektur.md), [Funktionsinventar](funktionsinventar-glpi.md) (24 Zeilen mit
Ziel SD), Practices IP4, IP19, IP24, IP28 und IP30 sowie Kernfähigkeiten KF8, KF14 und KF15 im
Hauptplan §2.1. Nichts davon ist gebaut, alle Aufwände sind geschätzt.

> **Kernaussage:** Der Service Desk ist fast nur Konfiguration (Hauptplan E7). Von 21 Aufgaben
> sind 9 reiner Paketinhalt unter `packages/`, 7 brauchen dazu die Oberfläche, 5 brauchen
> Go-Code. Im Kern fehlen drei Dinge: eine laufende Nummer je Typ, ein Zugang ohne Anmeldung und
> der Mail-Eingang (§2.3).

**Status:** ✅ abgenommen · 🟢 fertig, wartet auf die Abnahme · 🔨 in Arbeit · ⬜ offen ·
🔒 blockiert

**Für jede Aufgabe gilt:** Sie ist erst fertig, wenn `scripts/ci.sh` grün ist und die
Leistungsmessung aus Hauptplan H11 keinen Zielwert verfehlt und nichts um mehr als 10 %
langsamer macht (Hauptplan E32). Der Prüfweg nennt nur, was dazukommt. Alle Befehle laufen in
WSL in `~/police/itsm`. „Inventar G…" verweist auf die Zeilen des Funktionsinventars, „trägt KF…"
auf die Kernfähigkeit, für die die Aufgabe die Verantwortung hat. Mails prüfst du in Mailpit
unter http://localhost:8026 (`docker compose --profile mail up -d`).

---

## Aufgaben

### A — Grundpakete (zuerst)

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **SD1** | Grundpaket `sd`: Auswahllisten Kategorie als Baum mit Sichtbarkeit je Organisationseinheit, Anfragequelle, Lösungsart; Merkmal `ticket` mit Titel, Beschreibung (formatiert), Kategorie, Quelle, Auswirkung, Dringlichkeit; Rollen Anforderer, Beobachter, Bearbeiter, Gruppe (E1, E4; Inventar G39, G55) | 🟢 | 1,5 d | `docker compose run --rm tools go run ./cmd/itsm package install packages/sd` → Listen und Merkmal sind da; eine Kategorie in drei Ebenen anlegen; eine nur für die Dienststelle Nord sichtbare Kategorie sieht eine Person aus Süd nicht; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD2** | Paket `incident`: Typ Störung mit Merkmal `ticket`; Priorität P1 bis P4 als berechnetes Feld aus Auswirkung und Dringlichkeit; Statusmodell Neu, In Arbeit, Wartet, Gelöst, Geschlossen mit Wiedereröffnen; Lösung mit Lösungsart, Bestätigen oder Ablehnen durch den Anforderer; Pflichtfelder je Status. Die laufende Nummer kommt mit Kern-Plan PL49 dazu (§2.3, N1) (E3, E5; Inventar G36, G38, G42; IP19; Kern-Bedarf N7, N8) | 🟢 | 3 d | Störung mit Auswirkung „mehrere Personen“ und Dringlichkeit „hoch“ → P2; „Lösen“ ohne Lösungstext → 422 am Feld `solution`; der Anforderer lehnt die Lösung im Portal ab → wieder „In Arbeit“; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD3** | Fristen der Störung: Reaktion und Lösung je Priorität mit Kalender der Servicezeit, Eskalation an die Teamleitung bei 80 %, Wartegründe halten die Uhr an, automatisch schließen fünf Arbeitstage nach „Gelöst“; baut auf WF8 bis WF12 (E3; IP19) | 🟢 | 2 d | P1 um 08:00 anlegen → Lösungsfrist nach Kalender; „Wartet“ mit Grund „Rückmeldung Anforderer“ hält die Uhr an; mit Testzeit bei 80 % eine Mail an die Teamleitung in Mailpit; nach fünf Arbeitstagen „Gelöst“ → „Geschlossen“; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD4** | Betroffene Elemente und Bezüge: Beziehungsart „betrifft“ von Störung und Anfrage zu jedem Typ; „Duplikat von“ und „verwandt“ aus `core`; Verträge kommen mit CM (Inventar G49, G50) | 🟢 | 1,5 d | An einer Störung einen Datensatz eines beliebigen Typs als betroffen verknüpfen → steht in den Bezügen beider Seiten; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD5** | Major Incident: Übergang „zum Major Incident erklären“ mit Begründung, Mail an einen Verteiler, untergeordnete Störungen verknüpfen und mit dem Major Incident gemeinsam lösen, Hinweis im Portal (IP19; N3) | 🟢 | 1,5 d | Störung zum Major Incident erklären → Mail an den Verteiler; drei Störungen unterordnen, den Major Incident lösen → alle drei gelöst mit derselben Lösung, je eine Mail an ihre Anforderer; der Hinweis steht im Portal unter „Aktuell“; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD6** | Paket `request`: Typ Anfrage mit Merkmal `ticket`, Statusmodell mit Genehmigung, Erfüllungsaufgaben aus einer Vorlage, wiederkehrende Anfragen, Fristen wie SD3; baut auf WF6, WF13, WF14 (Inventar G36; IP30) | 🔒 WF6 | 1,5 d | Anfrage mit zweistufiger Genehmigung → nach beiden Zustimmungen entstehen zwei Aufgaben für die Gruppe; eine Ablehnung schließt die Anfrage mit Grund; `docker compose run --rm tools scripts/ci.sh` grün |

### B — Arbeitsplatz und Kanäle

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **SD7** | Warteschlangen: mitgelieferte geteilte Suchen „Meine“, „Mein Team“, „Nicht zugewiesen“, „SLA-Risiko“; Zuweisung per Regel nach Kategorie an eine Gruppe (IP28; KF7) | 🟢 | 1 d | Nach der Installation stehen die vier Suchen in der Navigation; Störung mit Kategorie „Netzwerk › VPN“ → Gruppe Netzwerk per Regel, der Verlauf nennt die Regel; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD8** | Textbausteine für Antworten, Aufgaben und Lösungen mit Platzhaltern wie in den Benachrichtigungen (Hauptplan E120), Auswahl über „Textbaustein“ im Editor, sichtbar je Gruppe (Inventar G56) | 🟢 | 1,5 d | Baustein „Protokoll anfordern“ mit `{{record.title}}` wählen → der Text steht mit eingesetztem Titel im Editor, nur mit Tastatur bedienbar; ein Baustein einer Gruppe erscheint nur bei ihren Mitgliedern; `axe` ohne Befund; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD9** | Telefonnotiz: Störung in einem Schritt anlegen mit Quelle „Telefon“, Anforderer per Suche, Rückruf als Aufgabe (IP28) | 🟢 | 1,5 d | „Telefonnotiz“ im Arbeitsplatz → Formular; mit Anforderer und Text anlegen → Störung mit Quelle „Telefon“ und Aufgabe „Rückruf“; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD10** | Mail hinaus: Vorlagen der Benachrichtigungen für Störung und Anfrage (angelegt, Antwort, Bitte um Bestätigung, gelöst, geschlossen) mit Kennung im Betreff; baut auf WF15 (Inventar G163; KF14) | 🟢 | 1 d | Antwort an den Anforderer → Mail in Mailpit mit der Kennung im Betreff (bis PL49 die Kurzkennung); interne Notiz → keine Mail; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD11** | Mail hinein, Abholen: Postfächer per IMAP als Job abholen, Zugangsdaten nur in der Konfiguration; neue Mail → Störung; Antwort mit Kennung → Eintrag am Vorgang; Anhänge und eingebettete Bilder als Dateien; Absender → Person über `core.person_email`; IMAP-Server zum Testen im Compose-Profil `mail` (N4; Inventar G188; trägt KF14) | 🔒 Hauptplan V9, V29 | 2 d | Testmail an das Postfach im Profil `mail` → Störung mit Anhang und Anforderer; Antwort auf die Mail aus SD10 → Eintrag am selben Vorgang; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD12** | Mail hinein, Regeln und Schutz: Regeln für eingehende Mail, Sperrliste für Inhalte wie Signaturen, keine Schleifen bei Abwesenheitsnotizen (`Auto-Submitted`), unverarbeitbare Mails im Protokoll des Postfachs und als Benachrichtigung (Inventar G189, G190, G191) | 🔒 SD11 | 1,5 d | Regel „Absender endet auf @extern.example → Gruppe Netzwerk“ greift; eine Abwesenheitsnotiz erzeugt weder Störung noch Antwort; eine kaputte Mail steht im Protokoll, die Verwaltung bekommt eine Mail; `docker compose run --rm tools scripts/ci.sh` grün |

### C — Portal, Katalog und Wissen

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **SD13** | Servicekatalog: Paket `catalog` mit Service und Angebot (Formular, Zielgruppe nach Organisationseinheit und Gruppe, Lieferzeit, Kosten, übersetzbar); Katalogseite im Portal mit Kategorien und Suche; eine Bestellung legt eine Anfrage an (Inventar G68, G133; IP24) | 🔒 SD6 | 2 d | Angebot „Zugang Gruppenlaufwerk“ nur für die Dienststelle Nord → nur dort im Katalog; bestellen → Anfrage mit den Antworten; bei englischer Sprache englisch; `axe` ohne Befund; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD14** | Portal-Startseite: Kacheln (Formular, Katalogseite, Artikel, externer Link) je Zielgruppe; „Aktuell“ zeigt veröffentlichte Hinweise mit Ablauf (Inventar G78; IP28) | ⬜ | 1,5 d | Kachel für eine Gruppe freigeben → nur ihre Mitglieder sehen sie; einen Hinweis veröffentlichen → steht unter „Aktuell“, nach dem Ablauf nicht mehr; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD15** | Anonyme Formulare: Zugang über einen Link ohne Anmeldung, Captcha im Werkzeug ohne fremden Dienst, Ratenbegrenzung, Rückmeldung an eine Mail-Adresse (N2; Inventar G134, G137; Hauptplan E26; trägt KF8 für „anonym“) | 🔒 Hauptplan V34 | 2 d | Formular als anonym freigeben → der Link öffnet es ohne Anmeldung; ohne gelöstes Captcha → 400; 20 Absendungen in einer Minute → 429; Bestätigung per Mail in Mailpit; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD16** | Wissen, Artikel: Paket `knowledge` mit Artikel (formatiert, Kategorien, Sichtbarkeit nach Organisationseinheit, Gruppe, Rolle und Portal), Versionen und Vergleich (PL19), Ablaufdatum mit Erinnerung, Übersetzungen (Inventar G106; IP4) | ⬜ | 2 d | Ein Artikel nur für das Portal → Nutzer sehen ihn; ein abgelaufener verschwindet aus dem Portal, der Autor bekommt eine Mail; zwei Fassungen vergleichen; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD17** | Wissen, Freigabe: eine neue Fassung ist erst nach der Genehmigung sichtbar; baut auf WF6 (IP4; KF4) | 🔒 WF6, SD16 | 0,5 d | Fassung 2 bleibt im Portal unsichtbar, bis die Freigabe erteilt ist; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD18** | Wissen am Vorgang und im Portal: Vorschläge beim Anlegen im Portal und an der Störung aus der Volltextsuche (PL26), Artikel verknüpfen, „hat geholfen“, Kommentare (Inventar G52, G108; IP4) | 🔒 SD16 | 2 d | Im Portal „VPN bricht ab“ tippen → ein passender Artikel als Vorschlag; „hat geholfen“ zählt; Artikel an eine Störung verknüpfen → steht in den Bezügen; `docker compose run --rm tools scripts/ci.sh` grün |

### D — Zufriedenheit, Einstellungen, Abschluss

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **SD19** | Umfragen: Fragebogen mit dem Formular-Baukasten (Hauptplan E133), Versand nach dem Schließen mit Anteil und Verzögerung, Antwort über den Link in der Mail, Ergebnis am Vorgang und als Kennzahl (Inventar G47, G65; trägt KF15) | 🔒 Hauptplan V33 | 2 d | Anteil 100 %, Verzögerung 0 → nach dem Schließen eine Mail mit Link; die Antwort steht am Vorgang; der Durchschnitt ist über `:query` mit Aggregat abfragbar; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD20** | Einstellungen des Service Desks am Organisationsbaum: Matrix der Priorität, Anhänge (Größe, Arten), automatisches Schließen, Anteil der Umfragen (Inventar G173; Hauptplan E131) | ⬜ | 1 d | Matrix für die Dienststelle Nord ändern → dort die neue Priorität, sonst die alte; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD21** | Durchgang Meilenstein 1 als Browser-Test: Anmeldung per OIDC, Störung über das Portal, Bearbeitung mit Frist, Anfrage mit Genehmigung, Wissensartikel als Lösung; alles Konfiguration (E2; Hauptplan Etappe 2) | 🔒 SD1 bis SD18 | 1,5 d | Der Browser-Test des Durchgangs in `docker compose run --rm tools scripts/ci.sh` ist grün; `git grep -n -e '"incident\.' -e '"request\.' -e '"catalog\.' -e '"knowledge\.' -e '"sd\.' -- internal web/src ':!*_test.go' ':!*.test.tsx'` gibt nichts aus |

**Summe:** 30 d, rund 6 Wochen, geschätzt vor dem Bau; dazu N1 aus §2.3 als PL49 im
[Kern-Plan](01_plattform-kern.md) (1 d). N2 und N4 bauen SD15 und SD11 selbst. Das Helpdesk-Dashboard ist UI14 im
[UI-Plan](02_oberflaeche.md) und kommt nach SD7. SD2 am 2026-11-03 von 2 d auf 3 d für die Kern-Bedarfe N7 und N8 (E5): 31 d. SD3 am 2026-11-03 von 1 d auf 2 d für die Kern-Bedarfe N9 und N10 (E6): 32 d. SD4 am 2026-11-03 von 0,5 d auf 1,5 d für N11 und den Reiter Bezüge (E7): 33 d. SD9 am 2026-11-04 von 0,5 d auf 1,5 d, weil Notizen beliebig sein sollen: Formulare auch im Arbeitsplatz (Hauptplan E167): 34 d.

**Reihenfolge:** Block A zuerst, weil alles andere die Typen braucht: SD1, SD2, SD3, SD4, dann
SD5. SD6 wartet auf WF6 (Genehmigungen). Danach Block B mit SD7, SD10, SD8 und SD9; SD11 und
SD12 erst, wenn die Fachseite Hauptplan V29 beantwortet hat. Block C mit SD16 und SD18 vor SD13
und SD14, weil das Portal die Artikel zeigt; SD15 erst nach Hauptplan V34. Block D zum Schluss, SD21 als Abnahme des
Meilensteins. Wer parallel arbeiten will, findet in §2.2 die Art jeder Aufgabe.

---

## 1. Entscheidungen

| # | Entscheidung | Seit | Quelle |
| --- | --- | --- | --- |
| **E1** | Getrennte Pakete: `sd` als Grundpaket (Listen, Merkmal `ticket`, Rollen, Textbausteine, Warteschlangen, Portal-Startseite), dazu `incident`, `request`, `catalog` und `knowledge`. Ein Kunde kann so etwa nur das Wissen nutzen | 2026-10-30 | Umsetzung, aus der Übersicht im Hauptplan (Zeile 05) |
| **E2** | Kein Sondercode: Kein Typname der SD-Pakete steht in `internal/` oder `web/src` außerhalb von Tests; was dafür fehlt, ist ein Bedarf an den Kern (§2.3) | 2026-10-30 | Hauptplan E7 |
| **E3** | Matrix der Priorität, Fristen und Servicezeit bekommen vorläufige Werte nach ITIL-Brauch (§2.4), bis die Fachseite Hauptplan V30 und V31 beantwortet | 2026-10-30 | Umsetzung |
| **E4** | Grundpaket `sd` (SD1, deine Entscheidungen 2026-11-03, Hauptplan E158): **Sichtbarkeit** einer Kategorie über den Knoten des Eintrags im Organisationsbaum: sichtbar für Personen an diesem Knoten und darunter, an der Wurzel für alle (Kern-Bedarf N5). **Rollen** Anforderer, Beobachter, Bearbeiter und Gruppe stehen am Merkmal `ticket`; Paket-Merkmale dürfen dafür Rollen mitbringen (Kern-Bedarf N6). **Startinhalt:** ein kleiner vorläufiger Kategorienbaum (Hardware, Software, Zugang und Berechtigung, Netz und Telefonie, Sonstiges, je mit Unterkategorien), bis die Fachseite Hauptplan V32 beantwortet. **Listen:** Anfragequelle Telefon, Mail, Portal, Vor Ort, Überwachung; Lösungsart Behoben, Anleitung gegeben, Umgehungslösung, Kein Fehler, Duplikat, Nicht lösbar; vorläufig wie E3. Verworfen: Feld mit mehreren Einheiten; Rollen in jedem Typ; leere Liste | 2026-11-03 | Pascal |
| **E5** | Störung (SD2, deine Entscheidungen 2026-11-03, Hauptplan E159): **Merkmal** mit Paketnamen, `traits: [sd.ticket]` aus installierten Paketen, von denen das Paket abhängt (N7). **Bestätigen und Ablehnen** der Lösung durch den Anforderer über Übergänge, die nur eine Rolle auslösen darf (`by: [requester]`); der Kern prüft die Rolle, das Portal zeigt solche Übergänge als Knöpfe (N8). **Ablehnen** braucht eine Begründung, die als Eintrag am Vorgang steht; der Bearbeiter wird benachrichtigt. **Pflicht:** Gelöst braucht Lösung und Lösungsart, Wartet einen Wartegrund; **Geschlossen** ist endgültig, wiedereröffnet wird nur aus Gelöst. Verworfen: Merkmal kopieren; nur antworten statt bestätigen; Ablehnen ohne Grund; Pflicht Bearbeiter oder Gruppe für „In Arbeit“ | 2026-11-03 | Pascal |
| **E6** | Fristen der Störung (SD3, deine Entscheidungen 2026-11-03, Hauptplan E160): **Startdaten** bringt ein Paket unter `data/` mit: Datensätze und Regeln, beim ersten Installieren je Mandant angelegt, danach gehören sie dem Kunden; ein Update legt nur an, was noch nie angelegt war, und überschreibt nichts, Gelöschtes kommt nicht wieder (Kern-Bedarf N9). **Teamleitung** ist die verantwortliche Person der Gruppe in der Rolle Gruppe, als Empfänger `actors.group.responsible` (N10). **Reaktion** ist erfüllt, sobald die Störung „Neu“ verlässt. **Jedes Warten** hält die Lösungsuhr an. Kalender „Servicezeit“ und Wartegründe liegen im Grundpaket `sd`, weil die Anfrage (SD6) sie auch braucht; Fristen, Eskalation bei 80 % und automatisches Schließen liegen in `incident`. P1 zählt rund um die Uhr (Hauptplan V31, Beta). Verworfen: Startdaten, die jedes Update zurücksetzt; Einrichtung nur per Anleitung; feste Gruppe Teamleitung; Leitung je Organisationseinheit; Reaktion erst mit der ersten Antwort; Haken je Wartegrund | 2026-11-03 | Pascal |
| **E7** | Betroffene Elemente und Bezüge (SD4, deine Entscheidungen 2026-11-03 nach dem Mockup „Bezüge am Vorgang“, [Artifact](https://claude.ai/artifact/LB9eW2KAQP7HHerk2ZMugJ), Hauptplan E161): **Beziehungsart** „betrifft“ (invers „betroffen von“) einmal im Grundpaket `sd`, von jedem Typ mit dem Merkmal `ticket` zu jedem Typ; Beziehungsarten dürfen dafür ein Merkmal statt eines Typs nennen (`trait:sd.ticket`, Kern-Bedarf N11). **Oberfläche:** die Detailseite bekommt Reiter unter dem Formular, „Bezüge“ zuerst, „Zeitleiste“ zuletzt (später „Dateien“ dazwischen); Bezüge als Tabelle mit Art, Typ, Datensatz, Status und Lösen, darüber Art und Datensatz zum Verknüpfen, die Suche über alle Typen; für jeden Typ, nicht nur Vorgänge. **Portal:** zeigt keine Bezüge, das entscheidet die Portal-Aufgabe. SD4 von 0,5 d auf 1,5 d. Verworfen: je Paket eine eigene Art; „betrifft“ von jedem Typ; Karte in der Seitenleiste; Abschnitt unter dem Formular; Zeitleiste als erster Reiter | 2026-11-03 | Pascal |

## 2. Begründung und Befunde

### 2.1 Zuordnung des Funktionsinventars

Alle 24 Zeilen mit Ziel SD haben eine Aufgabe:

| Inventar | Aufgabe | Inventar | Aufgabe |
| --- | --- | --- | --- |
| G36 | SD2, SD6 | G106 | SD16 |
| G38 | SD2, SD20 | G108 | SD18 |
| G39 | SD1 | G133 | SD13 (Change und Problem mit PR) |
| G42 | SD2 | G134 | SD15 |
| G47 | SD19 | G137 | SD15 |
| G49 | SD4 | G163 | SD10 |
| G50 | SD4 legt die Beziehungsart an, Verträge mit CM | G173 | SD20 |
| G52 | SD18 | G188 | SD11 |
| G55 | SD1 | G189 | SD12 |
| G56 | SD8 | G190 | SD12 |
| G65 | SD19 | G191 | SD12 |
| G68 | SD13 | G78 | SD14 |

Die Practices: IP19 (Incident) SD2, SD3, SD5, SD10, SD11; IP30 (Service Request) SD6; IP24
(Servicekatalog) SD13, SD15; IP28 (Service Desk) SD7, SD8, SD9, SD14; IP4 (Wissen) SD16 bis
SD18. KF15 (Umfragen) trägt SD19.

### 2.2 Was schon da ist, und die Art jeder Aufgabe

Gelesen in den Plänen, nicht einzeln am Code nachgesehen. Vieles ist 🟢 und noch nicht
abgenommen:

- **Workflow:** Statusmodell (WF1), Feldzustände (WF2), Rollen am Datensatz (WF3), Regeln (WF4),
  Kalender (WF8), SLA (WF9), Eskalation (WF10), Wartegründe (WF11), Zeitregeln (WF12),
  Aufgaben (WF13), wiederkehrende Vorgänge (WF14), Mail hinaus (WF15). Die Benachrichtigungen
  kennen schon das Ereignis eines neuen Eintrags (`internal/notify/notify.go:54`), Antworten an
  den Anforderer gehen also ohne Kern-Änderung per Mail.
- **Kern:** Beziehungen auch zu jedem Typ (`"*"`, `internal/meta/model.go:307`), Auswahllisten
  als Baum (PL16), Versionen (PL19), Übersetzungen (PL21), berechnete Felder (PL22),
  Volltextsuche (PL26), Zeitleiste (PL42), Aggregate in `:query` (PL14).
- **Oberfläche:** Portal (UI10), Formulare mit Bedingungen (UI11, UI12), Detailseite mit
  Zeitleiste (UI3), Einstellungen am Baum (UI9, Hauptplan E131).
- **Fehlt noch:** Genehmigungen (WF6, WF7), Dashboards (UI13).

Die Art sagt, welche Dateien eine Aufgabe anfasst: **P** nur Paketinhalt unter `packages/`,
**O** dazu `web/src`, **K** dazu Go-Code, API oder Migration.

| Art | Aufgaben |
| --- | --- |
| P | SD1, SD2, SD3, SD4, SD6, SD7, SD10, SD16, SD17 |
| P und O | SD13, SD14, SD18, SD19, SD21 |
| K | SD5 (wenn N3 eine Regelaktion braucht), SD8 (der Server setzt die Platzhalter ein, deine Entscheidung 2026-11-04), SD9 (Formulare im Arbeitsplatz, Regelaktion mit `@record` und `@actor`, deine Entscheidungen 2026-11-04), SD11, SD12, SD15, SD20 (Schlüssel der Einstellungen in `internal/prefs`) |

### 2.3 Bedarf an den Kern

| # | Bedarf | Befund | Vorschlag |
| --- | --- | --- | --- |
| **N1** | Laufende Nummer je Typ mit Präfix, etwa `INC-24817` | Der Kern kennt die Feldtypen text, number, boolean, date, datetime, choice und reference (`internal/meta/model.go:49` bis `:57`), keinen Nummernkreis | Feldtyp `sequence` mit Präfix, fortlaufend je Typ und Mandant: Kern-Plan PL49 (1 d), steht vor SD2 |
| **N2** | Zugang ohne Anmeldung für anonyme Formulare | Ohne Sitzung erreichbar sind nur Version, Anmeldung und Token (`internal/httpapi/auth.go:37`) | ein eigener Endpunkt nur zum Abrufen und Absenden freigegebener Formulare, mit Captcha und Ratenbegrenzung; gebaut in SD15 |
| **N3** | Gemeinsam lösen in SD5 | Gebaut in SD5 (Hauptplan E163): Regelaktion `related` bewegt die verknüpften Datensätze einer Beziehungsart; dazu feste Werte am Übergang (`set`) und ein Text für den Übergang einer Regel | erledigt |
| **N4** | Mail hinein | Kein Paket im Kern holt Mail ab; Mailpit im Profil `mail` kann nur SMTP (`compose.yaml:135`) | neues Paket `internal/mailin`; IMAP-Bibliothek und Testserver wählt SD11 nach Lizenzprüfung (PL29) |
| **N5** | Sichtbarkeit von Listeneinträgen je Organisationseinheit (E4) | Listeneinträge darf jeder lesen (implizites Lesen, Kern-Plan PL16); der Ort `org_path` eines Eintrags schränkt nichts ein | beim Lesen von Listen nur Einträge, deren Knoten über oder auf dem Knoten der Person liegt; gebaut in SD1 |
| **N6** | Rollen an Paket-Merkmalen (E4) | Paket-Merkmale (`traits/*.yaml`, Kern-Plan PL17) bündeln nur Felder; Rollen stehen unter `actors:` am Typ (WF3) | Paket-Merkmale dürfen `actors:` mitbringen, der Typ bekommt sie dazu; gebaut in SD1 |
| **N7** | Ein Merkmal eines anderen Pakets nutzen (SD2) | Ein Typ findet Merkmale nur im Kern und im eigenen Paket (`meta.expandTraits`, `p.Trait(name)`); `incident` braucht `ticket` aus `sd` | Merkmale mit Paketnamen (`traits: [sd.ticket]`) aus installierten Paketen auflösen, das Paket hängt dann von `sd` ab; bauen in SD2 |
| **N8** | Übergänge, die nur eine Rolle auslösen darf, und ihre Knöpfe im Portal (E5) | Bedingungen eines Übergangs sehen nur den Datensatz (`record`), nicht die handelnde Person; das Portal zeigt keine Übergänge (`web/src/portal/Portal.tsx`) | `by: [rolle]` am Übergang, der Kern prüft, dass die Person (oder eine ihrer Gruppen) die Rolle hält; ein Übergang mit `comment: required` verlangt einen Eintrag; das Portal zeigt die Übergänge, die die Person auslösen darf; gebaut in SD2 |
| **N9** | Ein Paket bringt Startdaten mit: Kalender, Fristen, Eskalationsstufen, Wartegründe (E6) | Ein Paket kennt nur Typen, Merkmale, Beziehungsarten und Listeneinträge (`meta.Package`); Kalender, Fristen und Wartegründe sind Datensätze, Eskalationsstufen Regeln | Ordner `data/` mit Einträgen je Schlüssel: ein Datensatz eines Typs oder eine Regel, Verweise `@schlüssel` (auch `@paket.schlüssel`); je Mandant einmal angelegt, gemerkt in der Tabelle `package_seed`, auch für neue Mandanten; gebaut in SD3 |
| **N10** | Die Teamleitung benachrichtigen (E6) | Empfänger sind Rollen am Vorgang, Gruppen und Personen (`notify.checkRecipient`, Regelaktion `notify`); die verantwortliche Person einer Gruppe (`core.group.responsible`) nutzt niemand | Empfänger `actors.<rolle>.responsible`: die verantwortlichen Personen der Gruppen, die die Rolle halten; in Benachrichtigungen und Regeln; gebaut in SD3 |
| **N11** | Eine Beziehungsart für alle Vorgänge (E7) | `from` und `to` einer Beziehungsart nennen nur Typen oder `*` (`meta.RelationKind.Allows`) | Einträge `trait:<paket>.<merkmal>` gelten für jeden Typ mit dem Merkmal; das Paket muss das Merkmal selbst haben oder von seinem Paket abhängen; gebaut in SD4 |

### 2.4 Vorläufige Werte (E3)

- **Priorität:** Auswirkung in drei Stufen (hoch: viele Personen oder ein Fachverfahren;
  mittel: mehrere Personen; niedrig: eine Person) und Dringlichkeit in drei Stufen:

  | Auswirkung \ Dringlichkeit | hoch | mittel | niedrig |
  | --- | --- | --- | --- |
  | hoch | P1 | P2 | P3 |
  | mittel | P2 | P3 | P4 |
  | niedrig | P3 | P4 | P4 |

- **Fristen** (Reaktion / Lösung): P1 15 min / 4 h, P2 30 min / 8 h, P3 4 h / 3 Tage,
  P4 1 Tag / 10 Tage, gerechnet in der Servicezeit.
- **Servicezeit:** Montag bis Freitag 7 bis 18 Uhr, ohne gesetzliche Feiertage.

### 2.5 Befund für den Hauptplan: Datenübernahme aus GLPI

Kein Plan übernimmt Daten aus GLPI. Gesucht nach „Übernahme“, „übernehmen“ und „aus GLPI“ in
Hauptplan, Architektur und Funktionsinventar, ohne Treffer zur Datenübernahme. Für den
Service Desk betrifft das Kategorien, Wissensartikel, Textbausteine und offene Tickets.
Daraus sind die Frage Hauptplan V37 und die Aufgabe Hauptplan H15 entstanden.

## 3. Offene Fragen

| # | Frage | Hängt daran |
| --- | --- | --- |
| **V1** ✅ | Gemeinsam lösen beim Major Incident: als Regelaktion im Kern oder als Massenaktion aus der Oberfläche (N3)? Klärt die Umsetzung in SD5 | SD5 |
| **V2** 🟡 | Welche IMAP-Bibliothek und welcher Testserver, beide mit erlaubter Lizenz (PL29)? Klärt die Umsetzung in SD11 | SD11 |

Die Fragen an die Fachseite zu diesem Plan stehen im Hauptplan §6.2: V29 (Mailsystem), V30
(Matrix der Priorität), V31 (Servicezeiten und Fristen), V32 (Kategorien), V33 (Umfragen), V34
(anonyme Formulare), V35 (Major Incident), V36 (Telefonanlage), V37 (Datenübernahme, mit
Hauptplan H15).

## 4. Umsetzungsstand

### SD9 — Notizen im Arbeitsplatz, 2026-11-04

- **Grundlage:** IP28; Hauptplan E167; deine Entscheidungen vom 2026-11-04: Notizen sollen
  beliebig sein, nicht nur Telefon, darum Formulare auch im Arbeitsplatz; mitgeliefert
  Telefonnotiz und Vor-Ort-Notiz; die Rückruf-Aufgabe legt eine Regel an; eine eigene Aufgabe
  der Störung; Ort nach Variante A des Mockups (Menü „Neu“ an der Liste), die Rückrufnummer als
  freie Frage.
- **Gebaut:**
  - Kern, Formulare: Feld „Angeboten im“ (`place`: Portal, Arbeitsplatz, beides; leer heißt
    Portal) am Kerntyp `core.form`, `GET /api/v1/forms?place=workplace`. Eine Frage kann eine
    Rolle erfragen (`role`, nur Rollen mit einer Art von Inhaber, etwa der Anforderer per
    Suche); dann wird nicht mehr die absendende Person Anforderer. Feste Werte (`fixed`) setzen
    Felder ohne Frage, bei einer Auswahlliste über den Schlüssel des Eintrags
    (`{"channel": "phone"}`); geprüft beim Speichern.
  - Kern, Regeln: die Aktion „anlegen“ setzt `@record` (der Datensatz der Regel, in einem
    Verweis auf seinen Typ) und `@actor` (die Person, die geschrieben hat, als Inhaber einer
    Rolle) ein; die Startdaten lesen beides nicht als Verweis auf andere Startdaten.
  - Paket `incident` 0.7.0: Typ `incident.task` (Aufgabe: Titel, Störung, Beschreibung mit
    Textbausteinen „Aufgabe“, fällig am, Bearbeiter, offen und erledigt); an der Störung die
    Felder „Rückruf erbeten“ und „Offene Aufgaben“ (Summe). Startdaten `data/notizen.yaml`:
    Formulare „Telefonnotiz“ (Quelle Telefon, Rückrufnummer) und „Vor-Ort-Notiz“ (Quelle Vor Ort,
    Ort), Regel `rueckruf`: legt bei „Rückruf erbeten“ die Aufgabe „Rückruf“ für die Person an,
    die die Notiz aufnimmt.
  - Oberfläche: der Knopf „Neu“ einer Liste wird ein Menü, wenn es Formulare des Arbeitsplatzes
    für den Typ gibt (oben das Formular mit allen Feldern, darunter die Formulare); die
    Befehlszeile findet sie als „Formular: …“; die Seite `/forms/<id>` im Arbeitsplatz öffnet
    nach dem Anlegen den neuen Datensatz. Der Baukasten kennt Rollen-Fragen und feste Werte.
- **Belegt:**
  - Go `TestPhoneNoteOverHTTP` (eigene Datenbank): die Notizen stehen im Arbeitsplatz, nicht im
    Portal; Sabine nimmt Annas Anruf auf: Quelle Telefon, Anna als Anforderer, die Nummer im
    ersten Eintrag, die Aufgabe „Rückruf“ für Sabine, eine offene Aufgabe an der Störung; ohne
    Rückruf keine Aufgabe; abgelehnt: eine Rolle mit zwei Arten von Inhabern, ein Schlüssel,
    den die Liste nicht hat, ein Feld zugleich gefragt und fest, eine Frage mit Rolle und Feld,
    `@record` in einem Feld, das nicht auf die Störung verweist.
  - Vitest `FormPage.test.tsx`: Menü „Neu“, Telefonnotiz, Anforderer per Suche, fester Wert,
    neuer Datensatz geöffnet, `axe` ohne Befund; ohne Formulare bleibt der einfache Knopf.
  - Angepasst: die Tests von SD3, SD5, SD7, SD8 und SD10 zählen 39 statt 36 Startdaten von
    `incident`, ein neuer Mandant 67 statt 64; der Test des Baukastens zählt Regionen, darum
    sind die festen Werte keine eigene Region; der Browser-Test des Baukastens sucht die
    Checkbox jetzt als „Angeboten“ (vorher „Im Portal angeboten“, der Ort steht im neuen Feld).
  - Ganze CI am 2026-11-04 im dritten Lauf grün (19 Schritte, 100 Vitest-Fälle, 18 Tests im
    Browser). Lauf 1: der Browser-Test mit dem alten Namen der Checkbox; Lauf 2: LZ13 10,2 %
    unter der Basislinie bei gleichem ausgeliefertem Code wie Lauf 1, in dem die Messung grün war.
- **Nicht geprüft:** Die Aufgabe hat noch keine Frist für den Rückruf (das Feld „fällig am“ ist
  da, die Regel setzt es nicht). Der Baukasten nimmt feste Werte als Text: bei einer
  Auswahlliste den Schlüssel, ohne Auswahl der Einträge. Die Befehlszeile ist nicht eigens
  getestet. Kein Test im echten Browser.

### SD8 — Textbausteine, 2026-11-04

- **Grundlage:** Inventar G56; Hauptplan E166; deine Entscheidungen vom 2026-11-04: der Server
  setzt die Platzhalter ein, „sichtbar je Gruppe“ ist ein Filter in der Auswahl, ein Feld „Für
  Typ“ (leer heißt jeder Typ), die Verwendung „Aufgabe“ gibt es schon jetzt; die Auswahl nach
  Variante A des Mockups (Knopf „Textbaustein“ mit Liste). Damit ist SD8 nicht mehr nur Paket
  und Oberfläche, sondern hat einen Teil im Kern (§2.2).
- **Gebaut:**
  - Kern: Kerntyp `core.snippet` mit Name, Verwendung (Antwort, Lösung, Aufgabe), „Für Typ“,
    formatiertem Text und der Rolle „Sichtbar für“ (Personen und Gruppen). Das Paket
    `internal/snippets` prüft beim Speichern den Typ und die Platzhalter: `{{record.<feld>}}`,
    `{{type}}`, `{{title}}`, `{{link}}` und `{{actor}}` (wer schreibt). Ohne Typ muss das Feld
    in irgendeinem installierten Typ stehen. Eingesetzt wird mit den Rechten der Person, die
    schreibt: Werte maskiert, ein Verweis mit seinem Titel, eine Auswahl mit ihrer Bezeichnung,
    `{{link}}` als Link. `POST /api/v1/records/{typ}/{id}:snippet` schreibt nichts. Die
    Platzhalter kommen aus den Benachrichtigungen (`notify.Render` mit `HTML` und `Name`,
    `notify.CheckPlaceholders`).
  - Feldmerkmal `snippets: <verwendung>` an formatierten Textfeldern: der Editor des Felds bietet
    die Bausteine dieser Verwendung an. Die Lösung der Störung hat `snippets: solution`, die
    Einträge der Zeitleiste nehmen `answer`.
  - Oberfläche: Knopf „Textbaustein“ am Ende der Formatierungsleiste, auch Alt+T im Text. Die
    Liste hat eine Suche; oben stehen die Bausteine der eigenen Gruppen und die persönlichen,
    darunter die für alle; rechts die Vorschau, schon eingesetzt. Enter fügt an der Schreibmarke
    ein, Esc schließt, der Fokus geht in beiden Fällen zurück in den Text. Nur an einem
    bestehenden Vorgang und nicht im Portal.
  - Paket `incident` 0.6.0, Startdaten `data/bausteine.yaml`: „Protokoll anfordern“ (Antwort,
    mit dem Service Desk geteilt) und „Gelöst durch Neustart“ (Lösung, nur für Störungen).
- **Belegt:**
  - Go `TestSnippetsOverHTTP` (eigene Datenbank): Lena aus dem Service Desk findet „Protokoll
    anfordern“, Olaf nicht; eingesetzt mit maskiertem Titel und Lenas Namen; Kategorie mit
    Namen, Auswirkung „Niedrig“, Typ, Link und Nummer; die Störung bleibt unverändert; ein
    Baustein für Störungen an einer Person ergibt 422; beim Speichern abgelehnt: ein Feld, das
    der Typ nicht hat, `{{entry}}`, ein nicht installierter Typ, ohne Typ ein Feld keines Typs.
  - Go `TestRenderHTML`: Maskieren, Link, Verweis mit Titel.
  - Vitest `Snippets.test.tsx`: Alt+T, Suche, Reihenfolge, Vorschau, Enter fügt nach der Anrede
    ein, Esc, Fokus zurück im Text, `axe` ohne Befund; ohne Vorgang kein Knopf.
  - Angepasst: die Tests von SD3, SD5, SD7 und SD10 zählen 36 statt 34 Startdaten von
    `incident`, ein neuer Mandant 64 statt 62; `coverage_test.go` bekommt ein gültiges Muster
    für `core.snippet` (Für Typ `probe.probe`), weil das erzeugte Muster einen erfundenen Typ
    nennt.
  - Ganze CI am 2026-11-04 im ersten Lauf grün: 19 Schritte, 98 Vitest-Fälle, 18 Tests im Browser,
    die Lastmessung im Rahmen der Basislinie.
- **Nicht geprüft und Abweichungen:** Über den Bausteinen der Gruppen steht „Meine Gruppen“, nicht
  der Name der Gruppe wie im Mockup. Ohne Typ bleibt ein Platzhalter,
  dessen Feld der Typ des Vorgangs nicht hat, als `{{record.<feld>}}` im Text stehen, sichtbar
  zum Nachbessern. Aufgaben an der Störung gibt es noch nicht; die Verwendung „Aufgabe“ wirkt,
  sobald ein Aufgabentyp sein Textfeld mit `snippets: task` markiert. Beim Anlegen eines
  Vorgangs gibt es keine Bausteine, weil noch kein Datensatz zum Einsetzen da ist. Kein Test im
  echten Browser (Playwright).
- **Nachtrag, deine Entscheidung 2026-11-04:** Auch die Mails zeigen einen Verweis jetzt mit
  seinem Titel statt der Kennung (`notify.RefTitle`, von Mails und Textbausteinen geteilt).
  Belegt in `TestIncidentMailsOverHTTP`: eine Benachrichtigung mit `{{record.category}}` im
  Betreff schickt „SD10 Kategorie Netz und Telefonie“.

### SD10 — Mail hinaus, 2026-11-03

- **Grundlage:** Inventar G163, KF14; Hauptplan E165; deine Entscheidungen vom 2026-11-03:
  fünf Mails in beide Richtungen, die Kennung vorne in Klammern, der Link führt Anforderer zum
  Vorgang im Portal. Die Anfrage bekommt ihre Mails mit SD6.
- **Gebaut:**
  - Kern, Benachrichtigungen: die Bedingung sieht neben `record` die Rollen am Datensatz
    dessen, der das Ereignis ausgelöst hat (`by`, etwa `"requester" in by`); formatierter Text
    eines Felds (`{{record.solution}}`) kommt ohne Auszeichnung in die Mail, wie schon ein
    Eintrag.
  - Paket `incident` 0.5.0, Startdaten `data/mails.yaml`, jeder Betreff mit `[INC-…]` vorne:
    „Störung eingegangen“ an den Anforderer (auch wenn er selbst meldet), „Antwort an den
    Anforderer“, „Anforderer hat geantwortet“ an den Bearbeiter, ohne Bearbeiter an die Gruppe,
    „Störung gelöst“ mit der Lösung und der Bitte um Bestätigung (Schlüssel aus SD5, damit
    kein Mandant sie doppelt bekommt), „Störung geschlossen“. Wer schreibt, bekommt keine Mail
    davon; ein privater Eintrag geht nie an den Anforderer (PL42).
  - Oberfläche: wer keinen Arbeitsplatz hat und den Link eines Datensatzes öffnet
    (`/types/<typ>/<id>`, wie in jeder Mail), landet auf dem Datensatz im Portal.
- **Belegt:**
  - Go `TestIncidentMailsOverHTTP` (eigene Datenbank): Anna meldet mit eigenem Konto und bekommt
    „[INC-1] Ihre Störung ist eingegangen: …“; ihre Antwort ohne Bearbeiter geht an Gerd aus der
    Gruppe; die Antwort des Service Desk an sie, die interne Notiz nicht; ihre nächste Antwort
    an Ben als Bearbeiter, nicht an die Gruppe, nicht an sie selbst; „gelöst“ mit „Toner
    getauscht“ ohne HTML und der Bitte um Bestätigung; „geschlossen“.
  - Vitest `Portal.test.tsx`: der Link eines Datensatzes führt einen Nutzer ohne Arbeitsplatz
    zum Datensatz im Portal.
  - Angepasst: `TestMajorIncidentOverHTTP` erwartet den neuen Betreff „[INC-…] Ihre Störung ist
    gelöst“; die Tests von SD3, SD5 und SD7 zählen 34 statt 29 Startdaten von `incident`, ein
    neuer Mandant 62 statt 57.
  - Ganze CI am 2026-11-03 grün im ersten Lauf (11:48 min): alle 19 Schritte, 96 Vitest-Fälle,
    18 Browser-Tests, LZ1 p95 3,67 ms gegen 3,65 ms der Basislinie, LZ13 7.541 gegen 7.171
    Zeilen/s.
- **Nicht geprüft:** Mailpit im Profil `mail` habe ich nicht von Hand angesehen; die Mails
  belegt der SMTP-Testserver des Go-Tests. Bestätigt der Anforderer die Lösung selbst, bekommt
  er keine Mail „geschlossen“ (er hat es selbst ausgelöst); beim automatischen Schließen schon.
  Die Mails sind reiner Text, Deutsch; englische Texte trägt der Kunde als Übersetzung der
  Benachrichtigung nach. Die Bitte um Bestätigung nennt fünf Arbeitstage, wie die Startdaten von
  SD3; ändert ein Mandant die Frist, muss er den Text mit ändern.

### SD7 — Warteschlangen und Zuweisung nach Kategorie, 2026-11-03

- **Grundlage:** IP28, KF7; Hauptplan E164; deine Entscheidungen vom 2026-11-03: die vier
  Suchen geteilt mit einer Gruppe „Service Desk“ der Startdaten; die Gruppe nach der Kategorie
  mit Vererbung nach oben; „Nicht zugewiesen“ heißt ohne Bearbeiter. Weil Auswahllisten keine
  eigenen Felder haben (Hauptplan E93), steht die Zuordnung nicht an der Kategorie, sondern in
  einem eigenen Typ „Zuständigkeit“ (deine Entscheidung nach diesem Befund).
- **Gebaut:**
  - Kern, Regeln: `assign` nimmt statt fester Halter `lookup` (Typ, `match`, `take`): die
    Datensätze des Typs, deren Feld `match` dasselbe nennt wie der Datensatz, geben ihr
    Verweisfeld `take`; zeigt `match` in eine Liste als Baum und nennt niemand den Eintrag,
    zählt der Eintrag darüber, bis zur Wurzel. Die Regel prüft Typ, Felder und ob die Rolle den
    Halter nimmt.
  - Paket `sd` 0.4.0: Typ `sd.responsibility` „Zuständigkeit“ (Kategorie, Gruppe); Startdaten:
    Gruppe „Service Desk“.
  - Paket `incident` 0.4.0, Startdaten: Regel `gruppe_nach_kategorie` (vor dem Anlegen, nur
    ohne genannte Gruppe); die geteilten Suchen „Meine Störungen“ (Bearbeiter ich), „Störungen
    meines Teams“ (Gruppe eine meiner), „Störungen ohne Bearbeiter“ und „Störungen mit
    SLA-Risiko“ (Warnzeit der Frist erreicht), jeweils offen, nach Frist sortiert.
- **Belegt:**
  - Go `TestIncidentQueuesOverHTTP` (eigene Datenbank): Zuständigkeit für „Netz und Telefonie“,
    eine Störung mit Kategorie VPN bekommt die Gruppe, der Verlauf nennt die Regel; eine
    genannte Gruppe bleibt; „Sonstiges“ ohne Zuständigkeit bleibt ohne Gruppe; eine Regel mit
    falschem Feld wird abgelehnt (422). Sven, Mitglied von „Service Desk“ und der Gruppe, sieht
    die vier Suchen; „meines Teams“ findet die VPN-Störung, „ohne Bearbeiter“ alle drei, „Meine“
    erst, nachdem er Bearbeiter ist; „SLA-Risiko“ ist gleich nach dem Anlegen leer.
  - Angepasst: `TestIncidentDeadlinesOverHTTP` und `TestMajorIncidentOverHTTP` zählen 28 statt 27
    Startdaten von `sd` und 29 statt 24 von `incident`, ein neuer Mandant 57 statt 51.
  - Ganze CI am 2026-11-03 grün im zweiten Lauf (11:31 min): alle 19 Schritte, 96 Vitest-Fälle,
    18 Browser-Tests, LZ1 p95 3,85 ms gegen 3,75 ms der Basislinie, LZ13 6.736 gegen 7.160
    Zeilen/s. Lauf 1 scheiterte nur daran, dass die Typen der Weboberfläche nach der Änderung an
    `api/openapi.yaml` nicht neu erzeugt waren.
- **Nicht geprüft:** Kein Browser-Test, weil die CI-Instanz `sd` und `incident` nicht
  installiert; die Navigation zeigt jede lesbare gespeicherte Suche (UI30). Die Mitgliedschaft
  zählt nur direkt: wer die Suchen sehen soll, steht selbst in „Service Desk“, oder seine Gruppe
  wird an der Suche unter „Geteilt mit“ ergänzt; eine Untergruppe von „Service Desk“ reicht
  nicht. Die Regel greift nur beim Anlegen, nicht bei einer späteren Änderung der Kategorie.
  „SLA-Risiko“ mit erreichter Warnzeit ist im Go-Test nicht belegt (die Abfrage auf
  `sla_warn_at` ist es seit UI34). Gibt es zwei Zuständigkeiten für eine Kategorie, gilt die
  ältere.

### SD5 — Major Incident, 2026-11-03

- **Grundlage:** N3 dieses Plans, V1; Hauptplan E163; deine Entscheidungen vom 2026-11-03:
  allgemeine Bausteine im Kern, der Major Incident selbst als Standard im Paket, den jeder
  Mandant ändert; Erklären von Hand, eine Regel für automatisches Erklären ausgeschaltet dabei;
  Verteiler als Gruppe; gemeinsames Lösen als Regelaktion im Kern; im Portal Variante A des
  Mockups „Aktuell im Portal“ ([Artifact](https://claude.ai/artifact/Czt6iSNDdYvCFTLYkHMVmY)):
  ein Band unter dem Kopf, die Karten der Startseite nebeneinander, wo Platz ist.
- **Gebaut:**
  - Kern, Workflow: ein Übergang setzt feste Werte (`set`, `meta.Transition.Set`), die über den
    Feldern des Aufrufs stehen; `meta.Validate` prüft Feld und Art, ein Übergang einer Rolle
    (`by`) setzt keine.
  - Kern, Regeln: Aktion `related` (Art, Richtung `in` oder `out`, Übergang, `copy` von Feldern,
    feste Werte, Text) bewegt die verknüpften Datensätze; einer, dessen Status den Übergang
    nicht erlaubt, bleibt, wie er ist. Der Übergang einer Regel nimmt einen Text (`comment`).
  - Kern, Startdaten: `<paket.typ>:@schlüssel` nennt einen Eintrag als Empfänger
    (`core.group:@verteiler`).
  - Kern, Portal: Typ `core.portal_notice` (Typ, Bedingung als JSON-Abfrage, Textfeld, aktiv)
    mit Prüfung beim Schreiben (`internal/portal`); `GET /api/v1/portal/notices` liefert jedem
    Angemeldeten Text, Status und Zeiten der Datensätze, mit den Rechten des Systems im
    Mandanten.
  - Paket `incident` 0.3.0: Felder „Major Incident“ und „Mitteilung an Anwender“ (Pflicht, sobald
    Major Incident); Übergang „Zum Major Incident erklären“ von Neu, In Arbeit und Wartet nach In
    Arbeit, mit Begründung, setzt „Major Incident“; Beziehungsart `incident.part_of` „gehört zu“
    / „umfasst“. Startdaten: Gruppe „Major-Incident-Verteiler“, Regel `major_mail` an sie, Regel
    `major_p1` (P1 automatisch, ausgeschaltet), Regel `major_solve` (zugehörige Störungen mit
    Lösung und Lösungsart mitlösen), Benachrichtigung „Störung gelöst“ an den Anforderer,
    Hinweis „Offene Major Incidents“ im Portal.
  - Portal: Band „Aktuell“ unter dem Kopf auf jeder Seite, je Datensatz eine Zeile mit
    Mitteilung, seit wann und Status, „Mehr“ klappt den Text auf; Inhalt 1.152 px breit, die
    Karten der Startseite nebeneinander, wo jede 22,5 rem behält, auf dem Handy untereinander.
- **Belegt:**
  - Go `TestMajorIncidentOverHTTP` (eigene Datenbank): Erklären ohne Begründung und ohne
    Mitteilung 422, mit beiden „Major Incident“ und „In Arbeit“; Mail an das Mitglied des
    Verteilers; eine Person ohne Recht auf Störungen liest die Störung nicht, sieht aber im
    Portal genau die Mitteilung mit Status „In Arbeit“, ohne Titel; drei zugehörige Störungen
    (eine davon in Arbeit) nach dem Lösen gelöst mit derselben Lösung und Lösungsart, je eine
    Mail an ihre Anforderer; danach ist „Aktuell“ leer.
  - Go `TestTransitionSetsFixedValues` und `TestSeedRefAfterARecipientType`.
  - Vitest `Portal.test.tsx` (2 neue Fälle): das Band mit Mitteilung, Zeit und Status, „Mehr“
    und „Weniger“ mit `aria-expanded`, axe ohne Befund; ohne Hinweis kein Band.
  - Angepasst: `TestIncidentDeadlinesOverHTTP` zählt 24 statt 18 Startdaten von `incident` und
    51 statt 45 für einen neuen Mandanten, weil SD5 sechs dazubringt; der Beispieldatensatz der
    API-Abdeckung bekommt für `core.portal_notice` einen Typ und ein Textfeld, die es gibt.
  - Ganze CI am 2026-11-03 grün im zweiten Lauf (10:49 min): alle 19 Schritte, 96 Vitest-Fälle,
    18 Browser-Tests, LZ1 p95 2,92 ms gegen 3,00 ms der Basislinie, LZ13 8.119 gegen 8.214
    Zeilen/s. Lauf 1 scheiterte nur an LZ13 (7.948 gegen 8.908 Zeilen/s, 11 % weniger, erlaubt
    10 %); bewertet als Streuung, weil LZ13 schon bei gleichem Code 4 % und 7 % unter der
    Basislinie lag (PL52) und SD5 den Pfad nicht berührt.
- **Nicht geprüft:** Kein Browser-Test, weil die CI-Instanz `sd` und `incident` nicht
  installiert; das Band belegen Vitest und der Go-Test. „Zum Major Incident erklären“ wird auch
  an einem schon erklärten Major Incident angeboten (eine Bedingung am Übergang prüft den Stand
  nach dem Setzen und schlösse ihn aus); es ändert dann nichts und schickt keine zweite Mail.
  Die Mitteilung muss vor dem Erklären im Formular gespeichert sein; einen Dialog mit Feldern am
  Übergang gibt es noch nicht. Die automatische Regel für P1 ist nur ausgeschaltet geprüft
  (vorhanden, `enabled: false`).

### SD4 — Betroffene Elemente und Bezüge, 2026-11-03

- **Grundlage:** E7 dieses Plans; Hauptplan E161; UI-Plan E38; deine Entscheidungen vom
  2026-11-03 nach dem Mockup „Bezüge am Vorgang“
  ([Artifact](https://claude.ai/artifact/LB9eW2KAQP7HHerk2ZMugJ), Variante C mit „Bezüge“ als
  erstem und „Zeitleiste“ als letztem Reiter).
- **Gebaut:**
  - Kern-Bedarf N11: `from` und `to` einer Beziehungsart dürfen `trait:<paket>.<merkmal>` nennen;
    die Art gilt dann für jeden Typ mit dem Merkmal (`meta.RelationKind.Allows` nimmt Typen).
    `meta.Validate` lehnt ein Merkmal ohne Paket, ein unbekanntes eigenes und eines aus einem
    Paket ohne Abhängigkeit ab.
  - Paket `sd` 0.3.0: Beziehungsart `sd.affects` „betrifft“ / „betroffen von“ von
    `trait:sd.ticket` zu jedem Typ. „Duplikat von“ und „verknüpft mit“ kommen aus `core`.
  - Detailseite jedes Typs: Reiter unter dem Formular (`RecordTabs`, Pfeiltasten, Pos1 und Ende),
    „Bezüge“ zuerst, „Zeitleiste“ oder „Verlauf“ zuletzt. Der Reiter Bezüge (`Relations`) zeigt
    beide Richtungen als Tabelle mit Art (Beschriftung oder Gegenrichtung), Typ, Datensatz mit
    laufender Nummer und Status; Lösen fragt nach. Darüber Art (je Richtung, in der der Typ
    stehen darf) und Datensatz über die Suche der erlaubten Typen, bei `*` aller durchsuchbaren.
  - Das Portal zeigt keine Bezüge.
- **Belegt:**
  - Go `TestIncidentAffectsOverHTTP` (eigene Datenbank): die Art steht mit `trait:sd.ticket`
    unter den Arten; eine Störung betrifft eine Probe, beide Seiten zeigen den Bezug; eine Probe
    kann keine Störung „betreffen“ (422).
  - Go `TestRelationKindNamesATrait`: drei falsche Merkmalsangaben mit Meldung; die Art gilt
    für den Typ mit dem Merkmal und nicht für einen ohne.
  - Vitest `Relations.test.tsx` (2 Fälle): Merkmale in `relates`; Reiter in der Reihenfolge,
    ein eingehender Bezug liest „hat Duplikat“, Anna per Suche als „verknüpft mit“ verknüpft,
    axe ohne Befund, Lösen erst nach Rückfrage, Pfeiltaste zur Zeitleiste.
  - Playwright `relations.spec.ts`: zwei Proben verknüpft, Bezug mit Nummer auf beiden Seiten,
    axe ohne Befund.
  - Angepasst, weil die Zeitleiste jetzt im letzten Reiter steht: `RecordPage.test.tsx`,
    `e2e/records.spec.ts` und `e2e/live.spec.ts` klicken vorher auf den Reiter „Zeitleiste“.
  - Ganze CI am 2026-11-03 grün im sechsten Lauf (10:54 min): alle 19 Schritte, 94 Vitest-Fälle,
    18 Browser-Tests, LZ1 p95 3,67 ms, LZ13 8.763 Zeilen/s. Lauf 1 fand `live.spec` (Zeitleiste im
    Reiter, behoben); die Läufe 2 bis 5 scheiterten an wackelnden Tests ohne Bezug zu SD4:
    Formular-Baukasten per Tastatur (Spur in WSL `~/ci-befunde/formbuilder-2026-11-03`), Zeitgrenze
    von Formular-Baukasten und Workflow-Editor in Vitest unter Last, ein Neustart mitten im Lauf und
    LZ9 (Seitenwechsel 777 ms) direkt nach dem Neustart. Sie gehen in PL52.
- **Nicht geprüft:** Der Browser-Test nimmt zwei Proben, weil die CI-Instanz `sd` und `incident`
  nicht installiert; „betrifft“ an der Störung belegt der Go-Test. Jede Zeile lädt ihren
  Datensatz einzeln (für Name und Status); bei sehr vielen Bezügen wird das langsam. Die Wahl
  des Reiters bleibt nicht über einen Seitenwechsel erhalten.

### SD3 — Fristen der Störung, 2026-11-03

- **Grundlage:** E3, E6 dieses Plans; Hauptplan E160, V31 (Beta: P1 rund um die Uhr); deine
  Entscheidungen vom 2026-11-03.
- **Gebaut:**
  - Kern-Bedarf N9, Startdaten: ein Paket bringt unter `data/*.yaml` Einträge mit Schlüssel mit,
    je ein Datensatz eines Typs (`type`, `values`) oder eine Regel (`rule`); `@schlüssel` und
    `@paket.schlüssel` verweisen auf frühere Einträge (`meta.Seed`, geprüft in `meta.Validate`).
    `internal/seed` legt sie beim Installieren je Mandant an, ohne Mandanten an der Wurzel, und bei
    jedem neuen Mandanten (beide Betriebsarten); die Tabelle `package_seed` (Migration 42) merkt,
    was angelegt wurde. Ein zweites Installieren legt nichts an, Gelöschtes kommt nicht wieder;
    beim Übernehmen des ersten Mandanten wandern die Einträge mit. Regeln heißen
    `<paket>_<schlüssel>`, in einem Mandanten mit dessen Namen dahinter.
  - Kern-Bedarf N10: Empfänger `actors.<rolle>.responsible` sind die verantwortlichen Personen der
    Gruppen, die die Rolle halten, in Regelaktionen `notify` und in Benachrichtigungen; eine Rolle,
    die keine Gruppen hält, wird abgelehnt (422).
  - Paket `sd` 0.2.0: Kalender „Servicezeit“ Montag bis Freitag 7 bis 18 Uhr mit den bundesweiten
    Feiertagen 2026 und 2027; Wartegründe Rückmeldung Anforderer, Termin mit Anforderer, Zulieferung
    durch Dritte.
  - Paket `incident` 0.2.0: acht Fristen nach §2.4 (Reaktion erfüllt, sobald die Störung „Neu“
    verlässt; Lösung pausiert in „Wartet“), P1 rund um die Uhr, P2 bis P4 in der Servicezeit mit
    11 Stunden je Tag; je Frist eine Eskalationsstufe bei 80 % an `actors.group.responsible`;
    automatisch schließen 5 Tage der Servicezeit (3.300 Minuten) nach „Gelöst“.
- **Belegt:**
  - Go `TestIncidentDeadlinesOverHTTP` (eigene Datenbank): `sd` legt 27, `incident` 18 Einträge
    an, ein zweites Installieren keinen, ein gelöschter Wartegrund bleibt gelöscht; P2 am Freitag
    6.11. um 17:00 hat die Reaktion um 17:30 und die Lösung am Montag um 14:00, die Liste zeigt
    17:30; die Benachrichtigung an `actors.group.responsible` erreicht die Leiterin Lena; bei 80 %
    der Reaktion geht genau eine Mail an Lena; nach dem Beginnen ist die Reaktion erfüllt; Warten
    von 9 bis 11 Uhr verschiebt die Lösung auf 16:00; gelöst am Montag um 12:00 schließt sie am
    Montag darauf um 12:00; P1 am Freitag um 17:00 hat die Lösung um 21:00; P3 an Heiligabend um
    17:00 hat die Reaktion am Montag 28.12. um 10:00; `actors.requester.responsible` gibt 422; ein
    neuer Mandant bekommt 45 Einträge und die Regeln mit seinem Namen.
  - Go `TestStartDataLoadsChecksAndResolves`: Reihenfolge der Dateien, Verweise, und sieben falsche
    Einträge mit ihrer Meldung.
  - Ganze CI am 2026-11-03 grün im ersten Lauf (10:27 min): alle 19 Schritte, 92 Vitest-Fälle,
    17 Browser-Tests, LZ1 p95 3,18 ms, LZ13 7.979 Zeilen/s.
- **Nicht geprüft:** Kein Browser-Test, die Oberfläche zeigt nichts Neues (die Fristspalte aus UI34
  zeigt die Fristen). Den Server erreichen neue Fristen und Regeln nach dem Installieren erst nach
  höchstens 30 Sekunden, weil das Kommando die laufenden Instanzen nicht benachrichtigt. Einstellbar
  je Organisationseinheit (automatisches Schließen, Matrix) wird es mit SD20.

### SD2 — Störung, 2026-11-03

- **Grundlage:** E3, E5 dieses Plans; Hauptplan E159; deine Entscheidungen vom 2026-11-03.
- **Gebaut:**
  - Paket `packages/incident` (hängt von `core` und `sd` ab): Typ `incident.incident` mit dem
    Merkmal `sd.ticket` (Felder und Rollen aus `sd`), laufende Nummer `INC-` (PL49), Priorität P1
    bis P4 als berechnetes Feld aus Auswirkung und Dringlichkeit nach §2.4, Lösung (formatiert),
    Lösungsart, Wartegrund; Status Neu, In Arbeit, Wartet, Gelöst, Geschlossen; Übergänge
    Beginnen, Warten, Fortsetzen, Lösen, Wieder öffnen (nur aus Gelöst), Schließen, „Lösung
    bestätigen“ und „Lösung ablehnen“ nur für den Anforderer, Ablehnen mit Pflicht-Begründung;
    Gelöst braucht Lösung und Lösungsart, Wartet einen Wartegrund, Geschlossen ist schreibgeschützt;
    im Portal Titel, Beschreibung und Kategorie.
  - Kern-Bedarf N7: `traits: [sd.ticket]` holt das Merkmal aus einem installierten Paket, von dem das
    Paket abhängt (`meta.ResolveTraits`, beim Installieren und im Test-Installer); sonst eine klare
    Meldung, welches Paket fehlt.
  - Kern-Bedarf N8: Übergänge mit `by: [rolle]` darf nur auslösen, wer (selbst oder über eine
    Gruppe) die Rolle am Datensatz hält, ohne weiteres Recht am Typ, ohne Felder; sonst 403, die
    Liste der Übergänge sagt `reason: role`. `comment: required|optional` nimmt einen Text an,
    der als Eintrag in derselben Schreibaktion entsteht (`comment` im Aufruf `:transition`).
    Der Arbeitsplatz zeigt Übergänge fremder Rollen nicht; das Portal zeigt die Übergänge der
    eigenen Rollen unter „Ihre Rückmeldung“ und fragt bei Pflicht-Begründung erst den Text ab.
- **Belegt:**
  - Go `TestIncidentOverHTTP` (eigene Datenbank): der Typ hat Kategorie und Rollen aus `sd`;
    Auswirkung mittel und Dringlichkeit hoch ergeben P2, Nummer 1; „Lösen“ ohne Lösung 422 am Feld
    `solution`; der Bearbeiter darf nicht bestätigen (403, `reason: role`); die Anforderin sieht
    Bestätigen und Ablehnen, Ablehnen ohne Grund 422 am Feld `comment`, mit Grund wieder „In Arbeit“
    und der Grund als Eintrag von ihr; nach erneutem Lösen bestätigt sie, „Geschlossen“; Wieder
    öffnen danach 409, Ändern 422. `TestAPICoverage` prüft jetzt 21 Typen, darunter
    `incident.incident` und die drei Listen von `sd`.
  - Vitest (`portal/PortalSolution.test.tsx`, 2 Fälle): im Portal „Ihre Rückmeldung“ mit beiden
    Knöpfen, „Lösung ablehnen“ fragt die Begründung ab, ohne Text gesperrt, axe ohne Befund, mit
    Text wieder „In Arbeit“; im Arbeitsplatz fehlt der Knopf einer fremden Rolle.
  - Ganze CI am 2026-11-03 grün im ersten Lauf (11:33 min): alle 19 Schritte, 92 Vitest-Fälle,
    17 Browser-Tests, LZ1 p95 3,58 ms, LZ13 7.832 Zeilen/s.
- **Nicht geprüft:** Kein Browser-Test: die CI-Instanz installiert `sd` und `incident` noch nicht.
  Die Benachrichtigung des Bearbeiters bei einer Ablehnung braucht eine Benachrichtigung am Ereignis
  `comment` (WF15); die Vorlagen bringt SD10. Die Fristen der Störung kommen mit SD3.

### SD1 — Grundpaket `sd`, 2026-11-03

- **Grundlage:** E1, E4 dieses Plans; Hauptplan E158; deine Entscheidungen vom 2026-11-03.
- **Gebaut:**
  - Paket `packages/sd`: Listen `sd.category` (Baum, 17 vorläufige Einträge in fünf Bereichen),
    `sd.channel` (Telefon, Mail, Portal, Vor Ort, Überwachung) und `sd.solution_kind` (Behoben,
    Anleitung gegeben, Umgehungslösung, Kein Fehler, Duplikat, Nicht lösbar), alle mit deutschen
    und englischen Namen; Merkmal `ticket` mit Titel, Beschreibung (formatiert), Kategorie,
    Anfragequelle (`channel`, weil `source` die Systemspalte der Herkunft ist), Auswirkung und
    Dringlichkeit, dazu die Rollen Anforderer (`requester`, Person), Beobachter (`observer`,
    Person oder Gruppe), Bearbeiter (`assignee`, Person) und Gruppe (`group`, eine Gruppe).
  - Kern-Bedarf N5: ein Eintrag einer Auswahlliste ist für Personen an seinem Knoten und darunter
    sichtbar, an der Wurzel für alle (`authz.Filter` und `Permits`, Knoten der Person oder sonst der
    Mandant); wer ein eigenes Recht auf die Liste hat, sieht zusätzlich, was das Recht gibt.
  - Kern-Bedarf N6: Paket-Merkmale dürfen `actors:` mitbringen; der Typ bekommt die Rollen und das
    Kern-Merkmal `actors`, gleichnamige Rollen stoßen zusammen, die Rollen werden wie die eines Typs
    geprüft.
  - Startwerte einer Liste dürfen einen englischen Namen tragen (`name_en`); er wird als
    Übersetzung des Namens angelegt (Kern-Plan PL21).
  - N7 neu: Störung (SD2) braucht das Merkmal `ticket` aus einem anderen Paket; das baut SD2.
- **Belegt:**
  - Go `TestServiceDeskBaseOverHTTP` (eigene Datenbank): 17 Kategorien, „Hardware > Drucker“,
    englisch „Printer“; Anfragequelle und Lösungsart da; `/api/v1/traits` zeigt `ticket` mit
    Rollen; eine dritte Ebene „Hardware > Drucker > Laserdrucker“; eine Kategorie am Knoten Nord:
    eine Person aus Nord sieht sie, eine aus Süd nicht (Liste 18 statt 19, Einzelabruf 404), der
    Admin mit eigenem Recht sieht alle 19.
  - Go `TestTraitBringsRoles`: Rollen vom Merkmal, Kern-Merkmal `actors` dazu, Zusammenstoß,
    unbekannter Typ einer Rolle.
  - `authz` `TestTenantBoundary`: der Filter der Listen enthält jetzt den Knoten (angepasst, weil
    sich der Filter mit N5 absichtlich ändert).
  - `TestTenantBoundaryOverHTTP`: die Suche nach dem Ereignis blätterte nicht; `Events` gibt höchstens
    1.000, der Test bat um 100.000 und bekam 100 und fand das Ereignis nicht mehr, sobald mehr als
    100 davor lagen (angepasst: er blättert jetzt).
  - Ganze CI am 2026-11-03 grün im ersten Lauf (10:17 min): alle 19 Schritte, 90 Vitest-Fälle,
    17 Browser-Tests, LZ1 p95 3,14 ms, LZ13 7.954 Zeilen/s.
- **Nicht geprüft:** Die Oberfläche zeigt nichts Neues; Kategorien legt man über die Liste an, die
  Sichtbarkeit über „Ablegen in“. Störung und Anfrage, die das Merkmal nutzen, folgen mit SD2 und
  SD6.
