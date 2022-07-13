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

func (s *Storage) MergeProfiles(ctx context.Context, mi MergeExemplarsInput) (out MergeExemplarsOutput, err error) {
	var lastEntry *exemplarEntry
	out.Tree = tree.New()
	err = s.exemplars.fetch(ctx, mi.AppName, mi.ProfileIDs, func(e exemplarEntry) error {
		out.Tree.Merge(e.Tree)
		out.Count++
		lastEntry = &e
		return nil
	})
	if err != nil {
		return out, err
	}

	lastEntry.Labels["__name__"] = mi.AppName
	r, ok := s.segments.Lookup(segment.NewKey(lastEntry.Labels).Normalized())
	if !ok {
		return out, fmt.Errorf("no metadata found for profile %q", lastEntry.ProfileID)
	}

	seg := r.(*segment.Segment)
	out.SpyName = seg.SpyName()
	out.Units = seg.Units()
	out.SampleRate = seg.SampleRate()
	out.AggregationType = seg.AggregationType()

	if out.Count > 0 && out.AggregationType == metadata.AverageAggregationType {
		out.Tree = out.Tree.Clone(big.NewRat(1, int64(out.Count)))
	}

	return out, nil
}
