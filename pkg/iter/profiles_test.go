package iter

import (
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
	timestamp int64
}

func (p profile) Labels() phlaremodel.Labels {
	return p.labels
}

func (p profile) Timestamp() model.Time {
	return model.Time(p.timestamp)
}

func TestSortProfiles(t *testing.T) {
	it := NewSortProfileIterator([]Iterator[profile]{
		NewSliceIterator([]profile{
			{labels: aLabels, timestamp: 1},
			{labels: aLabels, timestamp: 2},
			{labels: aLabels, timestamp: 3},
		}),
		NewSliceIterator([]profile{
			{labels: bLabels, timestamp: 1},
			{labels: bLabels, timestamp: 2},
			{labels: bLabels, timestamp: 3},
		}),
		NewSliceIterator([]profile{
			{labels: cLabels, timestamp: 1},
			{labels: cLabels, timestamp: 2},
			{labels: cLabels, timestamp: 3},
		}),
	})

	actual := []profile{}
	for it.Next() {
		actual = append(actual, it.At())
	}
	require.NoError(t, it.Err())
	require.NoError(t, it.Close())
	require.Equal(t, []profile{
		{labels: aLabels, timestamp: 1},
		{labels: bLabels, timestamp: 1},
		{labels: cLabels, timestamp: 1},
		{labels: aLabels, timestamp: 2},
		{labels: bLabels, timestamp: 2},
		{labels: cLabels, timestamp: 2},
		{labels: aLabels, timestamp: 3},
		{labels: bLabels, timestamp: 3},
		{labels: cLabels, timestamp: 3},
	}, actual)
}

// todo test timedRangeIterator
