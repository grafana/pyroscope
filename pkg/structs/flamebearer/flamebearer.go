package flamebearer

import (
	"errors"
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

//revive:disable:max-public-structs Config structs

// swagger:model
// FlamebearerProfile is a versioned flambearer based profile.
// It's the native format both for rendering and file saving (in adhoc mode).
type FlamebearerProfile struct {
	// Version of the data format. No version / version zero is an unformalized format.
	Version uint `json:"version"`
	FlamebearerProfileV1
}

// swagger:model
// FlamebearerProfileV1 defines the v1 of the profile format
type FlamebearerProfileV1 struct {
	// Flamebearer data.
	// required: true
	Flamebearer FlamebearerV1 `json:"flamebearer"`
	// Metadata associated to the profile.
	// required: true
	Metadata FlamebearerMetadataV1 `json:"metadata"`
	// Timeline associated to the profile, used for continuous profiling only.
	Timeline *FlamebearerTimelineV1 `json:"timeline"`
	// Number of samples in the left / base profile. Only used in "double" format.
	LeftTicks uint64 `json:"leftTicks,omitempty"`
	// Number of samples in the right / diff profile. Only used in "double" format.
	RightTicks uint64 `json:"rightTicks,omitempty"`
}

// swagger:model
// FlamebearerV1 defines the actual profiling data.
type FlamebearerV1 struct {
	// Names is the sequence of symbol names.
	// required: true
	Names []string `json:"names"`
	// Levels contains the flamebearer nodes. Each level represents a row in the flamegraph.
	// For each row / level, there's a sequence of values. These values are grouped in chunks
	// which size depend on the flamebearer format: 4 for "single", 7 for "double".
	// For "single" format, each chunk has the following data:
	//     i+0 = x offset (prefix sum of the level total values), delta encoded.
	//     i+1 = total samples (including the samples in its children nodes).
	//     i+2 = self samples (excluding the samples in its children nodes).
	//     i+3 = index in names array
	//
	// For "double" format, each chunk has the following data:
	//     i+0 = x offset (prefix sum of the level total values), delta encoded, base / left tree.
	//     i+1 = total samples (including the samples in its children nodes)   , base / left tree.
	//     i+2 = self samples (excluding the samples in its children nodes)    , base / left tree.
	//     i+3 = x offset (prefix sum of the level total values), delta encoded, diff / right tree.
	//     i+4 = total samples (including the samples in its children nodes)   , diff / right tree.
	//     i+5 = self samples (excluding the samples in its children nodes)    , diff / right tree.
	//     i+6 = index in the names array
	//
	// required: true
	Levels [][]int `json:"levels"`
	// Total number of samples.
	// required: true
	NumTicks int `json:"numTicks"`
	// Maximum self value in any node.
	// required: true
	MaxSelf int `json:"maxSelf"`
}

type FlamebearerMetadataV1 struct {
	// Data format. Supported values are "single" and "double" (diff).
	// required: true
	Format string `json:"format"`
	// Name of the spy / profiler used to generate the profile, if any.
	SpyName string `json:"spyName"`
	// Sample rate at which the profiler was operating.
	SampleRate uint32 `json:"sampleRate"`
	// The unit of measurement for the profiled data.
	Units string `json:"units"`
	// A name that identifies the profile.
	Name string `json:"name"`
}

type FlamebearerTimelineV1 struct {
	// Time at which the timeline starts, as a Unix timestamp.
	// required: true
	StartTime int64 `json:"startTime"`
	// A sequence of samples starting at startTime, spaced by durationDelta seconds
	// required: true
	Samples []uint64 `json:"samples"`
	// Time delta between samples, in seconds.
	// required: true
	DurationDelta int64         `json:"durationDelta"`
	Watermarks    map[int]int64 `json:"watermarks"`
}

func NewProfile(name string, output *storage.GetOutput, maxNodes int) FlamebearerProfile {
	fb := output.Tree.FlamebearerStruct(maxNodes)
	return FlamebearerProfile{
		Version: 1,
		FlamebearerProfileV1: FlamebearerProfileV1{
			Flamebearer: newFlambearer(fb),
			Metadata:    newMetadata(name, fb.Format, output),
			Timeline:    NewTimeline(output.Timeline),
		},
	}
}

func NewCombinedProfile(name string, left, right *storage.GetOutput, maxNodes int) (FlamebearerProfile, error) {
	if left.Units != right.Units {
		// if one of them is empty, it still makes sense merging the profiles
		if left.Units != "" && right.Units != "" {
			msg := fmt.Sprintf("left units (%s) does not match right units (%s)", left.Units, right.Units)
			return FlamebearerProfile{}, errors.New(msg)
		}
	}

	if left.SampleRate != right.SampleRate {
		// if one of them is empty, it still makes sense merging the profiles
		if left.SampleRate != 0 && right.SampleRate != 0 {
			msg := fmt.Sprintf("left sample rate (%d) does not match right sample rate (%d)", left.SampleRate, right.SampleRate)
			return FlamebearerProfile{}, errors.New(msg)
		}
	}

	// Figure out the non empty one, since we will use its attributes
	// Notice that this does not handle when both are empty, since there's nothing todo
	nonEmptyOne := left
	if isEmpty(left) {
		nonEmptyOne = right
	}

	lt, rt := tree.CombineTree(left.Tree, right.Tree)
	fb := tree.CombineToFlamebearerStruct(lt, rt, maxNodes)
	return FlamebearerProfile{
		Version: 1,
		FlamebearerProfileV1: FlamebearerProfileV1{
			Flamebearer: newFlambearer(fb),
			Metadata:    newMetadata(name, fb.Format, nonEmptyOne),
			Timeline:    nil,
			LeftTicks:   lt.Samples(),
			RightTicks:  rt.Samples(),
		},
	}, nil
}

func newFlambearer(fb *tree.Flamebearer) FlamebearerV1 {
	return FlamebearerV1{
		Names:    fb.Names,
		Levels:   fb.Levels,
		NumTicks: fb.NumTicks,
		MaxSelf:  fb.MaxSelf,
	}
}

func newMetadata(name string, format tree.Format, output *storage.GetOutput) FlamebearerMetadataV1 {
	return FlamebearerMetadataV1{
		Name:       name,
		Format:     string(format),
		SpyName:    output.SpyName,
		SampleRate: output.SampleRate,
		Units:      output.Units,
	}
}

func NewTimeline(timeline *segment.Timeline) *FlamebearerTimelineV1 {
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

func isEmpty(t *storage.GetOutput) bool {
	// TODO: improve heuristic
	return t.SampleRate == 0
}
