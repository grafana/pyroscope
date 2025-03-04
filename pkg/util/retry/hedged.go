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

	// Throttler executes call retries. Optional.
	Throttler
}

type Throttler interface {
	Run(func())
}

type Call[T any] func(ctx context.Context, isRetry bool) (T, error)

func (s Hedged[T]) Do(ctx context.Context) (T, error) {
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		ret    T
		err    error
		failed uint64

		wg sync.WaitGroup
		do sync.Once
	)

	attempt := func(isRetry bool) {
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
			// If there is an ongoing attempt, it will be cancelled,
			// because we already got the result.
			cancel()
			do.Do(func() {
				ret, err = attemptRet, attemptErr
			})
		}()
	}

	attempt(false)
	select {
	case <-attemptCtx.Done():
		// Call has returned, or caller cancelled the request.
	case <-s.Trigger:
		if s.Throttler != nil {
			s.Throttler.Run(func() {
				attempt(true)
			})
		} else {
			attempt(true)
		}
	}

	wg.Wait()
	return ret, err
}
