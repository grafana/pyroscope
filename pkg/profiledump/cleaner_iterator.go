package profiledump

import (
	"context"
	"time"
)

// cleanupIterator pauses one provider listing between calls to next.
// Callers must serialize next and stop. The worker owns err until done closes.
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
		// The watchdog must not run while callbacks are paused between passes.
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
		// Return nil on cancellation so S3 can drain its listing producer.
		it.err = bucket.Iter(timedCtx, prefix, func(key string) error {
			watchdog.Stop()
			if err := timedCtx.Err(); err != nil {
				return nil
			}
			select {
			case <-timedCtx.Done():
				return nil
			case it.entries <- key:
			}
			awaitRequest()
			return nil
		})
		watchdog.Stop()
		if cause := context.Cause(timedCtx); cause != nil {
			it.err = cause
		}
	}()
	return it
}

// A canceled next leaves its pending fetch for the following pass.
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
