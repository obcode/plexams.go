package db

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// SlotResolver converts between an absolute start time and our slot grid. It is
// provided by the plexams layer (which owns the semester config) so the db layer can
// derive a plan entry's DayNumber/SlotNumber from its persisted Starttime on read,
// and translate a (day, slot) filter into the matching start time.
type SlotResolver interface {
	// SlotForTime returns the (dayNumber, slotNumber) of the slot the given time
	// falls on, or (0, 0) when it matches no slot (e.g. outside the exam period).
	SlotForTime(t time.Time) (day, slot int)
	// TimeForSlot returns the start time of the given (dayNumber, slotNumber), and
	// false when there is no such slot.
	TimeForSlot(day, slot int) (time.Time, bool)
}

type DB struct {
	Client       *mongo.Client
	uri          string
	semester     string
	databaseName string
	// slotResolver derives plan-entry day/slot numbers from the persisted Starttime
	// and vice versa; set by the plexams layer after the semester config is loaded.
	slotResolver SlotResolver
	// todosMu serializes the drop+insert in CacheInvigilatorTodos so concurrent
	// callers (e.g. parallel validation subscriptions) cannot interleave their
	// drops and inserts and leave more than one todos document behind.
	todosMu sync.Mutex
}

// SetSlotResolver installs the slot resolver used to derive plan-entry day/slot
// numbers from their persisted start time. Called after the semester config is
// (re-)derived.
func (db *DB) SetSlotResolver(r SlotResolver) {
	db.slotResolver = r
}

// decoratePlanEntry fills the derived DayNumber/SlotNumber from the persisted
// Starttime using the slot resolver (no-op when unset or unplanned).
func (db *DB) decoratePlanEntry(pe *model.PlanEntry) {
	if pe == nil || pe.Starttime == nil || db.slotResolver == nil {
		return
	}
	pe.DayNumber, pe.SlotNumber = db.slotResolver.SlotForTime(*pe.Starttime)
}

// decoratePlannedRoom fills the derived Day/Slot from the persisted Starttime.
func (db *DB) decoratePlannedRoom(pr *model.PlannedRoom) {
	if pr == nil || pr.Starttime == nil || db.slotResolver == nil {
		return
	}
	pr.Day, pr.Slot = db.slotResolver.SlotForTime(*pr.Starttime)
}

// decorateUnplacedExam fills the derived Day/Slot from the persisted Starttime.
func (db *DB) decorateUnplacedExam(ue *model.UnplacedExam) {
	if ue == nil || ue.Starttime == nil || db.slotResolver == nil {
		return
	}
	ue.Day, ue.Slot = db.slotResolver.SlotForTime(*ue.Starttime)
}

// decorateBlockedRoom fills the derived Day/Slot from the persisted Starttime.
func (db *DB) decorateBlockedRoom(br *model.BlockedRoom) {
	if br == nil || br.Starttime == nil || db.slotResolver == nil {
		return
	}
	br.Day, br.Slot = db.slotResolver.SlotForTime(*br.Starttime)
}

func NewDB(uri, semester string, dbName *string) (*DB, error) {
	// MongoDB stores all datetimes as UTC. Decode them back into the local
	// timezone (Europe/Berlin, set in main.go via time.Local) so that the rest
	// of plexams.go works with local time everywhere, matching the local times
	// given in the semester YAML config.
	client, err := mongo.Connect(context.Background(),
		options.Client().
			ApplyURI(uri).
			SetBSONOptions(&options.BSONOptions{UseLocalTimeZone: true}))
	if err != nil {
		return nil, err
	}
	err = client.Ping(context.Background(), readpref.Primary())
	if err != nil {
		return nil, err
	}

	databaseName := strings.Replace(semester, " ", "-", 1)
	if dbName != nil {
		databaseName = *dbName
	}

	log.Debug().Str("database name", databaseName).Msg("using database")

	return &DB{
		Client:       client,
		uri:          uri,
		semester:     semester,
		databaseName: databaseName,
	}, nil
}

func semesterName(semester string) string {
	return strings.Replace(semester, "-", " ", 1)
}

// MongoHost returns the host:port the client is connected to, with any
// credentials (user:pass@) and path/query stripped, so it is safe to display.
func (db *DB) MongoHost() string {
	s := db.uri
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if at := strings.LastIndex(s, "@"); at >= 0 {
		s = s[at+1:]
	}
	if i := strings.IndexAny(s, "/?"); i >= 0 {
		s = s[:i]
	}
	return s
}
