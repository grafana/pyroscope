// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package pproflite

import (
	"fmt"
	"io"

	"github.com/richardartoul/molecule"
)

type encoder interface {
	Field
	encode(*molecule.ProtoStream) error
}

type primtiveEncoder interface {
	Field
	encodePrimitive(*molecule.ProtoStream) error
}

// NewEncoder ...
func NewEncoder(w io.Writer) *Encoder {
	e := &Encoder{}
	e.Reset(w)
	return e
}

// Encoder ...
type Encoder struct {
	outWriter io.Writer
	outStream *molecule.ProtoStream
}

// Reset ...
func (e *Encoder) Reset(w io.Writer) {
	e.outWriter = w
	if e.outStream == nil {
		e.outStream = molecule.NewProtoStream(w)
	} else {
		e.outStream.Reset(w)
	}
}

// Encode ...
func (e *Encoder) Encode(f Field) error {
	switch t := f.(type) {
	case encoder:
		return e.outStream.Embedded(f.field(), t.encode)
	case primtiveEncoder:
		return t.encodePrimitive(e.outStream)
	default:
		return fmt.Errorf("field %T does not support encoder interface", f)
	}
}

func encodeFields(ps *molecule.ProtoStream, fields []interface{}) error {
	for i, f := range fields {
		if f == nil {
			continue
		}

		var err error
		switch t := f.(type) {
		case *bool:
			err = ps.Bool(i, *t)
		case *int64:
			err = ps.Int64(i, *t)
		case *uint64:
			err = ps.Uint64(i, *t)
		case *[]uint64:
			err = encodePackedUint64(ps, i, *t)
		case *[]int64:
			err = encodePackedInt64(ps, i, *t)
		case *[]Label:
			for j := range *t {
				if err = ps.Embedded(i, (*t)[j].encode); err != nil {
					break
				}
			}
		case *[]Line:
			for j := range *t {
				if err = ps.Embedded(i, (*t)[j].encode); err != nil {
					break
				}
			}
		default:
			err = fmt.Errorf("encodeFields: unknown type: %T", t)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func encodePackedInt64(ps *molecule.ProtoStream, field int, vals []int64) error {
	if len(vals) == 1 {
		return ps.Int64(field, vals[0])
	}
	return ps.Int64Packed(field, vals)
}

func encodePackedUint64(ps *molecule.ProtoStream, field int, vals []uint64) error {
	if len(vals) == 1 {
		return ps.Uint64(field, vals[0])
	}
	return ps.Uint64Packed(field, vals)
}
