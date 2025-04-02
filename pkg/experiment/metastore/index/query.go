package index

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block/metadata"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index/store"
	"github.com/grafana/pyroscope/pkg/model"
)

type InvalidQueryError struct {
	Query MetadataQuery
	Err   error
}

func (e *InvalidQueryError) Error() string {
	return fmt.Sprintf("invalid query: %v: %v", e.Query, e.Err)
}

type MetadataQuery struct {
	Expr      string
	StartTime time.Time
	EndTime   time.Time
	Tenant    []string
	Labels    []string
}

func (q *MetadataQuery) String() string {
	return fmt.Sprintf("start: %v, end: %v, tenants: %v, expr: %v",
		q.StartTime,
		q.EndTime,
		strings.Join(q.Tenant, ","),
		q.Expr)
}

type metadataQuery struct {
	startTime time.Time
	endTime   time.Time
	tenants   []string
	tenantMap map[string]struct{}
	matchers  []*labels.Matcher
	labels    []string
	index     *Index
}

func newMetadataQuery(index *Index, query MetadataQuery) (*metadataQuery, error) {
	if len(query.Tenant) == 0 {
		return nil, &InvalidQueryError{Query: query, Err: fmt.Errorf("tenant_id is required")}
	}
	matchers, err := parser.ParseMetricSelector(query.Expr)
	if err != nil {
		return nil, &InvalidQueryError{Query: query, Err: fmt.Errorf("failed to parse label matcher: %w", err)}
	}
	q := &metadataQuery{
		startTime: query.StartTime,
		endTime:   query.EndTime,
		index:     index,
		matchers:  matchers,
		labels:    query.Labels,
	}
	q.buildTenantMap(query.Tenant)
	return q, nil
}

func (q *metadataQuery) buildTenantMap(tenants []string) {
	q.tenantMap = make(map[string]struct{}, len(tenants)+1)
	for _, t := range tenants {
		q.tenantMap[t] = struct{}{}
	}
	// Always query the anonymous blocks: tenant datasets will be filtered out later.
	q.tenantMap[""] = struct{}{}
	q.tenants = make([]string, 0, len(q.tenantMap))
	for t := range q.tenantMap {
		q.tenants = append(q.tenants, t)
	}
	sort.Strings(q.tenants)
}

func (q *metadataQuery) overlaps(start, end time.Time) bool {
	return start.Before(q.endTime) && !end.Before(q.startTime)
}

func (q *metadataQuery) overlapsUnixMilli(start, end int64) bool {
	return q.overlaps(time.UnixMilli(start), time.UnixMilli(end))
}

func newBlockMetadataQuerier(tx *bbolt.Tx, q *metadataQuery) *blockMetadataQuerier {
	return &blockMetadataQuerier{
		query:  q,
		shards: newShardIterator(tx, q.index, q.startTime, q.endTime, q.tenants...),
		metas:  make([]*metastorev1.BlockMeta, 0, 256),
	}
}

type blockMetadataQuerier struct {
	query  *metadataQuery
	shards *shardIterator
	metas  []*metastorev1.BlockMeta
	cur    int
}

func (q *blockMetadataQuerier) queryBlocks() ([]*metastorev1.BlockMeta, error) {
	for q.shards.Next() {
		if err := q.collectBlockMetadata(q.shards.At()); err != nil {
			return nil, err
		}
	}
	slices.SortFunc(q.metas, func(a, b *metastorev1.BlockMeta) int {
		return strings.Compare(a.Id, b.Id)
	})
	return q.metas, q.shards.Err()
}

func (q *blockMetadataQuerier) collectBlockMetadata(s *indexShard) error {
	return s.view(q.shards.tx, func(shard *indexShard) error {
		matcher := metadata.NewLabelMatcher(
			shard.StringTable,
			q.query.matchers,
			q.query.labels...,
		)
		if !matcher.IsValid() {
			return nil
		}
		for _, md := range shard.blocks {
			if m := blockMetadataMatches(q.query, shard.StringTable, matcher, md); m != nil {
				q.metas = append(q.metas, m)
			}
		}
		return nil
	})
}

func blockMetadataMatches(
	q *metadataQuery,
	s *metadata.StringTable,
	m *metadata.LabelMatcher,
	md *metastorev1.BlockMeta,
) *metastorev1.BlockMeta {
	if !q.overlapsUnixMilli(md.MinTime, md.MaxTime) {
		return nil
	}
	var mdCopy *metastorev1.BlockMeta
	var ok bool
	matches := make([]int32, 0, 8)
	for _, ds := range md.Datasets {
		if _, ok := q.tenantMap[s.Lookup(ds.Tenant)]; !ok {
			continue
		}
		if !q.overlapsUnixMilli(ds.MinTime, ds.MaxTime) {
			continue
		}
		matches = matches[:0]
		if matches, ok = m.CollectMatches(matches, ds.Labels); ok {
			if mdCopy == nil {
				mdCopy = cloneBlockMetadataForQuery(md)
			}
			dsCopy := cloneDatasetMetadataForQuery(ds)
			if len(matches) > 0 {
				dsCopy.Labels = make([]int32, len(matches))
				copy(dsCopy.Labels, matches)
			}
			mdCopy.Datasets = append(mdCopy.Datasets, dsCopy)
		}
	}
	if mdCopy != nil {
		s.Export(mdCopy)
	}
	// May be nil.
	return mdCopy
}

