package sampling

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
)

const regionLabel = "region"

type fakeRuler struct {
	rules map[string][]*phlaremodel.RecordingRule
}

func (r fakeRuler) RecordingRules(tenant string) []*phlaremodel.RecordingRule {
	return r.rules[tenant]
}

// stripTestProfile returns a deterministic profile without sample labels,
// so it maps to a single series.
func stripTestProfile() *profilev1.Profile {
	return &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 1, Unit: 2},
			{Type: 3, Unit: 4},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{1, 2}, Value: []int64{10, 1000}},
			{LocationId: []uint64{2}, Value: []int64{5, 500}},
			// Dropped by Normalize (negative value): must not count towards totals.
			{LocationId: []uint64{1}, Value: []int64{-1, 100}},
			// Dropped by sanitization (value length mismatch): must not count either.
			{LocationId: []uint64{1}, Value: []int64{7}},
		},
		Mapping: []*profilev1.Mapping{{Id: 1, HasFunctions: true}},
		Location: []*profilev1.Location{
			{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1}}},
			{Id: 2, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 2}}},
		},
		Function: []*profilev1.Function{
			{Id: 1, Name: 5, SystemName: 5, Filename: 6},
			{Id: 2, Name: 7, SystemName: 7, Filename: 8},
		},
		StringTable: []string{
			"",
			"samples",
			"count",
			"cpu",
			"nanoseconds",
			"func-a",
			"app.py",
			"func-b",
			"path-b",
		},
		TimeNanos:     1000000000,
		DurationNanos: 10000000000,
		PeriodType:    &profilev1.ValueType{Type: 3, Unit: 4},
		Period:        10000000,
	}
}

// strippedTestProfile is what stripTestProfile must be reduced to when it
// is sampled out: a single sample with the totals and a string table that
// holds only the sample type units.
func strippedTestProfile() *profilev1.Profile {
	return &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 1, Unit: 2},
			{Type: 3, Unit: 4},
		},
		Sample:        []*profilev1.Sample{{Value: []int64{15, 1500}}},
		StringTable:   []string{"", "samples", "count", "cpu", "nanoseconds"},
		TimeNanos:     1000000000,
		DurationNanos: 10000000000,
		PeriodType:    &profilev1.ValueType{Type: 3, Unit: 4},
		Period:        10000000,
	}
}

func TestProfileStripper_Strip(t *testing.T) {
	p := stripTestProfile()
	NewProfileStripper(nil).Strip("", nil, p)

	want := strippedTestProfile()
	require.Len(t, p.Sample, 1)
	assert.Equal(t, want.Sample[0].Value, p.Sample[0].Value)
	assert.Empty(t, p.Sample[0].LocationId)
	assert.Empty(t, p.Sample[0].Label)
	assert.Empty(t, p.Location)
	assert.Empty(t, p.Function)
	assert.Empty(t, p.Mapping)
	assert.Equal(t, want.StringTable, p.StringTable)
	assert.Equal(t, want.SampleType, p.SampleType)
	assert.Equal(t, want.PeriodType, p.PeriodType)
	assert.Equal(t, want.SizeVT(), p.SizeVT())
}

func TestProfileStripper_Strip_NoSamples(t *testing.T) {
	p := stripTestProfile()
	p.Sample = nil
	NewProfileStripper(nil).Strip("", nil, p)
	assert.Empty(t, p.Sample)
	assert.Empty(t, p.Location)
}

func TestProfileStripper_Strip_AllSamplesInvalid(t *testing.T) {
	p := stripTestProfile()
	p.Sample = []*profilev1.Sample{
		{LocationId: []uint64{1}, Value: []int64{-1, 100}},
		{LocationId: []uint64{1}, Value: []int64{7}},
	}
	NewProfileStripper(nil).Strip("", nil, p)
	assert.Empty(t, p.Sample)
	assert.Empty(t, p.Location)
}

