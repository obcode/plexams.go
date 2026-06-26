package plexams

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/agnivade/levenshtein"
	set "github.com/deckarep/golang-set/v2"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) ExamerInPlan(ctx context.Context) ([]*model.ExamerInPlan, error) {
	return p.dbClient.ExamerInPlan(ctx)
}

// ExamersWithExamsPlannedByMe returns the main examers of all planned exams that
// are planned by me (i.e. not flagged NotPlannedByMe), as full teachers, sorted
// by short name. This is the set of examers the planner is responsible for (e.g.
// for cover-page mails).
func (p *Plexams) ExamersWithExamsPlannedByMe(ctx context.Context) ([]*model.Teacher, error) {
	plannedExams, err := p.PlannedExams(ctx)
	if err != nil {
		return nil, err
	}

	examerIDs := set.NewSet[int]()
	for _, exam := range plannedExams {
		if exam.PlanEntry == nil {
			continue
		}
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		examerIDs.Add(exam.ZpaExam.MainExamerID)
	}

	teachers := make([]*model.Teacher, 0, examerIDs.Cardinality())
	for examerID := range examerIDs.Iter() {
		teacher, err := p.GetTeacher(ctx, examerID)
		if err != nil {
			log.Error().Err(err).Int("examerID", examerID).Msg("cannot get teacher")
			continue
		}
		teachers = append(teachers, teacher)
	}

	sort.Slice(teachers, func(i, j int) bool { return teachers[i].Shortname < teachers[j].Shortname })

	return teachers, nil
}

// primussExamIndex holds all Primuss exams of all programs loaded once, indexed by
// program -> ancode and program -> all exams. It lets connected exams be computed
// on the fly without per-exam/per-program DB lookups.
type primussExamIndex struct {
	byAncode  map[string]map[int]*model.PrimussExam
	byProgram map[string][]*model.PrimussExam
}

// buildPrimussExamIndex loads every program's Primuss exams once (~one query per
// program) and indexes them in memory.
func (p *Plexams) buildPrimussExamIndex(ctx context.Context, programs []string) (*primussExamIndex, error) {
	idx := &primussExamIndex{
		byAncode:  make(map[string]map[int]*model.PrimussExam, len(programs)),
		byProgram: make(map[string][]*model.PrimussExam, len(programs)),
	}
	for _, program := range programs {
		exams, err := p.dbClient.PrimussExamsForProgram(ctx, program)
		if err != nil {
			// a program may not be (fully) imported yet — treat as empty, do not fail
			log.Debug().Err(err).Str("program", program).Msg("cannot get primuss exams for program")
			continue
		}
		byAncode := make(map[int]*model.PrimussExam, len(exams))
		for _, exam := range exams {
			byAncode[exam.AnCode] = exam
		}
		idx.byAncode[program] = byAncode
		idx.byProgram[program] = exams
	}
	return idx, nil
}

// get returns the Primuss exam of a program/ancode from the index, if present.
func (idx *primussExamIndex) get(program string, ancode int) (*model.PrimussExam, bool) {
	if byAncode, ok := idx.byAncode[program]; ok {
		exam, ok := byAncode[ancode]
		return exam, ok
	}
	return nil, false
}

