package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/planstate"
)

// The planning state is a condition/event model (a 1-safe Petri net): per phase a set of
// conditions (milestones). A condition can be set automatically when an operation finishes
// (markCondition) or by hand (SetPlanningCondition). A condition with a gate locks the
// matching generation operations while it is set (generationAllowed); explicit changes
// stay allowed.
//
// The net is DEFINED declaratively here (this app's policy); the generic engine lives in
// the plexams/planstate package and is wired up in NewPlexams.

// planning condition keys (use the constants when marking from operations).
const (
	condZPAPersonsImported        = "zpaPersonsImported"
	condExahmRequested            = "exahmRequested"
	condSebExahmDemandEntered     = "sebExahmDemandEntered"
	condAnnyRoomsBooked           = "annyRoomsBooked"
	condSebExahmPreplanned        = "sebExahmPreplanned"
	condZPAImported               = "zpaImported"
	condSebExahmZpaConnected      = "sebExahmZpaConnected"
	condZPAExamsSelected          = "zpaExamsSelected"
	condMucDaiImported            = "mucDaiImported"
	condPrimussImported           = "primussImported"
	condZpaPrimussConnected       = "zpaPrimussConnected"
	condConstraintsEntered        = "constraintsEntered"
	condExamPlanningInfoSent      = "examPlanningInfoSent"
	condAssembledExams            = "assembledExams"
	condStudentRegs               = "studentRegs"
	condStudentRegsUploaded       = "studentRegsUploaded"
	condPrimussDataAllSent        = "primussDataAllSent"
	condNTARoomAloneSent          = "ntaRoomAloneSent"
	condExahmSebPlanned           = "exahmSebPlanned"
	condExahmSebFixed             = "exahmSebFixed"
	condExamScheduleGenerated     = "examScheduleGenerated"
	condDraftSent                 = "draftSent"
	condExamPlanPublished         = "examPlanPublished"
	condRoomRequestsSent          = "roomRequestsSent"
	condRoomsAssigned             = "roomsAssigned"
	condSecretariatRoomsSent      = "secretariatRoomsSent"
	condRoomPlanPublished         = "roomPlanPublished"
	condKdpRoomsSent              = "kdpRoomsSent"
	condInvigReqsImported         = "invigReqsImported"
	condInvigilationsRequested    = "invigilationsRequested"
	condInvigilationsAssigned     = "invigilationsAssigned"
	condInvigilationPlanPublished = "invigilationPlanPublished"
	condNTAPlannedSent            = "ntaPlannedSent"
	condInvigSecretariatSent      = "invigSecretariatSent"
	condLbaRepeatersSent          = "lbaRepeatersSent"
	condCoverPagesSent            = "coverPagesSent"
)

var planningPhaseDefs = []planstate.PhaseDef{
	{Key: "phaseMinus1", Title: "Phase -1: noch im vorherigen Semester"},
	{Key: "phase0", Title: "Phase 0: Vorbereitung"},
	{Key: "phase1", Title: "Phase 1: Terminplanung"},
	{Key: "phase2", Title: "Phase 2: Raumplanung"},
	{Key: "phase3", Title: "Phase 3: Aufsichtenplanung"},
}

