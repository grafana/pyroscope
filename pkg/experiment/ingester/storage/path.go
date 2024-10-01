package storage

import (
	"fmt"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/tenant"
	"path"
	"strings"
)

const PathDLQ = "dlq"

const pathSegments = "segments"
const pathAnon = tenant.DefaultTenantID
const pathBlock = "block.bin"
const pathMetaPB = "meta.pb"

func PathForDLQ(meta *metastorev1.BlockMeta) string {
	return path.Join(PathDLQ, fmt.Sprintf("%d", meta.Shard), pathAnon, meta.Id, pathMetaPB)
}

func PathForSegment(meta *metastorev1.BlockMeta) string {
	return path.Join(pathSegments, fmt.Sprintf("%d", meta.Shard), pathAnon, meta.Id, pathBlock)
}

func IsDLQPath(p string) bool {
	fs := strings.Split(p, "/")
	return len(fs) == 5 && fs[0] == PathDLQ && fs[2] == pathAnon && fs[4] == pathMetaPB
}

func IsSegmentPath(p string) bool {
	fs := strings.Split(p, "/")
	return len(fs) == 5 && fs[0] == pathSegments && fs[2] == pathAnon && fs[4] == pathBlock
}
