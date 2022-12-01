package zpa

import "github.com/obcode/plexams.go/graph/model"

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
