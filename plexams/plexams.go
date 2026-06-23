package plexams

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/zpa"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type Plexams struct {
	semester       string
	dbClient       *db.DB
	zpa            *ZPA
	planer         *Planer
	email          *Email
	semesterConfig *model.SemesterConfig
	// allDays/allSlots hold the FULL list of days/slots from the numbering anchor
	// (usually `from`) through `until`, numbered from day 1. semesterConfig.Days /
	// .Slots only hold the planning window (date >= fromFK07). Code that resolves a
	// stored day number (incl. days before fromFK07, e.g. external exams of other
	// faculties) or indexes by day number must use these full lists.
	allDays  []*model.ExamDay
	allSlots []*model.Slot
	roomInfo map[string]*model.Room
	guard    *opGuard
}

type ZPA struct {
	client       *zpa.ZPA
	baseurl      string
	username     string
	password     string
	token        string
	fk07programs []string
	oldprograms  []string
}

type Planer struct {
	Name  string
	Email string
}

type Email struct {
	server   string
	port     int
	username string
	password string
	// testMail is the recipient used for dry-run sends (run == false). Configured
	// via smtp.testmail; falls back to the planner's address when empty.
	testMail string
	// bcc is added to the Bcc of every real send (smtp.bcc), e.g. a shared mailbox
	// that should receive a copy without being visible to the recipients.
	bcc string
	// replyMail is the Reply-To for mails that may be answered by email
	// (smtp.replymail); falls back to the planner's address when empty.
	replyMail string
	// noreplyMail is the Reply-To for mails that should be answered via JIRA, not
	// by email (smtp.noreplymail); falls back to a noreply alias when empty.
	noreplyMail string
}

func NewPlexams(semester, dbUri, zpaBaseurl, zpaUsername, zpaPassword, zpaToken string, fk07programs, oldprograms []string) (*Plexams, error) {

	var client *db.DB
	var err error
	if dbUri == "" {
		log.Info().Msg("starting without DB!")
	} else {
		dbName := viper.GetString("db.database")
		var databaseName *string
		if dbName == "" {
			databaseName = nil
		} else {
			databaseName = &dbName
		}

		client, err = db.NewDB(dbUri, semester, databaseName)

		if err != nil {
			log.Fatal().Err(err).Msg("cannot connect to plexams.db")
		}
	}

	plexams := Plexams{
		semester: semester,
		dbClient: client,
		zpa: &ZPA{
			client:       nil,
			baseurl:      zpaBaseurl,
			username:     zpaUsername,
			password:     zpaPassword,
			token:        zpaToken,
			fk07programs: fk07programs,
			oldprograms:  oldprograms,
		},
		planer: &Planer{
			Name:  viper.GetString("planer.name"),
			Email: viper.GetString("planer.email"),
		},
		email: &Email{
			server:      viper.GetString("smtp.server.name"),
			port:        viper.GetInt("smtp.server.port"),
			username:    viper.GetString("smtp.username"),
			password:    viper.GetString("smtp.password"),
			testMail:    viper.GetString("smtp.testmail"),
			bcc:         viper.GetString("smtp.bcc"),
			replyMail:   viper.GetString("smtp.replymail"),
			noreplyMail: viper.GetString("smtp.noreplymail"),
		},
		guard: &opGuard{},
	}

	plexams.setSemesterConfig()
	err = plexams.dbClient.SaveSemesterConfig(context.Background(), plexams.semesterConfig)
	if err != nil {
		log.Error().Err(err).Msg("cannot save semester config")
	}

	plexams.setRoomInfo()

	return &plexams, nil
}

func (p *Plexams) SetZPA() error {
	if p.zpa.client == nil {
		zpaClient, err := zpa.NewZPA(p.zpa.baseurl, p.zpa.username, p.zpa.password, p.zpa.token, p.semester)
		if err != nil {
			return err
		}
		p.zpa.client = zpaClient
	}
	return nil
}

