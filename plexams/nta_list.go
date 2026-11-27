package plexams

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/ntalist"
	"github.com/obcode/plexams.go/plexams/primuss"
	"github.com/rs/zerolog/log"
)

// The NTA lists of the exam board (see package ntalist): uploading stores the list
// of a faculty as it was read; the preview matches every row against the NTA
// master data and the Primuss registrations; applying writes the rows the
// planner selected (and possibly corrected) to the master data.

// NTAListFile is one uploaded file.
type NTAListFile struct {
	Filename string
	Data     []byte
}

// NTAListUploadSummary says what changed against the previous list of the faculty.
type NTAListUploadSummary struct {
	FK            string     `json:"fk"`
	Filename      string     `json:"filename"`
	Semester      string     `json:"semester"`
	Stand         *time.Time `json:"stand"`
	Rows          int        `json:"rows"`
	FirstUpload   bool       `json:"firstUpload"`
	PreviousStand *time.Time `json:"previousStand"`
	NewRows       int        `json:"newRows"`
	ChangedRows   int        `json:"changedRows"`
	RemovedRows   int        `json:"removedRows"`
	UnchangedRows int        `json:"unchangedRows"`
	Warnings      []string   `json:"warnings"`
}

// UploadNTALists reads the uploaded lists and stores each, replacing the previous
// list of the same faculty. If one file cannot be read, nothing is stored.
func (p *Plexams) UploadNTALists(ctx context.Context, files []NTAListFile) ([]*NTAListUploadSummary, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("es wurde keine Datei mitgeschickt")
	}
	lists := make([]*ntalist.List, 0, len(files))
	byFK := map[string]string{}
	for _, f := range files {
		list, err := ntalist.Parse(f.Filename, f.Data)
		if err != nil {
			return nil, err
		}
		if list.FK == "" {
			return nil, fmt.Errorf("%s: die Fakultät steht weder in der Tabelle noch im Dateinamen", f.Filename)
		}
		if other, ok := byFK[list.FK]; ok {
			return nil, fmt.Errorf("%s und %s sind beide die Liste der FK %s — bitte nur eine davon hochladen", other, f.Filename, list.FK)
		}
		byFK[list.FK] = f.Filename
		if list.SemesterID != "" && !p.isActiveSemester(list.SemesterID) {
			list.Warnings = append(list.Warnings, fmt.Sprintf(
				"Die Liste gehört zum Semester %s, eingelesen wurde sie aber in %s", list.SemesterID, p.semester))
		}
		lists = append(lists, list)
	}

	stored, err := p.dbClient.NTALists(ctx)
	if err != nil {
		return nil, err
	}
	previous := make(map[string]*db.StoredNTAList, len(stored))
	for _, s := range stored {
		previous[s.List.FK] = s
	}

	summaries := make([]*NTAListUploadSummary, 0, len(lists))
	for _, list := range lists {
		sum := &NTAListUploadSummary{
			FK: list.FK, Filename: list.Filename, Semester: list.SemesterID,
			Rows: len(list.Rows), Warnings: list.Warnings,
		}
		if !list.Stand.IsZero() {
			stand := list.Stand
			sum.Stand = &stand
		}
		firstSeen := map[string]time.Time{}
		if prev, ok := previous[list.FK]; ok {
			if !prev.List.Stand.IsZero() {
				ps := prev.List.Stand
				sum.PreviousStand = &ps
			}
			firstSeen = prev.FirstSeen
			sum.NewRows, sum.ChangedRows, sum.RemovedRows, sum.UnchangedRows = diffNTALists(prev.List, list)
		} else {
			sum.FirstUpload = true
			sum.NewRows = len(list.Rows)
		}
		if err := p.dbClient.ReplaceNTAList(ctx, list, firstSeen, p.actorID(ctx)); err != nil {
			log.Error().Err(err).Str("fk", list.FK).Msg("cannot store nta list")
			return nil, fmt.Errorf("Die Liste der FK %s konnte nicht gespeichert werden: %w", list.FK, err)
		}
		summaries = append(summaries, sum)
	}
	return summaries, nil
}

