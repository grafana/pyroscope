package sampling

import (
	"github.com/prometheus/prometheus/model/labels"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
)

// ProfileStripper reduces profiles that were sampled out but should be kept
// in a minimal form.
type ProfileStripper struct {
	ruler Ruler
}

func NewProfileStripper(ruler Ruler) *ProfileStripper {
	return &ProfileStripper{ruler: ruler}
}

// Strip reduces the profile to a single sample holding the sum of all sample
// values: stacktraces, symbols, and sample labels are dropped, only the series
// totals are kept. Stacktraces targeted by the tenant's recording rules are the
// exception: they are kept, trimmed to the frames of the targeted functions,
// together with the labels those rules need to match and group.
func (s *ProfileStripper) Strip(tenantID string, seriesLabels phlaremodel.Labels, p *profilev1.Profile) {
	exceptions := s.computeExceptions(tenantID, seriesLabels, p)

	stripSamples(p, exceptions)

	pruneUnreferencedSymbols(p)

	compactStringTable(p)
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
// (trimmed to those locations and to the labels the matching rules need) plus a
// single sample holding the summed values of everything else.
func stripSamples(p *profilev1.Profile, exceptions map[uint64][]deferredRule) {
	var total *profilev1.Sample
	var keptSamples int
	for _, sample := range p.Sample {
		// Samples that Normalize would drop (value length mismatch,
		// negative values) must not contribute to the totals.
		if len(sample.Value) != len(p.SampleType) || hasNegativeValue(sample) {
			continue
		}
		var keptLocations int
		var keepLabels map[string]struct{}
		for _, l := range sample.LocationId {
			rules, ok := exceptions[l]
			if !ok {
				continue
			}
			var fulfilled bool
			for _, rule := range rules {
				if !sampleFulfillsMatchers(sample, p.StringTable, rule.deferredMatchers) {
					continue
				}
				fulfilled = true
				for _, name := range rule.keepLabels {
					if keepLabels == nil {
						keepLabels = make(map[string]struct{})
					}
					keepLabels[name] = struct{}{}
				}
			}
			if fulfilled {
				sample.LocationId[keptLocations] = l
				keptLocations++
			}
		}
		if keptLocations > 0 {
			sample.LocationId = sample.LocationId[:keptLocations]
			sample.Label = retainLabels(sample.Label, p.StringTable, keepLabels)
			p.Sample[keptSamples] = sample
			keptSamples++
			continue
		}
		if total == nil {
			total = &profilev1.Sample{Value: make([]int64, len(p.SampleType))}
		}
		for i, v := range sample.Value {
			total.Value[i] += v
		}
	}
	p.Sample = p.Sample[:keptSamples]
	if total != nil {
		p.Sample = append(p.Sample, total)
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

func retainLabels(sampleLabels []*profilev1.Label, stringTable []string, keep map[string]struct{}) []*profilev1.Label {
	retained := sampleLabels[:0]
	for _, l := range sampleLabels {
		if _, ok := keep[stringTable[l.Key]]; ok {
			retained = append(retained, l)
		}
	}
	return retained
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
