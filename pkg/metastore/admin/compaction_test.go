package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
)

func testCompactionState(now time.Time) *metastorev1.GetCompactionStateResponse {
	return &metastorev1.GetCompactionStateResponse{
		JobLeaseDuration: (15 * time.Second).Nanoseconds(),
		JobMaxFailures:   3,
		MaxJobQueueSize:  10000,
		CompactionJobs: []*metastorev1.CompactionJobDetails{
			{
				Name:            "8bc44b23a733bee2-T-tenant-a-S1-L0",
				Tenant:          "tenant-a",
				Shard:           1,
				CompactionLevel: 0,
				Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
				Token:           42,
				LeaseExpiresAt:  now.Add(10 * time.Second).UnixNano(),
				AddedAt:         now.Add(-time.Minute).UnixNano(),
				SourceBlocks:    20,
				WorkerId:        "worker-1",
				AssignedAt:      now.Add(-30 * time.Second).UnixNano(),
				UpdatedAt:       now.Add(-5 * time.Second).UnixNano(),
			},
			{
				Name:            "9bc44b23a733bee2-T-tenant-a-S2-L0",
				Tenant:          "tenant-a",
				Shard:           2,
				CompactionLevel: 0,
				Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED,
				AddedAt:         now.Add(-30 * time.Second).UnixNano(),
				SourceBlocks:    20,
			},
			{
				Name:            "abc44b23a733bee2-T-tenant-b-S1-L1",
				Tenant:          "tenant-b",
				Shard:           1,
				CompactionLevel: 1,
				Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
				Token:           40,
				LeaseExpiresAt:  now.Add(-20 * time.Second).UnixNano(),
				AddedAt:         now.Add(-10 * time.Minute).UnixNano(),
				Failures:        1,
				SourceBlocks:    10,
				WorkerId:        "worker-2",
				UpdatedAt:       now.Add(-40 * time.Second).UnixNano(),
			},
			{
				Name:            "bbc44b23a733bee2-T-tenant-b-S2-L1",
				Tenant:          "tenant-b",
				Shard:           2,
				CompactionLevel: 1,
				Status:          metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
				Token:           30,
				LeaseExpiresAt:  now.Add(-time.Hour).UnixNano(),
				AddedAt:         now.Add(-2 * time.Hour).UnixNano(),
				Failures:        3,
				SourceBlocks:    10,
				WorkerId:        "worker-2",
				UpdatedAt:       now.Add(-time.Hour).UnixNano(),
			},
		},
		CompactionQueues: []*metastorev1.CompactionQueueDetails{
			{
				Tenant:          "tenant-a",
				Shard:           1,
				CompactionLevel: 0,
				Blocks:          15,
				OldestBlockAt:   now.Add(-25 * time.Second).UnixNano(),
				NewestBlockAt:   now.Add(-time.Second).UnixNano(),
			},
			{
				Tenant:          "tenant-c",
				Shard:           1,
				CompactionLevel: 0,
				Blocks:          7,
				OldestBlockAt:   now.Add(-5 * time.Second).UnixNano(),
				NewestBlockAt:   now.Add(-time.Second).UnixNano(),
			},
		},
	}
}

func TestCompactionHandler(t *testing.T) {
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(testCompactionState(time.Now()), nil).
		Once()

	a := &Admin{compactionClient: client}
	w := httptest.NewRecorder()
	a.CompactionHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metastore-compaction", nil))

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	// The tenant index links to the drill-down pages.
	assert.Contains(t, body, `href="/metastore-compaction?tenant=tenant-a"`)
	assert.Contains(t, body, `href="/metastore-compaction?tenant=tenant-b"`)
	assert.Contains(t, body, `href="/metastore-compaction?tenant=tenant-c"`)
	// Jobs that need attention are listed globally.
	assert.Contains(t, body, "bbc44b23a733bee2-T-tenant-b-S2-L1")
	assert.Contains(t, body, "worker-2")
	assert.Contains(t, body, "lease expired")
	assert.Contains(t, body, "failed")
	assert.Contains(t, body, "15s")
	// Healthy jobs are not listed on the overview: they are only
	// reachable through the tenant pages.
	assert.NotContains(t, body, "8bc44b23a733bee2-T-tenant-a-S1-L0")
	assert.NotContains(t, body, "9bc44b23a733bee2-T-tenant-a-S2-L0")
}

