package symdb

import (
	"slices"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

// SampleAppender is a dynamic data structure that accumulates
// samples, by summing them up by stack trace ID.
//
// It has two underlying implementations:
//   - map: a hash table is used for small sparse data sets (16k by default).
//     This representation is optimal for small profiles, like span profile,
//     or a short time range profile of a specific service/series.
//   - chunked sparse set: stack trace IDs serve as indices in a sparse set.
//     Provided that the stack trace IDs are dense (as they point to the node
//     index in the parent pointer tree), this representation is significantly
//     more performant, but may require more space, if the stack trace IDs set
//     is very sparse. In order to reduce memory consumption, the set is split
//     into chunks (16k by default), that are allocated once at least one ID
//     matches the chunk range. In addition, values are ordered by stack trace
//     ID without being sorted explicitly.
type SampleAppender struct {
	// Max number of elements in the map.
	// Once the limit is exceeded, values
	// are migrated to the chunked set.
	maxMapSize uint32
	hashmap    map[uint32]uint64
	chunkSize  uint32 // Must be a power of 2.
	chunks     [][]uint64
	size       int

	Append     func(stacktrace uint32, value uint64)
	AppendMany func(stacktraces []uint32, values []uint64)
}

// Hashmap is used for small data sets (<= 16k elements, be default).
// Once the limit is exceeded, the data is migrated to the chunked set.
// Chunk size is 16k (128KiB) by default.
const (
	defaultSampleAppenderSize = 16 << 10
	defaultChunkSize          = 16 << 10
)

func NewSampleAppender() *SampleAppender {
	return NewSampleAppenderSize(defaultSampleAppenderSize, defaultChunkSize)
}

func NewSampleAppenderSize(maxMapSize, chunkSize uint32) *SampleAppender {
	if chunkSize == 0 || (chunkSize&(chunkSize-1)) != 0 {
		panic("chunk size must be a power of 2")
	}
	s := &SampleAppender{
		chunkSize:  chunkSize,
		maxMapSize: maxMapSize,
		hashmap:    make(map[uint32]uint64),
	}
	s.Append = s.mapAppend
	s.AppendMany = s.mapAppendMany
	return s
}

func (s *SampleAppender) mapAppend(stacktrace uint32, value uint64) {
	if len(s.hashmap) < int(s.maxMapSize) {
		s.hashmap[stacktrace] += value
		return
	}
	s.migrate()
	s.Append(stacktrace, value)
}

func (s *SampleAppender) mapAppendMany(stacktraces []uint32, values []uint64) {
	if len(s.hashmap)+len(stacktraces) < int(s.maxMapSize) {
		for i, stacktrace := range stacktraces {
			if v := values[i]; v != 0 && stacktrace != 0 {
				s.hashmap[stacktrace] += v
			}
		}
		return
	}
	s.migrate()
	s.AppendMany(stacktraces, values)
}

func (s *SampleAppender) migrate() {
	s.Append = s.setAppend
	s.AppendMany = s.setAppendMany
	for k, v := range s.hashmap {
		s.Append(k, v)
	}
	s.hashmap = nil
}

func (s *SampleAppender) setAppend(stacktrace uint32, value uint64) {
	if value == 0 || stacktrace == 0 {
		return
	}
	ci := stacktrace / s.chunkSize
	vi := stacktrace & (s.chunkSize - 1) // stacktrace % s.chunkSize
	if x := int(ci) + 1; x > len(s.chunks) {
		s.chunks = slices.Grow(s.chunks, x)
		s.chunks = s.chunks[:x]
	}
	c := s.chunks[ci]
	if cap(c) == 0 {
		c = make([]uint64, s.chunkSize)
		s.chunks[ci] = c
	}
	v := c[vi]
	c[vi] += value
	if v == 0 {
		s.size++
	}
}

func (s *SampleAppender) setAppendMany(stacktraces []uint32, values []uint64) {
	// Inlined Append.
	for i, stacktrace := range stacktraces {
		value := values[i]
		if value == 0 || stacktrace == 0 {
			continue
		}
		ci := stacktrace / s.chunkSize
		vi := stacktrace & (s.chunkSize - 1) // stacktrace % s.chunkSize
		if x := int(ci) + 1; x > len(s.chunks) {
			s.chunks = slices.Grow(s.chunks, x)
			s.chunks = s.chunks[:x]
		}
		c := s.chunks[ci]
		if cap(c) == 0 {
			c = make([]uint64, s.chunkSize)
			s.chunks[ci] = c
		}
		v := c[vi]
		c[vi] += value
		if v == 0 {
			s.size++
		}
	}
}

func (s *SampleAppender) Len() int { return s.size + len(s.hashmap) }

func (s *SampleAppender) Samples() v1.Samples {
	if len(s.hashmap) > 0 {
		return v1.NewSamplesFromMap(s.hashmap)
	}
	samples := v1.NewSamples(s.Len())
	chunks := uint32(len(s.chunks))
	x := 0
	for i := uint32(0); i < chunks; i++ {
		values := uint32(len(s.chunks[i]))
		for j := uint32(0); j < values; j++ {
			if v := s.chunks[i][j]; v != 0 {
				if sid := i*s.chunkSize + j; sid > 0 {
					samples.StacktraceIDs[x] = sid
					samples.Values[x] = v
				}
				x++
			}
		}
	}
	return samples
}
