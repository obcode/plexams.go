package plexams

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// semesterConfigInputFromViper reads the raw per-semester config from the YAML
// (viper). It returns nil when no semesterConfig block is present, so the caller
// can fall back to the DB-stored input.
func semesterConfigInputFromViper() *model.SemesterConfigInput {
	if len(viper.GetStringMap("semesterConfig")) == 0 {
		return nil
	}

	emailsMap := viper.GetStringMapString("semesterConfig.emails")
	emails := &model.Emails{
		Profs:            emailsMap["profs"],
		Lbas:             emailsMap["lbas"],
		LbasLastSemester: emailsMap["lbaslastsemester"],
		Fs:               emailsMap["fs"],
		Sekr:             emailsMap["sekr"],
		RoomManagement:   emailsMap["roommanagement"],
		Kdp:              emailsMap["kdp"],
		Lbaba:            emailsMap["lbaba"],
		AdditionalExamer: viper.GetStringSlice("semesterConfig.additionalexamer"),
	}

	forbiddenDays := make([]time.Time, 0)
	if raw, ok := viper.Get("semesterConfig.forbiddenDays").([]interface{}); ok {
		for _, d := range raw {
			if t, ok := d.(time.Time); ok {
				forbiddenDays = append(forbiddenDays, t.Local())
			}
		}
	}

	// `from` now has the semantics of the former `fromFK07` (day 1 = from, no
	// pre-period). For a legacy YAML the planning start is the former numbering
	// anchor: `from` when dayNumberStart was "from", otherwise `fromFK07` — this
	// keeps existing day numbers stable.
	from := viper.GetTime("semesterConfig.from").Local()
	if viper.GetString("semesterConfig.dayNumberStart") != "from" {
		if fromFK07 := viper.GetTime("semesterConfig.fromFK07").Local(); !fromFK07.IsZero() {
			from = fromFK07
		}
	}

	return &model.SemesterConfigInput{
		From:          from,
		Until:         viper.GetTime("semesterConfig.until").Local(),
		Slots:         viper.GetStringSlice("semesterConfig.slots"),
		GoDay0:        viper.GetTime("semesterConfig.goDay0").Local(),
		ForbiddenDays: forbiddenDays,
		GoSlots:       goSlotsFromViper(),
		Emails:        emails,
	}
}

// goSlotsFromViper reads the top-level goslots block ([][]int) from the YAML.
func goSlotsFromViper() [][]int {
	goSlotsRaw, ok := viper.Get("goslots").([]interface{})
	if !ok {
		return nil
	}
	goSlots := make([][]int, 0, len(goSlotsRaw))
	for _, goSlotRaw := range goSlotsRaw {
		inner, ok := goSlotRaw.([]interface{})
		if !ok {
			continue
		}
		goSlot := make([]int, 0, 2)
		for _, intRaw := range inner {
			number, ok := intRaw.(int)
			if !ok {
				log.Error().Interface("intRaw", intRaw).Msg("cannot convert go slot entry to int")
				continue
			}
			goSlot = append(goSlot, number)
		}
		goSlots = append(goSlots, goSlot)
	}
	return goSlots
}

// SemesterConfigInput returns the raw, editable per-semester config (source of
// truth). It falls back to the YAML when nothing is stored yet, so the GUI can
// always show and then save the current values.
func (p *Plexams) SemesterConfigInput(ctx context.Context) (*model.SemesterConfigInput, error) {
	input, err := p.dbClient.GetSemesterConfigInput(ctx)
	if err != nil {
		return nil, err
	}
	if input == nil {
		input = semesterConfigInputFromViper()
	}
	return input, nil
}