var planningConditionDefs = []planstate.CondDef{
	{Key: condZPAPersonsImported, Title: "Personen aus ZPA importiert", Phase: "phaseMinus1"},
	{Key: condExahmRequested, Title: "EXaHM/SEB-Abfrage verschickt", Phase: "phaseMinus1"},
	{Key: condSebExahmDemandEntered, Title: "EXaHM/SEB-Prüfungsbedarf erfasst", Phase: "phaseMinus1"},
	{Key: condAnnyRoomsBooked, Title: "Anny-Räume gebucht", Phase: "phaseMinus1"},
	{Key: condSebExahmPreplanned, Title: "SEB/EXaHM-Vorplanung erzeugt", Phase: "phaseMinus1"},
	{Key: condZPAImported, Title: "Prüfungen aus ZPA importiert", Phase: "phase0"},
	{Key: condSebExahmZpaConnected, Title: "EXaHM/SEB-Vorplanung mit ZPA-Prüfungen verknüpft", Phase: "phase0"},
	{Key: condZPAExamsSelected, Title: "ZPA-Prüfungen für die Planung ausgewählt", Phase: "phase0"},
	{Key: condExamPlanningInfoSent, Title: "Prüfungsplanungs-Info an Prüfende verschickt", Phase: "phase0"},
	{Key: condMucDaiImported, Title: "MUC.DAI-Prüfungen importiert & verknüpft", Phase: "phase0"},
	{Key: condPrimussImported, Title: "Primuss-Anmeldedaten importiert", Phase: "phase0"},
	{Key: condZpaPrimussConnected, Title: "ZPA- & Primuss-Prüfungen verknüpft", Phase: "phase0"},
	{Key: condConstraintsEntered, Title: "Constraints eingepflegt", Phase: "phase0"},
	{Key: condAssembledExams, Title: "Aufbereitete Prüfungen erstellt", Phase: "phase0"},
	{Key: condStudentRegs, Title: "Anmeldungen erstellt", Phase: "phase0"},
	{Key: condStudentRegsUploaded, Title: "Anmeldungen ins ZPA hochgeladen", Phase: "phase0"},
	{Key: condPrimussDataAllSent, Title: "Primuss-Daten an alle verschickt", Phase: "phase0"},
	{Key: condNTARoomAloneSent, Title: "Info an NTAs mit eigenem Raum verschickt", Phase: "phase0"},
	{Key: condExahmSebPlanned, Title: "EXaHM/SEB in T-Bau-Räume geplant", Phase: "phase1"},
	{Key: condExamScheduleGenerated, Title: "Terminplan generiert", Phase: "phase1"},
	{Key: condExahmSebFixed, Title: "EXaHM/SEB fixiert (für Phase 2)", Phase: "phase1"},
	{Key: condDraftSent, Title: "Entwurf verschickt", Phase: "phase1", Gate: model.PlanningGateExams},
	{Key: condExamPlanPublished, Title: "Terminplan veröffentlicht (E-Mail)", Phase: "phase1", Gate: model.PlanningGateExams},
	{Key: condRoomRequestsSent, Title: "Raum-Anfragen ans Gebäudemanagement verschickt", Phase: "phase2"},
	{Key: condRoomsAssigned, Title: "Räume zugeordnet", Phase: "phase2"},
	{Key: condSecretariatRoomsSent, Title: "Raumbelegung ans Sekretariat verschickt", Phase: "phase2"},
	{Key: condRoomPlanPublished, Title: "Raumplan veröffentlicht (E-Mail)", Phase: "phase2", Gate: model.PlanningGateRooms},
	{Key: condKdpRoomsSent, Title: "EXaHM/SEB-Raumübersicht ans KDP verschickt", Phase: "phase2"},
	{Key: condInvigilationsRequested, Title: "Aufsichts-Anforderungsabfrage verschickt", Phase: "phase3"},
	{Key: condInvigReqsImported, Title: "Aufsichts-Anforderungen importiert", Phase: "phase3"},
	{Key: condInvigilationsAssigned, Title: "Aufsichten eingeteilt", Phase: "phase3"},
	{Key: condInvigilationPlanPublished, Title: "Aufsichtenplan veröffentlicht (E-Mail)", Phase: "phase3", Gate: model.PlanningGateInvigilations},
	{Key: condInvigSecretariatSent, Title: "Info 'Aufsichten veröffentlicht' ans Sekretariat verschickt", Phase: "phase3"},
	{Key: condNTAPlannedSent, Title: "Info an NTAs zu ihren Räumen verschickt", Phase: "phase3"},
	{Key: condLbaRepeatersSent, Title: "Info Wiederholungsprüfungen LBAs ans LBA-BA verschickt", Phase: "phase3"},
	{Key: condCoverPagesSent, Title: "Deckblätter an alle verschickt (letzter Schritt)", Phase: "phase3"},
}

// The following are thin delegators to the planstate engine (p.planState), so the many
// callers across the package keep using p.markCondition(ctx, condX) etc. unchanged.

func (p *Plexams) PlanningState(ctx context.Context) (*model.PlanningState, error) {
	return p.planState.State(ctx)
}

func (p *Plexams) SetPlanningCondition(ctx context.Context, key string, done bool) (*model.PlanningState, error) {
	return p.planState.SetCondition(ctx, key, done)
}

func (p *Plexams) markCondition(ctx context.Context, key string) {
	p.planState.Mark(ctx, key)
}

func (p *Plexams) unmarkCondition(ctx context.Context, key string) {
	p.planState.Unmark(ctx, key)
}

func (p *Plexams) emailSendAllowed(ctx context.Context, condKey string, run bool) error {
	return p.planState.EmailSendAllowed(ctx, condKey, run)
}

func (p *Plexams) generationAllowed(ctx context.Context, area model.PlanningGate) error {
	return p.planState.GenerationAllowed(ctx, area)
}
