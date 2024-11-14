package test

import (
	"testing"
)

// AssertIdempotent asserts that the test is valid when run multiple times.
func AssertIdempotent(t *testing.T, fn func(*testing.T)) {
	t.Helper()
	for i := 0; i < 2; i++ {
		fn(t)
		if t.Failed() {
			if i > 0 {
				t.Fatal("the function is not idempotent")
			}
			return
		}
	}
}

func AssertIdempotentSubtest(t *testing.T, fn func(*testing.T)) func(*testing.T) {
	t.Helper()
	return func(t *testing.T) {
		for i := 0; i < 2; i++ {
			fn(t)
			if t.Failed() {
				if i > 0 {
					t.Fatal("the function is not idempotent")
				}
				return
			}
		}
	}
}
