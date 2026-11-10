# Plan: Oberfläche (UI)

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
| **UI1** | Rahmen der Anwendung: TanStack Router, Anmeldeseite, Navigation, Theme über Design-Tokens (E62), shadcn/ui-Grundbausteine, `axe` in Vitest und Playwright | ✅ | 2 d | Im Browser http://localhost:8092 → Anmeldeseite, nach Anmeldung die Navigation; ein Theme mit anderer Hauptfarbe ändert alle Bausteine; Playwright mit `axe` ohne Befund; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI2** | Liste und Formular aus der Definition: TanStack Query, Table und Virtual, Seiten per Schlüssel, Spaltenwahl; Formular aus den Feldtypen mit `react-hook-form` und `zod`; Konfliktanzeige (E29); API-Client aus OpenAPI (Inventar G229) | ✅ | 2 d | Liste des Typs `probe` mit 100.000 Datensätzen scrollt ohne Ruckeln; anlegen und ändern im Formular; zwei Fenster ändern denselben Datensatz → das zweite zeigt den Konflikt mit beiden Werten; `docker compose run --rm tools scripts/ci.sh` grün |

### B — Verbreitern bis Meilenstein 1

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **UI3** | Detailseite mit Zeitleiste und Verlauf (Inventar G40); die Zeitleiste liefert PL42 (Hauptplan E121) | 🟢 | 2 d | Detailseite eines Datensatzes zeigt Folgemeldungen und Feldänderungen in zeitlicher Reihenfolge; private Einträge sieht nur, wer darf; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI28** | Auswahlfeld mit Suche für Verweise und Rollen: eigener Baustein nach dem WAI-ARIA-Muster „Combobox“, Suche über `/api/v1/search` bei Typen mit durchsuchbaren Feldern, sonst die ersten 100 mit Filter im Browser; Verweisfelder im Formular und die Rollen am Datensatz als Abschnitt im Formular, gespeichert mit den Feldern (Hauptplan E123) | 🟢 | 1,5 d | Im Formular „Übergeordnet“ per Name suchen und wählen; als Anforderer eine Person, als Bearbeiter eine Gruppe setzen, speichern → Seitenleiste und Zeitleiste zeigen die Namen; nur mit Tastatur bedienbar, `axe` ohne Befund; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI4** | Filter und gespeicherte Suchen, privat oder geteilt, auf Grundlage der JSON-Abfrage (E40) (Inventar G112; Hauptplan E124) | 🟢 | 2 d | Filter setzen, als Suche speichern, teilen → eine andere Person sieht sie in ihrer Liste mit eigenen Rechten; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI5** | Export der Liste als CSV, XLSX und ODS (Inventar G230, G231; Hauptplan E125) | 🟢 | 1 d | Export einer gefilterten Liste enthält genau die sichtbaren Spalten und Zeilen; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI6** | Massenaktionen auf gewählte Datensätze über die Bulk-API (E24) (Inventar G234); gebaut über den neuen Endpunkt `:batch` (Hauptplan E126) | 🟢 | 1,5 d | 500 Datensätze wählen, Feld setzen → ein Bulk-Aufruf, Ergebnis je Zeile angezeigt; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI7** | Editor für formatierten Text mit eingefügten Bildern; Bilder als Dateien (E22); TipTap nach E1 (Inventar G242; Hauptplan E127) | 🟢 | 1,5 d | Bild per Einfügen in eine Folgemeldung → erscheint als Datei am Datensatz und im Text; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI8** | Live: SSE einbinden, offene Listen und Detailseiten aktualisieren, anzeigen wer gerade bearbeitet, Benachrichtigung im Browser (Inventar G164; Hauptplan E130) | 🟢 | 1,5 d | Änderung in Fenster A erscheint in Fenster B ohne Neuladen; B zeigt „wird von A bearbeitet"; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI9** | Einstellungen: Sprache, Formate, Seitengröße, Standardwerte; persönlich und global (Inventar G170, G171, G205); gebaut nach Hauptplan E131: Sprache, Zeitzone, Theme, Vorbelegung beim Anlegen | 🟢 | 1 d | Sprache auf Englisch → alle Texte englisch, auch nach Neuanmeldung; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI10** | Zwei Oberflächen: Portal für Nutzer, Arbeitsplatz für Bearbeiter, gesteuert über die Rolle (Inventar G125; Hauptplan E132) | 🟢 | 2 d | Eine Person ohne Bearbeiterrolle landet im Portal und erreicht den Arbeitsplatz nicht, auch nicht per URL; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI11** | Formulare: Builder mit Abschnitten und Fragen aus den Feldtypen des Kerns, Anordnung per `dnd-kit` (trägt KF8; Inventar G130, G131; Hauptplan E133) | 🟢 | 2 d | Formular mit drei Abschnitten bauen, speichern, im Portal ausfüllen → Datensatz entsteht; per Tastatur allein bedienbar (`axe` ohne Befund); `docker compose run --rm tools scripts/ci.sh` grün |
| **UI12** | Bedingungen in Formularen und Übersetzung von Formularen (Inventar G132, G136; Hauptplan E134) | 🟢 | 1,5 d | Frage B erscheint nur bei Antwort „ja" auf A; dasselbe Formular englisch im Portal bei englischer Sprache; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI13** | Dashboards: Widgets mit ECharts (Torte, Balken, Linie, Kennzahl, Liste, Markdown), Filter, anlegen, teilen, als Kachel einbetten (trägt KF11; Inventar G238, G239) | ⬜ | 2 d | Dashboard mit vier Widgets aus gespeicherten Suchen anlegen, teilen, eine andere Person sieht ihre eigenen Zahlen; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI14** | Statistik und Helpdesk-Dashboard als mitgelieferte Dashboards (Inventar G76, G77) | ⬜ | 1,5 d | Nach Installation des Service-Desk-Pakets ist das Helpdesk-Dashboard da und zeigt die offenen Vorgänge je Status; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI15** | Kalenderansicht für Aufgaben und Termine aller Bearbeiter (trägt KF10; Inventar G73) | ⬜ | 2 d | Aufgaben mit Termin erscheinen im Wochenkalender; Verschieben per Ziehen ändert den Termin; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI16** | Weitere Sprachen als ladbares Sprachpaket (Inventar G247) | ⬜ | 0,5 d | Ein Probe-Sprachpaket „fr" mit drei Texten wird geladen, fehlende Texte fallen auf Deutsch zurück; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI25** | Typ- und Feld-Editor: Kunden ändern Typen in der Oberfläche, Änderungen landen in der Kundenschicht (Hauptplan E37; E29) | 🟢 | 2 d | Feld an einem Typ ergänzen → erscheint in Liste und Formular ohne Neustart; Export der Kundenschicht enthält genau diese Änderung; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI26** | Workflow-Editor, geteilt am 2026-10-30 in UI26.1 und UI26.2 (E26, §2) | ⬜ | 3,5 d | siehe UI26.1 und UI26.2 |
| **UI26.1** | Workflow-Editor als Tabelle: Status anlegen, umbenennen, Anfangsstatus wählen; Übergänge mit Von (mehrere), Nach und CEL-Bedingung; Prüfung im Editor, Speichern in die Kundenschicht (E26, E30) | 🟢 | 1,5 d | Im Editor des Typs `probe` nur mit Tastatur einen Status und einen Übergang mit zwei Ausgangsstatus und Bedingung anlegen → per API sofort nutzbar, der Export der Kundenschicht enthält genau diese Änderung; eine ungültige CEL-Bedingung → Fehler am Feld; ein Status ohne eingehenden Übergang wird gemeldet; `axe` ohne Befund; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI26.2** | Workflow-Editor als Graph mit React Flow über den Tabellen, einklappbar (E31): Status als Knoten, je Ausgangsstatus eine Kante, ein Übergang wird als Ganzes markiert; Anordnen mit dagre, Knoten verschieben, Übergang per Ziehen als Abkürzung zur Tabelle; Anordnung im Feld `layout` der Kundenschicht, dafür kennt `internal/meta` das Feld (E26, E27) | 🟢 | 2 d | Der Graph zeigt alle Status und Übergänge von `probe`; Klick auf eine Kante markiert alle Kanten ihres Übergangs und die Zeile in der Tabelle; Knoten per Pfeiltaste verschieben und speichern → nach Neuladen an derselben Stelle; „Anordnen“ ordnet neu; ein gezogener Übergang erscheint in der Tabelle; neue Pakete nur unter MIT; `axe` ohne Befund; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI27** | Rollen-Editor als Matrix Typen × Aktionen mit Reichweite je Zelle, Bedingung, Feldrechten, Sonderrechten und Zuweisungen (E33) | 🟢 | 2 d | Rolle im Editor einschränken → die betroffene Person sieht in der Liste sofort weniger; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI29** | Design-Kit „Klartext“ in die Tokens und Bausteine übernehmen: Werte und neue Tokens nach §2.1 in `tokens.css`; IBM Plex Sans und Mono (OFL) im Werkzeug ausgeliefert; Knöpfe in den Varianten des Kits, Bausteine für Status, Priorität und Frist (E22, E25); dunkler Modus mit der Einstellung „Hell, Dunkel, wie das System“ (E24, E34) | 🟢 | 2 d | Im Browser http://localhost:8092 → Schrift IBM Plex, Hauptknopf graphit, Fokusrahmen petrol; das Netzwerkprotokoll zeigt keinen Abruf außerhalb des Werkzeugs; Theme `probe` ändert weiter alle Bausteine; „Dunkel“ in Meine Einstellungen → alle Seiten dunkel, auch nach Neuanmeldung; Playwright mit `axe` (mit Kontrast) in beiden Modi ohne Befund; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI30** | Rahmen und Liste nach dem Entwurf (E22): Kopf mit Suchfeld und Mandant, Navigation mit Bereichen und gespeicherten Suchen, Zeilenhöhe kompakt oder normal, Vorschau des gewählten Datensatzes rechts neben der Liste (E35), Fristen auf der Detailseite aus `…/clocks`. Eine Fristspalte in der Liste nur, wenn die API die Restzeit je Zeile liefert, sonst als neue Aufgabe | 🟢 | 2 d | Umschalter „Kompakt“ → Zeilen 36 px, bleibt nach Neuladen; Klick auf eine Zeile zeigt die Vorschau, „Öffnen“ führt zur Detailseite; eine Frist über 75 % erscheint gelb, eine überschrittene rot, beide mit Text; bei 375 px Breite steht die Navigation über der Liste; `axe` ohne Befund; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI31** | Befehlszeile mit Strg K: Datensätze nach Titel finden über `/api/v1/search` (nach Nummer, sobald PL49 sie durchsuchbar macht), Befehle „Neu“ je Typ, „Gehe zu“ und Darstellung, Kürzel am Suchfeld angezeigt (E22, E36) | 🟢 | 1,5 d | Strg K, einen Teil des Titels tippen → der Datensatz erscheint, Enter öffnet ihn; „neu“ tippen → „Neu: Probe“; leer zeigt sie die zuletzt geöffneten Datensätze; alles nur mit Tastatur; `axe` ohne Befund; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI34** | Fristspalte in der Liste: die Liste und `:query` liefern je Zeile die nächste Frist der SLA-Uhren (Restzeit, Anteil, Zustand), sortier- und filterbar; Spalte mit dem Baustein Frist (E35, aus UI30 ausgelagert; E37) | 🟢 | 1 d | Liste von `probe` mit Spalte „Frist“: eine Frist über 75 % gelb, eine überschrittene rot, beide mit Text; nach Frist sortiert steht die knappste oben; LZ1 ohne Verschlechterung über die Toleranz; `axe` ohne Befund; `docker compose run --rm tools scripts/ci.sh` grün |

### C — Meilenstein 2

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **UI17** | Graph-Ansicht und Auswirkungsanalyse (Inventar G114) | ⬜ | 2 d | Graph eines CI zeigt abhängige Elemente über zwei Ebenen; Klick öffnet die Detailseite; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI18** | Asset-Dashboard als mitgeliefertes Dashboard (Inventar G32) | ⬜ | 0,5 d | Nach Installation des Asset-Pakets ist das Dashboard da; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI19** | Externe Links und Handbuch-Links an Typen und Datensätzen (Inventar G192, G193) | ⬜ | 1 d | Ein Link mit Platzhalter `{{serial}}` öffnet die Adresse mit der Seriennummer des Datensatzes; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI32** | SLA-Pflegemaske: eine Seite je Service-Level mit Ziel, Kalender oder rund um die Uhr und den Bedingungen gilt, Pause und erfüllt; darunter die Eskalationsstufen (Regeln mit Auslöser `clock`, WF10) und die Wartegründe mit ihren Erinnerungen (WF11), zusammen anlegen und ändern statt über Liste, Formular und Ids (E32, §2.2; IP29) | ⬜ | 2 d | Im Arbeitsplatz unter Verwaltung › Service-Level eine SLA „Lösung P2“ mit 480 Minuten, Kalender „Büro“, Stufe bei 80 % „Priorität hoch“ und Wartegrund „Rückmeldung Anforderer“ anlegen → `GET /api/v1/records/core.sla` und die Regel-API zeigen die drei Datensätze mit den Verweisen; eine Bedingung, die sich nicht übersetzen lässt, erscheint als Fehler am Feld; Playwright mit `axe` ohne Befund; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI33** | SLA-Kennzahlen: Einhaltung je Zeitraum, Typ, Service-Level, Gruppe und Kategorie aus den Uhren (erfüllt, verletzt, pausiert, mittlere Restzeit) als Widgets für UI13 und ein mitgeliefertes Dashboard „Service-Level“; die Zahlen kommen über die Abfrage-API, reicht sie nicht, kommt eine Aufgabe im Kern-Plan dazu (E32, §2.2; IP29, KF11) | ⬜ | 2 d | Zehn Störungen mit Testzeit anlegen, vier reißen die Frist → das Dashboard „Service-Level“ zeigt 60 % Einhaltung für den Monat und die Zahlen je Gruppe; ein Filter auf eine Kategorie ändert die Zahlen; der Export der Kennzahl als CSV stimmt mit der Liste der Uhren überein; `docker compose run --rm tools scripts/ci.sh` grün |

### D — Meilenstein 3 und 4

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **UI20** | Kanban-Ansicht (Inventar G103) | ⬜ | 1,5 d | Karte in eine andere Spalte ziehen → Statusübergang wird ausgelöst oder mit Grund abgelehnt; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI21** | Externe Termine mit Vorlagen und Kategorien (Inventar G74) | ⬜ | 1 d | Termin aus Vorlage anlegen, erscheint im Kalender; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI22** | Notizen mit Sichtbarkeit für Personen, Gruppen, Rollen; RSS-Feeds aus internen Quellen (E26) (Inventar G104, G105) | ⬜ | 1,5 d | Eine Notiz für eine Gruppe sehen nur deren Mitglieder; ein interner Feed erscheint auf der Startseite; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI23** | Feste Berichte und Export als PDF (Inventar G111, G232) | ⬜ | 2 d | Bericht „Verträge, die in 90 Tagen enden" als PDF exportieren, Inhalt stimmt mit der Liste überein; `docker compose run --rm tools scripts/ci.sh` grün |
| **UI24** | Kartenansicht über Orte mit Koordinaten, mit Kartenmaterial ohne Internet (PMTiles und MapLibre nach E2) (Inventar G233) | ⬜ | 2 d | Orte erscheinen auf der Karte, ohne dass der Browser eine Adresse außerhalb des Werkzeugs aufruft (Netzwerkprotokoll); `docker compose run --rm tools scripts/ci.sh` grün |

**Summe:** 57 d; 43 d geschätzt vor dem Bau (Block A: 4 d), UI28 am 2026-10-29 vor UI4 eingeschoben (Hauptplan E123), UI29 bis UI31 am 2026-10-30 aus dem Designentwurf (E22), UI26 am 2026-10-30 von 2 d auf 3,5 d (E26), UI32 und UI33 am 2026-11-02 für das SLA-Management nach Meilenstein 1 (E32), UI27 am 2026-11-02 von 1,5 d auf 2 d für die Zuweisungen (E33), UI34 am 2026-11-02 aus UI30 ausgelagert (E35).

**Stand der Reihenfolge (2026-10-28, Hauptplan E108):** Block B der Oberfläche kommt nach Block B der Workflow-Engine. Schon gebaut aus WF2 (WF-Plan E7): Das Formular holt die Feldzustände (`:field-states`), blendet Versteckte aus, markiert Pflichtfelder, zeigt Schreibgeschützte als Text und einen gesperrten Datensatz ohne „Speichern“. Offen für Block B: Rollen am Datensatz (WF3), Regeln (WF4), Kalender (WF8), SLAs (WF9), Eskalationsstufen (WF10, Regeln mit Auslöser `clock`), Wartegründe (WF11) und Zeitregeln (WF12), beide mit SLA und Stufen anzulegen, sowie Aufgaben am Vorgang (WF13; der Kalender UI15 liest sie per `:query` über `planned_start`) wiederkehrende Vorgänge (WF14, Vorlage als JSON-Text) und Benachrichtigungen (WF15, Empfänger und Ereignisse als Text) haben außer Liste und Formular der Typen und der Regel-API noch keine Oberfläche; die Uhren eines Datensatzes (`…/clocks`, Restzeit, verletzt) zeigt die Detailseite noch nicht. Für UI3 liefert PL42 (Hauptplan E121) die Zeitleiste: `GET …/{id}/timeline` mit Einträgen und Verlauf in zeitlicher Reihenfolge samt Namen, `POST …/{id}/timeline` für einen neuen Eintrag (Recht `comment`, privat zusätzlich `read_private`).

**Reihenfolge:** Block A mit dem Durchstich (Hauptplan E59): UI1, dann UI2, sobald PL4 steht. Danach Block B: UI3, UI8 und UI10 zuerst, weil der Service Desk (SD) sie als Erstes braucht; dann die Editoren UI25 bis UI27; Formulare UI11 und UI12 vor SD; Dashboards UI13 und UI14 zum Ende von Meilenstein 1. Block C und D mit ihren Meilensteinen. UI29, UI30 und UI31 kommen in dieser Folge nach den Editoren UI25 bis UI27 und vor SD (E23). UI32 und UI33 stehen in Block C nach Meilenstein 1 (E32).

---

## 1. Entscheidungen

Die übergreifenden Entscheidungen stehen im Hauptplan §1 (E5, E41, E42, E57, E58, E62).

