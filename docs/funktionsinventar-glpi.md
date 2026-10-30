# Bewertung: Funktionsinventar GLPI als Messlatte

**Stand:** 2026-10-20. Geprüft gegen den GLPI-Kern **11.0.11 ohne Plugins** (Hauptplan E23),
gelesen im Repository `..\glpi` mit `git show 11.0.11:<pfad>`. Alle Fundstellen sind
`Datei:Zeile` auf diesem Tag. Gehört zu Hauptplan H4.

**Frage:** Was kann GLPI, und wo landet jede dieser Funktionen im neuen Werkzeug?

**Ergebnis:**

1. **248 Funktionen** in acht Bereichen, von den Menüs über die Einrichtung bis zu API und
   Querschnitt. Davon **237 mit Ziel** im neuen Werkzeug und **11 entfallen**, jede mit
   Grund (§9).
2. **Was entfällt, entfällt aus fünf Gründen:** kein GLPI Agent (E24), kein Internet im Betrieb,
   nur REST (E11), die Anmeldung läuft allein über OIDC/SAML, LDAP und lokale Konten (E27, E30),
   und es gibt keine Sperren beim Bearbeiten (E29). Keine Funktion entfällt, weil sie zu
   aufwendig wäre.
3. **Vier Kandidaten bleiben** auf deine Entscheidung (E26): CalDAV, RSS, Reservierungen, anonyme
   Formulare.
4. **Der größte Teil landet im Kern** (PL, UI, WF), nicht in den Fachpaketen. Das ist gewollt:
   GLPI hat für viele Dinge eigene Klassen, die im neuen Werkzeug eine einzige Kernfähigkeit sind
   (§0, Befunde B1 bis B6).

**Verteilung der Funktionen mit Ziel:**

| Plan | Funktionen |
| --- | --- |
| PL | 82 |
| CM | 53 |
| UI | 33 |
| WF | 26 |
| SD | 24 |
| PR | 10 |
| BT | 7 |
| EX | 2 |

| Meilenstein | Funktionen |
| --- | --- |
| 1 | 133 |
| 2 | 65 |
| 3 | 34 |
| 4 | 5 |

Meilenstein 4 steht für Funktionen, die erst im Produktionsbetrieb nötig werden.

---

## 0. Wie gelesen wurde, und was daraus folgt

- **Quelle der Zeilen:** die Menüs aus `Html::getMenuInfos()` (`src/Html.php:1250`), je Menü die
  Objektarten; dazu die Reiter der Konfiguration (`src/Config.php:1014`), die Auswahllisten
  (`src/Dropdown.php:1233`), die Regelsammlungen, die automatischen Aktionen aus der Installation
  (`install/empty_data.php`), die Controller der API v2 und die Bausteine unter `src/Glpi/`.
- **Granularität:** eine Zeile je Menüpunkt, je Konfigurationsreiter, je Gruppe von Auswahllisten
  und je Fähigkeit, die über mehrere Objektarten wirkt. Einzelne Felder stehen nicht da; die
  zählt erst der Nebenplan, der die Funktion baut.
- **Ziel:** das Paket oder der Kernbaustein, der die Funktion übernimmt, mit dem Kürzel des
  Nebenplans und der Kernfähigkeit (`KF…`, Hauptplan §2.1.1). „Entfällt" steht mit Grund.
- **MS:** der Meilenstein, in dem die Funktion da sein muss. Er folgt der Karte der Practices
  (Hauptplan §2.1); wo eine Funktion früher gebraucht wird als ihre Practice, steht der frühere.

**Befunde für den Entwurf (H6):**

- **B1: Viele Klassen, eine Fähigkeit.** GLPI hat für Tickets, Changes und Problems je eigene
  Klassen für Akteure, Aufgaben, Kosten, Genehmigungen, Vorlagen und Zufriedenheit (Ticket*,
  Change*, Problem*: 45 Dateien unter `src/`, dazu die gemeinsamen CommonITIL*- und ITIL*-Klassen). Im neuen Werkzeug ist jedes davon eine
  Kernfähigkeit, die jeder Typ bekommt.
- **B2: Fähigkeiten zum Zuschalten.** GLPI 11 hat für eigene Asset-Typen 28 zuschaltbare
  Fähigkeiten (`src/Glpi/Asset/Capacity/`). Das ist das Muster, das KF16 braucht: Ein Typ
  bekommt Verlauf, Dokumente, Verträge oder Rack-Platz als Baustein, nicht als Code.
- **B3: 129 Auswahllisten in 21 Gruppen.** Jede ist in GLPI eine eigene Klasse und Tabelle. Im neuen
  Werkzeug ist eine Auswahlliste ein Datensatz eines generischen Typs, und jedes Paket bringt seine
  Listen mit.
- **B4: 34 Regelsammlungen.** Zuweisung, Import, Wörterbücher, Rechte, Mail, Orte, Kategorien:
  in GLPI je eine Klasse, hier eine Regel-Engine (KF7) mit Regeln als Daten, die zu Paketen
  gehören.
- **B5: Feldzustände je Vorlage.** ITIL-Vorlagen kennen vier Zustände eines Felds (Pflicht,
  versteckt, vorbelegt, schreibgeschützt), je Kategorie. Im neuen Werkzeug hängen diese Zustände
  am Statusmodell und an Formularvarianten (KF3, KF8), für jeden Typ.
- **B6: 48 automatische Aktionen** in der Erstinstallation. In GLPI eigene Cron-Methoden je Klasse; hier Jobs in
  River (E19) und Zeitregeln im Statusmodell (KF5, KF7).

## 1. Assets

Menü *Parc* (`src/Html.php:1259`).

