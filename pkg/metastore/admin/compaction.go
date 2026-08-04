package admin

import (
	"cmp"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	httputil "github.com/grafana/pyroscope/v2/pkg/util/http"
)

// maxCompactionJobRows and maxCompactionQueueRows limit the number of
// individual jobs and planner queues rendered on the compaction page.
// The summary sections cover the entire schedule regardless of the limits.
const (
	maxCompactionJobRows   = 1000
	maxCompactionQueueRows = 1000
)

type compactionPageContent struct {
	Now time.Time

	// Scheduler configuration.
	LeaseDuration time.Duration
	MaxFailures   uint64
	MaxQueueSize  uint64

	// Job counters over the entire schedule.
	TotalJobs    int
	Unassigned   int
	InProgress   int
	LeaseExpired int
	Failed       int

	Levels        []compactionLevelSummary
	Workers       []compactionWorkerSummary
	Jobs          []compactionJobRow
	TruncatedJobs int

	Queues          []compactionQueueRow
	TruncatedQueues int
	QueuedBlocks    uint64
}

type compactionLevelSummary struct {
	Level        uint32
	Total        int
	Unassigned   int
	InProgress   int
	LeaseExpired int
	Failed       int
	OldestJobAge string
}

type compactionWorkerSummary struct {
	Worker       string
	Jobs         int
	LeaseExpired int
	Failed       int
	LastUpdate   string
}

type compactionJobRow struct {
	Name          string
	Tenant        string
	Shard         uint32
	Level         uint32
	Status        string
	StatusBadge   string
	Worker        string
	Reassignments uint32
	SourceBlocks  uint32
	Token         uint64
	Age           string
	Assigned      string
	Lease         string
}

type compactionQueueRow struct {
	Tenant      string
	Shard       uint32
	Level       uint32
	Blocks      uint64
	OldestBlock string
	NewestBlock string
}

func (a *Admin) CompactionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state, err := a.compactionClient.GetCompactionState(r.Context(), new(metastorev1.GetCompactionStateRequest))
		if err != nil {
			httputil.Error(w, err)
			return
		}
		content := buildCompactionPageContent(state, time.Now().UTC())
		if err = pageTemplates.compactionTemplate.Execute(w, content); err != nil {
			httputil.Error(w, err)
		}
	})
}

func buildCompactionPageContent(state *metastorev1.GetCompactionStateResponse, now time.Time) *compactionPageContent {
	content := &compactionPageContent{
		Now:           now,
		LeaseDuration: time.Duration(state.JobLeaseDuration),
		MaxFailures:   state.JobMaxFailures,
		MaxQueueSize:  state.MaxJobQueueSize,
		TotalJobs:     len(state.CompactionJobs),
	}

	jobs := slices.Clone(state.CompactionJobs)
	// Jobs are listed in the scheduling order: the job at the top of a level
	// is the next to be assigned within this level. See the scheduler
	// documentation for details.
	slices.SortFunc(jobs, compareJobDetails)

	levels := make(map[uint32]*compactionLevelSummary)
	levelOldest := make(map[uint32]int64)
	workers := make(map[string]*compactionWorkerSummary)
	workerUpdated := make(map[string]int64)

	for _, job := range jobs {
		status, badge := jobDisplayStatus(job, state.JobMaxFailures, now)
		switch status {
		case jobStatusUnassigned:
			content.Unassigned++
		case jobStatusInProgress:
			content.InProgress++
		case jobStatusLeaseExpired:
			content.LeaseExpired++
		case jobStatusFailed:
			content.Failed++
		}

		level := levels[job.CompactionLevel]
		if level == nil {
			level = &compactionLevelSummary{Level: job.CompactionLevel}
			levels[job.CompactionLevel] = level
		}
		level.Total++
		switch status {
		case jobStatusUnassigned:
			level.Unassigned++
		case jobStatusInProgress:
			level.InProgress++
		case jobStatusLeaseExpired:
			level.LeaseExpired++
		case jobStatusFailed:
			level.Failed++
		}
		if oldest, ok := levelOldest[job.CompactionLevel]; !ok || job.AddedAt < oldest {
			levelOldest[job.CompactionLevel] = job.AddedAt
		}

		if job.WorkerId != "" {
			worker := workers[job.WorkerId]
			if worker == nil {
				worker = &compactionWorkerSummary{Worker: job.WorkerId}
				workers[job.WorkerId] = worker
			}
			worker.Jobs++
			switch status {
			case jobStatusLeaseExpired:
				worker.LeaseExpired++
			case jobStatusFailed:
				worker.Failed++
			}
			if job.UpdatedAt > workerUpdated[job.WorkerId] {
				workerUpdated[job.WorkerId] = job.UpdatedAt
			}
		}

		if len(content.Jobs) < maxCompactionJobRows {
			content.Jobs = append(content.Jobs, compactionJobRow{
				Name:          job.Name,
				Tenant:        job.Tenant,
				Shard:         job.Shard,
				Level:         job.CompactionLevel,
				Status:        status,
				StatusBadge:   badge,
				Worker:        job.WorkerId,
				Reassignments: job.Failures,
				SourceBlocks:  job.SourceBlocks,
				Token:         job.Token,
				Age:           formatAge(job.AddedAt, now),
				Assigned:      formatAge(job.AssignedAt, now),
				Lease:         formatLease(job, now),
			})
		}
	}
	content.TruncatedJobs = len(jobs) - len(content.Jobs)

	content.Levels = make([]compactionLevelSummary, 0, len(levels))
	for _, level := range levels {
		level.OldestJobAge = formatAge(levelOldest[level.Level], now)
		content.Levels = append(content.Levels, *level)
	}
	slices.SortFunc(content.Levels, func(a, b compactionLevelSummary) int {
		return cmp.Compare(a.Level, b.Level)
	})

	content.Workers = make([]compactionWorkerSummary, 0, len(workers))
	for _, worker := range workers {
		worker.LastUpdate = formatAge(workerUpdated[worker.Worker], now)
		content.Workers = append(content.Workers, *worker)
	}
	slices.SortFunc(content.Workers, func(a, b compactionWorkerSummary) int {
		return strings.Compare(a.Worker, b.Worker)
	})

	for _, q := range state.CompactionQueues {
		content.QueuedBlocks += q.Blocks
		if len(content.Queues) < maxCompactionQueueRows {
			content.Queues = append(content.Queues, compactionQueueRow{
				Tenant:      q.Tenant,
				Shard:       q.Shard,
				Level:       q.CompactionLevel,
				Blocks:      q.Blocks,
				OldestBlock: formatAge(q.OldestBlockAt, now),
				NewestBlock: formatAge(q.NewestBlockAt, now),
			})
		}
	}
	content.TruncatedQueues = len(state.CompactionQueues) - len(content.Queues)

	return content
}

