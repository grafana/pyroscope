package querier

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
)

func Test_ParseQuery(t *testing.T) {
	q := url.Values{
		"query": []string{`memory:alloc_space:bytes:space:bytes{foo="bar",bar=~"buzz"}`},
		"from":  []string{"now-6h"},
		"until": []string{"now"},
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost/render/render?%s", q.Encode()), nil)
	require.NoError(t, err)
	require.NoError(t, req.ParseForm())

	queryRequest, ptype, err := parseSelectProfilesRequest(renderRequestFieldNames{}, req)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), model.Time(queryRequest.End).Time(), 1*time.Minute)
	require.WithinDuration(t, time.Now().Add(-6*time.Hour), model.Time(queryRequest.Start).Time(), 1*time.Minute)

	require.Equal(t, &typesv1.ProfileType{
		ID:         "memory:alloc_space:bytes:space:bytes",
		Name:       "memory",
		SampleType: "alloc_space",
		SampleUnit: "bytes",
		PeriodType: "space",
		PeriodUnit: "bytes",
	}, ptype)

	require.Equal(t, `{foo="bar",bar=~"buzz"}`, queryRequest.LabelSelector)
}
