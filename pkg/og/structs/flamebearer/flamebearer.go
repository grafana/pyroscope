package flamebearer

import (
	"errors"
	"fmt"
	"sort"

	"github.com/grafana/pyroscope/pkg/og/storage/heatmap"
	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
	"github.com/grafana/pyroscope/pkg/og/storage/segment"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
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
	Heatmap  *Heatmap                          `json:"heatmap"`
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

type Heatmap struct {
	// Values matrix contain values that indicate count of value occurrences,
	// satisfying boundaries of X and Y bins: [StartTime:EndTime) and (MinValue:MaxValue].
	// A value can be accessed via Values[x][y], where:
	//   0 <= x < TimeBuckets, and
	//   0 <= y < ValueBuckets.
	Values [][]uint64 `json:"values"`
	// TimeBuckets denote number of bins on X axis.
	// Length of Values array.
	TimeBuckets int64 `json:"timeBuckets"`
	// ValueBuckets denote number of bins on Y axis.
	// Length of any item in the Values array.
	ValueBuckets int64 `json:"valueBuckets"`
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

type ProfileConfig struct {
	Name      string
	MaxNodes  int
	Metadata  metadata.Metadata
	Tree      *tree.Tree
	Timeline  *segment.Timeline
	Heatmap   *heatmap.Heatmap
	Groups    map[string]*segment.Timeline
	Telemetry map[string]interface{}
}

func NewProfile(in ProfileConfig) FlamebearerProfile {
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

func NewCombinedProfile(base, diff ProfileConfig) (FlamebearerProfile, error) {
	if base.Metadata.Units != diff.Metadata.Units {
		// if one of them is empty, it still makes sense merging the profiles
		if base.Metadata.Units != "" && diff.Metadata.Units != "" {
			msg := fmt.Sprintf("left units (%s) does not match right units (%s)", base.Metadata.Units, diff.Metadata.Units)
			return FlamebearerProfile{}, errors.New(msg)
		}
	}

	if base.Metadata.SampleRate != diff.Metadata.SampleRate {
		// if one of them is empty, it still makes sense merging the profiles
		if base.Metadata.SampleRate != 0 && diff.Metadata.SampleRate != 0 {
			msg := fmt.Sprintf("left sample rate (%d) does not match right sample rate (%d)", base.Metadata.SampleRate, diff.Metadata.SampleRate)
			return FlamebearerProfile{}, errors.New(msg)
		}
	}

	// Figure out the non empty one, since we will use its attributes
	// Notice that this does not handle when both are empty, since there's nothing todo
	output := base
	if isEmpty(base) {
		output = diff
	}

	lt, rt := tree.CombineTree(base.Tree, diff.Tree)
	fb := tree.CombineToFlamebearerStruct(lt, rt, base.MaxNodes)
	return FlamebearerProfile{
		Version: 1,
		FlamebearerProfileV1: FlamebearerProfileV1{
			Flamebearer: newFlambearer(fb),
			Timeline:    nil,
			LeftTicks:   lt.Samples(),
			RightTicks:  rt.Samples(),
			Metadata: newMetadata(base.Name, fb.Format, metadata.Metadata{
				SpyName:         output.Metadata.SpyName,
				SampleRate:      output.Metadata.SampleRate,
				Units:           output.Metadata.Units,
				AggregationType: output.Metadata.AggregationType,
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

func newHeatmap(h *heatmap.Heatmap) *Heatmap {
	if h == nil {
		return nil
	}
	return &Heatmap{
		Values:       h.Values,
		TimeBuckets:  h.TimeBuckets,
		ValueBuckets: h.ValueBuckets,
		StartTime:    h.StartTime.UnixNano(),
		EndTime:      h.EndTime.UnixNano(),
		MinValue:     h.MinValue,
		MaxValue:     h.MaxValue,
		MinDepth:     h.MinDepth,
		MaxDepth:     h.MaxDepth,
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
			if l[i] < 0 {
				return fmt.Errorf("invalid name index %d, it should be a non-negative value", l[i])
			}
		}
	}
	return nil
}

func isEmpty(p ProfileConfig) bool {
	return p.Metadata.SampleRate == 0 || p.Tree == nil || p.Tree.Samples() == 0
}

// Diff takes two single profiles and generates a diff profile
func Diff(name string, base, diff *FlamebearerProfile, maxNodes int) (FlamebearerProfile, error) {
	var fb FlamebearerProfile
	bt, err := ProfileToTree(*base)
	if err != nil {
		return fb, fmt.Errorf("unable to convert base profile to tree: %w", err)
	}
	dt, err := ProfileToTree(*diff)
	if err != nil {
		return fb, fmt.Errorf("unable to convert diff profile to tree: %w", err)
	}
	baseProfile := ProfileConfig{
		Name:     name,
		Tree:     bt,
		MaxNodes: maxNodes,
		Metadata: metadata.Metadata{
			SpyName:    base.Metadata.SpyName,
			SampleRate: base.Metadata.SampleRate,
			Units:      base.Metadata.Units,
		},
	}
	diffProfile := ProfileConfig{
		Name:     name,
		Tree:     dt,
		MaxNodes: maxNodes,
		Metadata: metadata.Metadata{
			SpyName:    diff.Metadata.SpyName,
			SampleRate: diff.Metadata.SampleRate,
			Units:      diff.Metadata.Units,
		},
	}
	return NewCombinedProfile(baseProfile, diffProfile)
}

// ProfileToTree converts a FlamebearerProfile into a Tree
// It currently only supports Single profiles
func ProfileToTree(fb FlamebearerProfile) (*tree.Tree, error) {
	if fb.Metadata.Format != string(tree.FormatSingle) {
		return nil, fmt.Errorf("unsupported flamebearer format %s", fb.Metadata.Format)
	}
	if fb.Version != 1 {
		return nil, fmt.Errorf("unsupported flamebearer version %d", fb.Version)
	}

	return flamebearerV1ToTree(fb.Flamebearer)
}

func flamebearerV1ToTree(fb FlamebearerV1) (*tree.Tree, error) {
	t := tree.New()
	deltaDecoding(fb.Levels, 0, 4)
	for i, l := range fb.Levels {
		if i == 0 {
			// Skip the first level: it'll contain the root ("total") node..
			continue
		}
		for j := 0; j < len(l); j += 4 {
			self := l[j+2]
			if self > 0 {
				t.InsertStackString(buildStack(fb, i, j), uint64(self))
			}
		}
	}
	return t, nil
}

func deltaDecoding(levels [][]int, start, step int) {
	for _, l := range levels {
		prev := 0
		for i := start; i < len(l); i += step {
			delta := l[i] + l[i+1]
			l[i] += prev
			prev += delta
		}
	}
}

func buildStack(fb FlamebearerV1, level, idx int) []string {
	// The stack will contain names in the range [1, level].
	// Level 0 is not included as its the root ("total") node.
	stack := make([]string, level)
	stack[level-1] = fb.Names[fb.Levels[level][idx+3]]
	x := fb.Levels[level][idx]
	for i := level - 1; i > 0; i-- {
		j := sort.Search(len(fb.Levels[i])/4, func(j int) bool { return fb.Levels[i][j*4] > x }) - 1
		stack[i-1] = fb.Names[fb.Levels[i][j*4+3]]
		x = fb.Levels[i][j*4]
	}
	return stack
}
