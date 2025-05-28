package plexams

import (
	"context"
	"fmt"
	"time"

	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) DraftFS(ctx context.Context, outfile string) error {
	m := p.draftFS(ctx)
	err := m.OutputFileAndClose(outfile)
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF")
		return err
	}
	return nil
}

func (p *Plexams) DraftFSBytes(ctx context.Context) ([]byte, error) {
	m := p.draftFS(ctx)
	buf, err := m.Output()
	if err != nil {
		log.Error().Err(err).Msg("Could not save PDF to bytes")
		return nil, err
	}
	return buf.Bytes(), nil
}

func (p *Plexams) draftFS(ctx context.Context) pdf.Maroto {
	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetPageMargins(10, 15, 10)

	m.RegisterFooter(func() {
		m.Row(20, func() {
			m.Col(12, func() {
				m.Text(fmt.Sprintf("Stand: %s Uhr, generiert mit https://github.com/obcode/plexams.go",
					time.Now().Format("02.01.06, 15:04")), props.Text{
					Top:   13,
					Style: consts.BoldItalic,
					Size:  8,
					Align: consts.Left,
				})
			})
		})
	})

	m.Row(6, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("Vorläufiger Planungsstand der Prüfungen der FK07 im %s", p.semesterFull()), props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Bold,
					Align: consts.Center,
				})
		})
	})
	m.Row(6, func() {
		m.Col(12, func() {
			m.Text(
				fmt.Sprintf("%s <%s>", p.planer.Name, p.planer.Email), props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})
	m.Row(15, func() {
		m.Col(12, func() {
			m.Text(
				"--- ENTWURF ---", props.Text{
					Top:   3,
					Size:  12,
					Style: consts.Normal,
					Align: consts.Center,
				})
		})
	})

	p.tableForProgram(ctx, "DA", "Master Data Analytics (DA)", m)
	p.tableForProgram(ctx, "DC", "Bachelor Data Science & Scientific Computing (DC)", m)
	p.tableForProgram(ctx, "IB", "Bachelor Wirtschaftsinformatik (IB)", m)
	p.tableForProgram(ctx, "IC", "Bachelor Scientific Computing (IC)", m)
	p.tableForProgram(ctx, "IF", "Bachelor Informatik (IF)", m)
	p.tableForProgram(ctx, "IG", "Master Informatik (IG)", m)
	p.tableForProgram(ctx, "IN", "Master Wirtschaftsinformatik (IN)", m)
	p.tableForProgram(ctx, "IS", "Master Stochastic Engineering in Business and Finance (IS)", m)
	p.tableForProgram(ctx, "IT", "Master IT-Sicherheit (IT)", m)
	p.tableForProgram(ctx, "WD", "Bachelor Wirtschaftsinformatik - Digitales Management (WD)", m)
	p.tableForProgram(ctx, "WT", "Bachelor Wirtschaftsinformatik - Informationstechnologie (WT)", m)

	return m
}
