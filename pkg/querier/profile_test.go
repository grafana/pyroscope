package querier

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/model"
)

func Test_DedupeProfiles(t *testing.T) {
	actual := dedupeProfiles([]responseFromIngesters[*ingestv1.SelectProfilesResponse]{
		{
			addr:     "A",
			response: buildResponses(t, []int64{1, 2, 3}, []string{`{app="foo"}`, `{app="bar"}`, `{app="buzz"}`}, []string{"foo", "bar", "buzz"}),
		},
		{
			addr:     "B",
			response: buildResponses(t, []int64{2, 3}, []string{`{app="bar"}`, `{app="buzz"}`}, []string{"bar", "buzz"}),
		},
		{
			addr:     "C",
			response: buildResponses(t, []int64{1, 2, 3}, []string{`{app="foo"}`, `{app="bar"}`, `{app="buzz"}`}, []string{"foo", "bar", "buzz"}),
		},
		{
			addr:     "D",
			response: buildResponses(t, []int64{2}, []string{`{app="bar"}`}, []string{"bar"}),
		},
		{
			addr:     "E",
			response: buildResponses(t, []int64{4}, []string{`{app="blah"}`}, []string{"blah"}),
		},
		{
			addr:     "F",
			response: buildResponses(t, []int64{}, []string{}, []string{}),
		},
	})
	require.Equal(t,
		[]profileWithSymbols{
			{profile: buildResponses(t, []int64{1}, []string{`{app="foo"}`}, nil).Profiles[0], symbols: []string{"foo", "bar", "buzz"}},
			{profile: buildResponses(t, []int64{2}, []string{`{app="bar"}`}, nil).Profiles[0], symbols: []string{"bar", "buzz"}},
			{profile: buildResponses(t, []int64{3}, []string{`{app="buzz"}`}, nil).Profiles[0], symbols: []string{"bar", "buzz"}},
			{profile: buildResponses(t, []int64{4}, []string{`{app="blah"}`}, nil).Profiles[0], symbols: []string{"blah"}},
		},
		actual)
}

func buildResponses(t *testing.T, timestamps []int64, labels []string, fns []string) *ingestv1.SelectProfilesResponse {
	t.Helper()
	result := &ingestv1.SelectProfilesResponse{
		Profiles:      make([]*ingestv1.Profile, len(timestamps)),
		FunctionNames: fns,
	}
	for i := range timestamps {
		ls, err := model.StringToLabelsPairs(labels[i])
		require.NoError(t, err)
		result.Profiles[i] = &ingestv1.Profile{
			ID:        fmt.Sprintf("%d - %s", timestamps[i], labels[i]),
			Timestamp: timestamps[i],
			Labels:    ls,
		}
	}
	sort.Slice(result.Profiles, func(i, j int) bool {
		return model.CompareProfile(result.Profiles[i], result.Profiles[j]) < 0
	})
	return result
}
