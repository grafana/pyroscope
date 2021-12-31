package flamebearer

import (
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

//revive:disable:max-public-structs Config structs

// FlamebearerProfile is a versioned flambearer based profile.
// It's the native format both for rendering and file saving (in adhoc mode).
type FlamebearerProfile struct {
	Version uint `json:"version"`
	FlamebearerProfileV1
}

type FlamebearerProfileV1 struct {
	Flamebearer FlamebearerV1          `json:"flamebearer"`
	Metadata    FlamebearerMetadataV1  `json:"metadata"`
	Timeline    *FlamebearerTimelineV1 `json:"timeline"`
	LeftTicks   uint64                 `json:"leftTicks,omitempty"`
	RightTicks  uint64                 `json:"rightTicks,omitempty"`
}

type FlamebearerV1 struct {
	Names    []string `json:"names"`
	Levels   [][]int  `json:"levels"`
	NumTicks int      `json:"numTicks"`
	MaxSelf  int      `json:"maxSelf"`
}

type FlamebearerMetadataV1 struct {
	Format     string `json:"format"`
	SpyName    string `json:"spyName"`
	SampleRate uint32 `json:"sampleRate"`
	Units      string `json:"units"`
}

type FlamebearerTimelineV1 struct {
	StartTime     int64         `json:"startTime"`
	Samples       []uint64      `json:"samples"`
	DurationDelta int64         `json:"durationDelta"`
	Watermarks    map[int]int64 `json:"watermarks"`
}

func NewProfile(output *storage.GetOutput, maxNodes int) FlamebearerProfile {
	fb := output.Tree.FlamebearerStruct(maxNodes)
	return FlamebearerProfile{
		Version: 1,
		FlamebearerProfileV1: FlamebearerProfileV1{
			Flamebearer: newFlambearer(fb),
			Metadata:    newMetadata(fb.Format, output),
			Timeline:    newTimeline(output.Timeline),
		},
	}
}

func NewCombinedProfile(output, left, right *storage.GetOutput, maxNodes int) FlamebearerProfile {
	lt, rt := tree.CombineTree(left.Tree, right.Tree)
	fb := tree.CombineToFlamebearerStruct(lt, rt, maxNodes)
	return FlamebearerProfile{
		Version: 1,
		FlamebearerProfileV1: FlamebearerProfileV1{
			Flamebearer: newFlambearer(fb),
			Metadata:    newMetadata(fb.Format, output),
			Timeline:    newTimeline(output.Timeline),
			LeftTicks:   lt.Samples(),
			RightTicks:  rt.Samples(),
		},
	}
}

func newFlambearer(fb *tree.Flamebearer) FlamebearerV1 {
	return FlamebearerV1{
		Names:    fb.Names,
		Levels:   fb.Levels,
		NumTicks: fb.NumTicks,
		MaxSelf:  fb.MaxSelf,
	}
}

func newMetadata(format tree.Format, output *storage.GetOutput) FlamebearerMetadataV1 {
	return FlamebearerMetadataV1{
		Format:     string(format),
		SpyName:    output.SpyName,
		SampleRate: output.SampleRate,
		Units:      output.Units,
	}
}

func newTimeline(timeline *segment.Timeline) *FlamebearerTimelineV1 {
	if timeline == nil {
		return nil
	}
	return &FlamebearerTimelineV1{
		StartTime:     timeline.StartTime,
		Samples:       timeline.Samples,
		DurationDelta: timeline.DurationDeltaNormalized,
		Watermarks:    timeline.Watermarks,
	}
}
