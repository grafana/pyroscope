package store

import (
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

// index:appended_at     *metastorev1.Tombstones

type TombstoneEntry struct {
	Index      uint64
	AppendedAt int64
	*metastorev1.Tombstones
}
