package zpa

import (
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (zpa *ZPA) GetExams() []*model.ZPAExam {
	return zpa.exams
}

func (zpa *ZPA) getExams() error {
	err := zpa.get(fmt.Sprintf("exams?semester=%s&all=true", strings.Replace(zpa.semester, " ", "%20", 1)), &zpa.exams)
	if err != nil {
		fmt.Printf("Error %s", err)
		return err
	}
	return nil
}

func (zpa *ZPA) PostExams(exams []*model.ZPAExamPlan) (string, []byte, error) {
	log.Debug().Int("count", len(exams)).Msg("posting exams to zpa")
	return zpa.post("exam_plan", exams)
}

type PlannedExam struct {
	Ancode            int    `json:"anCode"`
	Module            string `json:"module"`
	MainExamer        string `json:"main_examer"`
	MainExamerID      int    `json:"main_examer_id"`
	Number            int    `json:"number"`
	Date              string `json:"date"`
	Starttime         string `json:"start_time"`
	RoomName          string `json:"room"`
	IsReserve         bool   `json:"is_reserve"`
	IsHandicap        bool   `json:"is_handicap"`
	Supervisor        string `json:"supervisor"`
	Duration          int    `json:"duration"`
	ReserveSupervisor string `json:"reserve_supervisor"`
}

func (zpa *ZPA) GetPlannedExams() ([]*PlannedExam, error) {
	plannedExams := make([]*PlannedExam, 0)
	err := zpa.get(fmt.Sprintf("exam_schedules?semester=%s&all=true", strings.Replace(zpa.semester, " ", "%20", 1)), &plannedExams)
	return plannedExams, err
}
