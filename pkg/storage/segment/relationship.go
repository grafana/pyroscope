package segment

import (
	"time"
)

type rel int

const (
	// relationship                          overlap read    overlap write
	inside  rel = iota // | S E |            <1              1/1
	match              // matching ranges    1/1             1/1
	outside            // | | S E            0/1             0/1
	overlap            // | S | E            <1              <1
	contain            // S | | E            1/1             <1
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

// t1, t2 represent segment node, st, et represent the read/write query time range
func relationship(t1, t2, st, et time.Time) rel {
	if t1.Equal(st) && t2.Equal(et) {
		return match
	}
	if !t1.After(st) && !t2.Before(et) {
		return inside
	}
	if !t1.Before(st) && !t2.After(et) {
		return contain
	}
	if !t1.After(st) && !t2.After(st) {
		return outside
	}
	if !t1.Before(et) && !t2.Before(et) {
		return outside
	}

	return overlap
}
