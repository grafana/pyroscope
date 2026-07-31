package main

import (
	"context"

	"github.com/go-kit/log/level"
)

// Idle keeps the client container alive so Test Composer can invoke the test
// commands stored in the image. The idle process does not emit setup_complete;
// Kubernetes readiness is responsible for gating the start of this test setup.
func Idle(ctx context.Context) error {
	_ = level.Info(logger).Log("msg", "antithesis test driver is idle")
	<-ctx.Done()
	return nil
}
