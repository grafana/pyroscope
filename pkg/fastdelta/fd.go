// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

/*
Package fastdelta tries to match up samples between two pprof profiles and take
their difference. A sample is a unique (call stack, labels) pair with an
associated sequence of values, where "call stack" refers to a sequence of
program counters/instruction addresses, and labels are key/value pairs
associated with a stack (so we can have the same call stack appear in two
different samples if the labels are different)

The github.com/google/pprof already implements this functionality as
profile.Merge, but unfortunately it's causing an extreme amount of allocations
and memory usage. This package provides an alternative implementation that has
been highly optimized to be allocation free once steady-state is reached (no
more new samples are added) and to also use a minimum amount of memory and
allocations while growing.

# Implementation

Computing the delta profile takes six passes over the input:

Pass 1
* Build a mapping of location IDs to instruction addresses
* Build the string table, so we can resolve label keys and values
* Find the sample types by name, so we know which sample values to
compute differences for

Pass 2
* For each sample, aggregate the value for the sample. The Go runtime
heap profile can sometimes contain multiple samples with the same call stack and
labels, which should actually be aggregated into one sample.

Pass 3
* Compute the delta values for each sample usings its previous values
and write them out if this leaves us with at least one non-zero
values.
* Update the previous sample values for the next round.
* Keep track of the locations and strings we need given the samples we
wrote out.

Pass 4
* Write out all fields that were referenced by the samples in Pass 3.
* Keep track of strings and function ids we need to emit in the next pass.

Pass 5
* Write out the functions we need and keep track of their strings.

Pass 6
* Write out all the strings that were referenced by previous passes.
* For strings not referenced, write out a zero-length byte to save space
while preserving index references in the included messages

Note: It's possible to do all of the above with less passes, but doing so
requires keeping more stuff in memory. Since extra passes are relatively cheap
and our CPU usage is pretty low (~100ms for a 10MB heap profile), we prefer
optimizing for lower memory usage as there is a larger chance that customers
will complain about it.
*/
package fastdelta

import (
	"fmt"
	"io"

	"github.com/spaolacci/murmur3"

	"github.com/grafana/pyroscope/pkg/pproflite"
)

// ValueType describes the type and unit of a value.
type ValueType struct {
	Type string
	Unit string
}

// DeltaComputer calculates the difference between pprof-encoded profiles
type DeltaComputer struct {
	// poisoned indicates that the previous delta computation ended
	// prematurely due to an error. This means the state of the
	// DeltaComputer is invalid, and the delta computer needs to be re-set
	poisoned bool
	// fields are the name and types of the values in a sample for which we should
	// compute the difference.
	fields []valueType // TODO(fg) would be nice to push this into deltaMap

	decoder           pproflite.Decoder
	encoder           pproflite.Encoder
	deltaMap          *DeltaMap
	includedFunctions SparseIntSet
	includedStrings   DenseIntSet
	// locationIndex associates location IDs (used by the pprof format to
	// cross-reference locations) to the actual instruction address of the
	// location
	locationIndex locationIndex
	// strings holds (hashed) copies of every string in the string table
	// of the current profile, used to hold the names of sample value types,
	// and the keys and values of labels.
	strings          *stringTable
	curProfTimeNanos int64
	durationNanos    pproflite.DurationNanos
}

// NewDeltaComputer initializes a DeltaComputer which will calculate the
// difference between the values for profile samples whose fields have the given
// names (e.g. "alloc_space", "contention", ...)
func NewDeltaComputer(fields ...ValueType) *DeltaComputer {
	dc := &DeltaComputer{fields: newValueTypes(fields)}
	dc.initialize()
	return dc
}

func (dc *DeltaComputer) initialize() {
	dc.strings = newStringTable(murmur3.New128())
	dc.curProfTimeNanos = -1
	dc.deltaMap = NewDeltaMap(dc.strings, &dc.locationIndex, dc.fields)
}

func (dc *DeltaComputer) reset() {
	dc.strings.Reset()
	dc.locationIndex.Reset()
	dc.deltaMap.Reset()

	dc.includedFunctions.Reset()
	dc.includedStrings.Reset()
}

