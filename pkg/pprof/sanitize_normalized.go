package pprof

import profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"

// A profile whose IDs and references already satisfy the sanitizer's output
// invariants does not need ID maps or reference rewriting. All other profiles
// retain the original sanitizer's repair behavior.
func sanitizeProfile(p *profilev1.Profile, stats *sanitizeStats) {
	if profileAlreadyNormalized(p) {
		if stats.samplesTotal == 0 {
			stats.samplesTotal = len(p.Sample)
		}
		return
	}
	sanitizeProfileSlow(p, stats)
}

func profileAlreadyNormalized(p *profilev1.Profile) bool {
	if p == nil || len(p.StringTable) == 0 || p.StringTable[0] != "" {
		return false
	}
	str := func(i int64) bool { return i >= 0 && i < int64(len(p.StringTable)) }
	for _, s := range p.SampleType {
		if s == nil || !str(s.Type) || !str(s.Unit) {
			return false
		}
	}
	if p.PeriodType != nil && (!str(p.PeriodType.Type) || !str(p.PeriodType.Unit)) {
		return false
	}
	if !str(p.DefaultSampleType) || !str(p.DropFrames) || !str(p.KeepFrames) {
		return false
	}
	for _, c := range p.Comment {
		if !str(c) {
			return false
		}
	}
	for i, m := range p.Mapping {
		if m == nil || m.Id != uint64(i+1) || !str(m.Filename) || !str(m.BuildId) {
			return false
		}
	}
	for i, f := range p.Function {
		if f == nil || f.Id != uint64(i+1) || !str(f.Name) || !str(f.Filename) || !str(f.SystemName) {
			return false
		}
	}
	for i, l := range p.Location {
		if l == nil || l.Id != uint64(i+1) || (len(l.Line) == 0 && l.Address == 0) || l.MappingId == 0 || l.MappingId > uint64(len(p.Mapping)) {
			return false
		}
		for _, line := range l.Line {
			if line == nil || line.FunctionId == 0 || line.FunctionId > uint64(len(p.Function)) {
				return false
			}
		}
	}
	for _, s := range p.Sample {
		if s == nil || len(s.Value) != len(p.SampleType) {
			return false
		}
		for _, id := range s.LocationId {
			if id == 0 || id > uint64(len(p.Location)) {
				return false
			}
		}
		for _, l := range s.Label {
			if l == nil || !str(l.Key) || !str(l.Str) || !str(l.NumUnit) {
				return false
			}
		}
	}
	return true
}
