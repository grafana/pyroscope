// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package fastdelta

// locationIndex links location IDs to the addresses, mappings, and function
// IDs referenced by the location
type locationIndex struct {
	fastTable []location
	slowTable map[uint64]location
}

type location struct {
	address  uint64
	included bool
}

func (l *locationIndex) Reset() {
	l.fastTable = l.fastTable[:0]
	for k := range l.slowTable {
		delete(l.slowTable, k)
	}
}

// Insert associates the given address, mapping ID, and function IDs with the
// given location ID
func (l *locationIndex) Insert(id, address uint64) {
	loc := location{address: address}
	if l.slowTable == nil && id == uint64(len(l.fastTable)+1) {
		l.fastTable = append(l.fastTable, loc)
	} else {
		if l.slowTable == nil {
			l.slowTable = make(map[uint64]location, len(l.fastTable))
			for i, oldLoc := range l.fastTable {
				l.slowTable[uint64(i)+1] = oldLoc
			}
		}
		l.slowTable[id] = loc
	}
}

func (l *locationIndex) MarkIncluded(id uint64) {
	// TODO(fg) duplicated with get() function below
	if l.slowTable == nil {
		id--
		if id >= uint64(len(l.fastTable)) {
			return
		}
		l.fastTable[id].included = true
	} else {
		loc, ok := l.slowTable[id]
		if ok {
			loc.included = true
			l.slowTable[id] = loc
		}
	}
}

func (l *locationIndex) Included(id uint64) bool {
	loc, _ := l.get(id)
	return loc.included
}

// Get returns the address associated with the given location ID
func (l *locationIndex) Get(id uint64) (uint64, bool) {
	loc, ok := l.get(id)
	return loc.address, ok
}

func (l *locationIndex) get(id uint64) (loc location, ok bool) {
	if l.slowTable == nil {
		id--
		if id >= uint64(len(l.fastTable)) {
			return
		}
		ok = true
		loc = l.fastTable[id]
	} else {
		loc, ok = l.slowTable[id]
	}
	return
}
