package metrics

import (
	"reflect"
	"sort"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/mock"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockmetrics"
)

var (
	blockTime   = int64(0)
	profileTime = int64(1)
)

func Test_Observer_observe(t *testing.T) {
	exporter := new(mockmetrics.MockExporter)
	ruler := new(mockmetrics.MockRuler)
	ruler.On("RecordingRules", mock.Anything).Return([]*phlaremodel.RecordingRule{
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "1"),
				labels.MustNewMatcher(labels.MatchEqual, "b", "1"),
			},
			GroupBy:        []string{"c"},
			ExternalLabels: labels.Labels{{Name: "external1", Value: "external1"}},
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "d", "1"),
			},
			GroupBy: []string{},
		},
	})
	observer := NewSampleObserver(blockTime, exporter, ruler, labels.Label{Name: "external2", Value: "external2"})
	entries := entriesOf([][]any{
		{"tenant1", [][]string{{"a", "0"}, {"b", "0"}, {"c", "0"}, {"d", "0"}}, int64(1) << 0},
		{"tenant1", [][]string{{"a", "0"}, {"b", "0"}, {"c", "0"}, {"d", "1"}}, int64(1) << 1},
		{"tenant1", [][]string{{"a", "0"}, {"b", "0"}, {"c", "1"}, {"d", "0"}}, int64(1) << 2},
		{"tenant1", [][]string{{"a", "0"}, {"b", "0"}, {"c", "1"}, {"d", "1"}}, int64(1) << 3},
		{"tenant1", [][]string{{"a", "0"}, {"b", "1"}, {"c", "0"}, {"d", "0"}}, int64(1) << 4},
		{"tenant1", [][]string{{"a", "0"}, {"b", "1"}, {"c", "0"}, {"d", "1"}}, int64(1) << 5},
		{"tenant1", [][]string{{"a", "0"}, {"b", "1"}, {"c", "1"}, {"d", "0"}}, int64(1) << 6},
		{"tenant1", [][]string{{"a", "0"}, {"b", "1"}, {"c", "1"}, {"d", "1"}}, int64(1) << 7},
		{"tenant1", [][]string{{"a", "1"}, {"b", "0"}, {"c", "0"}, {"d", "0"}}, int64(1) << 8},
		{"tenant1", [][]string{{"a", "1"}, {"b", "0"}, {"c", "0"}, {"d", "1"}}, int64(1) << 9},
		{"tenant1", [][]string{{"a", "1"}, {"b", "0"}, {"c", "1"}, {"d", "0"}}, int64(1) << 10},
		{"tenant1", [][]string{{"a", "1"}, {"b", "0"}, {"c", "1"}, {"d", "1"}}, int64(1) << 11},
		{"tenant1", [][]string{{"a", "1"}, {"b", "1"}, {"c", "0"}, {"d", "0"}}, int64(1) << 12},
		{"tenant1", [][]string{{"a", "1"}, {"b", "1"}, {"c", "0"}, {"d", "1"}}, int64(1) << 13},
		{"tenant1", [][]string{{"a", "1"}, {"b", "1"}, {"c", "1"}, {"d", "0"}}, int64(1) << 14},
		{"tenant1", [][]string{{"a", "1"}, {"b", "1"}, {"c", "1"}, {"d", "1"}}, int64(1) << 15},
		{"tenant2", [][]string{{"x", "1"}}, int64(1)},
		{"tenant3", [][]string{{"a", "1"}, {"b", "1"}, {"c", "1"}, {"d", "1"}}, int64(1) << 0},
		{"tenant3", [][]string{{"a", "1"}, {"b", "1"}, {"c", "1"}, {"d", "1"}}, int64(1) << 1},
		{"tenant3", [][]string{{"a", "1"}, {"b", "1"}, {"c", "1"}, {"d", "1"}}, int64(1) << 2},
	})

	exporter.On("Send", "tenant1",
		mock.MatchedBy(func(series []prompb.TimeSeries) bool {
			return sameSeries(series, []prompb.TimeSeries{
				timeSeriesOf([]any{[][]string{{"c", "0"}, {"external1", "external1"}, {"external2", "external2"}}, 1<<12 + 1<<13, blockTime}),
				timeSeriesOf([]any{[][]string{{"c", "1"}, {"external1", "external1"}, {"external2", "external2"}}, 1<<14 + 1<<15, blockTime}),
				timeSeriesOf([]any{[][]string{{"external2", "external2"}}, 1<<1 + 1<<3 + 1<<5 + 1<<7 + 1<<9 + 1<<11 + 1<<13 + 1<<15, blockTime}),
			})
		}),
	).Return(nil).Once()

	exporter.On("Send", "tenant3",
		mock.MatchedBy(func(series []prompb.TimeSeries) bool {
			return sameSeries(series, []prompb.TimeSeries{
				timeSeriesOf([]any{[][]string{{"c", "1"}, {"external1", "external1"}, {"external2", "external2"}}, 1<<0 + 1<<1 + 1<<2, blockTime}),
				timeSeriesOf([]any{[][]string{{"external2", "external2"}}, 1<<0 + 1<<1 + 1<<2, blockTime}),
			})
		}),
	).Return(nil).Once()

	for _, entry := range entries {
		observer.Observe(entry)
	}
	observer.Close()

	ruler.AssertExpectations(t)
	exporter.AssertExpectations(t)
	exporter.AssertNotCalled(t, "Send", "tenant2", mock.Anything)
}

func sameSeries(series1 []prompb.TimeSeries, series2 []prompb.TimeSeries) bool {
	for _, s := range series1 {
		found := false
		for _, s2 := range series2 {
			found = reflect.DeepEqual(s, s2)
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func timeSeriesOf(values []any) prompb.TimeSeries {
	lbls := make([]prompb.Label, len(values[0].([][]string)))
	for i, label := range values[0].([][]string) {
		lbls[i] = prompb.Label{
			Name:  label[0],
			Value: label[1],
		}
	}
	return prompb.TimeSeries{
		Labels: lbls,
		Samples: []prompb.Sample{
			{
				Value:     float64(values[1].(int)),
				Timestamp: values[2].(int64),
			},
		},
	}
}

func entriesOf(values [][]any) []block.ProfileEntry {
	profileEntries := make([]block.ProfileEntry, len(values))
	for i, value := range values {
		ls := make(phlaremodel.Labels, len(value[1].([][]string)))
		for j, label := range value[1].([][]string) {
			ls[j] = &typesv1.LabelPair{
				Name:  label[0],
				Value: label[1],
			}
		}
		sort.Sort(ls)
		row := make(v1.ProfileRow, 4)
		row[3] = parquet.Int64Value(value[2].(int64))
		profileEntries[i] = block.ProfileEntry{
			Dataset:     datasetForTenant(value[0].(string)),
			Timestamp:   profileTime,
			Fingerprint: model.Fingerprint(ls.Hash()),
			Labels:      ls,
			Row:         row,
		}
	}
	return profileEntries
}

func datasetForTenant(tenant string) *block.Dataset {
	return block.NewDataset(
		&metastorev1.Dataset{},
		block.NewObject(
			nil,
			&metastorev1.BlockMeta{StringTable: []string{tenant}},
		),
	)
}
