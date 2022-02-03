package flamebearer

import (
	"fmt"

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

func (fb FlamebearerProfile) Validate() error {
	if fb.Version > 1 {
		return fmt.Errorf("unsupported version %d", fb.Version)
	}
	return fb.FlamebearerProfileV1.Validate()
}

// Validate the V1 profile.
// A custom validation is used as the constraints are hard to define in a generic way
// (e.g. using https://github.com/go-playground/validator)
func (fb FlamebearerProfileV1) Validate() error {
	format := tree.Format(fb.Metadata.Format)
	if format != tree.FormatSingle && format != tree.FormatDouble {
		return fmt.Errorf("unsupported format %s", format)
	}
	if len(fb.Flamebearer.Names) == 0 {
		return fmt.Errorf("a profile must have at least one symbol name")
	}
	if len(fb.Flamebearer.Levels) == 0 {
		return fmt.Errorf("a profile must have at least one profiling level")
	}
	var mod int
	switch format {
	case tree.FormatSingle:
		mod = 4
	case tree.FormatDouble:
		mod = 7
	default: // This shouldn't happen at this point.
		return fmt.Errorf("unsupported format %s", format)
	}
	for _, l := range fb.Flamebearer.Levels {
		if len(l)%mod != 0 {
			return fmt.Errorf("a profile level should have a multiple of %d values, but there's a level with %d values", mod, len(l))
		}
		for i := mod - 1; i < len(l); i += mod {
			if l[i] >= len(fb.Flamebearer.Names) {
				return fmt.Errorf("invalid name index %d, it should be smaller than %d", l[i], len(fb.Flamebearer.Levels))
			}
		}
	}
	return nil
}
