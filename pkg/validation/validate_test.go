package validation

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
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
			},
		},
		{
			name:           "empty labels",
			lbs:            []*typesv1.LabelPair{},
			expectedErr:    `error at least one label pair is required per profile`,
			expectedReason: MissingLabels,
		},
		{
			name:           "max labels",
			lbs:            []*typesv1.LabelPair{{Name: "foo", Value: "bar"}, {Name: "foo1", Value: "bar"}, {Name: "foo2", Value: "bar"}, {Name: "foo3", Value: "bar"}, {Name: "foo4", Value: "bar"}},
			expectedErr:    `profile series '{foo="bar", foo1="bar", foo2="bar", foo3="bar", foo4="bar"}' has 5 label names; limit 3`,
			expectedReason: MaxLabelNamesPerSeries,
		},
		{
			name:           "invalid metric name",
			lbs:            []*typesv1.LabelPair{{Name: model.MetricNameLabel, Value: "&&"}},
			expectedErr:    `invalid labels '{__name__="&&"}' with error: invalid metric name`,
			expectedReason: InvalidLabels,
		},
		{
			name:           "invalid label value",
			lbs:            []*typesv1.LabelPair{{Name: model.MetricNameLabel, Value: "qux"}, {Name: "foo", Value: "\xc5"}},
			expectedErr:    "invalid labels '{__name__=\"qux\", foo=\"\\xc5\"}' with error: invalid label value '\xc5'",
			expectedReason: InvalidLabels,
		},
		{
			name:           "invalid label name",
			lbs:            []*typesv1.LabelPair{{Name: model.MetricNameLabel, Value: "qux"}, {Name: "\xc5", Value: "foo"}},
			expectedErr:    "invalid labels '{__name__=\"qux\", \xc5=\"foo\"}' with error: invalid label name '\xc5'",
			expectedReason: InvalidLabels,
		},
		{
			name: "name too long",
			lbs: []*typesv1.LabelPair{
				{Name: "foooooooooooooooo", Value: "bar"},
				{Name: model.MetricNameLabel, Value: "qux"},
			},
			expectedReason: LabelNameTooLong,
			expectedErr:    "profile with labels '{__name__=\"qux\", foooooooooooooooo=\"bar\"}' has label name too long: 'foooooooooooooooo'",
		},
		{
			name: "value too long",
			lbs: []*typesv1.LabelPair{
				{Name: "foo", Value: "barrrrrrrrrrrrrrr"},
				{Name: model.MetricNameLabel, Value: "qux"},
			},
			expectedReason: LabelValueTooLong,
			expectedErr:    `profile with labels '{__name__="qux", foo="barrrrrrrrrrrrrrr"}' has label value too long: 'barrrrrrrrrrrrrrr'`,
		},

		{
			name: "dupe",
			lbs: []*typesv1.LabelPair{
				{Name: "foo", Value: "bar"},
				{Name: "foo", Value: "bar"},
				{Name: model.MetricNameLabel, Value: "qux"},
			},
			expectedReason: DuplicateLabelNames,
			expectedErr:    "profile with labels '{__name__=\"qux\", foo=\"bar\", foo=\"bar\"}' has duplicate label name: 'foo'",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLabels(MockLimits{
				MaxLabelNamesPerSeriesValue: 3,
				MaxLabelNameLengthValue:     10,
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
