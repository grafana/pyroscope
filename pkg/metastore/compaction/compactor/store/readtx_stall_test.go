package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction"
)

// This experiment models the metastore under a compaction page scan: the FSM
// apply loop writes while a read transaction is held open for the duration of
// the scan (see fsm.Read). It measures what the writer pays for that.
//
// Two bbolt properties are at play, and they compound:
//
//   - Pages freed by a writer cannot be reused while an older read
//     transaction is open, so the file grows instead of being recycled.
//   - Growing past the mmap calls db.mmap, which takes an exclusive
//     mmaplock and therefore waits for every open read transaction.
func TestReadTxWriterStall(t *testing.T) {
	if os.Getenv("RUN_GROWTH_EXPERIMENT") == "" {
		t.Skip("set RUN_GROWTH_EXPERIMENT to run")
	}

	const (
		initial      = 100_000
		writeRounds  = 300
		writeBatch   = 200
		readTxHeldMs = 200
	)

	entry := func(i int) compaction.BlockEntry {
		return compaction.BlockEntry{
			Index:      uint64(i),
			AppendedAt: int64(i),
			ID:         fmt.Sprintf("01M14VWWJJEDW4TPDEFH%06d", i),
			Tenant:     "tenant-0001",
			Shard:      uint32(i % 64),
			Level:      1,
		}
	}

	run := func(t *testing.T, holdReadTx bool) (stalls []time.Duration, size int64) {
		// A small initial mmap so the file crosses the mmap boundary during
		// the run, as a growing metastore database does.
		db, err := bbolt.Open(filepath.Join(t.TempDir(), "boltdb"), 0644, &bbolt.Options{
			NoSync:          true,
			NoGrowSync:      true,
			NoFreelistSync:  true,
			FreelistType:    bbolt.FreelistMapType,
			InitialMmapSize: 1 << 20,
		})
		require.NoError(t, err)

		s := NewBlockQueueStore()
		require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
			if err := s.CreateBuckets(tx); err != nil {
				return err
			}
			for i := 0; i < initial; i++ {
				if err := s.StoreEntry(tx, entry(i)); err != nil {
					return err
				}
			}
			return nil
		}))

		var wg sync.WaitGroup
		if holdReadTx {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Let the writer get going first, then hold a read
				// transaction the way a page scan does.
				time.Sleep(20 * time.Millisecond)
				tx, err := db.Begin(false)
				require.NoError(t, err)
				time.Sleep(readTxHeldMs * time.Millisecond)
				require.NoError(t, tx.Rollback())
			}()
		}

		// The writer: the raft apply loop, retiring compacted blocks and
		// queueing new ones, one transaction at a time.
		stalls = make([]time.Duration, 0, writeRounds)
		for r := 0; r < writeRounds; r++ {
			start := time.Now()
			require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
				for i := 0; i < writeBatch; i++ {
					e := entry(r*writeBatch + i)
					if err := s.DeleteEntry(tx, e.Index, e.ID); err != nil {
						return err
					}
					if err := s.StoreEntry(tx, entry(initial+r*writeBatch+i)); err != nil {
						return err
					}
				}
				return nil
			}))
			stalls = append(stalls, time.Since(start))
			time.Sleep(time.Millisecond)
		}
		wg.Wait()

		info, err := os.Stat(db.Path())
		require.NoError(t, err)
		size = info.Size()
		require.NoError(t, db.Close())
		return stalls, size
	}

	report := func(name string, stalls []time.Duration, size int64) {
		sorted := make([]time.Duration, len(stalls))
		copy(sorted, stalls)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		var total time.Duration
		for _, d := range sorted {
			total += d
		}
		t.Logf("%s: commits=%d mean=%s p50=%s p99=%s max=%s db=%.1f MiB",
			name, len(sorted),
			total/time.Duration(len(sorted)),
			sorted[len(sorted)/2],
			sorted[len(sorted)*99/100],
			sorted[len(sorted)-1],
			float64(size)/(1<<20))
	}

	baseline, baseSize := run(t, false)
	withRead, readSize := run(t, true)
	report("no concurrent read tx", baseline, baseSize)
	report("read tx held 200ms   ", withRead, readSize)
}