func (p *Plexams) setGoSlots() {
	goSlotsValue := viper.Get("goslots")
	goSlotsRaw, ok := goSlotsValue.([]interface{})
	if !ok {
		log.Error().Interface("goSlots", goSlotsRaw).Msg("cannot get go slots from config")
		return
	}
	goSlotsII := make([][]int, 0, len(goSlotsRaw))
	for _, goSlotRaw := range goSlotsRaw {
		goSlot := make([]int, 0, 2)
		for _, intRaw := range goSlotRaw.([]interface{}) {
			number, ok := intRaw.(int)
			if !ok {
				log.Error().Interface("intRaw", intRaw).Msg("cannot convert to int")
				return
			}
			goSlot = append(goSlot, number)
		}
		goSlotsII = append(goSlotsII, goSlot)
	}

	p.semesterConfig.GoSlotsRaw = goSlotsII

	// Calculate real slots. The offset maps the GoDay0-relative day indices from
	// the config onto real day numbers, so it must be computed against the full
	// (anchor-based) day list, not the planning-window subset.
	offset := 0
	for i, day := range p.allDays {
		if p.semesterConfig.GoDay0.Year() == day.Date.Year() &&
			p.semesterConfig.GoDay0.Month() == day.Date.Month() &&
			p.semesterConfig.GoDay0.Day() == day.Date.Day() {
			offset = i + 1
			// fmt.Printf("offset == %d\n", offset)
			break
		}
	}

	type slotNumber struct {
		day, slot int
	}

	slotsMap := make(map[slotNumber]*model.Slot)
	for _, slot := range p.semesterConfig.Slots {
		slotsMap[slotNumber{
			day:  slot.DayNumber,
			slot: slot.SlotNumber,
		}] = slot
	}

	// for k, v := range slotsMap {
	// 	fmt.Printf("slot[%v] = %v\n", k, v)
	// }

	goSlots := make([]*model.Slot, 0, len(goSlotsII))

	for _, goSlot := range goSlotsII {
		slot, ok := slotsMap[slotNumber{
			day:  goSlot[0] + offset,
			slot: goSlot[1],
		}]
		if ok {
			goSlots = append(goSlots, slot)
		}
	}

	// offSet := (p.semesterConfig.GoDay0.Sub(p.semesterConfig.Days[0].Date).Hours() / 24)
	// fmt.Printf("day0 = %v, goday0 = %v, offset = %v\n", p.semesterConfig.Days[0].Date, p.semesterConfig.GoDay0, offset)
	p.semesterConfig.GoSlots = goSlots
	// fmt.Printf("Go-Slots = %+v\n", p.semesterConfig.GoSlots)
}

func (p *Plexams) GetGoSlots() [][]int {
	return p.semesterConfig.GoSlotsRaw
}

func (p *Plexams) GetAllSemesterNames(ctx context.Context) ([]*model.Semester, error) {
	return p.dbClient.AllSemesterNames()
}

func (p *Plexams) GetSemester(ctx context.Context) *model.Semester {
	return &model.Semester{
		ID: p.semester,
	}
}

