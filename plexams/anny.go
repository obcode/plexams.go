package plexams

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/spf13/viper"
)

type AnnyBooking struct {
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

type annyBookingRaw struct {
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

type annyBookingsPage struct {
	Data  []annyBookingRaw `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

func (p *Plexams) FetchFromAnny() error {
	token := viper.GetString("anny.token")
	personalizationName := strings.TrimSpace(viper.GetString("anny.personalization_name"))
	configRooms := viper.GetStringSlice("anny.rooms")
	allowedRooms := make(map[string]struct{}, len(configRooms))
	for _, room := range configRooms {
		normalizedRoom := normalizeRoomName(room)
		if normalizedRoom == "" {
			continue
		}
		allowedRooms[normalizedRoom] = struct{}{}
	}

	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("anny token is empty")
	}

	authToken := strings.TrimSpace(token)
	if !strings.HasPrefix(strings.ToLower(authToken), "bearer ") {
		authToken = "Bearer " + authToken
	}

	endpoint := "https://b.anny.eu/api/v1/bookings"
	query := url.Values{}
	query.Set("sort", "start_date")
	query.Set("page[size]", "100")
	query.Set("filter[upcoming_only]", "1")

	nextURL := endpoint + "?" + query.Encode()
	allBookings := make([]AnnyBooking, 0)
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

		var page annyBookingsPage
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("cannot decode anny response: %w", err)
		}

		for _, raw := range page.Data {
			booking, err := annyRawToBooking(raw)
			if err != nil {
				return err
			}
			allBookings = append(allBookings, booking)
		}
		nextURL = page.Links.Next
	}

	resourceIDToRoom := make(map[string]string)
	for _, booking := range allBookings {
		if booking.ResourceID == "" || booking.Room == "" {
			continue
		}
		resourceIDToRoom[booking.ResourceID] = booking.Room
	}

	if len(allowedRooms) > 0 {
		remainingRooms := make([]string, 0, len(allowedRooms))
		for room := range allowedRooms {
			isMapped := false
			for _, mappedRoom := range resourceIDToRoom {
				if mappedRoom == room {
					isMapped = true
					break
				}
			}
			if !isMapped {
				remainingRooms = append(remainingRooms, room)
			}
		}

		unknownResourceIDs := make(map[string]struct{})
		for _, booking := range allBookings {
			if booking.ResourceID == "" || booking.Room != "" {
				continue
			}
			if _, known := resourceIDToRoom[booking.ResourceID]; known {
				continue
			}
			unknownResourceIDs[booking.ResourceID] = struct{}{}
		}

		if len(remainingRooms) == 1 && len(unknownResourceIDs) == 1 {
			for resourceID := range unknownResourceIDs {
				resourceIDToRoom[resourceID] = remainingRooms[0]
			}
		}
	}

	bookings := make([]AnnyBooking, 0, len(allBookings))
	for _, booking := range allBookings {
		if booking.Room == "" {
			if inferredRoom, ok := resourceIDToRoom[booking.ResourceID]; ok {
				booking.Room = inferredRoom
			}
		}

		if personalizationName != "" && !strings.EqualFold(strings.TrimSpace(booking.PersonalizationName), personalizationName) {
			continue
		}
		if len(allowedRooms) > 0 {
			if _, ok := allowedRooms[normalizeRoomName(booking.Room)]; !ok {
				continue
			}
		}
		bookings = append(bookings, booking)
	}

	sort.Slice(bookings, func(i, j int) bool {
		return bookings[i].StartDate.Before(bookings[j].StartDate)
	})

	if p.dbClient == nil {
		return fmt.Errorf("no database configured for saving anny bookings")
	}

	dbBookings := make([]*model.AnnyBooking, 0, len(bookings))
	for _, booking := range bookings {
		dbBookings = append(dbBookings, annyBookingToDBBooking(booking))
	}

	if err := p.dbClient.SaveAnnyBookings(context.Background(), dbBookings); err != nil {
		return fmt.Errorf("cannot save anny bookings: %w", err)
	}

	printAnnySummary(bookings, personalizationName)
	fmt.Println("Saved to MongoDB collection anny_bookings")

	return nil
}

func printAnnySummary(bookings []AnnyBooking, personalizationName string) {
	roomMap := make(map[string]int)
	for _, booking := range bookings {
		room := booking.Room
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

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Anny-Buchungen für\t%s\t(gesamt %d)\n", personalizationName, len(bookings)) // nolint
	fmt.Fprintln(w, "Raum\tAnzahl")                                                             // nolint                                                // nolint
	for _, roomName := range roomNames {
		fmt.Fprintf(w, "%s\t%d\n", roomName, roomMap[roomName]) // nolint
	}

	maxRows := len(bookings)

	fmt.Fprintln(w, "")                                       // nolint
	fmt.Fprintln(w, "Alle Termine")                           // nolint
	fmt.Fprintln(w, "Datum\tZeit\tRaum\tDauer\tBeschreibung") // nolint
	for i := 0; i < maxRows; i++ {
		booking := bookings[i]
		room := booking.Room
		if room == "" {
			room = "(unknown)"
		}
		desc := booking.Description
		if len([]rune(desc)) > 36 {
			desc = string([]rune(desc)[:36]) + "..."
		}
		fmt.Fprintf(w, "%s\t%s-%s\t%s\t%d min\t%s\n", // nolint
			booking.StartDate.Format("02.01.2006"),
			booking.StartDate.Format("15:04"),
			booking.EndDate.Format("15:04"),
			room,
			booking.ChargedDuration,
			desc,
		)
	}
	_ = w.Flush()
}

func annyRawToBooking(raw annyBookingRaw) (AnnyBooking, error) {
	startDate, err := parseRFC3339Local(raw.Attributes.StartDate)
	if err != nil {
		return AnnyBooking{}, fmt.Errorf("cannot parse start_date %q: %w", raw.Attributes.StartDate, err)
	}

	endDate, err := parseRFC3339Local(raw.Attributes.EndDate)
	if err != nil {
		return AnnyBooking{}, fmt.Errorf("cannot parse end_date %q: %w", raw.Attributes.EndDate, err)
	}

	blockerStartDate, err := parseRFC3339Local(raw.Attributes.BlockerStartDate)
	if err != nil {
		return AnnyBooking{}, fmt.Errorf("cannot parse blocker_start_date %q: %w", raw.Attributes.BlockerStartDate, err)
	}

	blockerEndDate, err := parseRFC3339Local(raw.Attributes.BlockerEndDate)
	if err != nil {
		return AnnyBooking{}, fmt.Errorf("cannot parse blocker_end_date %q: %w", raw.Attributes.BlockerEndDate, err)
	}

	createdAt, err := parseRFC3339Local(raw.Attributes.CreatedAt)
	if err != nil {
		return AnnyBooking{}, fmt.Errorf("cannot parse created_at %q: %w", raw.Attributes.CreatedAt, err)
	}

	updatedAt, err := parseRFC3339Local(raw.Attributes.UpdatedAt)
	if err != nil {
		return AnnyBooking{}, fmt.Errorf("cannot parse updated_at %q: %w", raw.Attributes.UpdatedAt, err)
	}

	canceledAt, err := parseRFC3339LocalOptional(raw.Attributes.CanceledAt)
	if err != nil {
		return AnnyBooking{}, fmt.Errorf("cannot parse canceled_at %q: %w", raw.Attributes.CanceledAt, err)
	}

	cancelableUntil, err := parseRFC3339LocalOptional(raw.Meta.CancelableUntil)
	if err != nil {
		return AnnyBooking{}, fmt.Errorf("cannot parse cancelable_until %q: %w", raw.Meta.CancelableUntil, err)
	}

	return AnnyBooking{
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

func normalizeRoomName(room string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(room), " ", ""))
}

func extractResourceID(bookingGroupIdentifier string) string {
	matches := resourceIDPattern.FindStringSubmatch(bookingGroupIdentifier)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func annyBookingToDBBooking(booking AnnyBooking) *model.AnnyBooking {
	return &model.AnnyBooking{
		Number:                 booking.Number,
		StartDate:              booking.StartDate,
		EndDate:                booking.EndDate,
		BlockerStartDate:       booking.BlockerStartDate,
		BlockerEndDate:         booking.BlockerEndDate,
		ChargedDuration:        booking.ChargedDuration,
		Description:            booking.Description,
		CreatedAt:              booking.CreatedAt,
		UpdatedAt:              booking.UpdatedAt,
		CanceledAt:             booking.CanceledAt,
		Status:                 booking.Status,
		StatusReason:           booking.StatusReason,
		IsBlocker:              booking.IsBlocker,
		CanEdit:                booking.CanEdit,
		IsEditable:             booking.IsEditable,
		ManuallyCreated:        booking.ManuallyCreated,
		Note:                   booking.Note,
		Room:                   booking.Room,
		Self:                   booking.Self,
		PersonalizationName:    booking.PersonalizationName,
		BookingGroupIdentifier: booking.BookingGroupID,
		CancelableUntil:        booking.CancelableUntil,
		HasCustomDescription:   booking.HasCustomDescription,
		ResourceID:             booking.ResourceID,
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
