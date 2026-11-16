# itsm (Arbeitstitel)

Ein ITSM-Werkzeug nach ITIL 4 als Ablösung von GLPI: eine datengetriebene Plattform, auf der jede
Practice ein Paket ist. Produkt der group24 AG, Kern unter AGPL-3.0-only.

*An ITSM platform following ITIL 4, built to replace GLPI: a data-driven core on which every
practice is a package. A product of group24 AG; the core is licensed under AGPL-3.0-only.*

## Stand / Status

In Arbeit, Stand 2026-10-29 (Einzelheiten im Hauptplan `docs/0_maintask.md`): Der Plattform-Kern
steht mit Meta-Modell, Datensätzen, REST-API und OpenAPI, Organisationsbaum, Rollen und Rechten,
OIDC, LDAP, API-Zugängen, Abfragen, Beziehungen, Listen, Dateien, Versionen, Kosten, Übersetzungen,
Formeln, Jobs, Ereignissen mit Verteiler in Stapeln, Webhooks, Volltextsuche, Sicherheitseinstellungen, Dashboards mit Kennzahl, Torte, Balken, Linie, Liste und Text aus gespeicherten Suchen oder eigenen Bedingungen, mit Filtern für Zeitraum und Einheit, als Startseite wählbar, als Kachel im Portal und als Widget in der Seitenleiste eines Datensatzes, gruppiert auch nach einer Rolle und kopierbar als eigenes, Lösungsdatum und Lösungsdauer in Kalender- und Servicezeit an Störung und Anfrage, mitgeliefert „Helpdesk“ und „Statistik“ für den Service Desk, Einstellungen am Organisationsbaum, die die Pakete erklären und der Server prüft (Seite „Globale Einstellungen“ mit einer Leiste zum Speichern; Formeln lesen sie mit `setting("…")`, Fristen können ihr Ziel daraus nehmen, Anhänge zählen an der Einheit der Person),
Bulk und der Zeitleiste am Datensatz (öffentliche und private Einträge). Die Workflow-Engine hat Statusmodell, Feldzustände, Rollen am Datensatz, Regeln,
Kalender, SLA-Uhren mit Eskalationsstufen, Wartegründe, Zeitregeln, Aufgaben mit Summen am Vorgang, wiederkehrende Vorgänge, Genehmigungen in Stufen mit Mindestzahl, Vertretung, Vorlagen (feste Genehmiger und Rollen des Vorgangs) und Erinnerung alle n Arbeitstage (Reiter „Genehmigung“, „Zu genehmigen“ im Arbeitsplatz, Bereich „Genehmigungen“ im Portal) und Benachrichtigungen als Mail und im Posteingang der Person (Glocke oben, Einträge 90 Tage, einstellbar). Die Oberfläche zeigt Liste und Formular aus der Definition (Basis des Pakets und Kundenschicht mit eigenen Feldern `k_…`, die ein Admin im Typ- und Feld-Editor anlegt, umbenennt, ausblendet und verschiebt; im Workflow-Editor ergänzt er Status und Übergänge mit Bedingung, legt Übergänge des Pakets still, wählt den Anfangsstatus und sieht das Ganze als Graph mit React Flow, in dem er Übergänge markiert, Verbindungen zieht und die Anordnung speichert; im Rollen-Editor stellt er je Typ und Aktion die Reichweite ein, schränkt mit Bedingung und Feldrechten ein, vergibt Sonderrechte und weist die Rolle Personen und Gruppen an Knoten zu) und eine Detailseite mit Reitern für Bezüge (verknüpfen, lösen, beide Richtungen), Fassungen (zwei vergleichen, Wort für Wort markiert, eine ältere wiederherstellen) und Zeitleiste, Rollen, SLA-Uhren und Aufgaben; Verweise und Rollen wählt man per Namen; Listen lassen sich filtern, als Suche speichern und teilen und als CSV, XLSX oder ODS exportieren; gewählte Datensätze ändert man in einem Schritt; Einträge und markierte Textfelder sind formatierter Text mit Bildern; Listen und Detailseiten aktualisieren sich live und zeigen, wer gerade am Datensatz ist; Sprache, Zeitzone, hell oder dunkel, eine von sechs Akzentfarben und Vorbelegung stellt jede Person in Meine Einstellungen selbst ein und sie wirken sofort, Vorgaben je Organisationseinheit, das Theme setzt die Organisation; das Aussehen folgt dem Design-Kit „Klartext“ mit der Schrift IBM Plex, die das Werkzeug selbst ausliefert; die Navigation zeigt Bereiche und gespeicherte Suchen, jede Person heftet Einträge mit dem Stern unter „Favoriten“ an und ordnet oder blendet Bereiche in Meine Einstellungen aus, die Befehlszeile (Strg K oder das Suchfeld im Kopf) findet Datensätze über alle Typen, legt neue an, wechselt die Darstellung und führt zu jeder Seite, die Liste hat eine Vorschau des gewählten Datensatzes, kompakte oder normale Zeilen und bei Typen mit SLA eine Spalte „Frist“, nach der man sortiert und filtert (überschritten, knapp, fällig bis). Nutzer ohne Bearbeiterrecht sehen ein schlankes Portal mit einem Band „Aktuell“ (etwa offene Major Incidents und Hinweise von Hand mit Gültigkeit), Kacheln je Zielgruppe zu Formularen, Bereichen, Datensätzen und Links, ihren Vorgängen und den Formularen, die im Baukasten mit Abschnitten, Fragen, Bedingungen und englischen Texten entstehen. Ein Typ kann eine laufende Nummer je Mandant haben (etwa `INC-24817`), die der Kern vergibt und die Suche findet. Der Service Desk beginnt mit dem Grundpaket `sd`: Kategorien als Baum, sichtbar je Organisationseinheit, Anfragequelle, Lösungsart, Wartegründe, der Kalender der Servicezeit und das Merkmal Vorgang mit Feldern und Rollen. Darauf baut die Störung (`incident`) mit betroffenen Elementen (Beziehung „betrifft“), Nummer, Priorität aus Auswirkung und Dringlichkeit (Matrix je Organisationseinheit einstellbar), Bestätigen oder Ablehnen der Lösung im Portal und Fristen für Reaktion und Lösung je Priorität in der Servicezeit, mit Mail an die Teamleitung bei 80 % und automatischem Schließen, und Major Incidents: erklären mit Begründung und Mitteilung an die Anwender, Mail an einen Verteiler, zugehörige Störungen werden mitgelöst. Eine neue Störung geht an die Gruppe, die für ihre Kategorie zuständig ist; vier mitgelieferte Warteschlangen (meine, mein Team, ohne Bearbeiter, SLA-Risiko) sind mit dem Service Desk geteilt; Mails mit der Kennung vorne im Betreff gehen bei Eingang, Antwort, Lösung und Abschluss an den Anforderer und bei seiner Antwort an den Bearbeiter oder die Gruppe; Textbausteine für Antworten und Lösungen wählt man im Editor über „Textbaustein“ (Alt+T), der Server setzt die Werte des Vorgangs ein; Telefon- und Vor-Ort-Notiz sind Formulare des Arbeitsplatzes im Menü „Neu“ der Liste, mit Anforderer per Suche und dem Rückruf als Aufgabe. Wissensartikel (`knowledge`) stehen unter den Kategorien der Störung und sind für alle, nur für Bearbeiter oder nur für bestimmte Gruppen sichtbar; im Portal liest man sie im Bereich „Wissen“ mit Kategorienbaum und Suche. Listen, Suche und Portal zeigen Inhalte in der Sprache der Person, wo es eine Übersetzung gibt; gepflegt wird Deutsch. Ein Artikel kommt erst ins Portal, wenn die Wissensredaktion ihn freigegeben hat („Zur Freigabe vorlegen“, Status „In Prüfung“); ändert man Titel, Text oder „Sichtbar für“ eines veröffentlichten Artikels, wartet die Änderung auf die Freigabe, Leser sehen bis dahin die bisherige Fassung, die Änderung steht Wort für Wort im Reiter „Genehmigung“. Ein Artikel hat einen Autor; ab dem Tag nach „Gültig bis“ ist er im Portal weg, eine tägliche Regel setzt ihn auf Abgelaufen und schreibt dem Autor, 14 Tage vorher erinnert sie ihn. Beim Melden einer Störung schlägt das Portal neben dem Formular passende Artikel vor; „Das hat geholfen“ zählt und legt nichts an. An der Störung stehen sie im Reiter „Wissen“ zum Verknüpfen und „Als Lösung übernehmen“; Leser sagen, ob ein Artikel geholfen hat, und kommentieren ihn. Bedingungen kennen dafür `today()` und `now()`. Pakete bringen solche Startdaten (Kalender, Fristen, Eskalationen) mit; sie entstehen je Mandant einmal und gehören danach dem Kunden. Die Anfrage (Paket `request`, REQ-…) nennt eine Erfüllungsvorlage: deren Genehmigungsvorlage wird angefordert, nach der Zustimmung entstehen ihre Aufgaben für die Gruppen; im Portal steht dafür die Kachel „Etwas anfragen“. Der Servicekatalog (Paket `catalog`) zeigt im Portal unter „Katalog“ die Angebote, die eine Person sehen darf, nach Bereichen und als Karten mit Lieferzeit und Kosten; bestellt wird auf der Seite des Angebots mit dessen Formular, daraus wird eine Anfrage mit der Vorlage des Angebots, deren Erfüllungsfrist die Lieferzeit ist.