func TestCompactionHandler_filterAndSort(t *testing.T) {
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(testCompactionState(time.Now()), nil).
		Once()

	a := &Admin{compactionClient: client}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metastore-compaction?q=tenant-c&sort=name", nil)
	a.CompactionHandler().ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "1 of 3 tenants match.")
	// The sort order is carried over to the drill-down link.
	assert.Contains(t, body, `href="/metastore-compaction?tenant=tenant-c&amp;sort=name"`)
	assert.NotContains(t, body, `href="/metastore-compaction?tenant=tenant-a`)
}

func TestCompactionHandler_tenant(t *testing.T) {
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(testCompactionState(time.Now()), nil).
		Once()

	a := &Admin{compactionClient: client}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metastore-compaction?tenant=tenant-a", nil)
	a.CompactionHandler().ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "8bc44b23a733bee2-T-tenant-a-S1-L0")
	assert.Contains(t, body, "9bc44b23a733bee2-T-tenant-a-S2-L0")
	assert.Contains(t, body, "worker-1")
	assert.Contains(t, body, "in progress")
	assert.Contains(t, body, "unassigned")
	// Jobs and queues of the other tenants are not rendered.
	assert.NotContains(t, body, "tenant-b")
	assert.NotContains(t, body, "worker-2")
}

func TestCompactionHandler_tenantNotFound(t *testing.T) {
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(testCompactionState(time.Now()), nil).
		Once()

	a := &Admin{compactionClient: client}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metastore-compaction?tenant=tenant-x", nil)
	a.CompactionHandler().ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "No compaction jobs and no queued blocks for this tenant.")
}

func TestCompactionHandler_error(t *testing.T) {
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(nil, errors.New("no leader")).
		Once()

	a := &Admin{compactionClient: client}
	w := httptest.NewRecorder()
	a.CompactionHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metastore-compaction", nil))
	assert.NotEqual(t, http.StatusOK, w.Code)
}