// diffNTALists compares two lists of a faculty by mtknr; the line number does
// not count.
func diffNTALists(prev, cur *ntalist.List) (added, changed, removed, unchanged int) {
	key := func(r *ntalist.Row) string {
		c := *r
		c.Line = 0
		b, _ := json.Marshal(c)
		return string(b)
	}
	old := map[string]string{}
	for _, r := range prev.Rows {
		old[r.Mtknr] = key(r)
	}
	for _, r := range cur.Rows {
		k, ok := old[r.Mtknr]
		switch {
		case !ok:
			added++
		case k != key(r):
			changed++
		default:
			unchanged++
		}
		delete(old, r.Mtknr)
	}
	return added, changed, len(old), unchanged
}

// RemoveNTAList deletes the uploaded list of a faculty.
func (p *Plexams) RemoveNTAList(ctx context.Context, fk string) (bool, error) {
	return p.dbClient.RemoveNTAList(ctx, fk)
}

// NTAListPreview matches the uploaded lists against the master data and Primuss.
func (p *Plexams) NTAListPreview(ctx context.Context) (*model.NTAListPreview, error) {
	stored, err := p.dbClient.NTALists(ctx)
	if err != nil {
		return nil, err
	}
	programs, err := p.dbClient.StudyPrograms(ctx)
	if err != nil {
		return nil, err
	}
	hasRegs, err := p.dbClient.HasStudentRegs(ctx)
	if err != nil {
		return nil, err
	}
	ntas, err := p.dbClient.Ntas(ctx)
	if err != nil {
		return nil, err
	}
	ntaByMtknr := make(map[string]*model.NTA, len(ntas))
	for _, n := range ntas {
		ntaByMtknr[n.Mtknr] = n
	}

	var mtknrs []string
	for _, s := range stored {
		for _, r := range s.List.Rows {
			if r.Mtknr != "" {
				mtknrs = append(mtknrs, r.Mtknr)
			}
		}
	}
	regsByMtknr := map[string][]db.PrimussReg{}
	if hasRegs && len(mtknrs) > 0 {
		regsByMtknr, err = p.dbClient.StudentRegsForMtknrs(ctx, mtknrs)
		if err != nil {
			return nil, err
		}
	}

	res := &ntaListResolver{programs: programs, cache: map[string]resolvedProgram{}}
	preview := &model.NTAListPreview{Lists: make([]*model.NTAList, 0, len(stored)), PrimussRegsAvailable: hasRegs}
	for _, s := range stored {
		l := s.List
		out := &model.NTAList{
			Fk:              l.FK,
			Faculty:         "FK " + l.FK,
			Filename:        l.Filename,
			Semester:        l.SemesterID,
			SemesterMatches: p.isActiveSemester(l.SemesterID),
			ImportedAt:      s.ImportedAt,
			ImportedBy:      s.ImportedBy,
			Warnings:        l.Warnings,
			Rows:            make([]*model.NTAListRow, 0, len(l.Rows)),
		}
		if !l.Stand.IsZero() {
			stand := l.Stand
			out.Stand = &stand
		}
		joint := map[string]bool{}
		for _, r := range l.Rows {
			if jf := res.resolve(r.Stg).jointFaculty; jf != "" {
				joint[jf] = true
			}
			row := p.ntaListRow(ctx, r, l, ntaByMtknr[r.Mtknr], res, hasRegs, regsByMtknr[r.Mtknr])
			row.FirstSeen = s.FirstSeen[r.Mtknr]
			out.Rows = append(out.Rows, row)
		}
		if len(joint) > 0 {
			names := make([]string, 0, len(joint))
			for jf := range joint {
				names = append(names, jf)
			}
			sort.Strings(names)
			out.Faculty += " (" + strings.Join(names, ", ") + ")"
		}
		preview.Lists = append(preview.Lists, out)
	}
	return preview, nil
}

type resolvedProgram struct {
	program      string
	resolved     bool
	jointFaculty string
}

