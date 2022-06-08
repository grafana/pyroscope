package parquet

import (
	"io"

	"github.com/segmentio/parquet-go/bloom"
	"github.com/segmentio/parquet-go/bloom/xxhash"
	"github.com/segmentio/parquet-go/encoding"
	"github.com/segmentio/parquet-go/encoding/plain"
	"github.com/segmentio/parquet-go/format"
	"github.com/segmentio/parquet-go/internal/bits"
)

// BloomFilter is an interface allowing applications to test whether a key
// exists in a bloom filter.
type BloomFilter interface {
	// Implement the io.ReaderAt interface as a mechanism to allow reading the
	// raw bits of the filter.
	io.ReaderAt

	// Returns the size of the bloom filter (in bytes).
	Size() int64

	// Tests whether the given value is present in the filter.
	//
	// A non-nil error may be returned if reading the filter failed. This may
	// happen if the filter was lazily loaded from a storage medium during the
	// call to Check for example. Applications that can guarantee that the
	// filter was in memory at the time Check was called can safely ignore the
	// error, which would always be nil in this case.
	Check(value Value) (bool, error)
}

type bloomFilter struct {
	io.SectionReader
	hash  bloom.Hash
	check func(io.ReaderAt, int64, uint64) (bool, error)
}

func (f *bloomFilter) Check(v Value) (bool, error) {
	return f.check(&f.SectionReader, f.Size(), v.hash(f.hash))
}

func (v Value) hash(h bloom.Hash) uint64 {
	switch v.Kind() {
	case Boolean:
		return h.Sum64Uint8(uint8(v.u64))
	case Int32, Float:
		return h.Sum64Uint32(uint32(v.u64))
	case Int64, Double:
		return h.Sum64Uint64(v.u64)
	case Int96:
		return h.Sum64(v.Bytes())
	default:
		return h.Sum64(v.ByteArray())
	}
}

func newBloomFilter(file io.ReaderAt, offset int64, header *format.BloomFilterHeader) *bloomFilter {
	if header.Algorithm.Block != nil {
		if header.Hash.XxHash != nil {
			if header.Compression.Uncompressed != nil {
				return &bloomFilter{
					SectionReader: *io.NewSectionReader(file, offset, int64(header.NumBytes)),
					hash:          bloom.XXH64{},
					check:         bloom.CheckSplitBlock,
				}
			}
		}
	}
	return nil
}

// The BloomFilterColumn interface is a declarative representation of bloom filters
// used when configuring filters on a parquet writer.
type BloomFilterColumn interface {
	// Returns the path of the column that the filter applies to.
	Path() []string

	// Returns the hashing algorithm used when inserting values into a bloom
	// filter.
	Hash() bloom.Hash

	// Returns an encoding which can be used to write columns of values to the
	// filter.
	Encoding() encoding.Encoding

	// Returns the size of the filter needed to encode values in the filter,
	// assuming each value will be encoded with the given number of bits.
	Size(numValues int64, bitsPerValue uint) int
}

// SplitBlockFilter constructs a split block bloom filter object for the column
// at the given path.
func SplitBlockFilter(path ...string) BloomFilterColumn { return splitBlockFilter(path) }

type splitBlockFilter []string

func (f splitBlockFilter) Path() []string              { return f }
func (f splitBlockFilter) Hash() bloom.Hash            { return bloom.XXH64{} }
func (f splitBlockFilter) Encoding() encoding.Encoding { return splitBlockEncoding{} }
func (f splitBlockFilter) Size(numValues int64, bitsPerValue uint) int {
	return bloom.BlockSize * bloom.NumSplitBlocksOf(numValues, bitsPerValue)
}

// Creates a header from the given bloom filter.
//
// For now there is only one type of filter supported, but we provide this
// function to suggest a model for extending the implementation if new filters
// are added to the parquet specs.
func bloomFilterHeader(filter BloomFilterColumn) (header format.BloomFilterHeader) {
	switch filter.(type) {
	case splitBlockFilter:
		header.Algorithm.Block = &format.SplitBlockAlgorithm{}
	}
	switch filter.Hash().(type) {
	case bloom.XXH64:
		header.Hash.XxHash = &format.XxHash{}
	}
	header.Compression.Uncompressed = &format.BloomFilterUncompressed{}
	return header
}

func searchBloomFilterColumn(filters []BloomFilterColumn, path columnPath) BloomFilterColumn {
	for _, f := range filters {
		if path.equal(f.Path()) {
			return f
		}
	}
	return nil
}

