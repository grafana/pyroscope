package index

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	"github.com/grafana/pyroscope/pkg/metastore/index/store"
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
	if q.startTime.After(end) {
		return false
	}
	if q.endTime.Before(start) {
		return false
	}
	return true
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
}

func (q *blockMetadataQuerier) queryBlocks(ctx context.Context) ([]*metastorev1.BlockMeta, error) {
	for q.shards.Next() && ctx.Err() == nil {
		shard := q.shards.At()
		offset := len(q.metas)
		if err := q.collectBlockMetadata(shard); err != nil {
			return nil, err
		}
		slices.SortFunc(q.metas[offset:], func(a, b *metastorev1.BlockMeta) int {
			return strings.Compare(a.Id, b.Id)
		})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return q.metas, q.shards.Err()
}

func (q *blockMetadataQuerier) collectBlockMetadata(s *store.Shard) error {
	matcher := metadata.NewLabelMatcher(s.StringTable.Strings, q.query.matchers, q.query.labels...)
	if !matcher.IsValid() {
		return nil
	}
	blocks := s.Blocks(q.shards.tx)
	if blocks == nil {
		return nil
	}
	for blocks.Next() {
		md := q.shards.index.blocks.getOrCreate(s, blocks.At())
		if m := q.collectMatched(s.StringTable, matcher, md); m != nil {
			q.metas = append(q.metas, m)
		}
	}
	return nil
}

func (q *blockMetadataQuerier) collectMatched(
	s *metadata.StringTable,
	m *metadata.LabelMatcher,
	md *metastorev1.BlockMeta,
) *metastorev1.BlockMeta {
	if !q.query.overlapsUnixMilli(md.MinTime, md.MaxTime) {
		return nil
	}
	var mdCopy *metastorev1.BlockMeta
	var ok bool
	matches := make([]int32, 0, 8)
	for _, ds := range md.Datasets {
		if _, ok := q.query.tenantMap[s.Lookup(ds.Tenant)]; !ok {
			continue
		}
		if !q.query.overlapsUnixMilli(ds.MinTime, ds.MaxTime) {
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
		labels: metadata.NewLabelsCollector(q.labels...),
	}
}

type metadataLabelQuerier struct {
	query  *metadataQuery
	shards *shardIterator
	labels *metadata.LabelsCollector
}

func (q *metadataLabelQuerier) queryLabels(ctx context.Context) (*metadata.LabelsCollector, error) {
	if len(q.query.labels) == 0 {
		return q.labels, nil
	}
	for q.shards.Next() && ctx.Err() == nil {
		shard := q.shards.At()
		if err := q.collectLabels(shard); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return q.labels, q.shards.Err()
}

func (q *metadataLabelQuerier) collectLabels(s *store.Shard) error {
	matcher := metadata.NewLabelMatcher(s.StringTable.Strings, q.query.matchers, q.query.labels...)
	if !matcher.IsValid() {
		return nil
	}
	blocks := s.Blocks(q.shards.tx)
	if blocks == nil {
		return nil
	}
	for blocks.Next() {
		md := q.shards.index.blocks.getOrCreate(s, blocks.At())
		if q.query.overlapsUnixMilli(md.MinTime, md.MaxTime) {
			for _, ds := range md.Datasets {
				if _, ok := q.query.tenantMap[s.StringTable.Lookup(ds.Tenant)]; !ok {
					continue
				}
				if q.query.overlapsUnixMilli(ds.MinTime, ds.MaxTime) {
					matcher.Matches(ds.Labels)
				}
			}
		}
	}
	q.labels.CollectMatches(matcher)
	return nil
}

type shardIterator struct {
	tx        *bbolt.Tx
	index     *Index
	tenants   []string
	shards    []store.Shard
	cur       *store.Shard
	err       error
	startTime time.Time
	endTime   time.Time
}

func newShardIterator(tx *bbolt.Tx, index *Index, startTime, endTime time.Time, tenants ...string) *shardIterator {
	// See comment in DefaultConfig.queryLookaroundPeriod.
	startTime = startTime.Add(-index.config.queryLookaroundPeriod)
	endTime = endTime.Add(index.config.queryLookaroundPeriod)
	si := shardIterator{
		tx:        tx,
		tenants:   tenants,
		index:     index,
		startTime: startTime,
		endTime:   endTime,
	}
	for p := range index.store.Partitions(tx) {
		if !p.Overlaps(startTime, endTime) {
			continue
		}
		q := p.Query(tx)
		if q == nil {
			continue
		}
		for _, t := range si.tenants {
			for s := range q.Shards(t) {
				if s.ShardIndex.Overlaps(si.startTime, si.endTime) {
					si.shards = append(si.shards, s)
				}
			}
		}
	}
	slices.SortFunc(si.shards, compareShards)
	si.shards = slices.Compact(si.shards)
	return &si
}

func (si *shardIterator) Err() error { return si.err }

func (si *shardIterator) At() *store.Shard { return si.cur }

func (si *shardIterator) Next() bool {
	if si.err != nil || len(si.shards) == 0 {
		return false
	}
	c := si.shards[0]
	si.shards = si.shards[1:]
	si.cur, si.err = si.index.shards.getForRead(si.tx, c.Partition, c.Tenant, c.Shard)
	return si.err == nil
}

func compareShards(a, b store.Shard) int {
	cmp := strings.Compare(a.Tenant, b.Tenant)
	if cmp == 0 {
		return int(a.Shard) - int(b.Shard)
	}
	return cmp
}
