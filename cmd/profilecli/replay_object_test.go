package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	phlareobj "github.com/grafana/pyroscope/v2/pkg/objstore"
)

func TestApplyExtraLabels(t *testing.T) {
	t.Parallel()

	in := []*typesv1.LabelPair{
		{Name: "__profile_type__", Value: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"},
		{Name: "service_name", Value: "distributor"},
	}

	assert.Equal(t, in, applyExtraLabels(in, nil))

	got := applyExtraLabels(in, map[string]string{"replay_id": "r1", "service_name": "renamed"})
	require.Len(t, got, 3)
	assert.Equal(t, "__profile_type__", got[0].Name)
	assert.Equal(t, "replay_id", got[1].Name)
	assert.Equal(t, "r1", got[1].Value)
	assert.Equal(t, "service_name", got[2].Name)
	assert.Equal(t, "renamed", got[2].Value)
	assert.Equal(t, "distributor", in[1].Value)
}

func TestBuildSeries_AppliesExtraLabels(t *testing.T) {
	t.Parallel()

	rec := replayRecord{
		Labels:         []*typesv1.LabelPair{{Name: "service_name", Value: "svc"}},
		TimestampNanos: 1,
		Pprof:          testPprofBytes(t),
	}
	series, err := buildSeries(rec, time.Unix(0, 2), map[string]string{"replay_id": "r1"})
	require.NoError(t, err)
	require.Len(t, series.Labels, 2)
	assert.Equal(t, "replay_id", series.Labels[0].Name)
	assert.Equal(t, "service_name", series.Labels[1].Name)
}

func TestValidateExtraLabels(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateExtraLabels(nil))
	require.NoError(t, validateExtraLabels(map[string]string{"replay_id": "x"}))
	require.Error(t, validateExtraLabels(map[string]string{"": "x"}))
	require.Error(t, validateExtraLabels(map[string]string{"replay_id": ""}))
	require.Error(t, validateExtraLabels(map[string]string{"__profile_type__": "x"}))
	require.Error(t, validateExtraLabels(map[string]string{"__custom__": "x"}))
	require.NoError(t, validateExtraLabels(map[string]string{phlaremodel.LabelNameSessionID: "x"}))
}

func TestReplayDumpExists_Object(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	bucket := phlareobj.NewBucket(objstore.NewInMemBucket())
	params := &replayDumpParams{OutputObject: "replays/r1.replay"}

	exists, err := replayDumpExists(ctx, params, bucket)
	require.NoError(t, err)
	assert.False(t, exists)

	src := filepath.Join(t.TempDir(), "dump")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o600))
	require.NoError(t, uploadReplayDump(ctx, bucket, params.OutputObject, src))

	exists, err = replayDumpExists(ctx, params, bucket)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestReplayDump_SkipIfExistsLocal(t *testing.T) {
	t.Parallel()

	out := filepath.Join(t.TempDir(), "x.replay")
	require.NoError(t, os.WriteFile(out, []byte("keep"), 0o600))

	params := &replayDumpParams{
		bucketParams: &bucketParams{},
		Tenant:       "t",
		Query:        "{}",
		From:         "now-1h",
		To:           "now",
		Output:       out,
		SkipIfExists: true,
	}
	require.NoError(t, replayDump(context.Background(), params))
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(got))
}

func TestReplayDump_RequiresExactlyOneOutput(t *testing.T) {
	t.Parallel()

	err := replayDump(context.Background(), &replayDumpParams{bucketParams: &bucketParams{}})
	require.Error(t, err)

	err = replayDump(context.Background(), &replayDumpParams{bucketParams: &bucketParams{}, Output: "a", OutputObject: "b"})
	require.Error(t, err)
}

func TestReplayDump_SkipIfExistsAndForceAreExclusive(t *testing.T) {
	t.Parallel()

	err := replayDump(context.Background(), &replayDumpParams{bucketParams: &bucketParams{}, Output: "a", SkipIfExists: true, Force: true})
	require.ErrorContains(t, err, "mutually exclusive")
}

func TestReplayDumpOutputName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "dump.replay", (&replayDumpParams{Output: "dump.replay"}).outputName())
	assert.Equal(t, "object:replays/r1.replay", (&replayDumpParams{OutputObject: "replays/r1.replay"}).outputName())
}

