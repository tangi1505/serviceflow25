# Standard: Wie wir Pläne erstellen und abarbeiten

**Stand:** 2026-10-20. Fassung für das Arbeitsverzeichnis `C:\Users\PascalFrechen\Polizei\itsm`,
übernommen aus `..\glpi\docs\planungsstandard.md` (Stand 2026-10-01, Entwurf). §1 bis §9 sind
wortgleich mit der Fassung in `glpi`, bis auf den Pfad in §1; sie werden nur als Ganzes
übernommen, nie einzeln geändert (dort P12). §10 gilt nur hier. Die offenen Punkte zum
Verfeinern (dort §11) werden in `glpi` geklärt und dann hierher übernommen.

> **Der Kern in vier Sätzen:** Ein Hauptplan je Vorhaben führt die Nebenpläne, und jede Aufgabe
> steht in genau einem Plan. Jeder Plan beginnt mit der Aufgabenliste, jede Aufgabe hat einen
> Prüfweg. Die Umsetzung erledigt eine Aufgabe nach der anderen, trägt den Stand mit Belegen ein und
> sagt, wie geprüft wird. Erst die Rückmeldung der Abnahme macht aus „fertig" ein „abgenommen".

---

## Aufgaben

**Status:** ✅ abgenommen · 🟢 fertig, wartet auf die Abnahme · 🔨 in Arbeit · ⬜ offen ·
🔒 blockiert

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **S1** | Fassung aus `glpi` übernehmen, §10 für dieses Verzeichnis schreiben | 🟢 | — | `diff` der Abschnitte §1 bis §9 gegen `..\glpi\docs\planungsstandard.md` zeigt nur den Pfad in §1 |
| **S2** | §10 vervollständigen, sobald Hauptplan V5 (Repository) und V15 (Entwicklungsumgebung) beantwortet sind | 🟢 | 0,25 d | §10 nennt, wo die Arbeitskopie liegt, wer committet und wie Prüfbefehle laufen |
| **S3** | Den Standard in die `CLAUDE.md` dieses Verzeichnisses übernehmen (Hauptplan H5) | 🟢 | 0,25 d | `CLAUDE.md` im Wurzelverzeichnis enthält §1 bis §10 |

---

## 1. Wozu, wofür, wer

- **Wozu:** Jederzeit sehen, was zu tun ist, was fertig ist und woran man das erkennt. „Fertig"
  soll geprüft sein, nicht behauptet.
- **Wofür:** jedes Vorhaben mit mehr als einer Aufgabe, also Eigenbau, Analyse, Einführung oder
  Umstellung im Betrieb. Eine Frage, die sich in einer Antwort erledigt, braucht keinen Plan.
- **Geltung: je Arbeitsverzeichnis** (entschieden am 2026-09-30). Der Standard gilt für alle Pläne
  in dem Verzeichnis, in dem er liegt, hier `docs/planungsstandard.md` in
  `C:\Users\PascalFrechen\Polizei\itsm`. Jedes Verzeichnis hat seine eigene Fassung: §1 bis §9 sind
  überall gleich, §10 enthält die Regeln des Verzeichnisses.
- **Bestehende Vorhaben** (entschieden am 2026-09-30): Ihre Pläne bekommen das Nummernschema und
  die Aufgabenliste nach §4 und §5, jeder mit einer Zuordnungstabelle alt → neu. Ihr Code bleibt,
  wie er ist: Kommentare und Tests behalten die alten Nummern, die Zuordnungstabelle löst sie auf.
  Darum behalten auch die Abschnitte der Pläne ihre Nummern: Ein „§4" im Code würde nach einer
  Umnummerierung still auf einen anderen Abschnitt zeigen. Ganz nach dem Standard, im Aufbau (§3)
  wie im Code, arbeiten nur neue Projekte.
- **Rollen:**