func (p *Plexams) setSemesterConfig() {
	plan := viper.GetStringMap("semesterConfig")
	if len(plan) > 0 {
		from := viper.GetTime("semesterConfig.from").Local()
		fromFK07 := viper.GetTime("semesterConfig.fromFK07").Local()
		until := viper.GetTime("semesterConfig.until").Local()

		// The planning window always starts at fromFK07. Exam/room/invigilation
		// planning and the GraphQL API only ever see days inside this window.
		//
		// Day numbering starts at the anchor. By default the anchor is fromFK07,
		// so day 1 = fromFK07 and the pre-period does not exist at all. A semester
		// whose plan is already stored with day 1 = `from` (i.e. the pre-period
		// days have numbers 1..) must opt into the legacy numbering by setting
		// `semesterConfig.dayNumberStart: from`; the window then simply starts at
		// a higher number while those stored numbers stay valid.
		anchor := fromFK07
		if viper.GetString("semesterConfig.dayNumberStart") == "from" {
			anchor = from
		}

		// Full list of days from the anchor through until, no saturdays, no sundays.
		allDays := make([]*model.ExamDay, 0)
		day := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 12, 0, 0, 0, time.Local)
		number := 1
		for !day.After(until.Add(23 * time.Hour)) {
			if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
				allDays = append(allDays, &model.ExamDay{
					Number: number,
					Date:   time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.Local),
				})
				number++
			}
			day = day.Add(24 * time.Hour)
		}

		slotStarts := viper.GetStringSlice("semesterConfig.slots")
		starttimes := make([]*model.Starttime, 0, len(slotStarts))
		for i, start := range slotStarts {
			starttimes = append(starttimes, &model.Starttime{
				Number: i + 1,
				Start:  start,
			})
		}

		allSlots := make([]*model.Slot, 0, len(allDays)*len(starttimes))
		for _, day := range allDays {
			for _, starttime := range starttimes {
				start := strings.Split(starttime.Start, ":")
				hour, _ := strconv.Atoi(start[0])
				minute, _ := strconv.Atoi(start[1])
				allSlots = append(allSlots, &model.Slot{
					DayNumber:  day.Number,
					SlotNumber: starttime.Number,
					Starttime:  time.Date(day.Date.Year(), day.Date.Month(), day.Date.Day(), hour, minute, 0, 0, time.Local),
				})
			}
		}

		// Planning window: only days/slots on or after fromFK07.
		fromFK07Day := time.Date(fromFK07.Year(), fromFK07.Month(), fromFK07.Day(), 0, 0, 0, 0, time.Local)
		days := make([]*model.ExamDay, 0, len(allDays))
		for _, d := range allDays {
			if !d.Date.Before(fromFK07Day) {
				days = append(days, d)
			}
		}
		slots := make([]*model.Slot, 0, len(allSlots))
		for _, s := range allSlots {
			if !s.Starttime.Before(fromFK07Day) {
				slots = append(slots, s)
			}
		}

		p.allDays = allDays
		p.allSlots = allSlots

		// Forbidden slots are only meaningful inside the planning window; the
		// pre-period is excluded by the window anyway, so forbiddenDays no longer
		// need to list those days. forbiddenDays within the window still work.
		forbiddenSlots := make([]*model.Slot, 0)
		forbiddenDaysSlice := viper.Get("semesterConfig.forbiddenDays").([]interface{})
		for _, forbiddenDayRaw := range forbiddenDaysSlice {
			forbiddenDay, ok := forbiddenDayRaw.(time.Time)
			if !ok {
				log.Error().Interface("forbiddenDayRaw", forbiddenDayRaw).Msg("cannot convert forbidden day to time.Time")
				continue
			}
			for _, slot := range slots {
				if slot.Starttime.Year() == forbiddenDay.Year() &&
					slot.Starttime.Month() == forbiddenDay.Month() &&
					slot.Starttime.Day() == forbiddenDay.Day() {
					forbiddenSlots = append(forbiddenSlots, slot)
				}
			}
		}

		emails := &model.Emails{}
		emailsMap := viper.GetStringMapString("semesterConfig.emails")
		var ok bool
		emails.Profs, ok = emailsMap["profs"]
		if !ok {
			log.Error().Interface("emails", emailsMap).Msg("cannot get profs emails from config")
		}
		emails.Lbas, ok = emailsMap["lbas"]
		if !ok {
			log.Error().Interface("emails", emailsMap).Msg("cannot get lbas emails from config")
		}
		emails.LbasLastSemester, ok = emailsMap["lbaslastsemester"]
		if !ok {
			log.Error().Interface("emails", emailsMap).Msg("cannot get lbaslastsemester emails from config")
		}

		emails.Fs, ok = emailsMap["fs"]
		if !ok {
			log.Error().Interface("emails", emailsMap).Msg("cannot get fs emails from config")
		}
		emails.Sekr, ok = emailsMap["sekr"]
		if !ok {
			log.Error().Interface("emails", emailsMap).Msg("cannot get sekr emails from config")
		}
		emails.RoomManagement, ok = emailsMap["roommanagement"]
		if !ok {
			log.Error().Interface("emails", emailsMap).Msg("cannot get roommanagement emails from config")
		}

		emails.AdditionalExamer = viper.GetStringSlice("semesterConfig.additionalexamer")
		if len(emails.AdditionalExamer) == 0 {
			log.Debug().Msg("no additionalexamer emails in config")
		}

		p.semesterConfig = &model.SemesterConfig{
			Days:       days,
			Starttimes: starttimes,
			Slots:      slots,
			GoDay0:     viper.GetTime("semesterConfig.goDay0").Local(),
			Emails:     emails,
			// GoSlotsRaw: [][]int{},
			GoSlots:        slots,
			From:           from,
			FromFk07:       fromFK07,
			Until:          until,
			ForbiddenSlots: forbiddenSlots,
		}
	}
	p.setGoSlots()
}

