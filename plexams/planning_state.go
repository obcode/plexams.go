package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

// The planning state is a condition/event model (a 1-safe Petri net): per phase a
// set of conditions (milestones). A condition can be set automatically when an
// operation finishes (markCondition) or by hand (SetPlanningCondition). A
// condition with a gate locks the matching generation operations while it is set
// (generationAllowed); explicit changes stay allowed.
//
// The net is defined declaratively here and is meant to be extended in code.

type planningPhaseDef struct {
	key   string
	title string
}

type planningConditionDef struct {
	key   string
	title string
	phase string
	// gate, if not empty, is the area locked while this condition is set.
	gate model.PlanningGate
}

var planningPhaseDefs = []planningPhaseDef{
	{"phaseMinus1", "Phase -1: noch im vorherigen Semester"},
	{"phase0", "Phase 0: Vorbereitung"},
	{"phase1", "Phase 1: Terminplanung"},
	{"phase2", "Phase 2: Raumplanung"},
	{"phase3", "Phase 3: Aufsichtenplanung"},
}

// planning condition keys (use the constants when marking from operations).
const (
	condExahmRequested            = "exahmRequested"
	condAnnyRoomsBooked           = "annyRoomsBooked"
	condZPAImported               = "zpaImported"
	condMucDaiImported            = "mucDaiImported"
	condPrimussImported           = "primussImported"
	condZpaPrimussConnected       = "zpaPrimussConnected"
	condExamPlanningInfoSent      = "examPlanningInfoSent"
	condAssembledExams            = "assembledExams"
	condStudentRegs               = "studentRegs"
	condStudentRegsUploaded       = "studentRegsUploaded"
	condPrimussDataAllSent        = "primussDataAllSent"
	condNTARoomAloneSent          = "ntaRoomAloneSent"
	condDraftSent                 = "draftSent"
	condExamPlanPublished         = "examPlanPublished"
	condRoomRequestsSent          = "roomRequestsSent"
	condRoomsGenerated            = "roomsGenerated"
	condSecretariatRoomsSent      = "secretariatRoomsSent"
	condRoomPlanPublished         = "roomPlanPublished"
	condKdpRoomsSent              = "kdpRoomsSent"
	condInvigReqsImported         = "invigReqsImported"
	condInvigilationsRequested    = "invigilationsRequested"
	condInvigilationsGenerated    = "invigilationsGenerated"
	condInvigilationPlanPublished = "invigilationPlanPublished"
	condNTAPlannedSent            = "ntaPlannedSent"
	condInvigSecretariatSent      = "invigSecretariatSent"
	condLbaRepeatersSent          = "lbaRepeatersSent"
	condCoverPagesSent            = "coverPagesSent"
)

var planningConditionDefs = []planningConditionDef{
	{condExahmRequested, "EXaHM/SEB-Abfrage verschickt", "phaseMinus1", ""},
	{condAnnyRoomsBooked, "Anny-Räume gebucht", "phaseMinus1", ""},
	{condZPAImported, "Prüfungen & Personen aus ZPA importiert", "phase0", ""},
	{condExamPlanningInfoSent, "Prüfungsplanungs-Info an Prüfende verschickt", "phase0", ""},
	{condMucDaiImported, "MUC.DAI-Prüfungen importiert & verknüpft", "phase0", ""},
	{condPrimussImported, "Primuss-Anmeldedaten importiert", "phase0", ""},
	{condZpaPrimussConnected, "ZPA- & Primuss-Prüfungen verknüpft", "phase0", ""},
	{condAssembledExams, "Aufbereitete Prüfungen erstellt", "phase0", ""},
	{condStudentRegs, "StudentRegs erstellt", "phase0", ""},
	{condStudentRegsUploaded, "StudentRegs ins ZPA hochgeladen", "phase0", ""},
	{condPrimussDataAllSent, "Primuss-Daten an alle verschickt", "phase0", ""},
	{condNTARoomAloneSent, "Info an NTAs mit eigenem Raum verschickt", "phase0", ""},
	{condDraftSent, "Draft-Plan verschickt", "phase1", ""},
	{condExamPlanPublished, "Terminplan veröffentlicht (E-Mail)", "phase1", ""},
	{condRoomRequestsSent, "Raum-Anfragen ans Gebäudemanagement verschickt", "phase2", ""},
	{condRoomsGenerated, "Räume für Prüfungen generiert", "phase2", ""},
	{condSecretariatRoomsSent, "Raumbelegung ans Sekretariat verschickt", "phase2", ""},
	{condRoomPlanPublished, "Raumplan veröffentlicht (E-Mail)", "phase2", model.PlanningGateRooms},
	{condKdpRoomsSent, "EXaHM/SEB-Raumübersicht ans KDP verschickt", "phase2", ""},
	{condInvigilationsRequested, "Aufsichts-Anforderungsabfrage verschickt", "phase3", ""},
	{condInvigReqsImported, "Aufsichts-Anforderungen importiert", "phase3", ""},
	{condInvigilationsGenerated, "Aufsichten generiert", "phase3", ""},
	{condInvigilationPlanPublished, "Aufsichtenplan veröffentlicht (E-Mail)", "phase3", model.PlanningGateInvigilations},
	{condInvigSecretariatSent, "Info 'Aufsichten veröffentlicht' ans Sekretariat verschickt", "phase3", ""},
	{condNTAPlannedSent, "Info an NTAs zu ihren Räumen verschickt", "phase3", ""},
	{condLbaRepeatersSent, "Info Wiederholungsprüfungen LBAs ans LBA-BA verschickt", "phase3", ""},
	{condCoverPagesSent, "Deckblätter an alle verschickt (letzter Schritt)", "phase3", ""},
}