type ntaListResolver struct {
	programs []*model.StudyProgram
	cache    map[string]resolvedProgram
}

// resolve maps a program code of the list ("IF-B", "AW-") onto a study program.
// An unknown code falls back to the code without the degree.
func (r *ntaListResolver) resolve(code string) resolvedProgram {
	if rp, ok := r.cache[code]; ok {
		return rp
	}
	var rp resolvedProgram
	if prog, err := primuss.ResolveProgram(code, r.programs); err == nil {
		rp = resolvedProgram{program: prog, resolved: true}
		for _, sp := range r.programs {
			if sp.Shortname == prog && sp.JointFaculty != nil {
				rp.jointFaculty = *sp.JointFaculty
			}
		}
	} else {
		base, _, _ := strings.Cut(code, "-")
		rp = resolvedProgram{program: base}
	}
	r.cache[code] = rp
	return rp
}

func (p *Plexams) ntaListRow(ctx context.Context, r *ntalist.Row, l *ntalist.List, existing *model.NTA,
	res *ntaListResolver, hasRegs bool, primussRegs []db.PrimussReg) *model.NTAListRow {
	name := strings.TrimSpace(r.Name)
	if r.Vorname != "" {
		name += ", " + r.Vorname
	}
	beginn := r.BeginnStudium
	if id, err := ntalist.SemesterID(beginn); err == nil {
		beginn = id
	}
	row := &model.NTAListRow{
		Line:                r.Line,
		Mtknr:               r.Mtknr,
		Name:                name,
		Stg:                 r.Stg,
		BeginnStudium:       beginn,
		Compensation:        r.Compensation,
		Validity:            r.Validity,
		Changes:             []*model.NTAListFieldChange{},
		Problems:            append([]string{}, r.Problems...),
		Infos:               []string{},
		Regs:                []*model.NTAListReg{},
		RegsOnlyInList:      []*model.NTAListReg{},
		RegsOnlyInPrimuss:   []*model.NTAListReg{},
		ExistingDeactivated: existing != nil && existing.Deactivated,
	}

	prog := res.resolve(r.Stg)
	if r.Stg != "" && !prog.resolved {
		row.Infos = append(row.Infos, fmt.Sprintf(
			"Studiengang %s ist in den Stammdaten nicht angelegt; übernommen wird %s", r.Stg, prog.program))
	}
	if r.Compensation != "" && r.Parsed.Unparsed {
		row.Problems = append(row.Problems,
			"Die Schreibzeitverlängerung ist im Text nicht zu erkennen — bitte Prozent, eigenen Raum und Hardware von Hand setzen")
	}

	proposal := &model.NTAListProposal{
		Name:                 name,
		Mtknr:                r.Mtknr,
		Compensation:         r.Compensation,
		DeltaDurationPercent: r.Parsed.DeltaDurationPercent,
		NeedsRoomAlone:       r.Parsed.NeedsRoomAlone,
		NeedsHardware:        r.Parsed.NeedsHardware,
		Program:              prog.program,
		// empty if the validity could not be read (then it is a problem)
		Until: r.Until.Label(prog.program),
	}
	if !l.Stand.IsZero() {
		proposal.From = l.Stand.Format("02.01.2006")
	}
	if existing != nil {
		row.Existing = ntaToProposal(existing)
		proposal.From = existing.From
		proposal.Email = existing.Email
		if r.Parsed.Unparsed {
			proposal.DeltaDurationPercent = existing.DeltaDurationPercent
			proposal.NeedsRoomAlone = existing.NeedsRoomAlone
			proposal.NeedsHardware = existing.NeedsHardware
		}
		if proposal.Until == "" {
			proposal.Until = existing.Until
		}
		if existing.Deactivated {
			row.Infos = append(row.Infos, "Das NTA ist in den Stammdaten deaktiviert; Übernehmen aktiviert es nicht")
		}
	}
	if proposal.Email == nil && r.Mtknr != "" {
		if st, err := p.dbClient.GetZPAStudentByMtknr(ctx, r.Mtknr); err == nil && st != nil && st.Email != "" {
			email := st.Email
			proposal.Email = &email
		}
	}
	significant := false
	if existing != nil {
		// Changes are everything the list says differently; the proposal only takes
		// over what matters for planning. The stored text is usually the longer one
		// (typed from the decision), so it stays unless the decision changed.
		row.Changes = ntaListChanges(row.Existing, proposal)
		for _, c := range row.Changes {
			significant = significant || c.Significant
		}
		listProposal := *proposal
		*proposal = *row.Existing
		proposal.Email = listProposal.Email
		if significant {
			proposal.DeltaDurationPercent = listProposal.DeltaDurationPercent
			proposal.NeedsRoomAlone = listProposal.NeedsRoomAlone
			proposal.NeedsHardware = listProposal.NeedsHardware
			proposal.Program = listProposal.Program
			proposal.Compensation = listProposal.Compensation
			for _, c := range row.Changes {
				if c.Field == "until" && c.Significant {
					proposal.Until = listProposal.Until
				}
			}
		}
	}
	row.Proposal = proposal

	switch {
	case len(row.Problems) > 0:
		row.Status = model.NTAListRowStatusProblem
	case existing == nil:
		row.Status = model.NTAListRowStatusNew
	case significant:
		row.Status = model.NTAListRowStatusChanged
	default:
		row.Status = model.NTAListRowStatusUnchanged
	}

	// registrations: the list against Primuss
	type key struct {
		program string
		ancode  int
	}
	inList := map[key]bool{}
	for _, reg := range r.Regs {
		rp := res.resolve(reg.Stg)
		row.Regs = append(row.Regs, &model.NTAListReg{Stg: reg.Stg, Program: rp.program, PrimussAncode: reg.PrimussAncode})
		inList[key{rp.program, reg.PrimussAncode}] = true
	}
	if hasRegs {
		inPrimuss := map[key]bool{}
		studentPrograms := map[string]bool{}
		for _, reg := range primussRegs {
			k := key{reg.Program, reg.PrimussAncode}
			if reg.StudentProgram != "" {
				studentPrograms[reg.StudentProgram] = true
			}
			if inPrimuss[k] {
				continue
			}
			inPrimuss[k] = true
			if !inList[k] {
				row.RegsOnlyInPrimuss = append(row.RegsOnlyInPrimuss,
					&model.NTAListReg{Stg: reg.Program, Program: reg.Program, PrimussAncode: reg.PrimussAncode})
			}
		}
		for _, reg := range row.Regs {
			if !inPrimuss[key{reg.Program, reg.PrimussAncode}] {
				row.RegsOnlyInList = append(row.RegsOnlyInList, reg)
			}
		}
		if len(studentPrograms) > 0 && !studentPrograms[proposal.Program] {
			sp := make([]string, 0, len(studentPrograms))
			for s := range studentPrograms {
				sp = append(sp, s)
			}
			sort.Strings(sp)
			row.Infos = append(row.Infos, fmt.Sprintf(
				"In Primuss ist der Studiengang %s eingetragen, im NTA %s — so greift der Nachteilsausgleich nicht",
				strings.Join(sp, ", "), proposal.Program))
		}
	}
	return row
}

