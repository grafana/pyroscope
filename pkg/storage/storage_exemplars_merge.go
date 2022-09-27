package storage

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage/heatmap"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type MergeExemplarsInput struct {
	AppName    string
	ProfileIDs []string
	StartTime  time.Time
	EndTime    time.Time

	// FIXME: Not implemented: parameters are ignored.
	ExemplarsSelection ExemplarsSelection
	HeatmapParams      heatmap.HeatmapParams
}

type MergeExemplarsOutput struct {
	Tree          *tree.Tree
	Count         uint64
	Metadata      metadata.Metadata
	HeatmapSketch heatmap.HeatmapSketch // FIXME: Not implemented: the field is never populated.
	Telemetry     map[string]interface{}
}

func (s *Storage) MergeExemplars(ctx context.Context, mi MergeExemplarsInput) (out MergeExemplarsOutput, err error) {
	m, err := s.mergeExemplars(ctx, mi)
	if err != nil {
		return out, err
	}

	out.Tree = m.tree
	out.Count = m.count
	if m.segment != nil {
		out.Metadata = m.segment.GetMetadata()
	}

	if out.Count > 1 && out.Metadata.AggregationType == metadata.AverageAggregationType {
		out.Tree = out.Tree.Clone(big.NewRat(1, int64(out.Count)))
	}

	return out, nil
}

type exemplarsMerge struct {
	tree      *tree.Tree
	count     uint64
	segment   *segment.Segment
	lastEntry *exemplarEntry
}

func (s *Storage) mergeExemplars(ctx context.Context, mi MergeExemplarsInput) (out exemplarsMerge, err error) {
	out.tree = tree.New()
	startTime := unixNano(mi.StartTime)
	endTime := unixNano(mi.EndTime)
	err = s.exemplars.fetch(ctx, mi.AppName, mi.ProfileIDs, func(e exemplarEntry) error {
		if exemplarMatchesTimeRange(e, startTime, endTime) {
			out.tree.Merge(e.Tree)
			out.count++
			out.lastEntry = &e
		}
		return nil
	})
	if err != nil || out.lastEntry == nil {
		return out, err
	}
	// Note that exemplar entry labels don't contain the app name and profile ID.
	if out.lastEntry.Labels == nil {
		out.lastEntry.Labels = make(map[string]string)
	}
	r, ok := s.segments.Lookup(segment.AppSegmentKey(mi.AppName))
	if !ok {
		return out, fmt.Errorf("no metadata found for app %q", mi.AppName)
	}
	out.segment = r.(*segment.Segment)
	return out, nil
}

func unixNano(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

// exemplarMatchesTimeRange reports whether the exemplar is eligible for the
// given time range. Potentially, we could take exact fraction and scale the
// exemplar proportionally, in the way we do it in aggregate queries. However,
// with exemplars down-sampling does not seem to be a good idea as it may be
// confusing.
//
// For backward compatibility, an exemplar is considered eligible if the time
// range is not specified, or if the exemplar does not have timestamps.
func exemplarMatchesTimeRange(e exemplarEntry, startTime, endTime int64) bool {
	if startTime == 0 || endTime == 0 || e.StartTime == 0 || e.EndTime == 0 {
		return true
	}
	return !math.IsNaN(overlap(startTime, endTime, e.StartTime, e.EndTime))
}

// overlap returns the overlap of the ranges
// indicating the exemplar time range fraction.
//
//   query:    from  – until
//   exemplar: start – end
//
// Special cases:
//   +Inf - query matches or includes exemplar
//    NaN - ranges don't overlap
//
func overlap(from, until, start, end int64) float64 {
	span := end - start
	o := min(until, end) - max(from, start)
	switch {
	case o <= 0:
		return math.NaN()
	case o == span:
		return math.Inf(0)
	default:
		return float64(o) / float64(span)
	}
}

func min(a, b int64) int64 {
	if b < a {
		return b
	}
	return a
}

func max(a, b int64) int64 {
	if b > a {
		return b
	}
	return a
}