func TestProfileStripper_Strip_SampleLabels(t *testing.T) {
	p := stripTestProfile()
	// Indices into the extended string table: span_id=9, abc=10, def=11.
	p.StringTable = append(p.StringTable, "span_id", "abc", "def")
	p.Sample = []*profilev1.Sample{
		{LocationId: []uint64{1}, Value: []int64{1, 10}, Label: []*profilev1.Label{{Key: 9, Str: 10}}},
		{LocationId: []uint64{2}, Value: []int64{2, 20}, Label: []*profilev1.Label{{Key: 9, Str: 10}}},
		{LocationId: []uint64{1}, Value: []int64{4, 40}, Label: []*profilev1.Label{{Key: 9, Str: 11}}},
		{LocationId: []uint64{2}, Value: []int64{8, 80}},
		{LocationId: []uint64{1}, Value: []int64{16, 160}, Label: []*profilev1.Label{{Key: 9, Num: 42}}},
	}

	NewProfileStripper(nil).Strip("", nil, p)

	require.Len(t, p.Sample, 1)
	assert.Equal(t, []int64{31, 310}, p.Sample[0].Value)
	assert.Empty(t, p.Sample[0].Label)
	assert.Empty(t, p.Sample[0].LocationId)
	assert.Empty(t, p.Location)
	assert.Empty(t, p.Function)
	assert.Empty(t, p.Mapping)
	assert.Equal(t, []string{"", "samples", "count", "cpu", "nanoseconds"}, p.StringTable)
}

// A rule targeting func-a keeps the stacktraces that reach it (trimmed to the
// func-a frame), folds the rest into the totals, and leaves only the referenced
// symbols, re-interned.
func TestProfileStripper_Strip_KeepsTargetedFunction(t *testing.T) {
	p := stripTestProfile()
	ruler := fakeRuler{rules: map[string][]*phlaremodel.RecordingRule{
		"tenant-a": {{FunctionName: "func-a"}},
	}}
	NewProfileStripper(ruler).Strip("tenant-a", nil, p)

	require.Len(t, p.Sample, 2)
	// Kept targeted sample first, then the totals of the untargeted samples.
	kept, total := p.Sample[0], p.Sample[1]
	assert.Equal(t, []int64{10, 1000}, kept.Value)
	assert.Equal(t, []uint64{1}, kept.LocationId)
	assert.Empty(t, kept.Label)
	assert.Equal(t, []int64{5, 500}, total.Value)
	assert.Empty(t, total.LocationId)

	require.Len(t, p.Location, 1)
	require.Len(t, p.Function, 1)
	require.Len(t, p.Mapping, 1)
	assert.Equal(t, "func-a", p.StringTable[p.Function[0].Name])
	assert.Equal(t, "app.py", p.StringTable[p.Function[0].Filename])
	assert.Equal(t, []string{"", "samples", "count", "cpu", "nanoseconds", "func-a", "app.py"}, p.StringTable)
}

// Rules that don't target any function present in the profile leave the strip
// identical to a plain strip-to-totals.
func TestProfileStripper_Strip_RulesNoMatch(t *testing.T) {
	p := stripTestProfile()
	ruler := fakeRuler{rules: map[string][]*phlaremodel.RecordingRule{
		"tenant-a": {{FunctionName: "not-present"}},
	}}
	NewProfileStripper(ruler).Strip("tenant-a", nil, p)

	want := strippedTestProfile()
	require.Len(t, p.Sample, 1)
	assert.Equal(t, want.Sample[0].Value, p.Sample[0].Value)
	assert.Empty(t, p.Location)
	assert.Empty(t, p.Function)
	assert.Empty(t, p.Mapping)
	assert.Equal(t, want.StringTable, p.StringTable)
}

