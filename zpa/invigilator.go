package zpa

import (
	"fmt"
	"strings"
)

type SupervisorRequirements struct {
	Invigilator            string   `json:"invigilator"`
	InvigilatorID          int      `json:"invigilator_id"`
	ExcludedDates          []string `json:"excluded_dates"`
	PartTime               float64  `json:"part_time"`
	OralExamsContribution  int      `json:"oral_exams_contribution"`
	LivecodingContribution int      `json:"livecoding_contribution"`
	MasterContribution     int      `json:"master_contribution"`
	FreeSemester           float64  `json:"free_semester"`
	OvertimeLastSemester   float64  `json:"overtime_last_semester"`
	OvertimeThisSemester   float64  `json:"overtime_this_semester"`
}

func (zpa *ZPA) GetSupervisorRequirements() ([]*SupervisorRequirements, error) {
	if err := zpa.getSupervisorRequirements(); err != nil {
		return nil, err
	}
	return zpa.supervisorRequirements, nil
}

func (zpa *ZPA) getSupervisorRequirements() error {
	return zpa.get(fmt.Sprintf("supervisorrequirements?semester=%s", strings.Replace(zpa.semester, " ", "%20", 1)),
		&zpa.supervisorRequirements)
}
