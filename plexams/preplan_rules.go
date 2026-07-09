package plexams

import (
	"context"
	"fmt"

	"github.com/obcode/plexams.go/graph/model"
)

// PreplanConstraints returns the hard and soft rules the SEB/EXaHM pre-planning solver
// applies, as a human-readable German list for a read-only display in the GUI. It is built
// from the same constants the solver uses (preplan_solve.go / exahm_intervals.go), so the
// text stays in sync with the actual behaviour. See docs/algorithmen.md for the prose.
func (p *Plexams) PreplanConstraints(_ context.Context) ([]*model.PreplanRule, error) {
	hard := func(title, desc string) *model.PreplanRule {
		return &model.PreplanRule{Kind: model.PreplanRuleKindHard, Title: title, Description: desc}
	}
	soft := func(title, desc string) *model.PreplanRule {
		return &model.PreplanRule{Kind: model.PreplanRuleKindSoft, Title: title, Description: desc}
	}

	defBuf := int(exahmDefaultBuffer.Minutes())

	return []*model.PreplanRule{
		hard("Gleiche Zeit (sameSlot)",
			"Prüfungen, die per Constraint als sameSlot verknüpft sind, laufen zwingend im selben Slot – "+
				"sie werden zu einer untrennbaren Einheit zusammengefasst."),
		hard("Nur bestätigte Anny-Buchungen",
			"Als Kapazität zählen ausschließlich akzeptierte Anny-Buchungen der eigenen Personalisierung; "+
				"stornierte oder nur angefragte Buchungen werden ignoriert."),
		hard("Buchungsfenster deckt die Prüfung",
			fmt.Sprintf("EXaHM/SEB dürfen nur in Slots liegen, in denen gebuchte T-Bau-Räume das echte Prüfungsfenster "+
				"(Dauer + Vor-/Nachlauf, Standard je %d Min) mit genügend Plätzen abdecken. EXaHM braucht EXaHM-Räume; "+
				"SEB akzeptiert EXaHM- oder SEB-Räume.", defBuf)),
		hard("Kapazität pro Slot",
			"Die Summe der Plätze aller Prüfungen eines Slots darf die gebuchten Anny-Plätze nicht übersteigen. "+
				"Räume dürfen von mehreren Prüfungen geteilt werden (eine 10er-Prüfung blockiert keinen ganzen 30er-Raum)."),
		hard("Kapazität über die Zeit",
			"Zu jedem Zeitpunkt dürfen die gleichzeitig belegten EXaHM- (bzw. Gesamt-)Plätze inklusive Vor- und "+
				"Nachlauf die gebuchten Plätze nicht übersteigen – eine lange oder mit größerem Puffer überschriebene "+
				"Prüfung belegt ihre Räume bis in Nachbarslots hinein."),
		hard("MUC.DAI-Slots",
			"Prüfungen mit einem MUC.DAI-Programm (DE/GS/ID) dürfen nur in den dafür reservierten MUC.DAI-Slots liegen."),
		hard("Nicht gleichzeitig (notSameSlot)",
			"Explizit als „nicht gleichzeitig“ markierte Paare werden strikt getrennt gehalten."),

		soft("Studiengänge streuen",
			"Prüfungen desselben Studiengangs werden möglichst weit auseinandergelegt – bevorzugt auf verschiedene "+
				"Tage, sonst mit maximalem Abstand innerhalb eines Tages."),
		soft("Darf zusammen (canShareSlot)",
			"Explizit als „darf zusammen / direkt nacheinander“ markierte Paare sind von der Streuung ausgenommen."),
		soft("Wichtige Prüfungen zuerst",
			"EXaHM- und große SEB-Prüfungen werden nie zugunsten kleinerer weggelassen. Reicht die gebuchte Anny-"+
				"Kapazität nicht, weichen zuerst kleine, R-Bau-taugliche SEB-Prüfungen (mit quadratisch wachsenden "+
				"Kosten, d. h. lieber mehrere kleine als eine große oder gekoppelte Prüfung)."),
		soft("Puffer je Prüfung",
			fmt.Sprintf("Standard-Vor-/Nachlauf je %d Min, zwischen zwei eigenen aufeinanderfolgenden Prüfungen im "+
				"selben Raum geteilt; pro Prüfung über die Raum-Constraints überschreibbar (z. B. 60 Min).", defBuf)),
	}, nil
}
