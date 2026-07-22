package index

import (
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/grafana/pyroscope/v2/pkg/metastore/index/indexpb"
)

// blockMetaTimeBounds reads min_time and max_time from an encoded BlockMeta
// without materializing the message, so that queries can discard blocks
// outside their time range before paying for a full decode.
//
// indexpb.BlockMetaTimeBounds is a trimmed view of BlockMeta: it decodes the
// same wire format, and DiscardUnknown skips the remaining fields, including
// datasets, without retaining them.
func blockMetaTimeBounds(b []byte) (minTime, maxTime int64) {
	m := blockMetaBoundsPool.Get().(*indexpb.BlockMetaTimeBounds)
	defer blockMetaBoundsPool.Put(m)
	// A decoding error leaves the bounds at their defaults; such blocks are
	// filtered out by the time-range checks, matching full-decode behavior.
	_ = blockMetaBoundsOptions.Unmarshal(b, m)
	return m.MinTime, m.MaxTime
}

var blockMetaBoundsOptions = proto.UnmarshalOptions{DiscardUnknown: true}

var blockMetaBoundsPool = sync.Pool{
	New: func() any { return new(indexpb.BlockMetaTimeBounds) },
}
