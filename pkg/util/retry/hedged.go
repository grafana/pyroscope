package retry

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Hedged executes Call with a speculative retry after trigger fires
// if it has not returned earlier.
//
// By default, if one of the attempts fails, another one is not canceled.
// In this case the speculative attempt will not start until the trigger fires.
// For more granular control, use FailFast.
type Hedged[T any] struct {
	// The function must be thread-safe because multiple instances may be running
	// concurrently. The function must return as soon as possible after context
	// cancellation, otherwise the speculation makes no sense.
	//
	// The function argument indicates whether this is a speculative retry attempt.
	Call    Call[T]
	Trigger <-chan time.Time

	// FailFast specifies how a failure is handled. If it is set to true:
	//  - the result received first is returned, regardless of anything.
	//  - if Call fails before the trigger fires, it won't be retried.
	FailFast bool

	// Cleanup is called on the result of a losing attempt when it succeeded
	// but another attempt already won. Use this to release resources (e.g.,
	// close an io.ReadCloser) that would otherwise be abandoned.
	Cleanup func(T)
}

type Call[T any] func(ctx context.Context, isRetry bool) (T, error)

func (s Hedged[T]) Do(ctx context.Context) (T, error) {
	// Each attempt gets its own independent cancellable context derived from
	// the parent. The winner cancels only the loser — never itself — so that
	// values tied to the winning context (e.g. an io.ReadCloser backed by an
	// HTTP connection) remain usable after Do returns. The winner's context
	// lives until the parent ctx is cancelled.
	ctx1, cancel1 := context.WithCancel(ctx)
	ctx2, cancel2 := context.WithCancel(ctx)

	var (
		ret    T
		err    error
		failed uint64

		wg sync.WaitGroup
		do sync.Once
		// decided is closed by the goroutine that stores the winning result.
		decided = make(chan struct{})
	)

	attempt := func(attemptCtx context.Context, cancelSelf, cancelOther context.CancelFunc, isRetry bool) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			attemptRet, attemptErr := s.Call(attemptCtx, isRetry)
			if attemptErr != nil && atomic.SwapUint64(&failed, 1) == 0 && !s.FailFast {
				// This attempt has failed, but not another one. If allowed,
				// we give another attempt a chance. Otherwise, if both ones
				// did fail, or it's not allowed to proceed after the first
				// failure, we store the result with error and cancel any
				// ongoing attempt.
				return
			}
			stored := false
			do.Do(func() {
				ret, err = attemptRet, attemptErr
				stored = true
				close(decided)
			})
			if stored {
				// We won: cancel the other attempt.
				// Do NOT cancel our own context — the caller may still be
				// reading from our result (e.g. io.ReadCloser).
				cancelOther()
			} else {
				// We lost: cancel our own context and release our result.
				cancelSelf()
				if attemptErr == nil && s.Cleanup != nil {
					s.Cleanup(attemptRet)
				}
			}
		}()
	}

	attempt(ctx1, cancel1, cancel2, false)
	select {
	case <-decided:
		// A winner was found before the trigger fired.
	case <-ctx.Done():
		// Caller cancelled: abort both attempts.
		cancel1()
		cancel2()
	case <-s.Trigger:
		attempt(ctx2, cancel2, cancel1, true)
		select {
		case <-decided:
		case <-ctx.Done():
			cancel1()
			cancel2()
		}
	}

	wg.Wait()
	return ret, err
}
