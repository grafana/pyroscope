package queryfrontend

import queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"

// maxReadsForQueries batches more blocks per READ when queries do not need
// symbols or stacktraces. For mixed requests, the most expensive query wins.
func maxReadsForQueries(queries []*queryv1.Query) int {
	if len(queries) == 0 {
		return 4
	}
	maxReads := 64
	for _, query := range queries {
		switch query.GetQueryType() {
		case queryv1.QueryType_QUERY_LABEL_NAMES,
			queryv1.QueryType_QUERY_LABEL_VALUES,
			queryv1.QueryType_QUERY_SERIES_LABELS:
			// These queries only read the TSDB index.
		case queryv1.QueryType_QUERY_TIME_SERIES,
			queryv1.QueryType_QUERY_TIME_SERIES_COMPACT,
			queryv1.QueryType_QUERY_TIME_SERIES_ANALYSIS,
			queryv1.QueryType_QUERY_PROFILE_PRESENCE:
			maxReads = min(maxReads, 32)
		default:
			// Preserve the conservative limit for expensive and unknown queries.
			return 4
		}
	}
	return maxReads
}
