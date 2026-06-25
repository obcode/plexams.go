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

func (p *Plexams) prepareConnectedZPAExam(ctx context.Context, ancode int, allPrograms []string) (*model.ConnectedExam, error) {
	zpaExam, err := p.dbClient.GetZpaExamByAncode(ctx, ancode)
	if err != nil {
		return nil, err
	}

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

		primussExam, err := p.GetPrimussExam(ctx, primussAncode.Program, primussAncode.Ancode)
		if err != nil {
			warnings = append(warnings, p.primussNotFoundWarning(ctx, zpaExam, primussAncode.Program, primussAncode.Ancode))
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
		for _, p := range primussExams {
			if aP == p.Program {
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
		primussExam, err := p.GetPrimussExam(ctx, program, ancode)
		if err == nil {
			if otherPrimussExams == nil {
				otherPrimussExams = make([]*model.PrimussExam, 0)
			}
			otherPrimussExams = append(otherPrimussExams, primussExam)
			if sameExamer(zpaExam.MainExamer, primussExam.MainExamer) {
				warnings = append(warnings, &model.ConnectedExamWarning{
					Level:   "warning",
					Message: fmt.Sprintf("gleiche Nummer auch in %s/%d (gleicher Prüfer: %s) — zusätzlicher Studiengang?", program, ancode, primussExam.MainExamer),
				})
			} else {
				warnings = append(warnings, &model.ConnectedExamWarning{
					Level:   "info",
					Message: fmt.Sprintf("gleiche Nummer in %s/%d (anderer Prüfer: %s, %s) — vermutlich Zufall", program, ancode, primussExam.MainExamer, primussExam.Module),
				})
			}
		}
	}

	return &model.ConnectedExam{
		ZpaExam:           zpaExam,
		PrimussExams:      primussExams,
		OtherPrimussExams: otherPrimussExams,
		Warnings:          warnings,
	}, nil
}

// primussNotFoundWarning classifies a missing mapped primuss exam: when a likely
// counterpart (same examer, similar module) exists in the program it is a warning
// with a concrete suggestion, otherwise just an info (e.g. a program that isn't
// (fully) imported this semester).
func (p *Plexams) primussNotFoundWarning(ctx context.Context, zpaExam *model.ZPAExam, program string, ancode int) *model.ConnectedExamWarning {
	if suggestion := p.suggestPrimussExam(ctx, program, zpaExam.MainExamer, zpaExam.Module); suggestion != nil {
		return &model.ConnectedExamWarning{
			Level: "warning",
			Message: fmt.Sprintf("%s/%d nicht gefunden — evtl. %s/%d (gleicher Prüfer, Modul „%s“)",
				program, ancode, program, suggestion.AnCode, suggestion.Module),
		}
	}
	return &model.ConnectedExamWarning{
		Level:   "info",
		Message: fmt.Sprintf("%s/%d nicht gefunden", program, ancode),
	}
}

// suggestPrimussExam looks in one program for a primuss exam with the same examer
// (surname) and a similar module name as the given ZPA exam.
func (p *Plexams) suggestPrimussExam(ctx context.Context, program, zpaExamer, zpaModule string) *model.PrimussExam {
	exams, err := p.dbClient.PrimussExamsForProgram(ctx, program)
	if err != nil {
		return nil
	}
	for _, exam := range exams {
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
		return &model.ConnectedExamWarning{
			Level:   "info",
			Message: fmt.Sprintf("Prüfer-Schreibweise weicht ab (%s/%d): ZPA „%s“ / Primuss „%s“", program, ancode, zpaExamer, primussExamer),
		}
	}
	return &model.ConnectedExamWarning{
		Level:   "warning",
		Message: fmt.Sprintf("Prüfer unterschiedlich (%s/%d): ZPA „%s“ / Primuss „%s“", program, ancode, zpaExamer, primussExamer),
	}
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

func (p *Plexams) prepareConnectedNonZPAExam(ctx context.Context, ancode int, nonZPAExam *model.ZPAExam) (*model.ConnectedExam, error) {
	if nonZPAExam == nil {
		var err error
		nonZPAExam, err = p.dbClient.NonZpaExam(ctx, ancode)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get non zpa exam by ancode")
			return nil, err
		}
	}

	primussExams := make([]*model.PrimussExam, 0)
	for _, primuss := range nonZPAExam.PrimussAncodes {
		primussExam, err := p.GetPrimussExam(ctx, primuss.Program, primuss.Ancode)
		if err != nil {
			log.Error().Err(err).Str("program", primuss.Program).Int("ancode", primuss.Ancode).
				Msg("cannot get primuss exam")
			return nil, err
		}
		primussExams = append(primussExams, primussExam)
	}

	return &model.ConnectedExam{
		ZpaExam:           nonZPAExam,
		PrimussExams:      primussExams,
		OtherPrimussExams: nil,
		Warnings:          []*model.ConnectedExamWarning{},
	}, nil
}

func (p *Plexams) GetConnectedExams(ctx context.Context) ([]*model.ConnectedExam, error) {
	return p.dbClient.GetConnectedExams(ctx)
}

func (p *Plexams) GetConnectedExam(ctx context.Context, ancode int) (*model.ConnectedExam, error) {
	return p.dbClient.GetConnectedExam(ctx, ancode)
}

func (p *Plexams) PrepareConnectedExams() error {
	ctx := context.Background()
	ancodes, err := p.GetZpaAnCodesToPlan(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get zpa ancodes")
		return err
	}

	allPrograms, err := p.dbClient.GetPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get programs")
		return err
	}

	exams := make([]*model.ConnectedExam, 0)
	for _, ancode := range ancodes {
		exam, err := p.prepareConnectedZPAExam(ctx, ancode.Ancode, allPrograms)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode.Ancode).
				Msg("cannot connected exam")
			return err
		}
		exams = append(exams, exam)
	}

	nonZPAExams, err := p.dbClient.NonZpaExams(ctx)
	if err == nil {
		for _, nonZPAExam := range nonZPAExams {
			connectedExam, err := p.prepareConnectedNonZPAExam(ctx, nonZPAExam.AnCode, nonZPAExam)
			if err != nil {
				log.Error().Err(err).Int("ancode", nonZPAExam.AnCode).
					Msg("cannot prepare connected non zpa exam")
				return err
			}
			exams = append(exams, connectedExam)
		}
	}

	err = p.dbClient.SaveConnectedExams(ctx, exams)
	if err != nil {
		log.Error().Err(err).Msg("cannot save connected exams")
		return err
	}

	p.markCondition(ctx, condConnectedExams)
	return nil
}

