package validation

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

func TestValidateLabels(t *testing.T) {
	for _, tt := range []struct {
		name           string
		lbs            []*typesv1.LabelPair
		expectedErr    string
		expectedReason Reason
	}{
		{
			name: "valid labels",
			lbs: []*typesv1.LabelPair{
				{Name: "foo", Value: "bar"},
				{Name: model.MetricNameLabel, Value: "qux"},
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
			},
		},
		{
			name:           "empty labels",
			lbs:            []*typesv1.LabelPair{},
			expectedErr:    `error at least one label pair is required per profile`,
			expectedReason: MissingLabels,
		},
		{
			name: "missing service name",
			lbs: []*typesv1.LabelPair{
				{Name: model.MetricNameLabel, Value: "qux"},
			},
			expectedErr:    `invalid labels '{__name__="qux"}' with error: service name is not provided`,
			expectedReason: MissingLabels,
		},
		{
			name: "max labels",
			lbs: []*typesv1.LabelPair{
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
				{Name: "foo1", Value: "bar"},
				{Name: "foo2", Value: "bar"},
				{Name: "foo3", Value: "bar"},
				{Name: "foo4", Value: "bar"},
			},
			expectedErr:    `profile series '{foo1="bar", foo2="bar", foo3="bar", foo4="bar", service_name="svc"}' has 5 label names; limit 3`,
			expectedReason: MaxLabelNamesPerSeries,
		},
		{
			name: "invalid metric name",
			lbs: []*typesv1.LabelPair{
				{Name: model.MetricNameLabel, Value: "&&"},
			},
			expectedErr:    `invalid labels '{__name__="&&"}' with error: invalid metric name`,
			expectedReason: InvalidLabels,
		},
		{
			name: "invalid label value",
			lbs: []*typesv1.LabelPair{
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
				{Name: model.MetricNameLabel, Value: "qux"},
				{Name: "foo", Value: "\xc5"},
			},
			expectedErr:    "invalid labels '{__name__=\"qux\", foo=\"\\xc5\", service_name=\"svc\"}' with error: invalid label value '\xc5'",
			expectedReason: InvalidLabels,
		},
		{
			name: "invalid label name",
			lbs: []*typesv1.LabelPair{
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
				{Name: model.MetricNameLabel, Value: "qux"},
				{Name: "\xc5", Value: "foo"},
			},
			expectedErr:    "invalid labels '{__name__=\"qux\", service_name=\"svc\", \xc5=\"foo\"}' with error: invalid label name '\xc5'",
			expectedReason: InvalidLabels,
		},
		{
			name: "name too long",
			lbs: []*typesv1.LabelPair{
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
				{Name: "foooooooooooooooo", Value: "bar"},
				{Name: model.MetricNameLabel, Value: "qux"},
			},
			expectedReason: LabelNameTooLong,
			expectedErr:    "profile with labels '{__name__=\"qux\", foooooooooooooooo=\"bar\", service_name=\"svc\"}' has label name too long: 'foooooooooooooooo'",
		},
		{
			name: "value too long",
			lbs: []*typesv1.LabelPair{
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
				{Name: "foo", Value: "barrrrrrrrrrrrrrr"},
				{Name: model.MetricNameLabel, Value: "qux"},
			},
			expectedReason: LabelValueTooLong,
			expectedErr:    `profile with labels '{__name__="qux", foo="barrrrrrrrrrrrrrr", service_name="svc"}' has label value too long: 'barrrrrrrrrrrrrrr'`,
		},

		{
			name: "dupe",
			lbs: []*typesv1.LabelPair{
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
				{Name: phlaremodel.LabelNameServiceName, Value: "svc"},
				{Name: model.MetricNameLabel, Value: "qux"},
			},
			expectedReason: DuplicateLabelNames,
			expectedErr:    "profile with labels '{__name__=\"qux\", service_name=\"svc\", service_name=\"svc\"}' has duplicate label name: 'service_name'",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLabels(MockLimits{
				MaxLabelNamesPerSeriesValue: 3,
				MaxLabelNameLengthValue:     12,
				MaxLabelValueLengthValue:    10,
			}, "foo", tt.lbs)
			if tt.expectedErr != "" {
				require.Error(t, err)
				require.Equal(t, tt.expectedErr, err.Error())
				require.Equal(t, tt.expectedReason, ReasonOf(err))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_ValidateRangeRequest(t *testing.T) {
	now := model.Now()
	for _, tt := range []struct {
		name        string
		in          model.Interval
		expectedErr error
		expected    ValidatedRangeRequest
	}{
		{
			name: "valid",
			in: model.Interval{
				Start: now.Add(-24 * time.Hour),
				End:   now,
			},
			expected: ValidatedRangeRequest{
				Interval: model.Interval{
					Start: now.Add(-24 * time.Hour),
					End:   now,
				},
			},
		},
		{
			name: "empty outside of the lookback",
			in: model.Interval{
				Start: now.Add(-75 * time.Hour),
				End:   now.Add(-73 * time.Hour),
			},
			expected: ValidatedRangeRequest{
				IsEmpty: true,
				Interval: model.Interval{
					Start: now.Add(-75 * time.Hour),
					End:   now.Add(-73 * time.Hour),
				},
			},
		},
		{
			name: "too large range",
			in: model.Interval{
				Start: now.Add(-150 * time.Hour),
				End:   now.Add(time.Hour),
			},
			expected:    ValidatedRangeRequest{},
			expectedErr: NewErrorf(QueryLimit, QueryTooLongErrorMsg, "73h0m0s", "2d"),
		},
		{
			name: "reduced range to the lookback",
			in: model.Interval{
				Start: now.Add(-75 * time.Hour),
				End:   now.Add(-68 * time.Hour),
			},
			expected: ValidatedRangeRequest{
				Interval: model.Interval{
					Start: now.Add(-72 * time.Hour),
					End:   now.Add(-68 * time.Hour),
				},
			},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			actual, err := ValidateRangeRequest(MockLimits{
				MaxQueryLengthValue:   48 * time.Hour,
				MaxQueryLookbackValue: 72 * time.Hour,
			}, []string{"foo"}, tt.in, now)
			require.Equal(t, tt.expectedErr, err)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestValidateProfile(t *testing.T) {
	now := model.TimeFromUnixNano(1_676_635_994_000_000_000)

	for _, tc := range []struct {
		name        string
		profile     *googlev1.Profile
		size        int
		limits      ProfileValidationLimits
		expectedErr error
		assert      func(t *testing.T, profile *googlev1.Profile)
	}{
		{
			"nil profile",
			nil,
			0,
			MockLimits{},
			nil,
			nil,
		},
		{
			"too big",
			&googlev1.Profile{},
			3,
			MockLimits{
				MaxProfileSizeBytesValue: 1,
			},
			NewErrorf(ProfileSizeLimit, ProfileTooBigErrorMsg, `{foo="bar"}`, 3, 1),
			nil,
		},
		{
			"too many samples",
			&googlev1.Profile{
				Sample: make([]*googlev1.Sample, 3),
			},
			0,
			MockLimits{
				MaxProfileStacktraceSamplesValue: 2,
			},
			NewErrorf(SamplesLimit, ProfileTooManySamplesErrorMsg, `{foo="bar"}`, 3, 2),
			nil,
		},
		{
			"too many labels",
			&googlev1.Profile{
				Sample: []*googlev1.Sample{
					{
						Label: make([]*googlev1.Label, 3),
					},
				},
			},
			0,
			MockLimits{
				MaxProfileStacktraceSampleLabelsValue: 2,
			},
			NewErrorf(SampleLabelsLimit, ProfileTooManySampleLabelsErrorMsg, `{foo="bar"}`, 3, 2),
			nil,
		},
		{
			"truncate labels and stacktrace",
			&googlev1.Profile{
				StringTable: []string{"foo", "/foo/bar"},
				Sample: []*googlev1.Sample{
					{
						LocationId: []uint64{0, 1, 2, 3, 4, 5},
					},
				},
			},
			0,
			MockLimits{
				MaxProfileStacktraceDepthValue:   2,
				MaxProfileSymbolValueLengthValue: 3,
			},
			nil,
			func(t *testing.T, profile *googlev1.Profile) {
				t.Helper()
				require.Equal(t, []string{"foo", "bar"}, profile.StringTable)
				require.Equal(t, []uint64{4, 5}, profile.Sample[0].LocationId)
			},
		},
		{
			name: "newer than ingestion window",
			profile: &googlev1.Profile{
				TimeNanos: now.Add(1 * time.Hour).UnixNano(),
			},
			limits: MockLimits{
				RejectNewerThanValue: 10 * time.Minute,
			},
			expectedErr: &Error{
				Reason: NotInIngestionWindow,
				msg:    "profile with labels '{foo=\"bar\"}' is outside of ingestion window (profile timestamp: 2023-02-17 13:13:14 +0000 UTC, the ingestion window ends at 2023-02-17 12:23:14 +0000 UTC)",
			},
		},
		{
			name: "older than ingestion window",
			profile: &googlev1.Profile{
				TimeNanos: now.Add(-61 * time.Minute).UnixNano(),
			},
			limits: MockLimits{
				RejectOlderThanValue: time.Hour,
			},
			expectedErr: &Error{
				Reason: NotInIngestionWindow,
				msg:    "profile with labels '{foo=\"bar\"}' is outside of ingestion window (profile timestamp: 2023-02-17 11:12:14 +0000 UTC, the ingestion window starts at 2023-02-17 11:13:14 +0000 UTC)",
			},
		},
		{
			name: "just in the ingestion window",
			profile: &googlev1.Profile{
				TimeNanos: now.Add(-1 * time.Minute).UnixNano(),
			},
			limits: MockLimits{
				RejectOlderThanValue: time.Hour,
				RejectNewerThanValue: 10 * time.Minute,
			},
		},
		{
			name:    "without timestamp",
			profile: &googlev1.Profile{},
			limits: MockLimits{
				RejectOlderThanValue: time.Hour,
				RejectNewerThanValue: 10 * time.Minute,
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateProfile(tc.limits, "foo", tc.profile, tc.size, phlaremodel.LabelsFromStrings("foo", "bar"), now)
			if tc.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedErr, err)
			} else {
				require.NoError(t, err)
			}

			if tc.assert != nil {
				tc.assert(t, tc.profile)
			}
		})
	}
}

func TestValidateFlamegraphMaxNodes(t *testing.T) {
	type testCase struct {
		name      string
		maxNodes  int64
		validated int64
		limits    FlameGraphLimits
		err       error
	}

	testCases := []testCase{
		{
			name:      "default limit",
			maxNodes:  0,
			validated: 10,
			limits: MockLimits{
				MaxFlameGraphNodesDefaultValue: 10,
			},
		},
		{
			name:      "within limit",
			maxNodes:  10,
			validated: 10,
			limits: MockLimits{
				MaxFlameGraphNodesMaxValue: 10,
			},
		},
		{
			name:     "limit exceeded",
			maxNodes: 10,
			limits: MockLimits{
				MaxFlameGraphNodesMaxValue: 5,
			},
			err: &Error{Reason: "flamegraph_limit", msg: "max flamegraph nodes limit 10 is greater than allowed 5"},
		},
		{
			name:      "limit disabled",
			maxNodes:  -1,
			validated: -1,
			limits:    MockLimits{},
		},
		{
			name:     "limit disabled with max set",
			maxNodes: -1,
			limits: MockLimits{
				MaxFlameGraphNodesMaxValue: 5,
			},
			err: &Error{Reason: "flamegraph_limit", msg: "max flamegraph nodes limit must be set (max allowed 5)"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			v, err := ValidateMaxNodes(tc.limits, []string{"tenant"}, tc.maxNodes)
			require.Equal(t, tc.err, err)
			require.Equal(t, tc.validated, v)
		})
	}
}
