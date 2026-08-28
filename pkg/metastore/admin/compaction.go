package admin

import (
	"cmp"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	httputil "github.com/grafana/pyroscope/v2/pkg/util/http"
)

// The compaction page is split in two: the overview aggregates the entire
// schedule and never lists per-tenant entities beyond a bounded index, and
// the tenant page lists the jobs and the queues of a single tenant. The
// limits below bound the number of rows rendered by each of the sections;
// the summaries they accompany always cover the entire schedule.
const (
	maxCompactionJobRows       = 1000
	maxCompactionQueueRows     = 1000
	maxCompactionTenantRows    = 100
	maxCompactionAttentionRows = 50
)

// blockLinkPadding widens the time window of the block listing links. The
// job and queue timestamps record when the blocks were queued for compaction,
// whereas the block listing filters on the block time range: the padding
// compensates for the difference.
const blockLinkPadding = time.Hour

// Tenant index sort orders. Attention is the default: it surfaces the
// tenants compaction is not making progress on, which is the reason to
// open the page in the first place.
const (
	tenantSortAttention = "attention"
	tenantSortName      = "name"
	tenantSortJobs      = "jobs"
	tenantSortBlocks    = "blocks"
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

	// Planner counters over all the queues.
	QueuedBlocks uint64
	TotalQueues  int

	Levels  []compactionLevelSummary
	Workers []compactionWorkerSummary

	// Tenant index: the entry point of the per-tenant drill-down.
	TotalTenants     int
	MatchedTenants   int
	TruncatedTenants int
	Filter           string
	Sort             string
	Tenants          []compactionTenantSummary

	// Jobs that need attention across all tenants: the only per-job
	// listing of the overview, and a bounded one.
	Attention          []compactionJobRow
	TotalAttention     int
	TruncatedAttention int
}

type compactionTenantPageContent struct {
	Now    time.Time
	Tenant string

	// Scheduler configuration.
	LeaseDuration time.Duration
	MaxFailures   uint64

	// Job counters over the jobs of the tenant.
	TotalJobs    int
	Unassigned   int
	InProgress   int
	LeaseExpired int
	Failed       int

	QueuedBlocks uint64
	TotalQueues  int

	Levels []compactionLevelSummary

	Jobs          []compactionJobRow
	TruncatedJobs int

	Queues          []compactionQueueRow
	TruncatedQueues int

	// BlocksPath links to the block listing of the tenant in the object
	// store browser. Empty if the tenant is not addressable there.
	BlocksPath string
}

// Empty reports whether the tenant is present in the compaction state at all.
func (c *compactionTenantPageContent) Empty() bool {
	return c.TotalJobs == 0 && c.TotalQueues == 0
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

type compactionTenantSummary struct {
	Tenant       string
	Jobs         int
	Unassigned   int
	InProgress   int
	LeaseExpired int
	Failed       int
	Queues       int
	QueuedBlocks uint64
	OldestJobAge string
	Path         string

	// oldestJob is the creation timestamp of the oldest job of the tenant,
	// in Unix nanoseconds; zero if the tenant has no jobs.
	oldestJob int64
}

// Attention is the number of jobs the scheduler is not making progress on:
// the primary sort key of the default tenant order.
func (s compactionTenantSummary) Attention() int { return s.LeaseExpired + s.Failed }

// Known reports whether the tenant can be drilled down into. Jobs with an
// unknown tenant (e.g., if the job plan is missing) are grouped together
// and are not addressable.
func (s compactionTenantSummary) Known() bool { return s.Tenant != "" }

type compactionJobRow struct {
	Name          string
	Tenant        string
	TenantPath    string
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
	BlocksPath    string
}

type compactionQueueRow struct {
	Tenant      string
	Shard       uint32
	Level       uint32
	Blocks      uint64
	OldestBlock string
	NewestBlock string
	BlocksPath  string
}

func (a *Admin) CompactionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state, err := a.compactionClient.GetCompactionState(r.Context(), new(metastorev1.GetCompactionStateRequest))
		if err != nil {
			httputil.Error(w, err)
			return
		}
		now := time.Now().UTC()
		query := r.URL.Query()
		if tenant := query.Get("tenant"); tenant != "" {
			content := buildCompactionTenantPageContent(state, now, tenant)
			if err = pageTemplates.compactionTenantTemplate.Execute(w, content); err != nil {
				httputil.Error(w, err)
			}
			return
		}
		content := buildCompactionPageContent(state, now, query.Get("q"), query.Get("sort"))
		if err = pageTemplates.compactionTemplate.Execute(w, content); err != nil {
			httputil.Error(w, err)
		}
	})
}

