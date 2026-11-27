package index

import (
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestQueryPreservesExactSectionMetadata(t *testing.T) {
	offset := uint64(123)
	original := &metastorev1.Dataset{ProfilesFooterSize: 456, SymbolsIndexOffset: &offset}
	data, err := original.MarshalVT()
	require.NoError(t, err)
	var decoded metastorev1.Dataset
	require.NoError(t, decoded.UnmarshalVT(data))
	cloned := cloneDatasetMetadataForQuery(&decoded)
	require.Equal(t, original.ProfilesFooterSize, cloned.ProfilesFooterSize)
	require.Equal(t, original.SymbolsIndexOffset, cloned.SymbolsIndexOffset)
	require.Nil(t, cloneDatasetMetadataForQuery(&metastorev1.Dataset{}).SymbolsIndexOffset)
}