| # | Funktion in GLPI | Fundstelle | Ziel im neuen Werkzeug | Plan | MS |
| --- | --- | --- | --- | --- | --- |
| **G1** | Computer | `src/Computer.php:49` | Typ im Paket Asset | CM | 2 |
| **G2** | Monitore | `src/Monitor.php:49` | Typ im Paket Asset | CM | 2 |
| **G3** | Software mit Versionen und Installationen auf Geräten | `src/Item_SoftwareVersion.php:39` | Paket Software (IP20, IP34) | CM | 2 |
| **G4** | Netzwerkgeräte | `src/NetworkEquipment.php:50` | Typ im Paket Asset; Discovery per SNMP | CM | 2 |
| **G5** | Peripherie | `src/Peripheral.php:49` | Typ im Paket Asset | CM | 2 |
| **G6** | Drucker mit Seitenzählern | `src/Printer.php:50` | Typ im Paket Asset | CM | 2 |
| **G7** | Patronen mit Bestand, Modellbindung und Warnschwelle | `src/CartridgeItem.php:48` | Paket Verbrauchsmaterial; Warnung als Regel (KF7) | CM | 3 |
| **G8** | Verbrauchsmaterial mit Bestand und Ausgabe an Personen | `src/ConsumableItem.php:50` | Paket Verbrauchsmaterial | CM | 3 |
| **G9** | Telefone | `src/Phone.php:48` | Typ im Paket Asset | CM | 2 |
| **G10** | Racks mit Belegungsplan von vorn und hinten | `src/Rack.php:46` | Paket Infrastruktur; Rack-Ansicht (KF10) | CM | 2 |
| **G11** | Gehäuse für Einschübe | `src/Enclosure.php:47` | Paket Infrastruktur | CM | 2 |
| **G12** | Stromverteiler (PDU) mit Anschlüssen | `src/PDU.php:46` | Paket Infrastruktur | CM | 3 |
| **G13** | Passive Geräte im Rechenzentrum | `src/PassiveDCEquipment.php:47` | Paket Infrastruktur | CM | 3 |
| **G14** | Nicht verwaltete Geräte aus der Netzwerkerkennung | `src/Unmanaged.php:46` | Status „nicht zugeordnet" im Abgleich (KF12) | CM | 2 |
| **G15** | Kabel mit Typ, Strang und beiden Enden | `src/Cable.php:48` | Paket Infrastruktur | CM | 3 |
| **G16** | Eigene Asset-Typen ohne Code | `src/Glpi/Asset/AssetDefinition.php:63` | Kern: frei definierbare Entitätstypen (KF1) | PL | 1 |
| **G17** | Eigene Felder an Asset-Typen: Ja/Nein, Datum, Zeit, Auswahl, Zahl, Text, URL | `src/Glpi/Asset/CustomFieldDefinition.php:52` | Kern: typisierte Felder an jedem Typ (KF1) | PL | 1 |
| **G18** | Fähigkeiten je Asset-Typ zuschalten (28, etwa Verlauf, Dokumente, Verträge, Software, Rack, Reservierung) | `src/Glpi/Asset/Capacity/HasHistoryCapacity.php:43` | Kern: Fähigkeiten als zuschaltbare Bausteine eines Typs (KF1, KF16; Befund B2) | PL | 1 |
| **G19** | Eigene Auswahllisten ohne Code | `src/Glpi/Dropdown/DropdownDefinition.php:46` | Kern: Auswahllisten als Daten (Befund B3) | PL | 1 |
| **G20** | Komponenten in Geräten: 18 Arten, etwa Prozessor, Speicher, Festplatte, Netzwerkkarte, Firmware, SIM | `src/Item_Devices.php:50` | Paket Asset; Komponenten als Untertypen | CM | 2 |
| **G21** | Betriebssystem je Gerät mit Version, Edition, Kernel, Architektur | `src/Item_OperatingSystem.php:38` | Paket Asset | CM | 2 |
| **G22** | Laufwerke und Volumes je Gerät | `src/Item_Disk.php:43` | Paket Asset | CM | 2 |
| **G23** | Virtuelle Maschinen an ihrem Host | `src/ItemVirtualMachine.php:48` | Paket Infrastruktur; Beziehung „läuft auf" (KF2) | CM | 2 |
| **G24** | Virenschutz je Gerät | `src/ItemAntivirus.php:41` | Paket Asset | CM | 3 |
| **G25** | Fernwartungszugänge je Gerät | `src/Item_RemoteManagement.php:37` | Paket Asset | CM | 3 |
| **G26** | Netzwerkports mit Typen (Ethernet, WLAN, Glasfaser, Aggregat, Alias) | `src/NetworkPort.php:56` | Paket Infrastruktur | CM | 2 |
| **G27** | IP-Adressen und IP-Netze | `src/IPAddress.php:60` | Lesend aus Netbox; Netbox bleibt führend für IP-Adressen | CM | 2 |
| **G28** | Netzwerkdosen mit Modell und Verbindung zum Port | `src/Glpi/Socket.php:55` | Paket Infrastruktur | CM | 3 |
| **G29** | Finanz- und Verwaltungsdaten: Kauf, Lieferung, Garantie, Abschreibung, Wert | `src/Infocom.php:49` | Paket Finanzen an jedem Asset; Abschreibung als Formel (KF6) | PR | 2 |
| **G30** | Gesperrte Felder: manuelle Änderung schlägt die Inventur | `src/Lockedfield.php:46` | Kern: Feldhoheit je Quelle im Abgleich (KF12) | PL | 2 |
| **G31** | Vorlagen für neue Assets | `src/CommonDBTM.php:3352` | Kern: Vorlagen je Typ | PL | 2 |
| **G32** | Asset-Dashboard | `src/Html.php:1276` | Dashboard im Paket Asset (KF11) | UI | 2 |
| **G33** | Native Inventur über den GLPI Agent | `src/Glpi/Inventory/Inventory.php:106` | entfällt: kein GLPI Agent (E24); Ersatz sind eigene Discovery-Quellen und die Bulk-API | — | — |
| **G34** | Agents verwalten, alte Agents aufräumen | `src/Agent.php:55` | entfällt: kein GLPI Agent (E24) | — | — |
| **G35** | Inventur-Fähigkeit eigener Asset-Typen | `src/Glpi/Asset/Capacity/IsInventoriableCapacity.php:55` | Kern: jeder Typ kann Ziel der Bulk-API und der Discovery sein (KF12) | PL | 2 |

## 2. Assistance

Menü *Assistance* (`src/Html.php:1280`).

