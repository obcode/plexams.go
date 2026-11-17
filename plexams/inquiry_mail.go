package plexams

import (
	"context"
	"fmt"
	"strings"

	"github.com/obcode/plexams.go/plexams/inquiry"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// SendPortalActivityMail sends one planner the portal changes (inquiry.Mailer). A
// system mail: "Plexams <noreply>", independent of the active semester.
func (p *Plexams) SendPortalActivityMail(_ context.Context, to string, items []*inquiry.Activity, daily bool) error {
	view := newPortalActivityView(items, daily, publicURL())
	subject := fmt.Sprintf("[Plexams] Portal: %d %s", view.Count, plural(view.Count, "neue Eingabe", "neue Eingaben"))
	if daily {
		subject = fmt.Sprintf("[Plexams] Portal: %d %s des Tages", view.Count, plural(view.Count, "Eingabe", "Eingaben"))
	}
	text, html, err := p.mailRenderer().Render("portalActivityEmail.md.tmpl", false, view)
	if err != nil {
		log.Error().Err(err).Msg("portal activity mail: cannot render")
		return err
	}
	return p.sendSystemMail(true, []string{to}, subject, text, html)
}

// publicURL is where this installation is reachable, for links in mails. Empty when
// not configured.
func publicURL() string {
	return strings.TrimRight(strings.TrimSpace(viper.GetString("server.publicurl")), "/")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

type portalActivityView struct {
	Daily       bool
	Count       int
	Semesters   []portalActivitySemesterView
	OverviewURL string
}

type portalActivitySemesterView struct {
	Semester string
	Items    []portalActivityItemView
}

type portalActivityItemView struct {
	Time    string
	Kind    string
	Teacher string
	Actor   string
	Token   bool
	Action  string
	Label   string
	Hidden  bool
	Changes []portalActivityChangeView
}

type portalActivityChangeView struct {
	Field string
	Old   string
	New   string
}

var inquiryKindLabel = map[inquiry.Kind]string{
	inquiry.KindPreplan:       "EXaHM/SEB",
	inquiry.KindExamsToPlan:   "Zu planende Prüfungen",
	inquiry.KindConstraints:   "Besonderheiten",
	inquiry.KindDraftFeedback: "Rückmeldung zum Entwurf",
	inquiry.KindInvigilation:  "Aufsichten",
}

var activityActionLabel = map[string]string{
	inquiry.ActionSubmitted:       "neu gemeldet",
	inquiry.ActionChanged:         "geändert",
	inquiry.ActionWithdrawn:       "zurückgezogen",
	inquiry.ActionNothingToReport: "nichts zu melden",
	inquiry.ActionRevoked:         "„nichts zu melden“ zurückgenommen",
}

var activityFieldLabel = map[string]string{
	"examKind":         "Art",
	"module":           "Modul",
	"programs":         "Studiengänge",
	"expectedStudents": "erwartete Teilnehmende",
	"durationMin":      "Dauer (min)",
	"remark":           "Bemerkung",
	"preplan":          "Vorplanung",
	"nothingToReport":  "nichts zu melden",
}

func labelOr(m map[string]string, key string) string {
	if l, ok := m[key]; ok {
		return l
	}
	return key
}

// newPortalActivityView groups the changes by semester, newest semester first, each in
// the order they happened.
func newPortalActivityView(items []*inquiry.Activity, daily bool, base string) portalActivityView {
	view := portalActivityView{Daily: daily, Count: len(items)}
	if base != "" {
		view.OverviewURL = base + "/inquiries/activity"
	}
	index := map[string]int{}
	for _, a := range items {
		i, ok := index[a.Semester]
		if !ok {
			i = len(view.Semesters)
			index[a.Semester] = i
			view.Semesters = append(view.Semesters, portalActivitySemesterView{Semester: a.Semester})
		}
		teacher := a.TeacherName
		if teacher == "" {
			teacher = fmt.Sprintf("ZPA-Person %d", a.TeacherID)
		}
		kind := inquiryKindLabel[a.Kind]
		if kind == "" {
			kind = string(a.Kind)
		}
		item := portalActivityItemView{
			Time: a.At.Format("02.01. 15:04"), Kind: kind, Teacher: teacher, Actor: a.Actor,
			Token: a.Channel == "TOKEN", Action: labelOr(activityActionLabel, a.Action), Label: a.Label,
			// Invigilation details (part-time, sabbatical) stay out of mail.
			Hidden: a.Kind == inquiry.KindInvigilation,
		}
		if !item.Hidden {
			for _, c := range a.Changes {
				item.Changes = append(item.Changes, portalActivityChangeView{Field: labelOr(activityFieldLabel, c.Field), Old: c.Old, New: c.New})
			}
		}
		view.Semesters[i].Items = append(view.Semesters[i].Items, item)
	}
	// Newest semester first: semester ids sort as text.
	for i := 1; i < len(view.Semesters); i++ {
		for j := i; j > 0 && view.Semesters[j].Semester > view.Semesters[j-1].Semester; j-- {
			view.Semesters[j], view.Semesters[j-1] = view.Semesters[j-1], view.Semesters[j]
		}
	}
	return view
}
