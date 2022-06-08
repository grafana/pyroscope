package plain

import (
	"github.com/segmentio/parquet-go/encoding"
	"github.com/segmentio/parquet-go/format"
)

type DictionaryEncoding struct {
	encoding.NotSupported
	plain Encoding
}

func (e *DictionaryEncoding) String() string {
	return "PLAIN_DICTIONARY"
}

func (e *DictionaryEncoding) Encoding() format.Encoding {
	return format.PlainDictionary
}

func (e *DictionaryEncoding) EncodeInt32(dst, src []byte) ([]byte, error) {
	return e.plain.EncodeInt32(dst, src)
}

func (e *DictionaryEncoding) DecodeInt32(dst, src []byte) ([]byte, error) {
	return e.plain.DecodeInt32(dst, src)
}
