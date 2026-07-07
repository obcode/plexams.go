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

const (
	// defaultTimelagMin is the fallback room/invigilation turnaround (minutes) when
	// the semester config leaves TimelagMin unset. Matches the former generation default.
	defaultTimelagMin = 15
	// defaultNotTooCloseMinutes is the fallback "too close" threshold (minutes) for a
	// student's two exams on the same day when the config leaves it unset.
	defaultNotTooCloseMinutes = 120
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

	// MUC.DAI: absolute start times allowed for MUC.DAI exams (seed from YAML if present).
	mucDaiAllowedTimes := make([]time.Time, 0)
	if raw, ok := viper.Get("semesterConfig.mucDaiAllowedTimes").([]interface{}); ok {
		for _, d := range raw {
			if t, ok := d.(time.Time); ok {
				mucDaiAllowedTimes = append(mucDaiAllowedTimes, t.Local())
			}
		}
	}

	input := &model.SemesterConfigInput{
		From:               viper.GetTime("semesterConfig.from").Local(),
		Until:              viper.GetTime("semesterConfig.until").Local(),
		StartTimes:         viper.GetStringSlice("semesterConfig.startTimes"),
		ForbiddenDays:      forbiddenDays,
		MucDaiAllowedTimes: mucDaiAllowedTimes,
		Emails:             emails,
	}
	if gap := viper.GetInt("planer.examGapMinutes"); gap > 0 {
		input.ExamGapMinutes = &gap
	}
	if lag := viper.GetInt("rooms.timelag"); lag > 0 {
		input.TimelagMin = &lag
	}
	return input
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

	mucDaiAllowedTimes := make([]time.Time, 0, len(data.MucDaiAllowedTimes))
	for _, t := range data.MucDaiAllowedTimes {
		if t != nil {
			mucDaiAllowedTimes = append(mucDaiAllowedTimes, t.Local())
		}
	}

	input := &model.SemesterConfigInput{
		From:               data.From.Local(),
		Until:              data.Until.Local(),
		StartTimes:         data.StartTimes,
		ForbiddenDays:      forbiddenDays,
		MucDaiAllowedTimes: mucDaiAllowedTimes,
		Emails:             emails,
		ExamGapMinutes:     data.ExamGapMinutes,
		TimelagMin:         data.TimelagMin,
		NotTooCloseMinutes: data.NotTooCloseMinutes,
		MaxSeatsPerSlot:    data.MaxSeatsPerSlot,
	}
	if err := validateSemesterConfigInput(input); err != nil {
		return nil, err
	}
	return input, nil
}

// examGapMinutesOf returns the effective travel/break buffer between a student's
// consecutive exams: the configured value, or the built-in default when unset/invalid.
func examGapMinutesOf(input *model.SemesterConfigInput) int {
	if input != nil && input.ExamGapMinutes != nil && *input.ExamGapMinutes > 0 {
		return *input.ExamGapMinutes
	}
	return defaultExamGapMinutes
}

// timelagMinOf returns the effective room/invigilation turnaround (minutes): the
// configured value, or the built-in default when unset/invalid.
func timelagMinOf(input *model.SemesterConfigInput) int {
	if input != nil && input.TimelagMin != nil && *input.TimelagMin > 0 {
		return *input.TimelagMin
	}
	return defaultTimelagMin
}

// notTooCloseMinutesOf returns the effective "too close" threshold (minutes) for a
// student's two exams on the same day: the configured value, or the built-in default.
func notTooCloseMinutesOf(input *model.SemesterConfigInput) int {
	if input != nil && input.NotTooCloseMinutes != nil && *input.NotTooCloseMinutes > 0 {
		return *input.NotTooCloseMinutes
	}
	return defaultNotTooCloseMinutes
}

