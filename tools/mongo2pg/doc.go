package main

import (
	"fmt"
	"sort"
	"time"

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
	// children are sub-documents handed out by sub(); their leftovers are reported
	// with this document's, so an unread key inside `emails` or `constraints` is as
	// visible as one at the top level.
	children map[string]*doc
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

// timeVal reads a BSON datetime.
//
// The driver decodes it as primitive.DateTime (milliseconds since the epoch),
// whose Time() carries the UTC location. Everything downstream does day and slot
// arithmetic in Europe/Berlin and keys maps by time.Time -- where the same
// instant with a UTC location is a DIFFERENT key. The location is therefore
// converted here, once, instead of at every use. See TestTimestamptzKeepsLocation
// in db/ for the same rule on the other side.
func (d *doc) timeVal(names ...string) (time.Time, bool) {
	v, ok := d.get(names...)
	if !ok {
		return time.Time{}, false
	}
	switch t := v.(type) {
	case primitive.DateTime:
		return t.Time().Local(), true
	case time.Time:
		return t.Local(), true
	}
	return time.Time{}, false
}

func (d *doc) timePtr(names ...string) *time.Time {
	if t, ok := d.timeVal(names...); ok {
		return &t
	}
	return nil
}

// timeSlice reads an array of BSON datetimes, skipping entries that are not one.
func (d *doc) timeSlice(names ...string) []time.Time {
	v, ok := d.get(names...)
	if !ok {
		return nil
	}
	arr, ok := asSlice(v)
	if !ok {
		return nil
	}
	out := make([]time.Time, 0, len(arr))
	for _, e := range arr {
		switch t := e.(type) {
		case primitive.DateTime:
			out = append(out, t.Time().Local())
		case time.Time:
			out = append(out, t.Local())
		}
	}
	return out
}

// timePtrSlice is timeSlice for the model fields that hold []*time.Time.
func (d *doc) timePtrSlice(names ...string) []*time.Time {
	times := d.timeSlice(names...)
	if len(times) == 0 {
		return nil
	}
	out := make([]*time.Time, 0, len(times))
	for i := range times {
		out = append(out, &times[i])
	}
	return out
}

// ints reads an array of BSON numbers.
func (d *doc) ints(names ...string) []int {
	v, ok := d.get(names...)
	if !ok {
		return nil
	}
	arr, ok := asSlice(v)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(arr))
	for _, e := range arr {
		switch n := e.(type) {
		case int32:
			out = append(out, int(n))
		case int64:
			out = append(out, int(n))
		case int:
			out = append(out, n)
		case float64:
			out = append(out, int(n))
		}
	}
	return out
}

// sub returns an embedded document, or nil when there is none.
//
// The result is a doc of its own, so the same read-tracking applies one level
// down: `emails` and `constraints` are sub-documents with a dozen keys each, and
// a converter that forgets one of them would otherwise drop it without a word.
func (d *doc) sub(names ...string) *doc {
	v, ok := d.get(names...)
	if !ok {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	child := newDoc(m)
	if d.children == nil {
		d.children = map[string]*doc{}
	}
	d.children[names[0]] = child
	return child
}

// subSlice returns an array of embedded documents (e.g. jointProgramAllowedTimes).
func (d *doc) subSlice(names ...string) []*doc {
	v, ok := d.get(names...)
	if !ok {
		return nil
	}
	arr, ok := asSlice(v)
	if !ok {
		return nil
	}
	out := make([]*doc, 0, len(arr))
	for i, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		child := newDoc(m)
		if d.children == nil {
			d.children = map[string]*doc{}
		}
		d.children[fmt.Sprintf("%s[%d]", names[0], i)] = child
		out = append(out, child)
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

// leftovers lists the keys nobody asked for, sorted. Keys of sub-documents are
// qualified with their parent ("emails.bcc"), so the report says where to look.
func (d *doc) leftovers() []string {
	var out []string
	for k, v := range d.m {
		if !d.seen[k] && v != nil {
			out = append(out, k)
		}
	}
	for name, child := range d.children {
		for _, k := range child.leftovers() {
			out = append(out, name+"."+k)
		}
	}
	sort.Strings(out)
	return out
}

// missing formats a required-field complaint for the report.
func missing(kind, key, field string) string {
	return fmt.Sprintf("%s %s: %s fehlt", kind, key, field)
}