| # | Funktion in GLPI | Fundstelle | Ziel im neuen Werkzeug | Plan | MS |
| --- | --- | --- | --- | --- | --- |
| **G36** | Tickets mit den Arten Incident und Anfrage | `src/Ticket.php:59` | Pakete Incident und Service Request, zwei Typen statt eines Felds | SD | 1 |
| **G37** | Akteure am Vorgang: Anforderer, Beobachter, Bearbeiter, je als Person, Gruppe oder Lieferant | `src/CommonITILActor.php:39` | Kern: Rollen am Datensatz, je Typ definierbar | WF | 1 |
| **G38** | Dringlichkeit, Auswirkung und Priorität aus einer Matrix | `src/Config.php:271` | Formelfeld oder Matrix im Paket Incident (KF6) | SD | 1 |
| **G39** | ITIL-Kategorien als Baum, mit Sichtbarkeit und Vorlagen je Kategorie | `src/ITILCategory.php:41` | Auswahlliste mit Baum im Paket Service Desk | SD | 1 |
| **G40** | Zeitleiste mit Folgemeldungen, privat oder öffentlich | `src/ITILFollowup.php:44` | Kern: Zeitleiste an jedem Typ; Paket Service Desk | UI | 1 |
| **G41** | Aufgaben mit Zeit, Planung und Bearbeiter | `src/CommonITILTask.php:52` | Kern: Untervorgänge mit Planung (KF10, KF18) | WF | 1 |
| **G42** | Lösung mit Lösungsart, Annahme oder Ablehnung durch den Anforderer | `src/ITILSolution.php:42` | Statusmodell im Paket Incident (KF3) | SD | 1 |
| **G43** | Genehmigungen an Ticket und Change | `src/CommonITILValidation.php:49` | Kern: Genehmigungen (KF4) | WF | 1 |
| **G44** | Genehmigungsstufen mit Mindestanteil | `src/ITIL_ValidationStep.php:37` | Kern: Stufen in fester Reihenfolge oder parallel (KF4) | WF | 1 |
| **G45** | Vorlagen für Genehmigungen mit Zielen | `src/ITILValidationTemplate.php:41` | Kern: Genehmigungsvorlagen (KF4) | WF | 1 |
| **G46** | Kosten am Vorgang: Zeit, fest, Material | `src/CommonITILCost.php:45` | Kern: Kosten und Zeiten (KF18) | PL | 3 |
| **G47** | Zufriedenheitsumfrage, intern oder über einen externen Link | `src/CommonITILSatisfaction.php:43` | Umfragen (KF15) | SD | 1 |
| **G48** | Verknüpfte Tickets: verknüpft, Duplikat, Eltern und Kind | `src/CommonITILObject_CommonITILObject.php:40` | Kern: Beziehungen mit Typen (KF2) | PL | 1 |
| **G49** | Betroffene Elemente am Ticket | `src/Item_Ticket.php:41` | Beziehung Vorgang zu CI (KF2) | SD | 1 |
| **G50** | Verträge am Ticket | `src/Ticket_Contract.php:38` | Beziehung Vorgang zu Vertrag (KF2) | SD | 2 |
| **G51** | Projekte und Projektaufgaben am Ticket | `src/Itil_Project.php:45` | Beziehung Vorgang zu Projekt (KF2) | PR | 3 |
| **G52** | Wissensartikel am Ticket | `src/KnowbaseItem_Item.php:45` | Paket Wissen; Vorschläge am Ticket | SD | 1 |
| **G53** | ITIL-Vorlagen: Pflicht-, versteckte, vorbelegte und schreibgeschützte Felder je Kategorie | `src/ITILTemplate.php:46` | Kern: Feldzustände je Status und Variante (KF3; Befund B5) | WF | 1 |
| **G54** | Wartegründe mit automatischer Folgemeldung und Lösung | `src/PendingReason.php:41` | Kern: Pause der Uhr mit Wartegrund und Zeitregel (KF5) | WF | 1 |
| **G55** | Anfragequellen (Telefon, Mail, Portal, …) | `src/Dropdown.php:1256` | Auswahlliste im Paket Service Desk | SD | 1 |
| **G56** | Vorlagen für Folgemeldungen, Aufgaben und Lösungen | `src/ITILFollowupTemplate.php:42` | Textbausteine im Paket Service Desk | SD | 1 |
| **G57** | Regeln für Tickets: zuweisen, einordnen, ändern beim Anlegen und Ändern | `src/RuleTicket.php:36` | Kern: Regeln (KF7) | WF | 1 |
| **G58** | Regeln für Changes | `src/RuleChange.php:36` | Kern: Regeln (KF7) | WF | 2 |
| **G59** | Regeln für Problems | `src/RuleProblem.php:36` | Kern: Regeln (KF7) | WF | 2 |
| **G60** | Sperre beim gleichzeitigen Bearbeiten | `src/ObjectLock.php:47` | Kern: Konflikterkennung beim Speichern, live sichtbar wer bearbeitet; keine Sperren (E29) | PL | 1 |
| **G61** | Wiederkehrende Tickets | `src/TicketRecurrent.php:41` | Kern: Zeitpläne, die Datensätze anlegen | WF | 1 |
| **G62** | Wiederkehrende Changes | `src/RecurrentChange.php:36` | Kern: Zeitpläne, die Datensätze anlegen | WF | 2 |
| **G63** | Gelöste Tickets automatisch schließen | `install/empty_data.php:586` | Zeitregel im Statusmodell (KF3, KF5) | WF | 1 |
| **G64** | Warnung zu lange offener Tickets | `install/empty_data.php:598` | Zeitregel mit Benachrichtigung (KF7, KF14) | WF | 1 |
| **G65** | Umfrage nach dem Schließen anlegen, für Tickets und Changes | `install/empty_data.php:622` | Umfragen (KF15) | SD | 1 |
| **G66** | SLA- und OLA-Eskalationsstufen ausführen | `install/empty_data.php:610` | Kern: SLA-Uhren und Eskalation (KF5) | WF | 1 |
| **G67** | Erinnerung an offene Genehmigungen | `install/empty_data.php:1006` | Kern: Erinnerung in Genehmigungen (KF4) | WF | 1 |
| **G68** | Servicekatalog für Nutzer | `src/Glpi/Form/ServiceCatalog/ServiceCatalog.php:45` | Paket Servicekatalog | SD | 1 |
| **G69** | Problems mit Ursache, Symptom, Auswirkung | `src/Problem.php:46` | Paket Problem | PR | 2 |
| **G70** | Problems an Tickets und Changes | `src/Problem_Ticket.php:38` | Beziehungen (KF2) | PR | 2 |
| **G71** | Changes mit Analyse (Auswirkung, Kontrollliste) und Plan (Ausrollen, Rückfall, Prüfliste) | `src/Change.php:46` | Paket Change | PR | 2 |
| **G72** | Changes an Tickets, Problems und Elementen | `src/Change_Ticket.php:43` | Beziehungen (KF2) | PR | 2 |
| **G73** | Planung: Kalender der Aufgaben und Termine aller Bearbeiter | `src/Planning.php:60` | Kalenderansicht (KF10) | UI | 1 |
| **G74** | Externe Termine mit Vorlagen und Kategorien | `src/PlanningExternalEvent.php:45` | Paket Planung | UI | 3 |
| **G75** | Erinnerungen an geplante Termine | `src/PlanningRecall.php:44` | Benachrichtigung zu Terminen (KF14) | WF | 3 |
| **G76** | Statistik: global, nach Merkmal, nach Element, je Asset | `src/Stat.php:53` | Berichte und Dashboards (KF11) | UI | 1 |
| **G77** | Helpdesk-Dashboard | `src/Html.php:1291` | Dashboard im Paket Service Desk (KF11) | UI | 1 |
| **G78** | Startseite des Portals mit Kacheln (Formular, Seite, externer Link) | `src/Glpi/Helpdesk/Tile/TilesManager.php:45` | Portal im Paket Service Desk | SD | 1 |
| **G79** | Team je Vorgang mit Rollen | `src/Glpi/Team/Team.php:44` | Kern: Rollen am Datensatz | WF | 1 |
| **G80** | Bearbeitungszeit am Ticket | `src/Ticket.php:1497` | Kern: Zeiten (KF18) | PL | 1 |

