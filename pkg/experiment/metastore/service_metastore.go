package metastore

import (
	"context"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	adaptiveplacement "github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	"github.com/grafana/pyroscope/pkg/iter"
)

type DistributionStats interface {
	RecordStats(iter.Iterator[adaptiveplacement.Sample])
}

type AddBlockCommandLog interface {
	ProposeAddBlock(*metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error)
}

func NewMetastoreService(
	logger log.Logger,
	stats DistributionStats,
	raftLog AddBlockCommandLog,
) *MetastoreService {
	return &MetastoreService{
		logger:  logger,
		stats:   stats,
		raftLog: raftLog,
	}
}

type MetastoreService struct {
	logger  log.Logger
	stats   DistributionStats
	raftLog AddBlockCommandLog
}

func (svc *MetastoreService) AddBlock(
	_ context.Context,
	req *metastorev1.AddBlockRequest,
) (*metastorev1.AddBlockResponse, error) {
	// TODO(kolesnikovae): Validate input.
	logger := log.With(svc.logger, "shard", req.Block.Shard, "block_id", req.Block.Id, "ts", req.Block.MinTime)
	_, err := ulid.Parse(req.Block.Id)
	if err != nil {
		_ = level.Warn(logger).Log("failed to parse block id", "err", err)
		return nil, err
	}

	defer func() {
		if err == nil {
			svc.stats.RecordStats(statSamplesFromMeta(req.Block))
		}
	}()

	resp, err := svc.raftLog.ProposeAddBlock(req)
	if err != nil {
		_ = level.Error(logger).Log("msg", "failed to add block", "err", err)
	}

	return resp, err
}

func statSamplesFromMeta(md *metastorev1.BlockMeta) iter.Iterator[adaptiveplacement.Sample] {
	return &sampleIterator{md: md}
}

type sampleIterator struct {
	md  *metastorev1.BlockMeta
	cur int
}

func (s *sampleIterator) Err() error   { return nil }
func (s *sampleIterator) Close() error { return nil }

func (s *sampleIterator) Next() bool {
	if s.cur >= len(s.md.Datasets) {
		return false
	}
	s.cur++
	return true
}

func (s *sampleIterator) At() adaptiveplacement.Sample {
	ds := s.md.Datasets[s.cur-1]
	return adaptiveplacement.Sample{
		TenantID:    ds.TenantId,
		DatasetName: ds.Name,
		ShardOwner:  s.md.CreatedBy,
		ShardID:     s.md.Shard,
		Size:        ds.Size,
	}
}
