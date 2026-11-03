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
| **SD3** | Fristen der Störung: Reaktion und Lösung je Priorität mit Kalender der Servicezeit, Eskalation an die Teamleitung bei 80 %, Wartegründe halten die Uhr an, automatisch schließen fünf Arbeitstage nach „Gelöst“; baut auf WF8 bis WF12 (E3; IP19) | ⬜ | 1 d | P1 um 08:00 anlegen → Lösungsfrist nach Kalender; „Wartet“ mit Grund „Rückmeldung Anforderer“ hält die Uhr an; mit Testzeit bei 80 % eine Mail an die Teamleitung in Mailpit; nach fünf Arbeitstagen „Gelöst“ → „Geschlossen“; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD4** | Betroffene Elemente und Bezüge: Beziehungsart „betrifft“ von Störung und Anfrage zu jedem Typ; „Duplikat von“ und „verwandt“ aus `core`; Verträge kommen mit CM (Inventar G49, G50) | ⬜ | 0,5 d | An einer Störung einen Datensatz eines beliebigen Typs als betroffen verknüpfen → steht in den Bezügen beider Seiten; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD5** | Major Incident: Übergang „zum Major Incident erklären“ mit Begründung, Mail an einen Verteiler, untergeordnete Störungen verknüpfen und mit dem Major Incident gemeinsam lösen, Hinweis im Portal (IP19; N3) | ⬜ | 1,5 d | Störung zum Major Incident erklären → Mail an den Verteiler; drei Störungen unterordnen, den Major Incident lösen → alle drei gelöst mit derselben Lösung, je eine Mail an ihre Anforderer; der Hinweis steht im Portal unter „Aktuell“; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD6** | Paket `request`: Typ Anfrage mit Merkmal `ticket`, Statusmodell mit Genehmigung, Erfüllungsaufgaben aus einer Vorlage, wiederkehrende Anfragen, Fristen wie SD3; baut auf WF6, WF13, WF14 (Inventar G36; IP30) | 🔒 WF6 | 1,5 d | Anfrage mit zweistufiger Genehmigung → nach beiden Zustimmungen entstehen zwei Aufgaben für die Gruppe; eine Ablehnung schließt die Anfrage mit Grund; `docker compose run --rm tools scripts/ci.sh` grün |

### B — Arbeitsplatz und Kanäle

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **SD7** | Warteschlangen: mitgelieferte geteilte Suchen „Meine“, „Mein Team“, „Nicht zugewiesen“, „SLA-Risiko“; Zuweisung per Regel nach Kategorie an eine Gruppe (IP28; KF7) | ⬜ | 1 d | Nach der Installation stehen die vier Suchen in der Navigation; Störung mit Kategorie „Netzwerk › VPN“ → Gruppe Netzwerk per Regel, der Verlauf nennt die Regel; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD8** | Textbausteine für Antworten, Aufgaben und Lösungen mit Platzhaltern wie in den Benachrichtigungen (Hauptplan E120), Auswahl über „Textbaustein“ im Editor, sichtbar je Gruppe (Inventar G56) | ⬜ | 1,5 d | Baustein „Protokoll anfordern“ mit `{{record.title}}` wählen → der Text steht mit eingesetztem Titel im Editor, nur mit Tastatur bedienbar; ein Baustein einer Gruppe erscheint nur bei ihren Mitgliedern; `axe` ohne Befund; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD9** | Telefonnotiz: Störung in einem Schritt anlegen mit Quelle „Telefon“, Anforderer per Suche, Rückruf als Aufgabe (IP28) | ⬜ | 0,5 d | „Telefonnotiz“ im Arbeitsplatz → Formular; mit Anforderer und Text anlegen → Störung mit Quelle „Telefon“ und Aufgabe „Rückruf“; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD10** | Mail hinaus: Vorlagen der Benachrichtigungen für Störung und Anfrage (angelegt, Antwort, Bitte um Bestätigung, gelöst, geschlossen) mit Kennung im Betreff; baut auf WF15 (Inventar G163; KF14) | ⬜ | 1 d | Antwort an den Anforderer → Mail in Mailpit mit der Kennung im Betreff (bis PL49 die Kurzkennung); interne Notiz → keine Mail; `docker compose run --rm tools scripts/ci.sh` grün |
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
[UI-Plan](02_oberflaeche.md) und kommt nach SD7. SD2 am 2026-11-03 von 2 d auf 3 d für die Kern-Bedarfe N7 und N8 (E5): 31 d.

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
| P und O | SD8, SD9, SD13, SD14, SD18, SD19, SD21 |
| K | SD5 (wenn N3 eine Regelaktion braucht), SD11, SD12, SD15, SD20 (Schlüssel der Einstellungen in `internal/prefs`) |

