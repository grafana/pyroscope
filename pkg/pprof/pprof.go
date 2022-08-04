package pprof

import (
	"compress/gzip"
	"encoding/binary"
	"io/ioutil"
	"os"
	"sort"

	"github.com/cespare/xxhash/v2"
	"github.com/samber/lo"

	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	"github.com/grafana/fire/pkg/slices"
)

type Profile struct {
	*profilev1.Profile
	raw []byte

	hasher StacktracesHasher
}

func OpenFile(path string) (*Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	content, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return OpenRaw(content)
}

func OpenRaw(data []byte) (*Profile, error) {
	p := profilev1.ProfileFromVTPool()
	if err := p.UnmarshalVT(data); err != nil {
		return nil, err
	}

	return &Profile{Profile: p, raw: data}, nil
}

type sortedSample struct {
	samples []*profilev1.Sample
	hashes  []uint64
}

func (s *sortedSample) Len() int {
	return len(s.samples)
}

func (s *sortedSample) Less(i, j int) bool {
	return s.hashes[i] < s.hashes[j]
}

func (s *sortedSample) Swap(i, j int) {
	s.samples[i], s.samples[j] = s.samples[j], s.samples[i]
	s.hashes[i], s.hashes[j] = s.hashes[j], s.hashes[i]
}

// Normalize normalizes the profile by:
// - Removing all duplicate samples (summing their values).
// - Removing redundant profile labels (byte => unique of an allocation site)
//		todo: We should reassess if this was a good choice because by merging duplicate stacktrace samples
//            we cannot recompute the allocation per site ("bytes") profile label.
// - Removing empty samples.
// - Then remove unused references.
func (p *Profile) Normalize() {
	// first we sort the samples location ids.
	hashes := p.hasher.Hashes(p.Sample)

	ss := &sortedSample{samples: p.Sample, hashes: hashes}
	sort.Sort(ss)
	p.Sample = ss.samples
	hashes = ss.hashes

	// Remove samples.
	var removedSamples []*profilev1.Sample

	p.Sample = slices.RemoveInPlace(p.Sample, func(s *profilev1.Sample, i int) bool {
		// if the next sample has the same hashes, we can remove this sample but add the value to the next sample.
		if i < len(p.Sample)-1 && hashes[i] == hashes[i+1] {
			// todo handle hashes collisions
			for j := 0; j < len(s.Value); j++ {
				p.Sample[i+1].Value[j] += s.Value[j]
			}
			removedSamples = append(removedSamples, s)
			return true
		}
		for j := 0; j < len(s.Value); j++ {
			if s.Value[j] != 0 {
				// we found a non-zero value, so we can keep this sample, but remove redundant labels.
				s.Label = slices.RemoveInPlace(s.Label, func(l *profilev1.Label, _ int) bool {
					// remove labels block "bytes" as it's redundant.
					if l.Num != 0 && l.Key != 0 &&
						p.StringTable[l.Key] == "bytes" {
						return true
					}
					return false
				})
				return false
			}
		}
		// all values are 0, remove the sample.
		removedSamples = append(removedSamples, s)
		return true
	})

	// Remove references to removed samples.
	p.clearSampleReferences(removedSamples)
}

