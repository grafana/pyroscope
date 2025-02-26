package retry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func Test_Hedging(t *testing.T) {
	delay := time.Millisecond * 100 // Might be flaky.
	ctx := context.Background()
	e1 := errors.New("e1")
	e2 := errors.New("e2")

	type attempt struct {
		duration time.Duration
		err      error
	}

	type testCase struct {
		description string
		failFast    bool
		attempts    [2]attempt
		expectRetry bool
		expectError error
	}

	createCall := func(c testCase) func(context.Context, bool) (bool, error) {
		return func(ctx context.Context, isRetry bool) (bool, error) {
			d := c.attempts[0].duration
			e := c.attempts[0].err
			if isRetry {
				d = c.attempts[1].duration
				e = c.attempts[1].err
			}
			select {
			case <-time.After(d):
			case <-ctx.Done():
			}
			return isRetry, e
		}
	}

	testCases := []testCase{
		{
			description: "Attempt fails before retry and FailFast",
			failFast:    true,
			attempts: [2]attempt{
				{delay / 2, e1},
				{delay / 2, nil},
			},
			expectRetry: false,
			expectError: e1,
		},
		{
			description: "Attempt fails before retry and not FailFast",
			failFast:    false,
			attempts: [2]attempt{
				{delay / 2, e1},
				{delay / 2, nil},
			},
			expectRetry: true,
			expectError: nil,
		},

		{
			description: "Attempt fails before retry and retry fails and FailFast",
			failFast:    true,
			attempts: [2]attempt{
				{delay / 2, e1},
				{delay / 2, e2},
			},
			expectRetry: false,
			expectError: e1,
		},
		{
			description: "Attempt fails before retry and retry fails and not FailFast",
			failFast:    false,
			attempts: [2]attempt{
				{delay / 2, e1},
				{delay / 2, e2},
			},
			expectRetry: true,
			expectError: e2,
		},

		{
			description: "Attempt fails after retry and FailFast",
			failFast:    true,
			attempts: [2]attempt{
				{delay * 2, e1},
				{delay * 2, nil},
			},
			expectRetry: false,
			expectError: e1,
		},
		{
			description: "Attempt fails after retry and not FailFast",
			failFast:    false,
			attempts: [2]attempt{
				{delay * 2, e1},
				{delay * 2, nil},
			},
			expectRetry: true,
			expectError: nil,
		},

		{
			description: "Attempt fails after retry and retry fails and FailFast",
			failFast:    true,
			attempts: [2]attempt{
				{delay * 2, e1},
				{delay * 2, e2},
			},
			expectRetry: false,
			expectError: e1,
		},
		{
			description: "Attempt fails after retry and retry fails and not FailFast",
			failFast:    false,
			attempts: [2]attempt{
				{delay * 2, e1},
				{delay * 2, e2},
			},
			expectRetry: true,
			expectError: e2,
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.description, func(t *testing.T) {
			t.Parallel()
			a := Hedged[bool]{
				Call:     createCall(c),
				Trigger:  time.After(delay),
				FailFast: c.failFast,
			}
			r, err := a.Do(ctx)
			if r != c.expectRetry {
				t.Fatal("expected retry")
			}
			if !errors.Is(err, c.expectError) {
				t.Fatal("expected error", c.expectError)
			}
		})
	}
}

func Test_Hedging_Limiter(t *testing.T) {
	type testCase struct {
		description string
		runner      Throttler
		maxAttempts int64
	}

	const attempts = 5
	testCases := []testCase{
		{
			description: "zero limit disables retries",
			runner:      NewLimiter(0),
			maxAttempts: attempts,
		},
		{
			description: "number of attempts does not exceed the limit",
			runner:      NewLimiter(2),
			maxAttempts: attempts + 2,
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.description, func(t *testing.T) {
			t.Parallel()

			var n int64
			attempt := Hedged[int]{
				Throttler: NewLimiter(0),
				Call: func(context.Context, bool) (int, error) {
					atomic.AddInt64(&n, 1)
					<-time.After(time.Millisecond)
					return 0, nil
				},
			}

			for i := 0; i < 5; i++ {
				_, _ = attempt.Do(context.Background())
			}

			if n > c.maxAttempts {
				t.Fatal("number of attempts exceeded")
			}
		})
	}
}
