package querybackend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
)

func Test_strippedProfileFilter(t *testing.T) {
	kept := phlaremodel.Labels{}
	stripped := phlaremodel.LabelsFromStrings(phlaremodel.LabelNameSampled, "true")

	entries := []ProfileEntry{
		{RowNum: 0, Labels: kept},
		{RowNum: 1, Labels: stripped},
		{RowNum: 2, Labels: stripped},
		{RowNum: 3, Labels: kept},
		{RowNum: 4, Labels: stripped},
	}

	f := newStrippedProfileFilter(iter.NewSliceIterator(entries))
	var rows []int64
	for f.Next() {
		rows = append(rows, f.At().RowNum)
	}
	require.NoError(t, f.Err())
	require.NoError(t, f.Close())

	assert.Equal(t, []int64{0, 3}, rows)
	assert.Equal(t, &typesv1.ProfileSampling{
		Sampled:          true,
		KeptProfiles:     2,
		StrippedProfiles: 3,
	}, f.sampling())
}

func Test_strippedProfileFilter_NoStripped(t *testing.T) {
	entries := []ProfileEntry{
		{RowNum: 0, Labels: phlaremodel.Labels{}},
		{RowNum: 1, Labels: phlaremodel.Labels{}},
	}

	f := newStrippedProfileFilter(iter.NewSliceIterator(entries))
	n := 0
	for f.Next() {
		n++
	}
	require.NoError(t, f.Err())

	assert.Equal(t, 2, n)
	assert.Equal(t, &typesv1.ProfileSampling{
		Sampled:          false,
		KeptProfiles:     2,
		StrippedProfiles: 0,
	}, f.sampling())
}

func Test_samplingAggregator(t *testing.T) {
	var a samplingAggregator
	assert.Nil(t, a.build(), "no partial carried sampling")

	a.aggregate(nil)
	assert.Nil(t, a.build(), "nil partials do not count as present")

	a.aggregate(&typesv1.ProfileSampling{Sampled: false, KeptProfiles: 2})
	a.aggregate(&typesv1.ProfileSampling{Sampled: true, KeptProfiles: 3, StrippedProfiles: 7})
	a.aggregate(nil)

	assert.Equal(t, &typesv1.ProfileSampling{
		Sampled:          true,
		KeptProfiles:     5,
		StrippedProfiles: 7,
	}, a.build())
}
