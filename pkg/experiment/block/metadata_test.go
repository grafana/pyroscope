package block

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func TestMetadataStrings_Import(t *testing.T) {
	md1 := &metastorev1.BlockMeta{
		Id:        1,
		Tenant:    0,
		CreatedBy: 2,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 3, Name: 4, ProfileTypes: []int32{5, 6, 7}},
			{Tenant: 8, Name: 9, ProfileTypes: []int32{6, 7, 10}},
		},
		StringTable: []string{
			"", "id", "ingester",
			"tenant-a", "dataset-a", "1", "2", "3",
			"tenant-b", "dataset-b", "4",
		},
	}

	table := NewMetadataStringTable()

	md1c := md1.CloneVT()
	table.Import(md1c)
	assert.Equal(t, md1, md1c)

	md2 := md1.CloneVT()
	table.Import(md2)
	assert.Equal(t, 1, len(md2.StringTable))
	assert.Equal(t, 11, cap(md2.StringTable))

	md3 := &metastorev1.BlockMeta{
		Id:        1,
		Tenant:    0,
		CreatedBy: 2,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 3, Name: 4, ProfileTypes: []int32{5, 6, 7}},
			{Tenant: 8, Name: 9, ProfileTypes: []int32{5, 10}},
		},
		StringTable: []string{
			"", "id", "ingester",
			"tenant-a", "dataset-a", "1", "2", "3",
			"tenant-c", "dataset-c", "5",
		},
	}

	table.Import(md3)
	expected := &metastorev1.BlockMeta{
		Id:        1,
		Tenant:    0,
		CreatedBy: 2,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 3, Name: 4, ProfileTypes: []int32{5, 6, 7}},
			{Tenant: 11, Name: 12, ProfileTypes: []int32{5, 13}},
		},
		StringTable: []string{"", "tenant-c", "dataset-c", "5"},
	}

	assert.Equal(t, expected, md3)
}

func TestMetadataStrings_Export(t *testing.T) {
	table := NewMetadataStringTable()
	for _, s := range []string{
		"", "x1", "x2", "x3", "x4",
		"id", "ingester",
		"tenant-a", "dataset-a", "1", "2", "3",
		"tenant-b", "dataset-b", "4",
	} {
		table.Put(s)
	}

	md := &metastorev1.BlockMeta{
		Id:        5,
		Tenant:    0,
		CreatedBy: 6,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 7, Name: 8, ProfileTypes: []int32{9, 10, 11}},
			{Tenant: 12, Name: 13, ProfileTypes: []int32{10, 11, 14}},
		},
	}

	table.Export(md)

	expected := &metastorev1.BlockMeta{
		Id:        1,
		Tenant:    0,
		CreatedBy: 2,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 3, Name: 4, ProfileTypes: []int32{5, 6, 7}},
			{Tenant: 8, Name: 9, ProfileTypes: []int32{6, 7, 10}},
		},
		StringTable: []string{
			"", "id", "ingester",
			"tenant-a", "dataset-a", "1", "2", "3",
			"tenant-b", "dataset-b", "4",
		},
	}

	assert.Equal(t, expected, md)
}
