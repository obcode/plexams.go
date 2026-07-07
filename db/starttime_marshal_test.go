package db_test

import (
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
)

// marshalGQLTime renders a time.Time exactly the way the generated GraphQL
// `Time` scalar does (graphql.MarshalTime -> time.RFC3339Nano). This is the
// string the frontend actually receives on the wire.
func marshalGQLTime(t time.Time) string {
	var b strings.Builder
	graphql.MarshalTime(t).MarshalGQL(&b)
	return b.String() // includes the surrounding quotes, e.g. "2026-02-02T08:30:00+01:00"
}

// TestStarttimeMarshalsWithBerlinOffset locks the wire-format contract the
// plexams.gui frontend depends on: served start times carry the Europe/Berlin
// UTC offset (+01:00 / +02:00), never a UTC "Z".
//
// The GUI derives slot numbers by reading HH:MM literally out of the ISO string
// and reusing the day's offset, and compares plannedStarttime === slot.starttime
// by exact string. Both are only correct if the backend emits Berlin-local
// times. This test guards that invariant independently of the process timezone
// (it loads Europe/Berlin explicitly rather than trusting time.Local, which is
// only pinned to Berlin in main.go, not under `go test`).
func TestStarttimeMarshalsWithBerlinOffset(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("load Europe/Berlin: %v", err)
	}

	cases := []struct {
		name       string
		st         time.Time
		wantOffset string
	}{
		{"winter slot (CET)", time.Date(2026, 2, 2, 8, 30, 0, 0, berlin), "+01:00"},
		{"summer slot (CEST)", time.Date(2026, 7, 6, 8, 30, 0, 0, berlin), "+02:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := marshalGQLTime(tc.st)
			if strings.Contains(got, "Z\"") {
				t.Fatalf("served start time is UTC %q; frontend slot derivation "+
					"and plannedStarttime===slot.starttime equality require a Berlin offset", got)
			}
			if !strings.Contains(got, tc.wantOffset) {
				t.Fatalf("served start time %q does not carry expected offset %s", got, tc.wantOffset)
			}
		})
	}
}

// TestStarttimeExactStringEqualityAcrossFields guards the frontend's exact-string
// matching: the /preplan <select> matches plannedStarttime against each slot's
// starttime byte-for-byte, and RoomRequest keys use the raw starttime string.
//
// A slot's starttime is built fresh in local time (semester_config.go), while a
// planned/room-request starttime round-trips through MongoDB and comes back as
// the same instant re-expressed in the local zone. This test simulates both
// paths for one slot and asserts they marshal to identical strings.
func TestStarttimeExactStringEqualityAcrossFields(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("load Europe/Berlin: %v", err)
	}

	// Slot start time as constructed from config.
	slot := time.Date(2026, 2, 2, 8, 30, 0, 0, berlin)
	// Same instant after a Mongo round-trip: BSON stores UTC, the driver decodes
	// it back into the local zone (UseLocalTimeZone: true in NewDB).
	roundTripped := slot.UTC().In(berlin)

	if a, b := marshalGQLTime(slot), marshalGQLTime(roundTripped); a != b {
		t.Fatalf("slot vs round-tripped starttime marshal differently: %q != %q; "+
			"frontend exact-string matching (/preplan select, room-request keys) would break", a, b)
	}
}

// TestUTCTimeMarshalsAsZ documents the failure mode this design avoids: a
// time.Time in UTC serializes with a trailing "Z", which the frontend would read
// as the wrong wall-clock time. If a serving path is ever changed to hand the
// GraphQL layer a UTC time directly (bypassing the config builder and the Mongo
// UseLocalTimeZone decode), that is the regression that breaks the GUI.
func TestUTCTimeMarshalsAsZ(t *testing.T) {
	utc := time.Date(2026, 2, 2, 8, 30, 0, 0, time.UTC)
	if got := marshalGQLTime(utc); !strings.HasSuffix(got, "Z\"") {
		t.Fatalf("expected UTC time to marshal with trailing Z, got %q", got)
	}
}
