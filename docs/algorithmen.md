# Optimierungs-Algorithmen

`plexams.go` löst an zwei Stellen ein kombinatorisches Zuordnungsproblem, das sich
nicht mit einer einfachen „nimm der Reihe nach"-Heuristik zuverlässig lösen lässt:

1. **Aufsichtenplanung** — Aufsichten (invigilations) auf Prüfungs-Slots/Räume
   verteilen → **Simulated Annealing** ([plexams/invigplan/optimizer.go](../plexams/invigplan/optimizer.go)).
2. **Vorplanung SEB/EXaHM** — vorgeplante Prüfungen auf MUC.DAI-Slots mit gebuchten
   Anny-Räumen verteilen → **DSATUR-Konstruktion + SA-Reparatur**
   ([plexams/preplan_solve.go](../plexams/preplan_solve.go)).

Beide teilen dieselbe Grundidee: ein **Startplan** wird konstruktiv erzeugt und dann
durch lokale Veränderungen verbessert, wobei **harte Nebenbedingungen** zu jedem
Zeitpunkt eingehalten werden und **weiche Ziele** über eine Kostenfunktion
gegeneinander abgewogen werden.

---

## 1. Glossar

- **Harte Nebenbedingung (hard constraint):** muss *immer* gelten (z. B. eine Aufsicht
  kann nicht gleichzeitig an zwei Orten sein; gleicher Studiengang nie im selben Slot).
  Ein Plan, der eine harte Bedingung verletzt, ist ungültig.
- **Weiche Nebenbedingung (soft constraint):** soll *möglichst* gelten (z. B. Aufsichten
  fair verteilen; gleichen Studiengang auf verschiedene Tage legen). Verletzungen
  kosten „Strafpunkte", werden aber toleriert, wenn nötig.
- **Kostenfunktion:** Summe aller gewichteten Strafpunkte eines Plans. Je kleiner,
  desto besser. Der Optimierer sucht ein Minimum.
- **Move (Nachbarschaft):** eine kleine Änderung an einem Plan (eine Zuordnung
  umsetzen, tauschen, entfernen). Die Menge der erreichbaren Pläne bestimmt, wie gut
  die Suche entkommen kann.

---

## 2. Simulated Annealing (Aufsichtenplanung)