func TestBuildCompactionPageContent(t *testing.T) {
	now := time.Now()
	content := buildCompactionPageContent(testCompactionState(now), now, "", "")

	assert.Equal(t, 4, content.TotalJobs)
	assert.Equal(t, 1, content.Unassigned)
	assert.Equal(t, 1, content.InProgress)
	assert.Equal(t, 1, content.LeaseExpired)
	assert.Equal(t, 1, content.Failed)
	assert.Equal(t, uint64(22), content.QueuedBlocks)
	assert.Equal(t, 2, content.TotalQueues)

	require.Len(t, content.Levels, 2)
	assert.Equal(t, uint32(0), content.Levels[0].Level)
	assert.Equal(t, 2, content.Levels[0].Total)
	assert.Equal(t, 1, content.Levels[0].Unassigned)
	assert.Equal(t, 1, content.Levels[0].InProgress)
	assert.Equal(t, uint32(1), content.Levels[1].Level)
	assert.Equal(t, 1, content.Levels[1].LeaseExpired)
	assert.Equal(t, 1, content.Levels[1].Failed)

	require.Len(t, content.Workers, 2)
	assert.Equal(t, "worker-1", content.Workers[0].Worker)
	assert.Equal(t, 1, content.Workers[0].Jobs)
	assert.Equal(t, 0, content.Workers[0].LeaseExpired)
	assert.Equal(t, 0, content.Workers[0].Failed)
	assert.Equal(t, "worker-2", content.Workers[1].Worker)
	assert.Equal(t, 2, content.Workers[1].Jobs)
	assert.Equal(t, 1, content.Workers[1].LeaseExpired)
	assert.Equal(t, 1, content.Workers[1].Failed)

	// A tenant with queued blocks but no jobs is part of the index.
	assert.Equal(t, 3, content.TotalTenants)
	assert.Equal(t, 3, content.MatchedTenants)
	assert.Zero(t, content.TruncatedTenants)
	require.Len(t, content.Tenants, 3)

	// The default order surfaces the tenants the scheduler
	// is not making progress on.
	assert.Equal(t, tenantSortAttention, content.Sort)
	assert.Equal(t, "tenant-b", content.Tenants[0].Tenant)
	assert.Equal(t, 2, content.Tenants[0].Attention())
	assert.Equal(t, 1, content.Tenants[0].LeaseExpired)
	assert.Equal(t, 1, content.Tenants[0].Failed)
	assert.Zero(t, content.Tenants[0].QueuedBlocks)
	assert.Equal(t, "/metastore-compaction?tenant=tenant-b", content.Tenants[0].Path)

	assert.Equal(t, "tenant-a", content.Tenants[1].Tenant)
	assert.Equal(t, 2, content.Tenants[1].Jobs)
	assert.Equal(t, 1, content.Tenants[1].Unassigned)
	assert.Equal(t, 1, content.Tenants[1].InProgress)
	assert.Equal(t, 1, content.Tenants[1].Queues)
	assert.Equal(t, uint64(15), content.Tenants[1].QueuedBlocks)
	assert.Equal(t, "1m ago", content.Tenants[1].OldestJobAge)

	assert.Equal(t, "tenant-c", content.Tenants[2].Tenant)
	assert.Zero(t, content.Tenants[2].Jobs)
	assert.Equal(t, uint64(7), content.Tenants[2].QueuedBlocks)
	assert.Equal(t, "-", content.Tenants[2].OldestJobAge)

	// Jobs needing attention are ordered by severity.
	assert.Equal(t, 2, content.TotalAttention)
	assert.Zero(t, content.TruncatedAttention)
	require.Len(t, content.Attention, 2)
	assert.Equal(t, "bbc44b23a733bee2-T-tenant-b-S2-L1", content.Attention[0].Name)
	assert.Equal(t, jobStatusFailed, content.Attention[0].Status)
	assert.Equal(t, "/metastore-compaction?tenant=tenant-b", content.Attention[0].TenantPath)
	assert.Equal(t, "abc44b23a733bee2-T-tenant-b-S1-L1", content.Attention[1].Name)
	assert.Equal(t, jobStatusLeaseExpired, content.Attention[1].Status)
}

func TestBuildCompactionPageContent_filter(t *testing.T) {
	now := time.Now()
	state := testCompactionState(now)

	content := buildCompactionPageContent(state, now, "TENANT-A", "")
	assert.Equal(t, "TENANT-A", content.Filter)
	assert.Equal(t, 3, content.TotalTenants)
	assert.Equal(t, 1, content.MatchedTenants)
	require.Len(t, content.Tenants, 1)
	assert.Equal(t, "tenant-a", content.Tenants[0].Tenant)

	// The filter only narrows the tenant index: the counters
	// still cover the entire schedule.
	assert.Equal(t, 4, content.TotalJobs)
	assert.Equal(t, uint64(22), content.QueuedBlocks)
	assert.Len(t, content.Attention, 2)

	content = buildCompactionPageContent(state, now, "  ", "")
	assert.Empty(t, content.Filter)
	assert.Equal(t, 3, content.MatchedTenants)

	content = buildCompactionPageContent(state, now, "nothing", "")
	assert.Zero(t, content.MatchedTenants)
	assert.Empty(t, content.Tenants)
}

