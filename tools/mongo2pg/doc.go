package main

import (
	"fmt"
	"sort"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// doc wraps one BSON document and records which keys were read.
//
// The recording is the point. The task is to move data whose shape drifted over
// seven years into a schema that pins it down, and the dangerous direction is
// silence: a key nobody reads is a key nobody notices losing. So every accessor
// marks what it consumed, and leftovers() reports the rest -- the operator sees
// each dropped field with a count instead of finding out later that something
// was gone.
type doc struct {
	m    map[string]any
	seen map[string]bool
}

func newDoc(m map[string]any) *doc {
	return &doc{m: m, seen: map[string]bool{"_id": true}}
}

// get marks every candidate as consumed and returns the first one present.
//
// Several candidates is the normal case here, not an exception: model structs
// had no bson tags, so the driver lowercased Go field names, and a field written
// before that lowercasing sits under a different key than one written after. Both
// spellings are real data. Marking all candidates seen -- not just the hit --
// keeps the losing spelling out of the leftovers report.
func (d *doc) get(names ...string) (any, bool) {
	var found any
	ok := false
	for _, n := range names {
		d.seen[n] = true
		if !ok {
			if v, present := d.m[n]; present && v != nil {
				found, ok = v, true
			}
		}
	}
	return found, ok
}

func (d *doc) str(names ...string) string {
	if v, ok := d.get(names...); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (d *doc) strPtr(names ...string) *string {
	if s := d.str(names...); s != "" {
		return &s
	}
	return nil
}

func (d *doc) boolean(names ...string) bool {
	if v, ok := d.get(names...); ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// number accepts int32/int64/float64 because the same logical value can arrive
// as any of them depending on how it was written.
func (d *doc) number(names ...string) (int, bool) {
	v, ok := d.get(names...)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case int:
		return n, true
	case float64:
		return int(n), true
	}
	return 0, false
}

func (d *doc) integer(names ...string) int {
	n, _ := d.number(names...)
	return n
}

func (d *doc) intPtr(names ...string) *int {
	if n, ok := d.number(names...); ok {
		return &n
	}
	return nil
}

// asSlice unwraps a BSON array.
//
// The driver decodes arrays as primitive.A, which is a NAMED type whose
// underlying type is []interface{} -- so a plain `v.([]any)` assertion fails and
// returns nothing. It cost the Anny personalization names once: the import
// reported one document written and wrote an empty list. Anything reading an
// array here must go through this.
func asSlice(v any) ([]any, bool) {
	switch a := v.(type) {
	case primitive.A:
		return []any(a), true
	case []any:
		return a, true
	}
	return nil, false
}

func (d *doc) strings(names ...string) []string {
	v, ok := d.get(names...)
	if !ok {
		return nil
	}
	arr, ok := asSlice(v)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ignore marks keys as consumed without reading them: fields the model
// deliberately no longer has. Naming them here rather than letting them fall
// into the leftovers report is the difference between "we decided to drop this"
// and "we did not notice this".
func (d *doc) ignore(names ...string) {
	for _, n := range names {
		d.seen[n] = true
	}
}

// leftovers lists the keys nobody asked for, sorted.
func (d *doc) leftovers() []string {
	var out []string
	for k, v := range d.m {
		if !d.seen[k] && v != nil {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// missing formats a required-field complaint for the report.
func missing(kind, key, field string) string {
	return fmt.Sprintf("%s %s: %s fehlt", kind, key, field)
}
