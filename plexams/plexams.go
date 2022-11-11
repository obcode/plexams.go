package plexams

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gookit/color"
	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/zpa"
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
	client, err := db.NewDB(dbUri, semester)

	if err != nil {
		panic(fmt.Errorf("fatal cannot create mongo client: %w", err))
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
		from := viper.GetTime("semesterConfig.from")
		until := viper.GetTime("semesterConfig.until")
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

		p.semesterConfig = &model.SemesterConfig{
			Days:       days,
			Starttimes: starttimes,
			Slots:      slots,
		}
	}
}

func (p *Plexams) GetSemesterConfig() *model.SemesterConfig {
	return p.semesterConfig
}

func (p *Plexams) PrintSemester() {
	color.Style{color.FgCyan, color.BgYellow, color.OpBold}.Printf(" ---   Planning Semester: %s   --- ", p.semester)
	color.Println()
}
