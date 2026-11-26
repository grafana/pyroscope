package sampling

import (
	"sort"

	"github.com/prometheus/prometheus/model/labels"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

// ProfileStripper reduces profiles that were sampled out but should be kept
// in a minimal form.
type ProfileStripper struct {
	ruler Ruler
}

type StripResult struct {
	RulesUnavailable bool
}

func NewProfileStripper(ruler Ruler) *ProfileStripper {
	return &ProfileStripper{ruler: ruler}
}

// Strip reduces a profile that was sampled out but should be kept. Two things
// are kept:
//
//   - Totals: the samples that no function rule targets are folded into
//     samples with no stacktrace, holding the sum of their values. A total rule
//     (a rule without a function_name) matches and groups by sample labels, so
//     those labels are kept and the totals are split per distinct value of
//     them, for example one total for region=eu and one for region=us. If no
//     total rule needs a sample label, it is dropped.
//   - Targeted stacktraces: for a rule with function_name F, each sample whose
//     stacktrace goes through F is kept, trimmed to the frames of F. It also
//     keeps the labels the rules match and group by, so the rules can still be
//     evaluated when the profile is compacted.
//
// Everything else, such as the rest of the stacktraces, the symbols they
// reference and the other sample labels, is removed.
//
// For two rules, one with function_name "db.query" and one total rule grouping
// by region:
//
//	before                              after
//
//	main
//	└─ handler                          db.query     10  region=eu
//	   ├─ db.query                      db.query      7  region=us
//	   │  ├─ driver.exec  10 region=eu
//	   │  └─ parse         7 region=us  (no stack)     5  region=eu   <- totals
//	   ├─ render           5 region=eu  (no stack)     3  region=us
//	   └─ cache.get        3 region=us
//
// The two samples that went through db.query keep only that frame, and keep
// region because the total rule groups by it. render and cache.get have no
// frame a function rule targets, so they fold into totals, one per region.
//
// RulesUnavailable in the result is set when the tenant's rules could not be
// loaded. The profile is then reduced to the totals only, so the stacktraces
// the rules need are lost.
func (s *ProfileStripper) Strip(tenantID string, seriesLabels phlaremodel.Labels, p *profilev1.Profile) StripResult {
	exc := s.computeExceptions(tenantID, seriesLabels, p)

	stripSamples(p, exc)

	pruneUnreferencedSymbols(p)

	compactStringTable(p)

	return StripResult{RulesUnavailable: exc.rulesUnavailable}
}

// compactStringTable rebuilds p.StringTable so it holds only the strings still
// referenced after stripping, remapping every reference to the new indices.
func compactStringTable(p *profilev1.Profile) {
	oldStrings := p.StringTable
	newStrings := []string{""}
	remap := map[int64]int64{0: 0}
	intern := func(old int64) int64 {
		if n, ok := remap[old]; ok {
			return n
		}
		n := int64(len(newStrings))
		newStrings = append(newStrings, oldStrings[old])
		remap[old] = n
		return n
	}
	p.DropFrames = intern(p.DropFrames)
	p.KeepFrames = intern(p.KeepFrames)
	p.DefaultSampleType = intern(p.DefaultSampleType)
	for _, vt := range p.SampleType {
		vt.Type = intern(vt.Type)
		vt.Unit = intern(vt.Unit)
	}
	if p.PeriodType != nil {
		p.PeriodType.Type = intern(p.PeriodType.Type)
		p.PeriodType.Unit = intern(p.PeriodType.Unit)
	}
	for i, c := range p.Comment {
		p.Comment[i] = intern(c)
	}
	for _, fn := range p.Function {
		fn.Name = intern(fn.Name)
		fn.SystemName = intern(fn.SystemName)
		fn.Filename = intern(fn.Filename)
	}
	for _, m := range p.Mapping {
		m.Filename = intern(m.Filename)
		m.BuildId = intern(m.BuildId)
	}
	for _, sample := range p.Sample {
		for _, l := range sample.Label {
			l.Key = intern(l.Key)
			l.Str = intern(l.Str)
			l.NumUnit = intern(l.NumUnit)
		}
	}
	p.StringTable = newStrings
}

// stripSamples reduces p.Sample to the samples that hit an exception location
// (trimmed to those locations and to the labels the matching rules need) plus
// one totals sample per distinct label set holding the summed values of
// everything else.
func stripSamples(p *profilev1.Profile, exc exceptions) {
	var kept, folded []*profilev1.Sample
	// Reused per sample to avoid allocating a keep-set for every sample.
	totalKeep := make(map[string]struct{})
	funcKeep := make(map[string]struct{})
	for _, sample := range p.Sample {
		// Samples that Normalize would drop (value length mismatch,
		// negative values) must not contribute to the totals.
		if len(sample.Value) != len(p.SampleType) || hasNegativeValue(sample) {
			continue
		}
		// Total rules count every sample, so each sample keeps the labels they
		// need, whether or not its stacktrace is kept.
		clear(totalKeep)
		for _, rule := range exc.totalRules {
			if sampleFulfillsMatchers(sample, p.StringTable, rule.deferredMatchers) {
				keepMatchedLabels(totalKeep, rule)
			} else {
				keepEmptyAcceptingLabels(totalKeep, rule)
			}
		}

		// Keep only the frames that a function rule targets and this sample
		// matches, along with the labels those rules need.
		clear(funcKeep)
		var keptLocations int
		for _, l := range sample.LocationId {
			rules, ok := exc.functionLocations[l]
			if !ok {
				continue
			}
			var fulfilled bool
			for _, rule := range rules {
				if sampleFulfillsMatchers(sample, p.StringTable, rule.deferredMatchers) {
					keepMatchedLabels(funcKeep, rule)
					fulfilled = true
				} else {
					keepEmptyAcceptingLabels(funcKeep, rule)
				}
			}
			if fulfilled {
				sample.LocationId[keptLocations] = l
				keptLocations++
			}
		}
		if keptLocations > 0 {
			sample.LocationId = sample.LocationId[:keptLocations]
			sample.Label = retainLabels(sample.Label, p.StringTable, funcKeep, totalKeep)
			kept = append(kept, sample)
			continue
		}
		// Folds into a total: drop the stacktrace, keep only the total labels.
		sample.LocationId = nil
		sample.Label = retainLabels(sample.Label, p.StringTable, totalKeep)
		folded = append(folded, sample)
	}
	p.Sample = append(kept, foldToTotals(folded, len(p.SampleType))...)
}

// foldToTotals sums the folded samples that have the same labels into one totals
// sample per label set.
func foldToTotals(folded []*profilev1.Sample, valueLen int) []*profilev1.Sample {
	if len(folded) == 0 {
		return nil
	}
	for _, s := range folded {
		sort.Sort(pprof.LabelsByKeyValue(s.Label))
	}
	sort.Sort(pprof.SamplesByLabels(folded))
	groups := pprof.GroupSamplesByLabels(&profilev1.Profile{Sample: folded})
	totals := make([]*profilev1.Sample, len(groups))
	for i, g := range groups {
		// Copy the labels so the totals sample has no capacity past len(). The
		// series split (pprofsplit.VisitSampleSeries) hides labels in that space
		// and restores them afterwards (restoreRemovedLabels), so a dropped label
		// left there would come back pointing at a string the compacted string
		// table no longer holds.
		labels := make([]*profilev1.Label, len(g.Labels))
		copy(labels, g.Labels)
		total := &profilev1.Sample{Value: make([]int64, valueLen), Label: labels}
		for _, s := range g.Samples {
			for j, v := range s.Value {
				total.Value[j] += v
			}
		}
		totals[i] = total
	}
	return totals
}

// keepMatchedLabels keeps the labels the rule matches and groups by.
func keepMatchedLabels(dst map[string]struct{}, rule deferredRule) {
	for _, name := range rule.keepLabels {
		dst[name] = struct{}{}
	}
}

// keepEmptyAcceptingLabels keeps the labels of the matchers that accept an empty
// value, such as region!="eu". A dropped label reads as empty, so dropping one
// of these would turn a non-match into a match.
func keepEmptyAcceptingLabels(dst map[string]struct{}, rule deferredRule) {
	for _, m := range rule.deferredMatchers {
		if m.Matches("") {
			dst[m.Name] = struct{}{}
		}
	}
}

func sampleFulfillsMatchers(s *profilev1.Sample, stringTable []string, matchers []*labels.Matcher) bool {
	for _, m := range matchers {
		if !m.Matches(sampleLabelValue(s, stringTable, m.Name)) {
			return false
		}
	}
	return true
}

func sampleLabelValue(s *profilev1.Sample, stringTable []string, name string) string {
	for _, l := range s.Label {
		if l.Str > 0 && stringTable[l.Key] == name {
			return stringTable[l.Str]
		}
	}
	return ""
}

func retainLabels(sampleLabels []*profilev1.Label, stringTable []string, keepSets ...map[string]struct{}) []*profilev1.Label {
	retained := sampleLabels[:0]
	for _, l := range sampleLabels {
		name := stringTable[l.Key]
		for _, keep := range keepSets {
			if _, ok := keep[name]; ok {
				retained = append(retained, l)
				break
			}
		}
	}
	return retained[:len(retained):len(retained)]
}

func pruneUnreferencedSymbols(p *profilev1.Profile) {
	keepLoc := make(map[uint64]struct{})
	for _, s := range p.Sample {
		for _, locID := range s.LocationId {
			keepLoc[locID] = struct{}{}
		}
	}
	if len(keepLoc) == 0 {
		p.Location = nil
		p.Function = nil
		p.Mapping = nil
		return
	}
	keepFunc := make(map[uint64]struct{})
	keepMapping := make(map[uint64]struct{})
	locations := p.Location[:0]
	for _, loc := range p.Location {
		if _, ok := keepLoc[loc.Id]; !ok {
			continue
		}
		locations = append(locations, loc)
		if loc.MappingId != 0 {
			keepMapping[loc.MappingId] = struct{}{}
		}
		for _, line := range loc.Line {
			keepFunc[line.FunctionId] = struct{}{}
		}
	}
	p.Location = locations
	functions := p.Function[:0]
	for _, fn := range p.Function {
		if _, ok := keepFunc[fn.Id]; ok {
			functions = append(functions, fn)
		}
	}
	p.Function = functions
	mappings := p.Mapping[:0]
	for _, m := range p.Mapping {
		if _, ok := keepMapping[m.Id]; ok {
			mappings = append(mappings, m)
		}
	}
	p.Mapping = mappings
}

func hasNegativeValue(s *profilev1.Sample) bool {
	for _, v := range s.Value {
		if v < 0 {
			return true
		}
	}
	return false
}
