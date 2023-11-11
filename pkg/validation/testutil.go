package validation

import (
	"time"
)

type MockLimits struct {
	QuerySplitDurationValue     time.Duration
	MaxQueryParallelismValue    int
	MaxQueryLengthValue         time.Duration
	MaxQueryLookbackValue       time.Duration
	MaxLabelNameLengthValue     int
	MaxLabelValueLengthValue    int
	MaxLabelNamesPerSeriesValue int

	MaxFlameGraphNodesDefaultValue int
	MaxFlameGraphNodesMaxValue     int

	DistributorAggregationWindowValue time.Duration
	DistributorAggregationPeriodValue time.Duration

	RejectOlderThanValue time.Duration
	RejectNewerThanValue time.Duration

	MaxProfileSizeBytesValue              int
	MaxProfileStacktraceSamplesValue      int
	MaxProfileStacktraceDepthValue        int
	MaxProfileStacktraceSampleLabelsValue int
	MaxProfileSymbolValueLengthValue      int
}

func (m MockLimits) QuerySplitDuration(string) time.Duration        { return m.QuerySplitDurationValue }
func (m MockLimits) MaxQueryParallelism(string) int                 { return m.MaxQueryParallelismValue }
func (m MockLimits) MaxQueryLength(tenantID string) time.Duration   { return m.MaxQueryLengthValue }
func (m MockLimits) MaxQueryLookback(tenantID string) time.Duration { return m.MaxQueryLookbackValue }

func (m MockLimits) MaxFlameGraphNodesDefault(string) int { return m.MaxFlameGraphNodesDefaultValue }
func (m MockLimits) MaxFlameGraphNodesMax(string) int     { return m.MaxFlameGraphNodesMaxValue }

func (m MockLimits) MaxLabelNameLength(userID string) int     { return m.MaxLabelNameLengthValue }
func (m MockLimits) MaxLabelValueLength(userID string) int    { return m.MaxLabelValueLengthValue }
func (m MockLimits) MaxLabelNamesPerSeries(userID string) int { return m.MaxLabelNamesPerSeriesValue }
func (m MockLimits) MaxProfileSizeBytes(userID string) int    { return m.MaxProfileSizeBytesValue }
func (m MockLimits) MaxProfileStacktraceSamples(userID string) int {
	return m.MaxProfileStacktraceSamplesValue
}

func (m MockLimits) DistributorAggregationWindow(userID string) time.Duration {
	return m.DistributorAggregationWindowValue
}
func (m MockLimits) DistributorAggregationPeriod(userID string) time.Duration {
	return m.DistributorAggregationPeriodValue
}

func (m MockLimits) MaxProfileStacktraceDepth(userID string) int {
	return m.MaxProfileStacktraceDepthValue
}

func (m MockLimits) MaxProfileStacktraceSampleLabels(userID string) int {
	return m.MaxProfileStacktraceSampleLabelsValue
}

func (m MockLimits) MaxProfileSymbolValueLength(userID string) int {
	return m.MaxProfileSymbolValueLengthValue
}

func (m MockLimits) RejectOlderThan(userID string) time.Duration {
	return m.RejectOlderThanValue
}

func (m MockLimits) RejectNewerThan(userID string) time.Duration {
	return m.RejectNewerThanValue
}