func (p *Plexams) PrintSemesterConfig() {
	fmt.Printf("Semester: %s\n", p.semester)
	fmt.Printf("Days: %v\n", p.semesterConfig.Days)
	fmt.Printf("Starttimes: %v\n", p.semesterConfig.Starttimes)
	fmt.Printf("Slots: %v\n", p.semesterConfig.Slots)
	fmt.Printf("GoDay0: %v\n", p.semesterConfig.GoDay0)
	fmt.Printf("GoSlots: %v\n", p.semesterConfig.GoSlots)
	fmt.Printf("Emails: %v\n", p.semesterConfig.Emails)
}

func (p *Plexams) GetSemesterConfig() *model.SemesterConfig {
	return p.semesterConfig
}

func (p *Plexams) GetStarttime(dayNumber, slotNumber int) (*time.Time, error) {
	for _, slot := range p.allSlots {
		if slot.DayNumber == dayNumber && slot.SlotNumber == slotNumber {
			time := slot.Starttime
			return &time, nil
		}
	}
	return nil, fmt.Errorf("no starttime for slot (%d/%d)", dayNumber, slotNumber)
}

func (p *Plexams) getSlotTime(dayNumber, slotNumber int) time.Time {
	for _, slot := range p.allSlots {
		if slot.DayNumber == dayNumber && slot.SlotNumber == slotNumber {
			return slot.Starttime
		}
	}
	return time.Date(0, 0, 0, 0, 0, 0, 0, nil)
}

func (p *Plexams) getSlotForTime(starttime time.Time, duration int) (*model.Slot, error) {
	var slotWithStarttimeInSlot, slotWithEndtimeInSlot *model.Slot
	endtime := starttime.Add(time.Duration(duration) * time.Minute)
	for _, slot := range p.allSlots {
		if starttime.After(slot.Starttime.Add(-1*time.Minute)) &&
			starttime.Before(slot.Starttime.Add(119*time.Minute)) {
			slotWithStarttimeInSlot = slot
		}
		if endtime.After(slot.Starttime.Add(-1*time.Minute)) &&
			endtime.Before(slot.Starttime.Add(119*time.Minute)) {
			slotWithEndtimeInSlot = slot
		}
		if slotWithStarttimeInSlot != nil &&
			slotWithEndtimeInSlot != nil {
			break
		}
	}

	if slotWithStarttimeInSlot == nil {
		return slotWithEndtimeInSlot, nil
	}
	if slotWithEndtimeInSlot == nil {
		return slotWithStarttimeInSlot, nil
	}

	minutesInEndtimeSlot := int(endtime.Sub(slotWithEndtimeInSlot.Starttime).Minutes())
	if minutesInEndtimeSlot >= duration/2 {
		return slotWithEndtimeInSlot, nil
	}

	return slotWithStarttimeInSlot, nil
}

func (p *Plexams) PrintInfo() {
	fmt.Println(aurora.Sprintf(aurora.Magenta(" ---   Planning Semester: %s   --- \n"), p.semester))
}

func (p *Plexams) setRoomInfo() {
	rooms, err := p.dbClient.Rooms(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("cannot get global rooms")
	}
	roomMap := make(map[string]*model.Room)
	for _, room := range rooms {
		roomMap[room.Name] = room
	}

	p.roomInfo = roomMap
}

func (p *Plexams) GetRoomInfo(roomName string) *model.Room {
	return p.roomInfo[roomName]
}

// roomInfoMapFromDB reads all rooms fresh from the DB into a name→room map.
// Validation uses this rather than the in-memory roomInfo map (built once at
// startup), so it sees rooms added or changed at runtime.
func (p *Plexams) roomInfoMapFromDB(ctx context.Context) (map[string]*model.Room, error) {
	rooms, err := p.dbClient.Rooms(ctx)
	if err != nil {
		return nil, err
	}
	roomInfos := make(map[string]*model.Room, len(rooms))
	for _, room := range rooms {
		roomInfos[room.Name] = room
	}
	return roomInfos, nil
}
