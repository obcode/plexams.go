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
	workflow       []*model.Step
	planer         *Planer
	email          *Email
	semesterConfig *model.SemesterConfig
	roomInfo       map[string]*model.Room
}

type ZPA struct {
	client       *zpa.ZPA
	baseurl      string
	username     string
	password     string
	fk07programs []string
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
}

func NewPlexams(semester, dbUri, zpaBaseurl, zpaUsername, zpaPassword string, fk07programs []string) (*Plexams, error) {

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
			fk07programs: fk07programs,
		},
		workflow: initWorkflow(),
		planer: &Planer{
			Name:  viper.GetString("planer.name"),
			Email: viper.GetString("planer.email"),
		},
		email: &Email{
			server:   viper.GetString("smtp.server.name"),
			port:     viper.GetInt("smtp.server.port"),
			username: viper.GetString("smtp.username"),
			password: viper.GetString("smtp.password"),
		},
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
		zpaClient, err := zpa.NewZPA(p.zpa.baseurl, p.zpa.username, p.zpa.password, p.semester)
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

	// Calculate real slots
	offset := 0
	for i, day := range p.semesterConfig.Days {
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

func (p *Plexams) SetSemester(ctx context.Context, s string) (*model.Semester, error) {
	p.semester = s
	err := p.dbClient.SetSemester(s)
	if err != nil {
		return nil, err
	}
	return &model.Semester{
		ID: p.semester,
	}, nil
}

func (p *Plexams) Log(ctx context.Context, subj, msg string) error {
	return p.dbClient.Log(ctx, subj, msg)
}

func (p *Plexams) setSemesterConfig() {
	plan := viper.GetStringMap("semesterConfig")
	if len(plan) > 0 {
		// Days from ... until, no saturdays, no sundays
		from := viper.GetTime("semesterConfig.from").Local()
		until := viper.GetTime("semesterConfig.until").Local()
		days := make([]*model.ExamDay, 0)
		day := from
		number := 1
		for !day.After(until) {
			if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
				days = append(days, &model.ExamDay{
					Number: number,
					Date:   time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local),
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

		slots := make([]*model.Slot, 0, len(days)*len(starttimes))
		for _, day := range days {
			for _, starttime := range starttimes {
				start := strings.Split(starttime.Start, ":")
				hour, _ := strconv.Atoi(start[0])
				minute, _ := strconv.Atoi(start[1])
				slots = append(slots, &model.Slot{
					DayNumber:  day.Number,
					SlotNumber: starttime.Number,
					Starttime:  day.Date.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute),
				})
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
		emails.Fs, ok = emailsMap["fs"]
		if !ok {
			log.Error().Interface("emails", emailsMap).Msg("cannot get fs emails from config")
		}

		p.semesterConfig = &model.SemesterConfig{
			Days:       days,
			Starttimes: starttimes,
			Slots:      slots,
			GoDay0:     viper.GetTime("semesterConfig.goDay0").Local(),
			Emails:     emails,
			// GoSlotsRaw: [][]int{},
			GoSlots:  slots,
			From:     from,
			FromFk07: viper.GetTime("semesterConfig.fromFK07").Local(),
			Until:    until,
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
	for _, slot := range p.semesterConfig.Slots {
		if slot.DayNumber == dayNumber && slot.SlotNumber == slotNumber {
			time := slot.Starttime.Local()
			return &time, nil
		}
	}
	return nil, fmt.Errorf("no starttime for slot (%d/%d)", dayNumber, slotNumber)
}

func (p *Plexams) getSlotTime(dayNumber, slotNumber int) time.Time {
	for _, slot := range p.semesterConfig.Slots {
		if slot.DayNumber == dayNumber && slot.SlotNumber == slotNumber {
			return slot.Starttime.Local()
		}
	}
	return time.Date(0, 0, 0, 0, 0, 0, 0, nil)
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

func (p *Plexams) Room(ctx context.Context, roomForExam *model.RoomForExam) (*model.Room, error) {
	room := p.GetRoomInfo(roomForExam.RoomName)
	if room == nil {
		log.Error().Str("room name", roomForExam.RoomName).Msg("cannot find room in global rooms")
	}

	return room, nil
}
