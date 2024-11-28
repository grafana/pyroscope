package metastore

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode"
	"github.com/grafana/pyroscope/pkg/model"
)

type IndexQuerier interface {
	FindBlocks(tx *bbolt.Tx, list *metastorev1.BlockList) []*metastorev1.BlockMeta
	FindBlocksInRange(tx *bbolt.Tx, start, end int64, tenants map[string]struct{}) []*metastorev1.BlockMeta
	ForEachPartition(ctx context.Context, f func(*index.PartitionMeta) error) error
}

func NewMetadataQueryService(
	logger log.Logger,
	state State,
	index IndexQuerier,
) *MetadataQueryService {
	return &MetadataQueryService{
		logger: logger,
		state:  state,
		index:  index,
	}
}

type MetadataQueryService struct {
	metastorev1.MetadataQueryServiceServer

	logger log.Logger
	state  State
	index  IndexQuerier
}

func (svc *MetadataQueryService) QueryMetadata(
	ctx context.Context,
	req *metastorev1.QueryMetadataRequest,
) (resp *metastorev1.QueryMetadataResponse, err error) {
	read := func(tx *bbolt.Tx, _ raftnode.ReadIndex) {
		// NOTE(kolesnikovae): that there's a little chance that we read
		// applied changes not yet committed by the quorum, because we
		// ignore the read index. This is fine in 99.(9)% of cases.
		// In the future we should ensure isolation that ensure that we
		// do not access the state beyond the read index.
		resp, err = svc.listBlocksForQuery(ctx, tx, req)
	}
	if readErr := svc.state.ConsistentRead(ctx, read); readErr != nil {
		return nil, status.Error(codes.Unavailable, readErr.Error())
	}
	return resp, err
}

func (svc *MetadataQueryService) listBlocksForQuery(
	_ context.Context, // TODO(kolesnikovae): Handle cancellation.
	tx *bbolt.Tx,
	req *metastorev1.QueryMetadataRequest,
) (*metastorev1.QueryMetadataResponse, error) {
	q, err := newMetadataQuery(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	var resp metastorev1.QueryMetadataResponse
	md := make(map[string]*metastorev1.BlockMeta, 32)

	blocks := svc.index.FindBlocksInRange(tx, q.startTime, q.endTime, q.tenants)
	if err != nil {
		level.Error(svc.logger).Log("msg", "failed to list metastore blocks", "query", q, "err", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	for _, b := range blocks {
		var clone *metastorev1.BlockMeta
		for _, ds := range b.Datasets {
			if q.matchService(ds) {
				if clone == nil {
					clone = cloneBlockForQuery(b)
					md[block.ID(clone)] = clone
				}
				clone.Datasets = append(clone.Datasets, ds)
			}
		}
	}

	resp.Blocks = make([]*metastorev1.BlockMeta, 0, len(md))
	for _, block := range md {
		resp.Blocks = append(resp.Blocks, block)
	}
	slices.SortFunc(resp.Blocks, func(a, b *metastorev1.BlockMeta) int {
		return strings.Compare(a.Id, b.Id)
	})
	return &resp, nil
}

type metadataQuery struct {
	startTime      int64
	endTime        int64
	tenants        map[string]struct{}
	serviceMatcher *labels.Matcher
}

func (q *metadataQuery) String() string {
	return fmt.Sprintf("start: %d, end: %d, tenants: %v, serviceMatcher: %v", q.startTime, q.endTime, q.tenants, q.serviceMatcher)
}

func newMetadataQuery(request *metastorev1.QueryMetadataRequest) (*metadataQuery, error) {
	if len(request.TenantId) == 0 {
		return nil, fmt.Errorf("tenant_id is required")
	}
	q := &metadataQuery{
		startTime: request.StartTime,
		endTime:   request.EndTime,
		tenants:   make(map[string]struct{}, len(request.TenantId)),
	}
	for _, tenant := range request.TenantId {
		q.tenants[tenant] = struct{}{}
	}
	selectors, err := parser.ParseMetricSelector(request.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse label selectors: %w", err)
	}
	for _, m := range selectors {
		if m.Name == model.LabelNameServiceName {
			q.serviceMatcher = m
			break
		}
	}
	// We could also validate that the service has the profile type
	// queried, but that's not really necessary: querying an irrelevant
	// profile type is rather a rare/invalid case.
	return q, nil
}

func (q *metadataQuery) matchService(md *metastorev1.BlockMeta, ds *metastorev1.Dataset) bool {
	_, ok := q.tenants[ds.TenantId]
	if !ok {
		return false
	}
	if !inRange(ds.MinTime, ds.MaxTime, q.startTime, q.endTime) {
		return false
	}
	if q.serviceMatcher != nil {
		return q.serviceMatcher.Matches(ds.Name)
	}
	return true
}

func inRange(blockStart, blockEnd, queryStart, queryEnd int64) bool {
	return blockStart <= queryEnd && blockEnd >= queryStart
}

func cloneBlockForQuery(b *metastorev1.BlockMeta) *metastorev1.BlockMeta {
	datasets := b.Datasets
	b.Datasets = nil
	c := b.CloneVT()
	b.Datasets = datasets
	c.Datasets = make([]*metastorev1.Dataset, 0, len(b.Datasets))
	return c
}