func TestBuildCompactionPageContent_sort(t *testing.T) {
	now := time.Now()
	state := testCompactionState(now)

	for _, test := range []struct {
		sort     string
		expected []string
	}{
		{sort: "", expected: []string{"tenant-b", "tenant-a", "tenant-c"}},
		{sort: "invalid", expected: []string{"tenant-b", "tenant-a", "tenant-c"}},
		{sort: tenantSortName, expected: []string{"tenant-a", "tenant-b", "tenant-c"}},
		{sort: tenantSortJobs, expected: []string{"tenant-a", "tenant-b", "tenant-c"}},
		{sort: tenantSortBlocks, expected: []string{"tenant-a", "tenant-c", "tenant-b"}},
	} {
		t.Run(test.sort, func(t *testing.T) {
			content := buildCompactionPageContent(state, now, "", test.sort)
			names := make([]string, 0, len(content.Tenants))
			for _, tenant := range content.Tenants {
				names = append(names, tenant.Tenant)
			}
			assert.Equal(t, test.expected, names)
		})
	}
}

func TestBuildCompactionPageContent_truncation(t *testing.T) {
	now := time.Now()
	state := &metastorev1.GetCompactionStateResponse{JobMaxFailures: 3}
	for i := 0; i < maxCompactionTenantRows+10; i++ {
		state.CompactionQueues = append(state.CompactionQueues, &metastorev1.CompactionQueueDetails{
			Tenant: fmt.Sprintf("tenant-%03d", i),
			Blocks: 2,
		})
	}
	for i := 0; i < maxCompactionAttentionRows+5; i++ {
		state.CompactionJobs = append(state.CompactionJobs, &metastorev1.CompactionJobDetails{
			Name:           fmt.Sprintf("job-%03d", i),
			Tenant:         fmt.Sprintf("tenant-%03d", i),
			Status:         metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
			LeaseExpiresAt: now.Add(-time.Minute).UnixNano(),
		})
	}

	content := buildCompactionPageContent(state, now, "", "")
	assert.Equal(t, maxCompactionTenantRows+10, content.TotalTenants)
	assert.Equal(t, maxCompactionTenantRows+10, content.MatchedTenants)
	assert.Len(t, content.Tenants, maxCompactionTenantRows)
	assert.Equal(t, 10, content.TruncatedTenants)

	assert.Equal(t, maxCompactionAttentionRows+5, content.TotalAttention)
	assert.Len(t, content.Attention, maxCompactionAttentionRows)
	assert.Equal(t, 5, content.TruncatedAttention)

	// The counters cover the entities that are not displayed.
	assert.Equal(t, maxCompactionAttentionRows+5, content.LeaseExpired)
	assert.Equal(t, uint64(2*(maxCompactionTenantRows+10)), content.QueuedBlocks)
}

// The truncation and empty-state alerts are only rendered when the limits
// are exceeded: make sure the template branches execute.
func TestCompactionHandler_truncation(t *testing.T) {
	now := time.Now()
	state := &metastorev1.GetCompactionStateResponse{JobMaxFailures: 3}
	for i := 0; i < maxCompactionTenantRows+3; i++ {
		state.CompactionQueues = append(state.CompactionQueues, &metastorev1.CompactionQueueDetails{
			Tenant: fmt.Sprintf("tenant-%03d", i),
			Blocks: 2,
		})
	}
	for i := 0; i < maxCompactionAttentionRows+7; i++ {
		state.CompactionJobs = append(state.CompactionJobs, &metastorev1.CompactionJobDetails{
			Name:           fmt.Sprintf("job-%03d", i),
			Tenant:         fmt.Sprintf("tenant-%03d", i),
			Status:         metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
			LeaseExpiresAt: now.Add(-time.Minute).UnixNano(),
		})
	}
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(state, nil).
		Once()

	a := &Admin{compactionClient: client}
	w := httptest.NewRecorder()
	a.CompactionHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metastore-compaction", nil))

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "3 more are not")
	assert.Contains(t, body, "Showing the 50 most affected jobs of 57.")
}

