package segment

import (
	"time"
)

type Timeline struct {
	st                      time.Time
	et                      time.Time
	StartTime               int64    `json:"startTime"`
	Samples                 []uint64 `json:"samples"`
	durationDelta           time.Duration
	DurationDeltaNormalized int64 `json:"durationDelta"`
}

func GenerateTimeline(st, et time.Time) *Timeline {
	st, et = normalize(st, et)
	totalDuration := et.Sub(st)
	minDuration := totalDuration / time.Duration(1024)
	delta := durations[0]
	for _, d := range durations {
		if d < 0 {
			break
		}
		if d < minDuration {
			delta = d
		}
	}
	return &Timeline{
		st:                      st,
		et:                      et,
		StartTime:               st.Unix(),
		Samples:                 make([]uint64, totalDuration/delta),
		durationDelta:           delta,
		DurationDeltaNormalized: int64(delta / time.Second),
	}
}

func (tl *Timeline) PopulateTimeline(s *Segment, r *RetentionPolicy) {
	if s.root != nil {
		s.root.populateTimeline(tl, r)
	}
}

func (sn streeNode) populateTimeline(tl *Timeline, r *RetentionPolicy) {
	if sn.relationship(tl.st, tl.et) == outside {
		return
	}

	currentDuration := durations[sn.depth]
	// TODO(kolesnikovae): use "watermarks" instead of thresholds as those may change.
	var hasDataBefore bool
	var levelRetentionPeriod time.Time
	if sn.depth > 0 {
		levelRetentionPeriod = r.levelRetentionPeriod(sn.depth - 1)
	}

	if len(sn.children) > 0 && currentDuration >= tl.durationDelta {
		for i, v := range sn.children {
			if v != nil {
				v.populateTimeline(tl, r)
				hasDataBefore = true
				continue
			}
			if levelRetentionPeriod.IsZero() || sn.isBefore(r.absolutePeriod) || hasDataBefore {
				continue
			}
			if c := sn.createSampledChild(i); c.isBefore(levelRetentionPeriod) {
				c.populateTimeline(tl, r)
			}
		}
		return
	}

	nodeTime := sn.time
	if currentDuration < tl.durationDelta {
		currentDuration = tl.durationDelta
		nodeTime = nodeTime.Truncate(currentDuration)
	}

	i := int(nodeTime.Sub(tl.st) / tl.durationDelta)
	rightBoundary := i + int(currentDuration/tl.durationDelta)

	l := len(tl.Samples)
	for i < rightBoundary {
		if i >= 0 && i < l {
			if tl.Samples[i] == 0 {
				tl.Samples[i] = 1
			}
			tl.Samples[i] += sn.samples
		}
		i++
	}
}

func (sn *streeNode) createSampledChild(i int) *streeNode {
	s := &streeNode{
		depth:   sn.depth - 1,
		time:    sn.time.Add(time.Duration(i) * durations[sn.depth-1]),
		samples: sn.samples / multiplier,
		writes:  sn.samples / multiplier,
	}
	if s.depth > 0 {
		s.children = make([]*streeNode, multiplier)
		for j := range s.children {
			s.children[j] = s.createSampledChild(j)
		}
	}
	return s
}