// SetSemesterConfigInput validates and stores a new raw per-semester config,
// recomputes the derived config and snapshot, and returns non-fatal warnings for
// changes that may invalidate an existing plan (the change is still applied).
func (p *Plexams) SetSemesterConfigInput(ctx context.Context, data *model.SemesterConfigInputData) (*model.SaveSemesterConfigResult, error) {
	input, err := semesterConfigInputFromData(data)
	if err != nil {
		return nil, err
	}

	warnings := p.semesterConfigChangeWarnings(ctx, input)

	if err := p.dbClient.SaveSemesterConfigInput(ctx, input); err != nil {
		return nil, fmt.Errorf("cannot save semester config: %w", err)
	}

	p.deriveSemesterConfig(input)
	if err := p.dbClient.SaveSemesterConfig(ctx, p.semesterConfig); err != nil {
		log.Error().Err(err).Msg("cannot save derived semester config")
	}

	return &model.SaveSemesterConfigResult{Ok: true, Warnings: warnings}, nil
}

// semesterConfigInputFromData converts the GraphQL input into the stored model
// and validates it.
func semesterConfigInputFromData(data *model.SemesterConfigInputData) (*model.SemesterConfigInput, error) {
	if data == nil {
		return nil, fmt.Errorf("no config provided")
	}

	forbiddenDays := make([]time.Time, 0, len(data.ForbiddenDays))
	for _, d := range data.ForbiddenDays {
		if d != nil {
			forbiddenDays = append(forbiddenDays, d.Local())
		}
	}

	var emails *model.Emails
	if data.Emails != nil {
		emails = &model.Emails{
			Profs:            data.Emails.Profs,
			Lbas:             data.Emails.Lbas,
			LbasLastSemester: data.Emails.LbasLastSemester,
			AdditionalExamer: data.Emails.AdditionalExamer,
			Fs:               data.Emails.Fs,
			Sekr:             data.Emails.Sekr,
			RoomManagement:   data.Emails.RoomManagement,
			Kdp:              data.Emails.Kdp,
			Lbaba:            data.Emails.Lbaba,
		}
	}

	input := &model.SemesterConfigInput{
		From:          data.From.Local(),
		Until:         data.Until.Local(),
		Slots:         data.Slots,
		GoDay0:        data.GoDay0.Local(),
		ForbiddenDays: forbiddenDays,
		GoSlots:       data.GoSlots,
		Emails:        emails,
	}
	if err := validateSemesterConfigInput(input); err != nil {
		return nil, err
	}
	return input, nil
}

// validateSemesterConfigInput checks date ordering and slot start-time format.
func validateSemesterConfigInput(input *model.SemesterConfigInput) error {
	if input == nil {
		return fmt.Errorf("no config provided")
	}
	if input.From.After(input.Until) {
		return fmt.Errorf("from (%s) must not be after until (%s)",
			input.From.Format("2006-01-02"), input.Until.Format("2006-01-02"))
	}
	if len(input.Slots) == 0 {
		return fmt.Errorf("at least one slot start time is required")
	}
	for _, s := range input.Slots {
		parts := strings.Split(s, ":")
		if len(parts) != 2 {
			return fmt.Errorf("invalid slot start time %q (expected HH:MM)", s)
		}
		hour, errH := strconv.Atoi(parts[0])
		minute, errM := strconv.Atoi(parts[1])
		if errH != nil || errM != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return fmt.Errorf("invalid slot start time %q (expected HH:MM)", s)
		}
	}
	return nil
}

var semesterNameRE = regexp.MustCompile(`^\d{4}-(SS|WS)$`)

// NewSemesterConfigDefaults returns a template for creating a new semester,
// based on the current semester's stored config (slots, emails, go-slots and — as
// a starting point — the dates carry over; the planner adjusts the dates). Falls
// back to minimal defaults when nothing is stored.
func (p *Plexams) NewSemesterConfigDefaults(ctx context.Context) (*model.SemesterConfigInput, error) {
	input, err := p.SemesterConfigInput(ctx)
	if err != nil {
		return nil, err
	}
	if input != nil {
		return input, nil
	}
	return &model.SemesterConfigInput{
		Slots:  []string{"08:30", "10:30", "12:30", "14:30", "16:30"},
		Emails: &model.Emails{},
	}, nil
}

