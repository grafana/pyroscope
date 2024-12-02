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
	"github.com/grafana/pyroscope/pkg/experiment/block"
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
	index     *Index
	// Currently, we only check if the dataset name matches
	// the service name label. We could extend this to match
	// any labels of a dataset.
	matcher *labels.Matcher
}

func newMetadataQuery(index *Index, query MetadataQuery) (*metadataQuery, error) {
	if len(query.Tenant) == 0 {
		return nil, &InvalidQueryError{Query: query, Err: fmt.Errorf("tenant_id is required")}
	}
	selectors, err := parser.ParseMetricSelector(query.Expr)
	if err != nil {
		return nil, &InvalidQueryError{Query: query, Err: fmt.Errorf("failed to parse label selectors: %w", err)}
	}
	// TODO: Validate the time range.
	q := &metadataQuery{
		startTime: query.StartTime,
		endTime:   query.EndTime,
		index:     index,
	}
	q.buildTenantList(query.Tenant)
	for _, m := range selectors {
		if m.Name == model.LabelNameServiceName && q.matcher == nil {
			q.matcher = m
			break
		}
	}
	// We could also validate that the service has the profile type
	// queried, but that's not really necessary: querying an irrelevant
	// profile type is rather a rare/invalid case.
	return q, nil
}

func (q *metadataQuery) buildTenantList(tenants []string) {
	m := make(map[string]struct{}, len(tenants)+1)
	for _, t := range tenants {
		m[t] = struct{}{}
	}
	m[""] = struct{}{}
	q.tenants = make([]string, 0, len(m))
	for t := range m {
		q.tenants = append(q.tenants, t)
	}
	sort.Strings(q.tenants)
}

func (q *metadataQuery) iterator(tx *bbolt.Tx) *iterator {
	shards := q.index.shardIterator(tx, q.startTime, q.endTime, q.tenants...)
	si := iterator{
		query:   q,
		tenants: make(map[string]struct{}, len(q.tenants)),
		shards:  shards,
	}
	for _, t := range q.tenants {
		si.tenants[t] = struct{}{}
	}
	return &si
}

type iterator struct {
	query   *metadataQuery
	tenants map[string]struct{}
	shards  iter.Iterator[*indexShard]
	metas   []*metastorev1.BlockMeta
	cur     int
}

func (mi *iterator) Close() error               { return mi.shards.Close() }
func (mi *iterator) Err() error                 { return mi.shards.Err() }
func (mi *iterator) At() *metastorev1.BlockMeta { return mi.metas[mi.cur] }

func (mi *iterator) Next() bool {
	if n := mi.cur + 1; n < len(mi.metas) {
		mi.cur = n
		return true
	}
	mi.cur = 0
	mi.metas = mi.metas[:0]
	for mi.shards.Next() {
		if mi.copyMatched(mi.shards.At()) {
			break
		}
	}
	return len(mi.metas) > 0
}

func (mi *iterator) copyMatched(shard *indexShard) bool {
	for _, md := range shard.blocks {
		if match := mi.metadataMatch(shard.StringTable, md); match != nil {
			mi.metas = append(mi.metas, match)
		}
	}
	slices.SortFunc(mi.metas, func(a, b *metastorev1.BlockMeta) int {
		return strings.Compare(a.Id, b.Id)
	})
	return len(mi.metas) > 0
}

func (mi *iterator) metadataMatch(s *block.MetadataStrings, md *metastorev1.BlockMeta) *metastorev1.BlockMeta {
	if !mi.query.overlaps(time.UnixMilli(md.MinTime), time.UnixMilli(md.MaxTime)) {
		return nil
	}
	var mdCopy *metastorev1.BlockMeta
	datasets := md.Datasets
	for _, ds := range datasets {
		if mi.datasetMatches(s, ds) {
			if mdCopy == nil {
				mdCopy = cloneMetadataForQuery(md)
			}
			mdCopy.Datasets = append(mdCopy.Datasets, ds.CloneVT())
		}
	}
	if mdCopy != nil {
		s.Export(mdCopy)
	}
	// May be nil.
	return mdCopy
}

func (mi *iterator) datasetMatches(s *block.MetadataStrings, ds *metastorev1.Dataset) bool {
	if _, ok := mi.tenants[s.Lookup(ds.Tenant)]; !ok {
		return false
	}
	if !mi.query.overlaps(time.UnixMilli(ds.MinTime), time.UnixMilli(ds.MaxTime)) {
		return false
	}
	// TODO: Cache; we shouldn't check the same name multiple times.
	if mi.query.matcher != nil {
		return mi.query.matcher.Matches(s.Lookup(ds.Name))
	}
	return true
}

func (q *metadataQuery) overlaps(start, end time.Time) bool {
	return start.Before(q.endTime) && !end.Before(q.startTime)
}

func cloneMetadataForQuery(b *metastorev1.BlockMeta) *metastorev1.BlockMeta {
	datasets := b.Datasets
	b.Datasets = nil
	c := b.CloneVT()
	b.Datasets = datasets
	c.Datasets = make([]*metastorev1.Dataset, 0, len(b.Datasets))
	return c
}
