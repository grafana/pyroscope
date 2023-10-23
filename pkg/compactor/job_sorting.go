// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/job_sorting.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"sort"
)

const (
	CompactionOrderOldestFirst = "smallest-range-oldest-blocks-first"
	CompactionOrderNewestFirst = "newest-blocks-first"
)

var CompactionOrders = []string{CompactionOrderOldestFirst, CompactionOrderNewestFirst}

type JobsOrderFunc func(jobs []*Job) []*Job

// GetJobsOrderFunction returns jobs ordering function, or nil, if name doesn't refer to any function.
func GetJobsOrderFunction(name string) JobsOrderFunc {
	switch name {
	case CompactionOrderNewestFirst:
		return sortJobsByNewestBlocksFirst
	case CompactionOrderOldestFirst:
		return sortJobsBySmallestRangeOldestBlocksFirst
	default:
		return nil
	}
}

// sortJobsBySmallestRangeOldestBlocksFirst returns input jobs sorted by smallest range, oldest min time first.
// The rationale of this sorting is that we may want to favor smaller ranges first (ie. to deduplicate samples
// sooner than later) and older ones are more likely to be "complete" (no missing block still to be uploaded).
// Split jobs are moved to the beginning of the output, because merge jobs are only generated if there are no split jobs in the
// same time range, so finishing split jobs first unblocks more jobs and gives opportunity to more compactors
// to work on them.
func sortJobsBySmallestRangeOldestBlocksFirst(jobs []*Job) []*Job {
	sort.SliceStable(jobs, func(i, j int) bool {
		// Move split jobs to the front.
		if jobs[i].UseSplitting() && !jobs[j].UseSplitting() {
			return true
		}

		if !jobs[i].UseSplitting() && jobs[j].UseSplitting() {
			return false
		}

		checkLength := true
		// Don't check length for splitting jobs. We want to the oldest split blocks to be first, no matter the length.
		if jobs[i].UseSplitting() && jobs[j].UseSplitting() {
			checkLength = false
		}

		if checkLength {
			iLength := jobs[i].MaxTime() - jobs[i].MinTime()
			jLength := jobs[j].MaxTime() - jobs[j].MinTime()

			if iLength != jLength {
				return iLength < jLength
			}
		}

		if jobs[i].MinTime() != jobs[j].MinTime() {
			return jobs[i].MinTime() < jobs[j].MinTime()
		}

		// Guarantee stable sort for tests.
		return jobs[i].Key() < jobs[j].Key()
	})

	return jobs
}

// sortJobsByNewestBlocksFirst returns input jobs sorted by most recent time ranges first
// (regardless of their compaction level). The rationale of this sorting is that in case the
// compactor is lagging behind, we compact up to the largest range (eg. 24h) the most recent
// blocks first and the move to older ones. Most recent blocks are the one more likely to be queried.
func sortJobsByNewestBlocksFirst(jobs []*Job) []*Job {
	sort.SliceStable(jobs, func(i, j int) bool {
		iMaxTime := jobs[i].MaxTime()
		jMaxTime := jobs[j].MaxTime()
		if iMaxTime != jMaxTime {
			return iMaxTime > jMaxTime
		}

		iLength := iMaxTime - jobs[i].MinTime()
		jLength := jMaxTime - jobs[j].MinTime()
		if iLength != jLength {
			return iLength < jLength
		}

		// Guarantee stable sort for tests.
		return jobs[i].Key() < jobs[j].Key()
	})

	return jobs
}
