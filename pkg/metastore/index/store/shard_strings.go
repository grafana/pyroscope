package store

import (
	"encoding/binary"

	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/metastore/store"
)

type stringIterator struct {
	iter.Iterator[store.KV]
	batch []string
	cur   int
	err   error
}

func newStringIter(i iter.Iterator[store.KV]) *stringIterator {
	return &stringIterator{Iterator: i}
}

func (i *stringIterator) Err() error {
	if err := i.Iterator.Err(); err != nil {
		return err
	}
	return i.err
}

func (i *stringIterator) At() string { return i.batch[i.cur] }

func (i *stringIterator) Next() bool {
	if n := i.cur + 1; n < len(i.batch) {
		i.cur = n
		return true
	}
	i.cur = 0
	i.batch = i.batch[:0]
	if !i.Iterator.Next() {
		return false
	}
	var err error
	if i.batch, err = decodeStrings(i.batch, i.Iterator.At().Value); err != nil {
		i.err = err
		return false
	}
	return len(i.batch) > 0
}

func encodeStrings(strings []string) []byte {
	size := 4
	for _, s := range strings {
		size += 4 + len(s)
	}
	data := make([]byte, 0, size)
	data = binary.BigEndian.AppendUint32(data, uint32(len(strings)))
	for _, s := range strings {
		data = binary.BigEndian.AppendUint32(data, uint32(len(s)))
		data = append(data, s...)
	}
	return data
}

func decodeStrings(dst []string, data []byte) ([]string, error) {
	offset := 0
	if len(data) < offset+4 {
		return dst, ErrInvalidStringTable
	}
	n := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	for i := uint32(0); i < n; i++ {
		if len(data) < offset+4 {
			return dst, ErrInvalidStringTable
		}
		size := binary.BigEndian.Uint32(data[offset:])
		offset += 4
		if len(data) < offset+int(size) {
			return dst, ErrInvalidStringTable
		}
		dst = append(dst, string(data[offset:offset+int(size)]))
		offset += int(size)
	}
	return dst, nil
}
