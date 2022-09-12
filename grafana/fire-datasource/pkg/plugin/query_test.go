package plugin

import (
	"context"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	v1 "github.com/grafana/fire/pkg/gen/common/v1"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/fire/pkg/gen/querier/v1"
)

// This is where the tests for the datasource backend live.
func Test_query(t *testing.T) {
	ds := &FireDatasource{
		client: &FakeClient{},
	}

	dataQuery := backend.DataQuery{
		RefID:         "A",
		QueryType:     "",
		MaxDataPoints: 0,
		Interval:      0,
		TimeRange: backend.TimeRange{
			From: time.UnixMilli(10000),
			To:   time.UnixMilli(20000),
		},
		JSON: []byte(`{"profileTypeId":"foo:bar","labelSelector":"{app=\\\"baz\\\"}"}`),
	}

	resp := ds.query(context.Background(), backend.PluginContext{}, dataQuery)
	require.Nil(t, resp.Error)
	require.Equal(t, 1, len(resp.Frames))
	require.Equal(t, data.NewField("levels", nil, []string{"[1,2,3,4]", "[5,6]", "[7,8,9]"}), resp.Frames[0].Fields[0])
}

// This is where the tests for the datasource backend live.
func Test_responseToDataFrames(t *testing.T) {
	resp := &connect.Response[querierv1.SelectMergeStacktracesResponse]{
		Msg: &querierv1.SelectMergeStacktracesResponse{
			Flamegraph: &querierv1.FlameGraph{
				Names: []string{"func1", "func2", "func3"},
				Levels: []*querierv1.Level{
					{Values: []int64{1, 2, 3, 4}},
					{Values: []int64{5, 6, 7, 8, 9}},
				},
				Total:   987,
				MaxSelf: 123,
			},
		},
	}
	frame, err := responseToDataFrames(resp, "memory:alloc_objects:count:space:bytes")
	require.NoError(t, err)
	require.Equal(t, []string{"func1", "func2", "func3"}, frame.Meta.Custom.(CustomMeta).Names)
	require.Equal(t, int64(123), frame.Meta.Custom.(CustomMeta).MaxSelf)
	require.Equal(t, int64(987), frame.Meta.Custom.(CustomMeta).Total)
	require.Equal(t, 1, len(frame.Fields))
	require.Equal(t, data.NewField("levels", nil, []string{"[1,2,3,4]", "[5,6,7,8,9]"}), frame.Fields[0])
	require.Equal(t, "memory:alloc_objects:count:space:bytes", frame.Meta.Custom.(CustomMeta).ProfileTypeID)
}

type FakeClient struct {
	Req *connect.Request[querierv1.SelectMergeStacktracesRequest]
}

func (f FakeClient) ProfileTypes(ctx context.Context, c *connect.Request[querierv1.ProfileTypesRequest]) (*connect.Response[querierv1.ProfileTypesResponse], error) {
	panic("implement me")
}

func (f FakeClient) LabelValues(ctx context.Context, c *connect.Request[querierv1.LabelValuesRequest]) (*connect.Response[querierv1.LabelValuesResponse], error) {
	panic("implement me")
}

func (f FakeClient) LabelNames(context.Context, *connect.Request[querierv1.LabelNamesRequest]) (*connect.Response[querierv1.LabelNamesResponse], error) {
	panic("implement me")
}

func (f FakeClient) Series(ctx context.Context, c *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	return &connect.Response[querierv1.SeriesResponse]{
		Msg: &querierv1.SeriesResponse{
			LabelsSet: []*v1.Labels{{
				Labels: []*v1.LabelPair{
					{
						Name:  "__unit__",
						Value: "cpu",
					},
					{
						Name:  "instance",
						Value: "127.0.0.1",
					},
					{
						Name:  "job",
						Value: "default",
					},
				},
			}},
		},
	}, nil
}

func (f FakeClient) SelectMergeStacktraces(ctx context.Context, c *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	f.Req = c
	return &connect.Response[querierv1.SelectMergeStacktracesResponse]{
		Msg: &querierv1.SelectMergeStacktracesResponse{
			Flamegraph: &querierv1.FlameGraph{
				Names: []string{"foo", "bar"},
				Levels: []*querierv1.Level{
					{Values: []int64{1, 2, 3, 4}},
					{Values: []int64{5, 6}},
					{Values: []int64{7, 8, 9}},
				},
				Total:   100,
				MaxSelf: 56,
			},
		},
	}, nil
}

func (f FakeClient) SelectSeries(ctx context.Context, req *connect.Request[querierv1.SelectSeriesRequest]) (*connect.Response[querierv1.SelectSeriesResponse], error) {
	panic("implement me")
}
