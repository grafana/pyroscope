// Package encoding provides the generic APIs implemented by parquet encodings
// in its sub-packages.
package encoding

import (
	"math"

	"github.com/segmentio/parquet-go/format"
)

const (
	MaxFixedLenByteArraySize = math.MaxInt16
)

// The Encoding interface is implemented by types representing parquet column
// encodings.
//
// Encoding instances must be safe to use concurrently from multiple goroutines.
type Encoding interface {
	// Returns a human-readable name for the encoding.
	String() string

	// Returns the parquet code representing the encoding.
	Encoding() format.Encoding

	// Encode methods serialize the source sequence of values into the
	// destination buffer, potentially reallocating it if it was too short to
	// contain the output.
	//
	// The source are expected to be encoded using the PLAIN encoding, and
	// therefore the methods act as conversions into the target encoding.
	EncodeLevels(dst, src []byte) ([]byte, error)
	EncodeBoolean(dst, src []byte) ([]byte, error)
	EncodeInt32(dst, src []byte) ([]byte, error)
	EncodeInt64(dst, src []byte) ([]byte, error)
	EncodeInt96(dst, src []byte) ([]byte, error)
	EncodeFloat(dst, src []byte) ([]byte, error)
	EncodeDouble(dst, src []byte) ([]byte, error)
	EncodeByteArray(dst, src []byte) ([]byte, error)
	EncodeFixedLenByteArray(dst, src []byte, size int) ([]byte, error)

	// Decode methods deserialize from the source buffer into the destination
	// slice, potentially growing it if it was too short to contain the result.
	//
	// Values are written in the destination buffer in the PLAIN encoding.
	DecodeLevels(dst, src []byte) ([]byte, error)
	DecodeBoolean(dst, src []byte) ([]byte, error)
	DecodeInt32(dst, src []byte) ([]byte, error)
	DecodeInt64(dst, src []byte) ([]byte, error)
	DecodeInt96(dst, src []byte) ([]byte, error)
	DecodeFloat(dst, src []byte) ([]byte, error)
	DecodeDouble(dst, src []byte) ([]byte, error)
	DecodeByteArray(dst, src []byte) ([]byte, error)
	DecodeFixedLenByteArray(dst, src []byte, size int) ([]byte, error)
}
