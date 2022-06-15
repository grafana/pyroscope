package ingester

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/google/pprof/profile"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	pushv1 "github.com/grafana/fire/pkg/gen/push/v1"
	"github.com/grafana/fire/pkg/profilestore"
)

func Test_ParseQuery(t *testing.T) {
	q := url.Values{
		"query": []string{`memory:alloc_space:bytes:space:bytes{foo="bar",bar=~"buzz"}`},
		"from":  []string{"now-6h"},
		"until": []string{"now"},
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost/render/render?%s", q.Encode()), nil)
	require.NoError(t, err)

	queryRequest, err := parseQueryRequest(req)
	require.NoError(t, err)
	require.Equal(t, `memory:alloc_space:bytes:space:bytes{foo="bar",bar=~"buzz"}`, queryRequest.query)
	require.WithinDuration(t, time.Now(), model.Time(queryRequest.end).Time(), 1*time.Minute)
	require.WithinDuration(t, time.Now().Add(-6*time.Hour), model.Time(queryRequest.start).Time(), 1*time.Minute)

	query, err := parseQuery(queryRequest.query)
	require.NoError(t, err)
	require.Equal(t, profileQuery{
		selector:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"), labels.MustNewMatcher(labels.MatchRegexp, "bar", "buzz")},
		name:       "memory",
		sampleType: "alloc_space",
		sampleUnit: "bytes",
		periodType: "space",
		periodUnit: "bytes",
	}, query)
}

func Test_selectMerge(t *testing.T) {
	cfg := defaultIngesterTestConfig(t)
	profileStore, err := profilestore.New(log.NewNopLogger(), nil, trace.NewNoopTracerProvider(), &profilestore.Config{})
	require.NoError(t, err)
	buf := bytes.NewBuffer(nil)
	mapping := &profile.Mapping{
		ID: 1,
	}
	fns := []*profile.Function{
		{ID: 1, Name: "foo", StartLine: 1},
		{ID: 2, Name: "bar", StartLine: 1},
		{ID: 3, Name: "buzz", StartLine: 1},
	}
	locs := []*profile.Location{
		{
			ID: 1, Address: 1, Mapping: mapping, Line: []profile.Line{
				{Function: fns[0], Line: 1},
			},
		},
		{
			ID: 2, Address: 2, Mapping: mapping, Line: []profile.Line{
				{Function: fns[1], Line: 1},
			},
		},
		{
			ID: 3, Address: 3, Mapping: mapping, Line: []profile.Line{
				{Function: fns[2], Line: 1},
			},
		},
	}
	p := &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "inuse_space", Unit: "bytes"},
		},
		PeriodType: &profile.ValueType{
			Type: "space",
			Unit: "bytes",
		},
		DurationNanos: 0,
		Period:        3,
		TimeNanos:     time.Now().Add(-1 * time.Minute).UnixNano(),
		Sample: []*profile.Sample{
			{Value: []int64{1}, Location: []*profile.Location{locs[1], locs[0]}},
			{Value: []int64{1}, Location: []*profile.Location{locs[2], locs[0]}},
		},
		Mapping: []*profile.Mapping{
			mapping,
		},
		Function: fns,
		Location: locs,
	}
	require.NoError(t, p.Write(buf))
	d, err := New(cfg, log.NewNopLogger(), nil, profileStore)
	require.NoError(t, err)
	resp, err := d.Push(context.Background(), connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*pushv1.LabelPair{
					{Name: "__name__", Value: "memory"},
				},
				Samples: []*pushv1.RawSample{
					{
						RawProfile: buf.Bytes(),
					},
				},
			},
		},
	}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	f, err := d.selectMerge(context.Background(), profileQuery{
		name:       "memory",
		sampleType: "inuse_space",
		sampleUnit: "bytes",
		periodType: "space",
		periodUnit: "bytes",
	}, 0, int64(model.Latest))
	require.NoError(t, err)

	// aggregate plan have no guarantee of order so we sort the results
	sort.Strings(f.Flamebearer.Names)

	require.Equal(t, []string{"bar", "buzz", "foo", "total"}, f.Flamebearer.Names)
	require.Equal(t, flamebearer.FlamebearerMetadataV1{
		Format: "single",
		Units:  "bytes",
		Name:   "inuse_space",
	}, f.Metadata)
	require.Equal(t, 2, f.Flamebearer.NumTicks)
	require.Equal(t, 1, f.Flamebearer.MaxSelf)
	require.Equal(t, []int{0, 2, 0, 0}, f.Flamebearer.Levels[0])
	require.Equal(t, []int{0, 2, 0, 1}, f.Flamebearer.Levels[1])
	require.Equal(t, []int{0, 1, 1}, f.Flamebearer.Levels[2][:3])
	require.Equal(t, []int{0, 1, 1}, f.Flamebearer.Levels[2][4:7])
	require.True(t, f.Flamebearer.Levels[2][3] == 3 || f.Flamebearer.Levels[2][3] == 2)
	require.True(t, f.Flamebearer.Levels[2][7] == 3 || f.Flamebearer.Levels[2][7] == 2)
	require.NoError(
		t,
		profileStore.Close(),
	)
}
