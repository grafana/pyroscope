package querybackend

import (
	"fmt"
	"sync"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

type merger struct {
	labels  *labelsMerger
	metrics *metricsMerger
	tree    *treeMerger
}

func (m *merger) merge(resp *querybackendv1.InvokeResponse, err error) error {
	if err != nil {
		return err
	}
	for _, r := range resp.Reports {
		if err = m.mergeReport(r); err != nil {
			return err
		}
	}
	return nil
}

func (m *merger) mergeReport(r *querybackendv1.Report) (err error) {
	switch x := r.ReportType.(type) {
	case *querybackendv1.Report_LabelNames:
		m.labels.mergeNames(x.LabelNames)
	case *querybackendv1.Report_LabelValues:
		m.labels.mergeValues(x.LabelValues)
	case *querybackendv1.Report_SeriesLabels:
		m.labels.mergeSeries(x.SeriesLabels)
	case *querybackendv1.Report_Metrics:
		m.metrics.merge(x.Metrics)
	case *querybackendv1.Report_Tree:
		err = m.tree.merge(x.Tree)
	default:
		return fmt.Errorf("unknown report type %T", x)
	}
	return err
}

func (m *merger) response() (*querybackendv1.InvokeResponse, error) {
	reports := make([]*querybackendv1.Report, 0, 4)
	reports = m.labels.append(reports)
	reports = m.metrics.append(reports)
	reports = m.tree.append(reports)
	return &querybackendv1.InvokeResponse{Reports: reports}, nil
}

type treeMerger struct {
	init  sync.Once
	query *querybackendv1.TreeQuery
	tree  *model.TreeMerger
}

func (m *treeMerger) merge(report *querybackendv1.TreeReport) error {
	m.init.Do(func() {
		m.tree = model.NewTreeMerger()
		m.query = report.Query.CloneVT()
	})
	return m.tree.MergeTreeBytes(report.Data)
}

func (m *treeMerger) append(reports []*querybackendv1.Report) []*querybackendv1.Report {
	if m.tree == nil {
		return reports
	}
	return append(reports, &querybackendv1.Report{
		ReportType: &querybackendv1.Report_Tree{
			Tree: &querybackendv1.TreeReport{
				Query: m.query,
				Data:  m.tree.Tree().Bytes(m.query.GetMaxNodes()),
			},
		},
	})
}

type metricsMerger struct {
	init    sync.Once
	query   *querybackendv1.MetricsQuery
	metrics *model.MetricsMerger
}

func (m *metricsMerger) merge(report *querybackendv1.MetricsReport) {
	m.init.Do(func() {
		sum := report.Query.GetAggregation() == typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM
		m.metrics = model.NewMetricsMerger(sum)
		m.query = report.Query.CloneVT()
	})
	m.metrics.MergeMetrics(report.Metrics)
}

func (m *metricsMerger) append(reports []*querybackendv1.Report) []*querybackendv1.Report {
	if m.metrics == nil {
		return reports
	}
	return append(reports, &querybackendv1.Report{
		ReportType: &querybackendv1.Report_Metrics{
			Metrics: &querybackendv1.MetricsReport{
				Query:   m.query,
				Metrics: m.metrics.Metrics(),
			},
		},
	})
}

type labelsMerger struct {
	initMerger  sync.Once
	labels      *model.LabelsMerger
	initNames   sync.Once
	queryNames  *querybackendv1.LabelNamesQuery
	initValues  sync.Once
	queryValues *querybackendv1.LabelValuesQuery
	initSeries  sync.Once
	querySeries *querybackendv1.SeriesLabelsQuery
}

func (m *labelsMerger) mergeNames(report *querybackendv1.LabelNamesReport) {
	m.initNames.Do(func() {
		m.queryNames = report.Query.CloneVT()
		m.init()
	})
	m.labels.MergeLabelNames(report.LabelNames)
}

func (m *labelsMerger) mergeValues(report *querybackendv1.LabelValuesReport) {
	m.initValues.Do(func() {
		m.queryValues = report.Query.CloneVT()
		m.init()
	})
	m.labels.MergeLabelValues(report.LabelValues)
}

func (m *labelsMerger) mergeSeries(report *querybackendv1.SeriesLabelsReport) {
	m.initSeries.Do(func() {
		m.querySeries = report.Query.CloneVT()
		m.init()
	})
	m.labels.MergeSeries(report.SeriesLabels)
}

func (m *labelsMerger) init() {
	m.initMerger.Do(func() {
		m.labels = model.NewLabelsMerger()
	})
}

func (m *labelsMerger) append(reports []*querybackendv1.Report) []*querybackendv1.Report {
	if m.labels == nil {
		return reports
	}
	if m.labels.HasNames() {
		reports = append(reports, &querybackendv1.Report{
			ReportType: &querybackendv1.Report_LabelNames{
				LabelNames: &querybackendv1.LabelNamesReport{
					LabelNames: m.labels.LabelNames(),
				},
			},
		})
	}
	if m.labels.HasValues() {
		reports = append(reports, &querybackendv1.Report{
			ReportType: &querybackendv1.Report_LabelValues{
				LabelValues: &querybackendv1.LabelValuesReport{
					LabelValues: m.labels.LabelValues(),
				},
			},
		})
	}
	if m.labels.HasSeries() {
		reports = append(reports, &querybackendv1.Report{
			ReportType: &querybackendv1.Report_SeriesLabels{
				SeriesLabels: &querybackendv1.SeriesLabelsReport{
					Query:        m.querySeries,
					SeriesLabels: m.labels.SeriesLabels(),
				},
			},
		})
	}
	return reports
}
