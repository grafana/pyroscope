package retry

import (
	"context"
	"errors"
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
			// With FailFast, it's expected the call returns
			// immediately after the first response. Increasing
			// the delay to make sure the test doesn't flake.
			d := delay
			if c.failFast {
				d = time.Second * 10
			}
			a := Hedged[bool]{
				Call:     createCall(c),
				Trigger:  time.After(d),
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
