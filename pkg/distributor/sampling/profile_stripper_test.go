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

// sampleView is a human-readable view of a sample: its stacktrace as function
// names (leaf -> root) and its sample labels.
type sampleView struct {
	stack  []string
	labels map[string]string
	value  int64
}

// buildProfile assembles a profile from sampleViews, interning function names
// (one location/function per name) and label strings.
func buildProfile(views []sampleView) *profilev1.Profile {
	stringTable := []string{"", "samples", "count"}
	intern := func(s string) int64 {
		for i, existing := range stringTable {
			if existing == s {
				return int64(i)
			}
		}
		stringTable = append(stringTable, s)
		return int64(len(stringTable) - 1)
	}
	locByFunc := map[string]uint64{}
	var functions []*profilev1.Function
	var locations []*profilev1.Location
	location := func(fn string) uint64 {
		if id, ok := locByFunc[fn]; ok {
			return id
		}
		id := uint64(len(locations) + 1)
		functions = append(functions, &profilev1.Function{Id: id, Name: intern(fn)})
		locations = append(locations, &profilev1.Location{Id: id, Line: []*profilev1.Line{{FunctionId: id}}})
		locByFunc[fn] = id
		return id
	}
	samples := make([]*profilev1.Sample, 0, len(views))
	for _, v := range views {
		var locs []uint64
		for _, fn := range v.stack {
			locs = append(locs, location(fn))
		}
		var lbls []*profilev1.Label
		for k, val := range v.labels {
			lbls = append(lbls, &profilev1.Label{Key: intern(k), Str: intern(val)})
		}
		samples = append(samples, &profilev1.Sample{LocationId: locs, Value: []int64{v.value}, Label: lbls})
	}
	return &profilev1.Profile{
		SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 2}},
		StringTable: stringTable,
		Function:    functions,
		Location:    locations,
		Sample:      samples,
	}
}

// readProfile is the inverse of buildProfile: it resolves each sample back to
// function names and label values (nil stack for a stackless total).
func readProfile(p *profilev1.Profile) []sampleView {
	funcName := map[uint64]string{}
	for _, f := range p.Function {
		funcName[f.Id] = p.StringTable[f.Name]
	}
	locFunc := map[uint64]string{}
	for _, l := range p.Location {
		locFunc[l.Id] = funcName[l.Line[0].FunctionId]
	}
	views := make([]sampleView, 0, len(p.Sample))
	for _, s := range p.Sample {
		var stack []string
		for _, id := range s.LocationId {
			stack = append(stack, locFunc[id])
		}
		labels := map[string]string{}
		for _, l := range s.Label {
			labels[p.StringTable[l.Key]] = p.StringTable[l.Str]
		}
		views = append(views, sampleView{stack: stack, labels: labels, value: s.Value[0]})
	}
	return views
}

