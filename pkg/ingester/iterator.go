package ingester

import (
	"bytes"
	"sort"
)

type sample struct {
	stacktraceID []byte
	locationIDs  [][]byte
	value        int64
}

type iterator struct {
	samples []sample
	current sample

	level       int
	total       int64
	initialized bool
}

func newIterator(samples []sample) *iterator {
	if len(samples) == 0 {
		return &iterator{}
	}
	return &iterator{
		samples: samples,
	}
}

func (i *iterator) Next() bool {
	if len(i.samples) == 0 {
		return false
	}
	if !i.initialized {
		i.initialized = true
		i.current = i.samples[0]
		i.level = 0
		i.total = i.current.value
		i.samples = i.samples[1:]
		return true
	}
	// s1 [1 2 3]
	// s2 [1 4 5]
	// s3 [1 3 6]
	_ = sort.Search(len(i.samples), func(idx int) bool {
		for _, locID := range i.samples[idx].locationIDs {
			if bytes.Equal(locID, i.current.locationIDs[0]) {
				i.current.value = i.samples[idx].value
				i.total += i.samples[idx].value
				return true
			}
		}
		return false
	})

	return false
}

func (i *iterator) Level() int {
	return i.level
}

func (i *iterator) Self() int64 {
	return i.current.value
}

func (i *iterator) Total() int64 {
	return i.total
}

func (i *iterator) StacktraceID() []byte {
	return i.current.stacktraceID
}

func (i *iterator) LocationIDs() [][]byte {
	return i.current.locationIDs
}
