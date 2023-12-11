// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

// Package pproflite implements zero-allocation pprof encoding and decoding.
package pproflite

import (
	"github.com/richardartoul/molecule"
	"github.com/richardartoul/molecule/src/codec"
)

// Field holds the value of a top-level profile.proto Profile.* field.
type Field interface {
	field() int
}

// SampleType is field 1.
type SampleType struct {
	ValueType
}

func (f SampleType) field() int { return 1 }

// Sample is field 2.
type Sample struct {
	LocationID []uint64
	Value      []int64
	Label      []Label
}

func (f Sample) field() int { return 2 }

func (f *Sample) decode(val molecule.Value) error {
	*f = Sample{LocationID: f.LocationID[:0], Value: f.Value[:0], Label: f.Label[:0]}
	// Not using decodeFields() to squeeze out a little more performance.
	return molecule.MessageEach(codec.NewBuffer(val.Bytes), func(field int32, val molecule.Value) (bool, error) {
		switch field {
		case 1:
			return true, decodePackedUint64(val, &f.LocationID)
		case 2:
			return true, decodePackedInt64(val, &f.Value)
		case 3:
			f.Label = append(f.Label, Label{})
			f.Label[len(f.Label)-1].decode(val)
		}
		return true, nil
	})
}

func (f *Sample) encode(ps *molecule.ProtoStream) error {
	if err := encodePackedUint64(ps, 1, f.LocationID); err != nil {
		return err
	} else if err := encodePackedInt64(ps, 2, f.Value); err != nil {
		return err
	}
	for i := range f.Label {
		ps.Embedded(3, f.Label[i].encode)
	}
	return nil
}

// Label is part of Sample.
type Label struct {
	Key     int64
	Str     int64
	Num     int64
	NumUnit int64
}

func (f *Label) fields() []interface{} {
	return []interface{}{nil, &f.Key, &f.Str, &f.Num, &f.NumUnit}
}

func (f *Label) decode(val molecule.Value) error {
	*f = Label{}
	// Not using decodeFields() to squeeze out a little more performance.
	return molecule.MessageEach(codec.NewBuffer(val.Bytes), func(field int32, val molecule.Value) (bool, error) {
		switch field {
		case 1:
			f.Key = int64(val.Number)
		case 2:
			f.Str = int64(val.Number)
		case 3:
			f.Num = int64(val.Number)
		case 4:
			f.NumUnit = int64(val.Number)
		}
		return true, nil
	})
}

func (f *Label) encode(ps *molecule.ProtoStream) error {
	return encodeFields(ps, f.fields())
}

// Mapping is field 3.
type Mapping struct {
	ID              uint64
	MemoryStart     uint64
	MemoryLimit     uint64
	FileOffset      uint64
	Filename        int64
	BuildID         int64
	HasFunctions    bool
	HasFilenames    bool
	HasLineNumbers  bool
	HasInlineFrames bool
}

func (f Mapping) field() int { return 3 }

func (f *Mapping) fields() []interface{} {
	return []interface{}{
		nil,
		&f.ID,
		&f.MemoryStart,
		&f.MemoryLimit,
		&f.FileOffset,
		&f.Filename,
		&f.BuildID,
		&f.HasFunctions,
		&f.HasFilenames,
		&f.HasLineNumbers,
		&f.HasInlineFrames,
	}
}

func (f *Mapping) decode(val molecule.Value) error {
	*f = Mapping{}
	return decodeFields(val, f.fields())
}

func (f *Mapping) encode(ps *molecule.ProtoStream) error {
	return encodeFields(ps, f.fields())
}

// Location is field 4.
type Location struct {
	ID        uint64
	MappingID uint64
	Address   uint64
	Line      []Line
	IsFolded  bool
}

func (f Location) field() int { return 4 }

func (f *Location) fields() []interface{} {
	return []interface{}{
		nil,
		&f.ID,
		&f.MappingID,
		&f.Address,
		&f.Line,
		&f.IsFolded,
	}
}

func (f *Location) decode(val molecule.Value) error {
	*f = Location{Line: f.Line[:0]}
	// Not using decodeFields() to squeeze out a little more performance.
	return molecule.MessageEach(codec.NewBuffer(val.Bytes), func(field int32, val molecule.Value) (bool, error) {
		switch field {
		case 1:
			f.ID = val.Number
		case 2:
			f.MappingID = val.Number
		case 3:
			f.Address = val.Number
		case 4:
			f.Line = append(f.Line, Line{})
			f.Line[len(f.Line)-1].decode(val)
		case 5:
			f.IsFolded = val.Number == 1
		}
		return true, nil
	})
}

func (f *Location) encode(ps *molecule.ProtoStream) error {
	return encodeFields(ps, f.fields())
}

// LocationFast is field 4. Unlike Location it only decodes the id and function
// ids of the location and stores its raw protobuf message. When encoding a
// LocationFast, the Data value gets written and changes to its other fields
// are ignored.
type LocationFast struct {
	ID         uint64
	FunctionID []uint64
	Data       []byte
}

func (f LocationFast) field() int { return 4 }

func (f *LocationFast) decode(val molecule.Value) error {
	*f = LocationFast{FunctionID: f.FunctionID[:0]}
	f.Data = val.Bytes
	return molecule.MessageEach(codec.NewBuffer(val.Bytes), func(field int32, val molecule.Value) (bool, error) {
		switch field {
		case 1:
			f.ID = val.Number
		case 4: // Line
			molecule.MessageEach(codec.NewBuffer(val.Bytes), func(field int32, val molecule.Value) (bool, error) {
				if field == 1 {
					f.FunctionID = append(f.FunctionID, val.Number)
				}
				return true, nil
			})
		}
		return true, nil
	})
}

