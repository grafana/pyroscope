package spanlogger

import (
	"context"
	"time"
)

// QueryStats is populated by the query-frontend during query execution and
// read by LogSpanParametersWrapper after the call returns, so that bytes
// fetched can be included in the query log line.
type QueryStats struct {
	// ObjectStorageBytes is logical query cost, not physical storage traffic.
	// Result-cache hits reuse the original execution's cost.
	ObjectStorageBytes uint64
	MetastoreBytes     uint64
	// EstimatedBytes is the pre-execution metadata size estimate (weight.Total())
	// derived from block dataset section offsets before the backend is invoked.
	EstimatedBytes uint64
	// Backend durations cover successful queries only. Failed queries do not
	// return the execution statistics needed for the critical-path breakdown.
	ProcessingDuration  time.Duration
	BackoffDuration     time.Duration
	UnaccountedDuration time.Duration
}

type queryStatsKey struct{}

// ContextWithQueryStats attaches a fresh QueryStats to ctx and returns both
// the enriched context and a pointer to the stats struct.
func ContextWithQueryStats(ctx context.Context) (context.Context, *QueryStats) {
	s := &QueryStats{}
	return context.WithValue(ctx, queryStatsKey{}, s), s
}

// QueryStatsFromContext returns the QueryStats stored in ctx, or nil if none.
func QueryStatsFromContext(ctx context.Context) *QueryStats {
	s, _ := ctx.Value(queryStatsKey{}).(*QueryStats)
	return s
}