*Work in progress (details in the main plan): the platform core, the first part of the workflow
engine and list and form in the web interface are built; no service desk yet.*

## Starten / Run

Alle Befehle in WSL im Verzeichnis des Repositorys; auf dem Rechner braucht es nur Docker
(Hauptplan E52). *All commands from the repository root; only Docker is required.*

```bash
docker compose up -d --build
```

Danach antworten / then these answer:

- http://localhost:8092/ — die Oberfläche / the web interface
- http://localhost:8092/readyz — Datenbank und Valkey / database and Valkey
- http://localhost:8092/api/v1/version — die Version / the version

Anmelden (PL5): Alles unter `/api/v1` außer `version` und `auth/login` braucht eine Sitzung.
Ein lokales Konto legt `itsm user create` an, das Kennwort kommt auf stdin (mindestens 8
Zeichen mit Groß-, Kleinbuchstabe, Ziffer und Sonderzeichen); schreibende Aufrufe tragen den
Header `X-Requested-With: itsm`. *Sign in: everything under `/api/v1` except `version` and
`auth/login` needs a session; create a local account with the password on stdin.*

```bash
docker compose run --rm tools go run ./cmd/itsm package install packages/core
```

```bash
docker compose run --rm tools go run ./cmd/itsm package install packages/probe
```

