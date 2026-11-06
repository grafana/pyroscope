package main

import (
	"bytes"
	"context"
	"io"
	"math"
	"os"
	"testing"

	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/objstore/testutil"
)

func TestDumpBlock_MultipleDatasets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	bucket, _ := testutil.NewFilesystemBucket(t, ctx, "../../pkg/block/testdata")

	raw, err := os.ReadFile("../../pkg/block/testdata/block-metas.json")
	require.NoError(t, err)
	var resp metastorev1.GetBlockMetadataResponse
	require.NoError(t, protojson.Unmarshal(raw, &resp))

	var md *metastorev1.BlockMeta
	for _, b := range resp.Blocks {
		var n int
		for _, ds := range b.Datasets {
			if block.DatasetFormat(ds.Format) == block.DatasetFormat0 {
				n++
			}
		}
		if n > 1 {
			md = b
			break
		}
	}
	require.NotNil(t, md, "test data must contain a block with more than one profile dataset")

	var buf bytes.Buffer
	rw, err := newReplayWriter(&buf, replayHeader{})
	require.NoError(t, err)

	count, err := dumpBlock(ctx, bucket, md, nil, 0, math.MaxInt64, rw, "")
	require.NoError(t, err)
	assert.NotZero(t, count)
	require.NoError(t, rw.Flush())

	// Compare the parallel, disk-backed path with the original row stream.
	// Repeating a block exercises overlapping timestamps and worker ordering.
	want := make(map[int64][][]byte)
	rr, err := newReplayReader(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	for {
		rec, err := rr.ReadRecord()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		want[rec.TimestampNanos] = append(want[rec.TimestampNanos], rec.Pprof, rec.Pprof)
	}

	var sorted bytes.Buffer
	writer, err := newReplayWriter(&sorted, replayHeader{})
	require.NoError(t, err)
	dir := t.TempDir()
	n, err := dumpBlocks(ctx, bucket, []*metastorev1.BlockMeta{md, md}, nil, 0, math.MaxInt64, writer, dir, "")
	require.NoError(t, err)
	require.Equal(t, count*2, n)
	require.NoError(t, writer.Flush())
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, files, "block spools must be removed")
	rr, err = newReplayReader(&sorted)
	require.NoError(t, err)
	got := make(map[int64][][]byte)
	previous := int64(math.MinInt64)
	for {
		rec, err := rr.ReadRecord()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.GreaterOrEqual(t, rec.TimestampNanos, previous)
		previous = rec.TimestampNanos
		got[rec.TimestampNanos] = append(got[rec.TimestampNanos], rec.Pprof)
	}
	require.Len(t, got, len(want))
	for timestamp, profiles := range want {
		require.ElementsMatch(t, profiles, got[timestamp])
	}

	var anonymized bytes.Buffer
	anonWriter, err := newReplayWriter(&anonymized, replayHeader{})
	require.NoError(t, err)
	anonymizer := replayAnonymizer("integration-test-salt")
	n, err = dumpBlocks(ctx, bucket, []*metastorev1.BlockMeta{md}, nil, 0, math.MaxInt64, anonWriter, dir, anonymizer)
	require.NoError(t, err)
	require.Equal(t, count, n)
	require.NoError(t, anonWriter.Flush())
	anonReader, err := newReplayReader(&anonymized)
	require.NoError(t, err)
	for range n {
		rec, err := anonReader.ReadRecord()
		require.NoError(t, err)
		for _, l := range rec.Labels {
			if !replayProfileTypeLabel(l.Name) {
				if l.Name != phlaremodel.LabelNameServiceName {
					require.Regexp(t, `^_[0-9a-f]{64}$`, l.Name)
				}
				require.Regexp(t, `^[0-9a-f]{64}$`, l.Value)
			}
		}
		p, err := profile.ParseData(rec.Pprof)
		require.NoError(t, err)
		require.NoError(t, p.CheckValid())
		require.Equal(t, rec.TimestampNanos, p.TimeNanos)
		pt, err := phlaremodel.ParseProfileTypeSelector(phlaremodel.Labels(rec.Labels).Get(phlaremodel.LabelNameProfileType))
		require.NoError(t, err)
		require.Equal(t, pt.SampleType, p.SampleType[0].Type)
		require.Equal(t, pt.SampleUnit, p.SampleType[0].Unit)
		for _, f := range p.Function {
			for _, s := range []string{f.Name, f.SystemName, f.Filename} {
				if s != "" {
					require.Regexp(t, `^[0-9a-f]{64}$`, s)
				}
			}
		}
		for _, m := range p.Mapping {
			for _, s := range []string{m.File, m.BuildID} {
				if s != "" {
					require.Regexp(t, `^[0-9a-f]{64}$`, s)
				}
			}
		}
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = dumpBlocks(cancelled, bucket, []*metastorev1.BlockMeta{md}, nil, 0, math.MaxInt64, writer, dir, "")
	require.ErrorIs(t, err, context.Canceled)
	files, err = os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, files)
}