| # | Entscheidung | Seit | Quelle |
| --- | --- | --- | --- |
| **E1** | Editor für formatierten Text ist TipTap (MIT), nur die freien Grundfunktionen (V1) | 2026-10-20 | Pascal |
| **E2** | Karten ohne Internet: Vektorkarte als PMTiles-Datei im Dateispeicher, angezeigt mit MapLibre (BSD); welche Kartendaten, klärt UI24 mit dem Kunden (V2) | 2026-10-20 | Pascal |
| **E3** | Routen im Code (`web/src/router.tsx`), nicht aus Dateinamen: der Baum steht an einer Stelle, Tests bauen denselben Router mit Speicher-Historie; Schutz vor Seiten ohne Sitzung im `beforeLoad` der Rahmen-Route (401 → `/login`, abgelaufenes Kennwort → `/password`, Hauptplan E77) | 2026-10-20 | Umsetzung |
| **E4** | Theme = Satz von Design-Tokens als CSS-Variablen in `web/src/styles/tokens.css`, gewählt über `data-theme` am `<html>` und im `localStorage` gemerkt; Tailwind kennt nur die Tokens (`@theme inline`), Komponenten keine festen Farben (ein Test prüft das). Standard neutral (Hauptplan E62), `probe` als zweites Theme für den Prüfweg; Theme-Pakete je Kunde liefern später dieselben Variablen | 2026-10-20 | Umsetzung |
| **E5** | shadcn/ui-Bausteine von Hand im Repository (`web/src/components/ui`: Button, Input, Label, Card, Alert auf Radix, cva, tailwind-merge), nicht per CLI erzeugt: nur was gebraucht wird, mit Lizenzkopf und ohne fremde Farbwerte | 2026-10-20 | Umsetzung |
| **E7** | `web/src` folgt Architektur §10 (`app`, `api`, `render`, `ui`, `i18n`, später `views`, `editors`, `portal`, `dashboards`); Importe über den Alias `@/` | 2026-10-20 | Umsetzung |
| **E8** | API-Client aus der OpenAPI: `openapi-typescript` erzeugt `web/src/api/schema.d.ts` aus `api/openapi.yaml` (die CI prüft, dass es aktuell ist), `openapi-fetch` ruft typisiert auf; die Typdefinitionen (Felder) kommen zur Laufzeit aus dem Meta-Modell und sind einmal in `api/client.ts` beschrieben | 2026-10-20 | Umsetzung |
| **E9** | Liste: Seiten zu 100 per Schlüssel (`after`) über `useInfiniteQuery`, Zeilen virtuell (TanStack Virtual, nächste Seite, sobald die letzten 20 Zeilen sichtbar werden), Sortierung nur auf den Schlüsselspalten der API (id, version, created_at, updated_at), Spaltenwahl je Typ im `localStorage`; nur sichtbare Spalten werden angefordert (`fields`) | 2026-10-20 | Umsetzung |
| **E10** | Konflikt beim Speichern (Hauptplan E29): nach 409 lädt das Formular den Serverstand und zeigt je abweichendem Feld beide Werte; „Serverstand übernehmen" lädt neu, „Meine Eingabe trotzdem speichern" schreibt mit der neuen Version. Wer gerade bearbeitet (live) kommt mit SSE in einer späteren Aufgabe | 2026-10-20 | Umsetzung |
| **E11** | Detailseite (Hauptplan E122): `RecordPage` ordnet Kopf, Formular mit Zeitleiste und Seitenleiste in einem Raster (`lg`: Inhalt und 20rem); alle Teile hängen unter dem Schlüssel des Datensatzes (`["records", typ, id, …]`), Speichern und Übergang laden sie gemeinsam neu. Die Zeitleiste kommt in der Reihenfolge der API und wird im Browser gruppiert und umgedreht. Die Bezüge der Seitenleiste kommen aus den Summenfeldern (`rollup`) des Typs, ohne eigene Angabe je Typ. Die Ampel nimmt die Tokens `--success`, `--warning`, `--destructive` | 2026-10-29 | Umsetzung |
| **E12** | Auswahlfeld (Hauptplan E123): `RefPicker` ist eine Combobox nach dem WAI-ARIA-Muster ohne Bibliothek; ob ein Typ gesucht oder gelistet wird, entscheidet `searchable` an seinen Feldern. Im Formular laufen Verweise über `Controller` von react-hook-form (Wert bleibt die ID, die zod-Regel `uuid` gilt weiter); die Rollen leben als eigener Zustand neben react-hook-form und gehen nur geändert in den Körper | 2026-10-29 | Umsetzung |
| **E13** | Filter und gespeicherte Suchen (Hauptplan E124): Die Filter sind eine Liste von Bedingungen, `toWhere` und `fromWhere` übersetzen sie in die JSON-Abfrage und zurück; was mehr ist als UND von Bedingungen, gilt unverändert und wird als Text gezeigt. Die Liste fragt mit Bedingung `:query`, sonst wie bisher `GET`. Eine gespeicherte Suche ist ein Datensatz von `core.saved_search`; die Seite des Typs lädt ihn über `?search=<id>` und baut die Ansicht neu, sobald Suche oder Version wechseln | 2026-10-29 | Umsetzung |
| **E14** | Export (Hauptplan E125): Der Knopf in der Liste schickt die Ansicht, wie die Liste sie gerade hat (Bedingung, sichtbare Spalten in ihrer Reihenfolge, Sortierung), dazu Sprache und Zeitzone des Browsers; der Server antwortet mit der Datei oder mit dem Job. Werte schreibt der Server, nicht der Browser, damit Export und Liste dieselben Bezeichnungen haben | 2026-10-29 | Umsetzung |
| **E15** | Massenaktionen (Hauptplan E126): Die Auswahl ist der Zeilenzustand von TanStack Table, geschlüsselt nach der Kennung; die Versionen kommen aus den geladenen Zeilen. Die Leiste schickt eine Änderung für alle Gewählten in einem Aufruf an `:batch` und zeigt das Ergebnis je Zeile; danach lädt die Liste neu, die Auswahl bleibt | 2026-10-29 | Umsetzung |
| **E16** | Formatierter Text (Hauptplan E127): `RichEditor` (TipTap) schreibt HTML, `RichText` zeigt nur HTML, das der Server bereinigt hat (`dangerouslySetInnerHTML` an genau dieser einen Stelle). Bilder lädt der Editor als vorläufige Dateien hoch und zeigt sie aus `/api/v1/files/{id}`; der Server hängt die Bilder eines Eintrags an. `plainText` in `fields.ts` liefert den Klartext für Listen, damit Listen TipTap nicht laden | 2026-10-29 | Umsetzung |
| **E17** | Live (Hauptplan E130): Der Rahmen hält eine EventSource je Tab; Seiten abonnieren über `useLive`. Eigene Änderungen (gleiches Konto) lösen nichts aus, auch aus einem anderen Fenster; die Konfliktansicht (E10) bleibt dafür zuständig. Anwesenheit über `PUT …/presence` alle 20 s mit `editing`; die Liste patcht nachgelesene Zeilen in die Seiten der Abfrage, statt neu zu laden | 2026-10-29 | Umsetzung |
| **E18** | Einstellungen (Hauptplan E131): `mySettingsQuery` lädt die Einstellungen der Person einmal; der Rahmen setzt daraus Sprache (`i18n`), Theme (`applyTheme`) und Zeitzone (`setDisplayTimeZone`, gelesen von `formatMoment`). Die Umschalter oben bleiben sofort wirksam und schreiben angemeldet nach `/api/v1/me/settings`. Die globale Seite nutzt die Einstellungen am Baum (`/api/v1/org/nodes/{path}/settings`) | 2026-10-29 | Umsetzung |
| **E21** | Bedingungen im Portal (Hauptplan E134): `Form` nimmt eine Sichtbarkeit `visibility(values)`, die ausgeblendete Felder und Gruppen nennt; sie werden weder gezeigt noch geprüft noch gesendet. Der Resolver rechnet sie aus den Werten, die er prüft. Dieselbe Auswertung steht im Server (`internal/forms`) und im Portal (`portal/conditions.ts`) | 2026-10-29 | Umsetzung |
| **E20** | Formulare (Hauptplan E133): der Baukasten ist die Darstellung des Felds `definition` von `core.form` im Formular des Datensatzes; das Portal füllt ein Formular mit demselben `Form`, dessen Felder aus den Fragen gebildet werden (gebundene mit dem Feld des Zieltyps, freie als Feld ohne Spalte), gruppiert nach Abschnitten | 2026-10-29 | Umsetzung |
| **E19** | Portal (Hauptplan E132): eigene Routen unter `/portal` mit eigenem Rahmen (`portal/Portal.tsx`); der Rahmen des Arbeitsplatzes prüft `me.workplace` im `beforeLoad` und schickt sonst ins Portal. Das Portal nutzt dieselben Bausteine (Formular mit `only`, Zeitleiste mit `portal`) und dieselbe API; „Meine“ ist eine JSON-Abfrage über Anforderer und Ersteller | 2026-10-29 | Umsetzung |
| **E22** | Gestaltung nach dem Designentwurf „Klartext“ ([Design-Fläche](https://claude.ai/artifact/1TScdXrimrgmFwUWFUdejH), mit erfundenen Beispielen und ohne Kundendaten). Graphit (#15171A) für Text und Hauptaktion, Petrol (#0B6E80) als einziger Akzent für Fokus, Auswahl und Links. Gelb und Rot nur für knappe und überschrittene Fristen, die Priorität ohne Ampel. Nummern und Zeiten in IBM Plex Mono, Text in IBM Plex Sans, beide im Werkzeug ausgeliefert. Neutral im Sinne von Hauptplan E62: ein Kunde tauscht Tokens, nicht Bausteine. Die Werte stehen in §2.1. Maßgeblich ist nach UI29 `tokens.css`, nicht die Design-Fläche | 2026-10-30 | Pascal |
| **E23** | Reihenfolge des Designs (V5): UI29 bis UI31 nach den Editoren UI25 bis UI27 und vor SD. Die Editoren entstehen noch im bisherigen Aussehen; UI29 zieht sie über die Tokens nach | 2026-10-30 | Pascal |
| **E24** | Dunkler Modus (V3): ein zweiter Satz Werte für dieselben Tokens, unabhängig vom Theme des Kunden; ein Theme-Paket liefert seine Werte für hell und dunkel. Gewählt in Meine Einstellungen mit „Hell, Dunkel, wie das System“, Vorgabe „wie das System“ (`prefers-color-scheme`). Werte nach der Design-Fläche, Kontrast nach WCAG 2.1 AA in beiden Modi | 2026-10-30 | Pascal |
| **E25** | Hauptaktion (V4): standardmäßig Graphit; ein Theme-Paket darf `--primary` auf die Farbe des Kunden setzen, die Fristfarben bleiben | 2026-10-30 | Pascal |
| **E26** | Workflow-Editor (UI26): bearbeitet wird zuerst in einer Tabelle, die allein mit Tastatur vollständig ist; der Graph mit React Flow (Hauptplan E41) steht daneben zum Ansehen, Prüfen und Anordnen. Automatisches Anordnen mit dagre (MIT), nicht mit elkjs (EPL-2.0). Begründung in §2 | 2026-10-30 | Pascal |
| **E27** | Anordnung im Workflow-Graph (V6): Feld `layout` am Typ in der Kundenschicht, je Status `x` und `y`; die Engine liest es nicht, `internal/meta` prüft nur, dass jeder Name ein Status des Typs ist. Ohne `layout` ordnet dagre bei jedem Öffnen. Gebaut mit UI26.2, ohne eigene Aufgabe im Kern-Plan | 2026-10-30 | Pascal |
| **E28** | Rand der Felder (V7): `--input` hell #8A8F98, dunkel #6E737A, damit der Rand eines Felds 3 : 1 gegen `--card` erreicht (WCAG 2.1, 1.4.11; 3,2 und 3,7 : 1). `--border` bleibt hell für Trenner und Karten; Werte in §2.1. Geändert am 2026-11-02; vorher #CFCFC8 und #3A3F45 aus der Design-Fläche (1,6 : 1) | 2026-11-02 | Pascal |
| **E29** | Typ- und Feld-Editor (UI25): Knopf „Typ anpassen“ an der Liste eines Typs, nur mit dem Recht `customize` an `core.type` an der Wurzel (dafür meldet `auth/me` `customize`); eigene Seite `/types/<typ>/customize`. Jede Aktion ist sofort eine Änderung der Kundenschicht (Hauptplan E37, E135), eine andere Version des Gegenübers ergibt eine Meldung und frisches Laden. Neue eigene Felder `k_…` in allen sieben Feldtypen, Text auch als formatierter Text, Verweis mit Zieltyp. Darunter die Liste der Änderungen mit Wer, Wann, Status und Grund, je mit „Zurücknehmen“, und der Export der Kundenschicht als JSON-Datei. Reihenfolge per Ziehen mit dnd-kit wie im Formular-Builder und mit „hoch“ und „runter“ | 2026-11-02 | Pascal |
| **E30** | Workflow-Editor als Tabelle (UI26.1, deine Entscheidungen 2026-11-02): eigene Seite `/types/<typ>/workflow` mit Link „Workflow“ an der Liste und im Typ-Editor, nur mit `customize`. Die Kundenschicht (Kern-Plan PL43) bekommt dafür die Operationen `add_state`, `change_state`, `remove_state`, `add_transition`, `change_transition`, `remove_transition`, `disable_transition`, `enable_transition`: eigene Status und Übergänge `k_…` frei; Status des Pakets nur Beschriftung und Anfangsstatus; Übergänge des Pakets Beschriftung, Von, Nach, Bedingung und stilllegen statt löschen; ein Status mit Datensätzen oder mit Übergängen bleibt. Die CEL-Bedingung prüft der Server beim Tippen über `POST /api/v1/types/<typ>:check-condition`; im Browser dazu Hinweise auf Status ohne eingehenden Übergang und stillgelegte Übergänge. Jede Zeile ist eine Änderung wie in UI25 | 2026-11-02 | Pascal |
| **E31** | Workflow-Graph (UI26.2, deine Entscheidungen 2026-11-02 nach dem Mockup „Workflow-Graph Platz“, [Artifact](https://claude.ai/artifact/FxWG8r1FE6YuuxdqFWp6oR) mit drei Varianten): **Variante A**, der Graph steht oben über den Tabellen in voller Breite und lässt sich einklappen; ob er offen ist, merkt sich der Browser je Konto. Verschieben und „Anordnen“ wirken nur auf dem Bildschirm; **„Anordnung speichern“** schreibt das ganze Feld `layout` als eine Änderung `set_layout` der Kundenschicht (zurücknehmbar), ein ungespeicherter Stand wird beim Verlassen gemeldet. Eine **gezogene Verbindung** von Status A nach B füllt das Formular „Neuer Übergang“ vor (Von = A, Nach = B, Fokus auf der Beschriftung); der Übergang entsteht erst mit „Übergang anlegen“. Verworfen: Graph neben den Tabellen (Tabellen zu eng), eigener Reiter (nie beides zugleich), Speichern nach jedem Verschieben (Liste der Änderungen voller Verschiebungen), Anlegen beim Loslassen mit erzeugter Beschriftung | 2026-11-02 | Pascal |
| **E38** | Reiter der Detailseite (aus SD4, SD-Plan E7, deine Entscheidungen 2026-11-03 nach dem Mockup „Bezüge am Vorgang“, [Artifact](https://claude.ai/artifact/LB9eW2KAQP7HHerk2ZMugJ)): unter dem Formular stehen Reiter, „Bezüge“ zuerst, „Zeitleiste“ zuletzt, „Dateien“ kommt dazwischen, sobald es sie gibt. Bezüge als Tabelle (Art, Typ, Datensatz, Status, Lösen) mit beiden Richtungen und der Beschriftung der Beziehungsart; darüber Art und Datensatz per Suche über alle Typen. Seit SD18 (SD-Plan E12, 2026-11-10) kommt nach „Bezüge“ je Typ, der hier vorgeschlagen wird, ein eigener Reiter mit dessen Namen, etwa „Wissen“. Verworfen: Karte in der Seitenleiste (A), Abschnitt unter dem Formular (B), Zeitleiste als erster Reiter | 2026-11-03 | Pascal |
| **E37** | Fristspalte (UI34, deine Entscheidungen 2026-11-02): **Speicherung** als Systemspalten am Datensatz (nächste Fälligkeit und 75-%-Punkt der laufenden Uhren) mit Index, gepflegt in derselben Transaktion wie die Uhren, damit Sortieren und Blättern schnell bleiben. **Welche Uhr:** die nächste fällige laufende; überschrittene zuerst; eine pausierte nur, wenn keine läuft, als „pausiert“. **Filter:** überschritten, knapp (ab 75 %), fällig bis … Verworfen: Frist beim Abfragen aus der Uhren-Tabelle; je SLA eine Spalte; Filter „ohne Frist“ | 2026-11-02 | Pascal |
| **E36** | Befehlszeile (UI31, deine Entscheidungen 2026-11-02 nach dem Mockup „Befehlszeile Strg K“, [Artifact](https://claude.ai/artifact/Y7SKJQGpz8L6cRGy6WfkLX) mit zwei Varianten): **Dialog in der Mitte**, 640 px, Seite dahinter abgedunkelt, Esc gibt den Fokus zurück; das Suchfeld im Kopf öffnet denselben Dialog. **Nummer:** UI31 findet über Titel und Volltext; die Suche nach der laufenden Nummer kommt ohne weitere Arbeit mit PL49, deren Prüfweg den Schritt übernimmt. **Befehle:** „Neu“ je Typ, „Gehe zu“ (Seiten der Navigation, gespeicherte Suchen, Einstellungen, Verwaltung), Darstellung (Hell, Dunkel, System; Kompakt, Normal). **Leer:** die fünf zuletzt geöffneten Datensätze, je Konto im Browser gemerkt. Verworfen: Ausklapper unter dem Suchfeld; PL49 vorziehen; Übergänge des offenen Datensatzes als Befehle; zuletzt Geöffnete am Konto auf dem Server | 2026-11-02 | Pascal |
| **E35** | Rahmen und Liste (UI30, deine Entscheidungen 2026-11-02 nach dem Mockup „Listen-Vorschau“, [Artifact](https://claude.ai/artifact/QZQNJhazfaMTJ64uB32wV8) mit zwei Varianten): **Vorschau rechts** neben der Liste, 400 px, mit „Schließen“ und „Öffnen“; unter 1100 px Breite führt der Klick direkt zur Detailseite. **Zeilenhöhe** „Kompakt“ (36 px) oder „Normal“ (52 px) je Person in den Einstellungen (`density`, Vorgabe am Baum `ui.density`, sonst „Normal“), Umschalter an der Liste. **Fristspalte** wird eine eigene Aufgabe (UI34), weil die Liste die Restzeit je Zeile noch nicht liefert. Verworfen: Vorschau unter der Liste; Zeilenhöhe nur im Browser; Fristspalte in UI30 | 2026-11-02 | Pascal |
| **E34** | Design-Kit im Werkzeug (UI29, deine Entscheidungen 2026-11-02): **Schrift** IBM Plex Sans und Mono als npm-Pakete von Fontsource; die SIL Open Font License wird nur für diese zwei Schriftpakete erlaubt (Ausnahmen in `test/licenses/licenses.txt`), für Code bleibt sie gesperrt. Begründung: Eine gleichwertige Schrift unter MIT oder Apache gibt es kaum; die OFL ist von OSI und FSF als freie Lizenz anerkannt, gilt nur für die Schrift selbst und erlaubt die Auslieferung mit kommerzieller Software. **Darstellung** „Hell, Dunkel, wie das System“ je Person in Meine Einstellungen (`color_scheme`), Vorgabe am Organisationsbaum `ui.color_scheme`, sonst „wie das System“; der Browser merkt sich die letzte Wahl, damit eine dunkle Seite nicht hell aufblitzt. Für die Bausteine kennt die Typdefinition zwei optionale Angaben: `category` am Status (open, active, waiting, done) für die Farbe des Punkts und `format: priority` an Zahlfeldern für die Prioritätsmarke; ohne Angabe ist der Anfangsstatus „open“ und jeder andere „active“. Verworfen: Systemschrift ohne eigene Dateien, alte Apache-Fassungen von Roboto oder Open Sans, OFL allgemein erlauben, Darstellung nur im Browser | 2026-11-02 | Pascal |
| **E33** | Rollen-Editor (UI27, deine Entscheidungen 2026-11-02 nach dem Mockup „Rollen-Editor Aufbau“, [Artifact](https://claude.ai/artifact/437B156YyKtxHdik1AGhsG) mit zwei Varianten): **Matrix** je Typ eine Zeile, Spalten lesen, anlegen, ändern, löschen, in jeder Zelle die Reichweite (– , eigene, Gruppe, beteiligt, Einheit, Unterbaum, alle); „…“ an der Zelle öffnet darunter Bedingung, Feldrechte und weitere Reichweiten. **Jede Änderung gilt sofort** und steht im Sicherheitsprotokoll. **Feldrechte:** „Alle Felder“ als Vorgabe, abgewählt die Felder zum Ankreuzen; je Zelle die Spalte, die für ihre Aktion zählt (lesen bei „lesen“, schreiben bei „anlegen“ und „ändern“, keine bei „löschen“), weil der Kern Feldrechte je Aktion auswertet. **Sonderrechte** als Kästchen mit Reichweite „alle“; was weder Matrix noch Kästchen zeigen (etwa `read_private`, `*`), steht als „Weitere Rechte“ in einer Liste. **Zuweisungen** (Person oder Gruppe mit Knoten) als eigener Abschnitt, +0,5 d. Verworfen: Tabelle je Berechtigung, Entwurf mit „Rolle speichern“, Feldrechte als Liste von Ausnahmen | 2026-11-02 | Pascal |
| **E32** | SLA-Management nach Meilenstein 1 (deine Entscheidung 2026-11-02): Die Mechanik aus WF8 bis WF12 und die Fristen des Service Desk (SD3, SD6, SD7) reichen für Meilenstein 1. Pflegemaske (UI32) und Kennzahlen (UI33) kommen in Block C; ob OLA und Lieferantenverträge eigene Begriffe werden, fragt der Hauptplan die Fachseite (Hauptplan V39). Befund in §2.2 | 2026-11-02 | Pascal |
| **E6** | Barrierefreiheit prüft `axe-core` direkt: in Vitest je gerenderter Seite (ohne Kontrast, da jsdom kein Layout hat), in Playwright mit `@axe-core/playwright` nach WCAG 2 A und AA im Browser; ein Befund bricht die CI | 2026-10-20 | Umsetzung |

## 2. Begründung

- **Was UI baut,** steht in der [Architektur](architektur.md) §10.
- **Abgedeckt:** alle 33 Zeilen des Funktionsinventars mit Ziel UI und die Kernfähigkeiten KF8,
  KF10, KF11 als Träger. Dazu die Editoren UI25 bis UI27, die keine GLPI-Zeile haben, aber
  E7 und E37 brauchen.
- **Barrierefreiheit** (E42) ist keine eigene Aufgabe: `axe` läuft ab UI1 in jeder Prüfung.
- **Workflow-Editor mit React Flow** (E26), gelesen am 2026-10-30:
  - Lizenz MIT, Rechteinhaber webkid GmbH
    ([LICENSE](https://github.com/xyflow/xyflow/blob/main/LICENSE)). Knoten und Kanten sind
    DOM-Elemente, Tokens und dunkler Modus greifen ohne eigene Zeichenlogik.
  - Laut [Hersteller](https://reactflow.dev/learn/advanced-use/accessibility): Knoten und Kanten
    per Tab erreichbar, Verschieben mit Pfeiltasten, Ansage der Bewegung über `aria-live`,
    Beschriftungen übersetzbar über `ariaLabelConfig`. WCAG AA erreicht man nach eigener Aussage
    nur „when properly implemented“.
  - Lücke: Einen Übergang legt man durch Ziehen zwischen Anschlüssen an; einen Weg per Tastatur
    nennt die Seite nicht (gelesen, nicht ausprobiert). Darum ist die Tabelle der vollständige Weg.
  - Verworfen: Cytoscape.js (zeichnet auf Canvas, kaum barrierefrei, keine Tokens), JointJS
    (Bearbeiten nur in der bezahlten Fassung), bpmn-js (anderes Modell, Lizenz verlangt das
    Wasserzeichen), maxGraph (schwer, altes Aussehen), Eigenbau (etwa 3 bis 4 d mehr).
  - Aus dem Code: Ein Übergang hat mehrere Ausgangsstatus (`From []string`,
    `internal/meta/model.go:150` auf `main`), im Graph also mehrere Kanten für einen Übergang. Ein
    Status hat keine Position (`internal/meta/model.go:139`); gespeicherte Anordnung siehe E27.

### 2.1 Design-Kit „Klartext“: Werte

Die Werte für UI29, übernommen am 2026-10-31 aus der
[Design-Fläche](https://claude.ai/artifact/1TScdXrimrgmFwUWFUdejH) (Seiten „Ansichten“ und
„Kit“). Abgenommen hat Pascal den hellen Modus am 2026-10-30 (E22) und den dunklen am 2026-10-31.
Bis UI29 gilt diese Tabelle, danach `tokens.css` (E22). Die Kontraste sind nach der Formel von
WCAG 2.1 gerechnet, nicht im Browser gemessen; das tut UI29 mit `axe`.

**Farben.** Die Namen folgen `web/src/ui/tokens.css`. „neu“ heißt: Das Token gibt es dort noch
nicht. Kontrast ohne Angabe gegen `--card`.

| Token | Wofür | Hell | Dunkel | Kontrast hell · dunkel |
| --- | --- | --- | --- | --- |
| `--background` | Grund der Seite | #F6F6F3 | #111315 | |
| `--card` | Flächen, Tabellen, Felder | #FFFFFF | #181A1D | |
| `--card-foreground` | wie `--foreground` | | | |
| `--foreground` | Text | #15171A | #ECEDEA | 18,0 · 14,8 |
| `--foreground-2` (neu) | Zweittext | #4A4F57 | #B4B8BE | 8,2 · 8,8 |
| `--muted-foreground` | Beschriftung, Hilfetext | #6B7079 | #8F949B | 5,0 · 5,7; auf `--background` 4,6 · 6,1 |
| `--muted` | Segmente, Avatare, gesperrte Knöpfe | #ECECE7 | #24272B | |
| `--border` | Trenner, Karten | #E4E4DF | #2C3035 | |
| `--border-soft` (neu) | Trenner innerhalb einer Karte | #EFEFEA | #23262A | |
| `--border-strong` (neu) | Rahmen, der führt: Hinweis, Kachel im Portal | #15171A | #6E737A | |
| `--input` | Rand von Feldern und umrandeten Knöpfen | #8A8F98 | #6E737A | 3,2 · 3,7 (E28) |
| `--primary` | Hauptaktion, P1 | #15171A | #A9ADB2 | dunkel 7,7 |
| `--primary-foreground` | Text auf der Hauptaktion | #FFFFFF | #111315 | 18,0 · 8,3 |
| `--kbd-on-primary` (neu) | Grund des Kürzels im Hauptknopf | #3A3D44 | #8C9096 | |
| `--secondary` | umrandeter Knopf | wie `--card` | wie `--card` | |
| `--secondary-foreground` | wie `--foreground` | | | |
| `--accent` (neu) | Fokus, Auswahl, Links | #0B6E80 | #71ABB5 | 5,9 · 6,8 |
| `--accent-soft` (neu) | gewählte Zeile und Option | #E7F1F2 | #2A373B | Text darauf 15,6 · 10,5 |
| `--ring` | Fokusrahmen | wie `--accent` | wie `--accent` | |
| `--success` | Frist erfüllt, gelöst | #2E7D4F | #5BC48A | 5,0 · 8,1 |
| `--warning` | Balken der knappen Frist, Status „Wartet“ | #C77A00 | #E0A030 | 3,4 · 7,7 |
| `--warning-foreground` (neu) | Text der knappen Frist | #8A5200 | #F2C36B | auf `--warning-background` 5,9 · 8,4 |
| `--warning-background` (neu) | Grund der knappen Frist und von Hinweisen | #FFF4DE | #3A2A10 | |
| `--warning-border` (neu) | Rand des Hinweises im Portal | #F0D9A8 | #5A4317 | |
| `--destructive` | Frist gerissen, Fehler, Löschen | #B42318 | #FF8A80 | 6,6 · 7,6 |
| `--destructive-foreground` | Text auf rot gefülltem Grund | #FFFFFF | #111315 | 6,6 · 8,2 |
| `--destructive-background` (neu) | Grund der gerissenen Frist | #FDECEA | #3D1715 | Rot darauf 5,7 · 6,9 |
| `--surface-subtle` (neu) | Kopf von Tabellen, Unterflächen | #FBFBF9 | #1C1F22 | |
| `--surface-active` (neu) | aktiver Eintrag der Navigation | #E9E9E4 | #262A2E | |
| `--row-checked` (neu) | angehakte Zeile der Liste | #FAFAF7 | #1C1F22 | |
| `--segment-active` (neu) | gewähltes Segment | #FFFFFF | #3A3F45 | |
| `--clock-running` (neu) | Balken der laufenden Frist | #8A8F98 | #6E737A | auf `--clock-track` 2,7 · 2,9 |
| `--clock-paused` (neu) | Balken der angehaltenen Frist | #CFCFC8 | #3A3F45 | |
| `--clock-track` (neu) | Spur des Fristbalkens | #ECECE7 | #2A2E33 | |
| `--step-done` (neu) | erledigter Schritt im Portal | #15171A | #8F949B | |
| `--batch-background` (neu) | Leiste der Mehrfachauswahl | #15171A | #2A2E33 | |
| `--batch-foreground` (neu) | Text der Leiste | #FFFFFF | #ECEDEA | 18,0 · 11,6 |
| `--batch-border` (neu) | Trenner in der Leiste | #4A4F57 | #4A4F57 | |

**Regeln zu den Farben:**

- **Ein Theme-Paket** setzt `--accent` und, wenn es will, `--primary` (E25). Grund, Linien und
  Fristfarben bleiben, damit die Bedeutung überall gleich ist. Die Probe auf der Fläche:
  Akzent #3F4BB8, einmal auch als Hauptaktion.
- **Abgeleitet, nicht gesetzt:** `--accent` dunkel ist die Farbe des hellen Modus mit 58 %
  Farbe und 42 % Weiß gemischt. `--accent-soft` ist hell 10 % Akzent auf Weiß, dunkel 20 % Akzent
  auf `--card`. So bleibt ein Theme-Paket mit einer Farbe in beiden Modi lesbar.
- **`--muted-foreground` nie als Text auf `--muted`:** hell nur 4,2 : 1. Dort steht
  `--foreground-2` (7,0 · 7,5).
- **Der Fristbalken** erreicht keine 3 : 1 gegen seine Spur. Das ist erlaubt, weil die Restzeit
  immer als Text daneben steht und der Balken nur ergänzt.
- **Abweichungen der Fläche:** Die Fläche schreibt bei Petrol dunkel #72ABB5 und bei Rot 6,1 : 1.
  Gerechnet sind es #71ABB5 und 6,6 : 1; es gilt diese Tabelle. Das Portal der Fläche hatte dunkel
  `--warning-background` #2E2410. Einheitlich gilt jetzt #3A2A10 (Text darauf 8,4 : 1). Der
  Feldrand `--input` ist nach E28 kräftiger als auf der Fläche.

**Schrift:** IBM Plex Sans (400, 500, 600) und IBM Plex Mono (400, 500), SIL Open Font License.
Die Dateien liefert das Werkzeug aus, ohne Abruf aus dem Netz; die Fläche lädt sie nur zur
Ansicht von Google Fonts.

| Rolle | Größe / Zeilenhöhe | Gewicht | Wo |
| --- | --- | --- | --- |
| Display | 34 / 1,15, −0,025 em | 600 | Begrüßung im Portal |
| Titel | 22 / 1,25, −0,015 em | 600 | Seitentitel |
| Abschnitt | 15 / 1,4 | 600 | Überschriften in Karten |
| Text | 14 / 1,45 | 400 | Grundschrift |
| Klein | 12,5 / 1,4 | 400 | Angaben zu Person und Zeit, Hilfetext |
| Rubrik | 11,5, +0,06 em, Großbuchstaben | 500 | Gruppen der Navigation, Kopf von Bausteinen |
| Mono | 12,5 bis 13 | 400, 500 | Nummern, Zeiten, Fristen, Kürzel |

**Raster und Ecken:**

- **Raster 4 px:** 4 zwischen Symbol und Text, 8 in Gruppen, 12 zwischen Feldern, 16 innen in
  Flächen, 24 Seitenrand, 48 zwischen Abschnitten.
- **Ecken:** 4 px Marke (Priorität, Kürzel), 6 px Feld und Option, 8 px Knopf und Auswahlliste,
  10 px Fläche, rund für Status und Zähler.
- **Höhen:** Zeile der Liste 36 px kompakt, 52 px normal. Klickflächen mindestens 24 px,
  Knöpfe und Felder 36 px, im Portal 44 bis 52 px.
- **Schatten** nur am gewählten Segment (hell `0 1px 2px rgba(21,23,26,0.12)`, dunkel keiner)
  und an der aufgeklappten Auswahlliste (`0 6px 20px rgba(21,23,26,0.08)`).
- **Fokus:** 2 px `--ring` mit 2 px Abstand, immer sichtbar.

**Bausteine:**

- **Knöpfe**, 36 px hoch, Ecke 8 px, Gewicht 500:
  - **Primär:** Grund `--primary`, Text `--primary-foreground`, Kürzel rechts im Kasten
    `--kbd-on-primary`. Je Ansicht genau einer.
  - **Umrandet:** Grund `--card`, Rand `--input`.
  - **Ruhig:** ohne Grund und Rand.
  - **Gefahr:** Grund `--card`, Text und Rand `--destructive`.
  - **Gesperrt:** Grund `--muted`, Text `--muted-foreground`, Rand `--border`.
  - **Nur Symbol:** 36 × 36 px umrandet, immer mit `aria-label`.
- **Felder**, 36 px hoch, Ecke 6 px, Rand 1 px `--input`. Beschriftung 13 px, Gewicht 500,
  Hilfetext „Klein“ in `--muted-foreground`. Pflicht mit rotem Stern. Fehler mit 1,5 px Rand
  `--destructive`, Symbol und Text darunter (`aria-invalid`, `aria-describedby`). Nur lesbar:
  Grund `--background`, Rand `--border`. Ankreuzfeld und Optionsfeld 16 px, `accent-color` ist
  `--accent`.
- **Auswahlfeld mit Suche** (UI28): Feld mit 2 px `--accent` im Fokus. Die Liste hat Ecke 8 px,
  Rand `--input` und Schatten. Die gewählte Option ist `--accent-soft`, mit Avatar und Gruppe.
- **Status:** runde Pille mit Rand `--border`, 8-px-Punkt und Text. Neu ist ein leerer Ring in
  `--muted-foreground`, In Arbeit `--accent`, Wartet `--warning`, Gelöst `--success`.
- **Priorität** ohne Ampel: Marke 26 × 20 px, Ecke 4 px, Mono 11,5 und Gewicht 500.
  - P1: gefüllt `--primary`.
  - P2: Rand `--foreground`.
  - P3: Rand `--input`, Text `--foreground-2`.
  - P4: Rand `--border`, Text `--muted-foreground`, ohne Grund.
- **Frist:** Restzeit in Mono über einem 4-px-Balken auf `--clock-track`.
  - Läuft: Balken `--clock-running`.
  - Knapp ab 75 %: Text `--warning-foreground` auf `--warning-background`, Balken `--warning`.
  - Gerissen: Text `--destructive` auf `--destructive-background`, negative Zeit wie
    „−0 h 42 min“.
  - Angehalten: Text „angehalten“ in `--muted-foreground`, Balken `--clock-paused`.
  - Erfüllt: `--success`.
- **Hinweise:**
  - Konflikt beim Speichern (`role="alert"`): Rand 1,5 px `--border-strong`, Tabelle „Feld,
    Ihre Eingabe, Jetzt gespeichert“, Knöpfe „Gespeichertes übernehmen“ und „Meine Eingabe
    trotzdem speichern“.
  - Ergebnis einer Massenaktion: „48 Vorgänge geändert, 2 abgelehnt“ mit „Ergebnis zeigen“.
- **Navigation:** aktiver Eintrag `--surface-active`, Zähler in Mono. Dazu das Segment „Kompakt,
  Normal“, Filter als Chip („Gruppe ist 1st-Level“) und das Kürzel „Strg K“ am Suchfeld (UI31).
- **Verlauf:** Eine interne Notiz hat einen gestrichelten Rand und die Marke „Intern“, nie nur
  einen anderen Grund. Eine Änderung steht als eine Zeile („Status Neu → In Arbeit“, Uhrzeit in
  Mono).
- **Leiste der Mehrfachauswahl:** `--batch-background`, `--batch-foreground`, `--batch-border`.
  Auch dunkel bleibt sie eine dunkle Fläche.

**Ansichten auf der Fläche:**

- Vorgangsliste mit Vorschau, Detailseite eines Vorgangs und Portal, je hell und dunkel.
- Alle Beispiele sind erfunden.
- Bedienbar: Auswahl der Zeile, Haken mit Leiste, Umschalter für Dichte und Modus.

### 2.2 SLA-Management: was da ist und was nach Meilenstein 1 kommt

Gelesen am 2026-11-02 in den Plänen WF, SD und Hauptplan, nicht am Code nachgesehen. Der
Hauptplan nennt das Practice IP29 „Service level management“ mit Priorität 1 und ordnet es WF zu.

- **Da, als 🟢 und noch nicht abgenommen** (WF-Plan Block B): Kalender mit Arbeitszeiten und
  Feiertagen (WF8), Service-Level als Kerntyp `core.sla` für jeden Typ mit Uhren, Pause, Restzeit
  und Verstoß (WF9), Eskalationsstufen als Regeln mit Auslöser `clock` (WF10), Wartegründe (WF11),
  Zeitregeln (WF12). Im SD-Plan ⬜: Fristen je Priorität mit Servicezeit (SD3), Fristen der
  Anfragen (SD6), Suche „SLA-Risiko“ (SD7); die Werte sind vorläufig (SD-Plan E3, Hauptplan V30
  und V31).
- **Keine Pflegemaske.** SLAs gibt es nur über Liste und Formular von `core.sla`; Stufen und
  Wartegründe hängen über CEL-Bedingungen und Ids zusammen (WF-Plan E11, E12). Der
  Umsetzungsstand zu WF11 sagt selbst: „eine Oberfläche, die das zusammen anlegt, fehlt“. Das
  schließt UI32.
- **Kein Bericht zur Einhaltung.** IP29 nennt ihn, keine Aufgabe baut ihn. UI13 und UI14 zeigen
  Zahlen aus gespeicherten Suchen, UI23 feste Berichte; eine Quote über die Uhren braucht eigene
  Kennzahlen. Das schließt UI33.
- **OLA und Lieferantenverträge (UC)** sind technisch nur weitere SLAs, weil `core.sla` für jeden
  Typ gilt. Es fehlt aber ein Begriff, der SLA, OLA und UC mit Service, Kunde und Vertrag
  verbindet; Lieferantenverträge sind IP13 im Plan CM. Fachfrage, Hauptplan V39.
- **Welche SLA gilt,** entscheidet allein die Bedingung `applies` (WF-Plan E10). Eine SLA je
  Kunde, Service oder Vertrag geht damit, aber nur, wenn jemand die Bedingung schreibt; ein
  Regelwerk „SLA aus Service und Priorität“ wie in GLPI ist nicht vorgesehen.
- **Bekannte Grenzen** stehen im WF-Plan unter WF9, „Nicht geprüft“: Pausen vor dem späten Start
  einer SLA zählen mit; ein entfernter Feiertag wird erst zur alten Frist bemerkt; Bulk mit SLAs
  läuft Zeile für Zeile; kein Zwischenspeicher für Kalender.
- **Anzeige der Uhren** am Vorgang kommt mit UI30 (Detailseite aus `…/clocks`; die Fristspalte in
  der Liste nur, wenn die API die Restzeit je Zeile liefert).

## 3. Offene Fragen

| # | Frage | Hängt daran |
| --- | --- | --- |
Keine offen.

**Beantwortet am 2026-10-20:** V1 (E1), V2 (E2). **Am 2026-10-30:** V3 (E24), V4 (E25), V5 (E23), V6 (E27). **Am 2026-11-02:** V7 (E28).

Die Fragen an die Fachseite stehen im Hauptplan.

## 4. Umsetzungsstand

### UI34 — Fristspalte in der Liste, 2026-11-02

- **Grundlage:** E35, E37 dieses Plans; Hauptplan E156; deine Entscheidungen vom 2026-11-02
  (Systemspalten am Datensatz, die nächste fällige laufende Uhr, Filter überschritten, knapp,
  fällig bis).
- **Gebaut:**
  - Migration 41: jede Typtabelle und `list_entry` bekommen `sla_due_at`, `sla_warn_at` und
    `sla_paused`, die Typtabellen einen Index `(sla_due_at, id)`; vorhandene Datensätze werden aus
    den Uhren gefüllt (der 75-%-Punkt dabei als Frist minus ein Viertel des Ziels, genau rund um
    die Uhr, mit Kalender bis zum nächsten Schreiben oder Job der Uhr zu früh). Neue Typtabellen
    bekommen die Spalten von `schema.Apply`.
  - `internal/sla`: nach jedem Bewegen der Uhren (Schreiben des Datensatzes, Job der Frist) die
    nächste fällige laufende Uhr, ihr 75-%-Punkt im Kalender der Uhr (wie die Eskalationsstufen),
    sonst „angehalten“, wenn eine Uhr wartet; geschrieben über `records.SetDeadlineTx` in derselben
    Transaktion, ohne Version, Verlauf und Ereignis. Die Antwort des Schreibens zeigt die Werte.
  - Liste: `sort=sla_due_at` mit Blättern, Datensätze ohne Frist zuletzt (auch absteigend). Die
    JSON-Abfrage kennt den Wert `@now` für Systemzeiten, damit eine gespeicherte Suche
    „überschritten“ richtig bleibt. Typen tragen `timed: true`, wenn auf ihnen Uhren laufen.
  - Oberfläche: Spalte „Frist“ (`render/DeadlineCell.tsx`) mit dem Baustein Frist aus UI29, bei
    Typen mit SLA ohne Zutun sichtbar, sonst über „Spalten“; rot „Überfällig seit …“, gelb ab dem
    75-%-Punkt, sonst „Noch …“ (unter 24 h) oder „fällig …“, „Angehalten“ bei wartenden Uhren; der
    Balken schätzt den Anteil aus Frist und 75-%-Punkt. Filter „Frist ist überschritten“, „ist knapp
    (ab 75 %)“ und „fällig bis“ einem Tag; gespeicherte Suchen lesen sie zurück.
- **Belegt:**
  - Go: `TestDeadlineOverHTTP`: Frist und 75-%-Punkt in der Antwort, Version bleibt 1; Liste nach
    Frist, eine Zeile je Seite, auf- und absteigend, zwei Datensätze ohne Frist zuletzt nach id;
    Abfragen überschritten, knapp, fällig bis und gegen `@now`; der Job der Frist markiert den
    Verstoß, die Frist bleibt; Warten macht „angehalten“, Fortsetzen gibt die um die Pause
    verschobene Frist zurück; der Verlauf zeigt die Frist nicht; `/types` meldet `timed`.
  - Vitest (`render/DeadlineList.test.tsx`, 5 Fälle): Spalte mit „Überfällig seit 15 min“, „Noch …“,
    „Angehalten“, „–“, axe ohne Befund; keine Spalte ohne Uhren; Filter überschritten und knapp
    mit `@now`; Rücklesen aus einer gespeicherten Suche; Anteil aus dem 75-%-Punkt.
  - Playwright (`e2e/deadline.spec.ts`, Konto `load`): mit einem SLA an Probe steht die Spalte
    „Frist“ da, aufsteigend sortiert führt der neue Datensatz mit „Noch 59/60 min“, axe ohne
    Befund, „ist überschritten“ lässt ihn weg; das SLA geht am Ende wieder.
  - Ganze CI am 2026-11-02 (`scripts/ci-run.sh`, 9:24 min, dritter vollständiger Lauf): alle 19
    Schritte grün, 87 Vitest-Fälle, 16 Browser-Tests, LZ1 p95 5,04 ms, LZ13 7.783 Zeilen/s, LZ9
    143 ms. Der erste Lauf fand meine Versäumnisse: sqlc- und OpenAPI-Code nicht neu erzeugt,
    `deadline.spec.ts` nicht formatiert, zwei Schema-Tests zählten die Schritte einer neuen
    Tabelle (ein Index mehr: 14 statt 13, 8 statt 7 Zeilen, drei Spalten mehr; angepasst, weil
    der Index der Frist dazukommt) und der Browser-Test erwartete „Noch 59/60 min“, angezeigt
    wird „Noch 1 h 0 min“. Ein Lauf brach durch einen Neustart ab; im nächsten war nur der
    Selbsttest des Lastwerkzeugs rot (LZ13 617 Zeilen/s, Lauf drei Stunden statt neun Minuten,
    der Rechner war in der Zeit nicht frei); ohne Änderung danach grün.
- **Nicht geprüft:** Gelb und Rot im Browser-Test (die CI kann nicht 45 Minuten warten; belegt in
  Vitest und Go). LZ1 misst die Liste nach `updated_at`; eine Messung nach Frist gibt es noch nicht.
  Der Prüfweg-Punkt „LZ1 ohne Verschlechterung“ hängt an der normalen Liste.

### UI31 — Befehlszeile mit Strg K, 2026-11-02

- **Grundlage:** E22, E36 dieses Plans; deine Entscheidungen vom 2026-11-02 nach dem Mockup
  „Befehlszeile Strg K“ (Dialog in der Mitte, Nummer mit PL49, Befehle Neu/Gehe zu/Darstellung,
  leer die zuletzt geöffneten Datensätze im Browser).
- **Gebaut:**
  - `app/CommandPalette.tsx`: Dialog in der Mitte (640 px, Seite abgedunkelt), Eingabe als
    Combobox mit Liste in Gruppen „Zuletzt geöffnet“, „Datensätze“, „Befehle“, „Gehe zu“.
    Pfeiltasten wählen, Enter führt aus, Esc schließt und gibt den Fokus zurück, Tab bleibt im
    Dialog; ein Klick neben den Dialog schließt ihn.
  - Datensätze aus `/api/v1/search` über alle lesbaren Typen ab zwei Zeichen, 200 ms nach dem
    Tippen; die laufende Nummer findet sie, sobald PL49 sie durchsuchbar macht. Die letzte Zeile
    „Alle Treffer für … in der Suche“ führt zur Suchseite aus UI30.
  - Befehle: „Neu: <Typ>“ für jeden Typ der Navigation (wie der Knopf „Neu“ an der Liste; ob man
    anlegen darf, sagt wie bisher das Formular), „Darstellung: Hell/Dunkel/Wie das System“ und
    „Zeilenhöhe: Kompakt/Normal“, gespeichert wie in Meine Einstellungen. „Gehe zu“: Typen,
    gespeicherte Suchen, Meine und Globale Einstellungen, Rollen und Typen anpassen (nur mit dem
    Recht), Portal.
  - Leer: die fünf zuletzt geöffneten Datensätze, je Konto im Browser (`app/recent.ts`, nur Typ,
    ID und Titel); die Abmeldung im Arbeitsplatz und im Portal löscht sie.
  - Strg K (auf dem Mac auch Cmd K) öffnet die Befehlszeile überall, auch aus einem Feld heraus.
    Das Suchfeld im Kopf ist jetzt ein Knopf „Suchen oder Befehl …“ mit dem Kürzel „Strg K“ und
    öffnet denselben Dialog.
- **Belegt:**
  - Vitest (`app/CommandPalette.test.tsx`, 3 Fälle): Strg K, Fokus in der Eingabe, „berg“ →
    Person als erste Option, axe ohne Befund, Enter öffnet sie; „neu probe“ → „Neu: Probe“, Enter
    führt zum Formular, „dunkel“ → Darstellung dunkel und gespeichert; nach dem Öffnen eines
    Datensatzes steht er leer unter „Zuletzt geöffnet“, Pfeil runter wählt weiter, Esc schließt
    und der Fokus steht wieder auf dem Knopf im Kopf. `ListPreview.test.tsx`: der Fall „Suchfeld
    im Kopf“ geht jetzt über die Befehlszeile und „Alle Treffer …“ (angepasst, weil das Suchfeld
    seit E36 die Befehlszeile öffnet).
  - Playwright (`e2e/command.spec.ts`, Konto `load`, nur Tastatur): Strg K, ein Wort des Titels →
    der Datensatz ist gewählt, axe ohne Befund, Enter öffnet ihn; Strg K, „neu probe“, Enter →
    Formular; Strg K leer zeigt den Datensatz unter „Zuletzt geöffnet“, Esc schließt.
  - Ganze CI am 2026-11-02 (`scripts/ci-run.sh`, 9:11 min, zweiter Lauf): alle 19 Schritte
    grün, 82 Vitest-Fälle, 15 Browser-Tests, LZ1 p95 3,48 ms, LZ13 9.159 Zeilen/s, LZ9
    Seitenwechsel zur Liste 132 ms. Der erste Lauf direkt nach einem Neustart des Rechners war
    in zwei Schritten rot, beide ohne Bezug zu UI31: LZ9 mit 1.113 ms (Rechner noch unter Last,
    LZ1 einzelne Läufe p95 8 bis 9 ms) und ein Go-Test, in dem zwei Testpakete gleichzeitig die
    Tabellen von River auf der frischen Datenbank anlegten; die Sperre dafür kommt als Nachtrag
    im Kern-Plan (deine Entscheidung 2026-11-02).
- **Nicht geprüft:** Suche nach der Nummer (kommt mit PL49, dort im Prüfweg). „Neu“ steht wie an
  der Liste für jeden Typ, auch ohne Anlegerecht; das Formular sagt es dann. Pfeiltasten in der
  Liste selbst (Nachtrag aus UI30) sind nicht Teil von UI31 geworden.

### UI30 — Rahmen und Liste nach dem Entwurf, 2026-11-02

- **Grundlage:** E22, E35 dieses Plans; deine Entscheidungen vom 2026-11-02 (Vorschau rechts
  nach dem Mockup, Zeilenhöhe je Person, Fristspalte als UI34).
- **Gebaut:**
  - Kopf mit Suchfeld (Enter führt zu `/search?q=…`, Treffer der Volltextsuche über alle
    lesbaren Typen, `render/SearchPage.tsx`), Mandant, Sprache, Theme wie bisher.
  - Navigation nach Bereichen (die Typen je Paket unter einer Rubrik, Beschriftung `area.<paket>`)
    und darunter die gespeicherten Suchen, die man lesen darf (eigene und geteilte), als Link auf
    die Liste mit `?search=`; Verwaltung wie bisher; aktiver Eintrag auf `--surface-active`.
    Unter 768 px steht die Navigation über dem Inhalt.
  - Zeilenhöhe „Kompakt“ (36 px) oder „Normal“ (52 px) als Umschalter an der Liste und in Meine
    Einstellungen; gespeichert als `density` je Person (`internal/prefs`), Vorgabe am Baum
    `ui.density`, sonst „Normal“.
  - Vorschau `render/Preview.tsx` rechts neben der Liste (400 px): Titel, Status mit
    Übergängen, SLA-Uhren, die ersten sechs Felder (Priorität als Marke), „Öffnen“ und
    „Vorschau schließen“. Klick oder Enter auf eine Zeile zeigt sie ab 1100 px Breite, darunter
    öffnet der Klick die Detailseite; Doppelklick öffnet immer. Die gewählte Zeile steht auf
    `--accent-soft` mit `aria-selected`.
  - Fristen auf der Detailseite und in der Vorschau: knapp ab 75 % der Zeit (vorher ab 80 %),
    gelb mit Text, überschritten rot mit Text (Baustein Frist aus UI29).
- **Belegt:**
  - Go: `TestSettingsOverHTTP` (+ `density`: Vorgabe „normal“, eigener Wert, ungültiger 422).
  - Vitest (`render/ListPreview.test.tsx`, 4 Fälle): Zeilenhöhe 52 → „Kompakt“ → 36 px und
    `density: compact` gespeichert; Klick zeigt die Vorschau mit Titel, Status und Feldern, die
    Zeile ist gewählt, axe ohne Befund, eine andere Zeile wechselt die Vorschau, „Öffnen“ führt
    zur Detailseite; „Vorschau schließen“, unter 1100 px öffnet der Klick direkt; Suchfeld im
    Kopf findet eine Person. `RecordPage.test.tsx`: Schwelle 75 % (50 von 240 Minuten übrig
    ist knapp, 70 nicht).
  - Playwright (`e2e/frame.spec.ts`, Konto `gestalter`): Zeile 52 px, „Kompakt“ 36 px, nach
    Neuladen 36 px; Klick zeigt die Vorschau mit P2, axe ohne Befund, „Öffnen“ führt zur
    Detailseite; bei 375 px steht die Navigation über der Liste in voller Breite.
  - Ganze CI am 2026-11-02 (`scripts/ci-run.sh`, 8:56 min, dritter Lauf): alle 19 Schritte
    grün, 79 Vitest-Fälle, 14 Browser-Tests, LZ1 p95 3,24 ms, LZ13 8.788 Zeilen/s. Der erste
    Lauf fand drei Browser-Tests, die die Überschrift „Probe“ suchten und jetzt auch den
    Bereich „Probepaket“ der Navigation trafen; sie suchen die Überschrift jetzt exakt. Im
    zweiten Lauf stand im Test der Zeitleiste (UI3, UI7) einmal ein frischer Eintrag nicht
    oben; im dritten Lauf ohne Änderung grün, Ursache nicht gefunden (Zeitleiste und
    Eintragsfeld hat UI30 nicht berührt).
- **Nicht geprüft:** Gelb und Rot der Frist im Browser-Test (die CI hat kein SLA an `probe`;
  belegt in Vitest an `light()` und in UI29 am Baustein). Zähler an den Einträgen der
  Navigation aus dem Entwurf fehlen, die API zählt nicht. Pfeiltasten in der Liste bewegen den
  Fokus nicht von Zeile zu Zeile (Tab geht), das kommt mit der Befehlszeile UI31 oder als
  Nachtrag.

### UI29 — Design-Kit „Klartext“ im Werkzeug, 2026-11-02

- **Grundlage:** E22 bis E25, E28, E34 dieses Plans und §2.1; deine Entscheidungen vom
  2026-11-02 zu Schrift und Darstellung.
- **Gebaut:**
  - `web/src/ui/tokens.css` mit allen Werten aus §2.1, hell unter `:root`, dunkel unter
    `data-color-scheme="dark"` an `<html>` (Tailwind-Variante `dark` folgt dem Attribut);
    neue Tokens `--foreground-2`, `--accent`, `--accent-soft`, `--warning-*`,
    `--destructive-background`, `--surface-*`, `--clock-*`, `--batch-*` und weitere; Ecken 6, 8
    und 10 px; Fokusrahmen 2 px `--ring` (Petrol) mit 2 px Abstand für alles; Kästchen und
    Optionsfelder in `--accent`. Theme `probe`: Akzent und Hauptaktion #3F4BB8, dunkel nach der
    Formel aus §2.1 (#9097D6).
  - IBM Plex Sans (400, 500, 600) und Mono (400, 500) aus `@fontsource` 5.3.0, von Vite
    gebündelt; kein Abruf außerhalb des Werkzeugs.
  - Knöpfe nach dem Kit (36 px, Primär graphit, Umrandet, Ruhig, Gefahr mit rotem Rand auf
    `--card`, Gesperrt auf `--muted`), Felder 36 px mit Rand `--input`, Fehler 1,5 px rot,
    Hinweise auf `--warning-background`.
  - Bausteine `web/src/ui/marks.tsx`: `StatePill` (Punkt je Statusart: Ring, Petrol, Gelb,
    Grün), `PriorityMark` (P1 gefüllt, P2 Rand, darunter ruhiger), `Deadline` (Restzeit in Mono
    über 4-px-Balken). Eingesetzt in der Liste (Status, Felder mit `format: priority`), im Kopf
    der Detailseite (Status) und bei den SLA-Uhren der Seitenleiste.
  - Kern: `State.Category` und `Field.Format = priority` mit Prüfung in `meta.Validate`;
    `probe` nutzt beides. Einstellung `color_scheme` in `internal/prefs` mit Vorgabe am Baum
    `ui.color_scheme`; `web/src/ui/colorScheme.ts` wendet sie an und folgt bei „wie das System“
    `prefers-color-scheme`, auch während die Seite offen ist. Auswahl „Darstellung“ in Meine
    Einstellungen und in den globalen Einstellungen.
  - Lizenzliste: zwei Ausnahmen für die Fontsource-Pakete mit OFL-1.1.
- **Belegt:**
  - Go: `TestSettingsOverHTTP` (Vorgabe „system“, Wert vom Baum, eigener Wert, ungültiger 422),
    `TestValidateNamesFieldAndReason` (`format: priority` nur an Zahlen, Statusart aus der
    Liste), `TestLoadProbePackageFromRepository`.
  - Vitest: `ui/marks.test.tsx` (Bausteine mit Text, Breite des Balkens, axe; hell, dunkel,
    „wie das System“ folgt dem Gerät, „Hell“ hört auf zu folgen), `SettingsPage.test.tsx`
    (+1: „Dunkel“ gespeichert, Seite sofort dunkel, nach Neustart wieder dunkel).
  - Playwright (`e2e/design.spec.ts`, Konto `gestalter`): Schrift IBM Plex geladen, Hauptknopf
    `rgb(21, 23, 26)`, Fokusrahmen `rgb(11, 110, 128)`, P1 als Marke; Theme `probe` färbt den
    Knopf `rgb(63, 75, 184)`; axe mit Kontrast auf Liste, Detailseite, Workflow, Rollen und
    Einstellungen hell; „Dunkel“ gespeichert → Grund `rgb(17, 19, 21)`, nach Abmelden und
    Anmelden dunkel, dieselben Seiten mit axe dunkel; kein Abruf außerhalb des Werkzeugs.
  - Ganze CI am 2026-11-02 (`scripts/ci-run.sh`, 9:20 min, dritter Lauf): alle 19 Schritte
    grün, 75 Vitest-Fälle, 13 Browser-Tests, LZ1 p95 4,05 ms, LZ13 8.057 Zeilen/s. Der erste
    Lauf fand mit Kontrast den grauen Text der Navigation auf dem grauen Grund der Seitenleiste
    (4,43 : 1, die Regel aus §2.1); die Überschriften nehmen jetzt `--foreground-2`, die
    Seitenleiste `--surface-subtle`. Dazu ein Unit-Test der Lizenzprüfung, der die echte Liste
    liest (die Schriftpakete fehlten in seinem Beispiel), und zwei Testfehler im neuen
    Browser-Test. Der dunkle Modus bestand axe auf allen fünf Seiten im ersten Anlauf.
  - Die Instanz auf 8092 hat das neue Paket `probe` (Statusart, `format: priority`).
- **Nicht geprüft:** Das Portal und die Anmeldeseite prüft axe nur hell. Der Graph im
  Workflow-Editor nimmt die Tokens, aber React Flow selbst hat keinen Test im dunklen Modus
  außer axe. Rahmen, Navigation und Liste nach dem Entwurf folgen mit UI30, die Befehlszeile
  mit UI31.

### UI27 — Rollen-Editor, 2026-11-02

- **Grundlage:** E33 dieses Plans; Kern-Plan PL8, PL9 (Rollen, Reichweiten, Bedingungen,
  Feldrechte), PL27 (Sicherheitsprotokoll); deine Entscheidungen vom 2026-11-02 nach dem Mockup.
- **Gebaut:**
  - Kern: nur `me.manage_rights` (Recht `manage_rights` an `core.role`), damit die Oberfläche
    den Editor anbietet; Rollen, Rechte und Zuweisungen hatte die API schon (PL8).
  - Oberfläche `render/RolePage.tsx`: Navigation „Verwaltung › Rollen und Rechte“, Seite
    `/roles` mit allen Rollen (Rechte und Zuweisungen gezählt), „Neue Rolle“ und Entfernen mit
    Rückfrage (nicht `admin`); Seite `/roles/<name>` mit der Matrix aller Typen (Kästchen „Nur
    Typen mit Rechten“, eine Zeile „Alle Typen“, wenn die Rolle `*` hat). Eine Reichweite in der
    Zelle wählen setzt die Zeile, eine andere wählen setzt die neue und nimmt die alte weg
    (Bedingung und Feldrechte wandern mit), „–“ nimmt alle Zeilen der Zelle weg. Hat eine Zelle
    mehrere Reichweiten, zeigt sie „Gruppe + beteiligt“ und wird in den Details bearbeitet.
    „…“ öffnet die Details: je Reichweite Bedingung (CEL, der Server prüft beim Speichern, ob
    sie sich in SQL übersetzen lässt), „Alle Felder“ oder die Felder zum Ankreuzen, Speichern
    und Entfernen; „Weitere Reichweite“. Sonderrechte als Kästchen: Arbeitsplatz, Typen und
    Workflows anpassen, Rollen und Rechte, Organisationsbaum, Einstellungen, Regeln, Webhooks,
    Hintergrundaufträge, Sicherheitsprotokoll, Auswertungen über Mandanten. „Weitere Rechte“
    listet den Rest mit Entfernen und legt ein Recht mit freier Aktion an. „Zuweisungen“: Person
    oder Gruppe per Auswahlfeld mit Suche (UI28) und Knoten aus dem Organisationsbaum.
- **Belegt:**
  - Go: `TestCustomerLayerOverHTTP` prüft `manage_rights` in `auth/me` (ohne Recht `false`, als
    Admin `true`).
  - Vitest (`render/RolePage.test.tsx`, 4 Fälle): Rolle über die Navigation anlegen, Seite
    öffnet sich, axe ohne Befund; Probe lesen von „alle“ auf „eigene“ (erst PUT neu, dann DELETE
    alt), ändern auf „Gruppe“, lesen auf „–“, „Nur Typen mit Rechten“; Details: ungültige
    Bedingung mit Grund abgelehnt, gültige mit Feldauswahl gespeichert (`fields.read` ohne
    „Fällig am“), weitere Reichweite „beteiligt“ sperrt die Auswahl der Zelle und zeigt
    „eingeschränkt“; Sonderrecht erteilt und entzogen; Person per Suche an „IT“ zugewiesen, mit
    Namen in der Liste, wieder entfernt. Jeder Aufruf genau wie erwartet.
  - Playwright (`e2e/roles.spec.ts`): Admin und Konto `pruefer` (Rolle `e2e_rollen`, angelegt
    von `scripts/ci.sh`) in zwei Browsern; `pruefer` sieht Datensätze von Probe; Admin stellt
    im Editor Probe lesen auf „eigene“ → `pruefer` sieht nach Neuladen 0; zurück auf „alle“ →
    wieder dieselbe Zahl; axe ohne Befund.
  - Ganze CI am 2026-11-02 (`scripts/ci-run.sh`, 8:32 min, erster Lauf): alle 19 Schritte
    grün, 72 Vitest-Fälle, 12 Browser-Tests, LZ1 p95 2,91 ms, LZ13 10.002 Zeilen/s.
- **Nicht geprüft:** Beschriftung einer Rolle ändern (die API kennt nur Anlegen und Entfernen).
  Die Bedingung wird nicht beim Tippen geprüft wie im Workflow-Editor, sondern beim Speichern
  (die Rechte-Bedingung muss sich in SQL übersetzen lassen, das prüft `authz.Translate`). Zwei
  Admins in derselben Zelle: der letzte gewinnt, es gibt keine Version an Rechten. Die Matrix
  zeigt alle Typen; bei vielen Paketen wird sie lang (Kästchen „Nur Typen mit Rechten“).

### UI26.2 — Workflow-Editor als Graph, 2026-11-02

- **Grundlage:** E26, E27, E31 dieses Plans; Kern-Plan PL43 (Kundenschicht, Nachtrag
  2026-11-02 zu `set_layout`); deine Entscheidungen vom 2026-11-02 nach dem Mockup.
- **Gebaut:**
  - Kern: `Type.Layout` (`map[string]Point`, nur JSON, nicht im YAML des Pakets) und die
    Operation `set_layout` der Kundenschicht (Feld `layout`): ersetzt die ganze Anordnung, jeder
    Name muss ein Status sein, ein entfernter Status verlässt die Anordnung, ein leeres
    `layout` nimmt sie weg; `validateStates` meldet veraltete Namen. Die Engine liest das Feld
    nicht.
  - Oberfläche `render/WorkflowGraph.tsx` über den Tabellen der Workflow-Seite: React Flow
    12.12 (MIT) mit eigenem Knoten je Status (Beschriftung, technischer Name, „Anfang“;
    kräftiger Rahmen Anfangsstatus, gestrichelt eigener Status), je Ausgangsstatus eine Kante
    mit Pfeil und Beschriftung (gestrichelt und grau: stillgelegt). Kante oder Zeile
    anklicken markiert den Übergang in Graph (alle Kanten) und Tabelle (`aria-selected`, „im
    Graphen markiert“). Eine Verbindung von Knoten zu Knoten ziehen füllt im Formular „Neuer
    Übergang“ Von und Nach vor und setzt den Fokus auf die Beschriftung, mit Hinweis „Aus dem
    Graphen gezogen“. Knoten bewegen mit Maus oder mit Enter und Pfeiltasten (React Flow);
    „Anordnen“ ordnet mit dagre 3.1 (MIT) von oben nach unten; beides setzt „Anordnung
    verändert, noch nicht gespeichert“ und warnt beim Verlassen der Seite; „Anordnung
    speichern“ ist eine Änderung `set_layout` mit der Position jedes Status, in der Liste der
    Änderungen „Anordnung des Graphen gespeichert“. Ohne gespeicherte Anordnung ordnet dagre
    beim Öffnen, ein Status ohne Position bekommt seine Stelle aus dagre. „Graph einklappen“
    merkt sich der Browser je Konto (`localStorage`). Farben aus den Tokens der Oberfläche,
    Beschriftungen und Hilfetexte von React Flow auf Deutsch (`ariaLabelConfig`).
- **Belegt:**
  - Go: `TestApplyLayout` (meta: Anordnung gesetzt, unbekannter Status abgelehnt, entfernter
    Status verlässt die Anordnung, `nil` abgelehnt, leer nimmt sie weg, `ValidateCustomer`
    findet einen veralteten Namen), `TestCustomerLayerInsideItsPackage` (`set_layout` über
    HTTP, in `GET /api/v1/types/probe.probe` sichtbar, unbekannter Status 422).
  - Vitest (`render/WorkflowPage.test.tsx`, +2 Fälle, React Flow mit den Mess-Ersatzstücken
    der Bibliothek für jsdom): Knoten je Status, Klick auf die Kante „Erledigen“ markiert die
    Zeile und nur diese, Klick auf die Zeile „Wieder öffnen“ wechselt die Markierung;
    „Anordnung speichern“ gesperrt bis „Anordnen“, dann eine Änderung `set_layout` mit genau
    den drei Status, Hinweis verschwindet, Eintrag in den Änderungen, axe ohne Befund. Graph
    einklappen entfernt ihn samt Knöpfen, `localStorage` je Konto, nach Neuaufbau zu,
    aufklappen zeigt ihn wieder.
  - Playwright (`e2e/workflow.spec.ts`, erweitert): alle Status von Probe im Graphen, Klick
    auf eine Kante des neuen Übergangs markiert beide Kanten und die Zeile; neuer Status per
    Enter und fünfmal Pfeil rechts bewegt, gespeichert, nach Neuladen an derselben Stelle
    (gleicher `transform`); Verbindung vom neuen Status nach „Neu“ gezogen → Formular
    vorbelegt und fokussiert, Beschriftung getippt, angelegt, Zeile da; „Anordnen“ setzt den
    Hinweis; der Export hält genau `add_state`, `add_transition`, `set_layout`,
    `add_transition`; der Datensatz nimmt den gezogenen Übergang zurück nach „Neu“; alles
    zurückgenommen, auch die Anordnung. Die Kante wählt der Test per Tastatur (Fokus, Enter):
    ein Mausklick auf die Mitte des Rechtecks um eine gebogene Kante trifft die Linie nicht.
  - Ganze CI am 2026-11-02 (`scripts/ci-run.sh`, 8:28 min): alle 19 Schritte grün, 68
    Vitest-Fälle, 11 Browser-Tests, LZ1 p95 2,93 ms, LZ13 9.923 Zeilen/s. Davor sechs rote
    Läufe, alle an Tests oder Darstellung, keiner am Kern: Ersatz-`ResizeObserver` riss
    fremde Vitest-Fälle mit; der Herstellerlink von React Flow hatte 2,8 : 1 Kontrast (jetzt
    ausgeblendet, `proOptions.hideAttribution`, nach MIT erlaubt, und kein Link nach außen
    aus einem VS-NfD-System); der eigene Knoten braucht sein Aussehen selbst; Zeilen im
    Browser-Test nur in der Tabelle „Übergänge“ suchen; Position ohne z-index vergleichen;
    ein doppelter Variablenname. Einmal war `TestSecurityEventLog` rot, ohne Bezug zu UI26.2:
    der Test sucht mit „seit vor einer Sekunde“, ein Rücksprung der Uhr in WSL (Migration 40)
    wirft seine Einträge aus dem Filter; im nächsten Lauf grün. Bleibt beobachtet.
- **Nicht geprüft:** Dunkler Modus des Graphen (Tokens kommen mit UI29). Sehr große
  Statusmodelle (dagre ordnet, aber die Fläche ist 384 px hoch; Zoom und Einpassen gibt es).
  Die Pfeiltasten bewegen in Schritten von React Flow, keine Rasterung. Die Lizenzen der neuen
  Pakete prüft der CI-Schritt `licenses` (React Flow, `@xyflow/system`, zustand, classcat MIT;
  d3-Teile ISC; dagre und graphlib MIT).

### UI26.1 — Workflow-Editor als Tabelle, 2026-11-02

- **Grundlage:** E26, E30 dieses Plans; Kern-Plan PL43 (Kundenschicht, Nachtrag 2026-11-02);
  deine Entscheidungen vom 2026-11-02 (Umfang wie bei den Feldern, eigene Seite, Live-Prüfung
  der Bedingung).
- **Gebaut:**
  - Kern: die Kundenschicht kennt Status und Übergänge (acht Operationen, siehe Kern-Plan PL43,
    Nachtrag); `Transition.Disabled`; `POST /api/v1/types/{typ}:check-condition` sagt, ob eine
    CEL-Bedingung kompiliert (Recht `customize`).
  - Oberfläche `render/WorkflowPage.tsx` auf `/types/<typ>/workflow`, Link „Workflow“ an der
    Liste und im Typ-Editor. Tabelle der Status (Beschriftung, technischer Name mit Herkunft,
    Anfangsstatus als Auswahlknopf, Bearbeiten mit Beschriftung deutsch und englisch, Entfernen
    für eigene) und Formular „Neuer Status“; Tabelle der Übergänge (Beschriftung, Von, Nach,
    Bedingung; Bearbeiten mit Ausgangsstatus als Kästchen, Zielstatus, Bedingung; Stilllegen
    und Aktivieren; Entfernen für eigene) und Formular „Neuer Übergang“. Die Bedingung wird 300 ms
    nach der letzten Eingabe geprüft; eine ungültige steht als Fehler am Feld und sperrt
    „Anlegen“ und „Speichern“. Hinweise: Status, zu dem kein Übergang führt; stillgelegte
    Übergänge; Typ ohne Statusmodell. Jede Zeile ist eine Änderung mit der gesehenen Version,
    Konflikt und Ablehnung wie im Typ-Editor; darunter die Änderungen des Workflows mit
    „Zurücknehmen“ und der Export. Der Typ-Editor und der Workflow-Editor teilen sich
    `useCustomizer`.
- **Belegt:**
  - Go: `TestApplyWorkflow`, `TestCustomerLayerInsideItsPackage` (Kern-Plan PL43, Nachtrag).
  - Vitest (`render/WorkflowPage.test.tsx`, 3 Fälle): Status „Prüfung“ per Tastatur mit Enter
    anlegen → Hinweis „Kein Übergang führt nach „Prüfung““; Übergang von Neu und In Arbeit mit
    Bedingung: `record.priority <` → „Bedingung ungültig“ und Knopf gesperrt, `= 2` dazu → gültig,
    anlegen → zwei Änderungen genau wie erwartet, Hinweis verschwindet, axe ohne Befund.
    „Erledigen“ stilllegen → Hinweis und Knopf „aktivieren“, kein „entfernen“ an einem Übergang
    des Pakets; Anfangsstatus auf „In Arbeit“; Zurücknehmen stellt „Erledigen“ wieder her.
    „Erledigt“ in „Fertig“ umbenennen und „Beginnen“ zusätzlich von „Fertig“ aus erlauben → je
    eine Änderung nur mit dem, was sich geändert hat.
  - Playwright (`e2e/workflow.spec.ts`): Status und Übergang mit zwei Ausgangsstatus und
    Bedingung nur per Tastatur, ungültige Bedingung am Feld, Hinweis bis der Übergang da ist,
    axe ohne Befund; ein neuer Datensatz mit Priorität 2 nimmt den Übergang über die API
    sofort; der Export enthält genau die zwei Änderungen; am Ende alles zurückgenommen.
  - Ganze CI am 2026-11-02 (zweiter Lauf, `scripts/ci-run.sh`, 8:37 min): alle 19 Schritte
    grün, 66 Vitest-Fälle, 11 Browser-Tests, LZ1 p95 3,14 ms, LZ13 8.930 Zeilen/s. Der erste
    Lauf war nur an `gofmt` in `internal/meta/customer_test.go` rot (17 von 19 grün).
- **Nicht geprüft:** Die Vitest-Prüfung der Bedingung ist nachgebaut (ein Operator am Ende ist
  ungültig); die echte Prüfung belegt der Browser-Test. Ein Status lässt sich nur dann entfernen,
  wenn er in keiner Datenbank Datensätze hat; der Editor zeigt die Zahl nicht. Übergänge des
  Pakets lassen sich umleiten; ob Regeln, SLA und Feldzustände des Pakets dann noch passen,
  prüft niemand (Hinweis in der Einleitung der Seite fehlt noch). Der Graph kommt mit UI26.2.

### UI25 — Typ- und Feld-Editor, 2026-11-02

- **Grundlage:** Hauptplan E37, E135; Kern-Plan PL43 (Kundenschicht); E29 dieses Plans; deine
  Entscheidungen vom 2026-11-02 (Knopf an der Liste, jede Aktion sofort, alle sieben Feldtypen,
  Liste der Änderungen mit Zurücknehmen und Export).
- **Gebaut:**
  - Server: `auth/me` meldet `customize` (Recht `customize` an `core.type` an der Wurzel); jede
    Änderung der Kundenschicht trägt in der Antwort `by_name`, den Namen des Kontos
    (`internal/customer`, eine Abfrage für alle Namen).
  - Oberfläche `render/CustomizePage.tsx`: Knopf „Typ anpassen“ an der Liste und „Verwaltung ›
    Typen anpassen“ in der Navigation, beides nur mit dem Recht. Die Seite
    `/types/<typ>/customize` zeigt die Felder in ihrer Reihenfolge mit Herkunft (eigenes Feld oder
    aus dem Paket), Pflicht, ausgeblendet, berechnet, Titel; verschieben per Ziehen (dnd-kit, auch
    mit der Tastatur) und mit ↑ ↓, ein- und ausblenden, bearbeiten (Beschriftung und
    Beschreibung deutsch und englisch, Pflicht, Vorgabe, bei eigenen Textfeldern die Höchstlänge,
    bei Auswahlfeldern weitere Werte), eigene Felder entfernen. „Neues Feld“ legt ein Feld `k_…`
    an: Text, formatierter Text, Zahl, Ja/Nein, Datum, Zeitpunkt, Auswahl mit Werten je Zeile,
    Verweis mit Zieltyp; der technische Name folgt der Beschriftung, bis man ihn selbst ändert.
    Jede Aktion ist eine Änderung mit der gesehenen Version; 409 meldet „Jemand anders hat den
    Typ gerade geändert“ und lädt neu, 422 nennt den Grund des Servers. Darunter die Änderungen,
    neueste zuerst, mit Wer, Wann, Status (wirkt, stillgelegt, Konflikt mit Grund) und
    „Zurücknehmen“; „Kundenschicht exportieren“ lädt `GET /api/v1/customer` als
    `kundenschicht.json`. Die Übersicht `/customize` zählt die Änderungen je Typ.
  - Nach jeder Änderung lädt die Oberfläche die Typen neu: Liste und Formular zeigen den neuen
    Stand ohne Neuladen der Seite.
  - Befund aus dem Browser-Test, behoben im Kern (Kern-Plan PL43, Nachtrag): Ein Feld an Probe
    wurde abgelehnt, weil die Kundenschicht den Typ ohne die übrigen Typen und Merkmale seines
    Pakets prüfte (Merkmal `tagged`, Summen aus `probe.task`). Jetzt prüft
    `meta.ValidateCustomer` den Typ in seinem Paket, wie bei der Installation.
- **Belegt:**
  - Über HTTP (`TestCustomerLayerOverHTTP`, ergänzt): `auth/me` meldet `customize` false ohne und
    true mit dem Recht; die erste Änderung trägt den Namen des Admins.
    `TestCustomerLayerInsideItsPackage` (neu): ein Feld an `probe.probe` (mit Merkmal und Summen
    des Pakets) wird angenommen und wieder zurückgenommen.
  - Nachtrag aus deiner Prüfung (2026-11-02): Nach „Zurücknehmen“ eines eigenen Felds lehnte die
    API die Liste mit 422 ab (`"k_dienststelle" is not a field of probe.probe`), weil die Liste
    ihre Spaltenwahl je Typ im Browser merkt (`localStorage`) und das entfernte Feld weiter
    anforderte. `render/List.tsx` nimmt aus der gemerkten Wahl jetzt nur Spalten, die der Typ
    noch hat; Vitest `List.test.tsx` „drops a remembered column the type no longer has“.
    Ganze CI danach grün am 2026-11-02 im ersten Lauf (8 min 22 s; 63 Vitest-Fälle, 10
    Playwright-Fälle mit axe, Leistungsgate bestanden; LZ1 p95 2,93 ms, LZ4 8,09 ms).
  - Vitest (`render/CustomizePage.test.tsx`, 6 Fälle): ohne Recht kein Knopf und kein Eintrag in
    der Navigation; „Dienststelle“ anlegen → genau eine Änderung `add_field` mit Version 0, das Feld
    steht im Editor, die Änderung mit „Pascal“, die Spalte in der Liste und das Feld im Formular;
    ausblenden, mit ↑ verschieben und das Ausblenden zurücknehmen, je eine Änderung mit der
    richtigen Version; Werte ergänzen und Beschriftung ändern an einem Feld des Pakets; eine
    Änderung dazwischen meldet den Konflikt, lädt neu und der zweite Versuch gelingt; die
    Ablehnung des Servers erscheint mit Grund; die Übersicht zählt; axe ohne Befund.
  - Playwright (`e2e/customize.spec.ts`): Feld per Tastatur anlegen, axe ohne Befund, der Export
    enthält genau diese Änderung mit dem Namen des Kontos, Spalte in der Liste und Feld im
    Formular ohne Neuladen, Zurücknehmen entfernt es wieder.
  - Ganze CI grün am 2026-11-02 im fünften Lauf, dem ersten mit eigenen Servern je Lauf (Kern-Plan PL51, Hauptplan E150): Go und Web (62 Vitest-Fälle, 10 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,19 ms, LZ2 5,26 ms, LZ3 6,99 ms, LZ4 10,34 ms, LZ5 11.120 Zeilen/s, LZ13 10.100 Zeilen/s. Davor viermal rot: einmal Lint (gosec G602, Fehlalarm an einer Schleife über die Änderungen; umgeschrieben), zweimal der Browser-Test des Editors selbst (der Befund an `probe.probe`, oben; dann „Neu“ als Link statt Knopf gesucht, und Reste früherer Läufe; der Test räumt jetzt vorher auf), einmal `TestConcurrentWritersAndHorizon` in `internal/events` ohne Bezug zu UI25 (Ereignis-Grenze unter Last anderer Pakete, Anlass für PL51).
- **Nicht geprüft:** Das Ziehen mit der Maus ist nur über die Tastatur getestet (wie im
  Formular-Builder). Ein zurückgenommenes eigenes Feld behält Spalte und Werte in der Datenbank
  (PL43); der Editor sagt das nicht eigens. Die Vorgabe lässt sich setzen, aber nicht wieder
  entfernen, weil die Kundenschicht dafür keine Operation hat. Felder von Merkmalen und berechnete
  Felder bietet der Editor wie alle an; was der Server ablehnt, erscheint mit Grund.

### UI1 — Rahmen der Anwendung, 2026-10-20

- **Grundlage:** Hauptplan E41, E42, E62, E74, E77, E78; PL5; E3 bis E6 dieses Plans.
- **Gebaut:** `web/src/router.tsx` (TanStack Router: `/login`, Rahmen mit `/`, `/types/$typeName`,
  `/password`), `lib/api.ts` (Aufrufe für Sitzung, Typen, Version mit CSRF-Header und
  Problem-Fehlern; TanStack Query für Sitzung und Typen), `lib/theme.ts` und `styles/tokens.css`
  (Tailwind 4 mit Tokens, Themes `neutral` und `probe`), Bausteine Button, Input, Label, Card,
  Alert, Umschalter für Sprache und Theme (native `select`), Seiten Anmelden (react-hook-form +
  zod, Fehler je Feld mit `aria-describedby`), Rahmen (Kopf mit Konto und Abmelden, Navigation
  mit den installierten Typen aus `/api/v1/types`), Start, Typ-Platzhalter (Liste kommt mit UI2),
  Kennwort ändern (E77). Texte deutsch und englisch in `i18n/*.json`. Tests: Vitest mit jsdom,
  Testing Library und `axe-core`; Playwright mit `@axe-core/playwright` gegen den laufenden
  Server (`scripts/ci.sh` startet das Binary mit `web/dist` auf Port 18082). Das Werkzeug-Image
  trägt Chromium für Playwright 1.64.1.
- **Belegt, 2026-10-20 im Werkzeug-Container:**
  - Vitest, 11 Tests grün: ohne Sitzung landet `/` auf `/login` und die Seite ist ohne
    axe-Befund; leere Felder werden benannt (`aria-invalid`, Text „Bitte ausfüllen."), falsches
    Kennwort zeigt den Hinweis als `role="alert"`; richtiges Kennwort führt zum Rahmen mit
    „Willkommen, Pascal", Link „Probe" auf `/types/probe.probe`, Platzhalter mit Feldzahl,
    Abmelden zurück auf `/login`, alles ohne axe-Befund. Themes: Attribut und Speicher; jedes
    Theme hat seinen Block in `tokens.css`; kein Baustein nutzt feste Farbklassen.
  - ESLint (mit jsx-a11y) und Prettier ohne Befund, `tsc` und Vite-Build grün.
  - Playwright im Werkzeug-Container gegen das Binary mit `web/dist`: Anmeldeseite ohne axe-Befund (WCAG 2 A/AA, mit Kontrast), Theme `probe` ändert die Hintergrundfarbe des Anmelde-Buttons, Anmelden als `load` führt zur Navigation „Typen
  - `scripts/ci.sh` komplett: Lizenz bis Browser-Test, Gate bestanden, 11 Vitest- und 1 Playwright-Test.
- **Nicht geprüft:** Das Bundle ist 538 kB (gzip 170 kB), Vite warnt; Code-Splitting je Route
  kommt mit UI2, wenn die Listen dazukommen. Kein Dunkelmodus, keine Schriftvariablen über
  `--font-sans` hinaus. Die Anmeldeseite merkt sich nicht, wohin der Nutzer wollte; nach dem
  Anmelden landet er auf der Startseite. Kontrast prüft nur Playwright, nicht Vitest.

### UI12 — Bedingungen und Übersetzung von Formularen, 2026-10-29

- **Grundlage:** Hauptplan E134, Inventar G132, G136; UI11 (Formulare); E21 dieses Plans;
  deine Entscheidungen vom 2026-10-29 (Regeln je Frage mit UND; Oberfläche und Server;
  Umschalter im Baukasten; Eintrag immer Deutsch).
- **Gebaut:**
  - Aufbau: Abschnitte und Fragen tragen `show_if` mit Regeln `{question, op, value}`,
    `op` ist `eq`, `ne`, `answered` oder `empty`; dazu `title` und `intro` für Name und
    Beschreibung in anderen Sprachen.
  - Prüfung beim Speichern (`internal/forms`): eine Regel nennt nur eine Frage davor, `eq` und
    `ne` brauchen einen Wert, `answered` und `empty` keinen; bei Ja/Nein und Auswahl muss der
    Wert eine der Antworten sein; eine Frage auf ein Pflichtfeld des Zieltyps darf nicht
    ausblendbar sein, auch nicht über ihren Abschnitt.
  - Absenden: der Server wertet die Regeln in der Reihenfolge des Formulars aus; ausgeblendete
    Antworten werden verworfen, „Pflicht“ gilt nur für Sichtbares. Der Eintrag ist deutsch,
    `freeAnswer` nimmt keine Sprache mehr.
  - Oberfläche: `Form` nimmt `visibility(values)`; Ausgeblendetes wird nicht gezeigt, nicht
    geprüft (der Resolver rechnet aus den geprüften Werten) und nicht gesendet.
    `portal/conditions.ts` wertet wie der Server aus. Im Baukasten je Abschnitt und Frage
    „Bedingung hinzufügen“ mit Bezugsfrage, Bedingung und Wert (Auswahl bei Ja/Nein und
    Antworten); der Umschalter „Sprache der Texte“ stellt alle Eingaben auf Englisch, mit dem
    deutschen Text als Platzhalter, dazu Name und Beschreibung auf Englisch und je Antwort eine
    englische Fassung. Das Portal zeigt die Texte in der Sprache der Person, fehlendes
    Englisch auf Deutsch.
  - CI: Konto `englisch` mit der Rolle `e2e_portal` für den englischen Browsertest.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Einzeln (`internal/forms`): jede Ablehnung der Regeln, auch die ausblendbare Pflichtfrage
    und ein Wert, den ein gebundenes Auswahlfeld nicht hat; `holds` für alle Operatoren, auch
    fehlende Antworten; freie Antworten deutsch.
  - Über HTTP (`TestFormConditionsOverHTTP`): eine Regel auf eine spätere Frage wird beim
    Speichern abgelehnt; die englischen Texte kommen mit dem Formular. Bei „nein“ verwirft der
    Server B und den Abschnitt „Lieferung“ samt Antworten, B ist nicht Pflicht; bei „ja“ ohne B
    kommt 422 an `q_which`, mit B steht alles im Eintrag.
  - Vitest: im Baukasten eine Regel auf eine frühere Frage, auf Englisch Name, Abschnitt und
    Frage, das gespeicherte JSON genau so; die erste Frage kann keine Regel haben. Im Portal
    erscheint B erst bei „ja“, ist dann Pflicht, verschwindet bei „nein“ und wird nicht
    gesendet; auf Englisch Name, Abschnitt, Fragen und Knopf englisch, ein Feld ohne eigenen
    Text mit seiner englischen Beschriftung; axe ohne Befund.
  - Playwright (`e2e/form-conditions.spec.ts`): die Person `englisch` sieht das Formular
    englisch, B erscheint erst bei „yes“ und ist Pflicht; der Vorgang trägt den deutschen
    Eintrag; axe ohne Befund.
  - Angepasst: `TestFreeAnswer` erwartet Deutsch statt der Sprache des Absenders (E134); die
    Attrappe der Tests wertet die Regeln mit derselben Auswertung aus.
  - Befund der ersten CI: `TestFanOutFiltersAndRights` (Webhooks) erwartete die Position der
    Abonnements genau beim Ereignis des Tests; der laufende Container auf derselben Datenbank
    reicht spätere Ereignisse weiter und schob sie schon weiter. Der Test verlangt jetzt
    „mindestens bis zum Ereignis“, was PL41 meint.
  - Ganze CI grün am 2026-10-29 im zweiten Lauf (nach dem Befund der Webhooks oben), in der WSL-Kopie: Go und Web (52 Vitest-Fälle, 9 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,99 ms, LZ2 10,02 ms, LZ3 12,65 ms, LZ4 10,71 ms, LZ5 9.704 Zeilen/s, LZ13 7.813 Ereignisse/s, LZ9 228 ms.
- **Nicht geprüft:** Regeln mit ODER und Vergleiche wie „größer als“ gibt es nicht (deine
  Entscheidung). Eine Frage, auf die sich eine Regel bezieht, lässt sich im Baukasten hinter die
  Regel schieben; das Speichern lehnt das dann ab, der Baukasten warnt nicht vorher. Die
  englischen Texte eines Felds ohne eigenen Fragetext kommen aus dem Typ. Weitere Sprachen
  als Englisch bietet der Umschalter nicht an.

### UI11 — Formulare, 2026-10-29

- **Grundlage:** Hauptplan E133, Inventar G130, G131, KF8; architektur.md (`internal/forms`);
  UI10 (Portal), PL42 (Zeitleiste); E20 dieses Plans; deine Entscheidungen vom 2026-10-29
  (Kerntyp `core.form`; Feld des Zieltyps plus freie Fragen; `dnd-kit` mit Tastatur, dazu
  Knöpfe; sehen und absenden darf, wer den Zieltyp anlegen darf).
- **Gebaut:**
  - Kerntyp `core.form` (`packages/core/types/form.yaml`): Name, Beschreibung, „Legt an“
    (Zieltyp), „Im Portal angeboten“, Aufbau als JSON. Geschrieben über die Datensätze und die
    Rechte an `core.form`.
  - `internal/forms`: prüft den Aufbau beim Speichern (Abschnitte und Fragen mit eigener ID,
    deutscher Titel; gebundene Fragen nur an vorhandene, nicht berechnete Felder und je Feld
    einmal; freie Fragen nur Text, Zahl, Ja/Nein, Datum, Zeitpunkt, Auswahl mit Antworten;
    jedes Pflichtfeld des Zieltyps ohne Vorgabe muss gefragt sein; freie Fragen nur bei
    Zeitleiste). `List` und `Get` geben angebotene Formulare an Konten mit `create` am
    Zieltyp, ohne Leserecht an `core.form`; `Submit` prüft die Antworten je Frage, setzt die
    Person als Anforderer und legt Datensatz und ersten Eintrag mit `records.CreateWithEntry`
    in einer Transaktion an. Probleme tragen die IDs der Fragen.
  - API: `GET /api/v1/forms`, `GET /api/v1/forms/{id}`, `POST /api/v1/forms/{id}:submit`.
  - Oberfläche: `render/FormBuilder.tsx` ist die Darstellung von `definition` im Formular des
    Datensatzes (`Form` kennt eigene Editoren je Feld): Abschnitte und Fragen ziehen mit
    `dnd-kit` 6.3.1 / sortable 10.0.0 (MIT) per Maus und Tastatur, Ansagen auf Deutsch und
    Englisch, dazu „hoch“, „runter“, „entfernen“; Antworten einer Auswahl je Zeile. Im Portal
    stehen „Formulare“ über „Meine Vorgänge“; `PortalFormPage` füllt mit demselben `Form`
    (Felder aus den Fragen, je Abschnitt eine Gruppe, Knopf „Absenden“) und führt auf die
    Seite des neuen Vorgangs, bei einem Typ ohne Portal auf einen Dank.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Einzeln (`internal/forms`): jede Ablehnung der Prüfung; freie Antworten als Text in der
    Sprache (ja/nein, Zahl ohne Nullen, Auswahl mit ihrer Beschriftung).
  - Über HTTP (`TestFormsOverHTTP`): sieben falsche Aufbauten abgelehnt; ein angebotenes
    Formular sieht das Konto mit `create`, ein Entwurf niemand, ein Leser ohne `create` bekommt
    404; das Konto liest `core.form` selbst nicht. Fehlende Pflichtantworten, falsche Auswahl,
    falscher Typ und unbekannte Fragen tragen die IDs der Fragen, ein zu langer Titel auch.
    Absenden legt den Vorgang mit der Person als Anforderer an; der erste Eintrag ist
    „Ort / Raum: 2.14 / Etage: 1. OG / Dringlichkeit / Dringend: ja / Betroffene: 3“, ohne
    Recht `comment`. Zurückgezogen nimmt es niemand mehr an.
  - Vitest (`render/FormBuilder.test.tsx`, `portal/Portal.test.tsx`): drei Abschnitte mit
    gebundener Frage, freier Pflichtfrage und Auswahl, Reihenfolge per Knopf, axe ohne Befund,
    gespeichertes JSON genau wie erwartet; Fragen hoch und entfernen, ein entferntes Feld ist
    wieder wählbar. Im Portal: Formular in der Liste, Abschnitte als Gruppen, Pflichtfrage am
    Platz gemeldet, Absenden schickt die Antworten je Frage, die Seite des Vorgangs zeigt den
    Eintrag; axe ohne Befund.
  - Playwright (`e2e/forms.spec.ts`): die Verwaltung baut drei Abschnitte nur mit der
    Tastatur, zieht „Ort“ mit Leertaste und Pfeil vor „Etage“, bietet das Formular an; die
    Nutzerin füllt es im Portal aus; der Vorgang zeigt die freien Antworten; axe ohne Befund
    auf Baukasten und Portalformular.
  - Angepasst: Die Attrappe der Tests kennt Formulare, `core.form` und Feldzustände anderer
    Typen; die Abdeckung der API hat ein gültiges Beispiel für `core.form`.
  - Befund beim Bau: `dnd-kit` meldet das Element gleich nach dem Aufnehmen „über sich
    selbst“, das überschrieb die Ansage „aufgenommen“ sofort. Angesagt wird jetzt nur ein
    anderes Element; bleibt es am Platz, heißt es „bleibt an seinem Platz“. Der Browsertest
    wartet auf die Ansagen, weil ein Pfeil vor dem Vermessen nichts bewegt.
  - Ganze CI grün am 2026-10-29 im dritten Lauf (erst Lint, dann der Browsertest, siehe oben), in der WSL-Kopie: Go und Web (49 Vitest-Fälle, 8 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,95 ms, LZ2 9,56 ms, LZ3 12,25 ms, LZ4 10,66 ms, LZ5 10.770 Zeilen/s, LZ13 8.790 Ereignisse/s, LZ9 187 ms.
- **Nicht geprüft:** Bedingungen und Übersetzung der Formulare kommen mit UI12; die Fragen
  und Abschnitte tragen bis dahin nur ihren deutschen Text. Der Eintrag mit den freien
  Antworten ist deutsch, weil der Client die Daten in Deutsch schreibt (E98). Fragen ziehen
  nur innerhalb eines Abschnitts. Anonyme Formulare, Ziele über mehrere Typen und Umfragen
  (architektur.md) kommen mit SD. Ein Formular, dessen Zieltyp später ein neues Pflichtfeld
  bekommt, wird erst beim nächsten Speichern abgelehnt; bis dahin scheitert das Absenden mit
  dem Problem des Felds.

### UI10 — Portal und Arbeitsplatz, 2026-10-29

- **Grundlage:** Hauptplan E132, Inventar G125; architektur.md §10 (`portal/`); PL42
  (Zeitleiste), UI9 (Einstellungen); E19 dieses Plans; deine Entscheidungen vom 2026-10-29
  (Sonderrecht `workplace`; Portal mit „Meine“, Anlegen und Antworten; die Oberfläche sperrt,
  die API bleibt bei ihren Rechten; Portal unter `/portal`).
- **Gebaut:**
  - Server: `/api/v1/auth/me` meldet `workplace` (Recht `workplace` an `core.ui`, System und
    Verwaltung über `*`) und `person_id`; Typen können `portal` mit den Feldern zum Anlegen
    nennen (`meta.Portal`, geprüft: Felder des Typs, nicht berechnet, je einmal);
    `probe.probe` bietet Titel und Art an.
  - Oberfläche: `portal/Portal.tsx` (eigener Rahmen mit Sprache, Theme, Name zu den eigenen
    Einstellungen, „Zum Arbeitsplatz“ nur mit `workplace`; „Meine Vorgänge“ je angebotenem
    Typ über die JSON-Abfrage „Anforderer ich oder selbst angelegt“; Anlegen nur mit den
    Portal-Feldern, ohne Rollenabschnitt, mit der Person als Anforderer; schlanke Seite mit
    Status, Portal-Feldern und Zeitleiste ohne „Privat“); der Router schickt ohne `workplace`
    jede Adresse des Arbeitsplatzes ins Portal; der Arbeitsplatz hat oben „Zum Portal“; die
    eigenen Einstellungen im Portal ohne Vorbelegung und globale Seite; `Form` mit `only`,
    `withRoles`, `extra`; `Timeline` mit `portal`.
  - CI: Rolle `e2e_portal` (Probe: lesen eigen und beteiligt, anlegen, kommentieren eigen und
    beteiligt) über die API des Testservers, Konto `nutzerin` mit dieser Rolle.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Einzeln (`internal/meta`): Portal-Felder, die es nicht gibt, berechnete und doppelte
    werden abgelehnt, eine leere Liste auch.
  - Über HTTP (`TestPortalOverHTTP`): die Verwaltung hat `workplace` und ihre Person; ein Konto
    ohne das Recht nicht; mit dem Recht in seiner Rolle schon (nach der Invalidierung); der Typ
    nennt seine Portal-Felder.
  - Vitest (`portal/Portal.test.tsx`): ohne `workplace` landen `/` und `/types/probe.probe` im
    Portal, ohne „Zum Arbeitsplatz“; Anlegen zeigt nur Titel und Art, keine Rollen, und
    schickt die Person als Anforderer; die Seite des neuen Vorgangs hat keine Möglichkeit für
    private Einträge, axe ohne Befund; er steht danach unter „Meine Vorgänge“; mit
    `workplace` geht es hin und zurück.
  - Playwright (`e2e/portal.spec.ts`): die Nutzerin landet nach der Anmeldung im Portal, die
    Adresse der Liste führt zurück ins Portal; sie legt einen Vorgang an (ohne Priorität),
    schreibt einen Eintrag, findet ihn unter „Meine Vorgänge“; axe ohne Befund.
  - Angepasst: Die Attrappe der Tests meldet `workplace` (Vorgabe ja) und `person_id`, kennt
    `or` in der Abfrage und den Ersteller neuer Datensätze; der Probe-Typ dort hat `portal`.
  - Befund der ersten CI: `TestDeliveryRetriesUntilAccepted` (Webhooks) schlug einmal fehl,
    „completed“ nach drei Versuchen bei zwei Aufrufen. Ursache in den Tests: die Ereignistests
    von `events` und `httpapi` löschen zur Simulation der Aufbewahrung den Anfang des
    gemeinsamen Protokolls, auch Ereignisse parallel laufender Pakete, und die Zustellung eines
    gelöschten Ereignisses endet still. Dasselbe traf in einem späteren Lauf
    `TestConcurrentWritersAndHorizon` (18 von 20 eigenen Ereignissen). Eine Sperre nur für die
    Webhooks reichte deshalb nicht: die beiden Aufbewahrungstests bekommen jetzt je eine eigene,
    frische Datenbank auf demselben Server (`internal/db/dbtest`, `Isolated`, am Ende gelöscht)
    und löschen nur dort. Im Betrieb löscht die Aufbewahrung nur, was älter als die Frist ist
    und alle dauerhaften Verbraucher genommen haben (PL41).
  - Ein dritter Lauf wurde rot, weil Pascal zur selben Zeit eine zweite CI aus der WSL-Kopie
    startete; `compose.yaml` setzt den Projektnamen `itsm`, beide nutzten dieselbe Datenbank,
    und das Leeren der Probe-Tabelle durch die API-Tests lief in einen Deadlock mit dem
    Bulk-Job von LZ13. Seitdem läuft die CI nach `rsync` in der WSL-Kopie
    `/home/pascal/police/itsm`, nie zwei Läufe gleichzeitig.
  - Befund der zweiten CI: der Selbsttest des Leistungsgates (`TestGateCatchesInjectedDelayInAB`)
    schlug fehl, weil derselbe Attrappenserver für LZ5 einmal 11 % weniger Zeilen/s schaffte
    (Einzelläufe 17.000 bis 32.000). Der Test prüft die Kette, nicht das Rauschen: er erlaubt
    jetzt 50 %; die 20-ms-Bremse fällt weiter durch (p95 etwa 21 ms gegen 1 ms). Das echte Gate
    der CI bleibt bei 10 %.
  - Ganze CI grün am 2026-10-29 im fünften Lauf (nach den Befunden oben), erstmals in der WSL-Kopie: Go und Web (46 Vitest-Fälle, 7 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,10 ms, LZ2 8,47 ms, LZ3 10,16 ms, LZ4 9,91 ms, LZ5 9.930 Zeilen/s, LZ13 8.040 Ereignisse/s, LZ9 301 ms.
- **Nicht geprüft:** Die Sperre ist Bedienung: wer ohne `workplace` die API direkt nutzt,
  bekommt, was seine Rechte erlauben (deine Entscheidung). Ohne Rolle mit `workplace` landen
  auch heutige Konten mit eingeschränkten Rollen im Portal; Rollen der Bearbeiter brauchen das
  Recht (die Verwaltung hat es über `*`). Live (UI8) gibt es im Portal nicht. Katalog und
  Wissen kommen mit SD. Die Rolle der CI legt das Skript über die API an, weil die
  Kommandozeile nur zuweisen kann.

### UI9 — Einstellungen persönlich und global, 2026-10-29

- **Grundlage:** Hauptplan E131, Inventar G170, G171, G205; PL6 (Einstellungen am Baum),
  Kern-Plan E59 (Recht `manage_settings`), WF15 (Mails in der Sprache der Person); E18 dieses
  Plans; deine Entscheidungen vom 2026-10-29 (Sprache an der Person, Rest je Konto; Sprache,
  Zeitzone, Theme, Vorbelegung; persönlich vor global; eigene Seite und Verwaltungsseite).
- **Gebaut:**
  - Server: Migration `00034_account_setting.sql` (Kerntabelle je Konto und Schlüssel);
    `internal/prefs` (`Get`: Sprache aus `core.person.language`, Zeitzone und Theme aus der
    eigenen Tabelle, sonst aus `ui.language`, `ui.time_zone`, `ui.theme` am Knoten der Person
    vererbt, sonst Vorgabe de, Europe/Berlin, neutral, jeweils mit Quelle; `Set` prüft
    Sprache, Zeitzone (IANA), Theme-Namen und Vorbelegung je Typ gegen dessen Felder, null
    nimmt den eigenen Wert weg; die Sprache schreibt es an die Person mit Systemrechten, weil
    es die eigene ist); API `GET` und `PATCH /api/v1/me/settings`.
  - Oberfläche: Der Rahmen lädt die Einstellungen und setzt Sprache, Theme und Zeitzone;
    Sprachwahl und Theme oben schreiben angemeldet in die Einstellungen; der Name oben führt zu
    „Meine Einstellungen“ (`app/SettingsPage.tsx`: Sprache, Zeitzone, Theme je mit „wie für
    meine Einheit eingestellt“ und „Gilt jetzt: … (Quelle)“; Vorbelegung je Typ mit Feld und
    Wert); „Globale Einstellungen“ setzt und entfernt die drei Vorgaben an einem Knoten und
    sagt, wenn das Recht fehlt; Zeitpunkte erscheinen überall in der Zeitzone der Person
    (`formatMoment`), der Export nimmt sie mit; das Formular eines neuen Datensatzes nimmt die
    Vorbelegung (Feldzustände aus WF2 gehen vor).
  - CI: drittes Konto `einsteller` für den Browser-Test.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Über HTTP (`TestSettingsOverHTTP`): neue Person mit Sprache de (eigene), Zeitzone Vorgabe
    Europe/Berlin; die Wurzel setzt Europe/London, die Person sieht sie mit Quelle `root`;
    eigene Sprache en, Zeitzone America/New_York, Theme probe und Vorbelegung für Probe; die
    Sprache steht danach an der Person, obwohl die Rolle nur Probe lesen darf; null gibt die
    Zeitzone an den Baum und die Sprache an die Vorgabe zurück, das Theme bleibt. Abgelehnt
    mit 422 am Feld: Zeitzone Mars/Base, Theme mit Leerzeichen, Sprache fr, unbekanntes Feld,
    unbekannter Typ, berechnetes Feld, unbekannter Schlüssel.
  - Vitest (`app/SettingsPage.test.tsx`): Englisch und Zeitzone speichern → „My settings“,
    `lang` en; ein neuer Start mit deutschem Browser zeigt „Types“ und „Sign out“; die
    Vorbelegung „Art = Störung“ für Probe füllt das Formular eines neuen Datensatzes; die
    globale Seite setzt Europe/London an `root.it`, entfernt es wieder und meldet das fehlende
    Recht; axe ohne Befund.
  - Playwright (`e2e/settings.spec.ts`, Browser auf Deutsch): Englisch in „Meine
    Einstellungen“ → alles englisch, axe ohne Befund; abmelden, Anmeldeseite deutsch,
    anmelden → wieder englisch; am Ende zurück auf Deutsch.
  - Ganze CI grün am 2026-10-29 im ersten Lauf: Go und Web (43 Vitest-Fälle, 6 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,28 ms, LZ2 8,09 ms, LZ3 10,04 ms, LZ4 8,96 ms, LZ5 12.267 Zeilen/s, LZ13 8.745 Ereignisse/s, LZ9 146 ms.
- **Nicht geprüft:** Die Seitengröße bleibt fest (nicht gewählt). Formate von Datum und Zahl
  folgen der Sprache, eigene Formate gibt es nicht. Die Anmeldeseite spricht die Sprache des
  Browsers, weil vor der Anmeldung niemand bekannt ist. Die Vorbelegung prüft beim Speichern
  nur, ob es die Felder gibt; ob ein Wert passt, sagt erst das Anlegen. Verweise und Rollen
  lassen sich nicht vorbelegen außer über die Kennung im Auswahlfeld. Für Konten ohne Person
  liegt die Sprache in der Kontentabelle; Mails gehen an Personen und nutzen sie nicht.

### UI8 — Live: Ereignisstrom, Anwesenheit, Benachrichtigungen, 2026-10-29

- **Grundlage:** Hauptplan E130, E29, Inventar G164; PL24 (Ereignisstrom), WF3 (Rollen);
  E17 dieses Plans; deine Entscheidungen vom 2026-10-29 (Hinweis mit „Neu laden“; schaut /
  bearbeitet gerade; Benachrichtigungen für meine Vorgänge nach Zustimmung; geänderte Zeilen
  sofort, neue als Hinweis).
- **Gebaut:**
  - Server: `internal/presence` (Lebenszeichen je Datensatz als Hash in Valkey, 60 s gültig,
    Start bleibt erhalten, Abgelaufene werden beim Lesen entfernt); `cache` mit `HSetExpire`,
    `HGetAll`, `HDel`; API `PUT` und `DELETE /api/v1/records/{typ}/{id}/presence` (lesen am
    Datensatz nötig, Name aus der Sitzung); der Ereignisstrom markiert Ereignisse an
    Datensätzen, bei denen die Person oder ihre Gruppe eine Rolle hat, mit `mine`
    (`records.SeeEvent`, `authz.Involved`), auch beim Nachholen; `VisibleEvent` behält den
    schnellen Weg ohne Lesen für System und `core.event`.
  - Oberfläche: `app/live.tsx` (eine EventSource je Tab im Rahmen, Abonnenten je Seite);
    `app/Notifications.tsx` (Knopf „Benachrichtigungen erlauben“, danach Benachrichtigung bei
    `mine`, fremdem Auslöser und verdecktem Tab; Klick öffnet den Datensatz);
    `render/Presence.tsx` (Lebenszeichen alle 20 s und sofort bei Wechsel von ungespeichert,
    Verlassen beim Schließen; „Anna schaut gerade · Ben bearbeitet gerade“); Detailseite
    folgt fremden Änderungen (Zeitleiste, Uhren, Übergänge, Feldzustände sofort; das Formular
    nur ohne ungespeicherte Eingabe, sonst „Geändert von … – Neu laden“); `Form` meldet
    ungespeicherte Eingaben; Liste liest fremd geänderte geladene Zeilen nach 2 s gebündelt
    mit ihrer Bedingung nach (passt eine nicht mehr, fällt sie heraus) und zeigt neue als
    „3 neue Datensätze – anzeigen“; Texte deutsch und englisch.
  - CI: zweites Konto `kollegin` für den Browser-Test; der Server des Browser-Tests läuft mit
    allen Rollen, damit sein eigener Verteiler die Ereignisse liefert.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Einzeln (`internal/presence`): andere zuerst die Bearbeitenden, Start bleibt bei erneutem
    Zeichen, nach 60 s ohne Zeichen weg, Verlassen sofort.
  - Über HTTP (`TestLiveOverHTTP`): drei Konten auf einem Datensatz sehen einander, die
    Bearbeitende zuerst; nach Verlassen ist sie weg; ein Konto ohne Leserecht bekommt 404. Im
    Strom ist die Änderung an Annas Vorgang (Anforderin) `mine`, die an einem anderen nicht.
  - Vitest (`app/live.test.tsx`, mit nachgebautem EventSource): „Anna bearbeitet gerade“;
    Annas Änderung erscheint ohne Neuladen; mit ungespeicherter Eingabe bleibt sie stehen und
    der Hinweis nennt Anna, „Neu laden“ übernimmt; eigene Änderungen aus einem anderen Fenster
    zählen nicht; in der Liste kommt eine geänderte Zeile mit genau einer Abfrage nach `id`
    wieder, ein neuer Datensatz nur über „1 neuer Datensatz – anzeigen“; eine Benachrichtigung
    nur für das Ereignis mit `mine` von jemand anderem bei verdecktem Tab, nach Zustimmung.
  - Playwright (`e2e/live.spec.ts`), zwei Personen in zwei Browsern: B sieht „Lasttest
    bearbeitet gerade“, axe ohne Befund; A speichert, B zeigt den neuen Titel und die Zeile in
    der Zeitleiste ohne Neuladen.
  - Ganze CI grün am 2026-10-29 im ersten Lauf: Go und Web (40 Vitest-Fälle, 5 Playwright-Fälle mit axe, davon der Live-Test mit zwei Personen), Leistungsgate bestanden; LZ1 p95 3,05 ms, LZ2 7,33 ms, LZ3 9,60 ms, LZ4 8,52 ms, LZ5 12.174 Zeilen/s, LZ13 7.956 Ereignisse/s, LZ9 119 ms.
- **Nicht geprüft:** Je Verbindung und Ereignis liest der Server den Datensatz einmal, um
  Sichtbarkeit und `mine` zu bestimmen; bei einem Bulk-Lauf mit 20.000 Ereignissen und vielen
  offenen Tabs ist das ungemessen (H14). Wer gerade bearbeitet, erfährt die Gegenseite erst mit
  ihrem nächsten Lebenszeichen (bis 20 s). Benachrichtigungen nur bei offenem Tab und in
  Browsern mit Notification-API; die Liste der Ereignisarten im Browser ist fest (neue Arten
  brauchen eine Zeile in `live.tsx`). Mehrere Tabs derselben Person zählen als eine Person.

### UI7 — Editor für formatierten Text mit Bildern, 2026-10-29

- **Grundlage:** Hauptplan E127, E22, Inventar G242; E1 dieses Plans (TipTap, MIT); PL18
  (Dateien), PL42 (Zeitleiste), WF15 (Mails), UI5 (Export); E16 dieses Plans; deine
  Entscheidungen vom 2026-10-29 (HTML, auf dem Server bereinigt; Einträge und Textfelder mit
  `format: rich`; ein Bild gehört zu seinem Eintrag; Feld `format` je Eintrag).
- **Gebaut:**
  - Server: `internal/richtext` (bluemonday v1.0.27, BSD-3-Clause: Absätze, Hervorhebung,
    Listen, Überschriften 2–4, Zitat, Code, Tabellen, Links nur http/https/mailto mit
    `nofollow noreferrer noopener` und neuem Fenster, Bilder nur aus `/api/v1/files/{id}`;
    `Images` findet die Bilder, `Text` gibt Klartext für Mail, Liste und Export); Migration
    `00033_rich_text.sql` (`entry.format`, `attachment.entry_id`); Einträge mit `format`
    (`text` oder `html`, bereinigt, nur leere Absätze zählen als leer); Feldattribut
    `format: rich` an Textfeldern (`meta`, geprüft), beim Schreiben bereinigt, die Länge zählt
    das HTML; neue Einträge mit Bildern: die Bilder müssen Dateien der Person sein, der Typ
    braucht `documents`, dann hängt `files.AttachEntry` sie mit dem Eintrag an den Datensatz,
    auch ohne `update` (das Schreiben brauchte `comment`); Bilder privater Einträge sehen
    Dokumentenliste und Herunterladen nur mit `read_private`, und der Verlauf nennt sie nicht;
    Mails geben `{{entry}}` als Klartext, der Export Rich-Felder als Klartext; `probe.probe`
    hat `solution` als Rich-Feld.
  - Oberfläche: TipTap 3.32.0 (`@tiptap/react`, `@tiptap/pm`, `@tiptap/starter-kit`,
    `@tiptap/extension-image`, MIT); `render/RichEditor.tsx` (Werkzeugleiste Fett, Kursiv,
    Liste, Nummeriert, Zitat, Link, Bild einfügen; Einfügen und Ziehen von Bildern; Hochladen
    als vorläufige Datei; `RichText` zeigt gespeichertes, bereinigtes HTML); Zeitleiste mit
    Editor und HTML-Einträgen; Formular mit Editor für Rich-Felder, schreibgeschützt als HTML;
    Listen zeigen den Klartext; Texte deutsch und englisch.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Einzeln (`internal/richtext`): Skript, Ereignis-Attribute, Stil, fremde und kaputte
    Bildquellen, `javascript:`-Links und Iframes verschwinden; Bilder, Tabellen, sichere Links
    bleiben; Bilder werden gefunden; Klartext mit Listenstrichen, Linkadresse und „[Bild]“.
  - Prüfweg über HTTP (`TestRichTextOverHTTP`): ein privater Eintrag mit Bild, Skript und
    `onclick` wird bereinigt gespeichert; das Bild ist Datei am Datensatz, die Verwaltung
    sieht und lädt es, der Anforderer ohne `read_private` weder in den Dokumenten noch beim
    Herunterladen (404), der Verlauf nennt es nicht. Der Anforderer (nur `comment`) fügt einem
    öffentlichen Eintrag sein eigenes Bild bei: in den Dokumenten und im Verlauf. Das Bild
    eines anderen: 422; nur leere Absätze: 422; Format `md`: 422. `solution` wird bereinigt
    gespeichert, der Export zeigt „Kabel getauscht“, der Typ meldet `format: rich`.
  - Vitest (`RichEditor.test.tsx`, `RecordPage.test.tsx`): „Fett“ macht den markierten Text
    fett und ist gedrückt; ein Bild über „Bild einfügen“ wird hochgeladen und steht als
    `<img src="/api/v1/files/…">` im HTML; axe ohne Befund; gespeichertes HTML wird formatiert
    gezeigt, Listen bekommen Klartext; die Zeitleiste schickt `format: html`. In jsdom erreicht
    Tippen ProseMirror nicht; dort setzt der Test den Text über die Editor-Instanz.
  - Playwright gegen den Server: Einträge echt getippt; „Siehe“, dann „Fett“ und „Foto“,
    dann ein Bild über „Bild einfügen“; nach „Eintragen“ steht „Foto“ fett im neuesten
    Eintrag und das Bild mit `/api/v1/files/<id>`, die Dokumente des Datensatzes nennen diese
    Datei; axe ohne Befund.
  - Angepasst: `RecordPage.test.tsx` und `e2e/records.spec.ts` schrieben in ein Textfeld,
    jetzt in den Editor; der Browser-Test zählt mit dem Bildeintrag drei Einträge.
  - Ganze CI grün am 2026-10-29 im vierten Lauf: Go und Web (37 Vitest-Fälle, 4 Playwright-Fälle mit axe), Lizenzen mit TipTap und bluemonday (58 Go-Module, 86 Web-Pakete), Leistungsgate bestanden; LZ1 p95 3,08 ms, LZ2 6,40 ms, LZ3 10,25 ms, LZ4 9,13 ms, LZ5 12.931 Zeilen/s, LZ13 8.783 Ereignisse/s, LZ9 126 ms. Gefunden in der CI: Lauf 1 zeigte einen echten Fehler, ein Klick auf „Fett“ nahm dem Editor den Fokus und getippter Text ging an den Knopf (behoben: die Knöpfe der Leiste nehmen keinen Fokus); Lauf 2 nahm im Browser-Test die Verlaufszeile „Datei angehängt“ derselben Sekunde für den neuesten Eintrag (Test sucht den Eintrag jetzt über sein Bild); Lauf 3 scheiterte am Linter in einem angefangenen Test von UI8, der im Verzeichnis lag (behoben; seitdem kein Code der nächsten Aufgabe während einer CI).
- **Nicht geprüft:** Tabellen anlegen kann der Editor nicht (der Server ließe sie durch);
  Einfügen aus der Zwischenablage ist nur im Code, nicht im Browser-Test (dort über den
  Knopf). Bilder in Rich-Feldern (nicht Einträgen) bleiben vorläufige Dateien und verfallen
  nach `files.temporary_ttl`, solange niemand sie anhängt; das Anhängen beim Speichern eines
  Felds fehlt noch. Ein Eintrag, dessen Bild sich nicht anhängen ließ, steht trotzdem (Log).
  Die Volltextsuche liest Rich-Felder mit ihren Tags (PostgreSQL überliest Tags als solche,
  nicht gemessen). Link-Eingabe über `window.prompt`.

### UI6 — Massenaktionen auf gewählte Datensätze, 2026-10-29

- **Grundlage:** Hauptplan E126, E24, E29, Inventar G234; WF1 (Übergänge), WF2
  (Feldzustände), WF3 (Rollen), UI28 (Auswahlfeld); E15 dieses Plans; deine Entscheidungen
  vom 2026-10-29 (neuer Endpunkt `:batch`; Version je Zeile, Konflikt melden; Feld setzen,
  Übergang, Rolle setzen, Papierkorb; Auswahl je Zeile und alle geladenen).
- **Gebaut:**
  - Server: `internal/records/batch.go` (`Batch` mit bis zu 1.000 Zeilen, je Zeile genau eine
    Änderung: `set`, `transition` mit `set` als Feldern, `trash`; jede Zeile über `Update`,
    `Transition` oder `TrashVersion` mit ihrer Version; Ergebnis `ok`, `conflict`, `refused`,
    `forbidden`, `not_found`, `invalid`, `error` mit Meldung und Problemen); `TrashVersion`
    prüft die Version beim Papierkorb; API `POST /api/v1/records/{typ}:batch`
    (`internal/httpapi/batch.go`, OpenAPI `BatchRequest`, `BatchRow`, `BatchOutcome`).
  - Oberfläche: `render/List.tsx` (Spalte mit Kästchen je Zeile und im Kopf für alle geladenen,
    ein Klick ins Kästchen öffnet den Datensatz nicht; die Auswahl gehört nicht zu den Spalten
    der Ansicht, des Exports oder der Spaltenwahl); `render/Batch.tsx` (Leiste über der Liste
    mit der Zahl der Gewählten; Aktion Feld setzen mit Eingabe nach Feldart und „leeren“,
    Übergang, Rolle setzen mit dem Auswahlfeld, Papierkorb mit zweitem Klick; Ergebnis als
    Zählung je Ausgang und Liste der nicht geänderten mit Titel und Grund; danach lädt die
    Liste neu, die Auswahl bleibt); Texte deutsch und englisch.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Über HTTP (`TestBatchOverHTTP`): ein Aufruf mit neun Zeilen ergibt ok, conflict (alte
    Version), ok (Übergang start), refused (finish aus „Neu“), ok (Papierkorb), ok (Rolle an
    der neuen Version 2 → 3), invalid (Priorität 99, mit Problem am Feld), not_found, invalid
    (zwei Änderungen in einer Zeile). Danach hat der erste Datensatz Art und Anforderer, der
    dritte den Status „in Arbeit“, der vierte ist im Papierkorb, der zweite unverändert bei
    Version 1; der Verlauf nennt `kind` und `actors.requester`. Ein Konto mit nur Lesen bekommt
    `forbidden`. Leer und 1.001 Zeilen: 422.
  - Vitest (`Batch.test.tsx`): alle sechs geladenen gewählt, Art auf Störung, ein Datensatz
    inzwischen geändert → „5 geändert · 1 inzwischen geändert (Konflikt)“, die Zeile nennt
    ihren Titel; der Aufruf trägt sechs Zeilen mit ihren geladenen Versionen; axe ohne
    Befund. Zwei gewählt, Übergang „Beginnen“ → einer geändert, einer abgelehnt; Papierkorb
    erst nach „Wirklich 2 in den Papierkorb“.
  - Playwright gegen den Server: 500 Datensätze über den Abgleich angelegt, in der Liste
    gefiltert und bis 500 nachgeladen, alle gewählt, Priorität auf 5 → genau ein Aufruf an
    `:batch`, „500 geändert“; axe ohne Befund mit der Leiste.
  - Ganze CI grün am 2026-10-29 im ersten Lauf: Go und Web (35 Vitest-Fälle, 4 Playwright-Fälle mit axe, davon die 500 Datensätze in 17,5 s für alle vier), Leistungsgate bestanden; LZ1 p95 3,12 ms, LZ2 8,20 ms, LZ3 9,70 ms, LZ4 9,54 ms, LZ5 11.226 Zeilen/s, LZ13 8.020 Ereignisse/s, LZ9 121 ms.
- **Nicht geprüft:** „Alle Treffer“ ohne Laden gibt es nicht (deine Entscheidung); für 500
  muss die Liste fünf Seiten nachladen. Die Zeilen laufen nacheinander in einem Aufruf; bei
  1.000 Zeilen dauert das einige Sekunden, ohne Fortschrittsanzeige. Der Papierkorb hat
  in der Oberfläche keinen Weg zurück (Wiederherstellen nur über die API). Die Leiste prüft
  vorher nicht, welche Übergänge für welche Zeile gelten; das Ergebnis sagt es danach.

### UI5 — Export der Liste als CSV, XLSX und ODS, 2026-10-29

- **Grundlage:** Hauptplan E125, E40, Inventar G230, G231; PL14 (JSON-Abfrage), PL18
  (Dateien), PL23 (Jobs), PL27 (Ereignisprotokoll), UI4 (Bedingung, Spalten, Sortierung der
  Liste); E14 dieses Plans; deine Entscheidungen vom 2026-10-29 (auf dem Server gestreamt;
  bis 100.000 sofort, darüber als Job; Anzeigewerte, Datum als Datum; Protokoll im
  Ereignisprotokoll).
- **Gebaut:**
  - Server: `internal/export` (`export.go` Planung der Spalten, Lesen zu 500 über die
    JSON-Abfrage mit den Rechten der Person, Zellen nach Feldart, Job `export.file` mit
    vorläufiger Datei der Person; `csv.go` Semikolon, BOM, CRLF, Dezimalkomma; `xlsx.go` mit
    excelize v2.9.1 (BSD-3-Clause) als Stream, Kopf fett, Datum und Zeitpunkt mit deutschem
    Format; `ods.go` eigenes Schreibstück, `mimetype` zuerst und unkomprimiert, typisierte
    Zellen mit Datumsstil); API `POST /api/v1/records/{typ}:export` (200 mit Datei,
    `Content-Disposition`, `ITSM-Rows`; 202 mit Job über 100.000 Zeilen; 422 bei falschem
    Format, falscher Spalte, unbekannter Zeitzone; 503 ohne Dateispeicher für große);
    Migration `00032_export_log.sql` (Kategorie `data`), `GET /api/v1/audit/data`.
  - Oberfläche: `render/Export.tsx` (Format, „Exportieren“ mit Bedingung, Spalten, Sortierung,
    Sprache und Zeitzone des Browsers; Datei sofort gespeichert; bei einem Job Meldung, Abfrage
    alle 2 s, danach „Datei herunterladen“); in der Werkzeugleiste der Liste; Texte deutsch
    und englisch.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Prüfweg über HTTP (`TestExportOverHTTP`): drei Datensätze, gefiltert nach dem Anfang des
    Titels, Spalten Titel, Art, Priorität, Aktiv, Fällig am, Übergeordnet, Status, sortiert
    nach Titel. Die CSV ist Byte für Byte wie erwartet (BOM, „Störung“, „4,5“, „ja“/„nein“,
    „02.11.2026“, der Titel des übergeordneten Datensatzes, „Neu“), `ITSM-Rows: 3`; englisch
    „Fault“ und „yes“; XLSX mit vier Zeilen, Datum als „02.11.2026“ und 4.5 als Zahl; ODS mit
    `mimetype` zuerst, „Störung“ als Text und 4.5 als Zahl. Abgelehnt mit 422: Format pdf,
    unbekannte Spalte, Zeitzone Mars/Base. Eine Person, die nur eigene Datensätze liest, bekommt
    nur ihren einen. `/api/v1/audit/data` zeigt den Export mit Typ, Kategorie `data` und
    Zeilenzahl. Mit der Grenze auf 2 gesenkt: 202, der Job schreibt die Datei (4 Zeilen, die
    Verwaltung liest inzwischen auch den Datensatz der Person), die Verwaltung lädt sie, die
    andere Person bekommt 404.
  - Einzeln (`internal/export`): CSV genau, ODS-Aufbau und -Zellen, XLSX-Zellarten und
    angezeigte Daten, Dateiname.
  - Vitest (`Export.test.tsx`): der Knopf schickt Format, Bedingung, Spalten, `-updated_at`,
    Sprache und Zeitzone und speichert die Datei; axe ohne Befund; ein großer Export zeigt
    „läuft als Job“, sperrt den Knopf und bietet nach dem Job „Datei herunterladen“ mit dem
    Link auf die Datei.
  - Playwright gegen den Server: in der gespeicherten Suche aus UI4 CSV exportiert, der
    Browser speichert `probe-probe-….csv` mit Kopf und genau der einen Zeile.
  - Ganze CI grün am 2026-10-29 im ersten Lauf: Go und Web (33 Vitest-Fälle, 3 Playwright-Fälle mit axe), Lizenzprüfung mit excelize und seinen Abhängigkeiten (55 Go-Module) grün, Leistungsgate bestanden; LZ1 p95 3,35 ms, LZ2 7,71 ms, LZ3 10,16 ms, LZ4 9,11 ms, LZ5 10.587 Zeilen/s, LZ13 10.105 Ereignisse/s, LZ9 132 ms.
- **Nicht geprüft:** Ein echter Export über 100.000 Zeilen (der Job ist mit gesenkter Grenze
  belegt). Ein großer Export als Job ist durch `files.max_size` begrenzt (Vorgabe 100 MiB);
  darüber scheitert der Job mit Hinweis auf die Einstellung. Ob Excel und LibreOffice die
  Dateien ohne Rückfrage öffnen, ist nur über excelize und den Aufbau geprüft, nicht von Hand.
  Rollen am Datensatz sind keine Spalte der Liste und fehlen im Export. Ein Abbruch mitten im
  Streamen liefert eine unvollständige Datei, protokolliert ist der Export trotzdem. Die
  Zeitzone kommt aus dem Browser; ohne Angabe UTC.

### UI4 — Filter und gespeicherte Suchen, 2026-10-29

- **Grundlage:** Hauptplan E40, E124, Inventar G112; WF3 (Reichweite `involved`), PL14
  (JSON-Abfrage), UI28 (Auswahlfeld); E13 dieses Plans; deine Entscheidungen vom 2026-10-29
  (Kerntyp mit Rolle „geteilt mit“; Bedingungsliste mit UND; Filter, Spalten und Sortierung
  speichern; über der Liste und als Link) und, nach dem Befund beim Bauen, mehrere
  Reichweiten je Rolle (Kern-Plan E58).
- **Gebaut:**
  - Server: Kerntyp `packages/core/types/saved_search.yaml` (Name, Typ, Bedingung, Spalten,
    Sortierung; Merkmale `history`, `actors`; Rolle `shared` für Personen und Gruppen);
    `internal/savedsearch` (Prüfung beim Speichern: Typ installiert, Bedingung als JSON-Objekt
    und von der JSON-Abfrage angenommen, Spalten und Sortierung vorhanden); Migration
    `00031_permission_scopes.sql`, `authz.UnsetPermission` mit Reichweite, API `DELETE
    …/permissions/{typ}/{aktion}?scope=…`.
  - Oberfläche: `render/Filters.tsx` (Bedingungen „Feld – Vergleich – Wert“ mit UND; Vergleiche
    nach Feldart: Text enthält/ist/beginnt mit/leer, Zahl und Datum ist/ab/bis/leer, Zeitpunkt
    ab/bis mit Tagesbeginn und -ende, Auswahl und Status ist/ist nicht, Wahrheitswert,
    Verweis und Rolle mit dem Auswahlfeld, Rolle auch „ich“ und „meine Gruppen“; in die
    JSON-Abfrage und zurück); `render/List.tsx` (mit Bedingung über `:query`; Spalten,
    Reihenfolge und Sortierung aus der Suche, dann nichts im Browser gespeichert; meldet die
    Ansicht nach oben); `render/TypePage.tsx` (Auswahl „Gespeicherte Suchen“ mit „Eigene“ und
    „Mit mir geteilt“, „Als neue Suche speichern“, „Änderungen speichern“ nur für eigene,
    „Teilen und verwalten“ führt zur Seite der Suche, wo die Rolle „Geteilt mit“ gesetzt wird;
    `?search=<id>` in der Adresse; eine Bedingung, die die Filter nicht darstellen können,
    gilt wie sie ist und wird als Text gezeigt); `app/router.tsx` (`search` als Parameter);
    `api/client.ts` (`query`); Texte deutsch und englisch.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Prüfweg über HTTP (`TestSavedSearchesOverHTTP`): eine Rolle mit `read` auf Probe nur
    `own` und auf `core.saved_search` sowohl `own` als auch `involved`. Anna speichert
    „Störungen“ und teilt sie mit Ben: Anna und Ben sehen sie, Carl nicht (auch einzeln 404);
    Ben darf sie nicht ändern. Mit derselben Bedingung sieht Anna nur ihre Störung, Ben nur
    seine, die Verwaltung beide. Abgelehnt mit 422 am richtigen Feld: unbekannter Typ,
    Bedingung kein JSON, unbekanntes Feld, unbekannter Vergleich, unbekannte Spalte,
    unbekannte Sortierung. Die Rolle hält beide Reichweiten; `?scope=involved` entfernt nur die
    eine (Ben sieht die Suche nicht mehr, Anna schon), ohne Reichweite gehen alle, ein zweites
    Mal 404.
  - Vitest (`TypePage.test.tsx`): Bedingungen in die JSON-Abfrage und zurück, unvollständige
    weggelassen, ODER als nicht darstellbar erkannt, „bis“ beim Zeitpunkt am Tagesende; in der
    Liste „Art ist Störung“ → 3 von 9, gesendet als `where`; axe ohne Befund; gespeichert mit
    Bedingung, Spalten und `-updated_at`, die Adresse trägt die Suche, die Überschrift ihren
    Namen, die Auswahl trennt „Eigene“ und „Mit mir geteilt“; per Link wieder geöffnet mit
    „Störung“ im Filter; eine geteilte Suche zeigt ihre Bedingung, kein „Änderungen
    speichern“, dafür „Über diese Suche“.
  - Playwright gegen den Server: zwei neue Datensätze, Filter Titel enthält und Art ist
    Störung → genau einer; axe ohne Befund; als Suche gespeichert, die Adresse trägt sie,
    ohne sie ist der Filter weg, per Link wieder da mit beiden Bedingungen.
  - Angepasst: `TestSecurityEventLog` setzte zweimal dieselbe Typ-Aktion mit verschiedener
    Reichweite und erwartete, dass die zweite die erste ersetzt; seit E58 sind das zwei
    Zeilen, also setzt der Test dieselbe Reichweite zweimal (mit einer Bedingung) und
    erwartet „vorher“ an der Zeile derselben Reichweite. Die Abdeckungsprobe (PL30) legt
    `core.saved_search` mit leerer Bedingung an.
  - Ganze CI grün am 2026-10-29 im zweiten Lauf: Go und Web (31 Vitest-Fälle, 3 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,60 ms, LZ2 9,83 ms, LZ3 12,74 ms, LZ4 10,62 ms, LZ5 13.545 Zeilen/s, LZ13 8.782 Ereignisse/s, LZ9 97 ms. Der erste Lauf scheiterte nur im Browser-Test: „Neu“ traf auch „Als neue Suche speichern“; der Knopf wird jetzt genau gesucht.
- **Nicht geprüft:** Rollen für Nutzer, die Suchen speichern dürfen, liefert kein Paket mit;
  sie kommen mit dem Rollen-Editor (UI27) oder per API. Löschen einer Suche geht nur über die
  API (die Detailseite hat keinen Papierkorb-Knopf). Verschachtelte Bedingungen mit ODER legt
  nur die API an; die Liste wendet sie an und zeigt sie als Text. Sortieren geht weiter nur
  nach den Systemspalten im Spaltenkopf. Die Auswahl zeigt höchstens 100 Suchen je Typ.
  Spalten umordnen geht in der Oberfläche nicht, nur über die gespeicherte Liste der Spalten.

### UI28 — Auswahlfeld mit Suche für Verweise und Rollen, 2026-10-29

- **Grundlage:** Hauptplan E123; UI3 (Rollen nur angezeigt), UI2 (Verweise als UUID-Eingabe);
  PL26 (Volltextsuche), WF3 (Rollen); E12 dieses Plans; deine Entscheidungen vom 2026-10-29
  (vor UI4 einschieben; Volltextsuche, sonst Liste; Rollen im Formular mit Speichern; eigener
  Baustein; Nummer UI28).
- **Gebaut:** `render/RefPicker.tsx` (Combobox nach WAI-ARIA: Eingabe mit `role="combobox"`,
  Liste mit `role="listbox"`, Pfeiltasten, Enter, Escape, `aria-activedescendant`, Klick ohne
  Fokusverlust; Suche nach 200 ms Ruhe über `/api/v1/search` für Typen mit durchsuchbarem Feld,
  die ersten 100 mit Filter im Browser für die anderen; Gewähltes als Marken mit „Entfernen“;
  `single` für Verweise und einzelne Rollen); `render/Form.tsx` (Verweisfelder über den Baustein
  mit `Controller`; Abschnitt „Rollen am Datensatz“ mit einem Feld je Rolle, gesperrt wie der
  Datensatz oder bei schreibgeschütztem `actors`; gesendet werden nur geänderte Rollen, damit
  jemand ohne Recht auf die Rollen die Felder weiter speichern kann; 422 an einer Rolle landet
  an ihr; „Serverstand übernehmen“ nimmt auch die Rollen); `api/client.ts` (`search`,
  `single` an der Rolle); Texte deutsch und englisch.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Vitest (`RefPicker.test.tsx`): „Übergeordnet“ (Typ ohne durchsuchbares Feld) zeigt nach
    „000002“ nur noch „Probe 000002“, axe ohne Befund bei offener Liste, Pfeil runter und
    Enter wählen, das Feld ist danach leer und zu; der Anforderer über die Suche („ber“ →
    „Berg“), die Marke zeigt „Anna Berg“, Escape schließt; Anlegen schickt `parent` und
    `actors.requester`. Am bestehenden Datensatz schickt eine reine Titeländerung keine
    Rollen; „Entfernen“ und Speichern schickt `requester: []`.
  - Playwright gegen den Server (`e2e/records.spec.ts`): eine neue Person über die API, im
    Formular als Anforderer per Namen gesucht und gewählt, axe ohne Befund, gespeichert →
    Version 5, die Seitenleiste nennt sie, die Zeitleiste zeigt „Anforderer: – → Erika …“.
  - Die bisherigen Tests laufen unverändert; das Formular rendert „Übergeordnet“ jetzt als
    Auswahlfeld, der Formulartest mit axe blieb grün.
  - Ganze CI grün am 2026-10-29 im ersten Lauf: Go und Web (29 Vitest-Fälle, 2 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 2,97 ms, LZ2 7,05 ms, LZ3 8,33 ms, LZ4 7,83 ms, LZ5 12.236 Zeilen/s, LZ13 8.959 Ereignisse/s, LZ9 113 ms.
- **Nicht geprüft:** Ein Typ ohne durchsuchbares Feld mit mehr als 100 Datensätzen zeigt nur
  die ersten 100 (nach Kennung), der Rest ist nicht wählbar; das betrifft heute die kleinen
  Auswahltypen. Die Suche liefert höchstens 20 Treffer. Der Konfliktvergleich (E29) nennt
  geänderte Rollen nicht als Feld. Personen sucht die Suche nach Vor-, Nachname und weiteren
  durchsuchbaren Feldern; welche das sind, sagt das Paket. Die Marke „Entfernen“ nennt den
  Namen nicht im Knopftext. Bildschirmleser sind nicht von Hand geprüft, nur axe und die
  Tastatur im Test.

### UI3 — Detailseite mit Zeitleiste und Verlauf, 2026-10-29

- **Grundlage:** Hauptplan E121, E122, Inventar G40; PL42 (Zeitleiste), WF3 (Rollen), WF9
  (Uhren), WF13 (Aufgaben und Summen); E11 dieses Plans; deine Entscheidungen vom 2026-10-29
  (Kopf, Felder, Seitenleiste; Neuestes oben; je Version gruppiert mit Umschalter; Rollen nur
  anzeigen; nach dem Speichern auf der Seite bleiben).
- **Gebaut:** `render/RecordPage.tsx` (Kopf mit Titel, Version, „Gespeichert.“ und dem
  Statuskasten; Raster mit Formular und Zeitleiste links, Seitenleiste rechts; nach dem
  Speichern bleibt die Seite und lädt Feldzustände, Übergänge, Zeitleiste und Uhren neu, ein
  Übergang ebenso); `render/Timeline.tsx` (Eingabe mit „Privat“, 403 mit Grund, Blöcke je
  Eintrag oder je Version und Person, „Feld: alt → neu“ mit Bezeichnungen von Feld, Auswahl,
  Status, Rolle und Übersetzung, Aktionen ohne Feld als eine Zeile mit Regel, Umschalter
  „Alles / Nur Einträge“; ein Typ mit `history` ohne `timeline` zeigt seinen Verlauf ohne
  Eingabe); `render/Sidebar.tsx` (Rollen mit Namen, Uhren mit Ampel „Im Plan / Knapp / Verletzt
  / Erfüllt / Angehalten / Entfällt“ und Rest- oder Überzeit, Bezüge aus den Summenfeldern des
  Typs mit Summen und bis zu 50 Datensätzen mit Status); `render/RefName.tsx` (Name eines
  verwiesenen Datensatzes, Person mit Vor- und Nachname); `api/client.ts` (Zeitleiste,
  Eintrag, Verlauf, Uhren, verweisende Datensätze; `traits`, `actors`, `rollup` am Typ);
  Tokens `--success` und `--warning` für die Ampel; Texte deutsch und englisch.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Vitest (`RecordPage.test.tsx`): Blöcke je Version und umgedreht; Ampel und Dauer; die
    Seite zeigt den Anforderer als „Anna Berg“, die Uhr „Reaktion“ knapp mit „Noch 10 min“,
    „Lösung“ verletzt mit „Überfällig seit 1 h 30 min“; die Zeitleiste oben den privaten
    Eintrag mit „Privat“, dann Annas Eintrag, dann den Block der Version 2 mit „Art: Frage →
    Störung“ und „Priorität: 3 → 2“, unten „Angelegt“; axe ohne Befund; „Nur Einträge“ blendet
    die Änderungen aus; ein neuer Eintrag steht oben, das Feld ist danach leer; ein privater
    Eintrag ohne Recht meldet „keine privaten Einträge“.
  - Playwright gegen den Server (`e2e/records.spec.ts`): nach dem Speichern „Gespeichert.“ auf
    der Seite; die Zeitleiste zeigt „Titel: … (Fenster 2) → … (einig)“, „Status: Neu → In
    Arbeit“ und „Angelegt“; zwei neue Einträge stehen oben, der private mit „Privat“; „Nur
    Einträge“ zeigt genau zwei; die Version bleibt 4; axe ohne Befund.
  - Angepasst: `Form.test.tsx` und `e2e/records.spec.ts` erwarteten nach dem Speichern die
    Liste; jetzt bleibt die Seite stehen (deine Entscheidung). „Version n“ wird im Browser-Test
    genau gesucht, weil die Blöcke der Zeitleiste ihre Version nennen. Die Test-Attrappe
    (`test/fakeApi.ts`) kennt Zeitleiste, Uhren und Personen, der Probe-Typ dort `timeline` und
    die Rolle Anforderer.
  - Ganze CI grün am 2026-10-29 im ersten Lauf (mit der Korrektur an `/history`): Go und Web (26 Vitest-Fälle, 2 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 2,88 ms, LZ2 7,17 ms, LZ3 8,40 ms, LZ4 7,88 ms, LZ5 10.252 Zeilen/s, LZ13 8.933 Ereignisse/s, LZ9 103 ms.
- **Nicht geprüft:** Die Seite weiß nicht vorher, ob man `comment` oder `read_private` hat;
  Eingabe und „Privat“ stehen immer da, der Server lehnt ab und die Seite sagt warum. Rollen
  ändern und Verweise per Namen wählen kam mit UI28. Beziehungen stehen seit SD4 im Reiter „Bezüge“ (E38), Dateien fehlen noch. Eine lange
  Zeitleiste kommt ganz, ohne Nachladen. Die Ampel „knapp“ gilt fest unter einem Fünftel der
  Zielzeit. Die Seitenleiste zeigt höchstens 50 verweisende Datensätze. Nichts davon live (UI8).
  Das Raster mit Seitenleiste ist nur im Browser-Test und in jsdom geprüft, nicht von Hand auf
  schmalen Bildschirmen.

### UI2 — Liste und Formular aus der Definition, 2026-10-20

- **Grundlage:** Hauptplan E12, E29, E41, E42; Architektur §6.1 und §10; PL3, PL4; E7 bis E10
  dieses Plans.
- **Gebaut:** `web/src/render/fields.ts` (Feldtyp → Spalte, Formularfeld, zod-Regel, Umwandlung
  Formular ↔ API, geänderte Felder), `render/List.tsx` (TanStack Table und Virtual, Seiten per
  Schlüssel, Sortierung, Spaltenwahl, Zeile öffnet den Datensatz; vor der ersten Messung zeigt
  die Liste die ersten 30 Zeilen schlicht), `render/Form.tsx` (ein Feld je Typ: Text, Zahl,
  Kontrollkästchen, Datum, Datum mit Zeit, Auswahl, Verweis als ID; Pflicht und Regeln aus der
  Definition; 422 des Servers landet am Feld; 409 → Konfliktansicht), `render/RecordPage.tsx`
  (`/types/<typ>/new` und `/types/<typ>/<id>` mit Brotkrumen und Version), `render/TypePage.tsx`
  zeigt die Liste. `api/schema.d.ts` erzeugt, `api/client.ts` auf `openapi-fetch` umgestellt
  (auch Anmeldung und Typen). Testdaten: `scripts/seed-probe.sql` schreibt 100.000 Probe-Datensätze
  per `psql`. Browser-Tests: `e2e/records.spec.ts` (Liste, Anlegen, Konflikt zweier Fenster,
  LZ9-Messung), `e2e/scroll.spec.ts` (nur mit `ITSM_E2E_SCROLL=1`, gegen die Seed-Daten).
- **Belegt, 2026-10-20 im Werkzeug-Container:**
  - Vitest, 18 Tests grün: zod-Regeln aus den Feldern (Pflicht, min/max, Auswahl, UUID);
    Vorgaben ins Formular und leere Eingabe als `null`; Anlegen mit je einem Bedienelement pro
    Feldtyp ohne axe-Befund, Pflichtfeld benannt, Datensatz entsteht mit `kind=fault` und
    `priority=3` aus der Vorgabe; Ändern mit `If-Match`, bei Versionssprung Konfliktansicht
    mit „Von Kollegin geändert" gegen „Meine Fassung", genau ein PATCH, danach „trotzdem
    speichern" → Version 3. Liste: erste Seite mit `limit=100&sort=-updated_at`, weniger als
    100 Zeilen im DOM, Sortierklick → `sort=updated_at` und `aria-sort`, Spalte ausblenden →
    `localStorage`, Zeile öffnet den Datensatz.
  - Playwright gegen den Server: Liste, Anlegen (Version 1), zweites Fenster speichert zuerst,
    erstes sieht den Konflikt mit beiden Titeln, übernimmt den Serverstand, speichert → Version 3;
    axe ohne Befund auf Liste, Formular und Konflikt. **LZ9 Seitenwechsel zur Liste: 102 ms**
    (Ziel ≤ 500 ms, erste Lesung, kleine Tabelle).
  - **100.000 Datensätze:** `seed-probe.sql` → 138.016 Zeilen (inklusive Altbestand); der
    Scroll-Test lädt 40 Seiten nach (4.100 Zeilen), je Seite p50 42 ms, p95 144 ms, nie mehr
    als 80 Zeilen im DOM.
  - `scripts/ci.sh` komplett grün: 18 Vitest-Tests, beide Browser-Tests (LZ9 dort 191 ms), Gate bestanden.
  - ESLint, Prettier, `tsc`, Vite-Build grün; `pnpm api:gen` ergibt dieselbe `schema.d.ts`.
- **Nicht geprüft:** Verweisfelder sind eine UUID-Eingabe, der Auswahldialog kommt mit UI3;
  keine Filter in der Liste (UI3 gespeicherte Suchen), keine Volltextsuche (PL26); „wer bearbeitet
  gerade" (live, E29) braucht SSE (PL24/UI); Papierkorb, Wiederherstellen und Verlauf haben noch
  keine Oberfläche; `fields` der Liste beschränkt die Spalten, die Summe aller Felder je Zeile
  bleibt bei breiten Typen zu prüfen. Ruckelfreiheit beim Scrollen ist eine Sichtprüfung durch
  dich; gemessen ist nur das Nachladen.
