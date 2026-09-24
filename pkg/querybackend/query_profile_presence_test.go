package querybackend

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

// TestProfilePresenceIteratorOptions guards the two options queryProfilePresence relies on:
// excludeSampled (stripped profiles have no resolvable data, so must not be reported present)
// and allLabels (this query type returns each profile's own labels to the caller, unlike other
// profileEntryIterator callers that only use labels internally). Losing either compiles fine and
// changes behavior silently, so this fails loudly instead.
func TestProfilePresenceIteratorOptions(t *testing.T) {
	opts, err := profilePresenceIteratorOptions([]string{"11111111-1111-1111-1111-111111111111"})
	require.NoError(t, err)

	var it iteratorOpts
	var se seriesOpts
	for _, o := range opts {
		if o.iterator != nil {
			o.iterator(&it)
		}
		if o.series != nil {
			o.series(&se)
		}
	}

	require.True(t, it.excludeSampled, "queryProfilePresence must exclude __sampled__ profiles")
	require.True(t, se.allLabels, "queryProfilePresence must fetch each profile's own labels")
}

// Test_QueryProfilePresence_Basic checks QUERY_PROFILE_PRESENCE against real block data: a real
// profile ID mixed with a non-existent one resolves to just the real one, with its labels
// populated.
func (s *testSuite) Test_QueryProfilePresence_Basic() {
	validProfileID := s.getProfileIDFromExemplars(s.T())
	const nonExistentID = "00000000-0000-0000-0000-000000000000"

	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		StartTime:     startTime.UnixMilli(),
		EndTime:       startTime.Add(5 * time.Minute).UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_PROFILE_PRESENCE,
			ProfilePresence: &queryv1.ProfilePresenceQuery{
				ProfileIdSelector: []string{validProfileID, nonExistentID},
			},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Reports, 1)
	s.Assert().Equal(queryv1.ReportType_REPORT_PROFILE_PRESENCE, resp.Reports[0].ReportType)
	profiles := resp.Reports[0].ProfilePresence.Profiles
	s.Require().Len(profiles, 1)
	s.Assert().Equal(validProfileID, profiles[0].ProfileId)
	s.Assert().NotEmpty(profiles[0].Labels, "expected the profile's own labels to be populated")
	s.Assert().NotZero(profiles[0].Timestamp, "expected the profile's own timestamp to be populated")
}

// Test_QueryProfilePresence_NoneOfManyPresent checks a 100-candidate list, none present,
// resolves to an empty result.
func (s *testSuite) Test_QueryProfilePresence_NoneOfManyPresent() {
	candidates := make([]string, 100)
	for i := range candidates {
		candidates[i] = "00000000-0000-0000-0000-000000000000"
	}
	// Make them distinct but still non-existent (valid UUID shape required by withProfileIDSelector).
	for i := range candidates {
		candidates[i] = randomZeroUUIDVariant(i)
	}

	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		StartTime:     startTime.UnixMilli(),
		EndTime:       startTime.Add(5 * time.Minute).UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_PROFILE_PRESENCE,
			ProfilePresence: &queryv1.ProfilePresenceQuery{
				ProfileIdSelector: candidates,
			},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	s.Require().Len(resp.Reports, 1)
	s.Assert().Empty(resp.Reports[0].ProfilePresence.Profiles)
}

func randomZeroUUIDVariant(i int) string {
	// Same shape as "00000000-0000-0000-0000-000000000000" but with the last segment varied,
	// so every candidate is a distinct, well-formed, non-existent UUID.
	return "00000000-0000-0000-0000-" + padHex(i)
}

func padHex(i int) string {
	const hexDigits = "0123456789abcdef"
	b := make([]byte, 12)
	for pos := 11; pos >= 0; pos-- {
		b[pos] = hexDigits[i&0xf]
		i >>= 4
	}
	return string(b)
}
