package query

import (
	"testing"

	"github.com/grafana/fire/pkg/iter"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	firemodel "github.com/grafana/fire/pkg/model"
)

var (
	aLabels = firemodel.LabelsFromStrings("foo", "a")
	bLabels = firemodel.LabelsFromStrings("foo", "b")
	cLabels = firemodel.LabelsFromStrings("foo", "c")
)

func TestSortProfiles(t *testing.T) {
	it := NewSortIterator([]iter.Iterator[Profile]{
		iter.NewSliceIterator([]Profile{
			{Labels: aLabels, Fingerprint: model.Fingerprint(aLabels.Hash()), Timestamp: 1},
			{Labels: aLabels, Fingerprint: model.Fingerprint(aLabels.Hash()), Timestamp: 2},
			{Labels: aLabels, Fingerprint: model.Fingerprint(aLabels.Hash()), Timestamp: 3},
		}),
		iter.NewSliceIterator([]Profile{
			{Labels: bLabels, Fingerprint: model.Fingerprint(bLabels.Hash()), Timestamp: 1},
			{Labels: bLabels, Fingerprint: model.Fingerprint(bLabels.Hash()), Timestamp: 2},
			{Labels: bLabels, Fingerprint: model.Fingerprint(bLabels.Hash()), Timestamp: 3},
		}),
		iter.NewSliceIterator([]Profile{
			{Labels: cLabels, Fingerprint: model.Fingerprint(cLabels.Hash()), Timestamp: 1},
			{Labels: cLabels, Fingerprint: model.Fingerprint(cLabels.Hash()), Timestamp: 2},
			{Labels: cLabels, Fingerprint: model.Fingerprint(cLabels.Hash()), Timestamp: 3},
		}),
	})

	actual := []Profile{}
	for it.Next() {
		actual = append(actual, it.At())
	}
	require.NoError(t, it.Err())
	require.NoError(t, it.Close())
	require.Equal(t, []Profile{
		{Labels: aLabels, Fingerprint: model.Fingerprint(aLabels.Hash()), Timestamp: 1},
		{Labels: bLabels, Fingerprint: model.Fingerprint(bLabels.Hash()), Timestamp: 1},
		{Labels: cLabels, Fingerprint: model.Fingerprint(cLabels.Hash()), Timestamp: 1},
		{Labels: aLabels, Fingerprint: model.Fingerprint(aLabels.Hash()), Timestamp: 2},
		{Labels: bLabels, Fingerprint: model.Fingerprint(bLabels.Hash()), Timestamp: 2},
		{Labels: cLabels, Fingerprint: model.Fingerprint(cLabels.Hash()), Timestamp: 2},
		{Labels: aLabels, Fingerprint: model.Fingerprint(aLabels.Hash()), Timestamp: 3},
		{Labels: bLabels, Fingerprint: model.Fingerprint(bLabels.Hash()), Timestamp: 3},
		{Labels: cLabels, Fingerprint: model.Fingerprint(cLabels.Hash()), Timestamp: 3},
	}, actual)
}
