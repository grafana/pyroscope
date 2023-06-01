package iter

import (
	"math"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	phlaremodel "github.com/grafana/phlare/pkg/model"
)

var (
	aLabels = phlaremodel.LabelsFromStrings("foo", "a")
	bLabels = phlaremodel.LabelsFromStrings("foo", "b")
	cLabels = phlaremodel.LabelsFromStrings("foo", "c")
)

type profile struct {
	labels    phlaremodel.Labels
	timestamp model.Time
}

func (p profile) Labels() phlaremodel.Labels {
	return p.labels
}

func (p profile) Timestamp() model.Time {
	return p.timestamp
}

func TestMergeIterator(t *testing.T) {
	for _, tt := range []struct {
		name        string
		deduplicate bool
		input       [][]profile
		expected    []profile
	}{
		{
			name:        "deduplicate exact",
			deduplicate: true,
			input: [][]profile{
				{
					{labels: aLabels, timestamp: 1},
					{labels: aLabels, timestamp: 2},
					{labels: aLabels, timestamp: 3},
				},
				{
					{labels: aLabels, timestamp: 1},
					{labels: aLabels, timestamp: 2},
					{labels: aLabels, timestamp: 3},
				},
				{
					{labels: aLabels, timestamp: 1},
					{labels: aLabels, timestamp: 2},
					{labels: aLabels, timestamp: 3},
				},
			},
			expected: []profile{
				{labels: aLabels, timestamp: 1},
				{labels: aLabels, timestamp: 2},
				{labels: aLabels, timestamp: 3},
			},
		},
		{
			name: "no deduplicate",
			input: [][]profile{
				{
					{labels: aLabels, timestamp: 1},
					{labels: aLabels, timestamp: 2},
					{labels: aLabels, timestamp: 3},
				},
				{
					{labels: aLabels, timestamp: 1},
					{labels: aLabels, timestamp: 3},
				},
				{
					{labels: aLabels, timestamp: 2},
				},
			},
			expected: []profile{
				{labels: aLabels, timestamp: 1},
				{labels: aLabels, timestamp: 1},
				{labels: aLabels, timestamp: 2},
				{labels: aLabels, timestamp: 2},
				{labels: aLabels, timestamp: 3},
				{labels: aLabels, timestamp: 3},
			},
		},
		{
			name:        "deduplicate and sort",
			deduplicate: true,
			input: [][]profile{
				{
					{labels: aLabels, timestamp: 1},
					{labels: aLabels, timestamp: 2},
					{labels: aLabels, timestamp: 3},
					{labels: aLabels, timestamp: 4},
				},
				{
					{labels: aLabels, timestamp: 1},
					{labels: cLabels, timestamp: 2},
					{labels: aLabels, timestamp: 3},
				},
				{
					{labels: aLabels, timestamp: 2},
					{labels: bLabels, timestamp: 4},
				},
			},
			expected: []profile{
				{labels: aLabels, timestamp: 1},
				{labels: aLabels, timestamp: 2},
				{labels: cLabels, timestamp: 2},
				{labels: aLabels, timestamp: 3},
				{labels: aLabels, timestamp: 4},
				{labels: bLabels, timestamp: 4},
			},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			iters := make([]Iterator[profile], len(tt.input))
			for i, input := range tt.input {
				iters[i] = NewSliceIterator(input)
			}
			it := NewMergeIterator(
				profile{timestamp: math.MaxInt64},
				tt.deduplicate,
				iters...)
			actual := []profile{}
			for it.Next() {
				actual = append(actual, it.At())
			}
			require.NoError(t, it.Err())
			require.NoError(t, it.Close())
			require.Equal(t, tt.expected, actual)
		})
	}
}

func Test_BufferedIterator(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
		in   []profile
	}{
		{
			name: "empty",
			size: 1,
			in:   nil,
		},
		{
			name: "smaller than buffer",
			size: 1000,
			in:   generatesProfiles(t, 100),
		},
		{
			name: "bigger than buffer",
			size: 10,
			in:   generatesProfiles(t, 100),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := Slice(
				NewBufferedIterator(
					NewSliceIterator(tc.in), tc.size),
			)
			require.NoError(t, err)
			require.Equal(t, tc.in, actual)
		})
	}
}

func generatesProfiles(t *testing.T, n int) []profile {
	t.Helper()
	profiles := make([]profile, n)
	for i := range profiles {
		profiles[i] = profile{labels: aLabels, timestamp: model.Time(i)}
	}
	return profiles
}

// todo test timedRangeIterator
