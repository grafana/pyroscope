package streaming

import (
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/arenahelper"
)


var (
	profileIDLabel = []byte(segment.ProfileIDLabelName)
)

type valueType struct {
	Type int64
	unit int64

	resolvedType string
	resolvedUnit string
}
type function struct {
	id       uint64
	name     int32
	filename int32
}

type location struct {
	id uint64
	// packed from << 32 | to into values
	linesRef uint64
}

type line struct {
	functionID uint64
	line       int64
}

// from,to into profile buffer
type istr uint64

type sample struct {
	tmpValues []int64
	// k<<32|v
	//type labelPacked uint64
	tmpLabels   []uint64
	tmpStack    [][]byte
	tmpStackLoc []uint64
	//todo rename - remove tmp prefix
}

func (s *sample) reset(a arenahelper.ArenaWrapper) {
	// 64 is max pc for golang + speculative number of inlines
	if s.tmpStack == nil {
		s.tmpStack = arenahelper.MakeSlice[[]byte](a, 0, 64+8)
		s.tmpStackLoc = arenahelper.MakeSlice[uint64](a, 0, 64+8)
		s.tmpValues = arenahelper.MakeSlice[int64](a, 0, 4)
		s.tmpLabels = arenahelper.MakeSlice[uint64](a, 0, 4)
	} else {
		s.tmpStack = s.tmpStack[:0]
		s.tmpStackLoc = s.tmpStackLoc[:0]
		s.tmpValues = s.tmpValues[:0]
		s.tmpLabels = s.tmpLabels[:0]
	}
}
