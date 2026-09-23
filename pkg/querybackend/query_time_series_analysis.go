package querybackend

import (
	"math"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/model/timeseries"
	"github.com/grafana/pyroscope/v2/pkg/model/timeseriesanalysis"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_TIME_SERIES_ANALYSIS,
		queryv1.ReportType_REPORT_TIME_SERIES_ANALYSIS,
		queryTimeSeriesAnalysis,
		newTimeSeriesAnalysisAggregator,
		true,
		block.SectionTSDB,
		block.SectionProfiles,
	)
}

func queryTimeSeriesAnalysis(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	analysisQuery := query.TimeSeriesAnalysis
	if analysisQuery == nil {
		return nil, status.Error(codes.InvalidArgument, "time series analysis query is required")
	}
	if math.IsNaN(analysisQuery.Step) || math.IsInf(analysisQuery.Step, 0) || analysisQuery.Step < timeseries.MinimumStep.Seconds() {
		return nil, status.Error(codes.InvalidArgument, "step must be >= 1ms and finite")
	}
	if err := analysisConfig(analysisQuery.Config).WithDefaults().Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	result, err := executeTimeSeriesQuery(q, analysisQuery.GroupBy, typesv1.ExemplarType_EXEMPLAR_TYPE_NONE)
	if err != nil {
		return nil, err
	}
	return &queryv1.Report{
		TimeSeriesAnalysis: &queryv1.TimeSeriesAnalysisReport{
			Query:      analysisQuery.CloneVT(),
			TimeSeries: result.series,
		},
	}, nil
}

type timeSeriesAnalysisAggregator struct {
	init      sync.Once
	startTime int64
	endTime   int64
	query     *queryv1.TimeSeriesAnalysisQuery
	series    *timeseries.Merger
}

func newTimeSeriesAnalysisAggregator(req *queryv1.InvokeRequest) aggregator {
	return &timeSeriesAnalysisAggregator{
		startTime: req.StartTime,
		endTime:   req.EndTime,
	}
}

func (a *timeSeriesAnalysisAggregator) aggregate(report *queryv1.Report) error {
	r := report.TimeSeriesAnalysis
	a.init.Do(func() {
		a.series = timeseries.NewMerger(true)
		a.query = r.Query.CloneVT()
	})
	a.series.MergeTimeSeries(r.TimeSeries)
	return nil
}

func (a *timeSeriesAnalysisAggregator) build() *queryv1.Report {
	stepMillis := time.Duration(a.query.Step * float64(time.Second)).Milliseconds()
	sum := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM
	series := timeseries.RangeSeries(
		timeseries.NewTimeSeriesMergeIterator(a.series.TimeSeries()),
		a.startTime+stepMillis,
		a.endTime,
		stepMillis,
		&sum,
	)
	return &queryv1.Report{TimeSeriesAnalysis: &queryv1.TimeSeriesAnalysisReport{
		Query:      a.query,
		TimeSeries: series,
	}}
}

// finalizeTimeSeriesAnalysis runs once, on the root query backend, after all
// series have been merged and bucketed. No partial events or limits are applied
// at intermediate nodes.
func finalizeTimeSeriesAnalysis(report *queryv1.TimeSeriesAnalysisReport) error {
	stepMillis := time.Duration(report.Query.Step * float64(time.Second)).Milliseconds()
	events, err := timeseriesanalysis.Analyze(report.TimeSeries, time.Duration(stepMillis)*time.Millisecond, analysisConfig(report.Query.Config))
	if err != nil {
		return err
	}
	report.Events = analysisEvents(timeseriesanalysis.TopEvents(events, int(report.Query.Limit)))
	// The frontend only needs the final events, not the source series.
	report.TimeSeries = nil
	return nil
}

func analysisConfig(config *queryv1.TimeSeriesAnalysisConfig) timeseriesanalysis.Config {
	if config == nil {
		return timeseriesanalysis.Config{}
	}
	return timeseriesanalysis.Config{
		BaselineWindow:         int(config.BaselineWindow),
		ConfirmationWindow:     int(config.ConfirmationWindow),
		MinimumSustainedPoints: int(config.MinimumSustainedPoints),
		MinimumRelativeChange:  config.MinimumRelativeChange,
		ScoreThreshold:         config.ScoreThreshold,
		RecoveryThresholdRatio: config.RecoveryThresholdRatio,
		FlatnessThreshold:      config.FlatnessThreshold,
	}
}

func analysisEvents(events []timeseriesanalysis.Event) []*querierv1.TimeSeriesAnomaly {
	result := make([]*querierv1.TimeSeriesAnomaly, len(events))
	for i, event := range events {
		result[i] = &querierv1.TimeSeriesAnomaly{
			Type:           analysisEventType(event.Type),
			Labels:         event.Labels,
			TimestampStart: event.StartTime,
			TimestampEnd:   event.EndTime,
			TimestampPeak:  event.PeakTime,
			Baseline:       event.Baseline,
			PeakValue:      event.PeakValue,
			RelativeChange: event.RelativeChange,
			Score:          event.Score,
		}
	}
	return result
}

func analysisEventType(eventType timeseriesanalysis.EventType) querierv1.TimeSeriesAnomalyType {
	switch eventType {
	case timeseriesanalysis.EventTypeSpike:
		return querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_SPIKE
	case timeseriesanalysis.EventTypeDrop:
		return querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_DROP
	case timeseriesanalysis.EventTypeIncrease:
		return querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_INCREASE
	case timeseriesanalysis.EventTypeDecrease:
		return querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_DECREASE
	default:
		return querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_UNSPECIFIED
	}
}
