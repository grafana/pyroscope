package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
)

type replayTestSymbols struct {
	fetches  int
	releases int
}

func (s *replayTestSymbols) Partition(context.Context, uint64) (symdb.PartitionReader, error) {
	s.fetches++
	return s, nil
}

func (*replayTestSymbols) WriteStats(*symdb.PartitionStats) {}
func (*replayTestSymbols) Symbols() *symdb.Symbols          { return &symdb.Symbols{} }
func (s *replayTestSymbols) Release()                       { s.releases++ }

func TestReplaySymbolsRetainsPartitions(t *testing.T) {
	source := &replayTestSymbols{}
	cache := &replaySymbols{source: source, partitions: make(map[uint64]symdb.PartitionReader)}
	for _, id := range []uint64{1, 2, 1, 2, 1} {
		p, err := cache.Partition(context.Background(), id)
		require.NoError(t, err)
		p.Release()
	}
	require.Equal(t, 2, source.fetches)
	require.Zero(t, source.releases)
	cache.Close()
	require.Equal(t, 2, source.releases)
}

func TestAssembleReplay(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	// More than the descriptor cache limit, with large records that force
	// buffered writes to flush while offsets are being tracked.
	paths := make([]string, 40)
	var records []replayRecordOffset
	payload := bytes.Repeat([]byte("profile"), 2000)
	for i := range paths {
		paths[i] = filepath.Join(dir, fmt.Sprint(i))
		f, err := os.Create(paths[i])
		require.NoError(t, err)
		counter := &replayCountingWriter{Writer: f}
		w, err := newReplayWriter(counter, replayHeader{})
		require.NoError(t, err)
		spool := &replaySpool{writer: w, counter: counter, block: i}
		require.NoError(t, spool.WriteRecord(replayRecord{TimestampNanos: int64(i), Pprof: payload}))
		require.NoError(t, w.Flush())
		require.NoError(t, f.Close())
		records = append(records, spool.records...)
	}
	var buf bytes.Buffer
	w, err := newReplayWriter(&buf, replayHeader{})
	require.NoError(t, err)
	// Revisit evicted files as well as exercising the initial eviction.
	records = append(records, records...)
	require.NoError(t, assembleReplay(ctx, paths, records, w))
	require.NoError(t, w.Flush())
	r, err := newReplayReader(&buf)
	require.NoError(t, err)
	for _, offset := range records {
		rec, err := r.ReadRecord()
		require.NoError(t, err)
		require.Equal(t, offset.timestamp, rec.TimestampNanos)
		require.Equal(t, payload, rec.Pprof)
	}
	_, err = r.ReadRecord()
	require.ErrorIs(t, err, io.EOF)

	require.NoError(t, os.Truncate(paths[0], 0))
	require.ErrorIs(t, assembleReplay(ctx, paths, records[:1], w), io.ErrUnexpectedEOF)
}
