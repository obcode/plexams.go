package plexams

// ConstraintsEmail is the shared template data for the period/feedback emails (draft,
// invigilations, published). The standalone constraints-request email was replaced by
// the consolidated exam-planning info email (see email_exam_planning.go).
type ConstraintsEmail struct {
	FromDate     string
	UntilDate    string
	FeedbackDate string
	PlanerName   string
}
