package storage

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type MergeExemplarsInput struct {
	AppName    string
	StartTime  time.Time
	EndTime    time.Time
	ProfileIDs []string
}

type MergeExemplarsOutput struct {
	Tree  *tree.Tree
	Count uint64

	SpyName         string
	SampleRate      uint32
	Units           metadata.Units
	AggregationType metadata.AggregationType

	Telemetry map[string]interface{}
}

func (s *Storage) MergeExemplars(ctx context.Context, mi MergeExemplarsInput) (out MergeExemplarsOutput, err error) {
	m, err := s.mergeExemplars(ctx, mi)
	if err != nil {
		return out, err
	}

	out.Tree = m.tree
	out.Count = m.count
	if m.segment != nil {
		out.SpyName = m.segment.SpyName()
		out.Units = m.segment.Units()
		out.SampleRate = m.segment.SampleRate()
		out.AggregationType = m.segment.AggregationType()
	}

	if out.Count > 1 && out.AggregationType == metadata.AverageAggregationType {
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
	err = s.exemplars.fetch(ctx, mi.AppName, mi.ProfileIDs, func(e exemplarEntry) error {
		out.tree.Merge(e.Tree)
		out.count++
		out.lastEntry = &e
		return nil
	})
	if err != nil || out.lastEntry == nil {
		return out, err
	}
	out.segment, err = s.findSegmentForExemplar(out.lastEntry)
	return out, err
}

func (s *Storage) findSegmentForExemplar(e *exemplarEntry) (*segment.Segment, error) {
	// Note that exemplar entry labels doesn't contain the app name and profile ID.
	if e.Labels == nil {
		e.Labels = make(map[string]string)
	}
	labels := map[string]string{"__name__": e.AppName}
	for k, v := range e.Labels {
		labels[k] = v
	}
	r, ok := s.segments.Lookup(segment.NewKey(labels).Normalized())
	if !ok {
		return nil, fmt.Errorf("no metadata found for profile %q", e.ProfileID)
	}
	return r.(*segment.Segment), nil
}