func TestBuildCompactionTenantPageContent(t *testing.T) {
	now := time.Now()
	content := buildCompactionTenantPageContent(testCompactionState(now), now, "tenant-a")

	assert.False(t, content.Empty())
	assert.Equal(t, "tenant-a", content.Tenant)
	assert.Equal(t, 2, content.TotalJobs)
	assert.Equal(t, 1, content.Unassigned)
	assert.Equal(t, 1, content.InProgress)
	assert.Zero(t, content.LeaseExpired)
	assert.Zero(t, content.Failed)
	assert.Equal(t, uint64(15), content.QueuedBlocks)
	assert.Equal(t, 1, content.TotalQueues)

	require.Len(t, content.Levels, 1)
	assert.Equal(t, uint32(0), content.Levels[0].Level)
	assert.Equal(t, 2, content.Levels[0].Total)

	// Jobs are ordered by level and scheduling priority:
	// unassigned jobs go before the in-progress ones.
	require.Len(t, content.Jobs, 2)
	assert.Equal(t, "9bc44b23a733bee2-T-tenant-a-S2-L0", content.Jobs[0].Name)
	assert.Equal(t, jobStatusUnassigned, content.Jobs[0].Status)
	assert.Equal(t, "8bc44b23a733bee2-T-tenant-a-S1-L0", content.Jobs[1].Name)
	assert.Equal(t, jobStatusInProgress, content.Jobs[1].Status)
	assert.Zero(t, content.TruncatedJobs)

	require.Len(t, content.Queues, 1)
	assert.Equal(t, uint64(15), content.Queues[0].Blocks)
	assert.Zero(t, content.TruncatedQueues)

	// A tenant that has queued blocks but no jobs is not empty.
	content = buildCompactionTenantPageContent(testCompactionState(now), now, "tenant-c")
	assert.False(t, content.Empty())
	assert.Zero(t, content.TotalJobs)
	assert.Equal(t, uint64(7), content.QueuedBlocks)

	content = buildCompactionTenantPageContent(testCompactionState(now), now, "tenant-x")
	assert.True(t, content.Empty())
}

func TestBuildCompactionTenantPageContent_truncation(t *testing.T) {
	now := time.Now()
	state := &metastorev1.GetCompactionStateResponse{}
	for i := 0; i < maxCompactionJobRows+100; i++ {
		state.CompactionJobs = append(state.CompactionJobs, &metastorev1.CompactionJobDetails{
			Name:   "job",
			Tenant: "tenant-a",
		})
		// Jobs of the other tenants do not count towards the limit.
		state.CompactionJobs = append(state.CompactionJobs, &metastorev1.CompactionJobDetails{
			Name:   "job",
			Tenant: "tenant-b",
		})
	}
	for i := 0; i < maxCompactionQueueRows+50; i++ {
		state.CompactionQueues = append(state.CompactionQueues, &metastorev1.CompactionQueueDetails{
			Tenant: "tenant-a",
			Shard:  uint32(i),
			Blocks: 2,
		})
	}

	content := buildCompactionTenantPageContent(state, now, "tenant-a")
	assert.Len(t, content.Jobs, maxCompactionJobRows)
	assert.Equal(t, 100, content.TruncatedJobs)
	assert.Equal(t, maxCompactionJobRows+100, content.TotalJobs)
	assert.Len(t, content.Queues, maxCompactionQueueRows)
	assert.Equal(t, 50, content.TruncatedQueues)
	// The blocks counter covers the queues that are not displayed.
	assert.Equal(t, uint64(2*(maxCompactionQueueRows+50)), content.QueuedBlocks)
}

func TestCompactionHandler_tenantTruncation(t *testing.T) {
	state := &metastorev1.GetCompactionStateResponse{}
	for i := 0; i < maxCompactionJobRows+5; i++ {
		state.CompactionJobs = append(state.CompactionJobs, &metastorev1.CompactionJobDetails{
			Name:   "job",
			Tenant: "tenant-a",
		})
	}
	for i := 0; i < maxCompactionQueueRows+7; i++ {
		state.CompactionQueues = append(state.CompactionQueues, &metastorev1.CompactionQueueDetails{
			Tenant: "tenant-a",
			Shard:  uint32(i),
		})
	}
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(state, nil).
		Once()

	a := &Admin{compactionClient: client}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metastore-compaction?tenant=tenant-a", nil)
	a.CompactionHandler().ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "Showing the first 1000 jobs in the scheduling order; 5 more")
	assert.Contains(t, body, "7 more queues are not displayed")
}

