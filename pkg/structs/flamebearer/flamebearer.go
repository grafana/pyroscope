package flamebearer

import (
	"errors"
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
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
	Telemetry map[string]interface{} `json:"telemetry,omitempty"`
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
	Timeline *FlamebearerTimelineV1            `json:"timeline"`
	Groups   map[string]*FlamebearerTimelineV1 `json:"groups"`
	Heatmap  *FlamebearerHeatmapV1             `json:"heatmap"`
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
	Units metadata.Units `json:"units"`
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

type FlamebearerHeatmapV1 struct {
	// Values matrix contain values that indicate count of value occurrences,
	// satisfying boundaries of X and Y bins: [StartTime:EndTime) and (MinValue:MaxValue].
	// A value can be accessed via Values[x][y], where:
	//   0 <= x < TimeBuckets, and
	//   0 <= y < ValueBuckets.
	Values [][]uint64 `json:"values"`
	// TimeBuckets denote number of bins on X axis.
	// Length of Values array.
	TimeBuckets int `json:"timeBuckets"`
	// ValueBuckets denote number of bins on Y axis.
	// Length of any item in the Values array.
	ValueBuckets int `json:"valueBuckets"`
	// StartTime and EndTime indicate boundaries of X axis: [StartTime:EndTime).
	StartTime int64 `json:"startTime"`
	EndTime   int64 `json:"endTime"`
	// MinValue and MaxValue indicate boundaries of Y axis: (MinValue:MaxValue].
	MinValue uint64 `json:"minValue"`
	MaxValue uint64 `json:"maxValue"`
	// MinDepth and MaxDepth indicate boundaries of Z axis: [MinDepth:MaxDepth].
	// MinDepth is the minimal non-zero value that can be found in Values.
	MinDepth uint64 `json:"minDepth"`
	MaxDepth uint64 `json:"maxDepth"`
}

func NewProfile(name string, output *storage.GetOutput, maxNodes int) FlamebearerProfile {
	fb := output.Tree.FlamebearerStruct(maxNodes)
	return FlamebearerProfile{
		Version:   1,
		Telemetry: output.Telemetry,
		FlamebearerProfileV1: FlamebearerProfileV1{
			Flamebearer: newFlambearer(fb),
			Timeline:    newTimeline(output.Timeline),
			Groups:      convertGroups(output.Groups),
			Metadata: newMetadata(name, fb.Format, metadata.Metadata{
				SpyName:         output.SpyName,
				SampleRate:      output.SampleRate,
				Units:           output.Units,
				AggregationType: output.AggregationType,
			}),
		},
	}
}

type ProfileConfig struct {
	Name      string
	MaxNodes  int
	Metadata  metadata.Metadata
	Tree      *tree.Tree
	Timeline  *segment.Timeline
	Heatmap   *storage.Heatmap
	Groups    map[string]*segment.Timeline
	Telemetry map[string]interface{}
}

func NewProfileWithConfig(in ProfileConfig) FlamebearerProfile {
	fb := in.Tree.FlamebearerStruct(in.MaxNodes)
	return FlamebearerProfile{
		Version:   1,
		Telemetry: in.Telemetry,
		FlamebearerProfileV1: FlamebearerProfileV1{
			Flamebearer: newFlambearer(fb),
			Metadata:    newMetadata(in.Name, fb.Format, in.Metadata),
			Timeline:    newTimeline(in.Timeline),
			Heatmap:     newHeatmap(in.Heatmap),
			Groups:      convertGroups(in.Groups),
		},
	}
}

func convertGroups(v map[string]*segment.Timeline) map[string]*FlamebearerTimelineV1 {
	res := make(map[string]*FlamebearerTimelineV1)
	for k, v := range v {
		res[k] = newTimeline(v)
	}
	return res
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
	output := left
	if isEmpty(left) {
		output = right
	}

	lt, rt := tree.CombineTree(left.Tree, right.Tree)
	fb := tree.CombineToFlamebearerStruct(lt, rt, maxNodes)
	return FlamebearerProfile{
		Version: 1,
		FlamebearerProfileV1: FlamebearerProfileV1{
			Flamebearer: newFlambearer(fb),
			Timeline:    nil,
			LeftTicks:   lt.Samples(),
			RightTicks:  rt.Samples(),
			Metadata: newMetadata(name, fb.Format, metadata.Metadata{
				SpyName:         output.SpyName,
				SampleRate:      output.SampleRate,
				Units:           output.Units,
				AggregationType: output.AggregationType,
			}),
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

func newMetadata(name string, format tree.Format, md metadata.Metadata) FlamebearerMetadataV1 {
	return FlamebearerMetadataV1{
		Name:       name,
		Format:     string(format),
		SpyName:    md.SpyName,
		SampleRate: md.SampleRate,
		Units:      md.Units,
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

func newHeatmap(heatmap *storage.Heatmap) *FlamebearerHeatmapV1 {
	if heatmap == nil {
		return nil
	}
	return &FlamebearerHeatmapV1{
		Values:       heatmap.Values,
		TimeBuckets:  heatmap.TimeBuckets,
		ValueBuckets: heatmap.ValueBuckets,
		StartTime:    heatmap.StartTime.Unix(),
		EndTime:      heatmap.EndTime.Unix(),
		MinValue:     heatmap.MinValue,
		MaxValue:     heatmap.MaxValue,
		MinDepth:     heatmap.MinDepth,
		MaxDepth:     heatmap.MaxDepth,
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
