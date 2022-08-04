package zpa

import (
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
)

func (zpa *ZPA) GetTeachers() []*model.Teacher {
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