// computeConnectedZPAExam connects one ZPA exam to its Primuss registrations using
// the preloaded index (pure, no DB access).
func (p *Plexams) computeConnectedZPAExam(zpaExam *model.ZPAExam, allPrograms []string, idx *primussExamIndex) *model.ConnectedExam {
	primussExams := make([]*model.PrimussExam, 0)
	warnings := make([]*model.ConnectedExamWarning, 0)

	// Connect via the primuss ancodes the ZPA exam carries.
	for _, primussAncode := range zpaExam.PrimussAncodes {
		if primussAncode.Ancode == 0 || primussAncode.Ancode == -1 {
			continue
		}

		// Skip old (retired) programs
		skipProgram := false
		for _, oldProgram := range p.zpa.oldprograms {
			if primussAncode.Program == oldProgram {
				skipProgram = true
				break
			}
		}
		if skipProgram {
			continue
		}

		primussExam, ok := idx.get(primussAncode.Program, primussAncode.Ancode)
		if !ok {
			warnings = append(warnings, primussNotFoundWarning(zpaExam, primussAncode.Program, primussAncode.Ancode, idx))
		} else {
			primussExams = append(primussExams, primussExam)
			if w := examerMismatchWarning(primussAncode.Program, primussAncode.Ancode, zpaExam.MainExamer, primussExam.MainExamer); w != nil {
				warnings = append(warnings, w)
			}
		}
	}

	otherPrograms := make([]string, 0, len(allPrograms)-len(zpaExam.PrimussAncodes))
OUTER:
	for _, aP := range allPrograms {
		for _, pe := range primussExams {
			if aP == pe.Program {
				continue OUTER
			}
		}
		otherPrograms = append(otherPrograms, aP)
	}

	var otherPrimussExams []*model.PrimussExam

	// A primuss exam with the same ancode in another program: a real additional
	// program when the examer matches, otherwise most likely a coincidental number
	// (e.g. MUC.DAI) — only a hint.
	for _, program := range otherPrograms {
		primussExam, ok := idx.get(program, zpaExam.AnCode)
		if ok {
			if otherPrimussExams == nil {
				otherPrimussExams = make([]*model.PrimussExam, 0)
			}
			otherPrimussExams = append(otherPrimussExams, primussExam)
			if sameExamer(zpaExam.MainExamer, primussExam.MainExamer) {
				warnings = append(warnings, primussRefWarning("warning",
					fmt.Sprintf("gleiche Nummer auch in %s/%d (%s: %s) — zusätzlicher Studiengang?",
						program, zpaExam.AnCode, primussExam.MainExamer, primussExam.Module),
					program, primussExam.AnCode, primussExam.Module, primussExam.MainExamer))
			} else {
				warnings = append(warnings, primussRefWarning("info",
					fmt.Sprintf("gleiche Nummer in %s/%d (anderer Prüfer: %s, %s) — vermutlich Zufall",
						program, zpaExam.AnCode, primussExam.MainExamer, primussExam.Module),
					program, primussExam.AnCode, primussExam.Module, primussExam.MainExamer))
			}
		}
	}

	return &model.ConnectedExam{
		ZpaExam:           zpaExam,
		PrimussExams:      primussExams,
		OtherPrimussExams: otherPrimussExams,
		Warnings:          warnings,
	}
}

// primussNotFoundWarning classifies a missing mapped primuss exam: when a likely
// counterpart (same examer, similar module) exists in the program it is a warning
// with a concrete suggestion, otherwise just an info (e.g. a program that isn't
// (fully) imported this semester).
func primussNotFoundWarning(zpaExam *model.ZPAExam, program string, ancode int, idx *primussExamIndex) *model.ConnectedExamWarning {
	if suggestion := suggestPrimussExam(idx, program, zpaExam.MainExamer, zpaExam.Module); suggestion != nil {
		return primussRefWarning("warning",
			fmt.Sprintf("%s/%d nicht gefunden — evtl. %s/%d (gleicher Prüfer, Modul „%s“)",
				program, ancode, program, suggestion.AnCode, suggestion.Module),
			program, suggestion.AnCode, suggestion.Module, suggestion.MainExamer)
	}
	return primussRefWarning("info",
		fmt.Sprintf("%s/%d nicht gefunden", program, ancode),
		program, ancode, "", "")
}

// primussRefWarning builds a warning that references a specific Primuss exam, so
// the GUI can offer an add/fix action and show the module name.
func primussRefWarning(level, message, program string, ancode int, module, examer string) *model.ConnectedExamWarning {
	w := &model.ConnectedExamWarning{Level: level, Message: message}
	if program != "" {
		w.Program = &program
	}
	if ancode != 0 {
		a := ancode
		w.Ancode = &a
	}
	if module != "" {
		w.Module = &module
	}
	if examer != "" {
		w.Examer = &examer
	}
	return w
}

// suggestPrimussExam looks in one program for a primuss exam with the same examer
// (surname) and a similar module name as the given ZPA exam.
func suggestPrimussExam(idx *primussExamIndex, program, zpaExamer, zpaModule string) *model.PrimussExam {
	for _, exam := range idx.byProgram[program] {
		if sameExamer(zpaExamer, exam.MainExamer) && similarModule(zpaModule, exam.Module) {
			return exam
		}
	}
	return nil
}

// examerMismatchWarning compares the ZPA and Primuss examer of a matched exam:
// identical -> nil (silent); same surname, different spelling -> info; different
// surname -> warning.
func examerMismatchWarning(program string, ancode int, zpaExamer, primussExamer string) *model.ConnectedExamWarning {
	if strings.TrimSpace(zpaExamer) == strings.TrimSpace(primussExamer) {
		return nil
	}
	if sameExamer(zpaExamer, primussExamer) {
		return primussRefWarning("info",
			fmt.Sprintf("Prüfer-Schreibweise weicht ab (%s/%d): ZPA „%s“ / Primuss „%s“", program, ancode, zpaExamer, primussExamer),
			program, ancode, "", primussExamer)
	}
	return primussRefWarning("warning",
		fmt.Sprintf("Prüfer unterschiedlich (%s/%d): ZPA „%s“ / Primuss „%s“", program, ancode, zpaExamer, primussExamer),
		program, ancode, "", primussExamer)
}

