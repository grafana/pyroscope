package compactionworker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func shutdownTestJob() *compactionJob {
	ctx, cancel := context.WithCancel(context.Background())
	return &compactionJob{ctx: ctx, cancel: cancel}
}

func TestShutdownCancelsPendingButNotExecuting(t *testing.T) {
	w := &Worker{}
	running := shutdownTestJob()
	defer running.cancel()
	waiting := shutdownTestJob()
	defer waiting.cancel()
	require.True(t, w.prepareJob(running))
	require.True(t, w.beginExecution(running))
	require.True(t, w.prepareJob(waiting))
	budget := newByteBudget(100)
	release, err := budget.acquire(running.ctx, 100, nil)
	require.NoError(t, err)
	defer release()
	paused := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := budget.acquire(waiting.ctx, 100, func() { close(paused) })
		done <- err
	}()
	<-paused
	w.stopAdmission()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel budget waiter")
	}
	require.NoError(t, running.ctx.Err())
	require.False(t, w.beginExecution(waiting))
	queued := shutdownTestJob()
	defer queued.cancel()
	require.False(t, w.prepareJob(queued))
	require.ErrorIs(t, queued.ctx.Err(), context.Canceled)
	budget.mu.Lock()
	require.Equal(t, int64(100), budget.inUse)
	require.Zero(t, budget.waiting)
	require.Zero(t, budget.waiters.Len())
	budget.mu.Unlock()
}

func TestShutdownBeforeExecutionReleasesReservation(t *testing.T) {
	for _, limit := range []int64{0, 100} {
		w := &Worker{}
		job := shutdownTestJob()
		require.True(t, w.prepareJob(job))
		budget := newByteBudget(limit)
		release, err := budget.acquire(job.ctx, 100, nil)
		require.NoError(t, err)
		w.stopAdmission()
		require.False(t, w.beginExecution(job))
		release()
		require.Zero(t, budget.inUse)
		job.cancel()
	}
}

func TestShutdownSkipsQueuedJob(t *testing.T) {
	w := &Worker{}
	w.stopAdmission()
	job := shutdownTestJob()
	defer job.cancel()
	// No client, metrics or compactFn: queued work must return before using any.
	w.runCompaction(job)
	require.ErrorIs(t, job.ctx.Err(), context.Canceled)
	require.Nil(t, job.compacted)
}

func TestShutdownAdmissionRace(t *testing.T) {
	for i := 0; i < 100; i++ {
		w := &Worker{}
		job := shutdownTestJob()
		require.True(t, w.prepareJob(job))
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); w.stopAdmission() }()
		executing := w.beginExecution(job)
		wg.Wait()
		if executing {
			require.NoError(t, job.ctx.Err(), "admitted execution must not be canceled by shutdown")
		} else {
			require.ErrorIs(t, job.ctx.Err(), context.Canceled)
		}
		job.cancel()
	}
}
