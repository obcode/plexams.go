package plexams

import "fmt"

func (p *Plexams) ValidateZPA(withRooms, withInvigilators bool) error {
	if err := p.SetZPA(); err != nil {
		return err
	}

	plannedExamsFromZPA, err := p.zpa.client.GetPlannedExams()
	if err != nil {
		return err
	}

	fmt.Printf("%+v\n", plannedExamsFromZPA)

	return nil
}
