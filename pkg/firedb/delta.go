package firedb

import (
	"sync"

	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	firemodel "github.com/grafana/fire/pkg/model"
)

const (
	memoryProfileName   = "memory"
	allocObjectTypeName = "alloc_objects"
	allocSpaceTypeName  = "alloc_space"
)

// deltaProfiles is a helper to compute delta of profiles.
type deltaProfiles struct {
	mtx sync.Mutex
	// todo cleanup sample profiles that are not used anymore using a cleanup goroutine.
	highestSamples map[model.Fingerprint][]*schemav1.Sample
}

func newDeltaProfiles() *deltaProfiles {
	return &deltaProfiles{
		highestSamples: make(map[model.Fingerprint][]*schemav1.Sample),
	}
}

func (d *deltaProfiles) computeDelta(ps *schemav1.Profile, lbss []firemodel.Labels) (*schemav1.Profile, []firemodel.Labels) {
	deltaIdx := lo.FilterMap(lbss, func(lbs firemodel.Labels, i int) (int, bool) {
		// right now we only support one delta profile on memory
		return i, isDelta(lbs)
	})
	// there's no delta to compute for those profiles.
	if len(deltaIdx) == 0 {
		return ps, lbss
	}
	d.mtx.Lock()
	defer d.mtx.Unlock()

	// we store all series ref so fetching with one work.
	lastSamples, ok := d.highestSamples[ps.SeriesRefs[deltaIdx[0]]]
	if !ok {
		// if we don't have the last profile, we can't compute the delta.
		// so we remove the delta from the list of labels and profiles.
		for _, i := range deltaIdx {
			d.highestSamples[ps.SeriesRefs[i]] = ps.Samples
		}
		keepIdx := lo.FilterMap(ps.SeriesRefs, func(_ model.Fingerprint, i int) (int, bool) {
			return i, !lo.Contains(deltaIdx, i)
		})
		newProfile := *ps
		newProfile.SeriesRefs = make([]model.Fingerprint, len(keepIdx))
		for i, j := range keepIdx {
			newProfile.SeriesRefs[i] = ps.SeriesRefs[j]
		}

		newProfile.Samples = make([]*schemav1.Sample, 0, len(ps.Samples))

		for _, s := range ps.Samples {
			newValues := make([]int64, len(keepIdx))
			newProfileLbs := make([]*profilev1.Label, len(keepIdx))
			allZero := true
			for i, j := range keepIdx {
				if s.Values[j] != 0 {
					allZero = false
				}
				newValues[i] = s.Values[j]
				newProfileLbs = copySlice(s.Labels)
			}
			if allZero {
				// if we end up with remaining values all to zero skip the sample.
				continue
			}
			newProfile.Samples = append(newProfile.Samples, &schemav1.Sample{
				StacktraceID: s.StacktraceID,
				Values:       newValues,
				Labels:       newProfileLbs,
			})
		}

		newLbss := make([]firemodel.Labels, len(keepIdx))
		for i, j := range keepIdx {
			newLbss[i] = lbss[j]
		}
		return &newProfile, newLbss
	}

	// we have the last profile, we can compute the delta.
	// samples are sorted by stacktrace id.
	// we need to compute the delta for each stacktrace.
	if len(lastSamples) == 0 {
		return ps, lbss
	}

	highestSamples := deltaSamples(lastSamples, ps.Samples, deltaIdx)

	// remove samples that are all zero
	i := 0
	for _, x := range ps.Samples {
		for _, v := range x.Values {
			if v != 0 {
				ps.Samples[i] = x
				i++
				break
			}
		}
	}
	ps.Samples = ps.Samples[:i]
	for _, i := range deltaIdx {
		d.highestSamples[ps.SeriesRefs[i]] = highestSamples
	}
	return ps, lbss
}

func isDelta(lbs firemodel.Labels) bool {
	if lbs.Get(model.MetricNameLabel) == memoryProfileName {
		ty := lbs.Get(firemodel.LabelNameType)
		if ty == allocObjectTypeName || ty == allocSpaceTypeName {
			return true
		}
	}
	return false
}

func deltaSamples(highest, new []*schemav1.Sample, idx []int) []*schemav1.Sample {
	stacktraces := make(map[uint64]*schemav1.Sample)
	for _, h := range highest {
		stacktraces[h.StacktraceID] = h
	}
	for _, n := range new {
		if s, ok := stacktraces[n.StacktraceID]; ok {
			for _, k := range idx {
				if s.Values[k] <= n.Values[k] {
					newMax := n.Values[k]
					n.Values[k] -= s.Values[k]
					s.Values[k] = newMax
				} else {
					s.Values[k] = n.Values[k]
				}
			}
			continue
		}
		highest = append(highest, n)
	}
	return highest
}