```bash
printf 'Secret-1!' | docker compose run --rm -T itsm user create pascal --display-name "Pascal"
```

Ohne Rolle sieht ein Konto nichts (PL8); die Rolle `admin` darf alles:

```bash
docker compose run --rm itsm user grant pascal admin
```

Mandanten (PL44): Ohne Mandanten ist die Wurzel der einzige. Der erste übernimmt mit `--adopt`
alles Bestehende; die mit `--admin` genannten Konten bleiben Konten der Installation und wechseln
den Mandanten im Kopf der Oberfläche. Die Übernahme lässt sich nicht zurücknehmen. *Tenants: the
first one adopts what is there (for good); named administrators stay with the installation and
switch tenants in the header.*

Eindeutige Felder und externe Schlüssel gelten je Mandant (PL46). Ein neuer Datensatz landet im
Knoten des Rechtekontexts, der Person oder des einzigen Anlegerechts; bleibt eine Wahl, fragt das
Formular „Ablegen in“. *Unique fields and external keys are unique per tenant; a new record lands
in the node of the context, the person or the one right, else the form asks.*

```bash
docker compose run --rm itsm tenant create nord --label "Polizei Nord" --adopt --admin pascal
```

```bash
docker compose run --rm itsm tenant create sued --label "Polizei Süd"
```

