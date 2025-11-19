package querybackend

import (
	"sort"
	"sync"

	"github.com/prometheus/prometheus/model/labels"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_LABEL_NAMES,
		queryv1.ReportType_REPORT_LABEL_NAMES,
		queryLabelNames,
		newLabelNameAggregator,
		false,
		[]block.Section{block.SectionTSDB}...,
	)
}

func queryLabelNames(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	var names []string
	var err error
	if len(q.req.matchers) == 0 {
		names, err = q.ds.Index().LabelNames()
	} else {
		names, err = labelNamesForMatchers(q.ds.Index(), q.req.matchers)
	}
	if err != nil {
		return nil, err
	}
	resp := &queryv1.Report{
		LabelNames: &queryv1.LabelNamesReport{
			Query:      query.LabelNames.CloneVT(),
			LabelNames: names,
		},
	}
	return resp, nil
}

func labelNamesForMatchers(reader phlaredb.IndexReader, matchers []*labels.Matcher) ([]string, error) {
	postings, err := phlaredb.PostingsForMatchers(reader, nil, matchers...)
	if err != nil {
		return nil, err
	}
	l := make(map[string]struct{})
	for postings.Next() {
		var n []string
		if n, err = reader.LabelNamesFor(postings.At()); err != nil {
			return nil, err
		}
		for _, name := range n {
			l[name] = struct{}{}
		}
	}
	if err = postings.Err(); err != nil {
		return nil, err
	}
	names := make([]string, len(l))
	var i int
	for name := range l {
		names[i] = name
		i++
	}
	sort.Strings(names)
	return names, nil
}

type labelNameAggregator struct {
	init  sync.Once
	query *queryv1.LabelNamesQuery
	names *model.LabelMerger
}

func newLabelNameAggregator(*queryv1.InvokeRequest) aggregator {
	return new(labelNameAggregator)
}

func (m *labelNameAggregator) aggregate(report *queryv1.Report) error {
	r := report.LabelNames
	m.init.Do(func() {
		m.query = r.Query.CloneVT()
		m.names = model.NewLabelMerger()
	})
	m.names.MergeLabelNames(r.LabelNames)
	return nil
}

func (m *labelNameAggregator) build() *queryv1.Report {
	return &queryv1.Report{
		LabelNames: &queryv1.LabelNamesReport{
			Query:      m.query,
			LabelNames: m.names.LabelNames(),
		},
	}
}