// CreateSemester stores the config for a new semester in its own database. It
// refuses an invalid semester name or a semester that already has a config.
func (p *Plexams) CreateSemester(ctx context.Context, semester string, data *model.SemesterConfigInputData) (*model.SaveSemesterConfigResult, error) {
	input, err := semesterConfigInputFromData(data)
	if err != nil {
		return nil, err
	}
	return p.createSemesterWithInput(ctx, semester, input)
}

// CreateSemesterFromInput creates a new semester from an already-built raw
// config (the CLI init entry point).
func (p *Plexams) CreateSemesterFromInput(ctx context.Context, semester string, input *model.SemesterConfigInput) (*model.SaveSemesterConfigResult, error) {
	return p.createSemesterWithInput(ctx, semester, input)
}

// createSemesterWithInput is the shared core for the GUI mutation and the CLI
// init command.
func (p *Plexams) createSemesterWithInput(ctx context.Context, semester string, input *model.SemesterConfigInput) (*model.SaveSemesterConfigResult, error) {
	semester = strings.TrimSpace(semester)
	if !semesterNameRE.MatchString(semester) {
		return nil, fmt.Errorf("invalid semester %q (expected YYYY-SS or YYYY-WS)", semester)
	}
	if err := validateSemesterConfigInput(input); err != nil {
		return nil, err
	}

	existing, err := p.dbClient.GetSemesterConfigInputForSemester(ctx, semester)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("semester %q already has a config — edit it instead of creating it", semester)
	}

	if err := p.dbClient.SaveSemesterConfigInputForSemester(ctx, semester, input); err != nil {
		return nil, fmt.Errorf("cannot save config for new semester: %w", err)
	}
	return &model.SaveSemesterConfigResult{Ok: true, Warnings: []string{}}, nil
}

// semesterConfigChangeWarnings compares the new input against the currently
// derived config and reports changes that shift stored plan day/slot numbers.
func (p *Plexams) semesterConfigChangeWarnings(ctx context.Context, input *model.SemesterConfigInput) []string {
	warnings := make([]string, 0)
	old := p.semesterConfig
	if old == nil {
		return warnings
	}

	planExists := false
	if entries, err := p.dbClient.PlanEntries(ctx); err == nil && len(entries) > 0 {
		planExists = true
	}
	if !planExists {
		return warnings
	}

	if !old.From.Equal(input.From) {
		warnings = append(warnings, "from geändert: gespeicherte Tag-Nummern im Plan verschieben sich.")
	}
	if len(old.Starttimes) != len(input.Slots) {
		warnings = append(warnings, "Anzahl der Slots geändert: gespeicherte Slot-Nummern im Plan können ungültig werden.")
	}
	return warnings
}

// loadSemesterConfig loads the raw per-semester config: from the DB if present,
// otherwise from the YAML (viper), in which case it is migrated into the DB once.
// It then derives and stores the runtime SemesterConfig (days, slots, go-slots).
func (p *Plexams) loadSemesterConfig(ctx context.Context) {
	var input *model.SemesterConfigInput
	if p.dbClient != nil {
		if err := p.dbClient.MigrateLegacySemesterConfigInput(ctx); err != nil {
			log.Error().Err(err).Msg("cannot migrate legacy semester config input")
		}
		var err error
		input, err = p.dbClient.GetSemesterConfigInput(ctx)
		if err != nil {
			log.Error().Err(err).Msg("cannot read semester config input from db")
		}
	}

	if input == nil {
		// No stored config yet: read the YAML and migrate it into the DB once, so
		// subsequent starts no longer depend on <semester>.yaml.
		input = semesterConfigInputFromViper()
		if input != nil && p.dbClient != nil {
			if err := p.dbClient.SaveSemesterConfigInput(ctx, input); err != nil {
				log.Error().Err(err).Msg("cannot migrate semester config input into db")
			} else {
				log.Info().Msg("migrated semester config from YAML into the database")
			}
		}
	}

	if input == nil {
		log.Error().Msg("no semester config found (neither in db nor in yaml)")
		return
	}

	p.deriveSemesterConfig(input)
}

