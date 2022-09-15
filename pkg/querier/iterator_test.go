package querier

import (
	"context"
	"sort"
	"testing"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/testhelper"
	"github.com/stretchr/testify/require"
)

type testProfile struct {
	Ts     int64
	Labels *commonv1.Labels
}

type fakeBidiClient struct {
	profiles chan *ingestv1.ProfileSets
	batches  []*ingestv1.ProfileSets
	kept     []testProfile
	cur      *ingestv1.ProfileSets
}

func newFakeBidiClient(batches []*ingestv1.ProfileSets) *fakeBidiClient {
	res := &fakeBidiClient{
		profiles: make(chan *ingestv1.ProfileSets, 1),
	}
	res.profiles <- batches[0]
	batches = batches[1:]
	res.batches = batches
	return res
}

func (f *fakeBidiClient) Send(in *ingestv1.MergeProfilesStacktracesRequest) error {
	for i, b := range in.Profiles {
		if b {
			f.kept = append(f.kept, testProfile{
				Ts:     f.cur.Profiles[i].Timestamp,
				Labels: f.cur.LabelsSets[f.cur.Profiles[i].LabelIndex],
			})
		}
	}
	if len(f.batches) == 0 {
		close(f.profiles)
		return nil
	}
	f.profiles <- f.batches[0]
	f.batches = f.batches[1:]
	return nil
}

func (f *fakeBidiClient) Receive() (*ingestv1.MergeProfilesStacktracesResponse, error) {
	profiles := <-f.profiles
	if profiles == nil {
		return &ingestv1.MergeProfilesStacktracesResponse{
			Result: &ingestv1.MergeProfilesStacktracesResult{
				Stacktraces: []*ingestv1.StacktraceSample{
					{FunctionIds: []int32{0, 1, 2}, Value: 1},
				},
				FunctionNames: []string{"foo", "bar", "buzz"},
			},
		}, nil
	}
	f.cur = profiles
	return &ingestv1.MergeProfilesStacktracesResponse{
		SelectedProfiles: profiles,
	}, nil
}
func (f *fakeBidiClient) CloseSend() error    { return nil }
func (f *fakeBidiClient) CloseReceive() error { return nil }

func TestDedupeBidi(t *testing.T) {
	resp1 := newFakeBidiClient([]*ingestv1.ProfileSets{
		{
			LabelsSets: []*commonv1.Labels{{Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}}}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 1},
				{LabelIndex: 0, Timestamp: 2},
				{LabelIndex: 0, Timestamp: 4},
			},
		},
		{
			LabelsSets: []*commonv1.Labels{{Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}}}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 5},
				{LabelIndex: 0, Timestamp: 6},
			},
		},
	})
	resp2 := newFakeBidiClient([]*ingestv1.ProfileSets{
		{
			LabelsSets: []*commonv1.Labels{{Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}}}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 2},
				{LabelIndex: 0, Timestamp: 3},
				{LabelIndex: 0, Timestamp: 4},
			},
		},
		{
			LabelsSets: []*commonv1.Labels{{Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}}}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 5},
				{LabelIndex: 0, Timestamp: 6},
			},
		},
	})
	resp3 := newFakeBidiClient([]*ingestv1.ProfileSets{
		{
			LabelsSets: []*commonv1.Labels{{Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}}}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 3},
				{LabelIndex: 0, Timestamp: 5},
			},
		},
	})
	res, err := dedupe(context.Background(), []responseFromIngesters[BidiClientMergeProfilesStacktraces]{
		{
			response: resp1,
		},
		{
			response: resp2,
		},
		{
			response: resp3,
		},
	})
	require.NoError(t, err)
	require.Len(t, res, 1)
	all := []testProfile{}
	all = append(all, resp1.kept...)
	all = append(all, resp2.kept...)
	all = append(all, resp3.kept...)
	sort.Slice(all, func(i, j int) bool { return all[i].Ts < all[j].Ts })
	testhelper.EqualProto(t, all, []testProfile{
		{Ts: 1, Labels: &commonv1.Labels{Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}}}},
		{Ts: 2, Labels: &commonv1.Labels{Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}}}},
		{Ts: 3, Labels: &commonv1.Labels{Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}}}},
		{Ts: 4, Labels: &commonv1.Labels{Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}}}},
		{Ts: 5, Labels: &commonv1.Labels{Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}}}},
		{Ts: 6, Labels: &commonv1.Labels{Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}}}},
	})
	res, err = dedupe(context.Background(), []responseFromIngesters[BidiClientMergeProfilesStacktraces]{
		{
			response: newFakeBidiClient([]*ingestv1.ProfileSets{
				{
					LabelsSets: []*commonv1.Labels{{Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}}}},
					Profiles: []*ingestv1.SeriesProfile{
						{LabelIndex: 0, Timestamp: 1},
						{LabelIndex: 0, Timestamp: 2},
						{LabelIndex: 0, Timestamp: 4},
					},
				},
				{
					LabelsSets: []*commonv1.Labels{{Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}}}},
					Profiles: []*ingestv1.SeriesProfile{
						{LabelIndex: 0, Timestamp: 5},
						{LabelIndex: 0, Timestamp: 6},
					},
				},
			}),
		},
	})
	require.NoError(t, err)
	require.Len(t, res, 1)
}
