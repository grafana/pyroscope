package metadata

import (
	"bytes"
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func TestMetadata_New(t *testing.T) {
	blockID := ulid.MustNew(123, bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11})).String()
	strings := NewStringTable()
	md := &metastorev1.BlockMeta{
		FormatVersion:   0,
		Id:              blockID,
		Tenant:          0,
		Shard:           1,
		CompactionLevel: 0,
		MinTime:         123,
		MaxTime:         456,
		CreatedBy:       strings.Put("ingester-a"),
		Size:            567,
		Datasets:        nil,
		StringTable:     nil,
	}

	b := NewLabelBuilder(strings)
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		for _, dataset := range []string{"service_a", "service_b"} {
			ds := &metastorev1.Dataset{
				Tenant:          strings.Put(tenant),
				Name:            strings.Put(dataset),
				MinTime:         123,
				MaxTime:         456,
				TableOfContents: []uint64{1, 2, 3},
				Size:            567,
				Labels:          nil,
			}
			for _, n := range []string{"cpu", "memory"} {
				b.WithLabelSet("service_name", dataset, "__profile_type__", n)
			}
			ds.Labels = b.Build()
			md.Datasets = append(md.Datasets, ds)
		}
	}
	md.StringTable = strings.Strings

	expected := &metastorev1.BlockMeta{
		FormatVersion:   0,
		Id:              "000000003V000G40R40M30E209",
		Tenant:          0,
		Shard:           1,
		CompactionLevel: 0,
		MinTime:         123,
		MaxTime:         456,
		CreatedBy:       1,
		Size:            567,
		Datasets: []*metastorev1.Dataset{
			{
				Tenant:          2,
				Name:            3,
				MinTime:         123,
				MaxTime:         456,
				TableOfContents: []uint64{1, 2, 3},
				Size:            567,
				Labels:          []int32{2, 4, 3, 5, 6, 2, 4, 3, 5, 7},
			},
			{
				Tenant:          2,
				Name:            8,
				MinTime:         123,
				MaxTime:         456,
				TableOfContents: []uint64{1, 2, 3},
				Size:            567,
				Labels:          []int32{2, 4, 8, 5, 6, 2, 4, 8, 5, 7},
			},
			{
				Tenant:          9,
				Name:            3,
				MinTime:         123,
				MaxTime:         456,
				TableOfContents: []uint64{1, 2, 3},
				Size:            567,
				Labels:          []int32{2, 4, 3, 5, 6, 2, 4, 3, 5, 7},
			},
			{
				Tenant:          9,
				Name:            8,
				MinTime:         123,
				MaxTime:         456,
				TableOfContents: []uint64{1, 2, 3},
				Size:            567,
				Labels:          []int32{2, 4, 8, 5, 6, 2, 4, 8, 5, 7},
			},
		},
		StringTable: []string{
			"", "ingester-a",
			"tenant-a", "service_a", "service_name", "__profile_type__", "cpu", "memory",
			"service_b", "tenant-b",
		},
	}

	assert.Equal(t, expected, md)
}