// TestProfileStripper_Strip_Overview is a readable, end-to-end reference for Strip. It
// runs a whole profile of the series service_name="checkout" against a mix of
// function and total rules that together exercise every knob of the feature.
//
// Recording rules for the tenant (a rule is a "function rule" when it sets
// function_name, otherwise a "total rule"):
//
//	FA function_name="db.query"  matchers=[service_name="checkout"]  group_by=[route]
//	     base matcher matches this series -> active. Keeps db.query stacks; a kept
//	     sample carries its route (the group_by label).
//	FB function_name="encode"    matchers=[format="proto"]
//	     format is a sample label, not a series label -> the matcher is DEFERRED to
//	     each sample. Keeps encode stacks only for samples with format="proto",
//	     which then carry format.
//	FC function_name="db.query"  matchers=[service_name="billing"]
//	     base matcher does NOT match this series -> the rule is dropped entirely and
//	     never affects the output.
//	TA (total)                   group_by=[tenant]
//	     every surviving sample carries tenant; the leftovers split into per-tenant
//	     totals.
//	TB (total)                   matchers=[region!="eu"]
//	     region is a sample label -> DEFERRED. Because != also matches a missing
//	     label (""), region is kept on every sample that has it (even region="eu"),
//	     so the totals split by region too.
//
// A kept sample keeps only the frames of the functions it hits, plus the labels
// the matching rules need. A folded sample drops its stack and keeps only the
// total-rule labels. Samples Normalize would reject (negative values) are dropped.
//
//	INPUT
//	  #    stack (leaf -> root)                     labels                                      value
//	  I1   db.query -> handler                      tenant=a route=/x region=us                   10
//	  I2   db.query -> auth -> handler              tenant=b route=/y region=eu                   20
//	  I3   encode -> handler                        tenant=a format=proto                          5
//	  I4   encode -> handler                        tenant=a format=json                           8
//	  I5   db.query -> encode -> handler            tenant=b route=/z format=proto region=us      12
//	  I6   db.query -> cache -> handler             tenant=a route=/x region=us                    7
//	  I7   db.query -> parse -> db.query -> handler tenant=b route=/w region=eu                    9
//	  I8   gzip -> handler                          tenant=a region=us                             4
//	  I9   gzip -> handler                          tenant=a region=us                             6
//	  I10  read -> handler                          tenant=b region=eu                             7
//	  I11  read -> handler                          tenant=a                                       3
//	  I12  read -> handler                          tenant=a region=us                            -1
//
//	OUTPUT. Strip's output is not minimal: it does not merge identical kept
//	samples (I1 and I6 below) nor collapse repeated recursion frames (I7).
//	Downstream Normalize does both.
//	  from      stack               labels                                      value
//	  I1     -> [db.query]          tenant=a route=/x region=us                   10  <-+ same stack AND labels;
//	  I6     -> [db.query]          tenant=a route=/x region=us                    7  <-+ Normalize merges to 17
//	  I2     -> [db.query]          tenant=b route=/y region=eu                   20   trimmed to db.query
//	  I3     -> [encode]            tenant=a format=proto                          5   kept: format=proto matches FB
//	  I5     -> [db.query,encode]   tenant=b route=/z format=proto region=us      12   hits both FA and FB
//	  I7     -> [db.query,db.query] tenant=b route=/w region=eu                    9   recursion: both frames kept
//	  I8+I9  -> (total)             tenant=a region=us                            10   folded and summed
//	  I10    -> (total)             tenant=b region=eu                             7   folded
//	  I4+I11 -> (total)             tenant=a                                      11   I4 folds: format=json fails FB
//	  I12    -> dropped                                                                negative value
func TestProfileStripper_Strip_Overview(t *testing.T) {
	in := []sampleView{
		{stack: []string{"db.query", "handler"}, labels: map[string]string{"tenant": "a", "route": "/x", "region": "us"}, value: 10},
		{stack: []string{"db.query", "auth", "handler"}, labels: map[string]string{"tenant": "b", "route": "/y", "region": "eu"}, value: 20},
		{stack: []string{"encode", "handler"}, labels: map[string]string{"tenant": "a", "format": "proto"}, value: 5},
		{stack: []string{"encode", "handler"}, labels: map[string]string{"tenant": "a", "format": "json"}, value: 8},
		{stack: []string{"db.query", "encode", "handler"}, labels: map[string]string{"tenant": "b", "route": "/z", "format": "proto", "region": "us"}, value: 12},
		{stack: []string{"db.query", "cache", "handler"}, labels: map[string]string{"tenant": "a", "route": "/x", "region": "us"}, value: 7},
		{stack: []string{"db.query", "parse", "db.query", "handler"}, labels: map[string]string{"tenant": "b", "route": "/w", "region": "eu"}, value: 9},
		{stack: []string{"gzip", "handler"}, labels: map[string]string{"tenant": "a", "region": "us"}, value: 4},
		{stack: []string{"gzip", "handler"}, labels: map[string]string{"tenant": "a", "region": "us"}, value: 6},
		{stack: []string{"read", "handler"}, labels: map[string]string{"tenant": "b", "region": "eu"}, value: 7},
		{stack: []string{"read", "handler"}, labels: map[string]string{"tenant": "a"}, value: 3},
		{stack: []string{"read", "handler"}, labels: map[string]string{"tenant": "a", "region": "us"}, value: -1},
	}
	ruler := fakeRuler{rules: map[string][]*phlaremodel.RecordingRule{
		"tenant-a": {
			{FunctionName: "db.query", Matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, phlaremodel.LabelNameServiceName, "checkout")}, GroupBy: []string{"route"}},
			{FunctionName: "encode", Matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "format", "proto")}},
			{FunctionName: "db.query", Matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, phlaremodel.LabelNameServiceName, "billing")}},
			{GroupBy: []string{"tenant"}},
			{Matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, regionLabel, "eu")}},
		},
	}}

	p := buildProfile(in)
	NewProfileStripper(ruler).Strip("tenant-a", phlaremodel.LabelsFromStrings(phlaremodel.LabelNameServiceName, "checkout"), p)

	want := []sampleView{
		{stack: []string{"db.query"}, labels: map[string]string{"tenant": "a", "route": "/x", "region": "us"}, value: 10},
		{stack: []string{"db.query"}, labels: map[string]string{"tenant": "a", "route": "/x", "region": "us"}, value: 7},
		{stack: []string{"db.query"}, labels: map[string]string{"tenant": "b", "route": "/y", "region": "eu"}, value: 20},
		{stack: []string{"encode"}, labels: map[string]string{"tenant": "a", "format": "proto"}, value: 5},
		{stack: []string{"db.query", "encode"}, labels: map[string]string{"tenant": "b", "route": "/z", "format": "proto", "region": "us"}, value: 12},
		{stack: []string{"db.query", "db.query"}, labels: map[string]string{"tenant": "b", "route": "/w", "region": "eu"}, value: 9},
		{labels: map[string]string{"tenant": "a", "region": "us"}, value: 10},
		{labels: map[string]string{"tenant": "b", "region": "eu"}, value: 7},
		{labels: map[string]string{"tenant": "a"}, value: 11},
	}
	assert.ElementsMatch(t, want, readProfile(p))
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