**Simulated Annealing (SA)** ist eine lokale Suche, die dem physikalischen Abkühlen
(„annealing") eines Metalls nachempfunden ist: bei hoher „Temperatur" werden auch
*verschlechternde* Schritte akzeptiert (um lokalen Minima zu entkommen), bei sinkender
Temperatur immer seltener, bis die Suche am Ende nur noch Verbesserungen annimmt.

### Ablauf — `Optimize` in [optimizer.go](../plexams/invigplan/optimizer.go)

1. **Startplan (greedy):** `Greedy` füllt die offenen Positionen *am-stärksten-
   eingeschränkt zuerst* (die Position mit den wenigsten in Frage kommenden Aufsichten)
   und wählt jeweils die zulässige Aufsicht, die ihrem Soll-Minuten-Ziel am weitesten
   hinterherhinkt. So ist der Start schon fair und hart-zulässig.

2. **Iterieren:** in jeder Runde wird ein zufälliger **Move** vorgeschlagen
   (`proposeMove`):
   - 70 % eine Position **neu zuordnen**,
   - 10 % eine Position **freigeben** (unassign),
   - 20 % zwei Positionen **tauschen**.

3. **Zulässigkeit zuerst:** der Move wird angewendet und sofort gegen die harten
   Bedingungen geprüft (`feasible` / `Registry.Allows`). Verletzt er eine, wird er
   **rückgängig gemacht** (`undo`) — der Plan bleibt also *immer* hart-zulässig.

4. **Akzeptanzregel (Metropolis):** ist `delta = neueKosten − alteKosten`:
   - `delta ≤ 0` (besser oder gleich): **immer** akzeptieren.
   - `delta > 0` (schlechter): nur mit Wahrscheinlichkeit `exp(−delta / T)`
     akzeptieren. Bei hoher Temperatur `T` ist das oft, bei niedriger fast nie.

5. **Abkühlung:** `temperature` ist ein **geometrisches** Schema von `StartTemp` auf
   `EndTemp`. Anfangs darf die Suche „bergauf" wandern, am Ende wird sie gierig.

6. **Bestplan merken / früher Abbruch:** der beste je gesehene Plan wird festgehalten.
   Mit `StopOnBalance` endet die Suche vorzeitig, *sobald sie konvergiert ist* — d. h.
   die Temperatur ist nahe am Boden, der Bestplan hat sich seit `StagnationLimit`
   Runden nicht verbessert, und der Plan ist ausbalanciert und vollständig belegt.

### Warum SA hier?

Die Aufsichtenverteilung hat viele, teils gegenläufige weiche Ziele (Fairness der
Minuten, Verteilung über Tage, Spannweite, …) bei vielen harten Bedingungen. Eine rein
gierige Lösung bleibt leicht in einem mittelmäßigen lokalen Optimum hängen; SA kann
durch das gelegentliche Akzeptieren schlechterer Zwischenschritte entkommen und findet
in der Praxis deutlich bessere Pläne.

---

## 3. DSATUR + SA-Reparatur (Vorplanung SEB/EXaHM)

### Das Problem ist Bin-Packing mit weichen Überschneidungen

Die Vorplanung verteilt vorgeplante Prüfungen (`PreplanExam`) auf die MUC.DAI-Slots,
für die wir bereits Anny-Räume gebucht haben. Formal ist es **Bin-Packing mit weichen
Konflikten** (verwandt mit Graphfärbung / Timetabling):

- **Knoten:** jede Prüfung (bzw. eine `same-slot`-Gruppe als eine unteilbare *Einheit*).
- **Behälter (Slot):** jeder gebuchte Anny-Slot mit einem **Sitzplatz-Limit** (≈ 90 %
  der gebuchten physischen Plätze, `preplanCapacityFactor = 0.9`, also „nie randvoll").
  Das ist die **einzige harte** Schranke.
- **Same-slot (hart):** manche Prüfungen *müssen* zusammen (z. B. beide Varianten von
  „Betriebssysteme I") — sie werden zu einer Einheit verschmolzen.
- **Studiengang-Überschneidung (weich):** zwei Prüfungen desselben Studiengangs *dürfen*
  im selben Slot liegen (in verschiedenen Anny-Räumen) — das ist erlaubt, weil es nicht
  zwingend dieselben Studierenden betrifft. Es wird aber **bestraft** und damit
  gespreizt: stärker für denselben **Slot** (`preplanSameSlotProgWeight`), schwächer für
  denselben **Tag** (`preplanSameDayProgWeight`).
- **Priorität:** **alle EXaHM** und **große SEB** werden bevorzugt platziert und nie
  zugunsten kleinerer fallen gelassen (hoher Drop-Kostenzuschlag). **Kleine SEB** (die
  in einen einzelnen R-Bau-Laborraum passen, Größe ≤ größter Nicht-Anny-SEB-Raum) werden
  gar nicht erst in Anny gelegt, sondern mit der Anmerkung „im R-Bau planen" markiert.
- **Fixiert:** Prüfungen mit `isFixed` behalten ihren Slot und werden vorbelegt.

### Warum die alte „First-Fit"-Heuristik versagte

Die frühere Lösung war **First-Fit, größte zuerst**: jede Prüfung in den ersten Slot,
der gerade passt. Das ist eine der schwächsten Heuristiken. Sobald die ersten Slots
„verstopft" waren, fanden spätere Prüfungen *keinen* Slot mehr — obwohl eine andere
**Reihenfolge** mehr untergebracht hätte.

> **Wichtig:** Wenn der Gesamtbedarf die gebuchte Anny-Kapazität übersteigt (in
> `Test26SS`: ~1379 Plätze „must-place" vs. ~972 nutzbare Plätze in 10 gebuchten Slots),
> kann *kein* Algorithmus alles unterbringen — dann müssen erst **mehr Anny-Slots
> gebucht** werden. Der Algorithmus platziert in dem Fall alle EXaHM + die größten SEB
> und meldet den Engpass.

### Phase A — DSATUR (konstruktiv)

**DSATUR** (degree of saturation) färbt immer den **am stärksten eingeschränkten**
Knoten zuerst — nicht den größten. Hier: die Einheit mit den wenigsten Slots, in die sie
kapazitiv noch passt.

In `solvePreplan` ([preplan_solve.go](../plexams/preplan_solve.go)):

1. Berechne für jede noch nicht platzierte Einheit die Menge der Slots, in die sie
   **kapazitiv passt**.
2. Wähle die Einheit mit den **wenigsten** passenden Slots zuerst (`dsaturBefore`); bei
   Gleichstand: höhere Priorität (EXaHM/große SEB über die Drop-Kosten), dann kleinere
   ID — deterministisch.
3. Platziere sie im Slot, der die **wenigsten Studiengang-Überschneidungen** verursacht
   (erst Slot-, dann Tagesebene, `chooseSlot`), bei Gleichstand in den mit der meisten
   Restkapazität.
4. Passt eine Einheit in keinen Slot, wird sie übersprungen und der Reparatur überlassen.

DSATUR legt damit gezielt die „schwierigen" (großen) Prüfungen früh und packt den Rest
dazu.

### Phase B — Simulated-Annealing-Reparatur (nur bei Bedarf)

Lässt Phase A nichts liegen, ist man fertig. Andernfalls läuft eine **SA-Reparatur**
nach demselben Prinzip wie die Aufsichtenplanung, aber eigenständig und auf das
Vorplanungsproblem zugeschnitten:

- **Kostenfunktion:** Summe der **Drop-Kosten** aller nicht platzierten Einheiten plus
  der weiche **Studiengang-Spreizungsterm** (gleicher Studiengang im selben Slot bzw.
  am selben Tag). Die Drop-Kosten dominieren, also platziert die Suche zuerst so viele
  Prüfungen wie möglich. EXaHM bekommt einen sehr hohen Zuschlag (`preplanExahmKeep`),
  große SEB über den Sitzplatzterm — so werden diese **nie** die Fallenden.
- **Move mit Ejection:** eine zufällige Einheit wird in einen zufälligen Slot
  verschoben; reicht dort die **Kapazität** nicht, werden bis zu `preplanEjectDepth`
  (= 3) der *kleinsten* dortigen Einheiten „hinausgeworfen" (auf *unplaziert* gesetzt),
  um Platz zu schaffen. Fixierte Belegungen werden nie hinausgeworfen. Der Move hält den
  Slot innerhalb der Kapazität; Studiengang-Überschneidungen sind erlaubt und schlagen
  nur über den weichen Term zu Buche.
- **Akzeptanz & Abkühlung:** identisch zu SA oben — `delta ≤ 0` immer, sonst mit
  `exp(−delta/T)`; `T` kühlt geometrisch von `preplanSAStartTemp` auf
  `preplanSAEndTemp`. Der beste je gesehene Zustand wird zurückgegeben.
- **Deterministisch:** fester Zufalls-Seed (`rand.NewSource(1)`), damit dieselbe
  Eingabe immer dasselbe Ergebnis liefert.

Die hinausgeworfenen Einheiten werden in späteren Iterationen wieder eingeplant; über
die Kostenfunktion „wandert" so eine wichtige Prüfung in einen vollen Slot und schiebt
weniger wichtige in andere Slots — eine Kettenreaktion, die First-Fit nicht leisten
kann.

### Einordnung

Bewusst **keine** Wiederverwendung der invigplan-SA-Engine: deren `Problem`/`Registry`
ist auf Aufsichten zugeschnitten. Die Vorplanungs-Reparatur ist im selben *Geist*, aber
kompakt und in sich geschlossen. Eine generische „items→slots"-Engine ist erst
geplant, wenn der Terminplan-Generator als zweiter echter Abnehmer dazukommt (siehe
[cli-to-gui-migration-plan.md](cli-to-gui-migration-plan.md)).

---

## 4. Parameter zum Nachjustieren

### Aufsichtenplanung — `DefaultOptions()` in [optimizer.go](../plexams/invigplan/optimizer.go)

| Parameter         | Default     | Wirkung |
|-------------------|-------------|---------|
| `Iterations`      | 1 000 000   | Obergrenze der Suchschritte. |
| `StartTemp`       | 20 000      | Anfangstemperatur — wie „mutig" am Start. |
| `EndTemp`         | 0.5         | Endtemperatur — am Ende nahezu gierig. |
| `Seed`            | 1           | Zufalls-Seed (Reproduzierbarkeit). |
| `StopOnBalance`   | true        | Früher Abbruch nach Konvergenz. |
| `StagnationLimit` | 30 000      | Runden ohne Bestplan-Verbesserung vor Abbruch. |

### Vorplanung — Konstanten in [preplan_solve.go](../plexams/preplan_solve.go)

| Konstante                   | Default   | Wirkung |
|-----------------------------|-----------|---------|
| `preplanDropBase`           | 10 000    | Grundkosten, eine Einheit *nicht* zu platzieren. |
| `preplanExahmKeep`          | 1 000 000 | Zuschlag, damit EXaHM nie gedroppt wird. |
| `preplanSameSlotProgWeight` | 50        | Strafe je Studiengang-Paar im selben Slot. |
| `preplanSameDayProgWeight`  | 5         | Strafe je Studiengang-Paar am selben Tag. |
| `preplanSAIterations`       | 20 000    | Schritte der Reparatur-Suche. |
| `preplanSAStartTemp`        | 20 000    | Anfangstemperatur der Reparatur. |
| `preplanSAEndTemp`          | 1         | Endtemperatur der Reparatur. |
| `preplanEjectDepth`         | 3         | Max. Einheiten, die ein Move aus einem Slot wirft. |

Und in [preplan_assign.go](../plexams/preplan_assign.go):

| Konstante               | Default | Wirkung |
|-------------------------|---------|---------|
| `preplanCapacityFactor` | 0.9     | Nutzbarer Anteil der gebuchten Sitzplätze (10 % frei). |

---

## 5. Tests

Der Vorplanungs-Solver ist deterministisch und direkt testbar:
[plexams/preplan_solve_test.go](../plexams/preplan_solve_test.go) prüft u. a., dass bei
ausreichender Kapazität *alle* Einheiten platziert werden (auch wenn sie denselben
Studiengang teilen), dass bei Engpass die *kleinste* SEB gedroppt wird und EXaHM/große
SEB bleiben, sowie die Spreizung gleicher Studiengänge über Slots und Tage. Der
invigplan-Optimizer hat eigene Tests im selben Paket.
