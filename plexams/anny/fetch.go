package anny

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/zpaimport"
	"github.com/rs/zerolog/log"
)

type booking struct {
	Number               string          `json:"number"`
	StartDate            time.Time       `json:"start_date"`
	EndDate              time.Time       `json:"end_date"`
	BlockerStartDate     time.Time       `json:"blocker_start_date"`
	BlockerEndDate       time.Time       `json:"blocker_end_date"`
	ChargedDuration      int             `json:"charged_duration"`
	Description          string          `json:"description"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
	CanceledAt           *time.Time      `json:"canceled_at,omitempty"`
	Status               string          `json:"status"`
	StatusReason         json.RawMessage `json:"status_reason,omitempty"`
	IsBlocker            bool            `json:"is_blocker"`
	CanEdit              bool            `json:"can_edit"`
	IsEditable           bool            `json:"is_editable"`
	ManuallyCreated      bool            `json:"manually_created"`
	Note                 string          `json:"note"`
	Room                 string          `json:"room,omitempty"`
	Self                 string          `json:"self"`
	PersonalizationName  string          `json:"personalization_name"`
	BookingGroupID       string          `json:"booking_group_identifier,omitempty"`
	CancelableUntil      *time.Time      `json:"cancelable_until,omitempty"`
	HasCustomDescription bool            `json:"has_custom_description"`
	ResourceID           string          `json:"-"`
}

type bookingRaw struct {
	Attributes struct {
		Number           string          `json:"number"`
		StartDate        string          `json:"start_date"`
		EndDate          string          `json:"end_date"`
		BlockerStartDate string          `json:"blocker_start_date"`
		BlockerEndDate   string          `json:"blocker_end_date"`
		ChargedDuration  int             `json:"charged_duration"`
		Description      string          `json:"description"`
		CreatedAt        string          `json:"created_at"`
		UpdatedAt        string          `json:"updated_at"`
		CanceledAt       string          `json:"canceled_at"`
		Status           string          `json:"status"`
		StatusReason     json.RawMessage `json:"status_reason"`
		IsBlocker        bool            `json:"is_blocker"`
		CanEdit          bool            `json:"can_edit"`
		IsEditable       bool            `json:"is_editable"`
		ManuallyCreated  bool            `json:"manually_created"`
		Note             string          `json:"note"`
	} `json:"attributes"`
	Links struct {
		Self string `json:"self"`
	} `json:"links"`
	Meta struct {
		PersonalizationName  string `json:"personalization_name"`
		BookingGroupID       string `json:"booking_group_identifier"`
		CancelableUntil      string `json:"cancelable_until"`
		HasCustomDescription bool   `json:"has_custom_description"`
	} `json:"meta"`
}

type bookingsPage struct {
	Data  []bookingRaw `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// DiffWindow restricts the change report to bookings that start within [From, Until].
// A zero From falls back to "from now on"; a zero Until means no upper bound. The store
// itself is never restricted — only which bookings the added/changed/removed report
// covers (the nightly auto-sync passes the exam period so off-period bookings are noise).
type DiffWindow struct {
	From  time.Time
	Until time.Time
}

// Fetch fetches the bookings from the Anny API and stores them in the database. All
// bookings (all rooms, all people) are kept; "ours" is flagged via the personalization
// names at query time, not filtered away here. window restricts the change report (not
// the store) to the exam period.
func (s *Service) Fetch(ctx context.Context, reporter Reporter, window DiffWindow) error {
	reporter.Step("fetching bookings from anny.eu")
	personalizationNames := s.PersonalizationNames(ctx)

	token := strings.TrimSpace(s.cfg.Token)
	if token == "" {
		return fmt.Errorf("anny token is empty")
	}

	authToken := token
	if !strings.HasPrefix(strings.ToLower(authToken), "bearer ") {
		authToken = "Bearer " + authToken
	}

	endpoint := s.cfg.URL
	if endpoint == "" {
		endpoint = "https://b.anny.eu/api/v1/bookings"
	}
	query := url.Values{}
	query.Set("sort", "start_date")
	query.Set("page[size]", "100")
	query.Set("filter[upcoming_only]", "1")

	nextURL := endpoint + "?" + query.Encode()
	allBookings := make([]booking, 0)
	client := &http.Client{Timeout: 20 * time.Second}

	for nextURL != "" {
		req, err := http.NewRequest(http.MethodGet, nextURL, nil)
		if err != nil {
			return fmt.Errorf("cannot build anny request: %w", err)
		}

		req.Header.Set("Accept", "application/vnd.api+json")
		req.Header.Set("Authorization", authToken)

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("cannot fetch anny bookings: %w", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("cannot read anny response body: %w", readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("cannot close anny response body: %w", closeErr)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("anny request failed with status %s: %s", resp.Status, string(body))
		}

		var page bookingsPage
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("cannot decode anny response: %w", err)
		}

		for _, raw := range page.Data {
			b, err := rawToBooking(raw)
			if err != nil {
				return err
			}
			allBookings = append(allBookings, b)
		}
		nextURL = page.Links.Next
	}

	resourceIDToRoom := make(map[string]string)
	for _, b := range allBookings {
		if b.ResourceID == "" || b.Room == "" {
			continue
		}
		resourceIDToRoom[b.ResourceID] = b.Room
	}

	bookings := make([]booking, 0, len(allBookings))
	for _, b := range allBookings {
		if b.Room == "" {
			if inferredRoom, ok := resourceIDToRoom[b.ResourceID]; ok {
				b.Room = inferredRoom
			}
		}
		bookings = append(bookings, b)
	}

	sort.Slice(bookings, func(i, j int) bool {
		return bookings[i].StartDate.Before(bookings[j].StartDate)
	})

	if s.db == nil {
		return fmt.Errorf("no database configured for saving anny bookings")
	}

	dbBookings := make([]*model.AnnyBooking, 0, len(bookings))
	for _, b := range bookings {
		dbBookings = append(dbBookings, bookingToDBBooking(b))
	}

	// Snapshot the current bookings before the save overwrites them, so we can report
	// what actually changed (added/changed/removed) like the ZPA imports do — the
	// nightly auto-sync mails this diff.
	oldBookings, err := s.db.AllAnnyBookings(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot read current anny bookings for diff")
		oldBookings = nil
	}

	if err := s.db.SaveAnnyBookings(ctx, dbBookings); err != nil {
		return fmt.Errorf("cannot save anny bookings: %w", err)
	}

	summary(bookings, personalizationNames, reporter)
	reporter.StopProgress(fmt.Sprintf("saved %d anny bookings to the database", len(dbBookings)))

	rec := diffBookings(oldBookings, dbBookings, window, time.Now(), personalizationNames)
	for _, m := range rec.msgs {
		reporter.Println(m)
	}
	rec.entry.Operation = "anny-import-bookings"
	rec.entry.Label = "Buchungen aus Anny importiert"
	rec.entry.Direction = "import"
	rec.entry.System = "Anny"
	rec.entry.OK = true
	rec.entry.Summary = fmt.Sprintf("%d Buchungen gespeichert (%d neu, %d geändert, %d entfallen)",
		len(dbBookings), rec.entry.Added, rec.entry.Changed, rec.entry.Removed)
	s.logSync(ctx, rec.entry)

	return nil
}

// bookingDiff bundles the diff SyncLogEntry with the human-readable report lines.
type bookingDiff struct {
	entry *model.SyncLogEntry
	msgs  []string
}

// diffBookings compares the freshly fetched bookings against the previous DB state,
// keyed by the Anny booking number. Both sides are first restricted to the exam period
// (window): the Anny API is fetched with filter[upcoming_only]=1 and covers all rooms /
// all people, so a stored booking that merely aged out or that lies outside the period
// would otherwise show up as spurious noise — only genuine cancellations/changes within
// the period should be reported. Each entry is marked "[eigene]" / "[fremd]" and carries
// the booker so it is clear who booked (personalizationNames are our own names).
func diffBookings(old, neu []*model.AnnyBooking, window DiffWindow, now time.Time, personalizationNames []string) bookingDiff {
	keep := windowFilter(window, now)
	nameFn := func(b *model.AnnyBooking) string {
		return bookingName(b, MatchesAnyPersonalization(b.PersonalizationName, personalizationNames))
	}
	fieldsFn := func(b *model.AnnyBooking) map[string]string {
		mine := "nein"
		if MatchesAnyPersonalization(b.PersonalizationName, personalizationNames) {
			mine = "ja"
		}
		return map[string]string{
			"room":       b.Room,
			"start":      b.StartDate.Format("02.01.2006 15:04"),
			"end":        b.EndDate.Format("02.01.2006 15:04"),
			"duration":   fmt.Sprint(b.ChargedDuration),
			"status":     b.Status,
			"gebuchtVon": strings.TrimSpace(b.PersonalizationName),
			"eigene":     mine,
			"note":       b.Note,
		}
	}
	entry, msgs := zpaimport.DiffChanges(filterBookings(old, keep), filterBookings(neu, keep),
		func(b *model.AnnyBooking) string { return b.Number }, nameFn, fieldsFn)
	return bookingDiff{entry: entry, msgs: msgs}
}

// windowFilter returns a predicate that keeps bookings starting within the exam period.
// A zero window.From falls back to now (no past bookings); a zero window.Until means no
// upper bound. The upper bound is inclusive of the whole Until day.
func windowFilter(window DiffWindow, now time.Time) func(*model.AnnyBooking) bool {
	lower := window.From
	if lower.IsZero() {
		lower = now
	}
	hasUpper := !window.Until.IsZero()
	upper := window.Until.AddDate(0, 0, 1) // start of the day after Until → whole Until day counts
	return func(b *model.AnnyBooking) bool {
		if b == nil || b.StartDate.Before(lower) {
			return false
		}
		if hasUpper && !b.StartDate.Before(upper) {
			return false
		}
		return true
	}
}

// filterBookings returns the bookings for which keep reports true.
func filterBookings(bookings []*model.AnnyBooking, keep func(*model.AnnyBooking) bool) []*model.AnnyBooking {
	out := make([]*model.AnnyBooking, 0, len(bookings))
	for _, b := range bookings {
		if keep(b) {
			out = append(out, b)
		}
	}
	return out
}

// bookingName is the human-readable label of a booking used in the diff report. It leads
// with "[eigene]" / "[fremd]" and names the booker so mine-vs-others is obvious.
func bookingName(b *model.AnnyBooking, mine bool) string {
	room := b.Room
	if room == "" {
		room = "(unbekannt)"
	}
	who := strings.TrimSpace(b.PersonalizationName)
	if who == "" {
		who = "unbekannt"
	}
	owner := "fremd"
	if mine {
		owner = "eigene"
	}
	return fmt.Sprintf("[%s] %s %s–%s – %s [%s]", owner, room,
		b.StartDate.Format("02.01.2006 15:04"), b.EndDate.Format("15:04"), who, b.Number)
}

// logSync records a sync-log entry (best effort; failure is only logged).
func (s *Service) logSync(ctx context.Context, entry *model.SyncLogEntry) {
	entry.Time = time.Now()
	if err := s.db.AddSyncLogEntry(ctx, entry); err != nil {
		log.Error().Err(err).Str("operation", entry.Operation).Msg("cannot write sync-log entry")
	}
}

func summary(bookings []booking, personalizationNames []string, reporter Reporter) {
	roomMap := make(map[string]int)
	for _, b := range bookings {
		room := b.Room
		if room == "" {
			room = "(unknown)"
		}
		roomMap[room]++
	}

	roomNames := make([]string, 0, len(roomMap))
	for roomName := range roomMap {
		roomNames = append(roomNames, roomName)
	}
	sort.Strings(roomNames)

	who := "alle"
	if len(personalizationNames) > 0 {
		who = strings.Join(personalizationNames, ", ")
	}
	reporter.Println(fmt.Sprintf("Anny-Buchungen für %s (gesamt %d):", who, len(bookings)))
	for _, roomName := range roomNames {
		reporter.Println(fmt.Sprintf("  %-10s %d", roomName, roomMap[roomName]))
	}
	for _, b := range bookings {
		room := b.Room
		if room == "" {
			room = "(unknown)"
		}
		desc := b.Description
		if len([]rune(desc)) > 36 {
			desc = string([]rune(desc)[:36]) + "..."
		}
		reporter.Println(fmt.Sprintf("  %s %s-%s  %-10s %3d min  %s",
			b.StartDate.Format("02.01.2006"),
			b.StartDate.Format("15:04"),
			b.EndDate.Format("15:04"),
			room,
			b.ChargedDuration,
			desc,
		))
	}
}

func rawToBooking(raw bookingRaw) (booking, error) {
	startDate, err := parseRFC3339Local(raw.Attributes.StartDate)
	if err != nil {
		return booking{}, fmt.Errorf("cannot parse start_date %q: %w", raw.Attributes.StartDate, err)
	}

	endDate, err := parseRFC3339Local(raw.Attributes.EndDate)
	if err != nil {
		return booking{}, fmt.Errorf("cannot parse end_date %q: %w", raw.Attributes.EndDate, err)
	}

	blockerStartDate, err := parseRFC3339Local(raw.Attributes.BlockerStartDate)
	if err != nil {
		return booking{}, fmt.Errorf("cannot parse blocker_start_date %q: %w", raw.Attributes.BlockerStartDate, err)
	}

	blockerEndDate, err := parseRFC3339Local(raw.Attributes.BlockerEndDate)
	if err != nil {
		return booking{}, fmt.Errorf("cannot parse blocker_end_date %q: %w", raw.Attributes.BlockerEndDate, err)
	}

	createdAt, err := parseRFC3339Local(raw.Attributes.CreatedAt)
	if err != nil {
		return booking{}, fmt.Errorf("cannot parse created_at %q: %w", raw.Attributes.CreatedAt, err)
	}

	updatedAt, err := parseRFC3339Local(raw.Attributes.UpdatedAt)
	if err != nil {
		return booking{}, fmt.Errorf("cannot parse updated_at %q: %w", raw.Attributes.UpdatedAt, err)
	}

	canceledAt, err := parseRFC3339LocalOptional(raw.Attributes.CanceledAt)
	if err != nil {
		return booking{}, fmt.Errorf("cannot parse canceled_at %q: %w", raw.Attributes.CanceledAt, err)
	}

	cancelableUntil, err := parseRFC3339LocalOptional(raw.Meta.CancelableUntil)
	if err != nil {
		return booking{}, fmt.Errorf("cannot parse cancelable_until %q: %w", raw.Meta.CancelableUntil, err)
	}

	return booking{
		Number:               raw.Attributes.Number,
		StartDate:            startDate,
		EndDate:              endDate,
		BlockerStartDate:     blockerStartDate,
		BlockerEndDate:       blockerEndDate,
		ChargedDuration:      raw.Attributes.ChargedDuration,
		Description:          raw.Attributes.Description,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
		CanceledAt:           canceledAt,
		Status:               raw.Attributes.Status,
		StatusReason:         raw.Attributes.StatusReason,
		IsBlocker:            raw.Attributes.IsBlocker,
		CanEdit:              raw.Attributes.CanEdit,
		IsEditable:           raw.Attributes.IsEditable,
		ManuallyCreated:      raw.Attributes.ManuallyCreated,
		Note:                 raw.Attributes.Note,
		Room:                 extractRoomFromNote(raw.Attributes.Note),
		Self:                 raw.Links.Self,
		PersonalizationName:  raw.Meta.PersonalizationName,
		BookingGroupID:       raw.Meta.BookingGroupID,
		CancelableUntil:      cancelableUntil,
		HasCustomDescription: raw.Meta.HasCustomDescription,
		ResourceID:           extractResourceID(raw.Meta.BookingGroupID),
	}, nil
}

var roomFromNotePattern = regexp.MustCompile(`(?i)ressource:\s*([A-Z])\s*(\d\.[0-9]{3})`)
var resourceIDPattern = regexp.MustCompile(`resource:(\d+)`)

func extractRoomFromNote(note string) string {
	matches := roomFromNotePattern.FindStringSubmatch(note)
	if len(matches) != 3 {
		return ""
	}
	room := strings.ToUpper(matches[1]) + matches[2]
	return normalizeRoomName(room)
}

func extractResourceID(bookingGroupIdentifier string) string {
	matches := resourceIDPattern.FindStringSubmatch(bookingGroupIdentifier)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func bookingToDBBooking(b booking) *model.AnnyBooking {
	return &model.AnnyBooking{
		Number:                 b.Number,
		StartDate:              b.StartDate,
		EndDate:                b.EndDate,
		BlockerStartDate:       b.BlockerStartDate,
		BlockerEndDate:         b.BlockerEndDate,
		ChargedDuration:        b.ChargedDuration,
		Description:            b.Description,
		CreatedAt:              b.CreatedAt,
		UpdatedAt:              b.UpdatedAt,
		CanceledAt:             b.CanceledAt,
		Status:                 b.Status,
		StatusReason:           b.StatusReason,
		IsBlocker:              b.IsBlocker,
		CanEdit:                b.CanEdit,
		IsEditable:             b.IsEditable,
		ManuallyCreated:        b.ManuallyCreated,
		Note:                   b.Note,
		Room:                   b.Room,
		Self:                   b.Self,
		PersonalizationName:    b.PersonalizationName,
		BookingGroupIdentifier: b.BookingGroupID,
		CancelableUntil:        b.CancelableUntil,
		HasCustomDescription:   b.HasCustomDescription,
		ResourceID:             b.ResourceID,
	}
}

func parseRFC3339Local(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return t.Local(), nil
}

func parseRFC3339LocalOptional(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	t, err := parseRFC3339Local(value)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