// Two rules target the same function; one has a "!=" matcher on a sample label.
// A sample kept for the first rule must retain that label (its real value) so
// the "!=" rule still excludes it downstream instead of matching a dropped "".
func TestProfileStripper_Strip_FunctionRuleNotEqualKeepsLabel(t *testing.T) {
	// String table: func-a=3, region=4, us=5.
	p := &profilev1.Profile{
		SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 2}},
		StringTable: []string{"", "samples", "count", "func-a", regionLabel, "us"},
		Function:    []*profilev1.Function{{Id: 1, Name: 3}},
		Location:    []*profilev1.Location{{Id: 1, Line: []*profilev1.Line{{FunctionId: 1}}}},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{1}, Value: []int64{10}, Label: []*profilev1.Label{{Key: 4, Str: 5}}}, // region=us
		},
	}
	ruler := fakeRuler{rules: map[string][]*phlaremodel.RecordingRule{
		"tenant-a": {
			{FunctionName: "func-a"}, // keeps the stacktrace
			{FunctionName: "func-a", Matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, regionLabel, "us")}},
		},
	}}

	NewProfileStripper(ruler).Strip("tenant-a", nil, p)

	require.Len(t, p.Sample, 1)
	kept := p.Sample[0]
	require.NotEmpty(t, kept.LocationId)
	region := ""
	for _, l := range kept.Label {
		if p.StringTable[l.Key] == regionLabel {
			region = p.StringTable[l.Str]
		}
	}
	assert.Equal(t, "us", region)
}
