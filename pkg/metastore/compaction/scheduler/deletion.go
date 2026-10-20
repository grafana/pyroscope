package scheduler

import (
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

// BusyBlocks includes queued and leased jobs. Deletion must not race either.
// Exhausting MaxFailures only prevents further assignments: the final retry can
// still renew its lease and complete. Keep its sources reserved until the job is
// removed from the scheduler, which also fences off subsequent status reports.
func (sc *Scheduler) BusyBlocks(tx *bbolt.Tx) ([]*metastorev1.BlockList, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	it := sc.store.ListEntries(tx)
	defer func() { _ = it.Close() }()
	var result []*metastorev1.BlockList
	for it.Next() {
		state := it.At()
		p, err := sc.store.GetJobPlan(tx, state.Name)
		if err != nil {
			return nil, err
		}
		if p != nil {
			result = append(result, &metastorev1.BlockList{Tenant: p.Tenant, Shard: p.Shard, Blocks: p.SourceBlocks})
		}
	}
	return result, it.Err()
}