// examerSurname reduces a name to its lowercased surname for comparison:
// "Orehek, Martin" and "Orehek M." both -> "orehek".
func examerSurname(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.IndexAny(name, ","); i >= 0 {
		name = name[:i]
	} else if i := strings.IndexAny(name, " "); i >= 0 {
		name = name[:i]
	}
	return foldUmlauts(strings.ToLower(strings.TrimSpace(name)))
}

// foldUmlauts normalizes German umlaut spellings so that ASCII (ZPA) and umlaut
// (Primuss) variants of a name compare equal, e.g. "böhm" == "boehm".
func foldUmlauts(s string) string {
	r := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss")
	return r.Replace(s)
}

func sameExamer(a, b string) bool {
	sa, sb := examerSurname(a), examerSurname(b)
	return sa != "" && sa == sb
}

// similarModule reports whether two module names are close (case-insensitive,
// whitespace-normalized): equal, one contains the other, or small edit distance.
func similarModule(a, b string) bool {
	na, nb := normalizeModule(a), normalizeModule(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb || strings.Contains(na, nb) || strings.Contains(nb, na) {
		return true
	}
	longer := len(na)
	if len(nb) > longer {
		longer = len(nb)
	}
	threshold := longer / 4
	if threshold < 2 {
		threshold = 2
	}
	return levenshtein.ComputeDistance(na, nb) <= threshold
}

func normalizeModule(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// computeConnectedNonZPAExam connects one non-ZPA (locally created) exam to its
// Primuss registrations using the preloaded index (pure, no DB access).
func computeConnectedNonZPAExam(nonZPAExam *model.ZPAExam, idx *primussExamIndex) *model.ConnectedExam {
	primussExams := make([]*model.PrimussExam, 0)
	warnings := make([]*model.ConnectedExamWarning, 0)
	for _, primuss := range nonZPAExam.PrimussAncodes {
		primussExam, ok := idx.get(primuss.Program, primuss.Ancode)
		if !ok {
			warnings = append(warnings, primussRefWarning("info",
				fmt.Sprintf("%s/%d nicht gefunden", primuss.Program, primuss.Ancode),
				primuss.Program, primuss.Ancode, "", ""))
			continue
		}
		primussExams = append(primussExams, primussExam)
	}

	return &model.ConnectedExam{
		ZpaExam:           nonZPAExam,
		PrimussExams:      primussExams,
		OtherPrimussExams: nil,
		Warnings:          warnings,
	}
}

// GetConnectedExams computes all connected exams on the fly: it loads all Primuss
// exams once into an index and connects every ZPA-exam-to-plan and non-ZPA exam in
// memory, so the result is always consistent with the current data (no cache).
func (p *Plexams) GetConnectedExams(ctx context.Context) ([]*model.ConnectedExam, error) {
	allPrograms, err := p.dbClient.GetPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get programs")
		return nil, err
	}

	idx, err := p.buildPrimussExamIndex(ctx, allPrograms)
	if err != nil {
		return nil, err
	}

	zpaExams, err := p.GetZpaExamsToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
		return nil, err
	}

	exams := make([]*model.ConnectedExam, 0, len(zpaExams))
	for _, zpaExam := range zpaExams {
		exams = append(exams, p.computeConnectedZPAExam(zpaExam, allPrograms, idx))
	}

	nonZPAExams, err := p.dbClient.NonZpaExams(ctx)
	if err == nil {
		for _, nonZPAExam := range nonZPAExams {
			exams = append(exams, computeConnectedNonZPAExam(nonZPAExam, idx))
		}
	}

	sort.Slice(exams, func(i, j int) bool { return exams[i].ZpaExam.AnCode < exams[j].ZpaExam.AnCode })

	return exams, nil
}

// GetConnectedExam computes a single connected exam on the fly.
func (p *Plexams) GetConnectedExam(ctx context.Context, ancode int) (*model.ConnectedExam, error) {
	allPrograms, err := p.dbClient.GetPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get programs")
		return nil, err
	}

	idx, err := p.buildPrimussExamIndex(ctx, allPrograms)
	if err != nil {
		return nil, err
	}

	if ancode < 1000 {
		zpaExam, err := p.dbClient.GetZpaExamByAncode(ctx, ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get zpa exam")
			return nil, err
		}
		return p.computeConnectedZPAExam(zpaExam, allPrograms, idx), nil
	}

	nonZPAExam, err := p.dbClient.NonZpaExam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get non zpa exam")
		return nil, err
	}
	return computeConnectedNonZPAExam(nonZPAExam, idx), nil
}

// AddPrimussAncode adds a Primuss ancode mapping to a ZPA exam and returns the
// freshly computed connected exam.
func (p *Plexams) AddPrimussAncode(ctx context.Context, zpaAncode int, program string, primussAncode int) (*model.ConnectedExam, error) {
	if err := p.dbClient.AddAncode(ctx, zpaAncode, program, primussAncode); err != nil {
		return nil, err
	}
	return p.GetConnectedExam(ctx, zpaAncode)
}