func (f *LocationFast) encode(ps *molecule.ProtoStream) error {
	_, err := ps.Write(f.Data)
	return err
}

// Line is part of Location.
type Line struct {
	FunctionID uint64
	Line       int64
}

func (f *Line) fields() []interface{} {
	return []interface{}{nil, &f.FunctionID, &f.Line}
}

func (f *Line) decode(val molecule.Value) error {
	*f = Line{}
	// Not using decodeFields() to squeeze out a little more performance.
	return molecule.MessageEach(codec.NewBuffer(val.Bytes), func(field int32, val molecule.Value) (bool, error) {
		switch field {
		case 1:
			f.FunctionID = val.Number
		case 2:
			f.Line = int64(val.Number)
		}
		return true, nil
	})
}

func (f *Line) encode(ps *molecule.ProtoStream) error {
	return encodeFields(ps, f.fields())
}

// Function is field 5.
type Function struct {
	ID         uint64
	Name       int64
	SystemName int64
	FileName   int64
	StartLine  int64
}

func (f Function) field() int { return 5 }

func (f *Function) fields() []interface{} {
	return []interface{}{
		nil,
		&f.ID,
		&f.Name,
		&f.SystemName,
		&f.FileName,
		&f.StartLine,
	}
}

func (f *Function) decode(val molecule.Value) error {
	*f = Function{}
	return decodeFields(val, f.fields())
}

func (f *Function) encode(ps *molecule.ProtoStream) error {
	return encodeFields(ps, f.fields())
}

// StringTable is field 6.
type StringTable struct{ Value []byte }

func (f StringTable) field() int { return 6 }

func (f *StringTable) decode(val molecule.Value) error {
	f.Value = val.Bytes
	return nil
}

func (f *StringTable) encode(ps *molecule.ProtoStream) error {
	_, err := ps.Write(f.Value)
	return err
}

// DropFrames is field 7
type DropFrames struct{ Value int64 }

func (f DropFrames) field() int { return 7 }

func (f *DropFrames) decode(val molecule.Value) error {
	f.Value = int64(val.Number)
	return nil
}

func (f *DropFrames) encodePrimitive(ps *molecule.ProtoStream) error {
	ps.Int64(f.field(), f.Value)
	return nil
}

// KeepFrames is field 8
type KeepFrames struct{ Value int64 }

func (f KeepFrames) field() int { return 8 }

func (f *KeepFrames) decode(val molecule.Value) error {
	f.Value = int64(val.Number)
	return nil
}

func (f *KeepFrames) encodePrimitive(ps *molecule.ProtoStream) error {
	ps.Int64(f.field(), f.Value)
	return nil
}

// TimeNanos is field 9
type TimeNanos struct{ Value int64 }

func (f TimeNanos) field() int { return 9 }

func (f *TimeNanos) decode(val molecule.Value) error {
	f.Value = int64(val.Number)
	return nil
}

func (f *TimeNanos) encodePrimitive(ps *molecule.ProtoStream) error {
	ps.Int64(f.field(), f.Value)
	return nil
}

// DurationNanos is field 10
type DurationNanos struct{ Value int64 }

func (f DurationNanos) field() int { return 10 }

func (f *DurationNanos) decode(val molecule.Value) error {
	f.Value = int64(val.Number)
	return nil
}

func (f *DurationNanos) encodePrimitive(ps *molecule.ProtoStream) error {
	ps.Int64(f.field(), f.Value)
	return nil
}

// PeriodType is field 11.
type PeriodType struct {
	ValueType
}

func (f PeriodType) field() int { return 11 }

// Period is field 12
type Period struct{ Value int64 }

func (f Period) field() int { return 12 }

func (f *Period) decode(val molecule.Value) error {
	f.Value = int64(val.Number)
	return nil
}

func (f *Period) encodePrimitive(ps *molecule.ProtoStream) error {
	ps.Int64(f.field(), f.Value)
	return nil
}

// Comment is field 13
type Comment struct{ Value int64 }

func (f Comment) field() int { return 13 }

func (f *Comment) decode(val molecule.Value) error {
	f.Value = int64(val.Number)
	return nil
}

func (f *Comment) encodePrimitive(ps *molecule.ProtoStream) error {
	ps.Int64(f.field(), f.Value)
	return nil
}

// DefaultSampleType is field 14
type DefaultSampleType struct{ Value int64 }

func (f DefaultSampleType) field() int { return 14 }

func (f *DefaultSampleType) decode(val molecule.Value) error {
	f.Value = int64(val.Number)
	return nil
}

func (f *DefaultSampleType) encodePrimitive(ps *molecule.ProtoStream) error {
	ps.Int64(f.field(), f.Value)
	return nil
}

// ValueType is part of SampleType and PeriodType.
type ValueType struct {
	Type int64
	Unit int64
}

func (f *ValueType) fields() []interface{} {
	return []interface{}{nil, &f.Type, &f.Unit}
}

func (f *ValueType) decode(val molecule.Value) error {
	*f = ValueType{}
	return decodeFields(val, f.fields())
}

func (f *ValueType) encode(ps *molecule.ProtoStream) error {
	return encodeFields(ps, f.fields())
}
