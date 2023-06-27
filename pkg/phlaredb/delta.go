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
	highestSamples map[model.Fingerprint]map[uint32]uint64
}

func newDeltaProfiles() *deltaProfiles {
	return &deltaProfiles{
		highestSamples: make(map[model.Fingerprint]map[uint32]uint64),
	}
}

func newSampleDict(samples schemav1.Samples) map[uint32]uint64 {
	dict := make(map[uint32]uint64)
	for i, s := range samples.StacktraceIDs {
		dict[s] += samples.Values[i]
	}
	return dict
}

func (d *deltaProfiles) computeDelta(ps schemav1.InMemoryProfile, lbs phlaremodel.Labels) schemav1.Samples {
	// there's no delta to compute for those profile.
	if !isDelta(lbs) {
		return ps.Samples
	}

	d.mtx.Lock()
	defer d.mtx.Unlock()

	// we store all series ref so fetching with one work.
	lastSamples, ok := d.highestSamples[ps.SeriesFingerprint]
	if !ok {
		// if we don't have the last profile, we can't compute the delta.
		// so we remove the delta from the list of labels and profiles.
		d.highestSamples[ps.SeriesFingerprint] = newSampleDict(ps.Samples)

		return schemav1.Samples{}
	}

	// we have the last profile, we can compute the delta.
	// samples are sorted by stacktrace id.
	// we need to compute the delta for each stacktrace.
	if len(lastSamples) == 0 {
		return ps.Samples
	}

	reset := deltaSamples(lastSamples, ps.Samples)
	if reset {
		// if we reset the delta, we can't compute the delta anymore.
		// so we remove the delta from the list of labels and profiles.
		d.highestSamples[ps.SeriesFingerprint] = newSampleDict(ps.Samples)
		return schemav1.Samples{}
	}

	return ps.Samples.Compact(false).Clone()
}

func isDelta(lbs phlaremodel.Labels) bool {
	switch lbs.Get(phlaremodel.LabelNameDelta) {
	case "false":
		return false
	case "true":
		return true
	}
	if lbs.Get(model.MetricNameLabel) == memoryProfileName {
		ty := lbs.Get(phlaremodel.LabelNameType)
		if ty == allocObjectTypeName || ty == allocSpaceTypeName {
			return true
		}
	}
	return false
}

func deltaSamples(highest map[uint32]uint64, new schemav1.Samples) bool {
	for i, id := range new.StacktraceIDs {
		newValue := new.Values[i]
		if s, ok := highest[id]; ok {
			if s <= newValue {
				new.Values[i] -= s
				highest[id] = newValue
			} else {
				// this is a reset, we can't compute the delta anymore.
				return true
			}
			continue
		}
		highest[id] = newValue
	}
	return false
}