| Rolle | Heute | Tut |
| --- | --- | --- |
| **Umsetzung** | Claude | analysiert, baut, dokumentiert, prüft selbst vor, trägt den Stand ein |
| **Abnahme** | Pascal | prüft und nimmt ab, entscheidet, überträgt, committet und pusht |
| **Fachseite** | der Kunde | entscheidet fachlich, beantwortet Fragen |
| **Betrieb** | die Kollegen | spielt auf der Prod ein |

---

## 2. Welche Dokumente es gibt

| Art | Inhalt | Beispiel |
| --- | --- | --- |
| **Hauptplan** | einer je Vorhaben: Ziel, Etappen, die Nebenpläne mit ihrem Stand, übergreifende Entscheidungen, alle Fragen an die Fachseite, Aufgaben ohne eigenen Nebenplan | `0_maintask.md` |
| **Nebenplan** | ein Arbeitspaket mit eigenem Ergebnis: ein Eigenbau, ein Test, eine Einführung | `01_schnittstellen.md` |
| **Unterplan** | ein Teil eines Nebenplans, der selbst ein Arbeitspaket ist | `01_schnittstellen/011_….md` |
| **Bewertung** | Befunde mit Belegen: was ein System kann und was nicht | `anforderungen-ausserhalb-der-plugins.md`, `bewertung-*.md` |
| **Betrieb** | wie man installiert, einspielt, prüft und bedient | `setup-und-produktivbetrieb.md`, `api-zugriff.md` |

- **Befunde stehen in der Bewertung, der Plan verweist darauf.** Keine Aussage steht in zwei
  Dokumenten.
- **Kein Befund bleibt nur im Chat.** Was bei der Arbeit auffällt, wird eine neue Aufgabe, eine
  neue Frage oder ein Punkt in der Bewertung.
- **Neue Dokumente nur nach Absprache.** Lieber das passende vorhandene erweitern.
- **Was nicht mehr gepflegt wird,** liegt unter `docs/legacy/`: abgeschlossen, überholt oder
  ruhend. Verweise darauf zeigen auf den neuen Pfad.

### 2.1 Hauptplan und Nebenpläne

- **Ein Hauptplan je Vorhaben.** Er ist der Einstieg: Wer wissen will, wo das Vorhaben steht und
  was als Nächstes kommt, liest ihn zuerst.
- **Jede Aufgabe steht in genau einem Plan.** Der Hauptplan wiederholt keine Aufgaben der
  Nebenpläne. Er nennt ihren Stand im Ganzen und verweist auf sie.
- **Ein Nebenplan je Arbeitspaket,** das ein eigenes Ergebnis hat und sich für sich abschließen
  lässt. Faustregel: ab drei Aufgaben oder drei Tagen. Kleineres bleibt als Aufgabe im Hauptplan.
- **Drei Ebenen, nicht mehr:** Hauptplan, Nebenplan, Unterplan. Ein Nebenplan bekommt Unterpläne,
  wenn ein Teil von ihm selbst ein Arbeitspaket ist (dieselbe Faustregel). Ein Unterplan hat keine
  eigenen; wird er zu groß, wird er geteilt. Blöcke innerhalb eines Plans sind erlaubt (§4).
- **Ein Nebenplan mit Unterplänen** führt deren Übersicht so, wie der Hauptplan die Nebenpläne
  führt. Der Hauptplan nennt Unterpläne nicht einzeln. Die Regeln dieses Abschnitts gelten zwischen
  Nebenplan und Unterplan entsprechend; Fragen an die Fachseite sammelt trotzdem der Hauptplan.
- **Was der Hauptplan führt:**
  - die Übersicht der Nebenpläne mit Kurzzeichen, Ergebnis, Etappe und Stand;
  - die Etappen und die Reihenfolge über alle Pläne;
  - Entscheidungen, die mehr als einen Nebenplan betreffen;
  - die Fragen an die Fachseite, damit eine Runde mit dem Kunden sie auf einen Blick hat. Fragen,
    die nur das Innere eines Eigenbaus betreffen, bleiben in dessen Plan; ebenso die Fachfragen, die
    schon in einer Bewertung stehen. Der Hauptplan nennt, wo sie stehen;
  - was alle Nebenpläne brauchen, etwa Zielbild und Mengengerüst;
  - Aufgaben, die keinem Nebenplan gehören.
