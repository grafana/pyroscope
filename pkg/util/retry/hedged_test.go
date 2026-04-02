package retry

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func Test_Hedging(t *testing.T) {
	e1 := errors.New("e1")
	e2 := errors.New("e2")

	// hedgeDelay is the trigger delay. Attempts with returnAfter=0 return
	// immediately (before the trigger fires). Attempts with
	// returnAfter=2*hedgeDelay return after the trigger has fired.
	const hedgeDelay = time.Second

	type attempt struct {
		returnAfter time.Duration // 0 = immediate
		err         error
	}

	type testCase struct {
		description string
		failFast    bool
		attempts    [2]attempt
		expectRetry bool
		expectError error
	}

	testCases := []testCase{
		{
			description: "Attempt fails before retry and FailFast",
			failFast:    true,
			attempts:    [2]attempt{{err: e1}, {}},
			expectRetry: false,
			expectError: e1,
		},
		{
			description: "Attempt fails before retry and not FailFast",
			failFast:    false,
			attempts:    [2]attempt{{err: e1}, {}},
			expectRetry: true,
			expectError: nil,
		},
		{
			description: "Attempt fails before retry and retry fails and FailFast",
			failFast:    true,
			attempts:    [2]attempt{{err: e1}, {err: e2}},
			expectRetry: false,
			expectError: e1,
		},
		{
			description: "Attempt fails before retry and retry fails and not FailFast",
			failFast:    false,
			attempts:    [2]attempt{{err: e1}, {err: e2}},
			expectRetry: true,
			expectError: e2,
		},
		{
			description: "Attempt fails after retry and FailFast",
			failFast:    true,
			attempts:    [2]attempt{{returnAfter: hedgeDelay * 2, err: e1}, {returnAfter: hedgeDelay * 2}},
			expectRetry: false,
			expectError: e1,
		},
		{
			description: "Attempt fails after retry and not FailFast",
			failFast:    false,
			attempts:    [2]attempt{{returnAfter: hedgeDelay * 2, err: e1}, {returnAfter: hedgeDelay * 2}},
			expectRetry: true,
			expectError: nil,
		},
		{
			description: "Attempt fails after retry and retry fails and FailFast",
			failFast:    true,
			attempts:    [2]attempt{{returnAfter: hedgeDelay * 2, err: e1}, {returnAfter: hedgeDelay * 2, err: e2}},
			expectRetry: false,
			expectError: e1,
		},
		{
			description: "Attempt fails after retry and retry fails and not FailFast",
			failFast:    false,
			attempts:    [2]attempt{{returnAfter: hedgeDelay * 2, err: e1}, {returnAfter: hedgeDelay * 2, err: e2}},
			expectRetry: true,
			expectError: e2,
		},
	}

	for _, c := range testCases {
		t.Run(c.description, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				a := Hedged[bool]{
					Trigger:  time.After(hedgeDelay),
					FailFast: c.failFast,
					Call: func(ctx context.Context, isRetry bool) (bool, error) {
						spec := c.attempts[0]
						if isRetry {
							spec = c.attempts[1]
						}
						if spec.returnAfter > 0 {
							select {
							case <-time.After(spec.returnAfter):
							case <-ctx.Done():
							}
						}
						return isRetry, spec.err
					},
				}

				done := make(chan struct{})
				var gotResult bool
				var gotErr error
				go func() {
					defer close(done)
					gotResult, gotErr = a.Do(context.Background())
				}()

				synctest.Wait()
				time.Sleep(hedgeDelay) // fire trigger
				synctest.Wait()
				time.Sleep(hedgeDelay * 2) // let slow attempts complete
				synctest.Wait()
				<-done

				if gotResult != c.expectRetry {
					t.Fatalf("expected isRetry=%v, got %v", c.expectRetry, gotResult)
				}
				if !errors.Is(gotErr, c.expectError) {
					t.Fatalf("expected error %v, got %v", c.expectError, gotErr)
				}
			})
		})
	}
}
