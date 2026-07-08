package plexams

import (
	"context"
	"fmt"

	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/obcode/plexams.go/plexams/pdfgen"
)

func (p *Plexams) DraftFSBytes(ctx context.Context) ([]byte, error) {
	return marotoBytes(p.draftFS(ctx))
}

func (p *Plexams) draftFS(ctx context.Context) pdf.Maroto {
	m := pdfgen.DraftDoc(false,
		fmt.Sprintf("Vorläufiger Planungsstand der Prüfungen der FK07 im %s", p.semesterFull()),
		p.planer.Name, p.planer.Email, "--- ENTWURF ---")

	p.tableForProgram(ctx, "DA", "Master Data Analytics (DA)", m)
	p.tableForProgram(ctx, "DC", "Bachelor Data Science & Scientific Computing (DC)", m)
	p.tableForProgram(ctx, "IB", "Bachelor Wirtschaftsinformatik (IB)", m)
	// p.tableForProgram(ctx, "IC", "Bachelor Scientific Computing (IC)", m)
	p.tableForProgram(ctx, "IF", "Bachelor Informatik (IF)", m)
	p.tableForProgram(ctx, "IG", "Master Informatik (IG)", m)
	p.tableForProgram(ctx, "IN", "Master Wirtschaftsinformatik (IN)", m)
	// p.tableForProgram(ctx, "IS", "Master Stochastic Engineering in Business and Finance (IS)", m)
	p.tableForProgram(ctx, "IT", "Master IT-Sicherheit (IT)", m)
	p.tableForProgram(ctx, "WD", "Bachelor Wirtschaftsinformatik - Digitales Management (WD)", m)
	p.tableForProgram(ctx, "WT", "Bachelor Wirtschaftsinformatik - Informationstechnologie (WT)", m)

	return m
}