- **Was ein Nebenplan führt:** seine Aufgaben, seine Entscheidungen, die Fragen, die die Umsetzung
  selbst klärt, die Begründung und den Umsetzungsstand. Befunde stehen weiter in der Bewertung.
- **Verweise über Pläne hinweg** mit der Nummer: `🔒 SI8`. Weil das Kurzzeichen dazugehört, ist jede
  Aufgabennummer über alle Pläne eindeutig. `E` und `V` gelten je Plan und werden mit dem Plan
  genannt: „Hauptplan V17".
- **Der Stand im Hauptplan** ist ein Zeichen für den ganzen Nebenplan und ein paar Worte. Der
  Hauptplan wird angefasst, wenn ein Nebenplan beginnt, wartet, fertig wird oder ruht, wenn sich
  eine Abhängigkeit zwischen Plänen löst und wenn sich die Reihenfolge ändert. Den Stand einzelner
  Aufgaben führt nur der Nebenplan.

| Zeichen | Stand eines Plans |
| --- | --- |
| ⬜ | nicht begonnen |
| 🔨 | läuft |
| 🔒 | wartet; dahinter steht, worauf |
| ✅ | abgeschlossen: jede Aufgabe ist ✅ oder entfallen |
| 💤 | ruht; der Plan liegt unter `docs/legacy/` |

