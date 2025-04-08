package novelty

import (
	"sort"
)

type Sample struct {
	stackResolved int
	sample        int64
}

func abs(a, b int64) (match, miss int64) {
	if a > b {
		return b, a - b
	}
	return a, b - a
}

func getNovelty(profile1 profile, total1 int64, profile2 profile, total2 int64) float64 {
	factor := float64(float64(total2) / float64(total1))
	scale := func(sample int64) int64 {
		return int64(float64(sample) * factor)
	}

	idx1 := 0
	idx2 := 0

	var matches, misses int64
	for {
		// if we have reached the end of both profiles, we are done
		if idx1 >= len(profile1) && idx2 >= len(profile2) {
			break
		}

		// end of one of the profiles, we can break after adding the remaining samples
		if idx1 >= len(profile1) {
			misses += profile2[idx2].sample
			idx2++
			continue
		}
		if idx2 >= len(profile2) {
			misses += scale(profile1[idx1].sample)
			idx1++
			continue
		}

		// check for match
		if profile1[idx1].stackResolved == profile2[idx2].stackResolved {
			match, miss := abs(scale(profile1[idx1].sample), profile2[idx2].sample)
			matches += match
			misses += miss
			idx1++
			idx2++
			continue
		}

		// if profile1 is less than profile2, we need to move to the next profile1
		if profile1[idx1].stackResolved < profile2[idx2].stackResolved {
			misses += scale(profile1[idx1].sample)
			idx1++
			continue
		}

		// if profile2 is less than profile1, we need to move to the next profile2
		if profile1[idx1].stackResolved > profile2[idx2].stackResolved {
			misses += profile2[idx2].sample
			idx2++
			continue
		}
	}

	return float64(matches) / float64(matches+misses)

}

func mergeProfiles(profile1, profile2 profile) (merged profile, total int64) {
	merged = make(profile, 0, len(profile1))
	total = 0

	idx1 := 0
	idx2 := 0

	for {
		// if we have reached the end of both profiles, we are done
		if idx1 >= len(profile1) && idx2 >= len(profile2) {
			break
		}

		if idx1 >= len(profile1) {
			left := profile2[idx2:]
			merged = append(merged, left...)
			for _, sample := range left {
				total += sample.sample
			}
			break
		}
		if idx2 >= len(profile2) {
			left := profile1[idx1:]
			merged = append(merged, left...)
			for _, sample := range left {
				total += sample.sample
			}
			break
		}

		// check for match
		if profile1[idx1].stackResolved == profile2[idx2].stackResolved {
			value := profile1[idx1].sample + profile2[idx2].sample
			merged = append(merged, Sample{
				stackResolved: profile1[idx1].stackResolved,
				sample:        value,
			})
			total += value
			idx1++
			idx2++
			continue
		}

		// if profile1 is less than profile2, we attach that one
		if profile1[idx1].stackResolved < profile2[idx2].stackResolved {
			merged = append(merged, profile1[idx1])
			total += profile1[idx1].sample
			idx1++
			continue
		}

		// if profile2 is less than profile1, we attach that one
		if profile1[idx1].stackResolved > profile2[idx2].stackResolved {
			merged = append(merged, profile2[idx2])
			total += profile2[idx2].sample
			idx2++
			continue
		}
	}

	return merged, total
}

type profile []Sample

type Samples struct {
	threshold float64 // when is something considered a match

	stackMap   map[string]int
	stackTable []string

	profiles []profile
	totals   []int64
}

func NewSamples(size int, threshold float64) *Samples {
	return &Samples{
		stackMap:   make(map[string]int, size),
		stackTable: make([]string, 0, size),
		threshold:  threshold,
	}
}

func (n *Samples) Add(stack []string, value []int64) float64 {
	profile := make(profile, len(stack))
	total := int64(0)
	for idx := range stack {
		pos, ok := n.stackMap[stack[idx]]
		if !ok {
			pos = len(n.stackTable)
			n.stackMap[stack[idx]] = pos
			n.stackTable = append(n.stackTable, stack[idx])
		}
		profile[idx].stackResolved = pos
		profile[idx].sample = value[idx]
		total += value[idx]
	}

	// sort the profile by stackResolved
	sort.Slice(profile, func(i, j int) bool {
		return profile[i].stackResolved < profile[j].stackResolved
	})

	maxNovelty := 0.0
	maxNoveltyIdx := -1
	for idx, p := range n.profiles {
		novelty := getNovelty(p, n.totals[idx], profile, total)
		if novelty > maxNovelty {
			maxNovelty = novelty
			maxNoveltyIdx = idx
		}
	}

	if maxNovelty >= 0 && maxNovelty > n.threshold {
		n.profiles[maxNoveltyIdx], n.totals[maxNoveltyIdx] = mergeProfiles(n.profiles[maxNoveltyIdx], profile)
		return maxNovelty
	}

	// add a new sub profile
	n.profiles = append(n.profiles, profile)
	n.totals = append(n.totals, total)
	return maxNovelty
}