func (p *Plexams) PrepareConnectedExam(ancode int) error {
	ctx := context.Background()

	allPrograms, err := p.dbClient.GetPrograms(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get programs")
		return err
	}

	var exam *model.ConnectedExam
	if ancode < 1000 {
		exam, err = p.prepareConnectedZPAExam(ctx, ancode, allPrograms)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).
				Msg("cannot connected exam")
			return err
		}
	} else {
		exam, err = p.prepareConnectedNonZPAExam(ctx, ancode, nil)
		if err != nil {
			log.Error().Err(err).Int("ancode", ancode).Msg("cannot get non zpa exam by ancode")
			return err
		}
	}

	if exam != nil {
		err = p.dbClient.SaveConnectedExam(ctx, exam)
		if err != nil {
			log.Error().Err(err).Msg("cannot save connected exam")
			return err
		}
	}

	return nil
}

// TODO: check if there are Exams with same Ancode in other programs
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

	primussExam, err := p.GetPrimussExam(ctx, program, ancode)
	if err != nil {
		log.Error().Err(err).Str("program", program).Int("ancode", ancode).Msg("cannot get primuss exam")
		return err
	}

	connectedExam.PrimussExams = append(connectedExam.PrimussExams, primussExam)

	if len(connectedExam.OtherPrimussExams) > 0 {
		otherPrimussExams := make([]*model.PrimussExam, 0)
		for _, exam := range connectedExam.OtherPrimussExams {
			if exam.AnCode != ancode || exam.Program != program {
				otherPrimussExams = append(otherPrimussExams, exam)
			}
			if len(otherPrimussExams) > 0 {
				connectedExam.OtherPrimussExams = otherPrimussExams
			} else {
				connectedExam.OtherPrimussExams = nil
			}
		}
	}

	return p.dbClient.ReplaceConnectedExam(ctx, connectedExam)
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