func ntaToProposal(n *model.NTA) *model.NTAListProposal {
	return &model.NTAListProposal{
		Name:                 n.Name,
		Email:                n.Email,
		Mtknr:                n.Mtknr,
		Compensation:         n.Compensation,
		DeltaDurationPercent: n.DeltaDurationPercent,
		NeedsRoomAlone:       n.NeedsRoomAlone,
		NeedsHardware:        n.NeedsHardware,
		Program:              n.Program,
		From:                 n.From,
		Until:                n.Until,
	}
}

// wholeStudiesRE matches the ways the master data says "for the whole studies":
// "Studium IF", "Ende WD", "Studiengang WT", "Gesamtes Studium", ...
var wholeStudiesRE = regexp.MustCompile(`(?i)^\s*(Studium|Ende|Studiengang|Gesamtes\s+Studium)\b`)

func ntaListChanges(old, cur *model.NTAListProposal) []*model.NTAListFieldChange {
	changes := []*model.NTAListFieldChange{}
	add := func(field, label, o, n string, significant bool) {
		if o != n {
			changes = append(changes, &model.NTAListFieldChange{Field: field, Label: label, Old: o, New: n, Significant: significant})
		}
	}
	yesNo := func(b bool) string {
		if b {
			return "ja"
		}
		return "nein"
	}
	deref := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	add("deltaDurationPercent", "Zeitverlängerung (%)", strconv.Itoa(old.DeltaDurationPercent), strconv.Itoa(cur.DeltaDurationPercent), true)
	add("needsRoomAlone", "eigener Raum", yesNo(old.NeedsRoomAlone), yesNo(cur.NeedsRoomAlone), true)
	add("needsHardware", "spezielle Hardware", yesNo(old.NeedsHardware), yesNo(cur.NeedsHardware), true)
	add("program", "Studiengang", old.Program, cur.Program, true)
	add("until", "gilt bis", old.Until, cur.Until, !(wholeStudiesRE.MatchString(old.Until) && wholeStudiesRE.MatchString(cur.Until)))
	add("name", "Name", old.Name, cur.Name, false)
	add("compensation", "Ausgleich (Text)", strings.Join(strings.Fields(old.Compensation), " "), cur.Compensation, false)
	add("email", "E-Mail", deref(old.Email), deref(cur.Email), false)
	return changes
}

