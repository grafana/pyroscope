package storage

import (
	"io"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/profile"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

type profileCodec struct{}

func (profileCodec) New(_ string) interface{} { return profile.New() }

func (profileCodec) Serialize(w io.Writer, _ string, v interface{}) error {
	return v.(*profile.Profile).Serialize(w)
}

func (profileCodec) Deserialize(r io.Reader, _ string) (interface{}, error) {
	return profile.Deserialize(r)
}

type dictionaryCodec struct{}

func (dictionaryCodec) New(_ string) interface{} { return dict.New() }

func (dictionaryCodec) Serialize(w io.Writer, _ string, v interface{}) error {
	return v.(*dict.Dict).Serialize(w)
}

func (dictionaryCodec) Deserialize(r io.Reader, _ string) (interface{}, error) {
	return dict.Deserialize(r)
}

type segmentCodec struct{}

func (segmentCodec) New(_ string) interface{} { return segment.New() }

func (segmentCodec) Serialize(w io.Writer, _ string, v interface{}) error {
	return v.(*segment.Segment).Serialize(w)
}

func (segmentCodec) Deserialize(r io.Reader, _ string) (interface{}, error) {
	return segment.Deserialize(r)
}

type dimensionCodec struct{}

func (dimensionCodec) New(_ string) interface{} { return dimension.New() }

func (dimensionCodec) Serialize(w io.Writer, _ string, v interface{}) error {
	return v.(*dimension.Dimension).Serialize(w)
}

func (dimensionCodec) Deserialize(r io.Reader, _ string) (interface{}, error) {
	return dimension.Deserialize(r)
}