const (
	jobStatusUnassigned   = "unassigned"
	jobStatusInProgress   = "in progress"
	jobStatusLeaseExpired = "lease expired"
	jobStatusFailed       = "failed"
)

// jobDisplayStatus derives the effective job status the same way the
// scheduler does: "lease expired" and "failed" are not explicit states
// but are computed from the lease deadline and the failure count.
// Note that the lease deadline refers to the raft leader clock, whereas
// now is the local clock: the derivation is a close approximation.
func jobDisplayStatus(job *metastorev1.CompactionJobDetails, maxFailures uint64, now time.Time) (status, badge string) {
	switch job.Status {
	case metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED:
		return jobStatusUnassigned, "secondary"
	case metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS:
		switch {
		case maxFailures > 0 && uint64(job.Failures) >= maxFailures:
			return jobStatusFailed, "danger"
		case now.UnixNano() > job.LeaseExpiresAt:
			return jobStatusLeaseExpired, "warning"
		default:
			return jobStatusInProgress, "success"
		}
	default:
		return strings.ToLower(strings.TrimPrefix(job.Status.String(), "COMPACTION_STATUS_")), "info"
	}
}

// compareJobDetails matches the scheduling order of the jobs:
// see the scheduler queue implementation for reference.
func compareJobDetails(a, b *metastorev1.CompactionJobDetails) int {
	if a.CompactionLevel != b.CompactionLevel {
		return cmp.Compare(a.CompactionLevel, b.CompactionLevel)
	}
	if a.Status != b.Status {
		return cmp.Compare(a.Status, b.Status)
	}
	if a.Failures != b.Failures {
		return cmp.Compare(a.Failures, b.Failures)
	}
	if a.LeaseExpiresAt != b.LeaseExpiresAt {
		return cmp.Compare(a.LeaseExpiresAt, b.LeaseExpiresAt)
	}
	return strings.Compare(a.Name, b.Name)
}

// formatAge renders the time elapsed since the given timestamp
// (Unix nanoseconds), or "-" if the timestamp is not known.
func formatAge(nanos int64, now time.Time) string {
	if nanos <= 0 {
		return "-"
	}
	return formatDuration(now.Sub(time.Unix(0, nanos))) + " ago"
}

func formatLease(job *metastorev1.CompactionJobDetails, now time.Time) string {
	if job.LeaseExpiresAt <= 0 {
		return "-"
	}
	d := time.Unix(0, job.LeaseExpiresAt).Sub(now)
	if d >= 0 {
		return "expires in " + formatDuration(d)
	}
	return "expired " + formatDuration(-d) + " ago"
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%.1fd", d.Hours()/24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return d.Round(time.Millisecond).String()
	}
}