func planningConditionDefByKey(key string) (planningConditionDef, bool) {
	for _, def := range planningConditionDefs {
		if def.key == key {
			return def, true
		}
	}
	return planningConditionDef{}, false
}

// PlanningState assembles the current planning state from the declarative net and
// the conditions stored as set in the DB.
func (p *Plexams) PlanningState(ctx context.Context) (*model.PlanningState, error) {
	setKeys, err := p.dbClient.PlanningConditionsSet(ctx)
	if err != nil {
		return nil, err
	}
	done := make(map[string]bool, len(setKeys))
	for _, key := range setKeys {
		done[key] = true
	}

	phaseByKey := make(map[string]*model.PlanningPhase)
	phases := make([]*model.PlanningPhase, 0, len(planningPhaseDefs))
	for _, pd := range planningPhaseDefs {
		phase := &model.PlanningPhase{Key: pd.key, Title: pd.title, Conditions: []*model.PlanningCondition{}}
		phaseByKey[pd.key] = phase
		phases = append(phases, phase)
	}

	blocked := make([]model.PlanningGate, 0)
	for _, cd := range planningConditionDefs {
		cond := &model.PlanningCondition{
			Key:   cd.key,
			Title: cd.title,
			Phase: cd.phase,
			Done:  done[cd.key],
		}
		if cd.gate != "" {
			gate := cd.gate
			cond.Gate = &gate
			if cond.Done {
				blocked = append(blocked, gate)
			}
		}
		if phase, ok := phaseByKey[cd.phase]; ok {
			phase.Conditions = append(phase.Conditions, cond)
		}
	}

	return &model.PlanningState{Phases: phases, BlockedAreas: blocked}, nil
}

// SetPlanningCondition sets or clears a condition by hand. Errors on an unknown
// key.
func (p *Plexams) SetPlanningCondition(ctx context.Context, key string, done bool) (*model.PlanningState, error) {
	if _, ok := planningConditionDefByKey(key); !ok {
		return nil, fmt.Errorf("unknown planning condition %q", key)
	}
	if err := p.dbClient.SetPlanningCondition(ctx, key, done); err != nil {
		return nil, err
	}
	return p.PlanningState(ctx)
}

// markCondition sets a condition as done. It is best-effort: a failure is logged
// but never fails the operation that triggered it.
func (p *Plexams) markCondition(ctx context.Context, key string) {
	if err := p.dbClient.SetPlanningCondition(ctx, key, true); err != nil {
		log.Error().Err(err).Str("key", key).Msg("cannot auto-mark planning condition")
	}
}

// unmarkCondition clears a condition. Best-effort like markCondition.
func (p *Plexams) unmarkCondition(ctx context.Context, key string) {
	if err := p.dbClient.SetPlanningCondition(ctx, key, false); err != nil {
		log.Error().Err(err).Str("key", key).Msg("cannot auto-unmark planning condition")
	}
}

// emailSendAllowed enforces that a "send once" email is sent at most once: while
// its condition is set, a real send (run==true) is refused; a dry run
// (run==false) is always allowed. On a successful real send the caller marks the
// condition. To resend, the condition has to be unset by hand.
func (p *Plexams) emailSendAllowed(ctx context.Context, condKey string, run bool) error {
	if !run {
		return nil
	}
	setKeys, err := p.dbClient.PlanningConditionsSet(ctx)
	if err != nil {
		return err
	}
	for _, key := range setKeys {
		if key == condKey {
			def, _ := planningConditionDefByKey(condKey)
			return fmt.Errorf("email already sent (%s: %s); unset the planning condition to send it again",
				condKey, def.title)
		}
	}
	return nil
}

// generationAllowed reports whether generation for the given area is allowed, i.e.
// no gate condition for that area is set. Returns an error describing the lock
// otherwise.
func (p *Plexams) generationAllowed(ctx context.Context, area model.PlanningGate) error {
	setKeys, err := p.dbClient.PlanningConditionsSet(ctx)
	if err != nil {
		return err
	}
	set := make(map[string]bool, len(setKeys))
	for _, key := range setKeys {
		set[key] = true
	}
	for _, cd := range planningConditionDefs {
		if cd.gate == area && set[cd.key] {
			return fmt.Errorf("%s is published (%s); generation is locked. "+
				"Make explicit changes instead, or unset the condition to regenerate", cd.title, cd.key)
		}
	}
	return nil
}
