package block

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/testutil"
)

type contextRecordingBucket struct {
	objstore.Bucket
	readerContext context.Context
}

func (b *contextRecordingBucket) ReaderAt(ctx context.Context, name string) (objstore.ReaderAtCloser, error) {
	b.readerContext = ctx
	return b.Bucket.ReaderAt(ctx, name)
}

func Test_openParquetFile_ReaderContext(t *testing.T) {
	for _, mode := range []parquet.ReadMode{parquet.ReadModeSync, parquet.ReadModeAsync} {
		name := "sync"
		if mode == parquet.ReadModeAsync {
			name = "async"
		}
		t.Run(name, func(t *testing.T) {
			base, _ := testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())
			bucket := &contextRecordingBucket{Bucket: base}
			var buf bytes.Buffer
			createParquetTestFile(t, &buf, 100)
			require.NoError(t, bucket.Upload(context.Background(), "test.parquet", bytes.NewReader(buf.Bytes())))

			sc := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    trace.TraceID{1},
				SpanID:     trace.SpanID{2},
				TraceFlags: trace.FlagsSampled,
			})
			parent, cancel := context.WithCancel(trace.ContextWithSpanContext(context.Background(), sc))
			t.Cleanup(cancel)
			pf, err := openParquetFile(parent, bucket, "test.parquet", 0, int64(buf.Len()), 0,
				parquet.FileReadMode(mode), parquet.SkipBloomFilters(true))
			require.NoError(t, err)
			closed := false
			t.Cleanup(func() {
				if !closed {
					require.NoError(t, pf.Close())
				}
			})
			require.Equal(t, sc, trace.SpanContextFromContext(bucket.readerContext))

			// Model cancellation when the section-opening errgroup returns.
			cancel()
			require.NoError(t, bucket.readerContext.Err())
			reader := parquet.NewGenericReader[struct{ A int }](pf.File)
			rows := make([]struct{ A int }, 100)
			n, err := reader.Read(rows)
			if err != io.EOF {
				require.NoError(t, err)
			}
			require.Equal(t, 100, n)
			for i, row := range rows {
				require.Equal(t, i, row.A)
			}
			require.NoError(t, reader.Close())
			require.NoError(t, pf.Close())
			closed = true
			require.ErrorIs(t, bucket.readerContext.Err(), context.Canceled)
		})
	}
}
