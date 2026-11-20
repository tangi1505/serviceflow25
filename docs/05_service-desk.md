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
| **SD6** | Paket `request`: Typ Anfrage mit Merkmal `ticket`, Statusmodell mit Genehmigung, Erfüllungsaufgaben aus einer Vorlage, wiederkehrende Anfragen, Fristen wie SD3; baut auf WF6, WF13, WF14 (Inventar G36; IP30) | 🟢 | 2,5 d (vorher 1,5 d) | Anfrage mit zweistufiger Genehmigung → nach beiden Zustimmungen entstehen zwei Aufgaben für die Gruppe; eine Ablehnung schließt die Anfrage mit Grund; `docker compose run --rm tools scripts/ci.sh` grün |

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
| **SD13** | Servicekatalog: Paket `catalog` mit Service und Angebot (Formular, Zielgruppe nach Organisationseinheit und Gruppe, Lieferzeit, Kosten, übersetzbar); Katalogseite im Portal mit Kategorien und Suche; eine Bestellung legt eine Anfrage an (Inventar G68, G133; IP24) | 🟢 | 3 d (vorher 2 d) | Angebot „Zugang Gruppenlaufwerk“ nur für die Dienststelle Nord → nur dort im Katalog; bestellen → Anfrage mit den Antworten; bei englischer Sprache englisch; `axe` ohne Befund; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD14** | Portal-Startseite: Kacheln (Formular, Katalogseite, Artikel, externer Link) je Zielgruppe; „Aktuell“ zeigt veröffentlichte Hinweise mit Ablauf (Inventar G78; IP28) | 🟢 | 1,5 d | Kachel für eine Gruppe freigeben → nur ihre Mitglieder sehen sie; einen Hinweis veröffentlichen → steht unter „Aktuell“, nach dem Ablauf nicht mehr; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD15** | Anonyme Formulare: Zugang über einen Link ohne Anmeldung, Captcha im Werkzeug ohne fremden Dienst, Ratenbegrenzung, Rückmeldung an eine Mail-Adresse (N2; Inventar G134, G137; Hauptplan E26; trägt KF8 für „anonym“). Hauptplan E190 („kein Aufruf der API ohne Konto“) meint Agenten und die Datensatz-API; freigegebene Formulare mit Captcha zählen nicht dazu. Der Endpunkt kommt auf die Liste der Pfade ohne Anmeldung, die AP23 prüft | 🔒 Hauptplan V34 | 2 d | Formular als anonym freigeben → der Link öffnet es ohne Anmeldung; ohne gelöstes Captcha → 400; 20 Absendungen in einer Minute → 429; Bestätigung per Mail in Mailpit; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD16a** | Wissen, Artikel im Portal: Paket `knowledge` mit Artikel (Titel, formatierter Text, Kategorie der Störung, „Sichtbar für“ in drei Stufen mit der Rolle „Gruppen“, Status Entwurf, Veröffentlicht, Abgelaufen); Leser-Regeln im Kern (N12); Bereich „Wissen“ im Kopf des Portals mit Kategorienbaum, Suche und Artikelseite (E8, Variante B des Mockups); dazu der Befund des Klicktests: Verweise zeigen im Portal und in der Zeitleiste den Namen statt der Kennung (Inventar G106; IP4) | 🟢 | 2 d | Ein veröffentlichter Artikel „Alle, auch im Portal“ → ein Portal-Nutzer sieht ihn unter „Wissen“, einen Entwurf und einen „Nur Bearbeiter“ nicht; einer für eine Gruppe → nur ihre Mitglieder; im Portal zeigt eine Störung ihre Kategorie mit Namen; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD16b** | Wissen, Fassungen: die Versionen eines Datensatzes (PL19) als Reiter „Fassungen“ auf der Detailseite, zwei Fassungen nebeneinander mit markierten Wörtern, eine ältere wiederherstellen; für jeden Typ mit Verlauf (E8, E9; IP4) | 🟢 | 1 d | Einen Artikel zweimal ändern → Fassung 1 und 3 vergleichen zeigt beide Änderungen; Fassung 1 wiederherstellen → Fassung 4 mit dem alten Text; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD16c** | Wissen, Sprache: Inhalte in der Sprache der Person zeigen (Listen, Namen, Suche, Portal), Titel und Text des Artikels übersetzbar; das Formular bleibt auf Deutsch, eine Version; automatische Übersetzung später (E10; PL21; IP4) | 🟢 | 1 d | Ein Artikel mit englischem Titel → wer Englisch eingestellt hat, sieht ihn englisch, sonst deutsch; das Formular zeigt immer den deutschen Titel; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD16d** | Wissen, Ablauf: Rolle „Autor“, die beim Anlegen die anlegende Person bekommt; ab dem Tag nach „Gültig bis“ ist der Artikel im Portal weg, eine tägliche Regel setzt ihn auf Abgelaufen und schreibt dem Autor, 14 Tage vorher erinnert sie ihn; dafür kennen Bedingungen `today()` und `now()` mit Abstand (E8, E11; IP4) | 🟢 | 1 d | Artikel mit „Gültig bis“ gestern → sofort aus dem Portal verschwunden, nach dem Lauf der Regel abgelaufen, Mail an den Autor in Mailpit; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD17** | Wissen, Freigabe: eine neue Fassung ist erst nach der Genehmigung sichtbar; baut auf WF6 (IP4; KF4) | 🟢 | 1,5 d (vorher 0,5 d) | Fassung 2 bleibt im Portal unsichtbar, bis die Freigabe erteilt ist; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD18** | Wissen am Vorgang und im Portal: Vorschläge beim Anlegen im Portal und an der Störung aus der Volltextsuche (PL26), Artikel verknüpfen, „hat geholfen“, Kommentare (Inventar G52, G108; IP4) | 🟢 | 3 d | Im Portal „VPN bricht ab“ tippen → ein passender Artikel als Vorschlag neben dem Formular; „Das hat geholfen“ zählt und legt keine Störung an; an der Störung im Reiter „Wissen“ einen Artikel verknüpfen → steht in den Bezügen; „Als Lösung übernehmen“ → Lösung und Lösungsart im Formular; ein Portal-Nutzer kommentiert einen Artikel; `docker compose run --rm tools scripts/ci.sh` grün |

### D — Zufriedenheit, Einstellungen, Abschluss

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **SD19** | Umfragen: Fragebogen mit dem Formular-Baukasten (Hauptplan E133), Versand nach dem Schließen mit Anteil und Verzögerung, Antwort über den Link in der Mail, Ergebnis am Vorgang und als Kennzahl (Inventar G47, G65; trägt KF15) | 🔒 Hauptplan V33 | 2 d | Anteil 100 %, Verzögerung 0 → nach dem Schließen eine Mail mit Link; die Antwort steht am Vorgang; der Durchschnitt ist über `:query` mit Aggregat abfragbar; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD20** | Einstellungen des Service Desks am Organisationsbaum: Matrix der Priorität, Anhänge (Größe, Arten), automatisches Schließen, Anteil der Umfragen (Inventar G173; Hauptplan E131, E180; den Anteil der Umfragen erklärt SD19, E17) | 🟢 | 2,5 d (vorher 1 d) | Matrix für die Dienststelle Nord ändern → dort die neue Priorität, sonst die alte; `docker compose run --rm tools scripts/ci.sh` grün |
| **SD21** | Durchgang Meilenstein 1 als Browser-Test: Anmeldung per OIDC, Störung über das Portal, Bearbeitung mit Frist, Anfrage mit Genehmigung, Wissensartikel als Lösung; alles Konfiguration (E2; Hauptplan Etappe 2) | 🟢 | 1,5 d | Der Browser-Test des Durchgangs in `docker compose run --rm tools scripts/ci.sh` ist grün; `git grep -n -e '"incident\.' -e '"request\.' -e '"catalog\.' -e '"knowledge\.' -e '"sd\.' -- internal web/src ':!*_test.go' ':!*.test.tsx'` gibt nichts aus |

**Summe:** 30 d, rund 6 Wochen, geschätzt vor dem Bau; dazu N1 aus §2.3 als PL49 im
[Kern-Plan](01_plattform-kern.md) (1 d). N2 und N4 bauen SD15 und SD11 selbst. Das Helpdesk-Dashboard ist UI14 im
[UI-Plan](02_oberflaeche.md) und kommt nach SD7; seit 2026-11-13 geteilt, die Dashboards selbst sind UI14b (UI-Plan E40) und gebaut: „Helpdesk“ und „Statistik“ kommen mit dem Paket Service Desk, Störung und Anfrage füllen sie. SD2 am 2026-11-03 von 2 d auf 3 d für die Kern-Bedarfe N7 und N8 (E5): 31 d. SD3 am 2026-11-03 von 1 d auf 2 d für die Kern-Bedarfe N9 und N10 (E6): 32 d. SD4 am 2026-11-03 von 0,5 d auf 1,5 d für N11 und den Reiter Bezüge (E7): 33 d. SD9 am 2026-11-04 von 0,5 d auf 1,5 d, weil Notizen beliebig sein sollen: Formulare auch im Arbeitsplatz (Hauptplan E167): 34 d. SD16 am 2026-11-04 in SD16a bis SD16d geteilt, von 2 d auf 5 d (E8, Hauptplan E168): 37 d. SD16b brauchte dazu im Kern nur den Namen in der Liste der Fassungen. SD16c ohne Editor für Englisch (E10); die automatische Übersetzung wird eine eigene Aufgabe, sobald Hauptplan V40 beantwortet ist. SD18 am 2026-11-10 von 2 d auf 3 d für die Kern-Bedarfe N13 bis N16 (E12): 38 d. SD6 am 2026-11-10 von 1,5 d auf 2,5 d für N17 und N18 und die Stufen in der Vorlage (E14): 39 d. SD13 am 2026-11-10 von 2 d auf 3 d für N19 bis N21 (E15): 40 d. SD17 am 2026-11-11 von 0,5 d auf 1,5 d für N22 und den Status „In Prüfung“ (E16): 41 d. SD20 am 2026-11-13 von 1 d auf 2,5 d für N23 bis N25 und die Seite mit einer Leiste zum Speichern (E17): 42,5 d.

