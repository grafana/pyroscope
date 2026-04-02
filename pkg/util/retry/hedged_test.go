package retry

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
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

// contextAwareRC is an io.ReadCloser that returns ctx.Err() on Read
// if the context is already cancelled. This simulates how HTTP response
// bodies behave when backed by a cancelled context (e.g. S3/GCS clients).
type contextAwareRC struct {
	ctx  context.Context
	data []byte
}

func (r *contextAwareRC) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, io.EOF
}

func (r *contextAwareRC) Close() error { return nil }

func Test_Hedging_WinnerContextNotCancelled(t *testing.T) {
	// Regression test: the winning attempt's io.ReadCloser must remain readable
	// after Do returns. Previously, Hedged.Do cancelled attemptCtx (shared by
	// all attempts) before returning, making the winning RC unreadable against
	// real HTTP-backed object storage.
	synctest.Test(t, func(t *testing.T) {
		data := []byte("hello world")
		const hedgeDelay = time.Second

		a := Hedged[io.ReadCloser]{
			Trigger:  time.After(hedgeDelay),
			FailFast: true,
			Cleanup:  func(rc io.ReadCloser) { rc.Close() },
			Call: func(ctx context.Context, isRetry bool) (io.ReadCloser, error) {
				if !isRetry {
					// slow: block until cancelled by the winning hedge
					<-ctx.Done()
					return nil, ctx.Err()
				}
				// fast winner: return RC tied to this context
				return &contextAwareRC{ctx: ctx, data: append([]byte(nil), data...)}, nil
			},
		}

		done := make(chan struct{})
		var rc io.ReadCloser
		var doErr error
		go func() {
			defer close(done)
			rc, doErr = a.Do(context.Background())
		}()

		synctest.Wait()
		time.Sleep(hedgeDelay) // trigger hedge; second call wins
		synctest.Wait()
		<-done

		if doErr != nil {
			t.Fatalf("unexpected error from Do: %v", doErr)
		}

		// Read from the RC after Do has returned. This fails with the buggy
		// implementation because attemptCtx is already cancelled at this point.
		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("reading from winning RC after Do returned: %v (context was cancelled prematurely)", err)
		}
		if string(got) != string(data) {
			t.Fatalf("expected %q, got %q", data, got)
		}
	})
}

func Test_Hedging_Cleanup(t *testing.T) {
	t.Run("cleanup called on loser when both succeed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var cleaned int64
			// First call is slow; hedge fires and wins. First call then
			// succeeds but loses — Cleanup must be called on its result.
			const hedgeDelay = time.Second
			a := Hedged[*int]{
				Trigger:  time.After(hedgeDelay),
				FailFast: true,
				Cleanup:  func(v *int) { atomic.AddInt64(&cleaned, 1) },
				Call: func(ctx context.Context, isRetry bool) (*int, error) {
					if !isRetry {
						// slow: block until cancelled by the winning hedge
						<-ctx.Done()
					}
					v := 1
					return &v, nil
				},
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, err := a.Do(context.Background())
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}()
			synctest.Wait()
			time.Sleep(hedgeDelay) // hedge fires and wins; slow call gets cancelled
			synctest.Wait()
			<-done
			if atomic.LoadInt64(&cleaned) != 1 {
				t.Fatal("expected Cleanup to be called exactly once on the loser")
			}
		})
	})

	t.Run("cleanup not called when only one attempt runs", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var cleaned int64
			a := Hedged[*int]{
				Trigger:  time.After(time.Hour),
				FailFast: true,
				Cleanup:  func(v *int) { atomic.AddInt64(&cleaned, 1) },
				Call: func(ctx context.Context, _ bool) (*int, error) {
					v := 1
					return &v, nil
				},
			}
			_, err := a.Do(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if atomic.LoadInt64(&cleaned) != 0 {
				t.Fatal("expected Cleanup not to be called")
			}
		})
	})

	t.Run("cleanup not called on loser that errored", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var cleaned int64
			e := errors.New("fail")
			const hedgeDelay = time.Second
			// Hedge fires; first call errors, second succeeds.
			// Cleanup must NOT be called on the errored loser.
			firstBlocked := make(chan struct{})
			a := Hedged[*int]{
				Trigger:  time.After(hedgeDelay),
				FailFast: false,
				Cleanup:  func(v *int) { atomic.AddInt64(&cleaned, 1) },
				Call: func(ctx context.Context, isRetry bool) (*int, error) {
					if !isRetry {
						close(firstBlocked)
						<-ctx.Done()
						return nil, e
					}
					v := 1
					return &v, nil
				},
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, err := a.Do(context.Background())
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}()
			<-firstBlocked
			synctest.Wait()
			time.Sleep(hedgeDelay)
			synctest.Wait()
			<-done
			if atomic.LoadInt64(&cleaned) != 0 {
				t.Fatal("expected Cleanup not to be called on errored attempt")
			}
		})
	})
}
