package querier

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/ingester/clientpool"
	"github.com/grafana/fire/pkg/testutil"
	"github.com/grafana/fire/pkg/util"
)

func Test_QuerySampleType(t *testing.T) {
	querier, err := New(Config{
		PoolConfig: clientpool.PoolConfig{ClientCleanupPeriod: 1 * time.Millisecond},
	}, testutil.NewMockRing([]ring.InstanceDesc{
		{Addr: "1"},
		{Addr: "2"},
		{Addr: "3"},
	}, 3), func(addr string) (client.PoolClient, error) {
		q := newFakeQuerier()
		switch addr {
		case "1":
			q.On("ProfileTypes", mock.Anything, mock.Anything).Return(connect.NewResponse(&ingestv1.ProfileTypesResponse{Names: []string{"foo", "bar"}}), nil)
		case "2":
			q.On("ProfileTypes", mock.Anything, mock.Anything).Return(connect.NewResponse(&ingestv1.ProfileTypesResponse{Names: []string{"bar", "buzz"}}), nil)
		case "3":
			q.On("ProfileTypes", mock.Anything, mock.Anything).Return(connect.NewResponse(&ingestv1.ProfileTypesResponse{Names: []string{"buzz", "foo"}}), nil)
		}
		return q, nil
	}, log.NewLogfmtLogger(os.Stdout))

	require.NoError(t, err)
	out, err := querier.ProfileTypes(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"bar", "buzz", "foo"}, out)
}

func Test_QueryLabelValues(t *testing.T) {
	req := connect.NewRequest(&ingestv1.LabelValuesRequest{Name: "foo"})
	querier, err := New(Config{
		PoolConfig: clientpool.PoolConfig{ClientCleanupPeriod: 1 * time.Millisecond},
	}, testutil.NewMockRing([]ring.InstanceDesc{
		{Addr: "1"},
		{Addr: "2"},
		{Addr: "3"},
	}, 3), func(addr string) (client.PoolClient, error) {
		q := newFakeQuerier()
		switch addr {
		case "1":
			q.On("LabelValues", mock.Anything, req).Return(connect.NewResponse(&ingestv1.LabelValuesResponse{Names: []string{"foo", "bar"}}), nil)
		case "2":
			q.On("LabelValues", mock.Anything, req).Return(connect.NewResponse(&ingestv1.LabelValuesResponse{Names: []string{"bar", "buzz"}}), nil)
		case "3":
			q.On("LabelValues", mock.Anything, req).Return(connect.NewResponse(&ingestv1.LabelValuesResponse{Names: []string{"buzz", "foo"}}), nil)
		}
		return q, nil
	}, log.NewLogfmtLogger(os.Stdout))

	require.NoError(t, err)
	out, err := querier.LabelValues(context.Background(), req.Msg.Name)
	require.NoError(t, err)
	require.Equal(t, []string{"bar", "buzz", "foo"}, out)
}

type fakeQuerierIngester struct {
	mock.Mock
	testutil.FakePoolClient
}

func newFakeQuerier() *fakeQuerierIngester {
	return &fakeQuerierIngester{}
}

func (f *fakeQuerierIngester) LabelValues(ctx context.Context, req *connect.Request[ingestv1.LabelValuesRequest]) (*connect.Response[ingestv1.LabelValuesResponse], error) {
	var (
		args = f.Called(ctx, req)
		res  *connect.Response[ingestv1.LabelValuesResponse]
		err  error
	)
	if args[0] != nil {
		res = args[0].(*connect.Response[ingestv1.LabelValuesResponse])
	}
	if args[1] != nil {
		err = args.Get(1).(error)
	}
	return res, err
}

func (f *fakeQuerierIngester) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	var (
		args = f.Called(ctx, req)
		res  *connect.Response[ingestv1.ProfileTypesResponse]
		err  error
	)
	if args[0] != nil {
		res = args[0].(*connect.Response[ingestv1.ProfileTypesResponse])
	}
	if args[1] != nil {
		err = args.Get(1).(error)
	}

	return res, err
}

func Test_DedupeProfiles(t *testing.T) {
	actual := dedupeProfiles([]responseFromIngesters[*ingestv1.SelectProfilesResponse]{
		{
			addr:     "A",
			response: buildResponses(t, []int64{1, 2, 3}, []string{`{app="foo"}`, `{app="bar"}`, `{app="buzz"}`}),
		},
		{
			addr:     "B",
			response: buildResponses(t, []int64{2, 3}, []string{`{app="bar"}`, `{app="buzz"}`}),
		},
		{
			addr:     "C",
			response: buildResponses(t, []int64{1, 2, 3}, []string{`{app="foo"}`, `{app="bar"}`, `{app="buzz"}`}),
		},
		{
			addr:     "D",
			response: buildResponses(t, []int64{2}, []string{`{app="bar"}`}),
		},
	})
	require.Equal(t,
		buildResponses(t, []int64{1, 2, 3}, []string{`{app="foo"}`, `{app="bar"}`, `{app="buzz"}`}).Profiles,
		actual)
}

func buildResponses(t *testing.T, timestamps []int64, labels []string) *ingestv1.SelectProfilesResponse {
	t.Helper()
	result := &ingestv1.SelectProfilesResponse{
		Profiles: make([]*ingestv1.Profile, len(timestamps)),
	}
	for i := range timestamps {
		ls, err := util.StringToLabelsPairs(labels[i])
		require.NoError(t, err)
		result.Profiles[i] = &ingestv1.Profile{
			Timestamp: timestamps[i],
			Labels:    ls,
		}
	}
	sort.Slice(result.Profiles, func(i, j int) bool {
		return CompareProfile(result.Profiles[i], result.Profiles[j]) < 0
	})
	return result
}
