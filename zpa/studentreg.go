package zpa

import (
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
)

func (zpa *ZPA) StudentReg2ZPAStudentReg(studreg *model.StudentReg) *model.ZPAStudentReg {
	return &model.ZPAStudentReg{
		Semester: zpa.semester,
		AnCode:   studreg.AnCode,
		Mtknr:    studreg.Mtknr,
		Program:  studreg.Program,
	}
}

func (zpa *ZPA) PostStudentRegsToZPA(studentRegs []*model.ZPAStudentReg) (string, []byte, error) {
	return zpa.post("application", studentRegs)
}

func (zpa *ZPA) DeleteStudentRegsFromZPA(ancodes []*model.ZPAAncodes) (string, []byte, error) {
	return zpa.post("delete_applications", ancodes)
}

func (zpa *ZPA) GetStudents(mtknr string) ([]*model.ZPAStudent, error) {
	var zpaStudent []*model.ZPAStudent
	err := zpa.get(fmt.Sprintf("get_student_info?ask=%s", mtknr), &zpaStudent)
	return zpaStudent, err
}