- **Ein neuer Nebenplan** entsteht aus einer Aufgabe im Hauptplan („… als Plan ausarbeiten") oder
  auf Zuruf. Er bekommt ein Kurzzeichen (§10) und eine Zeile in der Übersicht.
- **Wandert eine Aufgabe in einen Nebenplan,** bekommt sie dort die nächste freie Nummer und den
  Vermerk „War Z16". Im Hauptplan verlässt sie die Liste und steht in der Tabelle „Verschoben" mit
  alter und neuer Nummer. Die alte Nummer wird nicht neu vergeben. Wandert ein Abschnitt mit, bleibt
  im Hauptplan seine Überschrift mit einem Verweis stehen, damit seine Nummer gültig bleibt (P14).

### 2.2 Dateinamen

Der Name eines Plans zeigt seine Stelle (entschieden am 2026-10-01, P16):

| Ebene | Name | Ort | Beispiel |
| --- | --- | --- | --- |
| Hauptplan | `0_maintask.md` | `docs/` | `docs/0_maintask.md` |
| Nebenplan | `0` + laufende Nummer + `_` + Name | `docs/` | `docs/01_schnittstellen.md` |
| Unterplan | Nummer des Nebenplans + laufende Nummer + `_` + Name | ein Ordner mit dem Namen des Nebenplans | `docs/01_schnittstellen/011_massentest.md` |

- **Eine Ziffer je Ebene,** also höchstens neun Pläne je Ebene. Der Nebenplan selbst bleibt in
  `docs/`, nur seine Unterpläne liegen im Ordner.
- **Der Name** nach der Nummer: Kleinbuchstaben, Wörter mit Bindestrich, ohne Umlaute und
  Leerzeichen.
- **Eine Nummer wird nie neu vergeben.** Ein ruhender Plan behält sie unter `docs/legacy/`, samt
  seinem Ordner, etwa `legacy/07_mcp.md`. Ein neuer Plan bekommt die nächste freie Nummer.
- **Was kein Plan ist,** also Bewertung, Betrieb und dieser Standard, behält einen sprechenden Namen
  ohne Nummer.
- **Dateiname und Kurzzeichen:** Der Dateiname zeigt die Stelle, das Kurzzeichen (§10) steht vor den
  Aufgabennummern. Beides bleibt, auch wenn ein Plan nach `legacy/` umzieht.
- **Links aus einem Ordner** gehen mit `../` zurück, etwa `[Hauptplan](../0_maintask.md)`.
- **Umbenennen** heißt: alle Verweise unter `docs/` mitziehen und die alte Datei überall löschen,
  wo die Arbeitskopie gespiegelt wird (§10).

---

## 3. Aufbau eines Plans

In dieser Reihenfolge; die Vorlage in §9 zeigt das Gerüst.

### 3.1 Hauptplan

1. **Kopf:** Titel `# Hauptplan: …`, `**Stand:** YYYY-MM-DD`, das Ziel in einem Satz. Wird der
   Plan neu geordnet, steht im Kopf, wann und warum.
2. **Nebenpläne:** die Übersicht (§2.1), darunter **Etappen** und **Reihenfolge über alle Pläne**.
3. **Aufgaben** des Hauptplans (§4), darunter die Tabelle **Verschoben**, falls Aufgaben in
   Nebenpläne gewandert sind.
4. **Entscheidungen, Begründung, Offene Fragen und Umsetzungsstand** wie im Nebenplan (§3.2). Die
   offenen Fragen umfassen die Fragen an die Fachseite (§2.1).

### 3.2 Nebenplan

1. **Kopf:**
   - Titel `# Plan: …`;
   - `**Stand:** YYYY-MM-DD`, dazu gegen welche Fassung geprüft wurde;
   - „Teil des Hauptplans", mit Etappe;
   - die Verweise auf die Bewertungen;
   - wurde der Plan aus dem Hauptplan herausgelöst oder neu geordnet, steht im Kopf, wann und
     welche Nummern;
   - auf Wunsch ein Kasten mit der Kernaussage oder der tragenden Entscheidung.
2. **Aufgabenliste**, vor allem anderen (§4). Darunter ein Absatz **Reihenfolge**: was zuerst kommt
   und warum. Hat der Plan Unterpläne, steht ihre Übersicht davor, wie im Hauptplan (§3.1, Punkt 2).
   Ein Unterplan nennt im Kopf statt des Hauptplans seinen Nebenplan.
3. **Entscheidungen** als Tabelle: Nummer, Entscheidung, seit wann, von wem oder woher.
4. **Begründung und Befunde** in nummerierten Abschnitten, möglichst als Verweis auf die Bewertung.
5. **Offene Fragen** als Tabelle: Nummer mit Gewicht, Frage und woran sie hängt. Beantwortete
   Fragen fallen heraus und stehen darunter in einer Zeile „Beantwortet am …". Fragen an die
   Fachseite stehen im Hauptplan, hier nur der Verweis darauf; Ausnahmen in §2.1.
6. **Umsetzungsstand**: je erledigter Aufgabe ein Abschnitt (§6, Schritt 3).

---

## 4. Die Aufgabenliste

| Spalte | Inhalt |
| --- | --- |
| **#** | Kurzzeichen des Plans und Nummer, fett: `**Z3**`, Unteraufgaben mit Punkt: `**Z3.1**`. Eine Nummer wird nie ein zweites Mal vergeben |
| **Aufgabe** | was getan wird, in einem Satz, mit Verweis auf den Abschnitt mit der Begründung |
| **Status** | ein Zeichen aus §5; bei 🔒 steht dahinter, worauf gewartet wird |
| **Aufwand** | grob in Tagen, in Schritten von 0,25 d; dazu, wer außer der Umsetzung beteiligt ist |
| **So prüfst du es** | der Prüfweg: Befehl oder Klickweg und das erwartete Ergebnis |

- **Diese fünf Spalten hat jeder Plan** (entschieden am 2026-09-30). „Art" (Bau, Prüfung,
  Entscheidung, Doku) und „Wer" kommen nur dazu, wenn ein Plan sie braucht, etwa weil die
  Fachseite Aufgaben übernimmt.
- **Aufgaben so schneiden, dass jede am Stück übertragbar und prüfbar ist.** Lieber M3.1, M3.2 und
  M3.3 als ein M3 über acht Tage. Vorschlag: höchstens zwei Tage je Aufgabe (P10).
- **Ein Prüfweg ist konkret:** ein Befehl, den die Abnahme so ausführen kann, oder ein Klickweg,
  dazu das Ergebnis, das kommen muss. „Funktioniert" ist kein Prüfweg.