**Reihenfolge:** Block A zuerst, weil alles andere die Typen braucht: SD1, SD2, SD3, SD4, dann
SD5. SD6 und SD13 (Katalog) sind seit 2026-11-10 gebaut, SD17 (Freigabe von Wissen) seit 2026-11-11. Danach Block B mit SD7, SD10, SD8 und SD9; SD11 und
SD12 erst, wenn die Fachseite Hauptplan V29 beantwortet hat. Block C mit SD16a bis SD16d und SD18 vor SD13
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
| **E8** | Wissen (SD16, deine Entscheidungen 2026-11-04 nach dem Mockup „Wissen im Portal“, [Artifact](https://claude.ai/artifact/At2t1hm51eJPvdGF1m9jGz), und dem Klicktest im Browser; Hauptplan E168): **Vier Teile** SD16a bis SD16d statt einer Aufgabe mit 2 d. **Sichtbarkeit wie bei Auswahllisten:** ein Typ bringt Leser-Regeln mit (`readers`), nach denen angemeldete Personen ohne eigenes Recht lesen (Kern-Bedarf N12). „Sichtbar für“ hat drei Stufen: „Alle, auch im Portal“ liest jeder an seinem Knoten des Artikels und darunter, sobald er veröffentlicht ist; „Nur bestimmte Gruppen“ lesen die Mitglieder der Gruppen in der Rolle „Gruppen“, wo sie auch sind; „Nur Bearbeiter“ lesen die, denen ihre Rollen es erlauben. **Status** Entwurf, Veröffentlicht, Abgelaufen; „Gültig bis“ bekommt in SD16d eine tägliche Regel. **Kategorien** sind die der Störung (`sd.category`). **Portal** nach Variante B: „Wissen“ als eigener Bereich im Kopf neben „Meine Vorgänge“, mit Kategorienbaum, Suche und Artikelseite; die Startseite bleibt: eine Karte je Vorgangsart, danach „Formulare“, auf dem Handy untereinander. **Befund des Klicktests:** Verweise zeigen im Portal und in der Zeitleiste den Namen, behoben in SD16a. Verworfen: eine Aufgabe; ein Recht je Artikel; eigene Kategorien des Wissens; Karte „Wissen“ auf der Startseite (Variante A); „Formulare“ immer an zweiter Stelle; der Befund als eigene Aufgabe | 2026-11-04 | Pascal |
| **E9** | Fassungen (SD16b, deine Entscheidungen 2026-11-04 nach dem Mockup „Fassungen vergleichen“, [Artifact](https://claude.ai/artifact/Nqgtdo42n6L56GMeQNxc1G), Hauptplan E169): **Variante A**, ein Reiter „Fassungen“ zwischen „Bezüge“ und „Zeitleiste“: links die Fassungen (wann, wer, was), zwei angeklickt, rechts nur die Felder, die sich unterscheiden. **Wörter** eines Textes markiert ein eigener kleiner Vergleich im Browser, ohne neue Bibliothek: gestrichen, was nur die ältere hat, unterstrichen, was neu ist. **Jeder Typ mit Verlauf** bekommt den Reiter, im Portal keiner. Wiederherstellen erst nach einer Rückfrage auf der Seite. Verworfen: eigene Seite mit dem ganzen Datensatz zweimal (Variante B); die Bibliothek „diff“; nur alter und neuer Text; nur am Artikel | 2026-11-04 | Pascal |
| **E10** | Sprache der Inhalte (SD16c, deine Entscheidungen 2026-11-04 nach dem Mockup „Übersetzungen im Formular“, [Artifact](https://claude.ai/artifact/C8r9bReahYm2tT2DgmNGik), Hauptplan E171): **Eine Version:** kein Editor für Englisch im Formular; wo eine Übersetzung nötig wird, soll sie später automatisch entstehen, mit einem Übersetzer im eigenen Rechenzentrum (VS-NfD, kein Cloud-Dienst) als eigene Aufgabe, nachdem die Fachseite gesagt hat, ob Englisch gebraucht wird (Hauptplan V40). **Anzeige** in der Sprache der Person in Listen, Namen, Suche und Portal, sonst Deutsch; das Formular liest und schreibt Deutsch. **Übersetzbar** sind am Artikel Titel und Text. Verworfen: Umschalter „Deutsch | Englisch“ am Formular (Variante A); Englisch je Feld (Variante B); Anzeige nur im Portal; nur der Text übersetzbar; Automatik jetzt | 2026-11-04 | Pascal |
| **E11** | Ablauf des Wissens (SD16d, deine Entscheidungen 2026-11-04, Hauptplan E172): **„Jetzt“ in Bedingungen** als `today()` (Datum in Europe/Berlin) und `now()` (Zeitpunkt), mit Abstand `today(14)`, `today(-30)`, `now("-24h")`; gleich in SQL (Listen, Leser-Regeln, Rechte, Regeln nach Zeitplan) und im Speicher. **Autor** ist eine Rolle am Artikel (eine Person); eine mitgelieferte Regel trägt beim Anlegen die anlegende Person ein, wenn das Formular keinen nennt; dafür kann „Rolle zuweisen“ auch die Person, die schreibt (`actor`). **Mails** an den Autor 14 Tage vor „Gültig bis“ und beim Ablauf, als Startdaten. **Portal:** ab dem Tag nach „Gültig bis“ ist der Artikel weg, über die Leser-Regel, auch bevor die nächtliche Regel ihn auf Abgelaufen setzt. Verworfen: nur `today()` und `now()` ohne Abstand; `"@now"` wie in Abfragen; Autor nur von Hand; Mail an den Ersteller (Systemfeld); nur eine Mail beim Ablauf; Portal erst nach der Regel | 2026-11-04 | Pascal |
| **E12** | Wissen am Vorgang und im Portal (SD18, deine Entscheidungen 2026-11-10 nach dem Mockup „Wissen am Vorgang“, [Artifact](https://claude.ai/artifact/BoRXwWtkH91hBXN5gXnkZa), Hauptplan E173): **Vorschläge sagt der Artikel:** ein Typ nennt unter `suggest`, bei welchen Typen er vorgeschlagen wird (auch `trait:sd.ticket`), aus welchen ihrer Felder gesucht wird, mit welcher Beziehungsart „Verknüpfen“ arbeitet und was „Als Lösung übernehmen“ setzt; Störung und Anfrage kennen das Wissen nicht (Kern-Bedarf N13). Gesucht wird mit jedem Wort statt mit allen, beste Treffer zuerst, nur was die Person lesen darf. **Portal (Variante 1A):** beim Anlegen stehen bis zu drei Artikel neben dem Formular, auf dem Telefon darunter; aufklappen zeigt den Text; „Das hat geholfen“ zählt ein Ja und legt keine Störung an, „Hilft nicht“ zählt ein Nein. **Arbeitsplatz:** ein eigener Reiter „Wissen“ an jedem Typ, bei dem etwas vorgeschlagen wird, mit Vorschlägen aus den Feldern des Datensatzes und einer eigenen Suche; je Artikel „Verknüpfen“ und „Als Lösung übernehmen“. **Als Lösung übernehmen** setzt Text und Lösungsart „Anleitung gegeben“ ins Formular (gespeichert erst mit „Speichern“) und verknüpft; welche Felder, legt das Paket fest. **Verknüpfen** mit der Beziehungsart „nutzt Wissen“ (invers „genutzt bei“) aus dem Paket `knowledge`; dafür braucht nur der Vorgang das Recht zum Ändern, am Artikel genügt Lesen (N16). **Hat geholfen** ist ein Merkmal `rating` im Kern: Ja oder Nein je Person, änderbar; Bearbeiter sehen „12 von 15 hilfreich · an 7 Vorgängen genutzt“ (N14). **Kommentare** über die Zeitleiste des Artikels; eine Leser-Regel darf neben dem Lesen auch das Kommentieren erlauben; öffentliche Kommentare sehen alle Leser unter dem Artikel im Portal, interne nur Bearbeiter (N15). Verworfen: der Vorgang nennt die Quelle seiner Vorschläge; ein Suchschritt vor dem Formular (Variante 1B); Vorschläge im Reiter „Bezüge“ oder neben dem Formular; nur ein Verweis oder nur Verknüpfen statt „Als Lösung übernehmen“; eine gelöste Störung bei „Das hat geholfen“; nur ein Zähler ohne Nein; Sterne; Kommentare nur intern oder später | 2026-11-10 | Pascal |
| **E17** | Einstellungen am Organisationsbaum (SD20, deine Entscheidungen 2026-11-13 nach dem Mockup „Einstellungen am Organisationsbaum“, [Artifact](https://claude.ai/artifact/JBJPky5vUxpAQpn6PTcMLq), Hauptplan E180): **Pakete erklären ihre Einstellungen** in `package.yaml` (`settings`: Schlüssel, Art, Bezeichnung, Gruppe, Vorgabe, Grenzen; Arten Auswahl, Text, Zeitzone, Zahl, Ja/Nein, Dauer in Minuten, Größe in Bytes, Liste, Matrix, Verweis; „nur an der Wurzel“); der Kern erklärt seine 32 Schlüssel im Paket `core`. **Nur erklärte Schlüssel** nimmt der Server, jeder Wert wird gegen die Erklärung geprüft (422), Entfernen geht immer. **Matrix der Priorität** ist die Einstellung `incident.priority_matrix`, die Formel liest sie mit `setting("…")` an der Einheit des Vorgangs; offene Störungen rechnen beim nächsten Speichern neu, die Störung zeigt keine Herkunft der Matrix. **Automatisches Schließen** über `incident.close_after`: jede Frist kann ihr Ziel aus einer Einstellung nehmen (`target_setting`), 0 heißt keine Uhr. **Anhänge**: Größe und Endungen an der Einheit der Person, die hochlädt, neu eine Liste erlaubter Endungen (leer: alles außer den gesperrten). **Seite „Globale Einstellungen“** mit einem Abschnitt je Gruppe, einem Feld je Art, der Herkunft unter jeder Einstellung und einer festen Leiste oben mit „Verwerfen“ und „Speichern“ (dein Kommentar am Mockup): alles ändern, dann einmal speichern, alles oder nichts. Den **Anteil der Umfragen** erklärt SD19, weil es Umfragen noch nicht gibt. Verworfen: eigener Datensatz für die Matrix; offene Störungen sofort neu rechnen; nur das Schließen einstellbar; eine zweite Frist je Einheit; Anhänge nur an der Wurzel; ein fester Abschnitt nur für den Service Desk; unbekannte Schlüssel weiter zulassen; Hinweis zur Herkunft an der Priorität | 2026-11-13 | Pascal |
| **E16** | Freigabe von Wissen (SD17, deine Entscheidungen 2026-11-11 nach dem Mockup „Freigabe von Wissen“, [Artifact](https://claude.ai/artifact/AbcVmjcAadRKWDayxuUyba), Hauptplan E178): **Die Änderung wartet auf Genehmigung:** wer Titel, Text oder „Sichtbar für“ eines veröffentlichten Artikels ändert, speichert einen Vorschlag an einer Genehmigung; der Artikel bleibt, wie er ist, bis die Genehmigung erteilt ist, dann wird die Änderung die neue Fassung; abgelehnt oder zurückgezogen fällt sie weg. Kategorie, „Gültig bis“ und Rollen ändern sich sofort. Allgemein im Kern für jeden Typ mit Genehmigungen (Kern-Bedarf N22). **Genehmiger** ist die Gruppe „Wissensredaktion“ aus den Startdaten in der neuen Rolle „Redaktion“ des Artikels; ein Mitglied entscheidet. **Auch die erste Fassung:** „Zur Freigabe vorlegen“ führt in den neuen Status „In Prüfung“, die Zustimmung veröffentlicht, die Ablehnung führt mit Grund zurück in den Entwurf; „Veröffentlichen“ von Hand gibt es nicht mehr. **Oberfläche:** ein Band am Artikel („wartet auf Freigabe“, „Ansehen“, „Änderung zurückziehen“), im Reiter „Genehmigung“ die beantragte Änderung Wort für Wort wie bei den Fassungen; solange sie wartet, sind ihre Felder gesperrt (Variante 2A). Verworfen: Überarbeitung als Kopie; Artikel verschwindet bis zur Freigabe; Genehmiger je Kategorie oder vom Autor gewählt; nur Änderungen nach der ersten Fassung; jede Änderung braucht Freigabe; Vorschlag weiter bearbeiten (2B); ohne Status „In Prüfung“ | 2026-11-11 | Pascal |
| **E15** | Servicekatalog (SD13, deine Entscheidungen 2026-11-10 nach dem Mockup „Servicekatalog“, [Artifact](https://claude.ai/artifact/R5HRiDjp7unY3bVNCEbeLV), Hauptplan E177): **Paket `catalog`** mit Katalogbereich (Auswahlliste als Baum, für die Bestellenden, nicht die Kategorien der Vorgänge), Service (Name, Beschreibung, Verantwortliche) und Angebot (Name und Beschreibung übersetzbar, Katalogbereich, Service, Formular, Erfüllungsvorlage, Lieferzeit in Arbeitstagen, Kosten in Euro mit Art einmalig, monatlich oder jährlich, „Sichtbar für“ wie bei den Kacheln). **Das Angebot nennt Formular und Vorlage:** Bestellen füllt das Formular des Angebots aus, die Anfrage bekommt Titel, Vorlage, Lieferzeit und Kosten vom Angebot und die Beziehung „bestellt aus“; Genehmigung und Aufgaben laufen wie bei jeder Anfrage mit Vorlage (Kern-Bedarf N19). **Die Lieferzeit ist die Erfüllungsfrist** (N20); ohne Angebot fünf Arbeitstage. **Kosten nur zur Anzeige,** keine Verrechnung. **Portal, Variante 1A:** „Katalog“ ist ein Bereich im Kopf wie „Wissen“, links die Katalogbereiche als Baum, rechts die Angebote als Karten mit Lieferzeit und Kosten, oben die Suche (N21); ein Angebot zeigt Beschreibung und Angaben und darunter gleich das Formular mit „Bestellen“; danach die Seite der Anfrage. Eine neue Kachel „Katalog“, „Etwas anfragen“ bleibt für alles ohne Angebot. Verworfen: nur Formular am Angebot (Vorlage als fester Wert); eigene Fragen am Angebot; Kategorien der Vorgänge oder Services als Gliederung; Lieferzeit nur als Angabe oder je Angebot eine Frist von Hand; Kosten als freier Text oder mit Kostenstelle; Bereiche als Kacheln (1B); erst Angebotsseite, dann Formular; „Etwas anfragen“ führt zum Katalog | 2026-11-10 | Pascal |
| **E14** | Anfrage (SD6, deine Entscheidungen 2026-11-10, Hauptplan E176): **Paket `request`** mit Anfrage (`request.request`, Nummer REQ-…, Merkmale `sd.ticket` und `approvals`), Erfüllungsaufgabe, Erfüllungsvorlage, Aufgabe und Genehmigungsstufe der Vorlage. **Statusmodell:** Neu, Wartet auf Genehmigung, In Erfüllung, Wartet, Erledigt, Geschlossen, Abgelehnt; die Genehmigung bewegt die Anfrage (angefordert, genehmigt, abgelehnt mit Grund), ohne Genehmigung geht „Erfüllung beginnen“. **Die Vorlage ist ein Datensatz** mit Aufgabenzeilen (Titel, Gruppe, Reihenfolge) und **nennt auch die Stufen** der Genehmigung (Name, nötige Zustimmungen, Genehmiger); nennt die Anfrage eine Vorlage, fordert eine Regel der Startdaten die Genehmigung an, beim Anlegen wie beim späteren Setzen; geht sie in Erfüllung, macht eine Regel je Zeile eine Aufgabe für die Gruppe der Zeile (Kern-Bedarf N17, N18). Damit ist von WF7 die Vorlage mit Stufen für die Anfrage vorgezogen; allgemeine Genehmigungsvorlagen und die Erinnerung bleiben in WF7. **Fristen:** eine für alle, vorläufig Reaktion ein Arbeitstag, Erfüllung fünf Arbeitstage in der Servicezeit, die Uhr hält beim Warten auf die Genehmigung und beim Warten; je Angebot mit SD13. **Portal:** Kachel „Etwas anfragen“ wie „Störung melden“. Wiederkehrende Anfragen über `core.recurrence` (WF14). Verworfen: je Vorlage eine feste Regel; Genehmigung nur von Hand bis WF7; Fristen nach Priorität; Anfragen im Portal erst mit dem Katalog | 2026-11-10 | Pascal |
| **E13** | Startseite des Portals (SD14, deine Entscheidungen 2026-11-10 nach dem Mockup „Startseite des Portals“, [Artifact](https://claude.ai/artifact/55ZVTdBSQhcsQP31sxtk9p), Hauptplan E174): **Anordnung A:** „Aktuell“ als Band über der ganzen Seite, darunter „Was möchten Sie tun?“ mit Kacheln, darunter „Meine Vorgänge“. **Kacheln ersetzen die Karte „Formulare“;** sieht eine Person keine Kachel, zeigt die Startseite die Formulare wie bisher. **Im Kern wie Formulare:** Typ `core.portal_tile` (Titel, Text, führt zu: Anlegen eines Typs, Formular, Bereich, Datensatz oder Link; Reihenfolge; „Sichtbar für“ wie beim Artikel, Alle am Ort der Kachel und darunter oder nur bestimmte Gruppen) und Typ `core.announcement` für Hinweise von Hand (Titel, Text, Gültig ab und bis mit Uhrzeit, „Sichtbar für“, Entwurf, Veröffentlicht, Abgelaufen); der Hinweis ist durchsuchbar, die Kachel nicht (deine Entscheidung nach der CI: eine Kachel sucht man kaum, und jeder durchsuchbare Typ kostet die Suche über alles etwas). Pakete bringen Kacheln als Startdaten mit: `incident` „Störung melden“, `knowledge` „Wissen“. „Aktuell“ zeigt zuerst die automatischen Hinweise (SD5), dann die von Hand, beide minutengenau. Verworfen: Aktuell als Spalte rechts (B); Formulare zusätzlich zu den Kacheln; beide Typen im Paket `sd`; Gültigkeit nur als Datum | 2026-11-10 | Pascal |

## 2. Begründung und Befunde

### 2.1 Zuordnung des Funktionsinventars

Alle 24 Zeilen mit Ziel SD haben eine Aufgabe:

| Inventar | Aufgabe | Inventar | Aufgabe |
| --- | --- | --- | --- |
| G36 | SD2, SD6 | G106 | SD16a |
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
- **Genehmigungen:** in Stufen mit Mindestzahl und Vertretung (WF6, Hauptplan E175).
- **Fehlt noch:** Vorlagen und Erinnerung für Genehmigungen (WF7), Dashboards (UI13).

Die Art sagt, welche Dateien eine Aufgabe anfasst: **P** nur Paketinhalt unter `packages/`,
**O** dazu `web/src`, **K** dazu Go-Code, API oder Migration.

| Art | Aufgaben |
| --- | --- |
| P | SD1, SD2, SD3, SD4, SD6, SD7, SD10, SD17 |
| P und O | SD13, SD14, SD16c (Lesen in der Sprache der Person, E10), SD18, SD19, SD21 |
| K | SD5 (wenn N3 eine Regelaktion braucht), SD8 (der Server setzt die Platzhalter ein, deine Entscheidung 2026-11-04), SD9 (Formulare im Arbeitsplatz, Regelaktion mit `@record` und `@actor`, deine Entscheidungen 2026-11-04), SD16a (Leser-Regeln im Kern, Lesebereich im Portal, E8), SD16b (Name in der Liste der Fassungen, E9), SD16d (Zeitregeln mit „jetzt“), SD11, SD12, SD15, SD20 (Einstellungen erklärt in den Paketen, N23 bis N25) |

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
| **N12** | Lesen ohne eigenes Recht nach Regeln des Typs (E8) | Ohne Recht lesen Personen nur die Einträge der Auswahllisten (`authz.Access.ReadAll`, N5) | Typ-Option `readers`: je Regel eine Bedingung in CEL, die nach SQL übersetzbar ist, und auf Wunsch Rollen; ohne Rollen lesen Personen am Knoten des Datensatzes und darunter, mit Rollen ihre Inhaber und die Mitglieder ihrer Gruppen; eine Auswahlliste ist eine Regel ohne Bedingung; gebaut in SD16a |
| **N13** | Vorschläge aus einem anderen Typ (E12) | Die Volltextsuche (Kern-Plan PL26) verlangt alle Wörter (`websearch_to_tsquery`); kein Typ kann sagen, wo er vorgeschlagen wird | Typ-Option `suggest` mit `for` (Typen oder `trait:`), `from` (Felder des Zieltyps), `relation` und `apply` (Felder des Zieltyps aus Feldern des Vorschlags oder Schlüsseln einer Auswahlliste); Endpunkt `POST /api/v1/records/{typeName}:suggest` mit den Werten des Formulars, Suche mit jedem Wort; gebaut in SD18 |
| **N14** | Hat geholfen (E12) | Es gibt keine Bewertung je Person | Merkmal `rating`: Ja oder Nein je Person und Datensatz, änderbar; `PUT` und `GET /api/v1/records/{typeName}/{id}/rating`; lesen darf, wer den Datensatz liest; gebaut in SD18 |
| **N15** | Leser kommentieren (E12) | Leser-Regeln (N12) erlauben nur das Lesen; ein Eintrag in der Zeitleiste braucht das Recht `comment` | Leser-Regeln nennen auf Wunsch `actions: [read, comment]`; private Einträge bleiben bei `read_private`; gebaut in SD18 |
| **N16** | Verknüpfen ohne Recht am Ziel (E12) | Eine Beziehung braucht das Recht zum Ändern an beiden Enden (`relations.Service.Create`) | Beziehungsart mit `update: from`: Ändern am Ausgang, Lesen am Ziel genügt; gebaut in SD18 |
| **N17** | Aufgaben aus einer Vorlage (E14) | Die Regelaktion „anlegen“ macht genau einen Datensatz mit festen Werten (`engine.Create`) | `create` mit `each` (`type_name`, `match`, `order`): ein Datensatz je Zeile eines anderen Typs, deren Felder in `match` dasselbe nennen wie der Datensatz; `@item` und `@item.<feld>` in Feldern und Rollen; je Regel, Datensatz und Zeile höchstens einmal (Schlüssel wie beim Bulk), auch wenn zwei Jobs die Regel gleichzeitig ausführen; gebaut in SD6 |
| **N18** | Genehmigung aus einer Vorlage (E14) | Eine Genehmigung fordert nur jemand mit dem Recht zum Ändern über die API an (WF6) | Regelaktion `approval` mit `stages` (wie `each`), `name`, `need` und der Rolle `approvers` der Zeile; ohne Zeile mit Genehmigern nichts, bei offener Genehmigung nichts; gebaut in SD6; seit WF7 (WF-Plan E18) nennt die Erfüllungsvorlage eine Genehmigungsvorlage des Kerns statt eigener Stufen, die Regeln fragen mit `template` |
| **N19** | Bestellen über ein Angebot (E15) | Ein Formular legt seinen Zieltyp an, nur mit seinen Antworten und festen Werten (`forms.Service.Submit`) | Block `order` an einem Typ (`form`, `copy`, `relation`): `POST /records/{typ}/{id}:order` liest das Angebot mit den Rechten der Person, reicht sein Formular ein mit den Werten, die `copy` vom Angebot übernimmt (vor den Antworten), und verknüpft die neue Anfrage mit dem Angebot; ein Formular mit „Angeboten im“ = „Nur über ein Angebot“ steht in keiner Liste und braucht keine Frage für Pflichtfelder, die das Angebot liefert; gebaut in SD13 |
| **N20** | Frist aus einem Feld (E15) | Das Ziel einer SLA ist eine feste Zahl Minuten (`core.sla.target_minutes`) | Feld `target_field` („Ziel mal Feld“): das Ziel ist das Ziel in Minuten mal der Zahl im Feld des Datensatzes, ohne Zahl das Ziel in Minuten; gilt, wenn die Uhr beginnt; gebaut in SD13 |
| **N21** | Angaben in der Liste eines Lesebereichs (E15) | Der Lesebereich des Portals zeigt Titel und Anriss, neueste zuerst (SD16a) | `portal.read.facts` (Felder je Eintrag, dann als Karten) und `portal.read.sort`; die Seite eines bestellbaren Datensatzes zeigt sein Formular; gebaut in SD13 |
| **N22** | Änderung wartet auf Genehmigung (E16) | Eine Änderung ist sofort geschrieben; eine Genehmigung bewegt nur den Status (WF6) | Block `hold` unter `approval` (`fields`, `when`, `approvers` als Rolle des Datensatzes, `message`): ändert ein Schreiben eines dieser Felder, solange `when` gilt, wird der Rest geschrieben und die Änderung dieser Felder an einer neuen Genehmigung gehalten (Spalte `approval.changes`, Migration 45), gefragt die Inhaber der Rolle in einer Stufe mit einer Zustimmung; erteilt wird sie mit Systemrechten und der fragenden Person als neue Fassung geschrieben, ohne Übergang; solange sie wartet, ändert ein weiteres Schreiben dieser Felder nichts (409). Dazu die Regelaktion `approval` mit `role` (Genehmiger aus einer Rolle des Datensatzes selbst); gebaut in SD17 |
| **N23** | Erklärte Einstellungen (E17) | Einstellungen am Organisationsbaum nehmen jeden Schlüssel und jeden JSON-Wert; die Seite „Globale Einstellungen“ kennt nur die fünf der Oberfläche | `settings` in `package.yaml` (`meta.Setting`), `internal/settings` (Erklärungen, Prüfung, Wert mit Vorgabe, Änderungen eines Knotens in einer Transaktion), `PATCH /api/v1/org/nodes/{path}/settings`, `GET /api/v1/settings/definitions`; gebaut in SD20 |
| **N24** | Einstellung in Formeln (E17) | Eine Formel sieht nur die Felder ihres Typs | `setting("schlüssel")` in Formeln: der Wert an der Einheit des Datensatzes oder darüber, sonst die Vorgabe; ein Schlüssel, den kein Paket erklärt, wird beim Installieren abgelehnt; gebaut in SD20 |
| **N25** | Frist aus einer Einstellung (E17) | Das Ziel einer SLA ist eine Zahl Minuten oder dieses mal ein Feld (N20) | Feld `target_setting` („Ziel aus Einstellung“): der Wert der Einstellung an der Einheit des Datensatzes in Minuten, wenn die Uhr beginnt; 0 heißt keine Uhr; gebaut in SD20 |

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

### SD21 — Durchgang Meilenstein 1 als Browser-Test, 2026-11-20

- **Grundlage:** SD-Plan E2 (alles Konfiguration); Hauptplan Etappe 2; deine Entscheidung vom
  2026-11-20: Anmeldung über den nachgebauten Anbieter der Tests, kein Keycloak in der CI.
- **Gebaut:**
  - `test/fakeidp`: der kleine OIDC-Anbieter der Go-Tests (`internal/authn/authntest`) als
    eigener Prozess auf einer festen Adresse; `POST /next` legt fest, wer sich als Nächstes
    anmeldet (Name, E-Mail, Gruppen). Nur für Tests, ohne Kennwort.
  - `scripts/sd21-instance.sh`: eine eigene Datenbank neben der der anderen Browser-Tests,
    frisch migriert, nur die Pakete `core`, `sd`, `incident`, `request`, `catalog` und
    `knowledge`, ein Administrator, der Anbieter auf 18083 und ein Server auf 18084, der über
    ihn anmeldet. Die CI startet sie vor den Browser-Tests (`scripts/ci.sh`); die anderen Tests
    bleiben unberührt (der Knopf „Anmelden mit …“ hätte ihre Anmeldung gestört).
  - `web/e2e/sd21.spec.ts`: Der Administrator richtet über die API ein, was ein Kunde
    einrichtet: drei Personen, die Rollen „Mitarbeitende (Portal)“ und „Servicedesk“, zwei
    Anmelderegeln nach Gruppen, eine Genehmigungsvorlage mit Gesa an „Neuer Arbeitsplatz“ und
    einen Artikel für alle mit Bernd in der Redaktion. Dann im Browser, jede Person über den
    Anbieter angemeldet: Bernd gibt den Artikel frei (Reiter „Genehmigung“, „Zustimmen“); Nora
    meldet im Portal über „Störung melden“ eine Störung, der Artikel steht als Vorschlag
    daneben; Bernd öffnet sie aus der Liste, setzt Auswirkung und Dringlichkeit hoch, sieht
    „Störung P1 Reaktion – noch 15 min“, beginnt, übernimmt im Reiter „Wissen“ den Artikel als
    Lösung, speichert und löst; Nora sieht die Lösung im Portal und bestätigt sie
    (geschlossen); Nora bestellt im Katalog „Neuer Arbeitsplatz“, die Anfrage wartet auf
    Genehmigung; Gesa stimmt im Portal zu; Bernd sieht die Anfrage „In Erfüllung“ mit den
    Aufgaben „Gerät bereitstellen“ und „Zugänge einrichten“. axe auf jeder Seite ohne Befund.
  - `scripts/check-core-packages.sh`, eigener Schritt jeder CI: kein Typ der Pakete steht im
    Code von Server und Oberfläche (die Prüfung dieses Plans, ohne Kommentare, die erzeugte
    API-Beschreibung und Tests). Ein Beispiel in einer Fehlermeldung der Formeln nannte
    `incident.priority_matrix`, jetzt `trash.keep_days`.
  - Behoben, weil axe es im Durchgang fand: grauer Text auf der hervorgehobenen eigenen Zeile
    der Genehmigung und der Ersatzname einer nicht lesbaren Person hatten zu wenig Kontrast
    (jetzt die zweite Textfarbe).
  - Nebenbei: die CI baut und testet auf ki-lab auf 14 statt 12 Threads (Kerne 0 bis 6, deine
    Angabe vom 2026-11-20: die anderen Projekte ruhen, llama.cpp läuft auf der GPU); die
    Lastmessung bleibt auf ihren 8 festen Threads. Schneller wurde die CI dadurch kaum
    (18 min), weil Lastmessung, Browser-Tests und die zeitabhängigen Go-Tests mit Absicht
    nacheinander laufen. Ein Test der Einstellungen suchte das Feld nach der Wahl von Englisch
    noch unter „Sprache“; er findet es jetzt unter beiden Namen.
- **Befunde des Durchgangs** (nicht behoben, zu deiner Entscheidung):
  - B1 Anmelderegel: Der Name muss eine Kurzform sein; die API meldet einen anderen Namen als
    Datenbankfehler mit SQL-Text statt am Feld.
  - B2 Artikel: Im Formular steht das Feld „Genehmigung“ als Textfeld mit dem Rohwert
    „pending“.
  - B3 Liste der Störungen: Die vorgegebenen Spalten beginnen mit Nummer, Priorität, Lösung,
    Lösungsart, Wartegrund; Titel und Status fehlen vorn.
  - B4 Seite der Störung: Titel und Beschreibung stehen unter den Feldern der Lösung; die
    Seitenleiste zeigt „INCIDENT.TASK“ als Überschrift.
  - B5 „Als Lösung übernehmen“ füllt nur das Formular; wer dann „Lösen“ drückt, bekommt
    „Übergang fehlgeschlagen: Eingabe ungültig“ ohne Hinweis auf das ungespeicherte Formular
    oder die fehlenden Felder.
  - B6 Portal: Die Zeitleiste einer Anfrage zeigt der Anwenderin „approval (Regel
    request_genehmigung_beim_anlegen)“.
  - B7 Portal: Den Zustand „Wartet auf Genehmigung“ setzt eine Regel im Hintergrund; die
    offene Seite zeigt ihn erst nach dem Neuladen.
  - B8 Portal: Die Genehmigerin sieht sich in der Tabelle der Genehmigung als „Person“ mit einer Kurzkennung,
    weil das Portal keine Personen lesen darf.
- **Belegt:** CI auf ki-lab am 2026-11-20: alle 35 Browser-Tests grün, der Durchgang in 11,6 s, der neue Schritt der Pakete grün; das Lasttor im letzten Lauf rot mit LZ16 +21 %, im Lauf davor mit gleichem Code grün (−2 %): die Basislinie hat jeweils einen schnellen ersten Lauf (2,98 bis 3,34 ms), danach liegen beide bei 3,6 bis 3,9 ms; Rauschen auf der bekannten Drift von LZ16, SD21 ändert am Server nichts auf diesem Weg. Die Basislinie erneuern wir nach deinem Test.
- **Prüfweg:** in der CI der Test „the walk-through of milestone 1“ unter den Browser-Tests und
  der Schritt „core: no type of a package named“. Von Hand auf 8092: dieselbe Folge mit Konten
  aus Keycloak oder lokalen Konten (die Rollen am Benutzer vergeben), wie im Test beschrieben.

### SD20 — Einstellungen am Organisationsbaum, 2026-11-13

- **Grundlage:** Inventar G173, G38; Hauptplan E131, E180; E17 dieses Plans; deine
  Entscheidungen vom 2026-11-13 und das Mockup „Einstellungen am Organisationsbaum“
  (Zeichenflächen 1 bis 3, dazu dein Kommentar: einmal speichern oben).
- **Gebaut:**
  - Kern (N23): `meta.Setting` mit zehn Arten und Prüfung der Werte
    (`internal/meta/setting.go`); `internal/settings` liest die Erklärungen der installierten
    Pakete, prüft, schreibt die Änderungen eines Knotens in einer Transaktion
    (`org.ApplySettings`) und gibt den Wert mit der Vorgabe; `PUT …/settings/{key}` prüft jetzt,
    neu `PATCH /api/v1/org/nodes/{path}/settings` und `GET /api/v1/settings/definitions`. Das
    Paket `core` 0.6.0 erklärt die 32 Schlüssel, die der Kern liest. `meta.Store` lehnt einen
    Schlüssel ab, den schon ein anderes Paket erklärt.
  - Formeln (N24): `setting("…")` als Makro auf eine Variable (`internal/rules/formula.go`),
    `records` gibt die Werte an der Einheit des Datensatzes (`settingsAt`).
  - Fristen (N25): Feld `target_setting` an `core.sla`, `sla.target` liest es, wenn die Uhr
    beginnt; die Prüfung der Frist verlangt eine erklärte Einstellung der Art Dauer oder Zahl.
  - Dateien: Größe und Endungen an der Einheit der Person, die hochlädt
    (`files.uploaderNode`), neu `files.allowed_extensions`; die Aufbewahrung unangehängter
    Dateien bleibt an der Wurzel.
  - Paket `incident` 0.9.0: Einstellungen `incident.priority_matrix` (Vorgabe wie §2.4) und
    `incident.close_after` (3300 Minuten), Formel der Priorität über die Matrix, die Frist
    „Störung automatisch schließen“ nimmt ihr Ziel aus der Einstellung; ein Umzugsschritt
    stellt die schon angelegte Frist beim Update um (`cmd/itsm/upgrade.go`).
  - Oberfläche: neue Seite „Globale Einstellungen“ (`web/src/app/GlobalSettings.tsx`) mit fester
    Leiste oben („n Änderungen nicht gespeichert“, „Verwerfen“, „Speichern“), Abschnitt je
    Gruppe, Feld je Art (Matrix als Tabelle, Listen als Marken, Dauer in Stunden, Größe in MB),
    Herkunft und „Eigene entfernen“ unter jeder Einstellung; Schlüssel nur für die Wurzel sind
    darunter gesperrt.
- **Belegt:**
  - Go `TestSettingsOfTheTreeOverHTTP` (Prüfweg, eigene Datenbank): die Erklärungen, der Kern
    zuerst; 13 abgelehnte Werte (unbekannter Schlüssel, Grenzen, nur Wurzel, Zeitzone, Liste,
    Matrix unvollständig oder mit falschem Wert, Verweis); ein `PATCH` mit einem falschen Wert
    schreibt nichts, einer ohne schreibt alles, `null` entfernt; hohe Auswirkung und mittlere
    Dringlichkeit sind in Nord P1, in Süd P2, mit einer Matrix an der Wurzel in Süd P3; ohne
    eigene Matrix rechnet Nord beim nächsten Speichern P2; Nord mit 0 hat keine Uhr zum
    Schließen, Süd 3300 Minuten, Nord mit 660 Minuten 660; eine Frist mit einer Einstellung,
    die keine Minuten ist oder die keiner erklärt, gibt 422; ein Paket, dessen Formel eine
    unbekannte Einstellung liest, wird abgelehnt.
  - Go `TestLimitsAtTheUploadersUnit`: in Nord nur pdf bis 1000 Bytes, in Süd wie an der
    Wurzel; die Aufbewahrung wird nur an der Wurzel gelesen.
  - Vitest `SettingsPage.test.tsx`: alles ändern und einmal speichern (Zeitzone, Zelle der
    Matrix, Stunden, Endung), Schlüssel der Wurzel gesperrt, `axe` ohne Befund; eigene
    entfernen und verwerfen, ein abgelehnter Wert steht an der Einstellung und nichts ist
    gespeichert, fehlendes Recht.
  - Alte Tests angepasst, weil der Server seit SD20 nur erklärte Schlüssel nimmt:
    `TestOrgTreeOverHTTP`, `TestAPICoverage` und `TestTenantBoundaryOverHTTP` setzen
    `probe.priority` und `probe.tenant` (erklärt im Testpaket `probe` 0.3.0) statt
    `apitest.…`; der Mustersatz von `core.sla` hat `target_setting` leer. Der Vitest der
    Seite „Globale Einstellungen“ ist neu, weil es statt eines Knopfs je Einstellung nur noch
    die Leiste oben gibt; „Knoten“ heißt dort jetzt „Einheit“.
  - Ganze CI am 2026-11-13 im zweiten Lauf bis auf LZ3 grün (12:22 min): 18 von 19 Schritten,
    127 Vitest-Fälle, 18 Browser-Tests; LZ3 lag 14 % über der alten Basislinie, im A/B gegen
    WF7 (`404146f`) gleich (7,88 gegen 7,91 ms), LZ13 dort −3 %. Der erste Lauf war rot: der
    Lint fand zwei verschluckte Fehler (`nilerr`), `TestOrgRightsOverHTTP` setzte
    `calendar.default` auf „x“ (jetzt eine Kennung, weil der Server seit SD20 den Wert prüft),
    und das Lasttor war unter fremder Last rot.
  - Auf 8092 eingespielt (`core` 0.6.0, `incident` 0.9.0, `probe` 0.3.0): der Umzugsschritt hat
    die Frist „Störung automatisch schließen“ auf `incident.close_after` gestellt.
- **Nicht geprüft:** Der Umzugsschritt für die schon angelegte Frist hat keinen automatischen
  Test (keine Datenbanktests für die Paketinstallation); geprüft auf 8092 beim Einspielen. Kein
  Browser-Test der Seite. Ein Wert, der vor SD20 gesetzt wurde und nicht zur Erklärung passt,
  bleibt stehen; die Leser des Kerns verwerfen ihn wie bisher. Andere Instanzen sehen eine
  Änderung erst nach höchstens 10 Sekunden.

### SD17 — Freigabe von Wissen, 2026-11-11

- **Grundlage:** IP4; KF4; Hauptplan E178; E16 dieses Plans; deine Entscheidungen vom
  2026-11-11 und das Mockup „Freigabe von Wissen“ (Zeichenflächen 1, 2A, 3).
- **Gebaut:**
  - Kern (N22): `approval.hold` am Typ (`internal/meta/approval.go`), `internal/records/hold.go`
    (zurückhalten beim Ändern, schreiben nach der Zustimmung), die Genehmigung trägt die
    Änderung (`changes`, Migration 00045, OpenAPI `Approval.changes`); Regelaktion `approval`
    mit `role` (`internal/rules/engine/each.go`).
  - Paket `knowledge` 0.6.0: Merkmal `approvals`, Rolle „Redaktion“, Status „In Prüfung“,
    Übergänge „Zur Freigabe vorlegen“, „Freigegeben“ und „Nicht freigegeben“ (folgen der
    Genehmigung), „Zurück in den Entwurf“ auch aus der Prüfung, wenn die Genehmigung
    zurückgezogen ist; `hold` für Titel, Text und „Sichtbar für“, solange veröffentlicht.
    Startdaten: Gruppe „Wissensredaktion“, Regel „Redaktion ist die Wissensredaktion“ beim
    Anlegen, Regel „Freigabe bei Vorlage“, Mail „Freigabe erbeten“; die Mail beim Ablauf sagt
    jetzt „wieder zur Freigabe vorlegen“.
  - Oberfläche: Band über dem Formular mit „Ansehen“ und „Änderung zurückziehen“, die Felder
    der wartenden Änderung schreibgeschützt; im Reiter „Genehmigung“ „Beantragte Änderung“
    Wort für Wort (der Vergleich der Fassungen, `Versions.tsx`, gibt `Words` und `plain` her).
- **Belegt:**
  - Go `TestKnowledgeApprovalOverHTTP` (Prüfweg): ein neuer Artikel hat die Wissensredaktion als
    Redaktion; „Veröffentlichen“ von Hand 409; vorgelegt fragt die Regel die Redaktion, ein
    Leser sieht ihn nicht; die Zustimmung veröffentlicht. Fassung 2 (Titel und Text) wartet,
    die Kategorie ist sofort geändert, der Leser sieht den alten Text; Titel noch einmal 409,
    „Gültig bis“ geht; nach der Zustimmung sieht der Leser Fassung 2 als neue Version, der
    Status bleibt; eine abgelehnte Änderung fällt weg, eine zurückgezogene auch, danach wartet
    die nächste. `TestApprovalSpec` prüft `hold`.
  - Vitest `Approvals.test.tsx`: Band, Titel gesperrt, Begründung nicht, „Ansehen“ öffnet den
    Reiter mit der beantragten Änderung, „Änderung zurückziehen“ zieht zurück; `axe` ohne
    Befund.
  - Alte Tests angepasst, weil ein Artikel seit SD17 nur über die Freigabe veröffentlicht wird
    und Titel und Text eines veröffentlichten Artikels erst nach der Zustimmung gelten:
    `TestKnowledgeOverHTTP`, `TestKnowledgeLifecycleOverHTTP` und `TestKnowledgeTicketOverHTTP`
    veröffentlichen über `publishArticle` (vorlegen, Redaktion stimmt zu), der englische Titel
    gilt nach `approveOpen`; die Zahl der Startdaten von `knowledge` ist 9 statt 5 (auch in
    `TestPortalStartOverHTTP`).
  - Ganze CI am 2026-11-11 im zweiten Lauf grün (12:33 min): alle 19 Schritte, 124 Vitest-Fälle,
    18 Browser-Tests, Lasttor im A/B nicht schlechter (LZ1 −19 %, LZ2 −2 %, LZ3 +1 %, LZ4 +8 %).
    Der erste Lauf war rot: der Lint fand, dass ein Fehler beim Auswerten der Bedingung von
    `hold` verschluckt wurde (jetzt gemeldet wie bei den Übergängen), und das Lasttor war unter
    fremder Last rot, auch bei LP2 (Bereitschaft), das SD17 nicht berührt.
- **Nicht geprüft:** Kein Browser-Test (die Browser-Tests der CI installieren `knowledge` nicht).
  Wird eine Zustimmung erteilt, nachdem sich die Regeln eines Felds geändert haben, schreibt der
  Kern die Änderung nicht und vermerkt das im Verlauf; die Genehmigung bleibt erteilt. Ist die
  Rolle „Redaktion“ leer, lehnt der Kern die Änderung eines veröffentlichten Artikels mit 422
  ab. Auf bestehenden Mandanten fehlen die neuen Startdaten nicht (sie kommen mit dem Update
  dazu), aber schon angelegte Artikel haben keine Redaktion: sie braucht man von Hand.

### SD13 — Servicekatalog, 2026-11-10

- **Grundlage:** IP24; Inventar G68, G133; Hauptplan E177; E15 dieses Plans; deine Entscheidungen
  vom 2026-11-10 und das Mockup „Servicekatalog“ (Variante 1A).
- **Gebaut:**
  - Kern (N19 bis N21): Block `order` am Typ (`internal/meta/order.go`) und
    `forms.Service.Order` mit `POST /api/v1/records/{typeName}/{id}:order` (`internal/forms/order.go`);
    „Angeboten im“ kennt „Nur über ein Angebot“; `core.sla` hat „Ziel mal Feld“
    (`target_field`, `internal/sla`); der Lesebereich kennt `facts` und `sort`; Startdaten
    dürfen einen Eintrag einer Auswahlliste mit seinem Schlüssel nennen (`internal/seed`).
    Paket `core` 0.4.0.
  - Paket `request` 0.2.0: Felder Lieferzeit in Arbeitstagen (Vorgabe 5), Kosten in Euro,
    Kostenart; die Erfüllungsfrist rechnet 660 Minuten mal Lieferzeit.
  - Paket `catalog` 0.1.0: Typen `area` (Katalogbereich, Baum mit Zugänge › Laufwerke,
    Anwendungen; Arbeitsplatz › Geräte, Software; Telefonie), `service`, `offer`; Beziehung
    `catalog.ordered`; Startdaten: Kachel „Katalog“, Service „Arbeitsplatz-Ausstattung“, Formular
    „Neuer Arbeitsplatz bestellen“ und Angebot „Neuer Arbeitsplatz“ mit der Vorlage aus `request`.
  - Oberfläche: Karten mit Angaben im Lesebereich, Formular „Bestellen“ auf der Seite des
    Angebots, danach die Seite der Anfrage; das Ausfüllen eines Formulars steht jetzt in
    `web/src/portal/FormFill.tsx`.
- **Belegt:**
  - Go `TestCatalogOverHTTP` (Prüfweg): ein Angebot an der Dienststelle Nord sieht nur Nord, Süd
    bekommt beim Bestellen 404; englisch heißt es englisch; ohne Pflichtantwort 422 an der
    Frage; die Bestellung ist eine Anfrage mit Titel, Vorlage, Lieferzeit 3, Kosten und
    Kostenart vom Angebot, der Person als Anforderer, den Antworten als erstem Eintrag und der
    Beziehung „bestellt aus“; ihre Erfüllungsuhr hat 3 × 660 Minuten, eine Anfrage ohne Angebot
    3300; die Vorlage fordert die Genehmigung an; das Formular steht nicht in der Liste des
    Portals; eine Vorlage ist nicht bestellbar, ein Angebot ohne Formular 422; „Ziel mal Feld“
    auf ein Textfeld 422. `TestOrderSpec` prüft den Block `order`, `facts` und `sort`.
  - Vitest `Catalog.test.tsx`: Karten mit Angaben, nach Name sortiert; Bestellen auf der Seite
    des Angebots, ohne Laufwerk nichts, danach die Seite der Anfrage; `axe` ohne Befund auf
    Liste und Angebot.
  - Ganze CI am 2026-11-11 im zweiten Lauf (13:08 min, Last beim Start 1,2): 18 von 19
    Schritten grün, darunter alle Go-Tests, 123 Vitest-Fälle und 18 Browser-Tests. Rot war nur
    das Lasttor bei LZ3 (Volltextsuche, p95 10,02 ms gegen 8,55 ms der festen Basislinie,
    +17 %). Ein A/B gegen den Stand von SD6 (Commit 6cb2de3) auf derselben Datenbank zeigt keinen
    Unterschied: LZ3 10,24 gegen 10,13 ms (+1 %), LZ1, LZ2 und LZ4 gleich oder schneller; die
    Lastmessung installiert nur `core` und `probe`, die neuen durchsuchbaren Typen sind dort
    nicht. Wie bei SD14 liegt es an der alten Basislinie, die du erneuerst.
  - Im ersten Lauf waren zwei allgemeine Tests rot (`TestAPICoverage`,
    `TestTenantBoundaryOverHTTP`): ihr Beispiel einer SLA schrieb Zufallstext in „Ziel mal
    Feld“, den die neue Prüfung zu Recht ablehnt. Angepasst: das Beispiel lässt das Feld leer,
    damit prüfen die Tests dasselbe wie vorher.
- **Nicht geprüft:** Kein Browser-Test, die Browser-Tests der CI installieren nur `core` und
  `probe` (wie bei „Wissen“). Ändert man die Lieferzeit einer Anfrage später, bleibt die laufende
  Uhr bei ihrem Ziel. Die Anfrage entsteht am Ort des Formulars (wie jedes Formular), nicht am
  Ort der Person. Die Suche im Katalog zeigt Treffer ohne Angaben. Startdaten ändern sich bei
  einem Update nicht: auf bestehenden Mandanten bleibt die Erfüllungsfrist fünf Tage fest, bis
  man an „Anfrage Erfüllung“ „Ziel mal Feld“ = `delivery_days` und 660 Minuten einträgt.

### SD6 — Anfrage, 2026-11-10

- **Grundlage:** IP30; Inventar G36; Hauptplan E176; E14 dieses Plans; deine Entscheidungen vom
  2026-11-10: Vorlage als Datensatz mit Aufgabenzeilen, die Vorlage nennt auch die Stufen,
  eine Frist für alle, Kachel im Portal.
- **Gebaut:**
  - Kern (N17, N18): `internal/rules/engine/each.go` mit `create` für jede Zeile und der
    Regelaktion `approval`; beide tun nichts doppelt, wenn zwei Jobs gleichzeitig laufen;
    `@item…` gelten in Startdaten nicht als Verweis (`internal/meta/seed.go`); OpenAPI der
    Regelaktionen. In WF6 nachgezogen: die Vorprüfung beim Anfordern sieht den Datensatz mit
    der offenen Genehmigung, damit ein Übergang mit der Bedingung `record.approval ==
    "pending"` geht.
  - Paket `request` 0.1.0: Typen `request` (Anfrage), `task` (Erfüllungsaufgabe), `template`
    (Erfüllungsvorlage), `template_task` (Aufgabe der Vorlage), `template_stage`
    (Genehmigungsstufe der Vorlage). Startdaten: zwei Regeln für die Genehmigung aus der
    Vorlage (beim Anlegen, beim Setzen der Vorlage), eine Regel für die Aufgaben, die Vorlage
    „Neuer Arbeitsplatz“ mit zwei Aufgaben für den Service Desk (ohne Stufen, die trägt der
    Mandant ein), zwei Fristen, die Kachel „Etwas anfragen“, die Mails „Anfrage eingegangen“
    und „Genehmigung erbeten“ (an die Gefragten).
- **Belegt:**
  - Go `TestRequestOverHTTP` (Prüfweg): Vorlage mit zwei Stufen (die zweite „mindestens 1 von
    2“) und zwei Aufgabenzeilen; eine Anfrage mit der Vorlage fordert beide Stufen in ihrer
    Reihenfolge an und wartet; nach beiden Zustimmungen ist sie in Erfüllung und hat genau zwei
    Aufgaben für die Gruppe in ihrer Reihenfolge; eine Ablehnung schließt die zweite Anfrage mit
    dem Grund in der Zeitleiste; ohne Vorlage wird nichts angefordert, eine später gesetzte
    Vorlage fordert an; „Erfüllung beginnen“ ohne Vorlage; der Übergang „angefordert“ geht von
    Hand nicht; die Kachel; falsche Regeln 422.
  - Gefunden beim Bau: die Regel für die Aufgaben lief zweimal gleichzeitig (ein Job zu einem
    früheren Ereignis lief erst, als die Anfrage schon in Erfüllung war) und machte vier statt
    zwei Aufgaben; jetzt hält ein Schlüssel je Regel, Anfrage und Zeile das auf einmal.
  - Ganze CI am 2026-11-10 im zweiten Lauf (14:48 min): 18 von 19 Schritten grün, darunter alle
    Go-Tests, das Lasttor (LZ1 bis LZ5, LZ13, LP1, LP2 im A/B-Vergleich nicht schlechter) und 18
    Browser-Tests. Rot waren sechs Vitest-Fälle, die SD6 nicht berührt, alle mit Zeitüberschreitung
    nach 5 s unter fremder Last (Last beim Start 15); allein nachgeholt grün, 121 von 121. Der erste
    Lauf war am Lasttor rot (LZ3 +14 %, LZ13 −11 %), ebenfalls unter fremder Last aus anderen
    Containern; im zweiten Lauf lagen dieselben Werte im Rahmen.
- **Nicht geprüft:** Kein eigener Browser-Test; Vorlagen, Stufen und Zeilen legt man im
  Arbeitsplatz mit dem allgemeinen Formular an (eine Seite, die alles einer Vorlage zusammen
  zeigt, fehlt). Ändert man die Vorlage, behalten laufende Anfragen ihre Stufen und Aufgaben.
  Die Mail an die Gefragten verweist auf den Arbeitsplatz, nicht auf den Bereich im Portal.
  Wiederkehrende Anfragen sind nicht eigens getestet (WF14 gilt für jeden Typ).

### SD14 — Startseite des Portals, 2026-11-10

- **Grundlage:** IP28; Inventar G78; Hauptplan E174; SD-Plan E13; deine Entscheidungen vom
  2026-11-10 nach dem Mockup: „Aktuell“ als Band oben, Kacheln ersetzen die Karte „Formulare“,
  Kachel und Hinweis von Hand als Kerntypen, Gültigkeit mit Uhrzeit.
- **Gebaut:**
  - Kern, Paket `core` 0.2.0: Typ `core.portal_tile` (Titel, Text, „Führt zu“: neuer Datensatz
    eines Typs, Formular, Bereich, Datensatz oder Link; Reihenfolge; „Sichtbar für“ mit der Rolle
    „Gruppen“ und Leser-Regeln wie beim Artikel; „Im Portal gezeigt“); Feldzustände blenden je
    Art nur das nötige Zielfeld ein und machen es zur Pflicht. Typ `core.announcement` (Titel,
    formatierter Text, Gültig ab und bis als Zeitpunkt, „Sichtbar für“, Entwurf, Veröffentlicht,
    Abgelaufen); die Leser-Regeln prüfen die Zeit mit `now()`. Der Hinweis ist durchsuchbar,
    die Kachel nicht (deine Entscheidung nach der CI, siehe unten); Kacheln findet man über
    Liste und Filter. Startdaten `data/hinweise.yaml`:
    alle 15 Minuten setzt eine Regel vorbei gegangene Hinweise auf „Abgelaufen“.
  - `GET /api/v1/portal/notices` liefert nach den Hinweisen aus Abfragen (SD5) die Hinweise von
    Hand, mit den Rechten der Person gelesen, mit Titel, Text als formatiertem Text und „bis“;
    wer mehr lesen darf (Verwaltung), sieht dort trotzdem nur, was gerade gilt.
  - Startdaten: `incident` 0.8.0 bringt die Kachel „Störung melden“, `knowledge` 0.5.0 die
    Kachel „Wissen“.
  - Oberfläche (`web/src/portal/Tiles.tsx`): „Was möchten Sie tun?“ mit den Kacheln in ihrer
    Reihenfolge; eine Kachel, deren Ziel die Person nicht erreicht (Typ nicht lesbar, Formular
    nicht angeboten), fehlt; ein Link nur mit `http` oder `https`, in neuem Fenster. Ohne
    Kachel zeigt die Startseite die Formulare wie bisher. Im Band „Aktuell“ stehen Hinweise von
    Hand mit Titel und „bis“, „Mehr“ zeigt den Text.
- **Alte Tests angepasst:** Die Pakete bringen je eine Kachel mehr mit: die Zahl der Startdaten
  von `incident` von 39 auf 40, von `knowledge` von 4 auf 5 in den Tests, die sie zählen; ein
  neuer Mandant bekommt in `TestIncidentDeadlinesOverHTTP` 69 statt 67 Startdaten (dazu die
  Regel der Hinweise aus `core`).
- **Belegt:**
  - Go `TestPortalStartOverHTTP`: die Kacheln der Pakete sehen alle, die Kachel einer Gruppe nur
    ihr Mitglied, eine abgeschaltete niemand; eine Formular-Kachel ohne Formular ist 422; unter
    „Aktuell“ steht ein Hinweis nur in seiner Zeit, für seine Zielgruppe, ein Entwurf nie, für
    die Verwaltung nur, was gilt; die Regel setzt den vorbei gegangenen auf „Abgelaufen“.
  - Vitest `portal/Tiles.test.tsx`: Kacheln in ihrer Reihenfolge mit ihren Zielen, nicht
    erreichbare und ein Link mit `javascript:` fehlen, keine Karte „Formulare“; ohne Kacheln die
    Formulare; der Hinweis von Hand mit Titel, „bis“ und Text nach „Mehr“; mit axe.
  - Ganze CI am 2026-11-10, zwei Läufe: alle Go-Pakete, Vitest (117 Fälle) und die
    Browser-Tests (18) grün; rot nur das Lasttor, im ersten Lauf LZ3 (Volltextsuche) +22 %,
    im zweiten, mit Kachel und Hinweis nicht durchsuchbar, LZ3 +19 % und LZ13 −20 %. Meine
    erste Erklärung (die zwei neuen durchsuchbaren Typen) war damit widerlegt. Ein Vergleich
    des Programms von SD14 gegen das von SD18 auf derselben Datenbank, deren Server-Code sich
    nur in `internal/portal/portal.go` unterscheidet, zeigte dieselbe Lücke in fast allen
    Szenarien (p50 LZ1 2,38 gegen 2,07 ms, LZ2 4,64 gegen 4,28 ms, LZ3 6,96 gegen 5,98 ms):
    die Abweichung kommt aus der Messung, nicht aus SD14. Deine Entscheidung: committen, die
    Basislinie erneuerst du; der Hinweis wird wieder durchsuchbar (so lief der erste Lauf), die
    Kachel nicht. Die Warnungen „now nicht bekannt“ kamen vom Programm der Basislinie, das älter
    als SD16d ist. Befund für später: Warum lag die neue Seite heute in allen drei Messungen
    hinten?
- **Nicht geprüft:** Kein eigener Browser-Test. Kachel und Hinweis legt man im Arbeitsplatz mit
  dem allgemeinen Formular an; eine Kachel „Datensatz“ nimmt Typ und Kennung als Text, eine
  Suche über alle Typen fehlt dort noch. Die Katalogseite kommt mit SD13.

### SD18 — Wissen am Vorgang und im Portal, 2026-11-10

- **Grundlage:** IP4; Inventar G52, G108; Hauptplan E173; SD-Plan E12; deine Entscheidungen
  vom 2026-11-10 nach dem Mockup: Vorschläge sagt der Artikel, im Portal neben dem Formular,
  an der Störung ein eigener Reiter „Wissen“, „Als Lösung übernehmen“ setzt Text und
  Lösungsart und verknüpft, „Das hat geholfen“ legt nichts an, Ja oder Nein je Person,
  Kommentare über die Zeitleiste.
- **Gebaut:**
  - Kern, Vorschläge (N13): Typ-Option `suggest` mit `label`, `for` (Typen oder `trait:`),
    `from`, `relation` und `apply` (`internal/meta/suggest.go`, geprüft beim Installieren);
    Endpunkt `POST /api/v1/records/{typeName}:suggest` (`internal/suggest`) mit den Werten des
    Formulars oder einem eigenen Text, je vorschlagendem Typ eine Gruppe mit Treffern, ihrer
    Bewertung, wie oft sie verknüpft sind und ob der Datensatz schon mit ihnen verknüpft ist.
    Die Volltextsuche kennt dafür „jedes Wort zählt“ (`SearchQuery.Any`, bis 2.000 Zeichen,
    das letzte Wort auch als Wortanfang); Rechte und Leser-Regeln gelten wie bei jeder Suche.
    Listen-Schlüssel in `apply` (`instructed`) löst der Kern in die Kennung des Eintrags auf,
    wie bei festen Werten der Formulare (`records.Service.ListKeys`).
  - Kern, Bewertung (N14): Merkmal `rating`, Tabelle `rating` (Migration 43), `GET` und `PUT
    /api/v1/records/{typeName}/{id}/rating`; Ja oder Nein je Person, änderbar, `null` nimmt
    zurück; braucht Lesen und eine Person; geht beim endgültigen Löschen mit.
  - Kern, Kommentare (N15): Leser-Regeln nennen auf Wunsch `actions: [read, comment]`; private
    Einträge bleiben bei `read_private`.
  - Kern, Verknüpfen (N16): Beziehungsart mit `update: from`; Anlegen und Lösen brauchen dann
    Ändern nur am Ausgang, am Ziel genügt Lesen.
  - Paket `knowledge` 0.4.0: Artikel mit `timeline` und `rating`, Leser dürfen kommentieren,
    `suggest` für `trait:sd.ticket` aus Titel und Beschreibung, Beziehungsart „nutzt Wissen“
    (invers „genutzt bei“) mit `update: from`, „Als Lösung übernehmen“ setzt `solution` aus
    dem Text und `solution_kind` „Anleitung gegeben“.
  - Oberfläche: Reiter je vorschlagendem Typ auf der Detailseite nach „Bezüge“
    (`web/src/render/Suggestions.tsx`) mit Vorschlägen aus den Feldern, eigener Suche,
    „Verknüpfen“, „Lösen“ und „Als Lösung übernehmen“, das die Werte ungespeichert ins Formular
    setzt (`Form` nimmt `apply`); im Portal beim Anlegen bis zu drei Vorschläge neben dem
    Formular, auf dem Telefon darunter (`web/src/portal/Suggest.tsx`), aufklappbar, „Das hat
    geholfen“ zählt Ja und beendet ohne Meldung, „Doch melden“ holt das Formular mit dem
    Geschriebenen zurück; unter dem Artikel im Portal „Hat Ihnen das geholfen?“ und die
    öffentlichen Kommentare mit Eingabe (`web/src/portal/Feedback.tsx`).
- **Abweichung vom Mockup:** Auf dem Telefon stehen die Vorschläge unter dem Formular, nicht
  unter dem Titel; sonst würde das Formular beim Tippen nach unten springen.
- **Belegt:**
  - Go `TestKnowledgeAtTicketOverHTTP`: die Nutzerin bekommt aus Titel und Beschreibung nur
    den veröffentlichten VPN-Artikel, nicht den Entwurf; eigene Suche; Bewertung Ja, Nein,
    zurücknehmen, je Person; Kommentar öffentlich ja, privat 403, am Entwurf 404; der
    Bearbeiter verknüpft mit nur Lesen am Artikel, die Nutzerin nicht; der Vorschlag zeigt
    die Verknüpfung und „1-mal genutzt“, nach dem Lösen nicht mehr.
  - Go `TestSuggestReadersAndRelationUpdate`: die Prüfungen von `suggest`, `actions` und
    `update` beim Installieren.
  - Vitest `Suggestions.test.tsx` (Reiter, Verknüpfen, Übernehmen ins Formular, eigene Suche,
    Markierungen ohne Markup) und `portal/Suggest.test.tsx` (Vorschläge beim Tippen, „Das hat
    geholfen“ ohne Anlegen, Bewerten und Kommentieren unter dem Artikel), je mit axe.
  - Ganze CI am 2026-11-10 im ersten Lauf grün: alle Go-Pakete, Vitest (114 Fälle), die
    Browser-Tests (18) und das Lasttor (LZ1 +7 %, LZ2 +7 %, LZ3 +1 %, LZ4 −2 % gegen die
    Basislinie im selben Lauf).
- **Nicht geprüft:** Kein Browser-Test eigens für SD18; der Durchgang von Meilenstein 1 (SD21)
  nimmt den Wissensartikel als Lösung mit. Die Güte der Treffer bei vielen Artikeln nicht
  gemessen. Eine Auswertung „durch Wissen vermieden“ gibt es noch nicht, die Zahlen liegen in
  der Tabelle `rating`.

### SD16d — Wissen, Ablauf, 2026-11-04

- **Grundlage:** IP4; Hauptplan E172; SD-Plan E8, E11; deine Entscheidungen vom 2026-11-04:
  `today()` und `now()` mit Abstand, Autor ist wer anlegt und änderbar, Mails 14 Tage vorher
  und beim Ablauf, im Portal sofort am Tag danach weg.
- **Gebaut:**
  - Kern: `internal/rules/now.go` mit `today()`, `today(n)`, `now()`, `now("-24h")` in den
    Bedingungen über einen Datensatz (Übergänge, Feldzustände, Leser-Regeln, Rechte, Regeln,
    Benachrichtigungen; nicht in den Anmelderegeln). Sie geben Text in der Form, die der
    Datensatz für Datum und Zeitpunkt hat; der Übersetzer nach SQL (`authz.Translate`) setzt
    den Wert beim Übersetzen als Parameter ein. Das Datum gilt in Europe/Berlin.
  - Regel-Engine: „Rolle zuweisen“ nimmt mit `actor: true` die Person, die schreibt, dazu;
    vor dem Schreiben aus der Anfrage, danach aus dem Ereignis; ohne Person niemanden. Die
    Rolle muss eine Person halten dürfen, sonst 422.
  - Paket `knowledge` 0.3.0: Rolle „Autor“ (eine Person); die Leser-Regeln prüfen „Gültig bis“
    gegen `today()`; Startdaten `data/ablauf.yaml`: Regel „Autor ist, wer anlegt“ (vor dem
    Anlegen, nur ohne Autor), „Erinnerung 14 Tage vor Ablauf“ (täglich 7:00), „Abgelaufene
    Artikel“ (täglich 0:15) und die Mail „Artikel abgelaufen“ an den Autor.
- **Belegt:**
  - Go `TestKnowledgeLifecycleOverHTTP`: die Autorin legt an und ist Autorin; ein Autor in der
    Anfrage bleibt; „Gestern“ ist für den Leser sofort weg, in der Liste und einzeln; nach den
    Regeln ist er abgelaufen, „In 14 Tagen“ noch veröffentlicht; die Autorin bekommt beide
    Mails.
  - Go `TestTodayAndNow` (Datum in Berlin kurz vor Mitternacht UTC, Abstände, falsche Dauer
    als Fehler) und `TestTranslateTodayAndNow` (Parameter in SQL, Abstand nur als feste Zahl).
  - Ganze CI am 2026-11-04, drei Läufe. Im ersten war ein alter Test rot:
    `TestFieldRightsConditionsAndInvalidation` erwartete außer Auswahllisten nur „probe“ als
    lesbaren Typ; der Abdeckungstest installiert aber alle Pakete in dieselbe Datenbank, und
    seit SD16a ist der Artikel über seine Leser-Regeln für jeden lesbar. Bisher liefen beide
    Tests zufällig in verschiedenen Teilen; der neue Test hat die Aufteilung verschoben. Der
    alte Test zählt Typen mit Leser-Regeln jetzt wie Auswahllisten nicht mit. Im zweiten und
    dritten Lauf waren alle Go-Pakete, Vitest (109 Fälle) und die Browser-Tests (18) grün; rot
    war nur das Lasttor, im zweiten LZ1, LZ3 und LZ4, im dritten nur noch LZ1 mit +11 % gegen
    die Basislinie (3,64 ms statt 3,29 ms; im ersten Lauf +7 %). Offen, siehe Hauptplan „Jetzt“.
- **Nicht geprüft:** Die Uhrzeit der Regeln nicht im echten Takt (der Test ruft die Regeln
  nach Zeitplan direkt auf). Fällt der Server an einem Morgen ganz aus, holt er die Erinnerung
  am selben Tag nach, an einem späteren nicht. Eine Regel-Oberfläche gibt es weiter nicht; die
  Regeln ändert ein Mandant über die Schnittstelle.

### SD16c — Wissen, Sprache der Inhalte, 2026-11-04

- **Grundlage:** IP4; Hauptplan E171, V40; SD-Plan E10; deine Entscheidungen vom 2026-11-04
  nach dem Mockup „Übersetzungen im Formular“
  ([Artifact](https://claude.ai/artifact/C8r9bReahYm2tT2DgmNGik)): eine Version, Automatik
  später mit einem Übersetzer im eigenen Rechenzentrum; Anzeige in der Sprache der Person;
  Titel und Text übersetzbar.
- **Gebaut:**
  - Oberfläche: Lesen folgt der Sprache der Person (`Accept-Language` aus der eingestellten
    Sprache): Listen, Suche, Namen von Verweisen, Vorschau und das Portal. Das Formular liest
    über `api.get` weiter Deutsch, und geschrieben wird immer Deutsch; Anzeigen nutzen den
    neuen `readQuery`. Wechselt die Sprache, liest die Oberfläche Datensätze und Treffer neu
    (`useContentLanguage` in Arbeitsplatz und Portal).
  - Paket `knowledge` 0.2.0: Titel und Text sind übersetzbar. Eine englische Fassung lässt sich
    heute nur über die Schnittstelle setzen (`{"title": {"en": …}}`), wie seit PL21.
- **Belegt:**
  - Go `TestKnowledgeOverHTTP`: ein englischer Titel kommt mit `Accept-Language: en`, Deutsch
    ohne; der Text ohne Übersetzung kommt deutsch.
  - Vitest `Read.test.tsx`: auf Englisch liest das Portal den Artikel mit `en`, das Formular
    im Arbeitsplatz den Datensatz mit `de`.
  - Ganze CI am 2026-11-04 im ersten Lauf grün (19 Schritte, kein rotes Paket im Protokoll,
    109 Vitest-Fälle, 18 Tests im Browser, Lastmessung im Rahmen).
- **Nicht geprüft:** Kein Selbsttest auf Englisch im Browser, weil das deine Einstellung auf
  8092 geändert hätte. Die Detailseite im Arbeitsplatz zeigt den Titel aus dem Formular, also
  Deutsch, auch für eine englische Sitzung.

### SD16b — Wissen, Fassungen, 2026-11-04

- **Grundlage:** IP4; Hauptplan E169, SD-Plan E9; deine Entscheidungen vom 2026-11-04 nach dem
  Mockup „Fassungen vergleichen“ ([Artifact](https://claude.ai/artifact/Nqgtdo42n6L56GMeQNxc1G)):
  Reiter auf der Detailseite (Variante A), Wörter markieren ohne Bibliothek, jeder Typ mit
  Verlauf.
- **Gebaut:**
  - Kern: die Liste der Fassungen nennt, wer sie geschrieben hat (`actor_name`, wie die
    Zeitleiste). Liste, Vergleich und Wiederherstellen gab es seit PL19.
  - Oberfläche: Reiter „Fassungen“ zwischen „Bezüge“ und „Zeitleiste“ an jedem Typ mit dem
    Merkmal Verlauf. Links die Fassungen, neueste oben, mit Zeit, Person und den geänderten
    Feldern („Angelegt“ bei der ersten); voreingestellt sind die zwei neuesten, ein Klick
    tauscht die ältere der beiden. Rechts eine Tabelle nur der Felder, die sich
    unterscheiden: Text und formatierter Text Wort für Wort (gestrichen und unterstrichen),
    Auswahl, Status, Verweise und Rollen mit Namen. „Fassung n wiederherstellen“ fragt auf
    der Seite nach und schickt die gesehene Version mit; hat jemand inzwischen gespeichert,
    sagt die Seite das.
  - Wortvergleich `web/src/render/wordDiff.ts`: längste gemeinsame Folge von Wörtern und
    Leerraum; über 2000 Wörter je Seite wird der Text als Ganzes verglichen.
- **Belegt:**
  - Go `TestVersionsOverHTTP` prüft jetzt auch den Namen in der Liste.
  - Vitest `wordDiff.test.ts` (Wörter, leer, gleich, sehr lang) und `Versions.test.tsx`:
    Reihenfolge und Personen, die zwei neuesten voreingestellt, Wörter gestrichen und
    unterstrichen, eine Auswahl mit ihrer Beschriftung, Rückfrage, Wiederherstellen mit
    `If-Match`, Konflikt mit Hinweis; `axe` ohne Befund.
  - Nachgezogen aus SD9: `TestProblems` in `internal/forms` erwartet die Meldung „only one of
    them“ statt „not both“, weil eine Frage seit SD9 auch eine Rolle erfragen kann.
  - Angepasst: `Relations.test.tsx` erwartete zwei Reiter; `probe` hat Verlauf, darum steht
    jetzt „Fassungen“ dazwischen, und die Pfeiltaste führt dorthin (Ende zur Zeitleiste).
  - Selbsttest auf 8092 in Chrome an einem Datensatz von `probe` mit acht Fassungen: Liste mit
    Namen, Vergleich 7 und 8 mit markiertem Zeitstempel im Titel. Wiederherstellen dort nicht
    ausprobiert, um keine Daten zu ändern.
  - Ganze CI am 2026-11-04 grün mit der korrigierten CI (19 Schritte, kein rotes Paket im
    Protokoll, 108 Vitest-Fälle, 18 Tests im Browser, Lastmessung im Rahmen). Der erste Lauf
    meldete grün, verbarg aber das rote Paket `internal/forms` (siehe Nachtrag bei SD9).
- **Nicht geprüft:** Kein Browser-Test in der CI für den Reiter. Felder, die die Person nicht
  lesen darf, lässt der Server aus; ob das Wiederherstellen solche Felder unverändert lässt,
  prüft der Test von PL19, nicht dieser.

### SD16a — Wissen, Artikel im Portal, 2026-11-04

- **Grundlage:** IP4, Inventar G106; Hauptplan E168, SD-Plan E8; deine Entscheidungen vom
  2026-11-04 nach dem Mockup „Wissen im Portal“
  ([Artifact](https://claude.ai/artifact/At2t1hm51eJPvdGF1m9jGz)) und dem Klicktest im Browser:
  SD16 in vier Teile, Sichtbarkeit wie bei Auswahllisten, Status mit täglicher Regel (SD16d),
  Kategorien der Störung, Variante B, „Sichtbar für“ in drei Stufen mit der Rolle „Gruppen“;
  „Formulare“ bleibt hinter den Vorgangsarten; der Befund mit der Kennung statt des Namens
  kommt in diese Aufgabe; das Paket `probe` bleibt auf 8092 für Tests.
- **Gebaut:**
  - Kern, Rechte (N12): Typ-Option `readers` mit Bedingung (`when`) und Rollen (`roles`). Die
    Rechte laden je Typ die Regeln (`Access.Readers` statt der Liste `ReadAll`); eine Auswahlliste
    ist eine Regel ohne Bedingung. Ohne Rollen gilt der Knoten der Person im Organisationsbaum
    wie bei den Auswahllisten, mit Rollen eine Zeile in `actor` für die Person oder ihre Gruppen.
    Die Suche und die Einzelabfrage halten sich an dieselben Regeln. Eine Bedingung, die sich
    nicht nach SQL übersetzen lässt, lässt niemanden lesen.
  - Kern, Typen: die Portal-Option kennt `read` (Name des Bereichs, Felder der Seite, Feld zum
    Gliedern); `fields` zum Anlegen ist dann nicht mehr Pflicht. Beides wird beim Laden geprüft.
  - Paket `knowledge` 0.1.0 (neu, hängt von `core` und `sd` ab): Typ `knowledge.article` mit
    Titel, Text (formatiert, durchsuchbar), Kategorie, „Sichtbar für“ (Alle, auch im Portal; Nur
    Bearbeiter, voreingestellt; Nur bestimmte Gruppen), „Gültig bis“, Rolle „Gruppen“; Status
    Entwurf, Veröffentlicht, Abgelaufen mit den Übergängen Veröffentlichen, Ablaufen lassen,
    Zurück in den Entwurf. Rechte bringt das Paket keine mit: Bearbeiter brauchen eine Rolle
    mit Lesen, Anlegen und Ändern für `knowledge.article`.
  - Portal: im Kopf „Meine Vorgänge | Wissen“, sobald ein Typ einen Lesebereich hat; die Seite
    `/portal/read/<typ>` mit Kategorienbaum (Eintrag und alles darunter), Suche und Liste mit
    Anriss; die Artikelseite `/portal/read/<typ>/<id>` mit Pfad, Kategorie, Stand und Text und
    darunter „Hilft das nicht weiter?“ mit „Störung anlegen“. Die Startseite zeigt nur Arten
    zum Anlegen als Karte.
  - Befund des Klicktests: ein Verweis (etwa die Kategorie) zeigt im Portal und in der
    Zeitleiste den Namen des Ziels statt der Kennung, auch im Arbeitsplatz.
- **Belegt:**
  - Go `TestReaders`: Regeln mit und ohne Rollen, Knoten darüber und daneben, anderer
    Mandant, Entwurf, nur für Bearbeiter, nicht übersetzbare Bedingung; Lesen gibt kein Ändern.
  - Go `TestKnowledgeOverHTTP` (eigene Datenbank): Personen ohne Recht auf Artikel; Nord liest
    „Für alle“ und „Nur Nord“, Süd als Mitglied der Gruppe „Für alle“ und „Gruppe Netz“; keiner
    liest den Entwurf und „Nur Bearbeiter“ (einzeln 404), keiner ändert; die Suche nach „VPN“
    findet für Süd genau seine zwei.
  - Vitest `Read.test.tsx`: Bereich im Kopf, Kategorienbaum mit Unterkategorie, Suche,
    Artikelseite mit Kategorie, Pfad und „Meldung anlegen“, die Kategorie einer eigenen Meldung
    mit Namen; `axe` ohne Befund.
  - Angepasst: `TestTenantBoundary` erwartet die Regel der Auswahlliste in Klammern, weil
    mehrere Regeln mit ODER verbunden werden; die Bedeutung ist gleich.
  - Selbsttest auf 8092 in Chrome: die Störung „Test“ zeigt im Portal die Kategorie
    „Telefonie“ statt der Kennung, auch in der Zeitleiste. Der Bereich „Wissen“ zeigte zuerst
    keinen Kategorienbaum: die Abfrage sortierte nach zwei Spalten, der Server nimmt eine (422).
    Behoben (nach Reihenfolge sortiert, nach Namen im Browser); die Attrappe der Vitest-Fälle
    lehnt zwei Sortierspalten jetzt ebenso ab, damit der Test den Fehler gefunden hätte.
  - Ganze CI am 2026-11-04 im ersten Lauf grün (19 Schritte, 103 Vitest-Fälle, 18 Tests im
    Browser, Lastmessung ohne Verschlechterung über die Toleranz); nach der Korrektur
    im vierten Lauf grün. Lauf 2 und 3 waren nur in der Lastmessung rot (LZ2 +49 % und LZ13
    −18 %, dann LZ4 +13 %) bei unverändertem Server-Code; jedes Mal eine andere Messung, darum
    als Rauschen gewertet.
- **Nicht geprüft:** Kein Browser-Test in der CI für den Bereich „Wissen“ (die Vitest-Fälle
  laufen in jsdom; im echten Browser nur der Selbsttest oben, ohne Artikel). Die Suche gilt im ganzen Bereich, nicht nur in der gewählten Kategorie.
  Die Startdaten bringen keine Artikel mit. „Nur bestimmte Gruppen“ ohne Gruppen lesen nur
  Bearbeiter; eine Pflicht für die Rolle gibt es nicht, weil Feldzustände keine Rollen kennen.

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
  - Nachtrag 2026-11-04: Der dritte Lauf war nicht wirklich grün. `TestProblems` in
    `internal/forms` erwartete noch die alte Meldung „not both“; die CI verschluckte das rote
    Paket (Fehler im Skript seit PL52, behoben im Commit 327ef18, Kern-Plan PL52). Der Test ist
    mit SD16b an die neue Meldung angepasst, die Feld, Rolle oder freie Frage nennt.
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
