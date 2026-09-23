package profiledump

import (
	"context"
	"time"
)

// cleanupIterator parks one Iter callback between entries and across passes.
// Only the service goroutine calls next/stop. The worker owns err until done.
type cleanupIterator struct {
	resume  chan struct{}
	entries chan string
	done    chan struct{}
	cancel  context.CancelFunc
	pending bool
	err     error
}

func newCleanupIterator(parent context.Context, bucket CleanupBucket, prefix string, timeout time.Duration) *cleanupIterator {
	ctx, cancel := context.WithCancel(parent)
	it := &cleanupIterator{resume: make(chan struct{}), entries: make(chan string), done: make(chan struct{}), cancel: cancel}
	go func() {
		defer close(it.done)
		defer cancel()
		// Time only provider fetches, excluding parked callbacks.
		var watchdog *time.Timer
		timedCtx, timedCancel := context.WithCancelCause(ctx)
		defer timedCancel(nil)
		awaitRequest := func() bool {
			select {
			case <-ctx.Done():
				return false
			case <-it.resume:
				watchdog = time.AfterFunc(timeout, func() { timedCancel(context.DeadlineExceeded) })
				return true
			}
		}
		if !awaitRequest() {
			it.err = ctx.Err()
			return
		}
		it.err = bucket.Iter(timedCtx, prefix, func(key string) error {
			watchdog.Stop()
			if err := timedCtx.Err(); err != nil {
				return err
			}
			select {
			case <-timedCtx.Done():
				return timedCtx.Err()
			case it.entries <- key:
			}
			if !awaitRequest() {
				return ctx.Err()
			}
			return timedCtx.Err()
		})
		watchdog.Stop()
		if cause := context.Cause(timedCtx); cause != nil {
			it.err = cause
		}
	}()
	return it
}

// next reports finished when Iter returns. Pending fetches survive pass expiry
// with their watchdog, so the next pass resumes without replay.
func (it *cleanupIterator) next(ctx context.Context) (key string, finished bool, err error) {
	if !it.pending {
		select {
		case <-it.done:
			return "", true, it.err
		case <-ctx.Done():
			return "", false, ctx.Err()
		case it.resume <- struct{}{}:
			it.pending = true
		}
	}
	select {
	case <-it.done:
		return "", true, it.err
	case <-ctx.Done():
		return "", false, ctx.Err()
	case key = <-it.entries:
		it.pending = false
		return key, false, nil
	}
}

func (it *cleanupIterator) stop() {
	it.cancel()
	<-it.done
}
