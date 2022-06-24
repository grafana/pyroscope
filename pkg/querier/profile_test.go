package querier

import (
	"sort"
	"testing"

	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/model"
	"github.com/stretchr/testify/require"
)

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
		{
			addr:     "E",
			response: buildResponses(t, []int64{4}, []string{`{app="blah"}`}),
		},
		{
			addr:     "F",
			response: buildResponses(t, []int64{}, []string{}),
		},
	})
	require.Equal(t,
		map[string][]*ingestv1.Profile{
			"A": buildResponses(t, []int64{1}, []string{`{app="foo"}`}).Profiles,
			"B": buildResponses(t, []int64{2}, []string{`{app="bar"}`}).Profiles,
			"C": buildResponses(t, []int64{3}, []string{`{app="buzz"}`}).Profiles,
			"E": buildResponses(t, []int64{4}, []string{`{app="blah"}`}).Profiles,
		},
		actual)
}

func buildResponses(t *testing.T, timestamps []int64, labels []string) *ingestv1.SelectProfilesResponse {
	t.Helper()
	result := &ingestv1.SelectProfilesResponse{
		Profiles: make([]*ingestv1.Profile, len(timestamps)),
	}
	for i := range timestamps {
		ls, err := model.StringToLabelsPairs(labels[i])
		require.NoError(t, err)
		result.Profiles[i] = &ingestv1.Profile{
			Timestamp: timestamps[i],
			Labels:    ls,
		}
	}
	sort.Slice(result.Profiles, func(i, j int) bool {
		return model.CompareProfile(result.Profiles[i], result.Profiles[j]) < 0
	})
	return result
}
