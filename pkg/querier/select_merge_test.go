package querier

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/testhelper"
)

func TestSelectMergeStacktraces(t *testing.T) {
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
	res, err := selectMergeStacktraces(context.Background(), []responseFromIngesters[BidiClientMergeProfilesStacktraces]{
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
	res, err = selectMergeStacktraces(context.Background(), []responseFromIngesters[BidiClientMergeProfilesStacktraces]{
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
