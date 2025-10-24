package metastore

import (
	"context"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/metastore/index"
	indexstore "github.com/grafana/pyroscope/pkg/metastore/index/store"
	"github.com/grafana/pyroscope/pkg/metastore/tracing"
)

type IndexInserter interface {
	InsertBlock(*bbolt.Tx, *metastorev1.BlockMeta) error
}

type IndexDeleter interface {
	DeleteShard(tx *bbolt.Tx, partition indexstore.Partition, tenant string, shard uint32) error
}

type IndexWriter interface {
	IndexInserter
	IndexDeleter
}

type Tombstones interface {
	AddTombstones(*bbolt.Tx, *raft.Log, *metastorev1.Tombstones) error
	DeleteTombstones(*bbolt.Tx, *raft.Log, ...*metastorev1.Tombstones) error
	Exists(tenant string, shard uint32, block string) bool
}

type IndexCommandHandler struct {
	logger     log.Logger
	index      IndexWriter
	tombstones Tombstones
	compactor  compaction.Compactor
}

func NewIndexCommandHandler(
	logger log.Logger,
	index IndexWriter,
	tombstones Tombstones,
	compactor compaction.Compactor,
) *IndexCommandHandler {
	return &IndexCommandHandler{
		logger:     logger,
		index:      index,
		tombstones: tombstones,
		compactor:  compactor,
	}
}

func (m *IndexCommandHandler) AddBlock(ctx context.Context, tx *bbolt.Tx, cmd *raft.Log, req *metastorev1.AddBlockRequest) (resp *metastorev1.AddBlockResponse, err error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "raft.AddBlockMetadata")
	span.SetTag("block_id", req.Block.GetId())
	span.SetTag("shard", req.Block.GetShard())
	span.SetTag("compaction_level", req.Block.GetCompactionLevel())
	span.SetTag("raft_log_index", cmd.Index)
	span.SetTag("raft_log_term", cmd.Term)
	defer func() {
		if err != nil {
			ext.LogError(span, err)
		}
		span.Finish()
	}()

	e := compaction.NewBlockEntry(cmd, req.Block)
	if m.tombstones.Exists(e.Tenant, e.Shard, e.ID) {
		level.Warn(m.logger).Log("msg", "block already added and compacted", "block", e.ID)
		return new(metastorev1.AddBlockResponse), nil
	}

	insertSpan, _ := tracing.StartSpanFromContext(ctx, "index.InsertBlock")
	if err = m.index.InsertBlock(tx, req.Block); err != nil {
		if errors.Is(err, index.ErrBlockExists) {
			level.Warn(m.logger).Log("msg", "block already added", "block", e.ID)
			return new(metastorev1.AddBlockResponse), nil
		}
		ext.LogError(insertSpan, err)
		insertSpan.Finish()
		level.Error(m.logger).Log("msg", "failed to add block to index", "block", e.ID, "err", err)
		return nil, err
	}
	insertSpan.Finish()

	compactSpan, _ := tracing.StartSpanFromContext(ctx, "compactor.Compact")
	defer compactSpan.Finish()
	if err = m.compactor.Compact(tx, e); err != nil {
		level.Error(m.logger).Log("msg", "failed to add block to compaction", "block", e.ID, "err", err)
		ext.LogError(compactSpan, err)
		return nil, err
	}
	return new(metastorev1.AddBlockResponse), nil
}

func (m *IndexCommandHandler) TruncateIndex(ctx context.Context, tx *bbolt.Tx, cmd *raft.Log, req *raft_log.TruncateIndexRequest) (resp *raft_log.TruncateIndexResponse, err error) {
	span, _ := tracing.StartSpanFromContext(ctx, "raft.TruncateIndex")
	span.SetTag("tombstone_count", len(req.Tombstones))
	span.SetTag("raft_log_index", cmd.Index)
	span.SetTag("raft_log_term", cmd.Term)
	span.SetTag("request_term", req.Term)
	defer func() {
		if err != nil {
			ext.LogError(span, err)
		}
		span.Finish()
	}()

	if req.Term != cmd.Term {
		level.Warn(m.logger).Log(
			"msg", "rejecting index truncation request; term mismatch: leader has changed",
			"current_term", cmd.Term,
			"request_term", req.Term,
		)
		return new(raft_log.TruncateIndexResponse), nil
	}
	for _, tombstone := range req.Tombstones {
		// Although it's not strictly necessary, we may pass any tombstones
		// to TruncateIndex, and the Partition member may be missing.
		if p := tombstone.Shard; p != nil {
			pk := indexstore.Partition{
				Timestamp: time.Unix(0, p.Timestamp),
				Duration:  time.Duration(p.Duration),
			}
			if err = m.index.DeleteShard(tx, pk, p.Tenant, p.Shard); err != nil {
				level.Error(m.logger).Log("msg", "failed to delete partition", "err", err)
				return nil, err
			}
		}
		if err = m.tombstones.AddTombstones(tx, cmd, tombstone); err != nil {
			level.Error(m.logger).Log("msg", "failed to add partition tombstone", "err", err)
			return nil, err
		}
	}
	return new(raft_log.TruncateIndexResponse), nil
}
