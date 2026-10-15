package timeseriesanalysis

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

const continuityFactor = 1.5

type EventType uint8

const (
	EventTypeSpike EventType = iota + 1
	EventTypeDrop
	EventTypeIncrease
	EventTypeDecrease
)

type Duration uint8

const (
	DurationShortTerm Duration = iota + 1
	DurationSustained
)

type Config struct {
	BaselineWindow         int
	ConfirmationWindow     int
	MinimumSustainedPoints int
	MinimumRelativeChange  float64
	ScoreThreshold         float64
	RecoveryThresholdRatio float64
	FlatnessThreshold      float64
}

func DefaultConfig() Config {
	return Config{
		BaselineWindow:         12,
		ConfirmationWindow:     5,
		MinimumSustainedPoints: 3,
		MinimumRelativeChange:  0.25,
		ScoreThreshold:         3.5,
		RecoveryThresholdRatio: 0.5,
		FlatnessThreshold:      0.01,
	}
}

func (c Config) WithDefaults() Config {
	d := DefaultConfig()
	if c.BaselineWindow == 0 {
		c.BaselineWindow = d.BaselineWindow
	}
	if c.ConfirmationWindow == 0 {
		c.ConfirmationWindow = d.ConfirmationWindow
	}
	if c.MinimumSustainedPoints == 0 {
		c.MinimumSustainedPoints = d.MinimumSustainedPoints
	}
	if c.MinimumRelativeChange == 0 {
		c.MinimumRelativeChange = d.MinimumRelativeChange
	}
	if c.ScoreThreshold == 0 {
		c.ScoreThreshold = d.ScoreThreshold
	}
	if c.RecoveryThresholdRatio == 0 {
		c.RecoveryThresholdRatio = d.RecoveryThresholdRatio
	}
	if c.FlatnessThreshold == 0 {
		c.FlatnessThreshold = d.FlatnessThreshold
	}
	return c
}

func (c Config) Validate() error {
	if c.BaselineWindow < 2 {
		return fmt.Errorf("baseline window must be at least 2")
	}
	if c.ConfirmationWindow < 1 {
		return fmt.Errorf("confirmation window must be at least 1")
	}
	if c.MinimumSustainedPoints < 1 || c.MinimumSustainedPoints > c.ConfirmationWindow+1 {
		return fmt.Errorf("minimum sustained points must be between 1 and confirmation window plus 1")
	}
	if !finitePositive(c.MinimumRelativeChange) {
		return fmt.Errorf("minimum relative change must be finite and positive")
	}
	if !finitePositive(c.ScoreThreshold) {
		return fmt.Errorf("score threshold must be finite and positive")
	}
	if !finitePositive(c.RecoveryThresholdRatio) || c.RecoveryThresholdRatio > 1 {
		return fmt.Errorf("recovery threshold ratio must be finite, positive, and at most 1")
	}
	if !finiteNonNegative(c.FlatnessThreshold) {
		return fmt.Errorf("flatness threshold must be finite and non-negative")
	}
	return nil
}

type Event struct {
	Type           EventType
	Duration       Duration
	Labels         []*typesv1.LabelPair
	StartTime      int64
	EndTime        int64
	PeakTime       int64
	Baseline       float64
	PeakValue      float64
	RelativeChange float64
	Score          float64
}

// Analyze detects events independently for each labeled series. Missing points
// split a series into separate runs rather than being interpreted as zeroes.
func Analyze(series []*typesv1.Series, step time.Duration, config Config) ([]Event, error) {
	config = config.WithDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if step < time.Millisecond {
		return nil, fmt.Errorf("step must be at least 1ms")
	}

	events := make([]Event, 0)
	for _, s := range series {
		if s == nil {
			continue
		}
		points := finitePoints(s.Points)
		if len(points) == 0 || isFlat(points, config.FlatnessThreshold) {
			continue
		}
		for _, run := range contiguousRuns(points, step) {
			events = append(events, analyzeRun(run, s.Labels, config)...)
		}
	}
	sortEvents(events)
	return events, nil
}

func finitePositive(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0 }

func finiteNonNegative(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }

func finitePoints(points []*typesv1.Point) []*typesv1.Point {
	result := make([]*typesv1.Point, 0, len(points))
	for _, point := range points {
		if point != nil && !math.IsNaN(point.Value) && !math.IsInf(point.Value, 0) {
			result = append(result, point)
		}
	}
	slices.SortFunc(result, func(a, b *typesv1.Point) int {
		if a.Timestamp < b.Timestamp {
			return -1
		}
		if a.Timestamp > b.Timestamp {
			return 1
		}
		return 0
	})
	return result
}

func isFlat(points []*typesv1.Point, threshold float64) bool {
	if len(points) < 2 {
		return true
	}
	min, max := points[0].Value, points[0].Value
	maxAbsolute := math.Abs(points[0].Value)
	for _, point := range points[1:] {
		min = math.Min(min, point.Value)
		max = math.Max(max, point.Value)
		maxAbsolute = math.Max(maxAbsolute, math.Abs(point.Value))
	}
	return max-min <= threshold*math.Max(maxAbsolute, 1)
}

