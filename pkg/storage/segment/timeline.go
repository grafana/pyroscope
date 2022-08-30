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

	// Watermarks map contains down-sampling watermarks (Unix timestamps)
	// describing resolution levels of the timeline.
	//
	// Resolution in seconds is calculated as 10^k, where k is the map key.
	// Meaning that any range within these 10^k seconds contains not more
	// than one sample. Any sub-range less than 10^k shows down-sampled data.
	//
	// Given the map:
	//  1: 1635508310
	//  2: 1635507500
	//  3: 1635506200
	//
	// This should be read as follows:
	//  1. Data after 1635508310 is as precise as possible (10s resolution),
	//     down-sampling was not applied.
	//  2. Data before 1635508310 has resolution 100s
	//  3. Data before 1635507500 has resolution 1000s
	//  4. Data before 1635506200 has resolution 10000s
	Watermarks map[int]int64 `json:"watermarks"`
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
		Watermarks:              make(map[int]int64),
	}
}

func (tl *Timeline) PopulateTimeline(s *Segment) {
	s.m.Lock()
	if s.root != nil {
		s.root.populateTimeline(tl, s)
	}
	s.m.Unlock()
}

func (sn streeNode) populateTimeline(tl *Timeline, s *Segment) {
	if sn.relationship(tl.st, tl.et) == outside {
		return
	}

	var (
		currentDuration = durations[sn.depth]
		levelWatermark  time.Time
		hasDataBefore   bool
	)

	if sn.depth > 0 {
		levelWatermark = s.watermarks.levels[sn.depth-1]
	}

	if len(sn.children) > 0 && currentDuration >= tl.durationDelta {
		for i, v := range sn.children {
			if v != nil {
				v.populateTimeline(tl, s)
				hasDataBefore = true
				continue
			}
			if hasDataBefore || levelWatermark.IsZero() || sn.isBefore(s.watermarks.absoluteTime) {
				continue
			}
			if c := sn.createSampledChild(i); c.isBefore(levelWatermark) && c.isAfter(s.watermarks.absoluteTime) {
				c.populateTimeline(tl, s)
				if m := c.time.Add(durations[c.depth]); m.After(tl.st) {
					tl.Watermarks[c.depth+1] = c.time.Add(durations[c.depth]).Unix()
				}
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
