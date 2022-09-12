package storage

import (
	"context"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
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
	Metadata  metadata.Metadata

	Telemetry map[string]interface{}
}

func (s *Storage) GetExemplar(ctx context.Context, gi GetExemplarInput) (out GetExemplarOutput, err error) {
	m, err := s.mergeExemplars(ctx, MergeExemplarsInput{
		AppName:    gi.AppName,
		StartTime:  gi.StartTime,
		EndTime:    gi.EndTime,
		ProfileIDs: []string{gi.ProfileID},
	})
	if err != nil {
		return out, err
	}

	out.Tree = m.tree
	if m.lastEntry != nil {
		out.Labels = m.lastEntry.Labels
		out.StartTime = time.Unix(0, m.lastEntry.StartTime)
		out.EndTime = time.Unix(0, m.lastEntry.EndTime)
	}

	if m.segment != nil {
		out.Metadata = m.segment.GetMetadata()
	}

	return out, nil
}
