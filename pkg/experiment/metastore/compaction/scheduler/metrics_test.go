package scheduler

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
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
