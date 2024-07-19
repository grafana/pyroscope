package metastore

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore/compactionpb"
)

func (m *Metastore) AddBlock(_ context.Context, req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	t1 := time.Now()
	defer func() {
		m.metrics.raftAddBlockDuration.Observe(time.Since(t1).Seconds())
	}()
	_, resp, err := applyCommand[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse](m.raft, req, m.config.Raft.ApplyTimeout)
	return resp, err
}

func (m *metastoreState) applyAddBlock(_ *raft.Log, request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	_ = level.Info(m.logger).Log("msg", "adding block", "block_id", request.Block.Id)

	name, key := keyForBlockMeta(request.Block.Shard, "", request.Block.Id)
	value, err := request.Block.MarshalVT()
	if err != nil {
		return nil, err
	}

	var jobToAdd *compactionpb.CompactionJob
	var blockToAddToQueue *metastorev1.BlockMeta

	err = m.db.boltdb.Update(func(tx *bbolt.Tx) error {
		err := updateBlockMetadataBucket(tx, name, func(bucket *bbolt.Bucket) error {
			return bucket.Put(key, value)
		})
		if err != nil {
			return err
		}
		// create and store an optional compaction job
		if job := m.tryCreateJob(request.Block); job != nil {
			level.Debug(m.logger).Log("msg", "persisting compaction job", "job", job.Name)
			jobBucketName, jobKey := keyForCompactionJob(request.Block.Shard, request.Block.TenantId, job.Name)
			err := updateCompactionJobBucket(tx, jobBucketName, func(bucket *bbolt.Bucket) error {
				data, _ := job.MarshalVT()
				return bucket.Put(jobKey, data)
			})
			if err != nil {
				return err
			}
			jobToAdd = job
		} else {
			blockToAddToQueue = request.Block
		}
		return nil
	})
	if err != nil {
		_ = level.Error(m.logger).Log(
			"msg", "failed to add block",
			"block", request.Block.Id,
			"err", err,
		)
		return nil, err
	}
	m.getOrCreateShard(request.Block.Shard).putSegment(request.Block)
	if jobToAdd != nil {
		m.addCompactionJob(jobToAdd)
		m.compactionMetrics.addedBlocks.WithLabelValues(
			fmt.Sprint(jobToAdd.Shard), jobToAdd.TenantId, fmt.Sprint(jobToAdd.CompactionLevel)).Inc()
		m.compactionMetrics.addedJobs.WithLabelValues(
			fmt.Sprint(jobToAdd.Shard), jobToAdd.TenantId, fmt.Sprint(jobToAdd.CompactionLevel)).Inc()
	} else if blockToAddToQueue != nil {
		m.addBlockToCompactionJobQueue(blockToAddToQueue)
		m.compactionMetrics.addedBlocks.WithLabelValues(
			fmt.Sprint(blockToAddToQueue.Shard), blockToAddToQueue.TenantId, fmt.Sprint(blockToAddToQueue.CompactionLevel)).Inc()
	}
	return &metastorev1.AddBlockResponse{}, nil
}