Datenbank je Mandant (PL47d): Mit `ITSM_TENANT_MODE=database` liegt jeder Mandant in seiner
eigenen Datenbank; die Steuer-Datenbank (`ITSM_DATABASE_URL`) kennt ihn nur beim Namen. Der
Betrieb legt die leere Datenbank samt Benutzer an und trägt ihre URL ein; `tenant create` richtet
sie ein (Migrationen, Tabellen aller Pakete, Auswahllisten, Startdaten) und trägt den Mandanten zuletzt ins
Verzeichnis. Danach die Instanzen neu starten. `migrate` und `package install` wirken auf jede
Datenbank. Gesichert wird je Datenbank mit `pg_dump`, wiederhergestellt mit `pg_restore` in eine
neue, leere Datenbank, auf die dann die URL zeigt; `tenant check` prüft danach, ob sie zur
Steuer-Datenbank passt, und nennt, was fehlt (etwa Konten, die nach der Sicherung angelegt
wurden). *Database per tenant: the operator creates the empty database and sets its URL; tenant
create sets it up; back up and restore per database with pg_dump and pg_restore, then tenant
check.*

Auswertungen über Mandanten (PL48): `POST /api/v1/tenants:query` mit Typ, Mandanten und
JSON-Abfrage liefert die Zeilen jedes Mandanten mit der Spalte `tenant`. Dafür braucht ein Konto
der Installation das Recht `report_tenants` an `core.tenant`, ausdrücklich in einer Rolle an der
Wurzel; die Rolle admin allein reicht nicht. *Evaluations across tenants need the right
report_tenants on core.tenant, named explicitly in a role at the root.*

```bash
ITSM_TENANT_MODE=database ITSM_TENANT_NORD_DATABASE_URL=postgres://nord:…@db/itsm_nord docker compose run --rm itsm tenant create nord --label "Polizei Nord"
```

```bash
pg_dump --format=custom --file=nord.dump postgres://nord:…@db/itsm_nord
```

```bash
pg_restore --no-owner --exit-on-error --dbname=postgres://nord:…@db/itsm_nord_neu nord.dump
```

```bash
ITSM_TENANT_MODE=database ITSM_TENANT_NORD_DATABASE_URL=postgres://nord:…@db/itsm_nord_neu docker compose run --rm itsm tenant check nord
```

```bash
curl -s -c cookies.txt -H 'Content-Type: application/json' -H 'X-Requested-With: itsm' \
  -d '{"username":"pascal","password":"Secret-1!"}' http://localhost:8092/api/v1/auth/login
```

```bash
curl -s -b cookies.txt http://localhost:8092/api/v1/auth/me
```

Bauen, prüfen und testen im Werkzeug-Container / build, lint and test in the tool container:

```bash
scripts/ci-run.sh
```

Während der Arbeit reicht die Teil-CI: nur die Go-Pakete, die sich seit dem letzten Commit
geändert haben, und alle, die sie nutzen, Vitest für die geänderten Quellen, die Browser-Tests
des Bereichs, keine Lastmessung. Vor jedem Commit läuft die ganze CI; ihre Lastmessung nur,
wenn sich Go-Code des Servers, Migrationen oder das Lastwerkzeug geändert haben (`--all`
misst trotzdem). *While working, `--part` checks what changed and what depends on it; the
whole CI runs before every commit.*

```bash
scripts/ci-run.sh --part
```

Der Lauf bekommt einen eigenen PostgreSQL und Valkey (Profil `ci`, Daten im Arbeitsspeicher),
die danach entfernt werden: jeder Lauf beginnt mit demselben Stand, die Instanz auf 8092 bleibt
unberührt. Die CI bricht nicht beim ersten Fehler ab; am Ende stehen alle Schritte mit Ergebnis
und die Fehler. *Every run gets a fresh PostgreSQL and Valkey of its own, removed afterwards;
all steps run, the summary lists every failure.* Einzelne Tests von Hand brauchen die Server:
`docker compose --profile ci up -d --wait postgres-ci valkey-ci`, danach
`docker compose run --rm tools go test ./internal/events/`.

Mails der Benachrichtigungen ansehen (Mailpit, Profil `mail`) / see the mails of the
notifications:

