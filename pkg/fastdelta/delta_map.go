// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package fastdelta

import (
	"fmt"

	"github.com/spaolacci/murmur3"

	"github.com/grafana/pyroscope/pkg/pproflite"
)

// As of Go 1.19, the Go heap profile has 4 values per sample, with 2 of them
// being relevant for delta profiling. This is the most for any of the Go
// runtime profiles. In order to make the map of samples to their values more
// GC-friendly, we prefer to have the values for that mapping be fixed-size
// arrays rather than slices. However, this means we can't process profiles
// with more than this many values per sample.
const maxSampleValues = 2

type (
	sampleValue     [maxSampleValues]int64
	fullSampleValue [maxSampleValues + 2]int64
)

// NewDeltaMap ...
func NewDeltaMap(st *stringTable, lx *locationIndex, fields []valueType) *DeltaMap {
	return &DeltaMap{
		h:                    Hasher{alg: murmur3.New128(), st: st, lx: lx},
		m:                    map[Hash]combinedSampleValue{},
		st:                   st,
		fields:               fields,
		computeDeltaForValue: make([]bool, 0, 4),
	}
}

type combinedSampleValue struct {
	// old tracks the previously observed value for a sample, restricted to
	// the values for which we want to compute deltas
	old sampleValue
	// newFull aggregates the full current value for the sample, as we may
	// have non-zero values for the non-delta fields in a duplicated sample.
	// At the very least, we haven't ruled out that possibilty.
	newFull fullSampleValue
	written bool
}

// DeltaMap ...
type DeltaMap struct {
	h  Hasher
	m  map[Hash]combinedSampleValue
	st *stringTable
	// fields are the name and types of the values in a sample for which we should
	// compute the difference.
	fields               []valueType
	computeDeltaForValue []bool
	// valueTypeIndices are string table indices of the sample value type names
	// (e.g.  "alloc_space", "cycles"...) and their types ("count", "bytes")
	valueTypeIndices [][2]int
}

// Reset ...
func (dm *DeltaMap) Reset() {
	dm.valueTypeIndices = dm.valueTypeIndices[:0]
	dm.computeDeltaForValue = dm.computeDeltaForValue[:0]
}

// AddSampleType ...
func (dm *DeltaMap) AddSampleType(st *pproflite.SampleType) error {
	dm.valueTypeIndices = append(dm.valueTypeIndices, [2]int{int(st.Type), int(st.Unit)})
	return nil
}

// UpdateSample ...
func (dm *DeltaMap) UpdateSample(sample *pproflite.Sample) error {
	if err := dm.prepare(); err != nil {
		return err
	}

	hash, err := dm.h.Sample(sample)
	if err != nil {
		return err
	}

	var c combinedSampleValue
	old := dm.m[hash]
	c.old = old.old
	// With duplicate samples, we want to aggregate all of the values,
	// even the ones we aren't taking deltas for.
	for i, v := range sample.Value {
		c.newFull[i] = old.newFull[i] + v
	}
	dm.m[hash] = c
	return nil
}

// Delta updates sample.Value by looking up the previous values for this sample
// and substracting them from the current values. The returned boolean is true
// if the the new sample.Value contains at least one non-zero value.
func (dm *DeltaMap) Delta(sample *pproflite.Sample) (bool, error) {
	if err := dm.prepare(); err != nil {
		return false, err
	}

	hash, err := dm.h.Sample(sample)
	if err != nil {
		return false, err
	}

	c, ok := dm.m[hash]
	if !ok {
		// !ok should not happen, since the prior pass visited every sample
		return false, fmt.Errorf("found sample with unknown hash in merge pass")
	}
	if c.written {
		return false, nil
	}
	all0 := true
	n := 0
	for i := range sample.Value {
		if dm.computeDeltaForValue[i] {
			sample.Value[i] = c.newFull[i] - c.old[n]
			c.old[n] = c.newFull[i]
			n++
		} else {
			sample.Value[i] = c.newFull[i]
		}
		if sample.Value[i] != 0 {
			all0 = false
		}
	}

	c.written = true
	c.newFull = fullSampleValue{}
	dm.m[hash] = c

	// If the sample has all 0 values, we drop it
	// this matches the behavior of Google's pprof library
	// when merging profiles
	return !all0, nil
}

func (dm *DeltaMap) prepare() error {
	if len(dm.computeDeltaForValue) > 0 {
		return nil
	}
	for len(dm.computeDeltaForValue) < len(dm.valueTypeIndices) {
		dm.computeDeltaForValue = append(dm.computeDeltaForValue, false)
	}
	n := 0
	for _, field := range dm.fields {
		for i, vtIdxs := range dm.valueTypeIndices {
			typeMatch := dm.st.Equals(vtIdxs[0], field.Type)
			unitMatch := dm.st.Equals(vtIdxs[1], field.Unit)
			if typeMatch && unitMatch {
				n++
				dm.computeDeltaForValue[i] = true
				if n > maxSampleValues {
					return fmt.Errorf("sample has more than %d maxSampleValues", maxSampleValues)
				}
				break
			}
		}
	}
	return nil
}