- **Blöcke** bei größeren Plänen als Zwischenüberschriften: vor der Freigabe, Werkzeug, nächste
  Fassung, Entscheidungen (laufen parallel), Erledigt. Innerhalb eines Blocks gilt die Reihenfolge
  von oben nach unten. Die Nummer trägt den Block nicht.
- **Weitere Nummern:** `E` für Entscheidungen, `V` für offene Fragen. Eigene Listen, etwa `L` für
  Lücken, nur wenn der Plan sie braucht. `E`- und `V`-Nummern gelten je Plan; wer auf einen anderen
  Plan verweist, nennt ihn mit: „Hauptplan V15". Aufgabennummern sind über alle Pläne eindeutig
  (§2.1).
- **Kommt eine Aufgabe dazu,** bekommt sie die nächste freie Nummer, auch wenn sie inhaltlich weiter
  oben stünde. Die Reihenfolge regelt der Absatz unter der Tabelle.
- **Entfällt eine Aufgabe,** wird sie durchgestrichen, nicht gelöscht:
  `~~**Z4**~~ | ~~Aufgabe~~ — **entfällt am YYYY-MM-DD:** Grund`.
- **Wandert eine Aufgabe in einen anderen Plan,** gilt §2.1.

---

## 5. Status und Gewicht

Zwei getrennte Skalen: der Status sagt, wie weit eine Aufgabe ist, das Gewicht, wie dringend ein
Befund oder eine Frage ist. Den Stand eines ganzen Plans regelt §2.1.

**Status einer Aufgabe:**

| Zeichen | Bedeutung | Wer setzt es |
| --- | --- | --- |
| ⬜ | offen | Umsetzung |
| 🔨 | in Arbeit | Umsetzung |
| 🟢 | fertig und selbst geprüft, wartet auf die Abnahme | Umsetzung |
| ✅ | abgenommen | Umsetzung, **nur nach Rückmeldung der Abnahme** |
| 🔒 | blockiert; dahinter steht, worauf gewartet wird, etwa `🔒 V15` oder `🔒 SI8, GA8` | Umsetzung |

- **🟢 ist eine Behauptung, ✅ eine Tatsache.** Beim ersten Eigenbau hielt „fertig" der Prüfung
  nicht stand: Die Prüfung fand acht leere Testdateien und zwei Fehler in der Rechtevergabe
  (`assetimport` M0).
- **Fällt die Prüfung durch,** geht die Aufgabe auf 🔨 zurück, und der Befund kommt in den
  Umsetzungsstand.

**Gewicht eines Befunds oder einer Frage:**

| Zeichen | Bedeutung |
| --- | --- |
| 🔴 | blockiert oder gefährdet die Freigabe |
| 🟠 | muss vor dem nächsten Schritt geklärt sein |
| 🟡 | wichtig, blockiert aber nichts |

---

## 6. Arbeitsablauf je Aufgabe

1. **Eine Aufgabe** auf 🔨 setzen, nie mehrere zugleich, auch nicht über mehrere Pläne.
2. **Erledigen und selbst prüfen**, bevor sie 🟢 wird: Tests und Prüfskript grün, Zeichensatz und
   Zeilenenden geprüft, Belege nachgesehen (§7).
3. **Stand eintragen**, sofort und nicht erst am Ende:
   - den Status in der Liste;
   - im Umsetzungsstand einen Abschnitt `### <Nr> — <was>, YYYY-MM-DD` mit **Grundlage**,
     **Belegt** und **Nicht geprüft**;
   - neue Befunde als neue Aufgabe, neue Frage oder Punkt in der Bewertung;
   - im Hauptplan nur, was §2.1 verlangt.
4. **Der Abnahme sagen, wie sie prüft:** zwei Sätze, der Befehl und die Liste der geänderten
   Dateien.
5. **Die Abnahme** überträgt, prüft und meldet das Ergebnis.
6. **Erst dann ✅**, oder zurück auf 🔨 mit dem Befund.