func TestTenantPath(t *testing.T) {
	assert.Equal(t, "/metastore-compaction?tenant=tenant-a", tenantPath("tenant-a", ""))
	assert.Equal(t, "/metastore-compaction?tenant=tenant-a", tenantPath("tenant-a", tenantSortAttention))
	assert.Equal(t, "/metastore-compaction?tenant=tenant-a&sort=jobs", tenantPath("tenant-a", tenantSortJobs))
	assert.Equal(t, "/metastore-compaction?tenant=a%2Fb+c%26d", tenantPath("a/b c&d", ""))
}

func TestBlocksPath(t *testing.T) {
	from := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	to := from.Add(30 * time.Minute)

	assert.Equal(t, "/ops/object-store/tenants/tenant-a/blocks", tenantBlocksPath("tenant-a"))
	assert.Equal(t, "/ops/object-store/tenants/a%2Fb/blocks", tenantBlocksPath("a/b"))
	// The window is padded on both ends.
	assert.Equal(t,
		"/ops/object-store/tenants/tenant-a/blocks"+
			"?queryFrom=2026-08-28T11%3A00%3A00Z&queryTo=2026-08-28T13%3A30%3A00Z",
		blocksPath("tenant-a", from, to))

	// Level 0 entities carry no tenant and are not addressable in the listing.
	assert.Empty(t, tenantBlocksPath(""))
	assert.Empty(t, blocksPath("", from, to))
}

func TestBuildCompactionPageContent_blockLinks(t *testing.T) {
	now := time.Now()
	state := testCompactionState(now)
	content := buildCompactionPageContent(state, now, "", "")

	// Attention rows link to the blocks of their own tenant.
	require.Len(t, content.Attention, 2)
	for _, row := range content.Attention {
		assert.Contains(t, row.BlocksPath, "/ops/object-store/tenants/tenant-b/blocks?queryFrom=")
	}

	tenantContent := buildCompactionTenantPageContent(state, now, "tenant-a")
	assert.Equal(t, "/ops/object-store/tenants/tenant-a/blocks", tenantContent.BlocksPath)
	require.Len(t, tenantContent.Jobs, 2)
	for _, row := range tenantContent.Jobs {
		assert.Contains(t, row.BlocksPath, "/ops/object-store/tenants/tenant-a/blocks?queryFrom=")
	}
	// The queue window is derived from the oldest and the newest entries.
	require.Len(t, tenantContent.Queues, 1)
	assert.Equal(t,
		blocksPath("tenant-a",
			time.Unix(0, state.CompactionQueues[0].OldestBlockAt),
			time.Unix(0, state.CompactionQueues[0].NewestBlockAt)),
		tenantContent.Queues[0].BlocksPath)
}

func TestBuildCompactionPageContent_blockLinksWithoutTenant(t *testing.T) {
	now := time.Now()
	// Level 0 jobs and queues operate on multi-tenant segments
	// and carry no tenant of their own.
	state := &metastorev1.GetCompactionStateResponse{
		JobMaxFailures: 3,
		CompactionJobs: []*metastorev1.CompactionJobDetails{{
			Name:           "segment-job",
			Status:         metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
			LeaseExpiresAt: now.Add(-time.Minute).UnixNano(),
			AddedAt:        now.Add(-time.Hour).UnixNano(),
		}},
		CompactionQueues: []*metastorev1.CompactionQueueDetails{{
			Blocks:        4,
			OldestBlockAt: now.Add(-time.Minute).UnixNano(),
			NewestBlockAt: now.UnixNano(),
		}},
	}

	content := buildCompactionPageContent(state, now, "", "")
	require.Len(t, content.Attention, 1)
	// The time-windowed listing link needs a tenant to query by, so there is
	// none; the drill-down instead goes to the segment group.
	assert.Empty(t, content.Attention[0].BlocksPath)
	assert.Equal(t, "/metastore-compaction?segments=1", content.Attention[0].TenantPath)

	tenantContent := buildCompactionTenantPageContent(state, now, "")
	assert.True(t, tenantContent.Segments)
	assert.Empty(t, tenantContent.BlocksPath)

	// The template renders the counts as plain text: no empty links.
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(state, nil).
		Once()
	w := httptest.NewRecorder()
	a := &Admin{compactionClient: client}
	a.CompactionHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metastore-compaction", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "/ops/object-store/")
}