// Delta calculates the difference between the pprof-encoded profile p and the
// profile passed in to the previous call to Delta. The encoded delta profile
// will be written to out.
//
// The first time Delta is called, the internal state of the DeltaComputer will
// be updated and the profile will be written unchanged.
func (dc *DeltaComputer) Delta(p []byte, out io.Writer) error {
	if err := dc.delta(p, out); err != nil {
		dc.poisoned = true
		return err
	}
	if dc.poisoned {
		// If we're recovering from a bad state, we'll use the first
		// profile to re-set the state. Technically the profile has
		// already been written to out, but we return an error to
		// indicate that the profile shouldn't be used.
		dc.poisoned = false
		return fmt.Errorf("delta profiler recovering from bad state, skipping this profile")
	}
	return nil
}

func (dc *DeltaComputer) delta(p []byte, out io.Writer) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("internal panic during delta profiling: %v", e)
		}
	}()

	if dc.poisoned {
		// If the last round failed, start fresh
		dc.initialize()
	}
	dc.reset()

	dc.encoder.Reset(out)
	dc.decoder.Reset(p)

	if err := dc.pass1Index(); err != nil {
		return fmt.Errorf("pass1Index: %w", err)
	} else if err := dc.pass2AggregateSamples(); err != nil {
		return fmt.Errorf("pass2AggregateSamples: %w", err)
	} else if err := dc.pass3MergeSamples(); err != nil {
		return fmt.Errorf("pass3MergeSamples: %w", err)
	} else if err := dc.pass4WriteAndPruneRecords(); err != nil {
		return fmt.Errorf("pass4WriteAndPruneRecords: %w", err)
	} else if err := dc.pass5WriteFunctions(); err != nil {
		return fmt.Errorf("pass5WriteFunctions: %w", err)
	} else if err := dc.pass6WriteStringTable(); err != nil {
		return fmt.Errorf("pass6WriteStringTable: %w", err)
	}
	return nil
}

func (dc *DeltaComputer) pass1Index() error {
	strIdx := 0
	return dc.decoder.FieldEach(
		func(f pproflite.Field) error {
			switch t := f.(type) {
			case *pproflite.SampleType:
				if err := dc.deltaMap.AddSampleType(t); err != nil {
					return err
				}
			case *pproflite.Location:
				dc.locationIndex.Insert(t.ID, t.Address)
			case *pproflite.StringTable:
				dc.strings.Add(t.Value)
				// always include the zero-index empty string, otherwise exclude by
				// default unless used by a kept sample in pass3MergeSamples
				dc.includedStrings.Append(strIdx == 0)
				strIdx++
			default:
				return fmt.Errorf("unexpected field: %T", f)
			}
			return nil
		},
		pproflite.SampleTypeDecoder,
		pproflite.LocationDecoder,
		pproflite.StringTableDecoder,
	)
}

func (dc *DeltaComputer) pass2AggregateSamples() error {
	return dc.decoder.FieldEach(
		func(f pproflite.Field) error {
			sample, ok := f.(*pproflite.Sample)
			if !ok {
				return fmt.Errorf("unexpected field: %T", f)
			}

			if err := validStrings(sample, dc.strings); err != nil {
				return err
			}

			return dc.deltaMap.UpdateSample(sample)
		},
		pproflite.SampleDecoder,
	)
}

func (dc *DeltaComputer) pass3MergeSamples() error {
	return dc.decoder.FieldEach(
		func(f pproflite.Field) error {
			sample, ok := f.(*pproflite.Sample)
			if !ok {
				return fmt.Errorf("unexpected field: %T", f)
			}

			if err := validStrings(sample, dc.strings); err != nil {
				return err
			}

			if hasNonZeroValues, err := dc.deltaMap.Delta(sample); err != nil {
				return err
			} else if !hasNonZeroValues {
				return nil
			}

			for _, locationID := range sample.LocationID {
				dc.locationIndex.MarkIncluded(locationID)
			}
			for _, l := range sample.Label {
				dc.includedStrings.Add(int(l.Key), int(l.Str), int(l.NumUnit))
			}
			return dc.encoder.Encode(sample)
		},
		pproflite.SampleDecoder,
	)
}