## 3. Verwaltung

Menü *Gestion* (`src/Html.php:1295`).

| # | Funktion in GLPI | Fundstelle | Ziel im neuen Werkzeug | Plan | MS |
| --- | --- | --- | --- | --- | --- |
| **G81** | Softwarelizenzen mit Anzahl, Zuweisung, Ablauf | `src/SoftwareLicense.php:48` | Paket Lizenzen mit Abgleich gegen Installationen | CM | 2 |
| **G82** | Warnung zu ablaufenden Lizenzen | `install/empty_data.php:466` | Zeitregel (KF7) | CM | 2 |
| **G83** | Budgets mit Typ, Zeitraum, Wert und Verbrauch | `src/Budget.php:46` | Paket Finanzen | PR | 3 |
| **G84** | Lieferanten | `src/Supplier.php:46` | Paket Lieferanten | CM | 2 |
| **G85** | Kontakte | `src/Contact.php:45` | Paket Lieferanten | CM | 2 |
| **G86** | Verträge mit Laufzeit, Verlängerung, Kündigungsfrist, Kosten | `src/Contract.php:47` | Paket Lieferanten | CM | 2 |
| **G87** | Warnung zu Vertragsenden und Kündigungsfristen | `install/empty_data.php:478` | Fristen (KF5) und Zeitregel | CM | 2 |
| **G88** | Warnung zu Garantieenden | `install/empty_data.php:490` | Zeitregel (KF7) | CM | 2 |
| **G89** | Warnung zu Patronen und Verbrauchsmaterial unter Mindestbestand | `install/empty_data.php:442` | Zeitregel (KF7) | CM | 3 |
| **G90** | Dokumente mit Typ, Kategorie, Verknüpfung zu jedem Objekt | `src/Document.php:67` | Kern: Dateien (E22) an jedem Typ | PL | 1 |
| **G91** | Verwaiste Dokumente aufräumen | `install/empty_data.php:826` | Kern-Job | PL | 2 |
| **G92** | Leitungen mit Betreiber und Typ | `src/Line.php:46` | Typ im Paket Infrastruktur | CM | 3 |
| **G93** | Zertifikate mit Ablauf | `src/Certificate.php:49` | Typ im Paket Asset | CM | 2 |
| **G94** | Warnung zu ablaufenden Zertifikaten | `install/empty_data.php:778` | Zeitregel (KF7) | CM | 2 |
| **G95** | Rechenzentren und Räume | `src/Datacenter.php:41` | Paket Infrastruktur | CM | 2 |
| **G96** | Cluster | `src/Cluster.php:44` | Paket Infrastruktur | CM | 2 |
| **G97** | Domains mit Einträgen und Ablauf | `src/Domain.php:42` | Typ im Paket Infrastruktur | CM | 3 |
| **G98** | Warnung zu ablaufenden Domains | `install/empty_data.php:874` | Zeitregel (KF7) | CM | 3 |
| **G99** | Appliances: Anwendungen mit Umgebung und Elementen | `src/Appliance.php:46` | Paket Software (IP34) | CM | 2 |
| **G100** | Datenbanken und Datenbankinstanzen | `src/Database.php:41` | Typ im Paket Infrastruktur | CM | 2 |

## 4. Werkzeuge

Menü *Outils* (`src/Html.php:1304`).

| # | Funktion in GLPI | Fundstelle | Ziel im neuen Werkzeug | Plan | MS |
| --- | --- | --- | --- | --- | --- |
| **G101** | Projekte mit Phasen, Aufgaben, Team, Kosten | `src/Project.php:50` | Paket Projekt | PR | 3 |
| **G102** | Projektaufgaben mit Abhängigkeiten, Planung, Vorlagen | `src/ProjectTask.php:60` | Paket Projekt | PR | 3 |
| **G103** | Kanban für Projekte | `src/Glpi/Features/Kanban.php:45` | Kanban-Ansicht (KF10) | UI | 3 |
| **G104** | Notizen mit Sichtbarkeit für Personen, Gruppen, Profile | `src/Reminder.php:51` | Paket Notizen | UI | 3 |
| **G105** | RSS-Feeds auf der Startseite | `src/RSSFeed.php:50` | bleibt (E26), nur interne Quellen | UI | 3 |
| **G106** | Wissensdatenbank mit Kategorien, Sichtbarkeit, FAQ | `src/KnowbaseItem.php:57` | Paket Wissen | SD | 1 |
| **G107** | Versionen von Wissensartikeln | `src/KnowbaseItem_Revision.php:43` | Kern: Versionen von Datensätzen (KF17) | PL | 1 |
| **G108** | Kommentare zu Wissensartikeln | `src/KnowbaseItem_Comment.php:43` | Paket Wissen | SD | 1 |
| **G109** | Reservierbare Elemente und Reservierungen | `src/ReservationItem.php:44` | bleibt (E26): Paket Reservierung | CM | 3 |
| **G110** | Warnung zu Reservierungen | `install/empty_data.php:574` | Zeitregel (KF7) | CM | 3 |
| **G111** | Feste Berichte (Verträge, Garantie, Netzwerk, Reservierungen) | `src/Report.php:52` | Berichte (KF11) | UI | 3 |
| **G112** | Gespeicherte Suchen, privat oder geteilt | `src/SavedSearch.php:52` | Gespeicherte Abfragen (KF11) | UI | 1 |
| **G113** | Benachrichtigung, wenn eine gespeicherte Suche Treffer hat | `src/SavedSearch_Alert.php:45` | Regel auf Abfrage (KF7, KF11) | WF | 3 |
| **G114** | Auswirkungsanalyse als Graph | `src/Impact.php:49` | Graph-Ansicht und Auswirkungsanalyse (KF2, KF10) | UI | 2 |
| **G115** | Notizblock an Objekten | `src/Notepad.php:45` | Kern: Notizen an jedem Typ | PL | 2 |