func cloneBlockMetadataForQuery(b *metastorev1.BlockMeta) *metastorev1.BlockMeta {
	return &metastorev1.BlockMeta{
		FormatVersion:   b.FormatVersion,
		Id:              b.Id,
		Tenant:          b.Tenant,
		Shard:           b.Shard,
		CompactionLevel: b.CompactionLevel,
		MinTime:         b.MinTime,
		MaxTime:         b.MaxTime,
		CreatedBy:       b.CreatedBy,
		MetadataOffset:  b.MetadataOffset,
		Size:            b.Size,
		//	Datasets:        b.Datasets,
		//	StringTable:     b.StringTable,
	}
}

func cloneDatasetMetadataForQuery(ds *metastorev1.Dataset) *metastorev1.Dataset {
	return &metastorev1.Dataset{
		Format:          ds.Format,
		Tenant:          ds.Tenant,
		Name:            ds.Name,
		MinTime:         ds.MinTime,
		MaxTime:         ds.MaxTime,
		TableOfContents: ds.TableOfContents,
		Size:            ds.Size,
		//	Labels:          ds.Labels,
	}
}

func newMetadataLabelQuerier(tx *bbolt.Tx, q *metadataQuery) *metadataLabelQuerier {
	return &metadataLabelQuerier{
		query:  q,
		shards: newShardIterator(tx, q.index, q.startTime, q.endTime, q.tenants...),
		labels: model.NewLabelMerger(),
	}
}

type metadataLabelQuerier struct {
	query  *metadataQuery
	shards *shardIterator
	labels *model.LabelMerger
}

func (q *metadataLabelQuerier) queryLabels() (*model.LabelMerger, error) {
	if len(q.query.labels) == 0 {
		return q.labels, nil
	}
	for q.shards.Next() {
		if err := q.collectLabels(q.shards.At()); err != nil {
			return nil, err
		}
	}
	return q.labels, q.shards.Err()
}

func (q *metadataLabelQuerier) collectLabels(s *indexShard) error {
	return s.view(q.shards.tx, func(shard *indexShard) error {
		m := metadata.NewLabelMatcher(
			shard.StringTable,
			q.query.matchers,
			q.query.labels...,
		)
		if !m.IsValid() {
			return nil
		}
		for _, md := range shard.blocks {
			if !q.query.overlapsUnixMilli(md.MinTime, md.MaxTime) {
				continue
			}
			for _, ds := range md.Datasets {
				if _, ok := q.query.tenantMap[shard.StringTable.Lookup(ds.Tenant)]; !ok {
					continue
				}
				if !q.query.overlapsUnixMilli(ds.MinTime, ds.MaxTime) {
					continue
				}
				m.Matches(ds.Labels)
			}
		}
		q.labels.MergeLabels(m.AllMatches())
		return nil
	})
}

type shardIterator struct {
	tx         *bbolt.Tx
	index      *Index
	tenants    []string
	partitions []*store.Partition
	shards     []*indexShard
	cur        int
	err        error
}

func newShardIterator(tx *bbolt.Tx, index *Index, startTime, endTime time.Time, tenants ...string) *shardIterator {
	startTime = startTime.Add(-index.config.QueryLookaroundPeriod)
	endTime = endTime.Add(index.config.QueryLookaroundPeriod)
	// We collect matching partitions under a global lock.
	index.global.Lock()
	defer index.global.Unlock()
	si := shardIterator{
		tx:         tx,
		partitions: make([]*store.Partition, 0, len(index.partitions)),
		tenants:    tenants,
		index:      index,
	}
	for _, p := range index.partitions {
		if !p.Overlaps(startTime, endTime) {
			continue
		}
		for _, t := range si.tenants {
			if p.HasTenant(t) {
				si.partitions = append(si.partitions, p)
				break
			}
		}
	}
	return &si
}

func (si *shardIterator) Err() error { return si.err }

func (si *shardIterator) At() *indexShard { return si.shards[si.cur] }

func (si *shardIterator) Next() bool {
	if n := si.cur + 1; n < len(si.shards) {
		si.cur = n
		return true
	}
	si.cur = 0
	si.shards = si.shards[:0]
	for len(si.shards) == 0 && len(si.partitions) > 0 {
		si.loadPartition(si.partitions[0])
		si.partitions = si.partitions[1:]
	}
	return si.cur < len(si.shards)
}

func (si *shardIterator) loadPartition(p *store.Partition) {
	for _, t := range si.tenants {
		si.loadTenantShards(p, t)
	}
	slices.SortFunc(si.shards, compareShards)
	si.shards = slices.Compact(si.shards)
}

func (si *shardIterator) loadTenantShards(p *store.Partition, tenant string) {
	si.index.global.Lock()
	defer si.index.global.Unlock()
	for shard := range p.TenantShards[tenant] {
		s := si.index.getOrCreateIndexShard(p, tenant, shard)
		si.shards = append(si.shards, s)
	}
}

func compareShards(a, b *indexShard) int {
	cmp := strings.Compare(a.Tenant, b.Tenant)
	if cmp == 0 {
		return int(a.Shard) - int(b.Shard)
	}
	return cmp
}
