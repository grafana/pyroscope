package index

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

const benchmarkOutOfRangeBlocks = 1000

func BenchmarkQueryMetadataLargeOutOfRange(b *testing.B) {
	benchmarkLargeOutOfRangeQuery(b, func(idx *Index, tx *bbolt.Tx, query MetadataQuery) error {
		blocks, err := idx.QueryMetadata(tx, context.Background(), query)
		if err != nil {
			return err
		}
		if len(blocks) != 1 {
			return fmt.Errorf("got %d blocks, want 1", len(blocks))
		}
		return nil
	})
}

func BenchmarkQueryMetadataLabelsLargeOutOfRange(b *testing.B) {
	benchmarkLargeOutOfRangeQuery(b, func(idx *Index, tx *bbolt.Tx, query MetadataQuery) error {
		labels, err := idx.QueryMetadataLabels(tx, context.Background(), query)
		if err != nil {
			return err
		}
		if len(labels) == 0 {
			return fmt.Errorf("got no label sets")
		}
		return nil
	})
}

func benchmarkLargeOutOfRangeQuery(b *testing.B, query func(*Index, *bbolt.Tx, MetadataQuery) error) {
	b.Helper()

	db := test.BoltDB(b)
	config := DefaultConfig
	config.BlockReadCacheSize = benchmarkOutOfRangeBlocks + 1
	config.BlockWriteCacheSize = benchmarkOutOfRangeBlocks + 1
	writer := NewIndex(util.Logger, NewStore(), config, nil)
	requireNoError(b, db.Update(writer.Init))

	queryStart := time.Date(2024, 9, 23, 8, 0, 0, 0, time.UTC)
	requireNoError(b, db.Update(func(tx *bbolt.Tx) error {
		if err := writer.InsertBlock(tx, benchmarkBlock(queryStart)); err != nil {
			return err
		}
		for range benchmarkOutOfRangeBlocks {
			if err := writer.InsertBlock(tx, benchmarkBlock(queryStart.Add(2*time.Hour))); err != nil {
				return err
			}
		}
		return nil
	}))

	metadataQuery := MetadataQuery{
		Expr:      `{}`,
		StartTime: queryStart,
		EndTime:   queryStart.Add(15 * time.Minute),
		Tenant:    []string{"tenant-a"},
		Labels:    []string{"service_name"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		// Recreate the index to measure a cold block cache every iteration.
		reader := NewIndex(util.Logger, NewStore(), config, nil)
		requireNoError(b, db.Update(reader.Init))
		b.StartTimer()
		requireNoError(b, db.View(func(tx *bbolt.Tx) error {
			return query(reader, tx, metadataQuery)
		}))
	}
}

func benchmarkBlock(timestamp time.Time) *metastorev1.BlockMeta {
	const (
		datasetsPerBlock = 10
		labelPairsPerSet = 50
	)

	labels := make([]int32, 0, labelPairsPerSet*2)
	for range labelPairsPerSet {
		labels = append(labels, 3, 4)
	}
	block := &metastorev1.BlockMeta{
		Id:      ulid.MustNew(ulid.Timestamp(timestamp), rand.Reader).String(),
		Tenant:  1,
		MinTime: timestamp.UnixMilli(),
		MaxTime: timestamp.Add(15 * time.Minute).UnixMilli(),
		StringTable: []string{
			"", "tenant-a", "dataset-a", "service_name", "service-a",
		},
	}
	for range datasetsPerBlock {
		block.Datasets = append(block.Datasets, &metastorev1.Dataset{
			Tenant:  1,
			Name:    2,
			MinTime: block.MinTime,
			MaxTime: block.MaxTime,
			Labels:  labels,
		})
	}
	return block
}

func requireNoError(b *testing.B, err error) {
	b.Helper()
	if err != nil {
		b.Fatal(err)
	}
}