### 2.3 Bedarf an den Kern

| # | Bedarf | Befund | Vorschlag |
| --- | --- | --- | --- |
| **N1** | Laufende Nummer je Typ mit Präfix, etwa `INC-24817` | Der Kern kennt die Feldtypen text, number, boolean, date, datetime, choice und reference (`internal/meta/model.go:49` bis `:57`), keinen Nummernkreis | Feldtyp `sequence` mit Präfix, fortlaufend je Typ und Mandant: Kern-Plan PL49 (1 d), steht vor SD2 |
| **N2** | Zugang ohne Anmeldung für anonyme Formulare | Ohne Sitzung erreichbar sind nur Version, Anmeldung und Token (`internal/httpapi/auth.go:37`) | ein eigener Endpunkt nur zum Abrufen und Absenden freigegebener Formulare, mit Captcha und Ratenbegrenzung; gebaut in SD15 |
| **N3** | Gemeinsam lösen in SD5 | Nicht geprüft, ob eine Regel einen Übergang an verknüpften Datensätzen auslösen kann | in SD5 klären; sonst über `:batch` (Hauptplan E126) aus der Oberfläche |
| **N4** | Mail hinein | Kein Paket im Kern holt Mail ab; Mailpit im Profil `mail` kann nur SMTP (`compose.yaml:135`) | neues Paket `internal/mailin`; IMAP-Bibliothek und Testserver wählt SD11 nach Lizenzprüfung (PL29) |
| **N5** | Sichtbarkeit von Listeneinträgen je Organisationseinheit (E4) | Listeneinträge darf jeder lesen (implizites Lesen, Kern-Plan PL16); der Ort `org_path` eines Eintrags schränkt nichts ein | beim Lesen von Listen nur Einträge, deren Knoten über oder auf dem Knoten der Person liegt; gebaut in SD1 |
| **N6** | Rollen an Paket-Merkmalen (E4) | Paket-Merkmale (`traits/*.yaml`, Kern-Plan PL17) bündeln nur Felder; Rollen stehen unter `actors:` am Typ (WF3) | Paket-Merkmale dürfen `actors:` mitbringen, der Typ bekommt sie dazu; gebaut in SD1 |
| **N7** | Ein Merkmal eines anderen Pakets nutzen (SD2) | Ein Typ findet Merkmale nur im Kern und im eigenen Paket (`meta.expandTraits`, `p.Trait(name)`); `incident` braucht `ticket` aus `sd` | Merkmale mit Paketnamen (`traits: [sd.ticket]`) aus installierten Paketen auflösen, das Paket hängt dann von `sd` ab; bauen in SD2 |
| **N8** | Übergänge, die nur eine Rolle auslösen darf, und ihre Knöpfe im Portal (E5) | Bedingungen eines Übergangs sehen nur den Datensatz (`record`), nicht die handelnde Person; das Portal zeigt keine Übergänge (`web/src/portal/Portal.tsx`) | `by: [rolle]` am Übergang, der Kern prüft, dass die Person (oder eine ihrer Gruppen) die Rolle hält; ein Übergang mit `comment: required` verlangt einen Eintrag; das Portal zeigt die Übergänge, die die Person auslösen darf; gebaut in SD2 |

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
| **V1** 🟡 | Gemeinsam lösen beim Major Incident: als Regelaktion im Kern oder als Massenaktion aus der Oberfläche (N3)? Klärt die Umsetzung in SD5 | SD5 |
| **V2** 🟡 | Welche IMAP-Bibliothek und welcher Testserver, beide mit erlaubter Lizenz (PL29)? Klärt die Umsetzung in SD11 | SD11 |

Die Fragen an die Fachseite zu diesem Plan stehen im Hauptplan §6.2: V29 (Mailsystem), V30
(Matrix der Priorität), V31 (Servicezeiten und Fristen), V32 (Kategorien), V33 (Umfragen), V34
(anonyme Formulare), V35 (Major Incident), V36 (Telefonanlage), V37 (Datenübernahme, mit
Hauptplan H15).

## 4. Umsetzungsstand

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