## 5. Administration

Menü *Administration* (`src/Html.php:1318`).

| # | Funktion in GLPI | Fundstelle | Ziel im neuen Werkzeug | Plan | MS |
| --- | --- | --- | --- | --- | --- |
| **G116** | Personen mit mehreren Mailadressen, Titel, Kategorie, Telefon, Vorgesetztem | `src/User.php:71` | Kern: Personen | PL | 1 |
| **G117** | Passwortablauf für lokale Konten | `install/empty_data.php:838` | Kern: lokale Konten (E10) | PL | 1 |
| **G118** | Gruppen als Baum, mit Verantwortlichen und Zweck (Anforderer, Bearbeiter, Beobachter) | `src/Group.php:45` | Kern: Gruppen | PL | 1 |
| **G119** | Entitäten als Baum mit vererbten Einstellungen | `src/Entity.php:58` | Kern: Organisationsbaum (E16) | PL | 1 |
| **G120** | Objekte zwischen Entitäten verschieben | `src/Transfer.php:50` | Kern: Datensatz im Organisationsbaum verschieben | PL | 2 |
| **G121** | Regeln zur Rechtevergabe aus LDAP und SSO | `src/RuleRight.php:43` | Kern: Rechteregeln bei der Anmeldung (E10, KF7) | PL | 1 |
| **G122** | Regeln zur Entitätszuordnung beim Import | `src/RuleImportEntity.php:39` | Abgleichsregeln (KF12) | CM | 2 |
| **G123** | Regeln testen, exportieren, importieren | `src/RuleCollection.php:48` | Kern: Regeln sind Teil eines Pakets (EX) | WF | 2 |
| **G124** | Profile mit Rechten je Objektart und Aktion | `src/Profile.php:56` | Kern: Rollen und Rechte (KF9) | PL | 1 |
| **G125** | Zwei Oberflächen: vereinfacht für Nutzer, Standard für Bearbeiter | `src/Profile.php:188` | Portal und Arbeitsplatz (UI, SD) | UI | 1 |
| **G126** | Warteschlange der Benachrichtigungen | `src/QueuedNotification.php:48` | Kern: Jobs (E19) | PL | 1 |
| **G127** | Protokolldateien in der Oberfläche ansehen | `src/Glpi/System/Log/LogViewer.php:43` | Betrieb: technische Logs strukturiert an das Monitoring, nicht in die Oberfläche (E28) | BT | 4 |
| **G128** | Ereignisprotokoll (Anmeldungen, Änderungen, Fehler) | `src/Glpi/Event.php:63` | Kern: Audit (PL); technische Logs (BT) | PL | 1 |
| **G129** | Konfiguration der Inventur | `src/Glpi/Inventory/Conf.php:132` | entfällt: kein GLPI Agent (E24) | — | — |
| **G130** | Formulare mit Abschnitten und Fragen | `src/Glpi/Form/Form.php:92` | Formulare (KF8) | UI | 1 |
| **G131** | Fragetypen: Text, Zahl, Datum, Auswahl, Ankreuzen, Datei, Person, Gerät, Kategorie, Dringlichkeit, … | `src/Glpi/Form/QuestionType/QuestionTypesManager.php:47` | Formularfelder aus den Feldtypen des Kerns (KF1, KF8) | UI | 1 |
| **G132** | Bedingungen in Formularen: Fragen und Abschnitte ein- und ausblenden | `src/Glpi/Form/Condition/Engine.php:51` | Formulare (KF8) | UI | 1 |
| **G133** | Formularziele: Ticket, Change, Problem mit Feldzuordnung | `src/Glpi/Form/Destination/FormDestinationTicket.php:47` | Formularziele für jeden Typ (KF8) | SD | 1 |
| **G134** | Zugriff auf Formulare: angemeldet, Gruppen, anonym mit Link | `src/Glpi/Form/AccessControl/FormAccessControl.php:55` | Formulare; anonym bleibt (E26) | SD | 1 |
| **G135** | Formulare exportieren und importieren | `src/Glpi/Form/Export/Serializer/FormSerializer.php:97` | Teil eines Pakets (EX) | EX | 3 |
| **G136** | Formulare übersetzen | `src/Glpi/Form/FormTranslation.php:45` | Mehrsprachige Inhalte (KF19) | UI | 1 |
| **G137** | Captcha für anonyme Formulare | `src/Glpi/Altcha/AltchaManager.php:81` | bleibt (E26) | SD | 1 |

## 6. Einrichtung

Menü *Configuration* (`src/Html.php:1327`).

