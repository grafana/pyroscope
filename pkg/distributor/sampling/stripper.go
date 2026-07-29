package sampling

import (
	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

// Stripper reduces profiles that were sampled out but should be kept
// in a minimal form.
type Stripper struct{}

func NewStripper() *Stripper {
	return &Stripper{}
}

// StripToTotals reduces the profile to a single sample holding the
// sum of all sample values: stacktraces, symbols, and sample labels are
// dropped, only the series totals are kept.
func (*Stripper) StripToTotals(p *profilev1.Profile) {
	var total *profilev1.Sample
	for _, s := range p.Sample {
		// Samples that Normalize would drop (value length mismatch,
		// negative values) must not contribute to the totals.
		if len(s.Value) != len(p.SampleType) || hasNegativeValue(s) {
			continue
		}
		if total == nil {
			total = &profilev1.Sample{Value: make([]int64, len(p.SampleType))}
		}
		for i, v := range s.Value {
			total.Value[i] += v
		}
	}
	p.Sample = nil
	if total != nil {
		p.Sample = []*profilev1.Sample{total}
	}
	p.Location = nil
	p.Function = nil
	p.Mapping = nil

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
	p.StringTable = newStrings
}

func hasNegativeValue(s *profilev1.Sample) bool {
	for _, v := range s.Value {
		if v < 0 {
			return true
		}
	}
	return false
}
