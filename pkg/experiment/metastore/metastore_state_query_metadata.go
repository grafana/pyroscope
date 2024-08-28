package metastore

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

func (m *Metastore) QueryMetadata(
	ctx context.Context,
	request *metastorev1.QueryMetadataRequest,
) (*metastorev1.QueryMetadataResponse, error) {
	// TODO(kolesnikovae): ReadIndex
	return m.state.listBlocksForQuery(ctx, request)
}

type metadataQuery struct {
	startTime      int64
	endTime        int64
	tenants        map[string]struct{}
	serviceMatcher *labels.Matcher
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

func (q *metadataQuery) matchBlock(b *metastorev1.BlockMeta) bool {
	return inRange(b.MinTime, b.MaxTime, q.startTime, q.endTime)
}

func (q *metadataQuery) matchService(s *metastorev1.Dataset) bool {
	_, ok := q.tenants[s.TenantId]
	if !ok {
		return false
	}
	if !inRange(s.MinTime, s.MaxTime, q.startTime, q.endTime) {
		return false
	}
	if q.serviceMatcher != nil {
		return q.serviceMatcher.Matches(s.Name)
	}
	return true
}

func inRange(blockStart, blockEnd, queryStart, queryEnd int64) bool {
	return blockStart <= queryEnd && blockEnd >= queryStart
}

func (s *metastoreShard) listBlocksForQuery(q *metadataQuery) map[string]*metastorev1.BlockMeta {
	s.segmentsMutex.Lock()
	defer s.segmentsMutex.Unlock()
	md := make(map[string]*metastorev1.BlockMeta, 32)
	for _, segment := range s.segments {
		if !q.matchBlock(segment) {
			continue
		}
		var block *metastorev1.BlockMeta
		for _, svc := range segment.Datasets {
			if q.matchService(svc) {
				if block == nil {
					block = cloneBlockForQuery(segment)
					md[segment.Id] = block
				}
				block.Datasets = append(block.Datasets, svc)
			}
		}
	}
	return md
}

func cloneBlockForQuery(b *metastorev1.BlockMeta) *metastorev1.BlockMeta {
	datasets := b.Datasets
	b.Datasets = nil
	c := b.CloneVT()
	b.Datasets = datasets
	c.Datasets = make([]*metastorev1.Dataset, 0, len(b.Datasets))
	return c
}

func (m *metastoreState) listBlocksForQuery(
	ctx context.Context,
	request *metastorev1.QueryMetadataRequest,
) (*metastorev1.QueryMetadataResponse, error) {
	q, err := newMetadataQuery(request)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	var respMutex sync.Mutex
	var resp metastorev1.QueryMetadataResponse
	g, ctx := errgroup.WithContext(ctx)
	m.shardsMutex.Lock()
	for _, s := range m.shards {
		s := s
		g.Go(func() error {
			blocks := s.listBlocksForQuery(q)
			respMutex.Lock()
			for _, b := range blocks {
				resp.Blocks = append(resp.Blocks, b)
			}
			respMutex.Unlock()
			return nil
		})
	}
	m.shardsMutex.Unlock()
	if err = g.Wait(); err != nil {
		return nil, err
	}
	slices.SortFunc(resp.Blocks, func(a, b *metastorev1.BlockMeta) int {
		return strings.Compare(a.Id, b.Id)
	})
	return &resp, nil
}
