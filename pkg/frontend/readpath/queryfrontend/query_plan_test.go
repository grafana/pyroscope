package queryfrontend

import (
	"testing"

	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

func TestMaxReadsForQueries(t *testing.T) {
	for _, tt := range []struct {
		name  string
		types []queryv1.QueryType
		want  int
	}{
		{name: "empty", want: 4},
		{name: "label names", types: []queryv1.QueryType{queryv1.QueryType_QUERY_LABEL_NAMES}, want: 64},
		{name: "label values", types: []queryv1.QueryType{queryv1.QueryType_QUERY_LABEL_VALUES}, want: 64},
		{name: "series labels", types: []queryv1.QueryType{queryv1.QueryType_QUERY_SERIES_LABELS}, want: 64},
		{name: "time series", types: []queryv1.QueryType{queryv1.QueryType_QUERY_TIME_SERIES}, want: 32},
		{name: "compact", types: []queryv1.QueryType{queryv1.QueryType_QUERY_TIME_SERIES_COMPACT}, want: 32},
		{name: "analysis", types: []queryv1.QueryType{queryv1.QueryType_QUERY_TIME_SERIES_ANALYSIS}, want: 32},
		{name: "presence", types: []queryv1.QueryType{queryv1.QueryType_QUERY_PROFILE_PRESENCE}, want: 32},
		{name: "tree", types: []queryv1.QueryType{queryv1.QueryType_QUERY_TREE}, want: 4},
		{name: "pprof", types: []queryv1.QueryType{queryv1.QueryType_QUERY_PPROF}, want: 4},
		{name: "heatmap", types: []queryv1.QueryType{queryv1.QueryType_QUERY_HEATMAP}, want: 4},
		{name: "unspecified", types: []queryv1.QueryType{queryv1.QueryType_QUERY_UNSPECIFIED}, want: 4},
		{name: "unknown", types: []queryv1.QueryType{queryv1.QueryType(999)}, want: 4},
		{name: "mixed light", types: []queryv1.QueryType{queryv1.QueryType_QUERY_TIME_SERIES, queryv1.QueryType_QUERY_LABEL_NAMES}, want: 32},
		{name: "mixed heavy", types: []queryv1.QueryType{queryv1.QueryType_QUERY_LABEL_VALUES, queryv1.QueryType_QUERY_TREE}, want: 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			queries := make([]*queryv1.Query, len(tt.types))
			for i, typ := range tt.types {
				queries[i] = &queryv1.Query{QueryType: typ}
			}
			require.Equal(t, tt.want, maxReadsForQueries(queries))
		})
	}
	t.Run("nil query", func(t *testing.T) {
		require.Equal(t, 4, maxReadsForQueries([]*queryv1.Query{nil}))
	})
}
