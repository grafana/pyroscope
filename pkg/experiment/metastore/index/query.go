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
	"github.com/grafana/pyroscope/pkg/iter"
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

func newBlockMetadataIterator(tx *bbolt.Tx, q *metadataQuery) *blockMetadataIterator {
	return &blockMetadataIterator{
		query:  q,
		shards: newShardIterator(tx, q.index, q.startTime, q.endTime, q.tenants...),
		metas:  make([]*metastorev1.BlockMeta, 0, 256),
	}
}

type blockMetadataIterator struct {
	query  *metadataQuery
	shards iter.Iterator[*indexShard]
	metas  []*metastorev1.BlockMeta
	cur    int
}

func (mi *blockMetadataIterator) Close() error               { return mi.shards.Close() }
func (mi *blockMetadataIterator) Err() error                 { return mi.shards.Err() }
func (mi *blockMetadataIterator) At() *metastorev1.BlockMeta { return mi.metas[mi.cur] }

func (mi *blockMetadataIterator) Next() bool {
	if n := mi.cur + 1; n < len(mi.metas) {
		mi.cur = n
		return true
	}
	mi.cur = 0
	mi.metas = mi.metas[:0]
	for mi.shards.Next() {
		if mi.collectBlockMetadata(mi.shards.At()) {
			break
		}
	}
	return len(mi.metas) > 0
}

func (mi *blockMetadataIterator) collectBlockMetadata(shard *indexShard) bool {
	matcher := metadata.NewLabelMatcher(
		shard.StringTable,
		mi.query.matchers,
		mi.query.labels...,
	)
	if !matcher.IsValid() {
		return false
	}
	for _, md := range shard.blocks {
		if m := blockMetadataMatches(mi.query, shard.StringTable, matcher, md); m != nil {
			mi.metas = append(mi.metas, m)
		}
	}
	slices.SortFunc(mi.metas, func(a, b *metastorev1.BlockMeta) int {
		return strings.Compare(a.Id, b.Id)
	})
	return len(mi.metas) > 0
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
	datasets := b.Datasets
	b.Datasets = nil
	c := b.CloneVT()
	b.Datasets = datasets
	c.Datasets = make([]*metastorev1.Dataset, 0, 8)
	return c
}

func cloneDatasetMetadataForQuery(ds *metastorev1.Dataset) *metastorev1.Dataset {
	ls := ds.Labels
	ds.Labels = nil
	c := ds.CloneVT()
	ds.Labels = ls
	return c
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
	shards iter.Iterator[*indexShard]
	labels *model.LabelMerger
}

func (mi *metadataLabelQuerier) queryLabels() (*model.LabelMerger, error) {
	if len(mi.query.labels) == 0 {
		return mi.labels, nil
	}
	for mi.shards.Next() {
		mi.collectLabels(mi.shards.At())
	}
	if err := mi.shards.Err(); err != nil {
		return nil, err
	}
	return mi.labels, nil
}

func (mi *metadataLabelQuerier) collectLabels(shard *indexShard) {
	m := metadata.NewLabelMatcher(
		shard.StringTable,
		mi.query.matchers,
		mi.query.labels...,
	)
	if !m.IsValid() {
		return
	}
	for _, md := range shard.blocks {
		if !mi.query.overlapsUnixMilli(md.MinTime, md.MaxTime) {
			continue
		}
		for _, ds := range md.Datasets {
			if _, ok := mi.query.tenantMap[shard.StringTable.Lookup(ds.Tenant)]; !ok {
				continue
			}
			if !mi.query.overlapsUnixMilli(ds.MinTime, ds.MaxTime) {
				continue
			}
			m.Matches(ds.Labels)
		}
	}
	mi.labels.MergeLabels(m.AllMatches())
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

func newShardIterator(tx *bbolt.Tx, index *Index, startTime, endTime time.Time, tenants ...string) iter.Iterator[*indexShard] {
	startTime = startTime.Add(-index.config.QueryLookaroundPeriod)
	endTime = endTime.Add(index.config.QueryLookaroundPeriod)
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

func (si *shardIterator) Close() error { return nil }

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
		si.loadShards(si.partitions[0])
		si.partitions = si.partitions[1:]
	}
	return si.cur < len(si.shards)
}

func (si *shardIterator) loadShards(p *store.Partition) {
	for _, t := range si.tenants {
		shards := p.TenantShards[t]
		if shards == nil {
			continue
		}
		for s := range shards {
			shard, err := si.index.getOrLoadTenantShard(si.tx, p, t, s)
			if err != nil {
				si.err = err
				return
			}
			if shard != nil {
				si.shards = append(si.shards, shard)
			}
		}
	}
	slices.SortFunc(si.shards, compareShards)
	si.shards = slices.Compact(si.shards)
}

func compareShards(a, b *indexShard) int {
	cmp := strings.Compare(a.Tenant, b.Tenant)
	if cmp == 0 {
		return int(a.Shard) - int(b.Shard)
	}
	return cmp
}