func TestOpenReplayInputObject_RoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	bucket := phlareobj.NewBucket(objstore.NewInMemBucket())

	var buf bytes.Buffer
	rw, err := newReplayWriter(&buf, replayHeader{SourceQuery: "{}", Tenants: []string{"t"}})
	require.NoError(t, err)
	pprofBytes := testPprofBytes(t)
	require.NoError(t, rw.WriteRecord(replayRecord{
		Labels:         []*typesv1.LabelPair{{Name: "service_name", Value: "svc"}},
		TimestampNanos: 42,
		Pprof:          pprofBytes,
	}))
	require.NoError(t, rw.Flush())
	require.NoError(t, bucket.Upload(ctx, "replays/r1.replay", bytes.NewReader(buf.Bytes())))

	params := &replayPushParams{InputObject: "replays/r1.replay"}
	rr, input, err := params.openReader(ctx, bucket)
	require.NoError(t, err)
	rec, err := rr.ReadRecord()
	require.NoError(t, err)
	assert.Equal(t, int64(42), rec.TimestampNanos)
	assert.Equal(t, pprofBytes, rec.Pprof)
	_, err = rr.ReadRecord()
	assert.ErrorIs(t, err, io.EOF)
	require.NoError(t, input.Close())
	assert.Len(t, input.HexSum(), 64)

	_, _, err = (&replayPushParams{InputObject: "missing"}).openReader(ctx, bucket)
	require.Error(t, err)
}

func TestReplayPush_RequiresExactlyOneInput(t *testing.T) {
	t.Parallel()

	base := replayPushParams{Speed: 1, BatchSize: 1}

	none := base
	require.Error(t, replayPush(context.Background(), &none))

	both := base
	both.Input, both.InputObject = "a", "b"
	require.Error(t, replayPush(context.Background(), &both))
}

func TestReplayBucketFlagsAreIndependent(t *testing.T) {
	app := kingpin.New("test", "")
	cmd := app.Command("c", "")
	source := addBucketParams(cmd)
	replay := addReplayBucketParams(cmd)

	_, err := app.Parse([]string{"c", "--storage.backend=filesystem", "--storage.filesystem.dir=/src", "--replay.storage.backend=filesystem", "--replay.storage.filesystem.dir=/dst"})
	require.NoError(t, err)
	require.Equal(t, "/src", source.objectStoreCfg.Filesystem.Directory)
	require.Equal(t, "/dst", replay.objectStoreCfg.Filesystem.Directory)

	ctx := context.Background()
	_, err = source.initClient(ctx)
	require.NoError(t, err)
	_, err = replay.initClient(ctx)
	require.NoError(t, err)
}

type flakyBucket struct {
	phlareobj.Bucket
	failAfter   int64
	failRanges  bool
	rangeOffset []int64
}

func (b *flakyBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	rc, err := b.Bucket.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return &failingReader{ReadCloser: rc, left: b.failAfter}, nil
}

func (b *flakyBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	b.rangeOffset = append(b.rangeOffset, off)
	if b.failRanges {
		return nil, errors.New("range unavailable")
	}
	return b.Bucket.GetRange(ctx, name, off, length)
}

type failingReader struct {
	io.ReadCloser
	left int64
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.left <= 0 {
		return 0, errors.New("connection reset by peer")
	}
	if int64(len(p)) > r.left {
		p = p[:r.left]
	}
	n, err := r.ReadCloser.Read(p)
	r.left -= int64(n)
	return n, err
}

func TestResumableObjectBody_ResumesAfterReadFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	content := bytes.Repeat([]byte("0123456789"), 1000)
	inner := phlareobj.NewBucket(objstore.NewInMemBucket())
	require.NoError(t, inner.Upload(ctx, "o", bytes.NewReader(content)))
	bucket := &flakyBucket{Bucket: inner, failAfter: 2500}

	rc, err := bucket.Get(ctx, "o")
	require.NoError(t, err)
	body := newResumableObjectBody(ctx, bucket, "o", rc, int64(len(content)))
	body.backoff = time.Millisecond

	got, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, content, got)
	assert.Equal(t, []int64{2500}, bucket.rangeOffset)
}

func TestResumableObjectBody_GivesUpAfterRetries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	content := bytes.Repeat([]byte("0123456789"), 1000)
	inner := phlareobj.NewBucket(objstore.NewInMemBucket())
	require.NoError(t, inner.Upload(ctx, "o", bytes.NewReader(content)))
	bucket := &flakyBucket{Bucket: inner, failAfter: 100, failRanges: true}

	rc, err := bucket.Get(ctx, "o")
	require.NoError(t, err)
	body := newResumableObjectBody(ctx, bucket, "o", rc, int64(len(content)))
	body.backoff = time.Millisecond

	got, err := io.ReadAll(body)
	require.Error(t, err)
	assert.Len(t, got, 100)
	assert.Len(t, bucket.rangeOffset, replayResumeMaxRetries)
}