func (dc *DeltaComputer) pass4WriteAndPruneRecords() error {
	firstPprof := dc.curProfTimeNanos < 0
	return dc.decoder.FieldEach(
		func(f pproflite.Field) error {
			switch t := f.(type) {
			case *pproflite.SampleType:
				dc.includedStrings.Add(int(t.Unit), int(t.Type))
			case *pproflite.Mapping:
				dc.includedStrings.Add(int(t.Filename), int(t.BuildID))
			case *pproflite.LocationFast:
				if !dc.locationIndex.Included(t.ID) {
					return nil
				}
				for _, funcID := range t.FunctionID {
					dc.includedFunctions.Add(int(funcID))
				}
			case *pproflite.DropFrames:
				dc.includedStrings.Add(int(t.Value))
			case *pproflite.KeepFrames:
				dc.includedStrings.Add(int(t.Value))
			case *pproflite.TimeNanos:
				curProfTimeNanos := t.Value
				if !firstPprof {
					prevProfTimeNanos := dc.curProfTimeNanos
					if err := dc.encoder.Encode(t); err != nil {
						return err
					}
					dc.durationNanos.Value = curProfTimeNanos - prevProfTimeNanos
					f = &dc.durationNanos
				}
				dc.curProfTimeNanos = curProfTimeNanos
			case *pproflite.DurationNanos:
				if !firstPprof {
					return nil
				}
			case *pproflite.PeriodType:
				dc.includedStrings.Add(int(t.Unit), int(t.Type))
			case *pproflite.Period:
			case *pproflite.Comment:
				dc.includedStrings.Add(int(t.Value))
			case *pproflite.DefaultSampleType:
				dc.includedStrings.Add(int(t.Value))
			default:
				return fmt.Errorf("unexpected field: %T", f)
			}
			return dc.encoder.Encode(f)
		},
		pproflite.SampleTypeDecoder,
		pproflite.MappingDecoder,
		pproflite.LocationFastDecoder,
		pproflite.DropFramesDecoder,
		pproflite.KeepFramesDecoder,
		pproflite.TimeNanosDecoder,
		pproflite.DurationNanosDecoder,
		pproflite.PeriodTypeDecoder,
		pproflite.PeriodDecoder,
		pproflite.CommentDecoder,
		pproflite.DefaultSampleTypeDecoder,
	)
}

func (dc *DeltaComputer) pass5WriteFunctions() error {
	return dc.decoder.FieldEach(
		func(f pproflite.Field) error {
			fn, ok := f.(*pproflite.Function)
			if !ok {
				return fmt.Errorf("unexpected field: %T", f)
			}

			if !dc.includedFunctions.Contains(int(fn.ID)) {
				return nil
			}
			dc.includedStrings.Add(int(fn.Name), int(fn.SystemName), int(fn.FileName))
			return dc.encoder.Encode(f)
		},
		pproflite.FunctionDecoder,
	)
}

func (dc *DeltaComputer) pass6WriteStringTable() error {
	counter := 0
	return dc.decoder.FieldEach(
		func(f pproflite.Field) error {
			str, ok := f.(*pproflite.StringTable)
			if !ok {
				return fmt.Errorf("unexpected field: %T", f)
			}
			if !dc.includedStrings.Contains(counter) {
				str.Value = nil
			}
			counter++
			return dc.encoder.Encode(str)
		},
		pproflite.StringTableDecoder,
	)
}

// TODO(fg) we should probably validate all strings? not just label strings?
func validStrings(s *pproflite.Sample, st *stringTable) error {
	for _, l := range s.Label {
		if !st.Contains(uint64(l.Key)) {
			return fmt.Errorf("invalid string index %d", l.Key)
		}
		if !st.Contains(uint64(l.Str)) {
			return fmt.Errorf("invalid string index %d", l.Str)
		}
		if !st.Contains(uint64(l.NumUnit)) {
			return fmt.Errorf("invalid string index %d", l.NumUnit)
		}
	}
	return nil
}

// newValueTypes is needed to avoid allocating DeltaMap.prepare.
func newValueTypes(vts []ValueType) (ret []valueType) {
	for _, vt := range vts {
		ret = append(ret, valueType{Type: []byte(vt.Type), Unit: []byte(vt.Unit)})
	}
	return
}

type valueType struct {
	Type []byte
	Unit []byte
}
