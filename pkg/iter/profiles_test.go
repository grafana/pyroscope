package iter

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	firemodel "github.com/grafana/fire/pkg/model"
)

var (
	aLabels = firemodel.LabelsFromStrings("foo", "a")
	bLabels = firemodel.LabelsFromStrings("foo", "b")
	cLabels = firemodel.LabelsFromStrings("foo", "c")
)

type profile struct {
	labels    firemodel.Labels
	timestamp int64
}

func (p profile) Labels() firemodel.Labels {
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
