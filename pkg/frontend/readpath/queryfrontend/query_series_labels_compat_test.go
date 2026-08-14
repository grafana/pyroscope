package queryfrontend

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockqueryfrontend"
)

func Test_isProfileTypeQuery(t *testing.T) {
	for _, tc := range []struct {
		name     string
		names    []string
		matchers []string
		expected bool
	}{
		{
			name:     "no matchers",
			names:    profileTypeLabels2,
			matchers: nil,
			expected: true,
		},
		{
			// Profiles Drilldown spells an unfiltered query this way.
			name:     "empty selector",
			names:    profileTypeLabels2,
			matchers: []string{"{}"},
			expected: true,
		},
		{
			name:     "empty selector with whitespace",
			names:    profileTypeLabels2,
			matchers: []string{"{} "},
			expected: true,
		},
		{
			name:     "several empty selectors",
			names:    profileTypeLabels2,
			matchers: []string{"{}", "{}"},
			expected: true,
		},
		{
			name:     "empty selector with five label names",
			names:    profileTypeLabels5,
			matchers: []string{"{}"},
			expected: true,
		},
		{
			name:     "label names in arbitrary order",
			names:    []string{phlaremodel.LabelNameServiceName, phlaremodel.LabelNameProfileType},
			matchers: []string{"{}"},
			expected: true,
		},
		{
			name:     "constraining matcher",
			names:    profileTypeLabels2,
			matchers: []string{`{service_name="a"}`},
			expected: false,
		},
		{
			name:     "empty selector alongside a constraining matcher",
			names:    profileTypeLabels2,
			matchers: []string{"{}", `{service_name="a"}`},
			expected: false,
		},
		{
			name:     "unrelated label names",
			names:    []string{"foo", "bar"},
			matchers: []string{"{}"},
			expected: false,
		},
		{
			name:     "unsupported label name count",
			names:    []string{phlaremodel.LabelNameServiceName},
			matchers: []string{"{}"},
			expected: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matchers, err := parseMatchers(tc.matchers)
			require.NoError(t, err)

			names := make([]string, len(tc.names))
			copy(names, tc.names)

			q := new(QueryFrontend)
			assert.Equal(t, tc.expected, q.isProfileTypeQuery(names, matchers))
		})
	}
}

// Test_QueryFrontend_Series_ProfileTypeQueryServedFromMetadata guards the read
// amplification fixed here: an unfiltered profile-type query spelled "{}" used
// to miss the metadata path and scan the TSDB index of every block in range.
func Test_QueryFrontend_Series_ProfileTypeQueryServedFromMetadata(t *testing.T) {
	for _, tc := range []struct {
		name     string
		matchers []string
	}{
		{name: "no matchers", matchers: nil},
		{name: "empty selector", matchers: []string{"{}"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// No expectations are set on the query backend, so reaching it at
			// all fails the test.
			mockQueryBackend := mockqueryfrontend.NewMockQueryBackend(t)

			mockMetadataClient := new(mockmetastorev1.MockMetadataQueryServiceClient)
			mockMetadataClient.On("QueryMetadataLabels", mock.Anything, mock.Anything).
				Return(&metastorev1.QueryMetadataLabelsResponse{
					Labels: []*typesv1.Labels{{Labels: []*typesv1.LabelPair{
						{Name: phlaremodel.LabelNameServiceName, Value: "service-a"},
						{Name: phlaremodel.LabelNameProfileType, Value: smpProfileType},
					}}},
				}, nil).
				Once()

			mockLimits := mockfrontend.NewMockLimits(t)
			mockLimits.On("MaxQueryLookback", "test-tenant").Return(time.Duration(0))
			mockLimits.On("MaxQueryLength", "test-tenant").Return(time.Duration(0))

			qf := NewQueryFrontend(
				log.NewNopLogger(),
				mockLimits,
				mockMetadataClient,
				nil,
				mockQueryBackend,
				nil,
				nil,
				nil,
			)

			start, end := smpValidTimeRange()
			ctx := tenant.InjectTenantID(context.Background(), "test-tenant")
			resp, err := qf.Series(ctx, connect.NewRequest(&querierv1.SeriesRequest{
				Matchers:   tc.matchers,
				LabelNames: []string{phlaremodel.LabelNameServiceName, phlaremodel.LabelNameProfileType},
				Start:      start,
				End:        end,
			}))

			require.NoError(t, err)
			require.Len(t, resp.Msg.LabelsSet, 1)
			mockMetadataClient.AssertExpectations(t)
		})
	}
}
