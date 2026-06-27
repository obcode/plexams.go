package zpa

import (
	"github.com/obcode/plexams.go/graph/model"
)

func (zpa *ZPA) GetTeachers() []*model.Teacher {
	return zpa.teachers
}

func (zpa *ZPA) getTeachers() error {
	return zpa.get("teachers", &zpa.teachers)
}
