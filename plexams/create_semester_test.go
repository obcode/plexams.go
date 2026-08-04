package plexams

import (
	"context"
	"testing"
	"time"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

func minimalConfigInput() *model.SemesterConfigInput {
	return &model.SemesterConfigInput{
		From:       time.Date(2027, 1, 25, 0, 0, 0, 0, time.Local),
		Until:      time.Date(2027, 2, 12, 0, 0, 0, 0, time.Local),
		StartTimes: []string{"08:30", "10:30"},
	}
}

// TestCreateSemesterRegistersTheSemesterFirst is the regression test for an
// ordering that only PostgreSQL could reject: semester_config_input references
// semester(id), so writing the config before registering the semester fails with
// a foreign key violation.
//
// Under MongoDB the insert created the database as a side effect, so the order did
// not matter and nothing here was ever exercised against an empty database --
// which is why creating the very first semester was broken without anyone noticing.
func TestCreateSemesterRegistersTheSemesterFirst(t *testing.T) {
	// deliberately NOT NewDBWithSemester: the semester must not exist yet
	pg := pgtest.NewDB(t)
	p := &Plexams{dbClient: pg}
	ctx := context.Background()

	res, err := p.CreateSemesterFromInput(ctx, "2027-SS", minimalConfigInput())
	if err != nil {
		t.Fatalf("CreateSemesterFromInput: %v", err)
	}
	if res == nil || !res.Ok {
		t.Fatalf("result = %+v, want Ok", res)
	}

	// the semester is registered and carries its config
	config, err := pg.GetSemesterConfigInputFor(ctx, "2027-SS")
	if err != nil {
		t.Fatalf("cannot read back the config: %v", err)
	}
	if config == nil {
		t.Fatal("the new semester has no config")
	}
	if got := pg.SwitchTo(ctx, "2027-SS"); got != "2027 SS" {
		t.Errorf("logical semester = %q, want %q", got, "2027 SS")
	}
}

// Creating the same semester twice must be refused, not silently overwrite the
// planner's config.
func TestCreateSemesterRefusesAnExistingOne(t *testing.T) {
	pg := pgtest.NewDB(t)
	p := &Plexams{dbClient: pg}
	ctx := context.Background()

	if _, err := p.CreateSemesterFromInput(ctx, "2027-SS", minimalConfigInput()); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := p.CreateSemesterFromInput(ctx, "2027-SS", minimalConfigInput()); err == nil {
		t.Error("creating an existing semester must fail")
	}
}

// An invalid config must leave nothing behind. Without the transaction the
// registry row would survive as an empty entry in the switcher -- the very thing
// the foreign key was introduced to prevent, only from the other side.
func TestCreateSemesterLeavesNothingBehindOnFailure(t *testing.T) {
	pg := pgtest.NewDB(t)
	p := &Plexams{dbClient: pg}
	ctx := context.Background()

	bad := minimalConfigInput()
	bad.StartTimes = nil // rejected by validateSemesterConfigInput

	if _, err := p.CreateSemesterFromInput(ctx, "2027-WS", bad); err == nil {
		t.Fatal("an invalid config must be refused")
	}
	names, err := pg.AllSemesterNames(ctx)
	if err != nil {
		t.Fatalf("AllSemesterNames: %v", err)
	}
	for _, s := range names {
		if s.ID == "2027-WS" {
			t.Errorf("a failed create left the semester %q behind", s.ID)
		}
	}
}
