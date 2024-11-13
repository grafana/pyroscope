package metastore

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/thanos-io/objstore"
	"go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/markers"
)

type DeletionMarkers interface {
	FindExpiredMarkers(now int64) map[string]*markers.BlockRemovalContext
	Remove(tx *bbolt.Tx, markers map[string]*markers.BlockRemovalContext) error
}

type CleanerCommandHandler struct {
	logger  log.Logger
	bucket  objstore.Bucket
	markers DeletionMarkers

	bucketObjectRemovals *prometheus.CounterVec

	lastRequestId atomic.Pointer[string]
}

func NewCleanerCommandHandler(
	bucket objstore.Bucket,
	logger log.Logger,
	reg prometheus.Registerer,
) *CleanerCommandHandler {
	m := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "pyroscope",
		Subsystem: "metastore",
		Name:      "block_cleaner_bucket_removal_count",
		Help:      "The number of expired blocks that were removed from the bucket",
	}, []string{"tenant", "shard"})
	if reg != nil {
		reg.MustRegister(m)
	}
	return &CleanerCommandHandler{
		logger:               logger,
		bucket:               bucket,
		bucketObjectRemovals: m,
	}
}

func (h *CleanerCommandHandler) ExpectRequest(request string) {
	h.lastRequestId.Store(&request)
}

func (h *CleanerCommandHandler) CleanBlocks(tx *bbolt.Tx, cmd *raft.Log, request *raft_log.CleanBlocksRequest) (*anypb.Any, error) {
	expired := h.markers.FindExpiredMarkers(cmd.AppendedAt.UnixMilli())
	localRequestID := h.lastRequestId.Load()
	level.Info(h.logger).Log(
		"msg", "cleaning expired block deletion markers",
		"count", len(expired),
		"request_id", request.RequestId,
		"stored_request_id", localRequestID,
	)
	cleanBucket := localRequestID != nil && request.RequestId == *localRequestID
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
				level.Debug(h.logger).Log(
					"msg", "removing block from bucket",
					"shard", removalContext.Shard,
					"tenant", removalContext.Tenant,
					"blockId", b,
					"expiryTs", removalContext.ExpiryTs,
					"bucket_key", key)
				err := h.bucket.Delete(grpCtx, key)
				if err != nil {
					level.Warn(h.logger).Log(
						"msg", "failed to remove block from bucket",
						"err", err,
						"blockId", b,
						"shard", removalContext.Shard,
						"tenant", removalContext.Tenant)
					// TODO(aleks-p): Detect if the error is "object does not exist" or something else. Handle each case appropriately.
					return err
				}
				h.bucketObjectRemovals.WithLabelValues(removalContext.Tenant, fmt.Sprint(removalContext.Shard)).Inc()
				cnt.Add(1)
				return nil
			})
		}
		err := g.Wait()
		level.Info(h.logger).Log("msg", "finished bucket cleanup", "blocks_removed", cnt.Load())
		if err != nil {
			return nil, err
		}
		return nil, h.markers.Remove(tx, expired)
	}
	return nil, h.markers.Remove(tx, expired)
}
