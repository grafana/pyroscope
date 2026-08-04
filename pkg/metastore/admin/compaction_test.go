package admin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

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
	assert.Contains(t, body, "8bc44b23a733bee2-T-tenant-a-S1-L0")
	assert.Contains(t, body, "tenant-b")
	assert.Contains(t, body, "worker-1")
	assert.Contains(t, body, "in progress")
	assert.Contains(t, body, "unassigned")
	assert.Contains(t, body, "lease expired")
	assert.Contains(t, body, "failed")
	assert.Contains(t, body, "15s")
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
	content := buildCompactionPageContent(testCompactionState(now), now)

	assert.Equal(t, 4, content.TotalJobs)
	assert.Equal(t, 1, content.Unassigned)
	assert.Equal(t, 1, content.InProgress)
	assert.Equal(t, 1, content.LeaseExpired)
	assert.Equal(t, 1, content.Failed)
	assert.Equal(t, uint64(15), content.QueuedBlocks)

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

	// Jobs are ordered by level and scheduling priority:
	// unassigned jobs go before the in-progress ones.
	require.Len(t, content.Jobs, 4)
	assert.Equal(t, "9bc44b23a733bee2-T-tenant-a-S2-L0", content.Jobs[0].Name)
	assert.Equal(t, jobStatusUnassigned, content.Jobs[0].Status)
	assert.Equal(t, jobStatusInProgress, content.Jobs[1].Status)
	assert.Equal(t, jobStatusLeaseExpired, content.Jobs[2].Status)
	assert.Equal(t, jobStatusFailed, content.Jobs[3].Status)
	assert.Zero(t, content.TruncatedJobs)

	require.Len(t, content.Queues, 1)
	assert.Equal(t, uint64(15), content.Queues[0].Blocks)
}

func TestBuildCompactionPageContent_truncation(t *testing.T) {
	now := time.Now()
	state := &metastorev1.GetCompactionStateResponse{}
	for i := 0; i < maxCompactionJobRows+100; i++ {
		state.CompactionJobs = append(state.CompactionJobs, &metastorev1.CompactionJobDetails{
			Name: "job",
		})
	}
	for i := 0; i < maxCompactionQueueRows+50; i++ {
		state.CompactionQueues = append(state.CompactionQueues, &metastorev1.CompactionQueueDetails{
			Tenant: "tenant",
			Shard:  uint32(i),
			Blocks: 2,
		})
	}
	content := buildCompactionPageContent(state, now)
	assert.Len(t, content.Jobs, maxCompactionJobRows)
	assert.Equal(t, 100, content.TruncatedJobs)
	assert.Equal(t, maxCompactionJobRows+100, content.TotalJobs)
	assert.Len(t, content.Queues, maxCompactionQueueRows)
	assert.Equal(t, 50, content.TruncatedQueues)
	// The blocks counter covers the queues that are not displayed.
	assert.Equal(t, uint64(2*(maxCompactionQueueRows+50)), content.QueuedBlocks)
}

// The truncation alerts are only rendered when the limits are exceeded:
// make sure the template branches execute.
func TestCompactionHandler_truncation(t *testing.T) {
	state := &metastorev1.GetCompactionStateResponse{}
	for i := 0; i < maxCompactionJobRows+5; i++ {
		state.CompactionJobs = append(state.CompactionJobs, &metastorev1.CompactionJobDetails{Name: "job"})
	}
	for i := 0; i < maxCompactionQueueRows+7; i++ {
		state.CompactionQueues = append(state.CompactionQueues, &metastorev1.CompactionQueueDetails{Shard: uint32(i)})
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
	assert.Contains(t, body, "Showing the first 1000 jobs in the scheduling order; 5 more")
	assert.Contains(t, body, "7 more queues are not displayed")
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