| # | Funktion in GLPI | Fundstelle | Ziel im neuen Werkzeug | Plan | MS |
| --- | --- | --- | --- | --- | --- |
| **G138** | Auswahllisten: 129 Arten in 21 Gruppen | `src/Dropdown.php:1233` | Kern: eine generische Auswahlliste statt einer Tabelle je Art (Befund B3); jedes Paket bringt seine Listen mit | PL | 1 |
| **G139** | Auswahllisten, Gruppe Allgemein: Orte (Baum, mit Koordinaten), Status, Hersteller, Sperrlisten, Filter | `src/Dropdown.php:1237` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G140** | Auswahllisten, Gruppe Assistance: Vorlagen, Kategorien, Aufgaben- und Lösungsarten, Anfragequellen, Wartegründe, Genehmigungsstufen | `src/Dropdown.php:1246` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G141** | Auswahllisten, Gruppe Typen je Objektart (Computer, Netzwerk, Drucker, Vertrag, …) | `src/Dropdown.php:1269` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G142** | Auswahllisten, Gruppe Modelle je Objektart und Komponente | `src/Dropdown.php:1303` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G143** | Auswahllisten, Gruppe Virtuelle Maschinen: Typ, System, Zustand | `src/Dropdown.php:1335` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G144** | Auswahllisten, Gruppe Verwaltung: Dokumentkategorien und -typen, Kritikalität | `src/Dropdown.php:1341` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G145** | Auswahllisten, Gruppe Werkzeuge: Wissenskategorien | `src/Dropdown.php:1348` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G146** | Auswahllisten, Gruppe Kalender und Feiertage | `src/Dropdown.php:1352` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G147** | Auswahllisten, Gruppe Betriebssysteme mit Version, Edition, Kernel, Architektur, Updatequelle | `src/Dropdown.php:1357` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G148** | Auswahllisten, Gruppe Netzwerk: Schnittstellen, Netze, VLANs, Porttypen, FQDN, WLAN | `src/Dropdown.php:1368` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G149** | Auswahllisten, Gruppe Kabel: Typen, Stränge, Dosenmodelle | `src/Dropdown.php:1381` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G150** | Auswahllisten, Gruppe Internet: Domaintypen, -beziehungen, Eintragstypen | `src/Dropdown.php:1387` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G151** | Auswahllisten, Gruppe Software: Kategorien | `src/Dropdown.php:1394` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G152** | Auswahllisten, Gruppe Personen: Titel und Kategorien | `src/Dropdown.php:1398` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G153** | Auswahllisten, Gruppe Parameter der Rechteregeln | `src/Dropdown.php:1403` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G154** | Auswahllisten, Gruppe Feldeindeutigkeit: gesperrte Werte | `src/Dropdown.php:1407` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G155** | Auswahllisten, Gruppe Externe Anmeldung: SSO-Variablen | `src/Dropdown.php:1411` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G156** | Auswahllisten, Gruppe Strom: Stecker | `src/Dropdown.php:1414` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G157** | Auswahllisten, Gruppe Appliances: Typ und Umgebung | `src/Dropdown.php:1417` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G158** | Auswahllisten, Gruppe Sonstige: Bildauflösung und -format, USB- und PCI-Hersteller, Webhook-Kategorien | `src/Dropdown.php:1425` | Auswahllisten in dem Paket, das sie braucht | PL | 1 |
| **G159** | Komponenten als Katalog (Prozessor, Speicher, …) | `src/CommonDevice.php:42` | Paket Asset | CM | 2 |
| **G160** | Benachrichtigungen: Ereignis je Objektart, Empfänger, Vorlage | `src/Notification.php:44` | Benachrichtigungen (KF14) | WF | 1 |
| **G161** | Benachrichtigungsvorlagen je Sprache mit Platzhaltern | `src/NotificationTemplate.php:47` | Vorlagen mit Platzhaltern aus dem Meta-Modell (KF14, KF19) | WF | 1 |
| **G162** | Ereignisse für Tickets: neu, geändert, gelöst, Lösung abgelehnt, Genehmigung, Umfrage, SLA-Erinnerung, … | `src/NotificationTargetTicket.php:51` | Ereignisse aus dem Statusmodell, nicht fest verdrahtet (KF3, KF14) | WF | 1 |
| **G163** | Benachrichtigung per Mail | `src/NotificationMailing.php:41` | Mail hinaus (KF14) | SD | 1 |
| **G164** | Benachrichtigung im Browser | `src/NotificationAjax.php:42` | Live-Benachrichtigung über SSE (E11) | UI | 1 |
| **G165** | Webhooks mit Ereignissen, Nutzlast, Signatur, Wiederholung | `src/Webhook.php:64` | Webhooks (KF13) | PL | 1 |
| **G166** | Warteschlange der Webhooks | `install/empty_data.php:958` | Kern: Jobs (E19) | PL | 1 |
| **G167** | Service-Levels: SLA und OLA, Zeit bis Übernahme und bis Lösung | `src/SLM.php:42` | Fristen und SLA-Uhren für jeden Typ (KF5) | WF | 1 |
| **G168** | Eskalationsstufen von SLA und OLA mit Aktionen | `src/SlaLevel.php:39` | Eskalation (KF5, KF7) | WF | 1 |
| **G169** | Kalender mit Arbeitszeiten und Feiertagen | `src/Calendar.php:43` | Kalender (KF5) | WF | 1 |
| **G170** | Konfiguration Anzeige: Sprache, Format, Seitengröße | `src/Config.php:1032` | Persönliche und globale Einstellungen | UI | 1 |
| **G171** | Konfiguration Standardwerte je Person | `src/Config.php:1033` | Persönliche Einstellungen | UI | 1 |
| **G172** | Konfiguration Assets: automatische Felder, Verbindungsregeln | `src/Config.php:1034` | Einstellungen im Paket Asset | CM | 2 |
| **G173** | Konfiguration Assistance: Matrix, Anhänge, Zeitleiste | `src/Config.php:1035` | Einstellungen im Paket Service Desk | SD | 1 |
| **G174** | Konfiguration Verwaltung: Finanzdaten automatisch | `src/Config.php:1036` | Einstellungen im Paket Finanzen | PR | 3 |
| **G175** | Aufbewahrung des Verlaufs je Ereignisart | `src/Config.php:1039` | Kern: Aufbewahrung je Typ, Partitionen löschen (§2.2) | PL | 4 |
| **G176** | Konfiguration System: Proxy, Wartungsmodus, Debug | `src/Config.php:1040` | Betrieb | BT | 1 |
| **G177** | Konfiguration Sicherheit: Passwortregeln, Sitzung, Zwei-Faktor | `src/Config.php:1041` | Kern: Anmeldung (E10) | PL | 1 |
| **G178** | Konfiguration Performance: Cache | `src/Config.php:1042` | Valkey (E20) | BT | 1 |
| **G179** | Konfiguration API | `src/Config.php:1043` | Kern: API (E11) | PL | 1 |
| **G180** | Lese-Replikat der Datenbank | `src/Config.php:1051` | Betrieb: PostgreSQL-Replikation, lesender Zugang für Berichte (V17) | BT | 4 |
| **G181** | Prüfung, ob das Replikat nachläuft | `install/empty_data.php:526` | Betrieb: Monitoring der Replikation | BT | 4 |
| **G182** | Eindeutigkeit von Feldern je Objektart | `src/FieldUnicity.php:41` | Kern: Eindeutigkeit je Typ, als Index (KF1) | PL | 1 |
| **G183** | Automatische Aktionen mit Zeitplan, Modus und Protokoll | `src/CronTask.php:59` | Kern: Jobs (E19) mit Verwaltung in der Oberfläche | PL | 1 |
| **G184** | Anmeldung über LDAP, mehrere Verzeichnisse, mit Import und Abgleich | `src/AuthLDAP.php:59` | LDAP/AD-Sync (E10) | PL | 1 |
| **G185** | Anmeldung über einen Mailserver | `src/AuthMail.php:41` | entfällt: OIDC/SAML, LDAP und lokale Konten genügen (E10, E27) | — | — |
| **G186** | Externe Anmeldung über Webserver-Variablen (Kopfzeilen-SSO, CAS, Client-Zertifikat) | `src/Auth.php:56` | entfällt: OIDC/SAML trägt die Anmeldung allein; Smartcard über den Identitätsanbieter (E30) | — | — |
| **G187** | OAuth-Clients für die API | `src/OAuthClient.php:45` | Kern: API-Zugänge mit OAuth 2 (E11) | PL | 1 |
| **G188** | Mailempfänger: Postfächer abholen, Tickets anlegen | `src/MailCollector.php:73` | Mail hinein (KF14; V9) | SD | 1 |
| **G189** | Regeln für eingehende Mail | `src/RuleMailCollector.php:37` | Regeln (KF7) | SD | 1 |
| **G190** | Sperrliste für Mailinhalte | `src/BlacklistedMailContent.php:43` | Mail hinein (KF14) | SD | 1 |
| **G191** | Fehlerhafte Mails melden | `install/empty_data.php:706` | Mail hinein (KF14) | SD | 1 |
| **G192** | Externe Links an Objekten mit Platzhaltern | `src/Link.php:47` | Kern: Links je Typ | UI | 2 |
| **G193** | Handbuch-Links an Objekten | `src/ManualLink.php:42` | Kern: Links je Datensatz | UI | 2 |
| **G194** | Plugins installieren und verwalten | `src/Plugin.php:62` | Ersetzt durch Pakete (E7, EX) | EX | 2 |
| **G195** | Marktplatz für Plugins | `src/Glpi/Marketplace/Controller.php:64` | entfällt: kein Internet im Betrieb | — | — |
| **G196** | Prüfung auf neue Versionen | `install/empty_data.php:538` | entfällt: kein Internet im Betrieb | — | — |
| **G197** | Telemetrie an den Hersteller | `src/Telemetry.php:51` | entfällt: kein Internet im Betrieb, VS-NfD | — | — |
| **G198** | Sperrliste für Werte aus der Inventur | `src/Blacklist.php:45` | Abgleichsregeln (KF12) | CM | 2 |
| **G199** | Wörterbücher: Software, Hersteller, Modelle, Typen, Betriebssysteme, Drucker umbenennen und vereinheitlichen | `src/RuleDictionnarySoftware.php:44` | Abgleichsregeln (KF12) | CM | 2 |
| **G200** | Regeln für den Import von Assets: neu, verknüpfen, verwerfen | `src/RuleImportAsset.php:46` | Identifikationsregeln im Abgleich (KF12) | CM | 2 |
| **G201** | Regeln für Orte beim Import | `src/RuleLocation.php:37` | Abgleichsregeln (KF12) | CM | 2 |
| **G202** | Regeln für Assets beim Anlegen und Ändern | `src/RuleAsset.php:36` | Regeln (KF7) | CM | 2 |
| **G203** | Regeln für Softwarekategorien | `src/RuleSoftwareCategory.php:43` | Abgleichsregeln (KF12) | CM | 2 |
| **G204** | Regel, die die Objektart beim Import bestimmt | `src/RuleDefineItemtype.php:39` | Identifikationsregeln (KF12) | CM | 2 |
| **G205** | Persönliche Einstellungen | `src/Html.php:1339` | Persönliche Einstellungen | UI | 1 |

