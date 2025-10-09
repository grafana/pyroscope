package scheduler

import (
	"bytes"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/grafana/dskit/multierror"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/metastore/compaction/scheduler/store"
	"github.com/grafana/pyroscope/pkg/test"
)

func TestCollectorRegistration(t *testing.T) {
	reg := prometheus.NewRegistry()
	config := Config{
		MaxFailures:   5,
		LeaseDuration: 15 * time.Second,
	}

	for i := 0; i < 2; i++ {
		sc := NewScheduler(config, nil, reg)
		sc.queue.put(&raft_log.CompactionJobState{Name: "a"})
		sc.queue.put(&raft_log.CompactionJobState{
			Name: "b", CompactionLevel: 1, Token: 1,
			Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS,
		})
		sc.queue.delete("a")
	}
}

func TestCollectorCollect(t *testing.T) {
	reg := prometheus.NewRegistry()
	config := Config{
		MaxFailures:   5,
		LeaseDuration: 15 * time.Second,
	}

	sc := NewScheduler(config, nil, reg)
	sc.queue.put(&raft_log.CompactionJobState{
		Name: "a", CompactionLevel: 0, Token: 1,
		Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED,
	})
	sc.queue.put(&raft_log.CompactionJobState{
		Name: "b", CompactionLevel: 2, Token: 1,
		Status: metastorev1.CompactionJobStatus_COMPACTION_STATUS_UNSPECIFIED,
	})
	sc.queue.delete("a")

	buf, err := os.ReadFile("testdata/metrics.txt")
	require.NoError(t, err)
	assert.NoError(t, testutil.GatherAndCompare(reg, bytes.NewReader(buf)))
}

func TestCollectorCollectRace(t *testing.T) {
	reg := prometheus.NewRegistry()
	config := Config{
		MaxFailures:   5,
		LeaseDuration: 15 * time.Second,
	}

	var wg sync.WaitGroup
	wg.Add(2)

	db := test.BoltDB(t)
	go func() {
		defer wg.Done()
		s := store.NewJobStore()
		sc := NewScheduler(config, s, reg)
		for i := 0; i < 100; i++ {
			require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
				var merr multierror.MultiError
				merr.Add(s.CreateBuckets(tx))
				merr.Add(sc.UpdateSchedule(tx, &raft_log.CompactionPlanUpdate{
					NewJobs: []*raft_log.NewCompactionJob{{
						State: &raft_log.CompactionJobState{Name: "a", CompactionLevel: 0, Token: 1},
						Plan:  &raft_log.CompactionJobPlan{Name: "a", CompactionLevel: 0},
					}},
				}))
				assigned, err := sc.NewSchedule(tx, &raft.Log{}).AssignJob()
				require.NoError(t, err)
				require.NotNil(t, assigned)
				require.Equal(t, "a", assigned.State.Name)
				merr.Add(sc.UpdateSchedule(tx, &raft_log.CompactionPlanUpdate{
					CompletedJobs: []*raft_log.CompletedCompactionJob{
						{State: &raft_log.CompactionJobState{Name: "a", CompactionLevel: 0, Token: 1}},
					},
				}))
				return merr.Err()
			}))
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_, err := testutil.GatherAndCount(reg)
			require.NoError(t, err)
		}
	}()

	wg.Wait()
}
