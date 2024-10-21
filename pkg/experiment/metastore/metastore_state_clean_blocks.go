package metastore

import (
	"context"
	"crypto/rand"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/thanos-io/objstore"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/blockcleaner"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftlogpb"
)

type blockCleaner struct {
	cfg *blockcleaner.Config

	raft    *raft.Raft
	raftCfg *RaftConfig

	bucket objstore.Bucket

	wg   sync.WaitGroup
	done chan struct{}

	logger               log.Logger
	bucketObjectRemovals *prometheus.CounterVec

	lastRequestId string
}

func newBlockCleaner(
	cfg *blockcleaner.Config,
	r *raft.Raft,
	raftCfg *RaftConfig,
	bucket objstore.Bucket,
	logger log.Logger,
	reg prometheus.Registerer,
) *blockCleaner {
	m := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "pyroscope",
		Subsystem: "metastore",
		Name:      "block_cleaner_bucket_removal_count",
		Help:      "The number of expired blocks that were removed from the bucket",
	}, []string{"tenant", "shard"})
	if reg != nil {
		reg.MustRegister(m)
	}
	return &blockCleaner{
		cfg:                  cfg,
		raft:                 r,
		raftCfg:              raftCfg,
		bucket:               bucket,
		done:                 make(chan struct{}),
		logger:               logger,
		bucketObjectRemovals: m,
	}
}

func (c *blockCleaner) Start() {
	c.wg.Add(1)
	go c.runLoop()
}

func (c *blockCleaner) runLoop() {
	t := time.NewTicker(c.cfg.CompactedBlocksCleanupInterval)
	defer func() {
		t.Stop()
		c.wg.Done()
	}()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
			if c.raft.State() != raft.Leader {
				continue
			}
			requestId := ulid.MustNew(ulid.Now(), rand.Reader).String()
			c.lastRequestId = requestId
			req := &raftlogpb.CleanBlocksRequest{RequestId: requestId}
			_, _, err := applyCommand[*raftlogpb.CleanBlocksRequest, *anypb.Any](c.raft, req, c.raftCfg.ApplyTimeout)
			if err != nil {
				_ = level.Error(c.logger).Log("msg", "failed to apply clean blocks command", "err", err)
			}
		}
	}
}

func (c *blockCleaner) Stop() {
	close(c.done)
	c.wg.Wait()
}

func (m *metastoreState) applyCleanBlocks(log *raft.Log, request *raftlogpb.CleanBlocksRequest) (*anypb.Any, error) {
	expired := m.deletionMarkers.FindExpiredMarkers(log.AppendedAt.UnixMilli())
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
