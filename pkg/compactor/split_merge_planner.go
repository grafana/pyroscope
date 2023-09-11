// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/split_merge_planner.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"context"
	"fmt"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

type SplitAndMergePlanner struct {
	ranges []int64
}

func NewSplitAndMergePlanner(ranges []int64) *SplitAndMergePlanner {
	return &SplitAndMergePlanner{
		ranges: ranges,
	}
}

// Plan implements compact.Planner.
func (c *SplitAndMergePlanner) Plan(_ context.Context, metasByMinTime []*block.Meta) ([]*block.Meta, error) {
	// The split-and-merge grouper creates single groups of blocks that are expected to be
	// compacted together, so there's no real planning to do here (reason why this function is
	// just a pass-through). However, we want to run extra checks before proceeding.
	if len(metasByMinTime) == 0 {
		return metasByMinTime, nil
	}

	// Ensure all blocks fits within the largest range. This is a double check
	// to ensure there's no bug in the previous blocks grouping, given this Plan()
	// is just a pass-through.
	largestRange := c.ranges[len(c.ranges)-1]
	rangeStart := getRangeStart(metasByMinTime[0], largestRange)
	rangeEnd := rangeStart + largestRange

	for _, b := range metasByMinTime {
		if int64(b.MinTime) < rangeStart || int64(b.MaxTime) > rangeEnd {
			return nil, fmt.Errorf("block %s with time range %d:%d is outside the largest expected range %d:%d",
				b.ULID.String(),
				b.MinTime,
				b.MaxTime,
				rangeStart,
				rangeEnd)
		}
	}

	return metasByMinTime, nil
}
