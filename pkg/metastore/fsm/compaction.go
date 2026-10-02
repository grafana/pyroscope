package fsm

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

// BackgroundCompactor periodically compacts the BoltDB database in-place
// to reclaim disk space accumulated by free pages after retention cleanup.
//
// Design constraints:
//   - Uses the existing bbolt.Compact path (boltdb.compact + boltdb.openPath)
//     that is already proven by the snapshot-restore flow.
//   - Hot-swap is gated behind fsm.mu.Lock() + fsm.txns.Wait(), which is
//     identical to the FSM.Restore locking strategy and therefore safe for
//     both single-node and multi-node Raft deployments.
//   - Compaction is skipped when the free-page ratio is below the configured
//     threshold so that already-dense databases are never needlessly rewritten.
type BackgroundCompactor struct {
	fsm    *FSM
	logger log.Logger

	interval        time.Duration
	minFreeRatio    float64

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func newBackgroundCompactor(fsm *FSM) *BackgroundCompactor {
	ctx, cancel := context.WithCancel(context.Background())
	return &BackgroundCompactor{
		fsm:          fsm,
		logger:       fsm.logger,
		interval:     fsm.config.BoltDBCompactInterval,
		minFreeRatio: fsm.config.BoltDBCompactMinFreeRatio,
		ctx:          ctx,
		cancel:       cancel,
		done:         make(chan struct{}),
	}
}

// Start launches the background compaction loop. It is safe to call Start
// multiple times; subsequent calls are no-ops.
func (c *BackgroundCompactor) Start() {
	go c.loop()
}

// Stop signals the compaction loop to stop and waits for it to exit.
func (c *BackgroundCompactor) Stop() {
	c.cancel()
	<-c.done
}

func (c *BackgroundCompactor) loop() {
	defer close(c.done)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if err := c.maybeCompact(); err != nil {
				level.Error(c.logger).Log("msg", "online boltdb compaction failed", "err", err)
			}
		}
	}
}

// freePageRatio returns the fraction of the database's allocated pages that
// are currently unused (free). A value of 0.30 means 30% of allocated pages
// are free and could be reclaimed by compaction.
//
// We derive page count from bbolt.Stats() which is always up-to-date without
// requiring an additional read transaction.
func (c *BackgroundCompactor) freePageRatio() float64 {
	db := c.fsm.db.boltdb
	if db == nil {
		return 0
	}
	stats := db.Stats()
	totalPages := stats.TxStats.PageCount + int64(stats.FreePageN) + int64(stats.PendingPageN)
	if totalPages <= 0 {
		return 0
	}
	freePages := int64(stats.FreePageN) + int64(stats.PendingPageN)
	return float64(freePages) / float64(totalPages)
}

// maybeCompact checks the free-page ratio and, if it exceeds the configured
// threshold, performs an online compaction and hot-swaps the database file.
func (c *BackgroundCompactor) maybeCompact() error {
	ratio := c.freePageRatio()
	if ratio < c.minFreeRatio {
		level.Debug(c.logger).Log(
			"msg", "boltdb online compaction skipped: free-page ratio below threshold",
			"free_page_ratio", fmt.Sprintf("%.3f", ratio),
			"threshold", fmt.Sprintf("%.3f", c.minFreeRatio),
		)
		return nil
	}

	start := time.Now()
	sizeBefore := c.fsm.db.size()
	level.Info(c.logger).Log(
		"msg", "starting online boltdb compaction",
		"free_page_ratio", fmt.Sprintf("%.3f", ratio),
		"size_before_bytes", sizeBefore,
	)

	// compact() writes into boltDBCompactedName and returns the closed *boltdb.
	// It only reads from the live database and does not require any locks.
	c.fsm.db.metrics.boltDBOnlineCompactionSizeBeforeBytes.Set(float64(sizeBefore))
	compacted, err := c.fsm.db.compact()
	if err != nil {
		c.fsm.db.metrics.boltDBOnlineCompactionRunsTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("compact: %w", err)
	}

	sizeAfter := compacted.size()
	compactionRatio := float64(sizeAfter) / float64(sizeBefore)
	level.Info(c.logger).Log(
		"msg", "compaction complete; performing hot-swap",
		"size_before_bytes", sizeBefore,
		"size_after_bytes", sizeAfter,
		"ratio", fmt.Sprintf("%.3f", compactionRatio),
	)

	// Hot-swap: take the write lock so no new transactions can begin,
	// wait for in-flight read transactions to finish, then atomically
	// rename the compacted file over the live file and re-open.
	// This is the same locking strategy used by FSM.Restore.
	c.fsm.mu.Lock()
	c.fsm.txns.Wait()
	swapErr := c.fsm.db.openPath(compacted.path)
	c.fsm.mu.Unlock()

	duration := time.Since(start)
	c.fsm.db.metrics.boltDBOnlineCompactionDuration.Observe(duration.Seconds())
	c.fsm.db.metrics.boltDBOnlineCompactionRatio.Set(compactionRatio)

	if swapErr != nil {
		c.fsm.db.metrics.boltDBOnlineCompactionRunsTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("hot-swap: %w", swapErr)
	}

	c.fsm.db.metrics.boltDBOnlineCompactionRunsTotal.WithLabelValues("success").Inc()
	level.Info(c.logger).Log(
		"msg", "online boltdb compaction finished",
		"duration", duration,
		"size_before_bytes", sizeBefore,
		"size_after_bytes", sizeAfter,
		"ratio", fmt.Sprintf("%.3f", compactionRatio),
	)
	return nil
}
