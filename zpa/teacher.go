package zpa

import "fmt"

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

func (zpa *ZPA) GetTeachers() []*Teacher {
	return zpa.teachers
}

func (zpa *ZPA) getTeachers() error {
	err := zpa.get("teachers", &zpa.teachers)
	if err != nil {
		fmt.Printf("Error %s", err)
		return err
	}
	return nil
}