```bash
docker compose --profile mail up -d
```

- http://localhost:8026/ — Mailpit (8025 ist der Mailpit der GLPI-Umgebung)

Leistung messen (Hauptplan H11): Die CI misst bei jedem Lauf den neuen Stand im Wechsel mit
dem freigegebenen Basis-Binary aus dem Volume `load-baseline`; `test/load/baseline.json` ist das
Protokoll der Freigabe. Ist der Rechner ausgelastet, bricht die Messung ab (Rückgabe 3). Die volle
Stufe und der Datensatz in Prod-Größe:

```bash
docker compose run --rm tools go run ./test/load generate --size=prod --out=/tmp/data
```

Die Messung in Prod-Größe (H14) lädt 70.000 Personen und 18,25 Mio. Störungen über die Bulk-API
einer laufenden Instanz und setzt die Geschichte direkt in deren Datenbank; danach misst die
volle Stufe auf diesem Bestand (`--only=LZ2,LZ4` für einzelne Ziele) und räumt die angelegten
Störungen wieder ab. Die Instanz braucht so viele Bulk-Worker (`ITSM_WORKERS_BULK`) wie
`--parallel`:

```bash
go run ./test/load fill --size=prod --target=http://itsm:8080 --database=postgres://… --chunk=500 --parallel=8
```

```bash
go run ./test/load run --stage=full --data=prod --target=http://itsm:8080 --out=full.json
```

## Aufbau / Layout

| Pfad | Inhalt |
| --- | --- |
| `cmd/itsm/` | das eine Binary mit `serve`, `migrate`, `package`, `user`, `ldap`, `client`, `calendar`, `healthcheck`, `version` (`itsm` ohne Argument zeigt alle) |
| `internal/` | die Bausteine aus `docs/architektur.md` §2 |
| `db/migrations/`, `db/queries/` | Migrationen (`goose`) und Abfragen (`sqlc`) der Kerntabellen |
| `web/` | die Oberfläche (React, TypeScript, Vite) |
| `tools/` | der Werkzeug-Container |
| `scripts/` | CI und Prüfskripte |
| `test/load/` | Lastwerkzeug, Datensatzgenerator, Basislinie |
| `docs/` | Hauptplan, Architektur, Entscheidungen (`docs/adr/`) |

## Konfiguration / Configuration

Nur über Umgebungsvariablen / environment variables only:

