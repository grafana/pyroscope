package segment

import (
	"time"
)

type Timeline struct {
	StartTime     int64    `json:"startTime"`
	Samples       []uint64 `json:"samples"`
	durationDelta time.Duration
	DurationDelta int64 `json:"durationDelta"`
}

func GenerateTimeline(st, et time.Time) *Timeline {
	st, et = normalize(st, et)

	totalDuration := et.Sub(st)
	minDuration := totalDuration / time.Duration(2048/2)
	delta := durations[0]
	for _, d := range durations {
		if d < 0 {
			break
		}
		if d < minDuration {
			delta = d
		}
	}

	// TODO: need to figure out optimal size
	//   also move this outside of this method so that even if there's no segments it still works
	res := make([]uint64, totalDuration/delta)
	// TODO: can encode it more efficiently
	// for i := range res {
	// 	// uint64(rand.Intn(100))
	// 	res[i] = []uint64{uint64(currentTime.Unix() * 1000), 0}
	// 	currentTime = currentTime.Add(delta)
	// }
	return &Timeline{
		StartTime:     st.Unix(),
		Samples:       res,
		durationDelta: delta,
		DurationDelta: int64(delta / time.Second),
	}
}

func (tl *Timeline) PopulateTimeline(st, et time.Time, s *Segment) {
	st, et = normalize(st, et)

	if s.root == nil {
		return
	}

	s.root.populateTimeline(st, et, tl.durationDelta, tl.Samples)
}

func (sn *streeNode) populateTimeline(st, et time.Time, minDuration time.Duration, buf []uint64) {
	rel := sn.relationship(st, et)
	if rel != outside {
		currentDuration := durations[sn.depth]
		if len(sn.children) > 0 && currentDuration >= minDuration {
			for _, v := range sn.children {
				if v != nil {
					v.populateTimeline(st, et, minDuration, buf)
				}
			}
			return
		}

		nodeTime := sn.time
		if currentDuration < minDuration {
			currentDuration = minDuration
			nodeTime = nodeTime.Truncate(currentDuration)
		}

		i := int(nodeTime.Sub(st) / minDuration)
		rightBoundary := i + int(currentDuration/minDuration)

		l := len(buf)
		for i < rightBoundary {
			if i >= 0 && i < l {
				if buf[i] == 0 {
					buf[i] = 1
				}
				buf[i] += sn.samples
			}
			i++
		}
	}
}
