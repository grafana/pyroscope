package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type GetExemplarInput struct {
	StartTime time.Time
	EndTime   time.Time
	AppName   string
	ProfileID string
}

type GetExemplarOutput struct {
	Tree      *tree.Tree
	Labels    map[string]string
	StartTime time.Time
	EndTime   time.Time

	SpyName         string
	SampleRate      uint32
	Units           metadata.Units
	AggregationType metadata.AggregationType

	Telemetry map[string]interface{}
}

func (s *Storage) GetExemplar(ctx context.Context, gi GetExemplarInput) (GetExemplarOutput, error) {
	var out GetExemplarOutput
	err := s.exemplars.fetch(ctx, gi.AppName, []string{gi.ProfileID}, func(e exemplarEntry) error {
		out.Tree = e.Tree
		out.StartTime = time.Unix(0, e.StartTime)
		out.EndTime = time.Unix(0, e.EndTime)
		out.Labels = e.Labels
		return nil
	})
	if err != nil {
		return out, err
	}

	// Exemplar labels map does not contain the app name.
	out.Labels["__name__"] = gi.AppName
	r, ok := s.segments.Lookup(segment.NewKey(out.Labels).Normalized())
	if !ok {
		return out, fmt.Errorf("no metadata found for profile %q", gi.ProfileID)
	}

	seg := r.(*segment.Segment)
	out.SpyName = seg.SpyName()
	out.Units = seg.Units()
	out.SampleRate = seg.SampleRate()
	out.AggregationType = seg.AggregationType()

	return out, nil
}