// RemovePrimussAncode removes a (manually added) Primuss ancode mapping of a program
// from a ZPA exam and returns the freshly computed connected exam.
func (p *Plexams) RemovePrimussAncode(ctx context.Context, zpaAncode int, program string) (*model.ConnectedExam, error) {
	if _, err := p.dbClient.RemoveAddedAncode(ctx, zpaAncode, program); err != nil {
		return nil, err
	}
	return p.GetConnectedExam(ctx, zpaAncode)
}

// FixPrimussAncode renumbers a Primuss exam (exam + student regs + conflicts) within
// a program and returns the freshly computed connected exam of the given ZPA exam.
func (p *Plexams) FixPrimussAncode(ctx context.Context, zpaAncode int, program string, fromAncode, toAncode int) (*model.ConnectedExam, error) {
	if _, err := p.ChangeAncode(ctx, program, fromAncode, toAncode); err != nil {
		return nil, fmt.Errorf("cannot change primuss exam ancode %s/%d->%d: %w", program, fromAncode, toAncode, err)
	}
	if _, err := p.ChangeAncodeInStudentRegs(ctx, program, fromAncode, toAncode); err != nil {
		return nil, fmt.Errorf("cannot change student regs ancode %s/%d->%d: %w", program, fromAncode, toAncode, err)
	}
	if _, err := p.ChangeAncodeInConflicts(ctx, program, fromAncode, toAncode); err != nil {
		return nil, fmt.Errorf("cannot change conflicts ancode %s/%d->%d: %w", program, fromAncode, toAncode, err)
	}
	return p.GetConnectedExam(ctx, zpaAncode)
}

// ConnectExam connects a Primuss exam (same ancode, given program) to a ZPA exam by
// adding the Primuss ancode mapping; the connected exam is computed live afterwards.
func (p *Plexams) ConnectExam(ancode int, program string) error {
	ctx := context.Background()
	connectedExam, err := p.GetConnectedExam(ctx, ancode)
	if err != nil {
		log.Error().Err(err).Int("ancode", ancode).Msg("cannot get connected exam")
		return err
	}

	for _, primussExam := range connectedExam.PrimussExams {
		if primussExam.AnCode == ancode && primussExam.Program == program {
			log.Debug().Msg("primuss exam already connected")
			return fmt.Errorf("primuss exam already connected")
		}
	}

	if _, err := p.GetPrimussExam(ctx, program, ancode); err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).Msg("cannot get primuss exam")
		return err
	}

	return p.dbClient.AddAncode(ctx, ancode, program, ancode)
}

func (p *Plexams) ExamInfo(ancode int) (string, error) {
	ctx := context.Background()
	found := false
	exam, err := p.PlannedExam(ctx, ancode)
	module, mainExamer := "", ""
	var planEntry *model.PlanEntry
	if err != nil {
		exam, err := p.GetZpaExamByAncode(ctx, ancode)
		if err != nil {
			// TODO: maybe external exam?
		} else {
			found = true
			module = exam.Module
			mainExamer = exam.MainExamer
			planEntry, err = p.dbClient.PlanEntry(ctx, ancode)
			if err != nil {
				log.Debug().Int("ancode", ancode).
					Msg("not planned yet")
			}
		}
	} else {
		found = true
		module = exam.ZpaExam.Module
		mainExamer = exam.ZpaExam.MainExamer
		planEntry = exam.PlanEntry
	}

	if !found {
		return fmt.Sprintf("no exam for ancode %d found", ancode), nil
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "%5d. %s (%s)", ancode, module, mainExamer)
	if planEntry != nil {
		starttime := p.getSlotTime(planEntry.DayNumber, planEntry.SlotNumber)
		if planEntry.ExternalTime != nil {
			starttime = *planEntry.ExternalTime
		}
		fmt.Fprintf(&sb, "\n    Termin: %s (Tag %d / Slot %d)", starttime.Format("02.01.06, 15:04 Uhr"), planEntry.DayNumber, planEntry.SlotNumber)
	} else {
		sb.WriteString("\n    Termin: fehlt")
	}

	return sb.String(), nil
}

func (p *Plexams) ExamsWithoutDuration() (string, error) {
	ctx := context.Background()
	exams, err := p.GeneratedExams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa exams to plan")
		return "", err
	}
	var sb strings.Builder

	for _, exam := range exams {
		if exam.MaxDuration > 0 {
			continue
		}
		if exam.Constraints != nil && exam.Constraints.NotPlannedByMe {
			continue
		}
		fmt.Fprintf(&sb, "%5d. %s (%s) %v\n", exam.Ancode, exam.ZpaExam.Module, exam.ZpaExam.MainExamer, exam.ZpaExam.Groups)
	}

	return sb.String(), nil
}