## 7. Schnittstellen

API und Protokolle.

| # | Funktion in GLPI | Fundstelle | Ziel im neuen Werkzeug | Plan | MS |
| --- | --- | --- | --- | --- | --- |
| **G206** | REST-API v1 | `src/Glpi/Api/APIRest.php:60` | Ersetzt durch die REST-API (E11) | PL | 1 |
| **G207** | High-Level-API v2 mit Router und OpenAPI | `src/Glpi/Api/HL/Router.php:95` | REST-API mit OpenAPI aus dem Meta-Modell (E11) | PL | 1 |
| **G208** | API v2: Administration (Personen, Gruppen, Entitäten, Profile) | `src/Glpi/Api/HL/Controller/AdministrationController.php:67` | REST-API (E11) | PL | 1 |
| **G209** | API v2: Assets | `src/Glpi/Api/HL/Controller/AssetController.php:189` | REST-API je Typ, mit Bulk (E11, E24) | PL | 1 |
| **G210** | API v2: Komponenten | `src/Glpi/Api/HL/Controller/ComponentController.php:136` | REST-API je Typ | PL | 2 |
| **G211** | API v2: eigene Assets | `src/Glpi/Api/HL/Controller/CustomAssetController.php:74` | REST-API je Typ, automatisch aus dem Meta-Modell | PL | 1 |
| **G212** | API v2: Dashboards | `src/Glpi/Api/HL/Controller/DashboardController.php:56` | REST-API | PL | 2 |
| **G213** | API v2: Auswahllisten | `src/Glpi/Api/HL/Controller/DropdownController.php:150` | REST-API | PL | 1 |
| **G214** | API v2: ITIL (Tickets, Changes, Problems, Zeitleiste) | `src/Glpi/Api/HL/Controller/ITILController.php:131` | REST-API | PL | 1 |
| **G215** | API v2: Inventur | `src/Glpi/Api/HL/Controller/InventoryController.php:54` | entfällt: kein GLPI Agent (E24); Ersatz ist die Bulk-API | — | — |
| **G216** | API v2: Wissen | `src/Glpi/Api/HL/Controller/KnowbaseController.php:82` | REST-API | PL | 1 |
| **G217** | API v2: Verwaltung (Verträge, Lieferanten, Budgets, Dokumente) | `src/Glpi/Api/HL/Controller/ManagementController.php:91` | REST-API | PL | 2 |
| **G218** | API v2: Notizen | `src/Glpi/Api/HL/Controller/NotepadController.php:78` | REST-API | PL | 2 |
| **G219** | API v2: Benachrichtigungen | `src/Glpi/Api/HL/Controller/NotificationController.php:58` | REST-API | PL | 1 |
| **G220** | API v2: Projekte | `src/Glpi/Api/HL/Controller/ProjectController.php:79` | REST-API | PL | 3 |
| **G221** | API v2: Berichte und Statistik | `src/Glpi/Api/HL/Controller/ReportController.php:58` | REST-API | PL | 3 |
| **G222** | API v2: Regeln | `src/Glpi/Api/HL/Controller/RuleController.php:87` | REST-API | PL | 2 |
| **G223** | API v2: Einrichtung | `src/Glpi/Api/HL/Controller/SetupController.php:75` | REST-API | PL | 1 |
| **G224** | API v2: Werkzeuge (Notizen, RSS, Reservierungen) | `src/Glpi/Api/HL/Controller/ToolController.php:56` | REST-API | PL | 3 |
| **G225** | GraphQL für lesende Abfragen | `src/Glpi/Api/HL/Controller/GraphQLController.php:48` | entfällt: nur REST (E11) | — | — |
| **G226** | API-Clients mit IP-Bereich und Token | `src/APIClient.php:42` | Kern: API-Zugänge (E11) | PL | 1 |
| **G227** | CalDAV-Server für Planung und Notizen | `src/Glpi/CalDAV/Server.php:53` | bleibt (E26) | PL | 3 |