| Variable | Pflicht | Vorgabe | Zweck |
| --- | --- | --- | --- |
| `ITSM_DATABASE_URL` | ja | — | PostgreSQL; mit eigenen Datenbanken je Mandant die Steuer-Datenbank |
| `ITSM_TENANT_MODE` | nein | `shared` | `shared`: alle Mandanten in einer Datenbank; `database`: jeder Mandant in seiner eigenen (PL47d, Hauptplan E144) |
| `ITSM_TENANT_<NAME>_DATABASE_URL` | mit `database` | — | Datenbank des Mandanten `<name>`, z. B. `ITSM_TENANT_NORD_DATABASE_URL`; fehlt sie für einen Mandanten, startet itsm nicht |
| `ITSM_VALKEY_ADDR` | ja | — | Valkey, `host:port` |
| `ITSM_ROLE` | nein | `all` | `api`, `worker` oder `all` |
| `ITSM_HTTP_ADDR` | nein | `:8080` | Adresse des HTTP-Servers |
| `ITSM_WEB_DIR` | nein | leer | Verzeichnis der gebauten Oberfläche |
| `ITSM_FILES_DIR` | nein | leer, im Image `/data/files` | Ablage der Dateien (PL18) und der NDJSON-Ströme des asynchronen Bulk (PL32); leer schaltet beides ab (503) |
| `ITSM_LOG_LEVEL` | nein | `info` | `debug`, `info`, `warn`, `error` |
| `ITSM_SHUTDOWN_TIMEOUT` | nein | `15s` | Zeit zum geordneten Beenden |
| `ITSM_REQUEST_TIMEOUT` | nein | `10s` | Zeitgrenze einer API-Anfrage samt ihrer Abfragen in PostgreSQL (PL56); `0` für keine; Export, Bulk-Strom, Ereignisstrom und Hochladen ausgenommen |
| `ITSM_WORKERS_INTERACTIVE` | nein | `8` | Worker je Instanz für die Warteschlange `interactive` (Rolle `worker`) |
| `ITSM_WORKERS_DEFAULT` | nein | `8` | Worker je Instanz für `default` |
| `ITSM_WORKERS_BULK` | nein | `2` | Worker je Instanz für `bulk`; `0` lässt eine Warteschlange auf dieser Instanz liegen |
| `ITSM_BULK_QUEUE` | nein | `bulk` | Warteschlange des asynchronen Bulk (PL32); die Laststufe der CI nimmt `bulkload`, damit eine andere Instanz auf derselben Datenbank ihre Jobs nicht nimmt |
| `ITSM_EVENT_RETENTION` | nein | `168h` | Aufbewahrung der Ereignisse (Hauptplan E46), nächtlicher Job `event.retention` |
| `ITSM_PUBLIC_URL` | nein | `http://localhost:8080` | Adresse des Servers aus Sicht des Browsers (Rückweg vom Identitätsanbieter) |
| `ITSM_OIDC_ISSUER` | nein | leer | OpenID-Connect-Anbieter (Aussteller); leer = keine Anmeldung über einen Anbieter |
| `ITSM_OIDC_CLIENT_ID` | mit Issuer | — | Client-ID beim Anbieter |
| `ITSM_OIDC_CLIENT_SECRET` | nein | leer | Client-Geheimnis |
| `ITSM_OIDC_NAME` | nein | `SSO` | Beschriftung des Knopfs auf der Anmeldeseite |
| `ITSM_OIDC_INTERNAL_URL` | nein | leer | Anbieter aus Sicht des Containers, wenn anders als für den Browser (Compose: `http://keycloak:8080`) |
| `ITSM_OIDC_SCOPES` | nein | `openid profile email` | Scopes, durch Leerzeichen getrennt |
| `ITSM_OIDC_GROUPS_CLAIM` | nein | `groups` | Name des Claims mit den Gruppen (nur für das Protokoll; Regeln sehen alle Claims) |
| `ITSM_SMTP_HOST` | nein | leer | Mailserver der Benachrichtigungen (WF15); leer = keine Mail, der Posteingang füllt sich trotzdem (WF19). Compose: `mailpit` (Profil `mail`) |
| `ITSM_SMTP_PORT` | nein | `25` | Port des Mailservers (Mailpit: `1025`) |
| `ITSM_SMTP_FROM` | mit Host | — | Absender, etwa `ITSM <itsm@example.org>` |
| `ITSM_SMTP_USER` | nein | leer | Anmeldung am Mailserver; nur mit STARTTLS |
| `ITSM_SMTP_PASSWORD` | nein | leer | Kennwort dazu |
| `ITSM_SMTP_STARTTLS` | nein | `false` | `true` verschlüsselt die Verbindung vor der Anmeldung |
| `ITSM_TRUST_PROXY` | nein | `false` | `true` hinter einem Reverse-Proxy: die Adresse des Aufrufers kommt aus `X-Forwarded-For` (IP-Bereiche der API-Zugänge) |
| `ITSM_LDAP_PASSWORD_<NAME>` | je Verzeichnis | — | Bind-Kennwort des LDAP-Verzeichnisses `<name>` (Großbuchstaben, `-` als `_`); die Verzeichnisse selbst stehen unter `/api/v1/directories` |

## Lizenz / License

Copyright (C) 2026 group24 AG. AGPL-3.0-only, siehe / see [LICENSE](LICENSE). Beiträge nur mit
Rechteübertragung an die group24 AG (CLA). *Contributions require a CLA with group24 AG.*

Abhängigkeiten, die itsm ausliefert, haben nur permissive Lizenzen oder MPL-2.0; die CI prüft
das in jedem Lauf gegen `test/licenses/licenses.txt` (Hauptplan E104). *Shipped dependencies
are permissively licensed or MPL-2.0, checked by the CI.*