// ApplyNTAList writes the selected rows to the master data, all or nothing.
func (p *Plexams) ApplyNTAList(ctx context.Context, items []*model.NTAListApplyItem) (*model.NTAListApplyResult, error) {
	result := &model.NTAListApplyResult{}
	if len(items) == 0 {
		return result, nil
	}
	err := p.dbClient.InTransaction(ctx, func(ctx context.Context) error {
		for _, item := range items {
			if item == nil || item.Nta == nil {
				continue
			}
			in := *item.Nta
			in.Mtknr = strings.TrimSpace(in.Mtknr)
			if in.Mtknr == "" || strings.TrimSpace(in.Name) == "" {
				return fmt.Errorf("ein Eintrag hat keine Matrikelnummer oder keinen Namen")
			}
			if in.DeltaDurationPercent < 0 {
				return fmt.Errorf("%s: die Zeitverlängerung darf nicht negativ sein", in.Name)
			}
			existing, err := p.dbClient.Nta(ctx, in.Mtknr)
			if err != nil {
				return err
			}
			switch {
			case item.Action == model.NTAListApplyActionAdd && existing != nil:
				return fmt.Errorf("%s ist bereits in den NTA-Stammdaten — bitte die Seite neu laden", in.Name)
			case item.Action == model.NTAListApplyActionUpdate && existing == nil:
				return fmt.Errorf("%s ist nicht (mehr) in den NTA-Stammdaten — bitte die Seite neu laden", in.Name)
			}
			switch item.Action {
			case model.NTAListApplyActionAdd:
				if _, err := p.AddNta(ctx, in); err != nil {
					return fmt.Errorf("%s konnte nicht angelegt werden: %w", in.Name, err)
				}
				result.Added++
			case model.NTAListApplyActionUpdate:
				if _, err := p.UpdateNta(ctx, in); err != nil {
					return fmt.Errorf("%s konnte nicht aktualisiert werden: %w", in.Name, err)
				}
				result.Updated++
			default:
				return fmt.Errorf("unbekannte Aktion %s", item.Action)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("Es wurde nichts übernommen. %w", err)
	}
	return result, nil
}

// isActiveSemester compares a semester id ("2026-WS") with the active semester,
// which p.semester holds in its logical form ("2026 WS").
func (p *Plexams) isActiveSemester(id string) bool {
	return strings.ReplaceAll(id, "-", " ") == strings.ReplaceAll(p.semester, "-", " ")
}