## 8. Querschnitt

Was an jedem Objekt wirkt.

| # | Funktion in GLPI | Fundstelle | Ziel im neuen Werkzeug | Plan | MS |
| --- | --- | --- | --- | --- | --- |
| **G228** | Suche mit Kriterien, verknüpften Kriterien und Sortierung | `src/Search.php:53` | Kern: Abfragen über das Meta-Modell, Rechte als SQL-Bedingung (§2.2) | PL | 1 |
| **G229** | Spalten je Liste wählen, global und persönlich | `src/DisplayPreference.php:41` | Listen (UI) | UI | 1 |
| **G230** | Export der Liste als CSV | `src/Glpi/Search/Output/Csv.php:38` | Export (KF11) | UI | 1 |
| **G231** | Export der Liste als XLSX und ODS | `src/Glpi/Search/Output/Spreadsheet.php:50` | Export (KF11) | UI | 1 |
| **G232** | Export der Liste als PDF | `src/Glpi/Search/Output/Pdf.php:43` | Export (KF11) | UI | 3 |
| **G233** | Kartenansicht über Orte mit Koordinaten | `src/Glpi/Search/Output/MapSearchOutput.php:48` | Kartenansicht (KF10), mit Kartenmaterial ohne Internet | UI | 3 |
| **G234** | Massenaktionen auf gewählte Datensätze | `src/MassiveAction.php:52` | Massenbearbeitung über die Bulk-API (E24) | UI | 1 |
| **G235** | Verlauf jeder Änderung je Feld | `src/Log.php:48` | Kern: Audit | PL | 1 |
| **G236** | Papierkorb und endgültiges Löschen | `src/CommonDBTM.php:2131` | Kern: Papierkorb je Typ | PL | 1 |
| **G237** | Übersetzung von Inhalten | `src/Glpi/ItemTranslation/ItemTranslation.php:50` | Mehrsprachige Inhalte (KF19) | PL | 1 |
| **G238** | Dashboards mit Widgets (Torte, Balken, Linie, Kennzahl, Liste, Markdown) und Filtern | `src/Glpi/Dashboard/Widget.php:58` | Dashboards (KF11) | UI | 1 |
| **G239** | Dashboards anlegen, teilen, als Kachel einbetten | `src/Glpi/Dashboard/Dashboard.php:66` | Dashboards (KF11) | UI | 1 |
| **G240** | Profil und Entität in der Sitzung wechseln | `src/Session.php:461` | Kern: aktive Rolle und Organisationseinheit | PL | 1 |
| **G241** | Sitzungen aufräumen | `install/empty_data.php:550` | Valkey (E20) | PL | 1 |
| **G242** | Formatierter Text mit eingefügten Bildern | `src/Glpi/RichText/RichText.php:51` | Editor in der Oberfläche; Bilder als Dateien (E22) | UI | 1 |
| **G243** | Sperren von Objekten nach Zeit lösen | `install/empty_data.php:730` | entfällt: es gibt keine Sperren mehr (E29) | — | — |
| **G244** | Verlauf und Tickets nach Zeit löschen | `install/empty_data.php:814` | Kern: Aufbewahrung je Typ | PL | 4 |
| **G245** | Temporäre Dateien aufräumen | `install/empty_data.php:886` | Kern-Job | PL | 1 |
| **G246** | Kommandozeile für Wartung, Migration, LDAP-Abgleich, Cache | `src/Glpi/Console/Application.php:66` | Betrieb: ein Binary mit Unterbefehlen (V10) | BT | 1 |
| **G247** | Oberfläche in 67 Sprachen | `src/Session.php:819` | Deutsch und Englisch zuerst (V6), weitere als Sprachpaket | UI | 1 |
| **G248** | Konfigurationsdatei und Fehlerbehandlung im Betrieb | `src/Glpi/Kernel/Kernel.php:58` | Betrieb: Konfiguration über Umgebungsvariablen und Helm-Werte | BT | 1 |

## 9. Was entfällt

- Native Inventur über den GLPI Agent: kein GLPI Agent (E24); Ersatz sind eigene Discovery-Quellen und die Bulk-API
- Agents verwalten, alte Agents aufräumen: kein GLPI Agent (E24)
- Konfiguration der Inventur: kein GLPI Agent (E24)
- Anmeldung über einen Mailserver: OIDC/SAML, LDAP und lokale Konten genügen (E10, E27)
- Externe Anmeldung über Webserver-Variablen (Kopfzeilen-SSO, CAS, Client-Zertifikat): OIDC/SAML trägt die Anmeldung allein; Smartcard über den Identitätsanbieter (E30)
- Marktplatz für Plugins: kein Internet im Betrieb
- Prüfung auf neue Versionen: kein Internet im Betrieb
- Telemetrie an den Hersteller: kein Internet im Betrieb, VS-NfD
- API v2: Inventur: kein GLPI Agent (E24); Ersatz ist die Bulk-API
- GraphQL für lesende Abfragen: nur REST (E11)
- Sperren von Objekten nach Zeit lösen: es gibt keine Sperren mehr (E29)

## 10. Nicht geprüft

- **Funktionen innerhalb der Objektarten** sind nur so weit erfasst, wie sie als eigene Klasse
  oder eigener Reiter sichtbar sind. Einzelne Felder, Reiter einzelner Objektarten und
  Massenaktionen je Typ zählt erst der Nebenplan, der die Funktion baut.
- **Plugins** sind nicht erfasst (E23). Was die Polizei über `datainjection`, `oauthsso` und
  `branding` nutzt, deckt das neue Werkzeug mit Import (CM), OIDC (E10) und einem eigenen
  Erscheinungsbild ab; das ist nicht Zeile für Zeile verglichen.
- **Rechte:** GLPI kennt je Objektart eigene Rechte (Lesen, Anlegen, Ändern, Löschen, dazu
  Sonderrechte wie „Ticket an sich selbst zuweisen"). Die Liste der Sonderrechte ist nicht
  erhoben; sie gehört in PL, bevor das Rechtemodell steht.
- **Die Oberfläche** ist nicht geklickt; die Zeilen kommen aus dem Code. Ob ein Menüpunkt in der
  Oberfläche anders heißt, ist nicht geprüft.