func contiguousRuns(points []*typesv1.Point, step time.Duration) [][]*typesv1.Point {
	maxGap := int64(continuityFactor * float64(step.Milliseconds()))
	runs := make([][]*typesv1.Point, 0, 1)
	start := 0
	for i := 1; i < len(points); i++ {
		if points[i].Timestamp-points[i-1].Timestamp > maxGap {
			runs = append(runs, points[start:i])
			start = i
		}
	}
	return append(runs, points[start:])
}

func analyzeRun(points []*typesv1.Point, labels []*typesv1.LabelPair, config Config) []Event {
	if len(points) < config.BaselineWindow+config.ConfirmationWindow+1 {
		return nil
	}

	events := make([]Event, 0)
	for i := config.BaselineWindow; i+config.ConfirmationWindow < len(points); i++ {
		baseline, mad := medianMAD(points[i-config.BaselineWindow : i])
		candidate := points[i]
		delta := candidate.Value - baseline
		direction := 1.0
		if delta < 0 {
			direction = -1
		}
		scaleFloor := math.Max(1, math.Abs(baseline)*0.01)
		denominator := math.Max(math.Abs(baseline), scaleFloor)
		relativeChange := math.Abs(delta) / denominator
		scoreScale := math.Max(1.4826*mad, scaleFloor)
		score := math.Abs(delta) / scoreScale
		if relativeChange < config.MinimumRelativeChange || score < config.ScoreThreshold {
			continue
		}

		end, confirmed := eventEnd(points, i, baseline, direction, denominator, config)
		peak, peakScore := eventPeak(points[i:end+1], baseline, direction, scoreScale)
		event := Event{
			Type:           EventTypeSpike,
			Duration:       DurationShortTerm,
			Labels:         labels,
			StartTime:      candidate.Timestamp,
			EndTime:        points[end].Timestamp,
			PeakTime:       peak.Timestamp,
			Baseline:       baseline,
			PeakValue:      peak.Value,
			RelativeChange: math.Abs(peak.Value-baseline) / denominator,
			Score:          peakScore,
		}
		if confirmed >= config.MinimumSustainedPoints {
			event.Duration = DurationSustained
			if direction > 0 {
				event.Type = EventTypeIncrease
			} else {
				event.Type = EventTypeDecrease
			}
		} else if direction < 0 {
			event.Type = EventTypeDrop
		}
		events = append(events, event)
		i = end
	}
	return events
}

func medianMAD(points []*typesv1.Point) (float64, float64) {
	values := make([]float64, len(points))
	for i, point := range points {
		values[i] = point.Value
	}
	baseline := median(values)
	for i, value := range values {
		values[i] = math.Abs(value - baseline)
	}
	return baseline, median(values)
}

func median(values []float64) float64 {
	slices.Sort(values)
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return (values[middle-1] + values[middle]) / 2
}

func eventEnd(points []*typesv1.Point, start int, baseline, direction, denominator float64, config Config) (int, int) {
	end := start
	confirmed := 1
	confirmationEnd := min(len(points), start+config.ConfirmationWindow+1)
	for i := start + 1; i < len(points); i++ {
		elevated := direction*(points[i].Value-baseline)/denominator >= config.MinimumRelativeChange*config.RecoveryThresholdRatio
		if !elevated {
			break
		}
		end = i
		if i < confirmationEnd {
			confirmed++
		}
	}
	return end, confirmed
}

func eventPeak(points []*typesv1.Point, baseline, direction, scoreScale float64) (*typesv1.Point, float64) {
	peak := points[0]
	for _, point := range points[1:] {
		if direction*(point.Value-peak.Value) > 0 {
			peak = point
		}
	}
	// Use the pre-event baseline variability, matching the detection score.
	return peak, math.Abs(peak.Value-baseline) / scoreScale
}

// TopEvents retains events for the N series with the highest individual event
// score while preserving chronological output order.
func TopEvents(events []Event, limit int) []Event {
	if limit <= 0 {
		return events
	}
	type seriesScore struct {
		key   string
		score float64
	}
	scores := make(map[string]float64)
	for _, event := range events {
		key := labelsKey(event.Labels)
		scores[key] = math.Max(scores[key], event.Score)
	}
	ranked := make([]seriesScore, 0, len(scores))
	for key, score := range scores {
		ranked = append(ranked, seriesScore{key: key, score: score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].key < ranked[j].key
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	selected := make(map[string]struct{}, len(ranked))
	for _, series := range ranked {
		selected[series.key] = struct{}{}
	}
	result := make([]Event, 0, len(events))
	for _, event := range events {
		if _, ok := selected[labelsKey(event.Labels)]; ok {
			result = append(result, event)
		}
	}
	return result
}

func sortEvents(events []Event) {
	sort.Slice(events, func(i, j int) bool {
		if events[i].StartTime != events[j].StartTime {
			return events[i].StartTime < events[j].StartTime
		}
		if left, right := labelsKey(events[i].Labels), labelsKey(events[j].Labels); left != right {
			return left < right
		}
		return events[i].Type < events[j].Type
	})
}

func labelsKey(labels []*typesv1.LabelPair) string {
	parts := make([]string, 0, len(labels))
	for _, label := range labels {
		if label != nil {
			parts = append(parts, label.Name+"="+label.Value)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "\xff")
}
