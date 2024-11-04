package compaction

import (
	"fmt"
	"strconv"
	"testing"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func TestBlockIter(t *testing.T) {
	q := newCompactionPlanner(defaultCompactionStrategy)
	for i := 0; i < 1000; i++ {
		q.enqueueBlock(&metastorev1.BlockMeta{
			Id:              strconv.Itoa(i),
			TenantId:        fmt.Sprintf("t-%d", i%2),
			Shard:           uint32(i % 3),
			CompactionLevel: uint32(i % 4),
		})
	}
}
