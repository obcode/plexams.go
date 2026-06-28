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
- **Behälter (Slot):** **jeder reguläre Prüfungs-Slot** (08:30/10:30/…), in dem wir
  Anny-Räume gebucht haben — *nicht* nur die MUC.DAI-Slots. Sitzplatz-Limit =
  `preplanCapacityFactor` × gebuchte physische Plätze (Default 1.0 = randvoll). Das ist
  die **einzige harte** Schranke.
- **MUC.DAI-Slots (reserviert):** Prüfungen mit einem MUC.DAI-Studiengang (DE/GS/ID)
  dürfen **nur** in (gebuchte) MUC.DAI-Slots; alle anderen Prüfungen in jeden gebuchten
  Slot (`preplanUnit.allowedSlots`).
- **Same-slot (hart):** manche Prüfungen *müssen* zusammen (z. B. beide Varianten von
  „Betriebssysteme I") — sie werden zu einer Einheit verschmolzen.
- **Studiengang-Überschneidung (weich, distanzbasiert):** zwei Prüfungen desselben
  Studiengangs *dürfen* im selben Slot liegen (verschiedene Anny-Räume) — erlaubt, weil
  es nicht zwingend dieselben Studierenden sind. Es wird aber über die **zeitliche
  Distanz** bestraft (`proximityPenalty`): voll bei selbem Slot, weniger je größer der
  Slot-Abstand am selben Tag, **0 sobald an verschiedenen Tagen**. So gilt „möglichst
  verschiedene Tage; wenn selber Tag, maximale Slot-Entfernung".
- **Explizit „nicht gleichzeitig" (weich, stärker):** für Paare, die *dieselben*
  Studierenden betreffen, ohne dass der Studiengang das zeigt, kann pro Prüfung ein
  Konfliktpartner gesetzt werden (`PreplanExam.NotSameSlot`, Mutation
  `setPreplanExamNotSameSlot`, symmetrisch). Gleiche Distanzlogik, aber mit dem höheren
  Gewicht `preplanExplicitConflictWeight`.
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

> **Wichtig:** Wenn der Gesamtbedarf die gebuchte Anny-Kapazität übersteigt, kann *kein*
> Algorithmus alles unterbringen — dann müssen erst **mehr Anny-Slots gebucht** werden.
> Der Algorithmus platziert in dem Fall alle EXaHM + die größten SEB und meldet den
> Engpass. (Anfänglich wurden in `Test26SS` zu wenige Slots erkannt, weil die Kandidaten
> fälschlich auf MUC.DAI-Slots beschränkt waren — siehe oben: jetzt zählen alle
> gebuchten regulären Slots.)

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
3. Platziere sie im Slot, der die **geringste Konflikt-Nähe** verursacht (`chooseSlot`
   über `proximityPenalty`), bei Gleichstand in den mit der meisten Restkapazität.
4. Passt eine Einheit in keinen Slot, wird sie übersprungen und der SA-Phase überlassen.

DSATUR legt damit gezielt die „schwierigen" (großen) Prüfungen früh und packt den Rest
dazu.

### Phase B — Simulated Annealing (immer)

Nach DSATUR läuft **immer** eine SA-Phase — nicht nur, um Übriggebliebenes zu platzieren,
sondern auch, um die **weiche Spreizung** zu optimieren (DSATUR-Greedy allein verteilt
gleiche Studiengänge sonst nicht gut genug). Prinzip wie bei der Aufsichtenplanung, aber
eigenständig:

- **Kostenfunktion:** Summe der **Drop-Kosten** aller nicht platzierten Einheiten plus
  der distanzbasierten **Konflikt-Nähe** (`proximityPenalty`) über alle konfligierenden
  Paare (Studiengang oder explizit). Die Drop-Kosten dominieren, also wird zuerst
  platziert; EXaHM (`preplanExahmKeep`) und große SEB sind **nie** die Fallenden.
- **Zwei Move-Typen:** (1) **Swap** — zwei platzierte Einheiten tauschen ihre Slots
  (alle bleiben platziert; optimiert die Spreizung in vollen Plänen); (2) **Relocate
  mit Ejection** — eine Einheit in einen Slot verschieben; reicht die **Kapazität**
  nicht, werden bis zu `preplanEjectDepth` (= 3) der *kleinsten* dortigen Einheiten
  hinausgeworfen. Fixierte Belegungen werden nie verschoben/geworfen.
- **Akzeptanz & Abkühlung:** `delta ≤ 0` immer, sonst mit `exp(−delta/T)`; `T` kühlt
  geometrisch von `preplanSAStartTemp` auf `preplanSAEndTemp`. Der beste je gesehene
  Zustand wird zurückgegeben. Fester Seed (`rand.NewSource(1)`) → deterministisch.

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
| `preplanDropBase`               | 10 000    | Grundkosten, eine Einheit *nicht* zu platzieren. |
| `preplanExahmKeep`              | 1 000 000 | Zuschlag, damit EXaHM nie gedroppt wird. |
| `preplanProgramConflictWeight`  | 100       | Basisstrafe für gemeinsamen Studiengang (× Nähe). |
| `preplanExplicitConflictWeight` | 1 000     | Basisstrafe für explizites „nicht gleichzeitig" (× Nähe). |
| `preplanSAIterations`           | 20 000    | Schritte der SA-Suche. |
| `preplanSAStartTemp`            | 20 000    | Anfangstemperatur. |
| `preplanSAEndTemp`              | 1         | Endtemperatur. |
| `preplanEjectDepth`             | 3         | Max. Einheiten, die ein Relocate-Move aus einem Slot wirft. |

Und in [preplan_assign.go](../plexams/preplan_assign.go):

| Konstante               | Default | Wirkung |
|-------------------------|---------|---------|
| `preplanCapacityFactor` | 1.0     | Nutzbarer Anteil der gebuchten Sitzplätze (1.0 = randvoll). |

---

## 5. Tests

Der Vorplanungs-Solver ist deterministisch und direkt testbar:
[plexams/preplan_solve_test.go](../plexams/preplan_solve_test.go) prüft u. a., dass bei
ausreichender Kapazität *alle* Einheiten platziert werden (auch bei gemeinsamem
Studiengang), dass bei Engpass die *kleinste* SEB gedroppt wird und EXaHM/große SEB
bleiben, die Spreizung gleicher Studiengänge bzw. expliziter „nicht gleichzeitig"-Paare
über Slots und Tage, die `proximityPenalty`-Distanzkurve sowie die `allowedSlots`-
Restriktion (MUC.DAI). Der invigplan-Optimizer hat eigene Tests im selben Paket.
