package model

type Teacher struct {
	Shortname    string `json:"person_shortname"`
	Fullname     string `json:"person_fullname"`
	IsProf       bool   `json:"is_prof"`
	IsLBA        bool   `json:"is_lba"`
	IsProfHC     bool   `json:"is_profhc"`
	IsStaff      bool   `json:"is_staff"`
	LastSemester string `json:"last_semester"`
	FK           string `json:"fk"`
	ID           int    `json:"person_id"`
	Email        string `json:"email"`
}

type ZPAExam struct {
	Semester       string   `json:"semester"`
	AnCode         int      `json:"anCode"`
	Module         string   `json:"module"`
	MainExamer     string   `json:"main_examer"`
	MainExamerID   int      `json:"main_examer_id"`
	ExamType       string   `json:"exam_type"`
	Duration       int      `json:"duration"`
	IsRepeaterExam bool     `json:"is_repeater_exam"`
	Groups         []string `json:"groups"`
}
