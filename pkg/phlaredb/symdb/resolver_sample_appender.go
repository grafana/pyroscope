package symdb

import (
	"slices"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type SampleAppender struct {
	// Max number of elements in the map.
	// Once the limit is exceeded, values
	// are migrated to the chunked set.
	maxMapSize uint32
	hashmap    map[uint32]uint64
	chunkSize  uint32     // Must be a power of 2.
	chunks     [][]uint64 // We could use a sparse set instead.
	size       int

	Append     func(stacktrace uint32, value uint64)
	AppendMany func(stacktraces []uint32, values []uint64)
}

type SampleValues interface {
	SetNodeValues(dst []Node)
}

func NewSampleAppender(maxMapSize, chunkSize uint32) *SampleAppender {
	if chunkSize == 0 || (chunkSize&(chunkSize-1)) != 0 {
		panic("chunk size must be a power of 2")
	}
	s := &SampleAppender{
		chunkSize:  chunkSize,
		maxMapSize: maxMapSize,
		hashmap:    make(map[uint32]uint64, maxMapSize),
	}
	s.Append = s.mapAppend
	s.AppendMany = s.mapAppendMany
	return s
}

func (s *SampleAppender) mapAppend(stacktrace uint32, value uint64) {
	if len(s.hashmap)+1 < int(s.maxMapSize) {
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
	s.size += int((v | -v) >> 63) // Skip zero.
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
		s.size += int((v | -v) >> 63) // Skip zero.
	}
}

func (s *SampleAppender) Samples() v1.Samples {
	if len(s.hashmap) > 0 {
		return v1.NewSamplesFromMap(s.hashmap)
	}
	samples := v1.NewSamples(int(s.size) + 1)
	chunks := uint32(len(s.chunks))
	var x uint32
	for i := uint32(0); i < chunks; i++ {
		values := uint32(len(s.chunks[i]))
		for j := uint32(0); j < values; j++ {
			if v := s.chunks[i][j]; v != 0 {
				x++
				samples.StacktraceIDs[x] = i*s.chunkSize + j
				samples.Values[x] = v
			}
		}
	}
	return samples
}

type stacktraceIDRange struct {
	offset int
	ids    []uint32
	v1.Samples
}

func (s stacktraceIDRange) SetNodeValues(dst []Node) {
	for i := s.offset; i < len(s.ids); i++ {
		x := s.StacktraceIDs[i]
		v := int64(s.Values[i])
		if x > 0 && v > 0 {
			dst[x].Value = v
		}
	}
}
