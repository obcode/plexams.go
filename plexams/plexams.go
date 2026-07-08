package plexams

import (
	"context"
	"fmt"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/anny"
	"github.com/obcode/plexams.go/plexams/email"
	"github.com/obcode/plexams.go/plexams/planstate"
	"github.com/obcode/plexams.go/plexams/primuss"
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
	sender         *email.Sender
	anny           *anny.Service
	primuss        *primuss.Service
	planState      *planstate.Machine
	semesterConfig *model.SemesterConfig
	// allDays/allSlots hold the list of days/slots from `from` (day 1) through
	// `until`. They currently equal semesterConfig.Days/.Slots (there is no
	// pre-period anymore); kept as separate fields for the callers that resolve a
	// stored day number or index by day number.
	allDays  []*model.ExamDay
	allSlots []*model.Slot
	roomInfo map[string]*model.Room
	// readOnly, when true, makes the AroundOperations middleware reject all
	// data-changing operations (so a semester can be inspected without changing it);
	// loaded per database from the semester meta on boot/switch.
	readOnly bool
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
	// cc is added to the Cc of every real send (smtp.cc), e.g. a shared mailbox.
	cc string
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
			cc:          viper.GetString("smtp.cc"),
			replyMail:   viper.GetString("smtp.replymail"),
			noreplyMail: viper.GetString("smtp.noreplymail"),
		},
		guard: &opGuard{},
	}
	plexams.sender = email.NewSender(email.SMTPConfig{
		Server:      plexams.email.server,
		Port:        plexams.email.port,
		Username:    plexams.email.username,
		Password:    plexams.email.password,
		TestMail:    plexams.email.testMail,
		CC:          plexams.email.cc,
		ReplyMail:   plexams.email.replyMail,
		NoreplyMail: plexams.email.noreplyMail,
		PlanerName:  plexams.planer.Name,
		PlanerEmail: plexams.planer.Email,
	})

	var annyDB anny.DB
	if plexams.dbClient != nil {
		annyDB = plexams.dbClient
	}
	plexams.anny = anny.New(annyDB, anny.Config{
		Token:                    viper.GetString("anny.token"),
		URL:                      viper.GetString("anny.url"),
		SeedPersonalizationNames: personalizationNamesFromConfig(),
	})
	if plexams.dbClient != nil {
		plexams.primuss = primuss.New(plexams.dbClient)
	}
	var planStateDB planstate.DB
	if plexams.dbClient != nil {
		planStateDB = plexams.dbClient
	}
	plexams.planState = planstate.New(planStateDB, planningPhaseDefs, planningConditionDefs)

	if plexams.dbClient != nil {
		ctx := context.Background()
		// FK07 programs come from the StudyProgram master data when present; the
		// config values are only the bootstrap/seed fallback.
		if current, old, err := plexams.fk07ProgramsFromStudyPrograms(ctx); err != nil {
			log.Error().Err(err).Msg("cannot read fk07 programs from study programs")
		} else if len(current) > 0 || len(old) > 0 {
			plexams.zpa.fk07programs = current
			plexams.zpa.oldprograms = old
		}
		// The planner is read from the DB when present (config is the fallback).
		if planer, err := plexams.dbClient.GetPlaner(ctx); err != nil {
			log.Error().Err(err).Msg("cannot read planer from db")
		} else if planer != nil {
			plexams.applyPlaner(planer.Name, planer.Email)
		}
	}

	plexams.loadSemesterConfig(context.Background())
	if plexams.semesterConfig != nil && plexams.dbClient != nil {
		// keep the derived snapshot in the DB for the GUI to read directly
		if err := plexams.dbClient.SaveSemesterConfig(context.Background(), plexams.semesterConfig); err != nil {
			log.Error().Err(err).Msg("cannot save semester config")
		}
	}
	plexams.loadSemesterMeta(context.Background())

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

func (p *Plexams) GetAllSemesterNames(ctx context.Context) ([]*model.Semester, error) {
	return p.dbClient.AllSemesterNames(ctx)
}

func (p *Plexams) GetSemester(ctx context.Context) *model.Semester {
	v := currentSchemaVersion
	s := p.semester
	return &model.Semester{
		ID:            p.dbClient.DatabaseName(),
		Semester:      &s,
		Compatible:    p.semesterConfig != nil,
		ReadOnly:      p.readOnly,
		SchemaVersion: &v,
	}
}

func (p *Plexams) PrintSemesterConfig() {
	fmt.Printf("Semester: %s\n", p.semester)
	fmt.Printf("Days: %v\n", p.semesterConfig.Days)
	fmt.Printf("Starttimes: %v\n", p.semesterConfig.Starttimes)
	fmt.Printf("Slots: %v\n", p.semesterConfig.Slots)
	fmt.Printf("MUC.DAI-Slots: %v\n", p.semesterConfig.MucDaiSlots)
	fmt.Printf("Emails: %v\n", p.semesterConfig.Emails)
}

func (p *Plexams) GetSemesterConfig() *model.SemesterConfig {
	return p.semesterConfig
}

// dayNumberForDate returns the 1-based exam-day number of the given calendar date (0
// when it is not an exam day). The number is the day's position in the ascending list
// of exam days, derived locally from the config — no stored day ordinal.
func (p *Plexams) dayNumberForDate(date time.Time) int {
	d := date.Local()
	for i, day := range p.allDays {
		if day.Date.Year() == d.Year() && day.Date.Month() == d.Month() && day.Date.Day() == d.Day() {
			return i + 1
		}
	}
	return 0
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
