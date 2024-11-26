package query_backend

import (
	"fmt"
	"strings"
	"sync"

	"github.com/grafana/dskit/runutil"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/query_backend/block"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	parquetquery "github.com/grafana/pyroscope/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_METRICS,
		queryv1.ReportType_REPORT_METRICS,
		queryMetrics,
		newMetricsAggregator,
		[]block.Section{
			block.SectionProfiles,
			block.SectionTSDB,
			block.SectionSymbols,
		}...,
	)
}

func queryMetrics(q *queryContext, query *queryv1.Query) (r *queryv1.Report, err error) {
	entries, err := profileEntryIterator(q)
	if err != nil {
		return nil, err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")

	var columns schemav1.SampleColumns
	if err := columns.Resolve(q.ds.Profiles().Schema()); err != nil {
		// TODO
	}
	column, err := schemav1.ResolveColumnByPath(q.ds.Profiles().Schema(), strings.Split("TotalValue", "."))
	if err != nil {
		return nil, err
	}

	rows := parquetquery.NewRepeatedRowIterator(q.ctx, entries, q.ds.Profiles().RowGroups(),
		column.ColumnIndex,
		columns.StacktraceID.ColumnIndex,
		columns.Value.ColumnIndex)
	defer runutil.CloseWithErrCapture(&err, rows, "failed to close column iterator")

	labelsFromFp := make(map[uint64]phlaremodel.Labels)
	builders := make(map[uint64]map[string]*phlaremodel.TimeSeriesBuilder)
	for rows.Next() {
		row := rows.At()
		fp := uint64(row.Row.Fingerprint)
		// Storing for later
		_, ok := labelsFromFp[fp]
		if !ok {
			labelsFromFp[fp] = row.Row.Labels
		}

		// Lazy init of builders
		_, ok = builders[fp]
		if !ok {
			builders[fp] = make(map[string]*phlaremodel.TimeSeriesBuilder)
		}

		// "" stands for total dimensions (no specific function)
		_, ok = builders[fp][""]
		if !ok {
			builders[fp][""] = phlaremodel.NewTimeSeriesBuilder()
		}
		builders[fp][""].Add(
			row.Row.Fingerprint,
			row.Row.Labels,
			int64(row.Row.Timestamp),
			float64(row.Values[0][0].Int64()),
		)

		// metrics for target functions
		reader, _ := q.ds.Symbols().Partition(q.ctx, row.Row.Partition)
		for _, target := range query.Metrics.FunctionsByServiceName[q.ds.Meta().Name].Functions {
			stringsTable := reader.Symbols().Strings
			targetIndex := -1
			for i, s := range stringsTable {
				if s == target {
					targetIndex = i
				}
			}
			if targetIndex == -1 {
				continue
			}
			targetFunction := -1
			for i, fn := range reader.Symbols().Functions {
				if targetIndex == int(fn.SystemName) {
					targetFunction = i
				}
			}
			if targetFunction == -1 {
				continue
			}
			total := 0
			for i, stacktraceId := range row.Values[1] {
				var locations []uint64
				locations = reader.Symbols().Stacktraces.LookupLocations(locations, stacktraceId.Uint32())
				found := false
				for _, location := range locations {
					for _, line := range reader.Symbols().Locations[location].Line {
						if int(line.FunctionId) == targetFunction {
							found = true
							break
						}
					}
					if found {
						break
					}
				}
				if !found {
					continue
				}
				total += int(row.Values[2][i].Uint32())
			}
			_, ok = builders[fp][target]
			if !ok {
				builders[fp][target] = phlaremodel.NewTimeSeriesBuilder()
			}
			builders[fp][target].Add(
				row.Row.Fingerprint,
				row.Row.Labels,
				int64(row.Row.Timestamp),
				float64(total),
			)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	// Build everything (all + functions)
	var timeSeries []*typesv1.Series
	for fingerPrint, buildersByFunction := range builders {
		for function, builder := range buildersByFunction {
			labels := getLabels(labelsFromFp[fingerPrint], function)
			samples := getSamples(builder)
			timeSeries = append(timeSeries, &typesv1.Series{
				Labels: labels,
				Points: samples,
			})
		}
	}

	resp := &queryv1.Report{
		Metrics: &queryv1.MetricsReport{
			Query:      query.Metrics.CloneVT(),
			TimeSeries: timeSeries,
		},
	}

	return resp, nil
}

type metricsAggregator struct {
	init   sync.Once
	query  *queryv1.MetricsQuery
	series []*typesv1.Series
}

func newMetricsAggregator(req *queryv1.InvokeRequest) aggregator {
	return &metricsAggregator{}
}

func (a *metricsAggregator) aggregate(report *queryv1.Report) error {
	r := report.Metrics
	a.init.Do(func() {
		a.series = make([]*typesv1.Series, 0)
		a.query = r.Query.CloneVT()
	})
	for _, s := range report.Metrics.GetTimeSeries() {
		a.series = append(a.series, s)
	}
	return nil
}

func (a *metricsAggregator) build() *queryv1.Report {
	return &queryv1.Report{
		Metrics: &queryv1.MetricsReport{
			Query:      a.query,
			TimeSeries: a.series,
		},
	}
}

func getLabels(labelsIn phlaremodel.Labels, functionName string) phlaremodel.Labels {
	prefix := "pyroscope_exported_metrics_"
	if functionName != "" {
		prefix = prefix + "functions_"
	}
	var labels phlaremodel.Labels
	var reservedLabels = make(map[string]*typesv1.LabelPair)
	for _, label := range labelsIn {
		if strings.HasPrefix(label.Name, "__") {
			reservedLabels[label.Name] = label
			continue
		}
		fmt.Print(label.Name, "=", label.Value, ", ")
		labels = append(labels, &typesv1.LabelPair{
			Name:  label.Name,
			Value: label.Value,
		})
	}
	labels = append(labels, &typesv1.LabelPair{
		Name:  "__name__",
		Value: prefix + strings.ReplaceAll(reservedLabels["__profile_type__"].Value, ":", "_"),
	})
	if functionName != "" {
		labels = append(labels, &typesv1.LabelPair{
			Name:  "function",
			Value: functionName,
		})
	}
	return labels
}

func getSamples(builder *phlaremodel.TimeSeriesBuilder) []*typesv1.Point {
	timeSeries := builder.Build()
	var samples []*typesv1.Point
	for _, point := range timeSeries[0].Points {
		fmt.Println("(", point.Timestamp, ",", point.Value, ")")
		samples = append(samples, &typesv1.Point{
			Value:     point.Value,
			Timestamp: point.Timestamp,
		})
	}
	return samples
}