func TestMetadataStrings_Import(t *testing.T) {
	md1 := &metastorev1.BlockMeta{
		Id:        "block_id",
		Tenant:    0,
		CreatedBy: 1,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 2, Name: 3, Labels: []int32{2, 10, 3, 11, 4, 2, 10, 3, 11, 5, 2, 10, 3, 11, 6}},
			{Tenant: 7, Name: 8, Labels: []int32{2, 10, 8, 11, 5, 2, 10, 8, 11, 6, 2, 10, 8, 11, 9}},
		},
		StringTable: []string{
			"", "ingester",
			"tenant-a", "dataset-a", "1", "2", "3",
			"tenant-b", "dataset-b", "4",
			"service_name", "__profile_type__",
		},
	}

	table := NewStringTable()
	md1c := md1.CloneVT()
	table.Import(md1c)
	assert.Equal(t, md1, md1c)
	assert.Equal(t, table.Strings, md1.StringTable)

	// Exactly the same metadata.
	md2 := md1.CloneVT()
	table.Import(md2)
	assert.Len(t, md2.StringTable, 12)
	assert.Len(t, table.Strings, 12)
	assert.Equal(t, table.Strings, md2.StringTable)

	md3 := &metastorev1.BlockMeta{
		Id:        "block_id_3",
		Tenant:    0,
		CreatedBy: 1,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 2, Name: 3, Labels: []int32{2, 10, 3, 11, 4, 2, 10, 3, 11, 5, 2, 10, 3, 11, 6}},
			{Tenant: 7, Name: 8, Labels: []int32{2, 10, 8, 11, 4, 2, 10, 8, 11, 9}},
		},
		StringTable: []string{
			"", "ingester",
			"tenant-a", "dataset-a", "1", "2", "3",
			"tenant-c", "dataset-c", "5",
			"service_name", "__profile_type__",
		},
	}

	table.Import(md3)
	expected := &metastorev1.BlockMeta{
		Id:        "block_id_3",
		Tenant:    0,
		CreatedBy: 1,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 2, Name: 3, Labels: []int32{2, 10, 3, 11, 4, 2, 10, 3, 11, 5, 2, 10, 3, 11, 6}},
			{Tenant: 12, Name: 13, Labels: []int32{2, 10, 13, 11, 4, 2, 10, 13, 11, 14}},
		},
		StringTable: []string{
			"", "ingester",
			"tenant-a", "dataset-a", "1", "2", "3",
			"tenant-c", "dataset-c", "5",
			"service_name", "__profile_type__",
		},
	}

	assert.Equal(t, expected, md3)
	assert.Len(t, table.Strings, 15)
}

func TestMetadataStrings_Export(t *testing.T) {
	table := NewStringTable()
	for _, s := range []string{
		"", "x1", "x2", "x3", "x4", "x5",
		"ingester",
		"tenant-a", "dataset-a", "1", "2", "3",
		"tenant-b", "dataset-b", "4",
		"service_name", "__profile_type__",
	} {
		table.Put(s)
	}

	md := &metastorev1.BlockMeta{
		Id:        "1",
		Tenant:    0,
		CreatedBy: 6,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 7, Name: 8, Labels: []int32{2, 15, 8, 16, 9, 2, 15, 8, 16, 10, 2, 15, 8, 16, 11}},
			{Tenant: 12, Name: 13, Labels: []int32{2, 15, 13, 16, 10, 2, 15, 13, 16, 11, 2, 15, 13, 16, 14}},
		},
	}

	table.Export(md)

	expected := &metastorev1.BlockMeta{
		Id:        "1",
		Tenant:    0,
		CreatedBy: 1,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 2, Name: 3, Labels: []int32{2, 4, 3, 5, 6, 2, 4, 3, 5, 7, 2, 4, 3, 5, 8}},
			{Tenant: 9, Name: 10, Labels: []int32{2, 4, 10, 5, 7, 2, 4, 10, 5, 8, 2, 4, 10, 5, 11}},
		},
		StringTable: []string{
			"", "ingester",
			"tenant-a", "dataset-a", "service_name", "__profile_type__", "1", "2", "3",
			"tenant-b", "dataset-b", "4",
		},
	}

	assert.Equal(t, expected, md)
}

func TestMetadata_EncodeDecode(t *testing.T) {
	md := &metastorev1.BlockMeta{
		Id:        "1",
		Tenant:    0,
		CreatedBy: 1,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 2, Name: 3, Labels: []int32{2, 4, 3, 5, 6, 2, 4, 3, 5, 7, 2, 4, 3, 5, 8}},
			{Tenant: 9, Name: 10, Labels: []int32{2, 4, 10, 5, 7, 2, 4, 10, 5, 8, 2, 4, 10, 5, 11}},
		},
		StringTable: []string{
			"", "ingester",
			"tenant-a", "dataset-a", "service_name", "__profile_type__", "1", "2", "3",
			"tenant-b", "dataset-b", "4",
		},
	}

	var buf bytes.Buffer
	require.NoError(t, Encode(&buf, md))

	var d metastorev1.BlockMeta
	raw := append([]byte("garbage"), buf.Bytes()...)
	require.NoError(t, Decode(raw, &d))
	assert.Equal(t, md, &d)
}