**Fragen an die Abnahme** stellt die Umsetzung nur, wenn die Entscheidung wirklich dort oder beim
Kunden liegt, und immer mit Empfehlung. Was sich aus Plan, Code oder einer sinnvollen Vorgabe
ergibt, entscheidet sie selbst und sagt es.

---

## 7. Belege und Ehrlichkeit

- **Jede Aussage über Code** hat eine Fundstelle `Datei:Zeile` auf einem genannten Stand (Branch
  oder Tag), etwa `src/Glpi/Inventory/Conf.php:761` auf `11.0/bugfixes`.
- **Gemessen, gelesen und angenommen** sind drei verschiedene Dinge und werden so benannt.
  „Gemessen" heißt: ausgeführt und die Zahlen gesehen.
- **Quellen aus dem Netz** mit Link; Angaben des Herstellers gehen vor Angaben Dritter. Was aus einer
  Recherche kommt, wird in Stichproben am Original nachgesehen, und der Umsetzungsstand sagt, bei
  welchen.
- **Jeder Umsetzungsstand hat „Nicht geprüft".** Lieber eine ehrliche Lücke als ein stilles „passt".
- **Geänderte Entscheidungen** lassen die alte Fassung sichtbar: „geändert am …; vorher: …".

---

## 8. Schreibregeln

- Deutsch, kurze Sätze, Stichpunkte wo sie helfen, Tabellen für Listen gleicher Art.
- Datum immer als `YYYY-MM-DD`.
- Keine Änderungsprotokolle im Text. Was sich wann geändert hat, zeigen Stand-Zeile,
  Umsetzungsstand und Git.
- Querverweise statt Kopien.
- Befehle so, wie die Abnahme sie ausführt, ein Befehl je Block.

---

## 9. Vorlage

### 9.1 Hauptplan

Datei `docs/0_maintask.md`:

````markdown
# Hauptplan: <Vorhaben>

**Stand:** YYYY-MM-DD.

> **Das Ziel in einem Satz:** …

---

## Nebenpläne

**Stand eines Plans:** ✅ abgeschlossen · 🔨 läuft · ⬜ nicht begonnen · 🔒 wartet · 💤 ruht

| Nr | Kürzel | Nebenplan | Ergebnis | Etappe | Stand |
| --- | --- | --- | --- | --- | --- |
| 01 | **XY** | [<Name>](01_<name>.md) | … | 1 | ⬜ |

**Etappen:**

1. **<Etappe>** — …

**Reihenfolge über alle Pläne:** …

---

## Aufgaben

**Status:** ✅ abgenommen · 🟢 fertig, wartet auf die Abnahme · 🔨 in Arbeit · ⬜ offen ·
🔒 blockiert

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **H1** | … (§2) | ⬜ | 0,5 d | `<Befehl>` → <erwartetes Ergebnis> |

**Verschoben am YYYY-MM-DD** in die Nebenpläne:

| Alt | Neu | Nebenplan |
| --- | --- | --- |
| H2 | XY1 | [<Name>](01_<name>.md) |

**Reihenfolge:** …

---

## 1. Entscheidungen

## 2. <Zielbild, Mengengerüst, was alle Nebenpläne brauchen>

## 3. Offene Fragen

(die Fragen an die Fachseite; dazu, wo die übrigen stehen)

## 4. Umsetzungsstand
````

### 9.2 Nebenplan

Datei `docs/0<N>_<name>.md`. Ein Unterplan hat dasselbe Gerüst, liegt in
`docs/0<N>_<name>/0<N><M>_<name>.md`, nennt im Kopf „Teil von [<Nebenplan>](../0<N>_<name>.md)" und
verlinkt mit `../`.

````markdown
# Plan: <Arbeitspaket>

**Stand:** YYYY-MM-DD. Teil des [Hauptplans](0_maintask.md), Etappe <N>. Geprüft gegen
<Fassung oder Branch>. Befunde: [<bewertung>.md](<bewertung>.md).

---

## Aufgaben

**Status:** ✅ abgenommen · 🟢 fertig, wartet auf die Abnahme · 🔨 in Arbeit · ⬜ offen ·
🔒 blockiert

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **XY1** | … (§2) | ⬜ | 0,5 d | `<Befehl>` → <erwartetes Ergebnis> |