// maxSeatsPerSlotOf returns the effective per-time seat cap for the Terminplan solver:
// the configured value, or 0 (no limit) when unset/invalid.
func maxSeatsPerSlotOf(input *model.SemesterConfigInput) int {
	if input != nil && input.MaxSeatsPerSlot != nil && *input.MaxSeatsPerSlot > 0 {
		return *input.MaxSeatsPerSlot
	}
	return 0
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
	if len(input.StartTimes) == 0 {
		return fmt.Errorf("at least one start time is required")
	}
	for _, s := range input.StartTimes {
		parts := strings.Split(s, ":")
		if len(parts) != 2 {
			return fmt.Errorf("invalid start time %q (expected HH:MM)", s)
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
// based on the current semester's stored config (slots, emails, MUC.DAI slots and
// — as a starting point — the dates carry over; the planner adjusts the dates).
// Falls back to minimal defaults when nothing is stored.
func (p *Plexams) NewSemesterConfigDefaults(ctx context.Context) (*model.SemesterConfigInput, error) {
	input, err := p.SemesterConfigInput(ctx)
	if err != nil {
		return nil, err
	}
	if input != nil {
		return input, nil
	}
	return &model.SemesterConfigInput{
		StartTimes: []string{"08:30", "10:30", "12:30", "14:30", "16:30"},
		Emails:     &model.Emails{},
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
	// stamp the new database with its (authoritative) logical semester
	logical := strings.Replace(semester, "-", " ", 1)
	if err := p.dbClient.SetMetaSemesterForSemester(ctx, logical, currentSchemaVersion); err != nil {
		log.Error().Err(err).Str("semester", logical).Msg("cannot stamp semester on new database")
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
	if len(old.Starttimes) != len(input.StartTimes) {
		warnings = append(warnings, "Anzahl der Anfangszeiten geändert: gespeicherte Slot-Nummern im Plan können ungültig werden.")
	}
	return warnings
}

// loadSemesterConfig loads the raw per-semester config: from the DB if present,
// otherwise from the YAML (viper), in which case it is migrated into the DB once.
// It then derives and stores the runtime SemesterConfig (days, start times, MUC.DAI slots).
func (p *Plexams) loadSemesterConfig(ctx context.Context) {
	var input *model.SemesterConfigInput
	if p.dbClient != nil {
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

	starttimes := make([]*model.Starttime, 0, len(input.StartTimes))
	for i, start := range input.StartTimes {
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

	mucDaiAllowedTimes := make([]*time.Time, 0, len(input.MucDaiAllowedTimes))
	for i := range input.MucDaiAllowedTimes {
		t := input.MucDaiAllowedTimes[i].Local()
		mucDaiAllowedTimes = append(mucDaiAllowedTimes, &t)
	}

	p.semesterConfig = &model.SemesterConfig{
		Days:               days,
		Starttimes:         starttimes,
		Slots:              slots,
		Emails:             input.Emails,
		MucDaiAllowedTimes: mucDaiAllowedTimes,
		MucDaiSlots:        slots,
		From:               from,
		Until:              until,
		ForbiddenSlots:     forbiddenSlots,
		ExamGapMinutes:     examGapMinutesOf(input),
		TimelagMin:         timelagMinOf(input),
		NotTooCloseMinutes: notTooCloseMinutesOf(input),
		MaxSeatsPerSlot:    maxSeatsPerSlotOf(input),
	}

	p.deriveMucDaiSlots(input.MucDaiAllowedTimes)
}

// deriveMucDaiSlots maps the MUC.DAI allowed start times onto real slots (matching by
// exact absolute start time) and stores them on the semester config.
func (p *Plexams) deriveMucDaiSlots(mucDaiAllowedTimes []time.Time) {
	slotsByStart := make(map[time.Time]*model.Slot)
	for _, slot := range p.semesterConfig.Slots {
		slotsByStart[slot.Starttime] = slot
	}

	mucDaiSlots := make([]*model.Slot, 0, len(mucDaiAllowedTimes))
	for _, t := range mucDaiAllowedTimes {
		if slot, ok := slotsByStart[t.Local()]; ok {
			mucDaiSlots = append(mucDaiSlots, slot)
		}
	}
	p.semesterConfig.MucDaiSlots = mucDaiSlots
}
