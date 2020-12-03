package segment

import (
	"time"
)

type rel int

const (
	inside  rel = iota // | S E |
	match              // matching ranges
	outside            // | | S E
	overlap            // | S | E
	contain            // S | | E
)

var overlapStrings map[rel]string

// TODO: I bet there's a better way
func init() {
	overlapStrings = make(map[rel]string)
	overlapStrings[inside] = "inside"
	overlapStrings[outside] = "outside"
	overlapStrings[match] = "match"
	overlapStrings[overlap] = "overlap"
	overlapStrings[contain] = "contain"
}

func (r rel) String() string {
	return overlapStrings[r]
}

func relationship(t1, t2, st, et time.Time) rel {
	if t1.Equal(st) && t2.Equal(et) {
		return match
	}
	if !t1.After(st) && !t2.Before(et) {
		return contain
	}
	if !t1.Before(st) && !t2.After(et) {
		return inside
	}
	if !t1.After(st) && !t2.After(st) {
		return outside
	}
	if !t1.Before(et) && !t2.Before(et) {
		return outside
	}

	return overlap
}

// t >= st && t + dur2 <= et
func isInside(t, st, et time.Time, dur2 time.Duration) bool {
	t2 := t.Add(dur2)
	rel := relationship(t, t2, st, et)
	// log.Debug("rel", rel)
	return rel == inside || rel == match
}

// st >= t && et <= t + dur2
func isMatchOrContain(t, st, et time.Time, dur2 time.Duration) bool {
	t2 := t.Add(dur2)
	rel := relationship(t, t2, st, et)
	// log.Debug("isMatchOrContain", t, dur2, st, et, rel)
	return rel == contain || rel == match
}

// st >= t && et <= t + dur2
func isNotOutside(t, st, et time.Time, dur2 time.Duration) bool {
	t2 := t.Add(dur2)
	rel := relationship(t, t2, st, et)
	// log.Debug("isMatchOrContain", t, dur2, st, et, rel)
	return rel == contain || rel == match || rel == inside || rel == overlap
}
