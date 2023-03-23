package phlaredb

import (
	"sync"

	"github.com/prometheus/common/model"

	phlaremodel "github.com/grafana/phlare/pkg/model"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
)

const (
	memoryProfileName   = "memory"
	allocObjectTypeName = "alloc_objects"
	allocSpaceTypeName  = "alloc_space"
	blockProfileName    = "block"
	contentionsTypeName = "contentions"
	delayTypeName       = "delay"
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

func (d *deltaProfiles) computeDelta(ps *schemav1.Profile, lbs phlaremodel.Labels) *schemav1.Profile {
	// there's no delta to compute for those profile.
	if !isDelta(lbs) {
		return ps
	}

	d.mtx.Lock()
	defer d.mtx.Unlock()

	// we store all series ref so fetching with one work.
	lastSamples, ok := d.highestSamples[ps.SeriesFingerprint]
	if !ok {
		// if we don't have the last profile, we can't compute the delta.
		// so we remove the delta from the list of labels and profiles.
		d.highestSamples[ps.SeriesFingerprint] = ps.Samples

		return nil
	}

	// we have the last profile, we can compute the delta.
	// samples are sorted by stacktrace id.
	// we need to compute the delta for each stacktrace.
	if len(lastSamples) == 0 {
		return ps
	}

	highestSamples := deltaSamples(lastSamples, ps.Samples)

	// remove samples that are all zero
	i := 0
	for _, x := range ps.Samples {
		if x.Value != 0 {
			ps.Samples[i] = x
			i++
		}
	}
	ps.Samples = ps.Samples[:i]
	d.highestSamples[ps.SeriesFingerprint] = highestSamples
	return ps
}

func isDelta(lbs phlaremodel.Labels) bool {
	if lbs.Get(phlaremodel.LabelNameDelta) == "false" {
		return false
	}
	if lbs.Get(model.MetricNameLabel) == memoryProfileName {
		ty := lbs.Get(phlaremodel.LabelNameType)
		if ty == allocObjectTypeName || ty == allocSpaceTypeName {
			return true
		}
	}
	//if lbs.Get(model.MetricNameLabel) == blockProfileName {
	//	ty := lbs.Get(phlaremodel.LabelNameType)
	//	if ty == contentionsTypeName || ty == delayTypeName {
	//		return true
	//	}
	//}
	return false
}

func deltaSamples(highest, new []*schemav1.Sample) []*schemav1.Sample {
	stacktraces := make(map[uint64]*schemav1.Sample)
	for _, h := range highest {
		stacktraces[h.StacktraceID] = h
	}
	for _, n := range new {
		if s, ok := stacktraces[n.StacktraceID]; ok {
			if s.Value <= n.Value {
				newMax := n.Value
				n.Value -= s.Value
				s.Value = newMax
			} else {
				s.Value = n.Value
			}
			continue
		}
		highest = append(highest, n)
	}
	return highest
}
