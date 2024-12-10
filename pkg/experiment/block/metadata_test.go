package block

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func TestMetadataStrings_Import(t *testing.T) {
	md1 := &metastorev1.BlockMeta{
		Id:        "block_id",
		Tenant:    0,
		CreatedBy: 1,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 2, Name: 3, ProfileTypes: []int32{4, 5, 6}},
			{Tenant: 7, Name: 8, ProfileTypes: []int32{5, 6, 9}},
		},
		StringTable: []string{
			"", "ingester",
			"tenant-a", "dataset-a", "1", "2", "3",
			"tenant-b", "dataset-b", "4",
		},
	}

	table := NewMetadataStringTable()
	md1c := md1.CloneVT()
	table.Import(md1c)
	assert.Equal(t, md1, md1c)
	assert.Equal(t, table.Strings, md1.StringTable)

	// Exactly the same metadata.
	md2 := md1.CloneVT()
	table.Import(md2)
	assert.Len(t, md2.StringTable, 10)
	assert.Len(t, table.Strings, 10)
	assert.Equal(t, table.Strings, md2.StringTable)

	md3 := &metastorev1.BlockMeta{
		Id:        "block_id_3",
		Tenant:    0,
		CreatedBy: 1,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 2, Name: 3, ProfileTypes: []int32{4, 5, 6}},
			{Tenant: 7, Name: 8, ProfileTypes: []int32{4, 9}},
		},
		StringTable: []string{
			"", "ingester",
			"tenant-a", "dataset-a", "1", "2", "3",
			"tenant-c", "dataset-c", "5",
		},
	}

	table.Import(md3)
	expected := &metastorev1.BlockMeta{
		Id:        "block_id_3",
		Tenant:    0,
		CreatedBy: 1,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 2, Name: 3, ProfileTypes: []int32{4, 5, 6}},
			{Tenant: 10, Name: 11, ProfileTypes: []int32{4, 12}},
		},
		StringTable: []string{
			"", "ingester",
			"tenant-a", "dataset-a", "1", "2", "3",
			"tenant-c", "dataset-c", "5",
		},
	}

	assert.Equal(t, expected, md3)
	assert.Len(t, table.Strings, 13)
}

func TestMetadataStrings_Export(t *testing.T) {
	table := NewMetadataStringTable()
	for _, s := range []string{
		"", "x1", "x2", "x3", "x4", "x5",
		"ingester",
		"tenant-a", "dataset-a", "1", "2", "3",
		"tenant-b", "dataset-b", "4",
	} {
		table.Put(s)
	}

	md := &metastorev1.BlockMeta{
		Id:        "1",
		Tenant:    0,
		CreatedBy: 6,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 7, Name: 8, ProfileTypes: []int32{9, 10, 11}},
			{Tenant: 12, Name: 13, ProfileTypes: []int32{10, 11, 14}},
		},
	}

	table.Export(md)

	expected := &metastorev1.BlockMeta{
		Id:        "1",
		Tenant:    0,
		CreatedBy: 1,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 2, Name: 3, ProfileTypes: []int32{4, 5, 6}},
			{Tenant: 7, Name: 8, ProfileTypes: []int32{5, 6, 9}},
		},
		StringTable: []string{
			"", "ingester",
			"tenant-a", "dataset-a", "1", "2", "3",
			"tenant-b", "dataset-b", "4",
		},
	}

	assert.Equal(t, expected, md)
}