// A matcher on a label absent from the series labels is deferred to the sample
// labels: only samples that satisfy it are kept, and only the labels the rule
// references (matchers + group_by) are retained.
func TestProfileStripper_Strip_DeferredSampleLabelMatcher(t *testing.T) {
	// String table: func-a=3, region=4, eu=5, us=6, env=7, prod=8, extra=9, x=10.
	p := &profilev1.Profile{
		SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 2}},
		StringTable: []string{"", "samples", "count", "func-a", regionLabel, "eu", "us", "env", "prod", "extra", "x"},
		Function:    []*profilev1.Function{{Id: 1, Name: 3}},
		Location:    []*profilev1.Location{{Id: 1, Line: []*profilev1.Line{{FunctionId: 1}}}},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{1}, Value: []int64{100}, Label: []*profilev1.Label{{Key: 4, Str: 5}, {Key: 7, Str: 8}, {Key: 9, Str: 10}}},
			{LocationId: []uint64{1}, Value: []int64{50}, Label: []*profilev1.Label{{Key: 4, Str: 6}}},
			{LocationId: []uint64{1}, Value: []int64{10}},
		},
	}
	ruler := fakeRuler{rules: map[string][]*phlaremodel.RecordingRule{
		"tenant-a": {{
			FunctionName: "func-a",
			GroupBy:      []string{"env"},
			Matchers:     []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, regionLabel, "eu")},
		}},
	}}

	NewProfileStripper(ruler).Strip("tenant-a", nil, p)

	require.Len(t, p.Sample, 2)
	kept, total := p.Sample[0], p.Sample[1]
	// region=eu is kept; region=us and the label-less sample fold into the total.
	assert.Equal(t, []int64{100}, kept.Value)
	require.Len(t, kept.LocationId, 1)
	// region (matcher) and env (group_by) retained, extra dropped, and their
	// string indices still resolve after the table was compacted.
	require.Len(t, kept.Label, 2)
	labelValues := map[string]string{}
	for _, l := range kept.Label {
		labelValues[p.StringTable[l.Key]] = p.StringTable[l.Str]
	}
	assert.Equal(t, map[string]string{regionLabel: "eu", "env": "prod"}, labelValues)
	assert.Equal(t, []int64{60}, total.Value)
	assert.Empty(t, total.LocationId)
}

// A non-function ("total") rule that groups by a sample label splits the totals
// into one sample per distinct value of that label, each carrying it, so the
// rule can still match and group after stripping.
func TestProfileStripper_Strip_TotalsSplitByNonFunctionRuleLabel(t *testing.T) {
	// String table: region=3, eu=4, us=5.
	p := &profilev1.Profile{
		SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 2}},
		StringTable: []string{"", "samples", "count", regionLabel, "eu", "us"},
		Sample: []*profilev1.Sample{
			{Value: []int64{10}, Label: []*profilev1.Label{{Key: 3, Str: 4}}},
			{Value: []int64{20}, Label: []*profilev1.Label{{Key: 3, Str: 4}}},
			{Value: []int64{5}, Label: []*profilev1.Label{{Key: 3, Str: 5}}},
			{Value: []int64{3}},
		},
	}
	ruler := fakeRuler{rules: map[string][]*phlaremodel.RecordingRule{
		"tenant-a": {{GroupBy: []string{regionLabel}}}, // no FunctionName -> total rule
	}}

	NewProfileStripper(ruler).Strip("tenant-a", nil, p)

	// One total per region value (eu=30, us=5) plus the region-less samples (3),
	// and the region label still resolves through the compacted string table.
	require.Len(t, p.Sample, 3)
	byRegion := map[string]int64{}
	for _, s := range p.Sample {
		assert.Empty(t, s.LocationId)
		region := ""
		for _, l := range s.Label {
			if p.StringTable[l.Key] == regionLabel {
				region = p.StringTable[l.Str]
			}
		}
		byRegion[region] = s.Value[0]
	}
	assert.Equal(t, map[string]int64{"eu": 30, "us": 5, "": 3}, byRegion)
	assert.Empty(t, p.Location)
}

// A non-function rule with a sample-label matcher only affects the samples that
// satisfy it: those keep the label (so the rule matches them downstream); the
// rest fold together into a single label-less total, not split by the label.
func TestProfileStripper_Strip_TotalsMatcherGatesLabelKeep(t *testing.T) {
	// String table: region=3, eu=4, us=5.
	p := &profilev1.Profile{
		SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 2}},
		StringTable: []string{"", "samples", "count", regionLabel, "eu", "us"},
		Sample: []*profilev1.Sample{
			{Value: []int64{10}, Label: []*profilev1.Label{{Key: 3, Str: 4}}}, // region=eu
			{Value: []int64{20}, Label: []*profilev1.Label{{Key: 3, Str: 4}}}, // region=eu
			{Value: []int64{5}, Label: []*profilev1.Label{{Key: 3, Str: 5}}},  // region=us
			{Value: []int64{3}}, // no region
		},
	}
	ruler := fakeRuler{rules: map[string][]*phlaremodel.RecordingRule{
		"tenant-a": {{Matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, regionLabel, "eu")}}},
	}}

	NewProfileStripper(ruler).Strip("tenant-a", nil, p)

	// region=eu samples keep region and merge (30); region=us and region-less
	// samples don't match, so they fold together into a label-less total (8).
	require.Len(t, p.Sample, 2)
	byRegion := map[string]int64{}
	for _, s := range p.Sample {
		region := ""
		for _, l := range s.Label {
			if p.StringTable[l.Key] == regionLabel {
				region = p.StringTable[l.Str]
			}
		}
		byRegion[region] = s.Value[0]
	}
	assert.Equal(t, map[string]int64{"eu": 30, "": 8}, byRegion)
}

