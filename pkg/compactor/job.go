// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/job.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.
package compactor

import (
	"context"
	"fmt"
	"math"
	"path"
	"sort"
	"time"

	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

// Job holds a compaction job, which consists of a group of blocks that should be compacted together.
// Not goroutine safe.
type Job struct {
	userID         string
	key            string
	labels         labels.Labels
	resolution     int64
	metasByMinTime []*block.Meta
	useSplitting   bool
	shardingKey    string

	// The number of shards to split compacted block into. Not used if splitting is disabled.
	splitNumShards uint32
	splitStageSize uint32
}

// NewJob returns a new compaction Job.
func NewJob(userID string, key string, lset labels.Labels, resolution int64, useSplitting bool, splitNumShards, splitStageSize uint32, shardingKey string) *Job {
	return &Job{
		userID:         userID,
		key:            key,
		labels:         lset,
		resolution:     resolution,
		useSplitting:   useSplitting,
		splitNumShards: splitNumShards,
		splitStageSize: splitStageSize,
		shardingKey:    shardingKey,
	}
}

// UserID returns the user/tenant to which this job belongs to.
func (job *Job) UserID() string {
	return job.userID
}

// Key returns an identifier for the job.
func (job *Job) Key() string {
	return job.key
}

// AppendMeta the block with the given meta to the job.
func (job *Job) AppendMeta(meta *block.Meta) error {
	if !labels.Equal(labelsWithout(job.labels.Map(), block.HostnameLabel), labelsWithout(meta.Labels, block.HostnameLabel)) {
		return errors.New("block and group labels do not match")
	}
	if job.resolution != meta.Downsample.Resolution {
		return errors.New("block and group resolution do not match")
	}

	job.metasByMinTime = append(job.metasByMinTime, meta)
	sort.Slice(job.metasByMinTime, func(i, j int) bool {
		return job.metasByMinTime[i].MinTime < job.metasByMinTime[j].MinTime
	})
	return nil
}

// IDs returns all sorted IDs of blocks in the job.
func (job *Job) IDs() (ids []ulid.ULID) {
	for _, m := range job.metasByMinTime {
		ids = append(ids, m.ULID)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].Compare(ids[j]) < 0
	})
	return ids
}

// MinTime returns the min time across all job's blocks.
func (job *Job) MinTime() int64 {
	if len(job.metasByMinTime) > 0 {
		return int64(job.metasByMinTime[0].MinTime)
	}
	return math.MaxInt64
}

// MaxTime returns the max time across all job's blocks.
func (job *Job) MaxTime() int64 {
	max := int64(math.MinInt64)
	for _, m := range job.metasByMinTime {
		if int64(m.MaxTime) > max {
			max = int64(m.MaxTime)
		}
	}
	return max
}

// MinCompactionLevel returns the minimum compaction level across all source blocks
// in this job.
func (job *Job) MinCompactionLevel() int {
	min := math.MaxInt

	for _, m := range job.metasByMinTime {
		if m.Compaction.Level < min {
			min = m.Compaction.Level
		}
	}

	return min
}

// Metas returns the metadata for each block that is part of this job, ordered by the block's MinTime
func (job *Job) Metas() []*block.Meta {
	out := make([]*block.Meta, len(job.metasByMinTime))
	copy(out, job.metasByMinTime)
	return out
}

// Labels returns the external labels for the output block(s) of this job.
func (job *Job) Labels() labels.Labels {
	return job.labels
}

// Resolution returns the common downsampling resolution of blocks in the job.
func (job *Job) Resolution() int64 {
	return job.resolution
}

// UseSplitting returns whether blocks should be split into multiple shards when compacted.
func (job *Job) UseSplitting() bool {
	return job.useSplitting
}

// SplittingShards returns the number of output shards to build if splitting is enabled.
func (job *Job) SplittingShards() uint32 {
	return job.splitNumShards
}

// SplitStageSize returns the number of stages split shards will be written to.
func (job *Job) SplitStageSize() uint32 {
	return job.splitStageSize
}

// ShardingKey returns the key used to shard this job across multiple instances.
func (job *Job) ShardingKey() string {
	return job.shardingKey
}

func (job *Job) String() string {
	return fmt.Sprintf("%s (minTime: %d maxTime: %d)", job.Key(), job.MinTime(), job.MaxTime())
}

// jobWaitPeriodElapsed returns whether the 1st level compaction wait period has
// elapsed for the input job. If the wait period has not elapsed, then this function
// also returns the Meta of the first source block encountered for which the wait
// period has not elapsed yet.
func jobWaitPeriodElapsed(ctx context.Context, job *Job, waitPeriod time.Duration, userBucket objstore.Bucket) (bool, *block.Meta, error) {
	if waitPeriod <= 0 {
		return true, nil, nil
	}

	if job.MinCompactionLevel() > 1 {
		return true, nil, nil
	}

	// Check if the job contains any source block uploaded more recently
	// than "wait period" ago.
	threshold := time.Now().Add(-waitPeriod)

	for _, meta := range job.Metas() {
		metaPath := path.Join(meta.ULID.String(), block.MetaFilename)

		attrs, err := userBucket.Attributes(ctx, metaPath)
		if err != nil {
			return false, meta, errors.Wrapf(err, "unable to get object attributes for %s", metaPath)
		}

		if attrs.LastModified.After(threshold) {
			return false, meta, nil
		}
	}

	return true, nil, nil
}