// deriveSemesterConfig computes the runtime SemesterConfig (days, slots, forbidden
// slots, go-slots) from the raw input and stores it on p (semesterConfig, allDays,
// allSlots). Day 1 = from; there is no pre-period. input must be non-nil.
func (p *Plexams) deriveSemesterConfig(input *model.SemesterConfigInput) {
	from := input.From.Local()
	until := input.Until.Local()

	// Days from `from` through until, no saturdays, no sundays; day 1 = from.
	days := make([]*model.ExamDay, 0)
	day := time.Date(from.Year(), from.Month(), from.Day(), 12, 0, 0, 0, time.Local)
	number := 1
	for !day.After(until.Add(23 * time.Hour)) {
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			days = append(days, &model.ExamDay{
				Number: number,
				Date:   time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.Local),
			})
			number++
		}
		day = day.Add(24 * time.Hour)
	}

	starttimes := make([]*model.Starttime, 0, len(input.Slots))
	for i, start := range input.Slots {
		starttimes = append(starttimes, &model.Starttime{
			Number: i + 1,
			Start:  start,
		})
	}

	slots := make([]*model.Slot, 0, len(days)*len(starttimes))
	for _, day := range days {
		for _, starttime := range starttimes {
			start := strings.Split(starttime.Start, ":")
			hour, _ := strconv.Atoi(start[0])
			minute, _ := strconv.Atoi(start[1])
			slots = append(slots, &model.Slot{
				DayNumber:  day.Number,
				SlotNumber: starttime.Number,
				Starttime:  time.Date(day.Date.Year(), day.Date.Month(), day.Date.Day(), hour, minute, 0, 0, time.Local),
			})
		}
	}

	p.allDays = days
	p.allSlots = slots

	forbiddenSlots := make([]*model.Slot, 0)
	for _, forbiddenDay := range input.ForbiddenDays {
		for _, slot := range slots {
			if slot.Starttime.Year() == forbiddenDay.Year() &&
				slot.Starttime.Month() == forbiddenDay.Month() &&
				slot.Starttime.Day() == forbiddenDay.Day() {
				forbiddenSlots = append(forbiddenSlots, slot)
			}
		}
	}

	p.semesterConfig = &model.SemesterConfig{
		Days:           days,
		Starttimes:     starttimes,
		Slots:          slots,
		GoDay0:         input.GoDay0.Local(),
		Emails:         input.Emails,
		GoSlots:        slots,
		From:           from,
		Until:          until,
		ForbiddenSlots: forbiddenSlots,
	}

	p.deriveGoSlots(input.GoSlots)
}

// deriveGoSlots maps the raw go-slot pairs ([dayOffsetFromGoDay0, slotNumber])
// onto real slots and stores them on the semester config. The offset maps the
// GoDay0-relative day indices onto real day numbers (day 1 = from).
func (p *Plexams) deriveGoSlots(goSlotsRaw [][]int) {
	p.semesterConfig.GoSlotsRaw = goSlotsRaw

	offset := 0
	for i, day := range p.allDays {
		if p.semesterConfig.GoDay0.Year() == day.Date.Year() &&
			p.semesterConfig.GoDay0.Month() == day.Date.Month() &&
			p.semesterConfig.GoDay0.Day() == day.Date.Day() {
			offset = i + 1
			break
		}
	}

	type slotNumber struct {
		day, slot int
	}
	slotsMap := make(map[slotNumber]*model.Slot)
	for _, slot := range p.semesterConfig.Slots {
		slotsMap[slotNumber{day: slot.DayNumber, slot: slot.SlotNumber}] = slot
	}

	goSlots := make([]*model.Slot, 0, len(goSlotsRaw))
	for _, goSlot := range goSlotsRaw {
		if len(goSlot) < 2 {
			continue
		}
		if slot, ok := slotsMap[slotNumber{day: goSlot[0] + offset, slot: goSlot[1]}]; ok {
			goSlots = append(goSlots, slot)
		}
	}
	p.semesterConfig.GoSlots = goSlots
}