// buildCompactionPageContent aggregates the compaction state into the global
// overview. Nothing rendered by the overview grows with the number of tenants
// or jobs: the tenant index and the attention list are both bounded, and the
// summaries are aggregates over the entire schedule.
func buildCompactionPageContent(
	state *metastorev1.GetCompactionStateResponse,
	now time.Time,
	filter string,
	sortOrder string,
) *compactionPageContent {
	content := &compactionPageContent{
		Now:           now,
		LeaseDuration: time.Duration(state.JobLeaseDuration),
		MaxFailures:   state.JobMaxFailures,
		MaxQueueSize:  state.MaxJobQueueSize,
		TotalJobs:     len(state.CompactionJobs),
		TotalQueues:   len(state.CompactionQueues),
		Filter:        strings.TrimSpace(filter),
		Sort:          tenantSortOrder(sortOrder),
	}

	levels := make(map[uint32]*compactionLevelSummary)
	levelOldest := make(map[uint32]int64)
	workers := make(map[string]*compactionWorkerSummary)
	workerUpdated := make(map[string]int64)
	tenants := make(map[string]*compactionTenantSummary)
	var attention []*metastorev1.CompactionJobDetails

	for _, job := range state.CompactionJobs {
		status, _ := jobDisplayStatus(job, state.JobMaxFailures, now)
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
		addJobToLevel(level, status)
		if oldest, ok := levelOldest[job.CompactionLevel]; !ok || job.AddedAt < oldest {
			levelOldest[job.CompactionLevel] = job.AddedAt
		}

		tenant := tenants[job.Tenant]
		if tenant == nil {
			tenant = &compactionTenantSummary{Tenant: job.Tenant}
			tenants[job.Tenant] = tenant
		}
		tenant.Jobs++
		switch status {
		case jobStatusUnassigned:
			tenant.Unassigned++
		case jobStatusInProgress:
			tenant.InProgress++
		case jobStatusLeaseExpired:
			tenant.LeaseExpired++
		case jobStatusFailed:
			tenant.Failed++
		}
		if job.AddedAt > 0 && (tenant.oldestJob == 0 || job.AddedAt < tenant.oldestJob) {
			tenant.oldestJob = job.AddedAt
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

		if status == jobStatusLeaseExpired || status == jobStatusFailed {
			attention = append(attention, job)
		}
	}

	for _, q := range state.CompactionQueues {
		content.QueuedBlocks += q.Blocks
		tenant := tenants[q.Tenant]
		if tenant == nil {
			tenant = &compactionTenantSummary{Tenant: q.Tenant}
			tenants[q.Tenant] = tenant
		}
		tenant.Queues++
		tenant.QueuedBlocks += q.Blocks
	}

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

	content.TotalTenants = len(tenants)
	matched := make([]compactionTenantSummary, 0, min(len(tenants), maxCompactionTenantRows))
	for _, tenant := range tenants {
		if !matchesTenantFilter(tenant.Tenant, content.Filter) {
			continue
		}
		tenant.OldestJobAge = formatAge(tenant.oldestJob, now)
		tenant.Path = tenantPath(tenant.Tenant, content.Sort)
		matched = append(matched, *tenant)
	}
	content.MatchedTenants = len(matched)
	slices.SortFunc(matched, tenantComparator(content.Sort))
	if len(matched) > maxCompactionTenantRows {
		content.TruncatedTenants = len(matched) - maxCompactionTenantRows
		matched = matched[:maxCompactionTenantRows]
	}
	content.Tenants = matched

	// Jobs that need attention are listed in the order of severity, so that
	// the truncated tail is the least interesting part of the list.
	slices.SortFunc(attention, compareAttentionJobs)
	content.TotalAttention = len(attention)
	if len(attention) > maxCompactionAttentionRows {
		content.TruncatedAttention = len(attention) - maxCompactionAttentionRows
		attention = attention[:maxCompactionAttentionRows]
	}
	content.Attention = make([]compactionJobRow, 0, len(attention))
	for _, job := range attention {
		content.Attention = append(content.Attention, newCompactionJobRow(job, state.JobMaxFailures, now))
	}

	return content
}

// buildCompactionTenantPageContent is the drill-down: the exhaustive listing
// of the jobs and the queues of a single tenant.
func buildCompactionTenantPageContent(
	state *metastorev1.GetCompactionStateResponse,
	now time.Time,
	tenant string,
) *compactionTenantPageContent {
	content := &compactionTenantPageContent{
		Now:           now,
		Tenant:        tenant,
		LeaseDuration: time.Duration(state.JobLeaseDuration),
		MaxFailures:   state.JobMaxFailures,
		BlocksPath:    tenantBlocksPath(tenant),
	}

	var jobs []*metastorev1.CompactionJobDetails
	for _, job := range state.CompactionJobs {
		if job.Tenant == tenant {
			jobs = append(jobs, job)
		}
	}
	content.TotalJobs = len(jobs)
	// Jobs are listed in the scheduling order: the job at the top of a level
	// is the next to be assigned within this level. See the scheduler
	// documentation for details.
	slices.SortFunc(jobs, compareJobDetails)

	levels := make(map[uint32]*compactionLevelSummary)
	levelOldest := make(map[uint32]int64)

	for _, job := range jobs {
		status, _ := jobDisplayStatus(job, state.JobMaxFailures, now)
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
		addJobToLevel(level, status)
		if oldest, ok := levelOldest[job.CompactionLevel]; !ok || job.AddedAt < oldest {
			levelOldest[job.CompactionLevel] = job.AddedAt
		}

		if len(content.Jobs) < maxCompactionJobRows {
			content.Jobs = append(content.Jobs, newCompactionJobRow(job, state.JobMaxFailures, now))
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

	for _, q := range state.CompactionQueues {
		if q.Tenant != tenant {
			continue
		}
		content.TotalQueues++
		content.QueuedBlocks += q.Blocks
		if len(content.Queues) < maxCompactionQueueRows {
			row := compactionQueueRow{
				Tenant:      q.Tenant,
				Shard:       q.Shard,
				Level:       q.CompactionLevel,
				Blocks:      q.Blocks,
				OldestBlock: formatAge(q.OldestBlockAt, now),
				NewestBlock: formatAge(q.NewestBlockAt, now),
			}
			if q.OldestBlockAt > 0 && q.NewestBlockAt > 0 {
				row.BlocksPath = blocksPath(q.Tenant,
					time.Unix(0, q.OldestBlockAt),
					time.Unix(0, q.NewestBlockAt))
			} else {
				row.BlocksPath = tenantBlocksPath(q.Tenant)
			}
			content.Queues = append(content.Queues, row)
		}
	}
	content.TruncatedQueues = content.TotalQueues - len(content.Queues)

	return content
}

func newCompactionJobRow(job *metastorev1.CompactionJobDetails, maxFailures uint64, now time.Time) compactionJobRow {
	status, badge := jobDisplayStatus(job, maxFailures, now)
	row := compactionJobRow{
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
	}
	if job.Tenant != "" {
		row.TenantPath = tenantPath(job.Tenant, "")
	}
	if job.AddedAt > 0 {
		// The source blocks were queued shortly before the job was created.
		added := time.Unix(0, job.AddedAt)
		row.BlocksPath = blocksPath(job.Tenant, added, added)
	} else {
		row.BlocksPath = tenantBlocksPath(job.Tenant)
	}
	return row
}

func addJobToLevel(level *compactionLevelSummary, status string) {
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
}

func tenantSortOrder(s string) string {
	switch s {
	case tenantSortName, tenantSortJobs, tenantSortBlocks:
		return s
	default:
		return tenantSortAttention
	}
}

func matchesTenantFilter(tenant, filter string) bool {
	if filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(tenant), strings.ToLower(filter))
}

// tenantComparator orders the tenant index. Every order falls back to the
// tenant name, so that the truncated listing is stable across reloads.
func tenantComparator(sortOrder string) func(a, b compactionTenantSummary) int {
	switch sortOrder {
	case tenantSortName:
		return func(a, b compactionTenantSummary) int {
			return strings.Compare(a.Tenant, b.Tenant)
		}
	case tenantSortJobs:
		return func(a, b compactionTenantSummary) int {
			if c := cmp.Compare(b.Jobs, a.Jobs); c != 0 {
				return c
			}
			return strings.Compare(a.Tenant, b.Tenant)
		}
	case tenantSortBlocks:
		return func(a, b compactionTenantSummary) int {
			if c := cmp.Compare(b.QueuedBlocks, a.QueuedBlocks); c != 0 {
				return c
			}
			return strings.Compare(a.Tenant, b.Tenant)
		}
	default:
		return func(a, b compactionTenantSummary) int {
			if c := cmp.Compare(b.Attention(), a.Attention()); c != 0 {
				return c
			}
			if c := cmp.Compare(b.Jobs, a.Jobs); c != 0 {
				return c
			}
			if c := cmp.Compare(b.QueuedBlocks, a.QueuedBlocks); c != 0 {
				return c
			}
			return strings.Compare(a.Tenant, b.Tenant)
		}
	}
}

// compareAttentionJobs orders the jobs that need attention by severity:
// the most reassigned jobs first, then the ones whose lease expired the
// longest time ago.
func compareAttentionJobs(a, b *metastorev1.CompactionJobDetails) int {
	if c := cmp.Compare(b.Failures, a.Failures); c != 0 {
		return c
	}
	if c := cmp.Compare(a.LeaseExpiresAt, b.LeaseExpiresAt); c != 0 {
		return c
	}
	return strings.Compare(a.Name, b.Name)
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

// tenantPath builds the link to the drill-down page of the tenant. The sort
// order of the tenant index is carried over, so that the browser navigation
// returns to the listing the tenant was picked from.
func tenantPath(tenant, sortOrder string) string {
	path := "/metastore-compaction?tenant=" + url.QueryEscape(tenant)
	if sortOrder != "" && sortOrder != tenantSortAttention {
		path += "&sort=" + url.QueryEscape(sortOrder)
	}
	return path
}

// tenantBlocksPath links to the block listing of the tenant in the object
// store browser, over its default time range.
//
// Level 0 blocks are segments written by the segment writers: they are
// multi-tenant and carry no tenant of their own, so the compaction entities
// that operate on them (identified by an empty tenant) are not addressable
// in the listing, which queries the metadata by tenant. No link is built
// for them.
func tenantBlocksPath(tenant string) string {
	if tenant == "" {
		return ""
	}
	return "/ops/object-store/tenants/" + url.PathEscape(tenant) + "/blocks"
}

// blocksPath links to the block listing of the tenant, narrowed to the time
// window the blocks are expected to fall into. The window is only a hint:
// the listing filters on the block time range, whereas the window is derived
// from the time the blocks were queued for compaction, and it is not
// restricted to the blocks of a single job or queue. The scheduler does not
// currently report the source block identifiers.
func blocksPath(tenant string, from, to time.Time) string {
	if tenant == "" {
		return ""
	}
	query := url.Values{
		"queryFrom": []string{from.Add(-blockLinkPadding).UTC().Format(time.RFC3339)},
		"queryTo":   []string{to.Add(blockLinkPadding).UTC().Format(time.RFC3339)},
	}
	return tenantBlocksPath(tenant) + "?" + query.Encode()
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

// formatDuration renders the duration with a second resolution, using at
// most two adjacent units: e.g., "<1s", "4s", "1m12s", "2h3m", "290d2h".
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}
	d = d.Round(time.Second)
	switch {
	case d >= 24*time.Hour:
		days := d / (24 * time.Hour)
		hours := (d % (24 * time.Hour)) / time.Hour
		if hours == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd%dh", days, hours)
	case d >= time.Hour:
		hours := d / time.Hour
		minutes := (d % time.Hour) / time.Minute
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, minutes)
	case d >= time.Minute:
		minutes := d / time.Minute
		seconds := (d % time.Minute) / time.Second
		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	default:
		return fmt.Sprintf("%ds", d/time.Second)
	}
}
