package delayhandler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/grafana/dskit/tenant"

	"github.com/grafana/pyroscope/pkg/util"
)

var (
	// have local variables to allow for mocking in tests
	timeNow   = time.Now
	timeAfter = time.After
)

type Limits interface {
	IngestionArtificialDelay(tenantID string) time.Duration
}

type delayCancelCtxKey struct{}

func CancelDelay(ctx context.Context) {
	if cancel, ok := ctx.Value(delayCancelCtxKey{}).(context.CancelFunc); ok && cancel != nil {
		cancel()
	}
}

func addDelayHeader(h http.Header, delay time.Duration) {
	durationInMs := strconv.FormatFloat(float64(delay)/float64(time.Millisecond), 'f', -1, 64)
	h.Add("Server-Timing", fmt.Sprintf("artificial_delay;dur=%s", durationInMs))
}

func getDelay(ctx context.Context, limits Limits) time.Duration {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return 0
	}

	delay := limits.IngestionArtificialDelay(tenantID)
	if delay > 0 {
		return util.DurationWithJitter(delay, 0.10)
	}
	return 0
}