**Reihenfolge:** …

---

## 1. Entscheidungen

| # | Entscheidung | Seit | Quelle |
| --- | --- | --- | --- |
| **E1** | … | YYYY-MM-DD | … |

## 2. <Begründung und Befunde>

## 3. Offene Fragen

| # | Frage | Hängt daran |
| --- | --- | --- |
| **V1** 🟡 | … | XY1 |

**Beantwortet am YYYY-MM-DD:** …

Die Fragen an die Fachseite zu diesem Plan stehen im Hauptplan: V…

## 4. Umsetzungsstand

### XY1 — <was>, YYYY-MM-DD

- **Grundlage:** …
- **Belegt:** … (`Datei:Zeile`)
- **Nicht geprüft:** …
````

---

## 10. Regeln dieses Arbeitsverzeichnisses

Sie gelten nur in `C:\Users\PascalFrechen\Polizei\itsm`. Stand 2026-10-20, mit S2 vervollständigt.

- **Neues Projekt:** Der Standard gilt hier ganz, im Aufbau der Pläne wie im Code (§1,
  „Bestehende Vorhaben" betrifft dieses Verzeichnis nicht).
- Committen und Pushen macht nur die Abnahme.
- Dateien mit LF und UTF-8; mehrzeilige Ersetzungen nie mit `perl -0777`; danach Zeilenenden und
  Umlaute prüfen.
- Lizenzköpfe von Hand setzen: AGPL, Rechteinhaber ist die group24 AG (Hauptplan E15).
- Keine eingestuften Inhalte nach Jira oder Confluence, keine Cloud-KI auf Kundendaten.
- Die Umsetzung schreibt nur in die Windows-Arbeitskopie `C:\Users\PascalFrechen\Polizei\itsm`,
  die Abnahme überträgt nach `~/police/itsm` in WSL (Hauptplan E52).
- Gelöschte oder umbenannte Dateien müssen auch in WSL gelöscht werden, sonst kommen sie über den
  Abgleich zurück.
- Befehle für die Abnahme laufen in WSL im Verzeichnis `~/police/itsm`, Werkzeuge im
  Werkzeug-Container über `docker compose`; auf dem Rechner wird nichts installiert (E52).
- Das Repository liegt vorerst nur lokal; ein Remote kommt vor Meilenstein 1 (E48).
- Eine Aufgabe, die Code ändert, ist erst fertig, wenn die Leistungsmessung aus Hauptplan H11 grün
  ist (E32), sobald H11 steht.
- **Kurzzeichen und Dateien der Pläne:**

| Kurzzeichen | Plan | Datei |
| --- | --- | --- |
| `H` | Hauptplan | `0_maintask.md` |
| `PL` | Plattform-Kern | `01_plattform-kern.md` |
| `UI` | Oberfläche | `02_oberflaeche.md` |
| `WF` | Workflow-Engine | `03_workflow-engine.md` |
| `EX` | Erweiterungen | `04_erweiterungen.md` (geplant) |
| `SD` | Service Desk | `05_service-desk.md` |
| `CM` | CMDB und Discovery | `06_cmdb-discovery.md` (geplant) |
| `PR` | Übrige Practices | `07_practices.md` (geplant) |
| `BT` | Betrieb und Auslieferung | `08_betrieb.md` (geplant) |
| `IN` | Integration | `09_integration.md` (geplant) |
| `S` | dieser Standard, kein Plan im Baum | `planungsstandard.md` |

  Für neue Listen gesperrt, weil schon belegt: `E` und `V` (§4), `H` und die Kürzel oben, dazu
  `IP` (Practices im Hauptplan §2.1), `KF` (Kernfähigkeiten im Hauptplan §2.1.1), `G` (Funktionen
  im Funktionsinventar), `B` (Befunde im Funktionsinventar), `LZ` (Leistungsziele im Hauptplan
  §2.5) und `LP` (Lastproben im Lastwerkzeug, H11).
