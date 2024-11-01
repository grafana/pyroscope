package metastore

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/anypb"
)

type CleanBlocksRequestHandler struct {
	logger log.Logger
}

func (m *CleanBlocksRequestHandler) Apply(tx *bbolt.Tx, log *raft.Log, request *raftlogpb.CleanBlocksRequest) (*anypb.Any, error) {
	expired := m.deletionMarkers.FindExpiredMarkers(log.AppendedAt)
	level.Info(m.logger).Log(
		"msg", "cleaning expired block deletion markers",
		"count", len(expired),
		"request_id", request.RequestId,
		"stored_request_id", m.blockCleaner.lastRequestId,
	)
	cleanBucket := m.blockCleaner.lastRequestId == request.RequestId
	if cleanBucket {
		var cnt atomic.Int64
		g, grpCtx := errgroup.WithContext(context.Background())
		for b, removalContext := range expired {
			g.Go(func() error {
				var key string
				if removalContext.Tenant != "" {
					key = filepath.Join("blocks", fmt.Sprint(removalContext.Shard), removalContext.Tenant, b, "block.bin")
				} else {
					key = filepath.Join("segments", fmt.Sprint(removalContext.Shard), "anonymous", b, "block.bin")
				}
				level.Debug(m.logger).Log(
					"msg", "removing block from bucket",
					"shard", removalContext.Shard,
					"tenant", removalContext.Tenant,
					"blockId", b,
					"expiryTs", removalContext.ExpiryTs,
					"bucket_key", key)
				err := m.blockCleaner.bucket.Delete(grpCtx, key)
				if err != nil {
					level.Warn(m.logger).Log(
						"msg", "failed to remove block from bucket",
						"err", err,
						"blockId", b,
						"shard", removalContext.Shard,
						"tenant", removalContext.Tenant)
					// TODO(aleks-p): Detect if the error is "object does not exist" or something else. Handle each case appropriately.
					return err
				}
				m.blockCleaner.bucketObjectRemovals.WithLabelValues(removalContext.Tenant, fmt.Sprint(removalContext.Shard)).Inc()
				cnt.Add(1)
				return nil
			})
		}
		err := g.Wait()
		level.Info(m.logger).Log("msg", "finished bucket cleanup", "blocks_removed", cnt.Load())
		if err != nil {
			return nil, err
		}
		return nil, m.deletionMarkers.Remove(expired)
	}
	return nil, m.deletionMarkers.Remove(expired)
}
