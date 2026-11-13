package querybackend

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/parquet-go/parquet-go"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/model"
	pq "github.com/grafana/pyroscope/v2/pkg/phlaredb/query"
)

const spanColumn = "Samples.list.element.SpanID"
const traceColumn = "Samples.list.element.TraceID"
const valueColumn = "Samples.list.element.Value"

func init() {
	registerQueryType(queryv1.QueryType_QUERY_SPAN_PRESENCE, queryv1.ReportType_REPORT_SPAN_PRESENCE, querySpanPresence, newSpanPresenceAggregator, false, block.SectionTSDB, block.SectionProfiles)
}

// spanPresencePredicate checks dictionaries before decoding any sample IDs.
type spanPresencePredicate struct{ spans model.SpanSelector }

func (p spanPresencePredicate) KeepColumnChunk(parquet.ColumnIndex) bool { return true }
func (p spanPresencePredicate) KeepValue(v parquet.Value) bool {
	if v.IsNull() {
		return false
	}
	_, ok := p.spans[v.Uint64()]
	return ok
}
func (p spanPresencePredicate) KeepPage(page parquet.Page) bool {
	if d := page.Dictionary(); d != nil {
		for i := 0; i < d.Len(); i++ {
			if p.KeepValue(d.Index(int32(i))) {
				return true
			}
		}
		return false
	}
	return true
}

func querySpanPresence(q *queryContext, query *queryv1.Query) (_ *queryv1.Report, err error) {
	r := query.SpanPresence
	if r == nil || len(r.SpanSelector)+len(r.TraceIdSelector) == 0 {
		return nil, fmt.Errorf("at least one span or trace selector is required")
	}
	spans, err := model.NewSpanSelector(r.SpanSelector)
	if err != nil {
		return nil, err
	}
	traces, err := model.NewTraceSelector(r.TraceIdSelector)
	if err != nil {
		return nil, err
	}
	spanIndex, _ := pq.GetColumnIndexByPath(q.ds.Profiles().Root(), spanColumn)
	traceIndex, _ := pq.GetColumnIndexByPath(q.ds.Profiles().Root(), traceColumn)
	if (len(spans) > 0 && spanIndex < 0) || (len(traces) > 0 && traceIndex < 0) {
		return &queryv1.Report{SpanPresence: &queryv1.SpanPresenceReport{Query: r.CloneVT()}}, nil
	}
	entries, err := profileEntryIterator(q, withGroupByLabels(model.LabelNameProfileType), withFetchPartition(false))
	if err != nil {
		return nil, err
	}
	defer entries.Close()
	valueIndex, _ := pq.GetColumnIndexByPath(q.ds.Profiles().Root(), valueColumn)
	columns := []int{valueIndex}
	spanPosition, tracePosition := -1, -1
	if spanIndex >= 0 {
		spanPosition = len(columns)
		columns = append(columns, spanIndex)
	}
	if traceIndex >= 0 {
		tracePosition = len(columns)
		columns = append(columns, traceIndex)
	}
	predicate := func(values [][]parquet.Value, i int) bool {
		if len(spans) > 0 && !(spanPresencePredicate{spans}).KeepValue(values[spanPosition][i]) {
			return false
		}
		if len(traces) > 0 {
			v := values[tracePosition][i]
			if v.IsNull() {
				return false
			}
			var id model.TraceID
			copy(id[:], v.ByteArray())
			if _, ok := traces[id]; !ok {
				return false
			}
		}
		return true
	}
	options := []pq.RepeatedRowOption{pq.WithSamplePredicate(predicate)}
	if len(spans) > 0 {
		options = append(options, pq.WithPagePredicate(spanIndex, spanPresencePredicate{spans}))
	}
	if len(traces) > 0 {
		ids := make([]string, 0, len(r.TraceIdSelector))
		for _, id := range r.TraceIdSelector {
			decoded, _ := hex.DecodeString(id)
			ids = append(ids, string(decoded))
		}
		options = append(options, pq.WithPagePredicate(traceIndex, pq.NewStringInPredicate(ids)))
	}
	it := pq.NewRepeatedRowIteratorWithOptions(q.ctx, entries, q.ds.Profiles().RowGroups(), columns, options...)
	defer func() {
		if e := it.Close(); err == nil {
			err = e
		}
	}()
	type sampleKey struct {
		span              uint64
		trace             model.TraceID
		hasSpan, hasTrace bool
		profileType       string
	}
	totals := make(map[sampleKey]int64)
	for it.Next() {
		result := it.At()
		vv := result.Values[0]
		var sv, tv []parquet.Value
		if spanPosition >= 0 {
			sv = result.Values[spanPosition]
		}
		if tracePosition >= 0 {
			tv = result.Values[tracePosition]
		}
		if spanIndex < 0 {
			sv = make([]parquet.Value, len(vv))
		}
		if traceIndex < 0 {
			tv = make([]parquet.Value, len(vv))
		}
		if len(sv) != len(vv) || len(tv) != len(vv) {
			return nil, fmt.Errorf("unaligned sample columns")
		}
		typ := result.Row.Labels.Get(model.LabelNameProfileType)
		for i, v := range vv {
			s := sv[i]
			t := tv[i]
			key := sampleKey{profileType: typ, hasSpan: !s.IsNull(), hasTrace: !t.IsNull()}
			if key.hasSpan {
				key.span = s.Uint64()
			}
			if key.hasTrace {
				copy(key.trace[:], t.ByteArray())
			}
			totals[key] += v.Int64()
		}
	}
	if err = it.Err(); err != nil {
		return nil, err
	}
	var merger model.SpanPresenceMerger
	for key, total := range totals {
		sid, trace := "", ""
		if key.hasSpan {
			var raw [8]byte
			binary.LittleEndian.PutUint64(raw[:], key.span)
			sid = hex.EncodeToString(raw[:])
		}
		if key.hasTrace {
			trace = hex.EncodeToString(key.trace[:])
		}
		merger.Merge([]*querierv1.SpanPresence{{SpanId: sid, TraceId: trace, Totals: []*querierv1.SpanProfileTotal{{ProfileTypeID: key.profileType, Total: total}}}})
	}
	return &queryv1.Report{SpanPresence: &queryv1.SpanPresenceReport{Query: r.CloneVT(), Spans: merger.Spans()}}, nil
}

type spanPresenceAggregator struct {
	once   sync.Once
	query  *queryv1.SpanPresenceQuery
	merger model.SpanPresenceMerger
}

func newSpanPresenceAggregator(*queryv1.InvokeRequest) aggregator { return new(spanPresenceAggregator) }
func (a *spanPresenceAggregator) aggregate(r *queryv1.Report) error {
	a.once.Do(func() { a.query = r.SpanPresence.Query.CloneVT() })
	a.merger.Merge(r.SpanPresence.Spans)
	return nil
}
func (a *spanPresenceAggregator) build() *queryv1.Report {
	return &queryv1.Report{SpanPresence: &queryv1.SpanPresenceReport{Query: a.query, Spans: a.merger.Spans()}}
}
