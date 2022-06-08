package rle

import (
	"github.com/segmentio/parquet-go/encoding"
	"github.com/segmentio/parquet-go/format"
	"github.com/segmentio/parquet-go/internal/bits"
)

type DictionaryEncoding struct {
	encoding.NotSupported
}

func (e *DictionaryEncoding) String() string {
	return "RLE_DICTIONARY"
}

func (e *DictionaryEncoding) Encoding() format.Encoding {
	return format.RLEDictionary
}

func (e *DictionaryEncoding) EncodeInt32(dst, src []byte) ([]byte, error) {
	if (len(src) % 4) != 0 {
		return dst[:0], encoding.ErrEncodeInvalidInputSize(e, "INT32", len(src))
	}
	src32 := bits.BytesToInt32(src)
	bitWidth := bits.MaxLen32(src32)
	dst = append(dst[:0], byte(bitWidth))
	dst, err := encodeInt32(dst, src32, uint(bitWidth))
	return dst, e.wrap(err)
}

func (e *DictionaryEncoding) DecodeInt32(dst, src []byte) ([]byte, error) {
	if len(src) == 0 {
		return dst[:0], nil
	}
	dst, err := decodeInt32(dst[:0], src[1:], uint(src[0]))
	return dst, e.wrap(err)
}

func (e *DictionaryEncoding) wrap(err error) error {
	if err != nil {
		err = encoding.Error(e, err)
	}
	return err
}

func clearInt32(data []int32) {
	for i := range data {
		data[i] = 0
	}
}
