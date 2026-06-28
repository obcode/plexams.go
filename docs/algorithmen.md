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

### Das Problem ist Graphfärbung mit Kapazität

Die Vorplanung verteilt vorgeplante Prüfungen (`PreplanExam`) auf die MUC.DAI-Slots,
für die wir bereits Anny-Räume gebucht haben. Das ist formal **Graphfärbung mit
Behälter-Kapazitäten** (ein Timetabling-Problem):

- **Knoten:** jede Prüfung (bzw. eine `same-slot`-Gruppe als eine unteilbare *Einheit*).
- **Kante:** zwei Prüfungen mit gemeinsamem Studiengang dürfen **nicht** in denselben
  Slot (harte Bedingung „nie gleichzeitig").
- **Farbe:** jeder gebuchte Anny-Slot ist eine Farbe mit einem **Sitzplatz-Limit**
  (≈ 90 % der gebuchten physischen Plätze, `preplanCapacityFactor = 0.9`, also „nie
  randvoll").
- **Weiche Ziele:** gleichen Studiengang über verschiedene **Tage** spreizen; EXaHM
  und große SEB bevorzugt platzieren (kleine SEB dürfen notfalls leer ausgehen).
- **Fixiert:** Prüfungen mit `isFixed` behalten ihren Slot und werden vorbelegt.

### Warum die alte „First-Fit"-Heuristik versagte

Die frühere Lösung war **First-Fit, größte zuerst**: jede Prüfung in den ersten Slot,
der gerade passt. Das ist eine der schwächsten Färbe-Heuristiken. Sobald die ersten
Slots mit Studiengängen „verstopft" sind, finden spätere Prüfungen *keinen* Slot mehr —
obwohl eine andere **Reihenfolge** alles untergebracht hätte. Konkret blieben in
`Test26SS` 11 von 27 Prüfungen ohne Slot, trotz freier Räume. (Der Klassiker dazu ist
der *Crown-Graph*: 2-färbbar, aber First-Fit braucht 3 Farben und strandet bei nur 2
verfügbaren Slots — genau dieser Fall wird im Test reproduziert.)

### Phase A — DSATUR (konstruktiv)

**DSATUR** (degree of saturation) ist eine Färbe-Heuristik, die immer den **am
stärksten eingeschränkten** Knoten zuerst färbt — nicht den größten. „Saturation" =
wie viele Farben/Slots für diesen Knoten bereits blockiert sind.

In `solvePreplan` ([preplan_solve.go](../plexams/preplan_solve.go)):

1. Berechne für jede noch nicht platzierte Einheit die Menge der **zulässigen** Slots
   (Kapazität frei *und* kein Studiengang-Konflikt).
2. Wähle die Einheit mit den **wenigsten** zulässigen Slots zuerst (`dsaturBefore`);
   bei Gleichstand: höhere Priorität (EXaHM/große SEB über die Drop-Kosten), dann
   kleinere ID — deterministisch.
3. Platziere sie im Slot, der die **Tage am besten spreizt** (`chooseSlot`), bei
   Gleichstand in den mit der meisten Restkapazität.
4. Bleibt für eine Einheit kein zulässiger Slot, wird sie zunächst übersprungen und
   der Reparatur überlassen.

DSATUR legt damit gezielt die „schwierigen" Prüfungen früh und packt den Rest dazu —
das löst die allermeisten Fälle bereits vollständig.

### Phase B — Simulated-Annealing-Reparatur (nur bei Bedarf)

Lässt Phase A nichts liegen, ist man fertig. Andernfalls läuft eine **SA-Reparatur**
nach demselben Prinzip wie die Aufsichtenplanung, aber eigenständig und auf das
Vorplanungsproblem zugeschnitten:

- **Kostenfunktion:** Summe der **Drop-Kosten** aller nicht platzierten Einheiten plus
  ein kleiner Tages-Spreizungsterm. Die Drop-Kosten dominieren, also platziert die
  Suche zuerst so viele Prüfungen wie möglich. EXaHM bekommt einen sehr hohen Zuschlag
  (`preplanExahmKeep`), große SEB über den Sitzplatzterm — so werden diese **nie** die
  Fallenden; notfalls bleibt eine *kleine* SEB ohne Slot.
- **Move mit Ejection:** eine zufällige Einheit wird in einen zufälligen Slot
  verschoben; bis zu `preplanEjectDepth` (= 2) dort konfligierende Einheiten werden
  „hinausgeworfen" (auf *unplaziert* gesetzt), um Platz zu schaffen. Fixierte
  Belegungen werden nie hinausgeworfen. Der Move hält den Slot hart-zulässig
  (Kapazität + Studiengang-disjunkt).
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

| Konstante              | Default   | Wirkung |
|------------------------|-----------|---------|
| `preplanDropBase`      | 10 000    | Grundkosten, eine Einheit *nicht* zu platzieren. |
| `preplanExahmKeep`     | 1 000 000 | Zuschlag, damit EXaHM nie gedroppt wird. |
| `preplanDaySpreadCost` | 1         | Strafe je doppeltem Studiengang am selben Tag. |
| `preplanSAIterations`  | 20 000    | Schritte der Reparatur-Suche. |
| `preplanSAStartTemp`   | 20 000    | Anfangstemperatur der Reparatur. |
| `preplanSAEndTemp`     | 1         | Endtemperatur der Reparatur. |
| `preplanEjectDepth`    | 2         | Max. Einheiten, die ein Move aus einem Slot wirft. |

Und in [preplan_assign.go](../plexams/preplan_assign.go):

| Konstante               | Default | Wirkung |
|-------------------------|---------|---------|
| `preplanCapacityFactor` | 0.9     | Nutzbarer Anteil der gebuchten Sitzplätze (10 % frei). |

---

## 5. Tests

Der Vorplanungs-Solver ist deterministisch und direkt testbar:
[plexams/preplan_solve_test.go](../plexams/preplan_solve_test.go) prüft u. a. den
Crown-Graph (alle Einheiten trotz First-Fit-Falle platziert), das korrekte Droppen der
*kleinsten* SEB bei Kapazitätsengpass und die Tages-Spreizung. Der invigplan-Optimizer
hat eigene Tests im selben Paket.
