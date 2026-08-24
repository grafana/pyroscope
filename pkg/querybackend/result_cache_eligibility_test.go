package querybackend

import (
	"testing"

	"github.com/stretchr/testify/require"
	thanobjstore "github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
)

func TestResultCacheEligible_ServiceSelector(t *testing.T) {
	q := cacheTestBackend(objstore.NewBucket(thanobjstore.NewInMemBucket()), nil)
	for _, tt := range []struct {
		selector string
		eligible bool
	}{
		{`{}`, true},
		{`{service_name="api"}`, false},
		{`{service_name="api",cluster="prod"}`, false},
		{`{service_name=~"api"}`, false},
		{`{service_name=~"api|web"}`, true},
		{`{service_name=~".*"}`, true},
		{`{service_name!="api"}`, true},
		{`{service_name!~"api"}`, true},
		{`{cluster="prod"}`, true},
		{`invalid{`, false},
	} {
		t.Run(tt.selector, func(t *testing.T) {
			req, block := cacheTestRequest()
			req.LabelSelector = tt.selector
			require.Equal(t, tt.eligible, q.resultCacheEligible(req))
			require.Equal(t, tt.eligible, q.blockCacheEligible(req, block))
		})
	}
}