func (p *Profile) clearSampleReferences(samples []*profilev1.Sample) {
	if len(samples) == 0 {
		return
	}
	// remove all data not used anymore.
	var removedLocationTotal int
	for _, s := range samples {
		removedLocationTotal = len(s.LocationId)
	}
	removedLocationIds := make([]uint64, 0, removedLocationTotal)

	for _, s := range samples {
		removedLocationIds = append(removedLocationIds, s.LocationId...)
	}
	removedLocationIds = lo.Uniq(removedLocationIds)

	// figure which removed Locations IDs are not used.
	for _, s := range p.Sample {
		for _, l := range s.LocationId {
			removedLocationIds = slices.RemoveInPlace(removedLocationIds, func(locID uint64, _ int) bool {
				return l == locID
			})
		}
	}
	if len(removedLocationIds) == 0 {
		return
	}
	var removedFunctionIds []uint64
	// remove the locations that are not used anymore.
	p.Location = slices.RemoveInPlace(p.Location, func(loc *profilev1.Location, _ int) bool {
		if lo.Contains(removedLocationIds, loc.Id) {
			for _, l := range loc.Line {
				removedFunctionIds = append(removedFunctionIds, l.FunctionId)
			}
			return true
		}
		return false
	})

	if len(removedFunctionIds) == 0 {
		return
	}
	removedFunctionIds = lo.Uniq(removedFunctionIds)
	// figure which removed Function IDs are not used.
	for _, l := range p.Location {
		for _, f := range l.Line {
			removedFunctionIds = slices.RemoveInPlace(removedFunctionIds, func(fnID uint64, _ int) bool {
				// that ID is used in another location, remove it.
				return f.FunctionId == fnID
			})
		}
	}
	var removedNames []int64
	// remove the functions that are not used anymore.
	p.Function = slices.RemoveInPlace(p.Function, func(fn *profilev1.Function, _ int) bool {
		if lo.Contains(removedFunctionIds, fn.Id) {
			removedNames = append(removedNames, fn.Name, fn.SystemName, fn.Filename)
			return true
		}
		return false
	})

	if len(removedNames) == 0 {
		return
	}
	removedNames = lo.Uniq(removedNames)
	// remove names that are still used.
	p.visitAllNameReferences(func(idx *int64) {
		removedNames = slices.RemoveInPlace(removedNames, func(name int64, _ int) bool {
			return *idx == name
		})
	})
	if len(removedNames) == 0 {
		return
	}
	// Sort to remove in order.
	sort.Slice(removedNames, func(i, j int) bool { return removedNames[i] < removedNames[j] })
	// remove the names that are not used anymore.
	p.StringTable = lo.Reject(p.StringTable, func(_ string, i int) bool {
		return lo.Contains(removedNames, int64(i))
	})

	// Now shift all indices [0,1,2,3,4,5,6]
	// if we removed [1,2,5] then we need to shift [3,4] to [1,2] and [6] to [3]
	// Basically we need to shift all indices that are greater than the removed index by the amount of removed indices.
	p.visitAllNameReferences(func(idx *int64) {
		var shift int64
		for i := 0; i < len(removedNames); i++ {
			if *idx > removedNames[i] {
				shift++
				continue
			}
			break
		}
		*idx -= shift
	})
}

func (p *Profile) visitAllNameReferences(fn func(*int64)) {
	fn(&p.DropFrames)
	fn(&p.KeepFrames)
	fn(&p.PeriodType.Type)
	fn(&p.PeriodType.Unit)
	for _, st := range p.SampleType {
		fn(&st.Type)
		fn(&st.Unit)
	}
	for _, m := range p.Mapping {
		fn(&m.Filename)
		fn(&m.BuildId)
	}
	for _, s := range p.Sample {
		for _, l := range s.Label {
			fn(&l.Key)
			fn(&l.Num)
			fn(&l.NumUnit)
		}
	}
	for _, f := range p.Function {
		fn(&f.Name)
		fn(&f.SystemName)
		fn(&f.Filename)
	}
	for i := 0; i < len(p.Comment); i++ {
		fn(&p.Comment[i])
	}
}

type StacktracesHasher struct {
	hash *xxhash.Digest
	b    [8]byte
}

// todo we might want to reuse the results to avoid allocations
func (h StacktracesHasher) Hashes(samples []*profilev1.Sample) []uint64 {
	if h.hash == nil {
		h.hash = xxhash.New()
	} else {
		h.hash.Reset()
	}

	hashes := make([]uint64, len(samples))
	for i, sample := range samples {
		for _, locID := range sample.LocationId {
			binary.LittleEndian.PutUint64(h.b[:], locID)
			if _, err := h.hash.Write(h.b[:]); err != nil {
				panic("unable to write hash")
			}
		}
		hashes[i] = h.hash.Sum64()
		h.hash.Reset()
	}

	return hashes
}