// A "!=" matcher also matches "", so a non-matching sample must keep its label
// (its real value) rather than fold into the label-less total — otherwise the
// dropped label would read as "" downstream and wrongly satisfy the matcher.
func TestProfileStripper_Strip_TotalsNotEqualMatcherKeepsLabel(t *testing.T) {
	// String table: region=3, eu=4, us=5.
	p := &profilev1.Profile{
		SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 2}},
		StringTable: []string{"", "samples", "count", regionLabel, "eu", "us"},
		Sample: []*profilev1.Sample{
			{Value: []int64{10}, Label: []*profilev1.Label{{Key: 3, Str: 4}}}, // region=eu (excluded by !=eu)
			{Value: []int64{20}, Label: []*profilev1.Label{{Key: 3, Str: 5}}}, // region=us
			{Value: []int64{5}}, // no region
		},
	}
	ruler := fakeRuler{rules: map[string][]*phlaremodel.RecordingRule{
		"tenant-a": {{Matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, regionLabel, "eu")}}},
	}}

	NewProfileStripper(ruler).Strip("tenant-a", nil, p)

	// region=eu stays in its own total (region=eu) so the observer's region!=eu
	// excludes it; it must NOT fold into the label-less total with the 5.
	require.Len(t, p.Sample, 3)
	byRegion := map[string]int64{}
	for _, s := range p.Sample {
		region := ""
		for _, l := range s.Label {
			if p.StringTable[l.Key] == regionLabel {
				region = p.StringTable[l.Str]
			}
		}
		byRegion[region] = s.Value[0]
	}
	assert.Equal(t, map[string]int64{"eu": 10, "us": 20, "": 5}, byRegion)
}

// A sample kept for a function rule must also carry the labels of the total
// rules it matches, so it still counts toward their per-series totals.
func TestProfileStripper_Strip_KeptSampleCarriesTotalRuleLabels(t *testing.T) {
	// String table: func-a=3, func-b=4, region=5, eu=6.
	p := &profilev1.Profile{
		SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 2}},
		StringTable: []string{"", "samples", "count", "func-a", "func-b", regionLabel, "eu"},
		Function:    []*profilev1.Function{{Id: 1, Name: 3}, {Id: 2, Name: 4}},
		Location: []*profilev1.Location{
			{Id: 1, Line: []*profilev1.Line{{FunctionId: 1}}}, // func-a
			{Id: 2, Line: []*profilev1.Line{{FunctionId: 2}}}, // func-b
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{1}, Value: []int64{10}, Label: []*profilev1.Label{{Key: 5, Str: 6}}}, // func-a, region=eu
			{LocationId: []uint64{2}, Value: []int64{5}, Label: []*profilev1.Label{{Key: 5, Str: 6}}},  // func-b, region=eu
		},
	}
	ruler := fakeRuler{rules: map[string][]*phlaremodel.RecordingRule{
		"tenant-a": {
			{FunctionName: "func-a"},         // function rule
			{GroupBy: []string{regionLabel}}, // total rule
		},
	}}

	NewProfileStripper(ruler).Strip("tenant-a", nil, p)

	require.Len(t, p.Sample, 2)
	regionOf := func(s *profilev1.Sample) string {
		for _, l := range s.Label {
			if p.StringTable[l.Key] == regionLabel {
				return p.StringTable[l.Str]
			}
		}
		return ""
	}
	// Kept func-a sample keeps its stack AND region, so it joins the region total.
	kept, total := p.Sample[0], p.Sample[1]
	require.NotEmpty(t, kept.LocationId)
	assert.Equal(t, []int64{10}, kept.Value)
	assert.Equal(t, "eu", regionOf(kept))
	// The func-b sample folds into the region=eu total.
	assert.Empty(t, total.LocationId)
	assert.Equal(t, []int64{5}, total.Value)
	assert.Equal(t, "eu", regionOf(total))
}
