package querier

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	ingesterv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
)

func Test_ParseQuery(t *testing.T) {
	q := url.Values{
		"query": []string{`memory:alloc_space:bytes:space:bytes{foo="bar",bar=~"buzz"}`},
		"from":  []string{"now-6h"},
		"until": []string{"now"},
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost/render/render?%s", q.Encode()), nil)
	require.NoError(t, err)

	queryRequest, err := parseSelectProfilesRequest(req)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), model.Time(queryRequest.End).Time(), 1*time.Minute)
	require.WithinDuration(t, time.Now().Add(-6*time.Hour), model.Time(queryRequest.Start).Time(), 1*time.Minute)

	require.Equal(t, &ingesterv1.ProfileType{
		Name:       "memory",
		SampleType: "alloc_space",
		SampleUnit: "bytes",
		PeriodType: "space",
		PeriodUnit: "bytes",
	}, queryRequest.Type)

	require.Equal(t, `{foo="bar",bar=~"buzz"}`, queryRequest.LabelSelector)
}