const (
	// Size of the stack buffer used to perform bulk operations on bloom filters.
	//
	// This value was determined as being a good default empirically,
	// 128 x uint64 makes a 1KiB buffer which amortizes the cost of calling
	// methods of bloom filters while not causing too much stack growth either.
	filterEncodeBufferSize = 128
)

type splitBlockEncoding struct {
	encoding.NotSupported
}

func (splitBlockEncoding) EncodeBoolean(dst, src []byte) ([]byte, error) {
	splitBlockEncodeUint8(bloom.MakeSplitBlockFilter(dst), src)
	return dst, nil
}

func (splitBlockEncoding) EncodeInt32(dst, src []byte) ([]byte, error) {
	splitBlockEncodeUint32(bloom.MakeSplitBlockFilter(dst), bits.BytesToUint32(src))
	return dst, nil
}

func (splitBlockEncoding) EncodeInt64(dst, src []byte) ([]byte, error) {
	splitBlockEncodeUint64(bloom.MakeSplitBlockFilter(dst), bits.BytesToUint64(src))
	return dst, nil
}

func (e splitBlockEncoding) EncodeInt96(dst, src []byte) ([]byte, error) {
	splitBlockEncodeFixedLenByteArray(bloom.MakeSplitBlockFilter(dst), src, 12)
	return dst, nil
}

func (splitBlockEncoding) EncodeFloat(dst, src []byte) ([]byte, error) {
	splitBlockEncodeUint32(bloom.MakeSplitBlockFilter(dst), bits.BytesToUint32(src))
	return dst, nil
}

func (splitBlockEncoding) EncodeDouble(dst, src []byte) ([]byte, error) {
	splitBlockEncodeUint64(bloom.MakeSplitBlockFilter(dst), bits.BytesToUint64(src))
	return dst, nil
}

func (splitBlockEncoding) EncodeByteArray(dst, src []byte) ([]byte, error) {
	filter := bloom.MakeSplitBlockFilter(dst)
	buffer := make([]uint64, 0, filterEncodeBufferSize)

	err := plain.RangeByteArrays(src, func(value []byte) error {
		if len(buffer) == cap(buffer) {
			filter.InsertBulk(buffer)
			buffer = buffer[:0]
		}
		buffer = append(buffer, xxhash.Sum64(value))
		return nil
	})

	filter.InsertBulk(buffer)
	return dst, err
}

func (splitBlockEncoding) EncodeFixedLenByteArray(dst, src []byte, size int) ([]byte, error) {
	filter := bloom.MakeSplitBlockFilter(dst)
	if size == 16 {
		splitBlockEncodeUint128(filter, bits.BytesToUint128(src))
	} else {
		splitBlockEncodeFixedLenByteArray(filter, src, size)
	}
	return dst, nil
}

func splitBlockEncodeFixedLenByteArray(filter bloom.SplitBlockFilter, data []byte, size int) {
	buffer := make([]uint64, 0, filterEncodeBufferSize)

	for i, j := 0, size; j <= len(data); {
		if len(buffer) == cap(buffer) {
			filter.InsertBulk(buffer)
			buffer = buffer[:0]
		}
		buffer = append(buffer, xxhash.Sum64(data[i:j]))
		i += size
		j += size
	}

	filter.InsertBulk(buffer)
}

func splitBlockEncodeUint8(filter bloom.SplitBlockFilter, values []uint8) {
	buffer := make([]uint64, filterEncodeBufferSize)

	for i := 0; i < len(values); {
		n := xxhash.MultiSum64Uint8(buffer, values[i:])
		filter.InsertBulk(buffer[:n])
		i += n
	}
}

func splitBlockEncodeUint32(filter bloom.SplitBlockFilter, values []uint32) {
	buffer := make([]uint64, filterEncodeBufferSize)

	for i := 0; i < len(values); {
		n := xxhash.MultiSum64Uint32(buffer, values[i:])
		filter.InsertBulk(buffer[:n])
		i += n
	}
}

func splitBlockEncodeUint64(filter bloom.SplitBlockFilter, values []uint64) {
	buffer := make([]uint64, filterEncodeBufferSize)

	for i := 0; i < len(values); {
		n := xxhash.MultiSum64Uint64(buffer, values[i:])
		filter.InsertBulk(buffer[:n])
		i += n
	}
}

func splitBlockEncodeUint128(filter bloom.SplitBlockFilter, values [][16]byte) {
	buffer := make([]uint64, filterEncodeBufferSize)

	for i := 0; i < len(values); {
		n := xxhash.MultiSum64Uint128(buffer, values[i:])
		filter.InsertBulk(buffer[:n])
		i += n
	}
}