func TestCompactionHandler_blockLinks(t *testing.T) {
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(testCompactionState(time.Now()), nil).
		Once()

	a := &Admin{compactionClient: client}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metastore-compaction?tenant=tenant-a", nil)
	a.CompactionHandler().ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `href="/ops/object-store/tenants/tenant-a/blocks"`)
	assert.Contains(t, body, `href="/ops/object-store/tenants/tenant-a/blocks?queryFrom=`)
}

// The request filter is what keeps the drill-down from pulling the source
// blocks of the entire cluster, so assert exactly what the handler asks for.
func TestCompactionHandler_request(t *testing.T) {
	for _, test := range []struct {
		name               string
		url                string
		expectTenant       *string
		expectSourceBlocks bool
	}{
		{
			name: "overview asks for no filter and no source blocks",
			url:  "/metastore-compaction",
		},
		{
			name:               "tenant drill-down filters and asks for source blocks",
			url:                "/metastore-compaction?tenant=tenant-a",
			expectTenant:       ptr("tenant-a"),
			expectSourceBlocks: true,
		},
		{
			name:               "segment drill-down selects the empty tenant",
			url:                "/metastore-compaction?segments=1",
			expectTenant:       ptr(""),
			expectSourceBlocks: true,
		},
		{
			name: "an empty tenant parameter is the overview, not the segments",
			url:  "/metastore-compaction?tenant=",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got *metastorev1.GetCompactionStateRequest
			client := mockmetastorev1.NewMockCompactionServiceClient(t)
			client.EXPECT().
				GetCompactionState(mock.Anything, mock.Anything).
				Run(func(_ context.Context, in *metastorev1.GetCompactionStateRequest, _ ...grpc.CallOption) {
					got = in
				}).
				Return(testCompactionState(time.Now()), nil).
				Once()

			a := &Admin{compactionClient: client}
			w := httptest.NewRecorder()
			a.CompactionHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, test.url, nil))
			require.Equal(t, http.StatusOK, w.Code)

			require.NotNil(t, got)
			if test.expectTenant == nil {
				assert.Nil(t, got.Tenant, "the tenant filter must be absent, not empty")
			} else {
				require.NotNil(t, got.Tenant)
				assert.Equal(t, *test.expectTenant, *got.Tenant)
			}
			assert.Equal(t, test.expectSourceBlocks, got.IncludeSourceBlocks)
		})
	}
}

func TestCompactionHandler_sourceBlockLinks(t *testing.T) {
	now := time.Now()
	state := testCompactionState(now)
	state.CompactionJobs[0].SourceBlockIds = []string{"01M14VWWJJEDW4TPDEFH5H5XWJ", "01M14VWWJJEDW4TPDEFH5H5XWK"}

	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(state, nil).
		Once()

	a := &Admin{compactionClient: client}
	w := httptest.NewRecorder()
	a.CompactionHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metastore-compaction?tenant=tenant-a", nil))

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body,
		`href="/ops/object-store/tenants/tenant-a/blocks/01M14VWWJJEDW4TPDEFH5H5XWJ`+
			`?block_tenant=tenant-a&amp;shard=1"`)
	assert.Contains(t, body, "01M14VWWJJEDW4TPDEFH5H5XWK")
}

// A server that does not report the source block identifiers must leave the
// page on the windowed fallback rather than showing an empty block list.
func TestCompactionHandler_sourceBlockLinksAbsent(t *testing.T) {
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(testCompactionState(time.Now()), nil).
		Once()

	a := &Admin{compactionClient: client}
	w := httptest.NewRecorder()
	a.CompactionHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metastore-compaction?tenant=tenant-a", nil))

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.NotContains(t, body, "/blocks/")
	assert.Contains(t, body, `href="/ops/object-store/tenants/tenant-a/blocks?queryFrom=`)
}

