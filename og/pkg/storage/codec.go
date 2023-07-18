package storage

import (
	"fmt"
	"io"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type treeCodec struct{ *Storage }

func (treeCodec) New(_ string) interface{} { return tree.New() }

func (c treeCodec) Serialize(w io.Writer, k string, v interface{}) error {
	key := segment.FromTreeToDictKey(k)
	d, err := c.dicts.GetOrCreate(key)
	if err != nil {
		return err
	}
	err = v.(*tree.Tree).SerializeTruncate(d.(*dict.Dict), c.config.maxNodesSerialization, w)
	if err != nil {
		return err
	}
	c.dicts.Put(key, d)
	return nil
}

func (c treeCodec) Deserialize(r io.Reader, k string) (interface{}, error) {
	key := segment.FromTreeToDictKey(k)
	d, err := c.dicts.GetOrCreate(key)
	if err != nil {
		return nil, fmt.Errorf("dicts cache for %v: %w", key, err)
	}
	return tree.Deserialize(d.(*dict.Dict), r)
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
