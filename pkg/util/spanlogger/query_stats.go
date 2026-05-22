package spanlogger

import "context"

// QueryStats is populated by the query-frontend during query execution and
// read by LogSpanParametersWrapper after the call returns, so that bytes
// fetched can be included in the query log line.
type QueryStats struct {
	ObjectStorageBytes uint64
	MetastoreBytes     uint64
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