func TestBlockPath(t *testing.T) {
	assert.Equal(t,
		"/ops/object-store/tenants/tenant-a/blocks/block-1?block_tenant=tenant-a&shard=7",
		blockPath("tenant-a", 7, "block-1"))
	// Segments belong to no tenant: the path names the anonymous tenant,
	// while the block tenant, which resolves the block, stays empty.
	assert.Equal(t,
		"/ops/object-store/tenants/anonymous/blocks/block-1?block_tenant=&shard=0",
		blockPath("", 0, "block-1"))
}

func TestCompactionHandler_segments(t *testing.T) {
	now := time.Now()
	state := &metastorev1.GetCompactionStateResponse{
		JobMaxFailures: 3,
		CompactionJobs: []*metastorev1.CompactionJobDetails{{
			Name:           "5f0a-T-S38-L0",
			Shard:          38,
			AddedAt:        now.Add(-time.Minute).UnixNano(),
			SourceBlocks:   1,
			SourceBlockIds: []string{"01M14VWWJJEDW4TPDEFH5H5XWJ"},
		}},
	}
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(state, nil).
		Once()

	a := &Admin{compactionClient: client}
	w := httptest.NewRecorder()
	a.CompactionHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metastore-compaction?segments=1", nil))

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "Multi-tenant segments")
	// The anonymous tenant stands in for the segments, and the page says why.
	assert.Contains(t, body,
		`href="/ops/object-store/tenants/anonymous/blocks/01M14VWWJJEDW4TPDEFH5H5XWJ`+
			`?block_tenant=&amp;shard=38"`)
	assert.Contains(t, body, "<code>anonymous</code>")
}

func ptr[T any](v T) *T { return &v }

func TestMatchesTenantFilter(t *testing.T) {
	assert.True(t, matchesTenantFilter("tenant-a", ""))
	assert.True(t, matchesTenantFilter("", ""))
	assert.True(t, matchesTenantFilter("tenant-a", "ANT-A"))
	assert.True(t, matchesTenantFilter("TENANT-A", "ant-a"))
	assert.False(t, matchesTenantFilter("tenant-a", "tenant-b"))
	assert.False(t, matchesTenantFilter("", "tenant"))
}

func TestFormatDuration(t *testing.T) {
	assert.Equal(t, "<1s", formatDuration(500*time.Millisecond))
	assert.Equal(t, "<1s", formatDuration(-time.Second))
	assert.Equal(t, "4s", formatDuration(4031*time.Millisecond))
	assert.Equal(t, "6s", formatDuration(5981*time.Millisecond))
	assert.Equal(t, "42s", formatDuration(42*time.Second))
	// Rounds up across the unit boundary.
	assert.Equal(t, "1m", formatDuration(59700*time.Millisecond))
	assert.Equal(t, "1m", formatDuration(time.Minute))
	assert.Equal(t, "1m30s", formatDuration(90*time.Second))
	assert.Equal(t, "2h", formatDuration(2*time.Hour))
	assert.Equal(t, "2h3m", formatDuration(2*time.Hour+3*time.Minute))
	assert.Equal(t, "1d12h", formatDuration(36*time.Hour))
	assert.Equal(t, "290d", formatDuration(290*24*time.Hour))
	assert.Equal(t, "290d2h", formatDuration(290*24*time.Hour+150*time.Minute))
}

func TestFormatLease(t *testing.T) {
	now := time.Now()
	assert.Equal(t, "-", formatLease(&metastorev1.CompactionJobDetails{}, now))
	assert.Equal(t, "expires in 10s", formatLease(&metastorev1.CompactionJobDetails{
		LeaseExpiresAt: now.Add(10 * time.Second).UnixNano(),
	}, now))
	assert.Equal(t, "expired 10s ago", formatLease(&metastorev1.CompactionJobDetails{
		LeaseExpiresAt: now.Add(-10 * time.Second).UnixNano(),
	}, now))
}
