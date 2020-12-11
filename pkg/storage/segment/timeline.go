package segment

import (
	"time"

	"github.com/sirupsen/logrus"
)

type Timeline struct {
	data         [][]uint64
	durThreshold time.Duration
}

func GenerateTimeline(st, et time.Time) *Timeline {
	st, et = normalize(st, et)

	totalDuration := et.Sub(st)
	minDuration := totalDuration / time.Duration(2048/2)
	durThreshold := durations[0]
	for _, d := range durations {
		if d < 0 {
			break
		}
		if d < minDuration {
			durThreshold = d
		}
	}

	// TODO: need to figure out optimal size
	//   also move this outside of this method so that even if there's no segments it still works
	res := make([][]uint64, totalDuration/durThreshold)
	currentTime := st
	for i := range res {
		// uint64(rand.Intn(100))
		res[i] = []uint64{uint64(currentTime.Unix() * 1000), 0}
		currentTime = currentTime.Add(durThreshold)
	}
	return &Timeline{
		data:         res,
		durThreshold: durThreshold,
	}
}

func (tl *Timeline) Data() [][]uint64 {
	return tl.data
}

func (tl *Timeline) PopulateTimeline(st, et time.Time, s *Segment) {
	st, et = normalize(st, et)

	if s.root == nil {
		return
	}

	s.root.populateTimeline(st, et, tl.durThreshold, tl.data)
}

func (sn *streeNode) populateTimeline(st, et time.Time, minDuration time.Duration, buf [][]uint64) {
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
				logrus.WithFields(logrus.Fields{
					"sn.samples": sn.samples,
				}).Info("sn.samples")
				if buf[i][1] == 0 {
					buf[i][1] = 1
				}
				buf[i][1] += sn.samples

				// if buf[i][1] == 0 {
				// 	buf[i][1] = 1
				// }
			}
			i++
		}
	}
}
