package db_test

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/internal/pgtest"
)

// distinctGenerationConfig fills every exported field with its own value.
//
// Filled by reflection on purpose: the struct is ~40 numeric weights that are all
// spelled alike, and a hand-written fixture would drift the moment a weight is
// added -- the new field would arrive at its zero value and the round trip would
// still pass. The enum-typed fields get valid values by hand; everything else is
// derived from the field index, so no two fields can be swapped without the
// comparison noticing.
func distinctGenerationConfig(t *testing.T) *model.GenerationConfig {
	t.Helper()

	// Deliberately not the first value of each enum: a mapper that dropped the
	// field would land on the zero value, which is the empty string, but a
	// default-carrying one would land on AUTO/HARD.
	enums := map[string]string{
		"SlotTimeMode":        string(model.SlotTimeConstraintModeSummer),
		"SlotTimeEnforcement": string(model.SlotTimeConstraintEnforcementSoft),
		"RoomHeatMode":        string(model.RoomHeatConstraintModeOff),
	}

	cfg := &model.GenerationConfig{}
	value := reflect.ValueOf(cfg).Elem()
	typ := value.Type()

	for i := range typ.NumField() {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		target := value.Field(i)

		switch target.Kind() {
		case reflect.Int:
			target.SetInt(int64(100 + i))
		case reflect.Float64:
			target.SetFloat(float64(i) + 0.5)
		case reflect.String:
			if enum, ok := enums[field.Name]; ok {
				target.SetString(enum)
				continue
			}
			// SlotTimeWinterEarliest / SlotTimeSummerLatest are HH:MM strings.
			target.SetString("1" + strconv.Itoa(i%10) + ":30")
		default:
			t.Fatalf("field %s has kind %s, which this fixture does not know how to fill",
				field.Name, target.Kind())
		}
	}

	return cfg
}

// TestGenerationConfigRoundTrip is the value counterpart to
// TestJSONBTypesLoseNoField: that one proves no field is dropped structurally,
// this one proves none is dropped, swapped or truncated on the way through jsonb.
func TestGenerationConfigRoundTrip(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	want := distinctGenerationConfig(t)
	if err := pg.SetGenerationConfig(ctx, want); err != nil {
		t.Fatalf("SetGenerationConfig: %v", err)
	}

	got, err := pg.GetGenerationConfig(ctx)
	if err != nil {
		t.Fatalf("GetGenerationConfig: %v", err)
	}
	if got == nil {
		t.Fatal("generation config is nil")
	}
	if !reflect.DeepEqual(want, got) {
		// Name the fields that differ rather than dumping 40 numbers twice.
		wantValue, gotValue := reflect.ValueOf(want).Elem(), reflect.ValueOf(got).Elem()
		typ := wantValue.Type()
		for i := range typ.NumField() {
			if !typ.Field(i).IsExported() {
				continue
			}
			if !reflect.DeepEqual(wantValue.Field(i).Interface(), gotValue.Field(i).Interface()) {
				t.Errorf("%s = %v, want %v", typ.Field(i).Name,
					gotValue.Field(i).Interface(), wantValue.Field(i).Interface())
			}
		}
	}
}

func TestGenerationConfigMissingReturnsNilNil(t *testing.T) {
	pg := pgtest.NewDB(t)

	got, err := pg.GetGenerationConfig(t.Context())
	if err != nil {
		t.Fatalf("GetGenerationConfig: %v", err)
	}
	if got != nil {
		t.Errorf("GetGenerationConfig = %v, want nil", got)
	}
}

// TestGenerationConfigUnknownFormatVersionFails is what the format_version column
// is for. The json tags of model.GenerationConfig are simultaneously the GraphQL
// contract, so a rename in the .graphqls changes the storage format without
// touching anything that looks like storage. Without this check the weights would
// come back at zero and the generator would run with them.
func TestGenerationConfigUnknownFormatVersionFails(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	if err := pg.SetGenerationConfig(ctx, distinctGenerationConfig(t)); err != nil {
		t.Fatalf("SetGenerationConfig: %v", err)
	}
	exec(t, pg, "update generation_config set format_version = 2 where id = 1")

	got, err := pg.GetGenerationConfig(ctx)
	if err == nil {
		t.Fatal("a blob in an unknown format version was read without complaint")
	}
	if got != nil {
		t.Errorf("GetGenerationConfig = %v, want nil alongside the error", got)
	}
}

func TestGenerationConfigIsASingleton(t *testing.T) {
	pg := pgtest.NewDB(t)
	ctx := t.Context()

	first := distinctGenerationConfig(t)
	if err := pg.SetGenerationConfig(ctx, first); err != nil {
		t.Fatalf("SetGenerationConfig: %v", err)
	}

	second := distinctGenerationConfig(t)
	second.Iterations = 4711
	if err := pg.SetGenerationConfig(ctx, second); err != nil {
		t.Fatalf("SetGenerationConfig (second): %v", err)
	}

	if n := count(t, pg, "select count(*) from generation_config"); n != 1 {
		t.Errorf("generation_config rows = %d, want 1", n)
	}
	got, err := pg.GetGenerationConfig(ctx)
	if err != nil {
		t.Fatalf("GetGenerationConfig: %v", err)
	}
	if got.Iterations != 4711 {
		t.Errorf("Iterations = %d, want 4711", got.Iterations)
	}
}
