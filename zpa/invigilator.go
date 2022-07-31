package zpa

import "fmt"

type SupervisorRequirements struct {
	Invigilator            string   `json:"invigilator"`
	InvigilatorID          int      `json:"invigilator_id"`
	ExcludedDates          []string `json:"excluded_dates"`
	PartTime               float32  `json:"part_time"`
	OralExamsContribution  int      `json:"oral_exams_contribution"`
	LivecodingContribution int      `json:"livecoding_contribution"`
	MasterContribution     int      `json:"master_contribution"`
	FreeSemester           float32  `json:"free_semester"`
	OvertimeLastSemester   float32  `json:"overtime_last_semester"`
	OvertimeThisSemester   float32  `json:"overtime_this_semester"`
}

func (zpa *ZPA) GetSupervisorRequirements() []*SupervisorRequirements {
	return zpa.supervisorRequirements
}

func (zpa *ZPA) getSupervisorRequirements() error {
	err := zpa.get(fmt.Sprintf("supervisorrequirements?semester=%s", zpa.semester),
		&zpa.supervisorRequirements)
	if err != nil {
		fmt.Printf("Error %s", err)
		return err
	}
	return nil
}
