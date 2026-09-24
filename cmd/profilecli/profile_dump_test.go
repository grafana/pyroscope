package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	phlareobj "github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
)

func parseDumpCommand(t *testing.T, args ...string) *profileDumpParams {
	t.Helper()
	app := kingpin.New("test", "test")
	commands := addProfileDumpCommands(app)
	name, err := app.Parse(append([]string{"profile-dump"}, args...))
	require.NoError(t, err)
	return commands[name]
}

func dumpFixture(t *testing.T, tenant string, at time.Time, format profiledump.Format, source profiledump.SourceProtocol, payload []byte) (string, profiledump.Metadata, []byte) {
	t.Helper()
	key, id, err := profiledump.NewObjectKey(tenant, at, format)
	require.NoError(t, err)
	m := profiledump.Metadata{
		SchemaVersion: profiledump.Version, CapturedAt: at, TenantID: tenant,
		SourceProtocol: source, NativeFormat: format,
		PayloadEncoding: "identity",
		DistributorID:   "test", ActivationSource: profiledump.ActivationRuntimeOverride,
		PolicyFingerprint: strings.Repeat("a", 64), CaptureID: id.String(), PayloadSize: int64(len(payload)),
	}
	return key, m, encodeDumpFixture(t, m, payload)
}

func encodeDumpFixture(t *testing.T, m profiledump.Metadata, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, profiledump.Encode(&buf, m, bytes.NewReader(payload), math.MaxInt64))
	return buf.Bytes()
}

var dumpTestTime = time.Date(2026, 9, 18, 12, 0, 0, 123456789, time.UTC)

type dumpTestBucket struct {
	objstore.Bucket
	prefixes                                   []string
	ranges, gets, opened, closed, clientClosed int
	rangeBytes                                 int64
	attrsErr, rangeErr, getErr, closeErr       error
	onRead                                     func()
	onIter                                     func()
	getBody                                    []byte
	extraKeys                                  []string
}

func newDumpTestBucket() *dumpTestBucket { return &dumpTestBucket{Bucket: objstore.NewInMemBucket()} }
func (b *dumpTestBucket) Close() error   { b.clientClosed++; return b.Bucket.Close() }
func (b *dumpTestBucket) Iter(ctx context.Context, prefix string, f func(string) error, options ...objstore.IterOption) error {
	b.prefixes = append(b.prefixes, prefix)
	if b.onIter != nil {
		b.onIter()
	}
	for _, k := range b.extraKeys {
		if err := f(k); err != nil {
			return err
		}
	}
	return b.Bucket.Iter(ctx, prefix, f, options...)
}
func (b *dumpTestBucket) Attributes(ctx context.Context, key string) (objstore.ObjectAttributes, error) {
	if b.attrsErr != nil {
		return objstore.ObjectAttributes{}, b.attrsErr
	}
	return b.Bucket.Attributes(ctx, key)
}
func (b *dumpTestBucket) GetRange(ctx context.Context, key string, off, length int64) (io.ReadCloser, error) {
	b.ranges++
	b.rangeBytes += length
	if b.rangeErr != nil {
		return nil, b.rangeErr
	}
	r, err := b.Bucket.GetRange(ctx, key, off, length)
	if err != nil {
		return nil, err
	}
	b.opened++
	return &dumpTestReader{Reader: r, bucket: b, closer: r}, nil
}
func (b *dumpTestBucket) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	b.gets++
	if b.getErr != nil {
		return nil, b.getErr
	}
	r, err := b.Bucket.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if b.getBody != nil {
		requireClose := r.Close()
		if requireClose != nil {
			return nil, requireClose
		}
		r = io.NopCloser(bytes.NewReader(b.getBody))
	}
	b.opened++
	return &dumpTestReader{Reader: r, bucket: b, closer: r}, nil
}

type dumpTestReader struct {
	io.Reader
	bucket *dumpTestBucket
	closer io.Closer
}

func (r *dumpTestReader) Read(p []byte) (int, error) {
	if r.bucket.onRead != nil {
		r.bucket.onRead()
	}
	return r.Reader.Read(p)
}
func (r *dumpTestReader) Close() error {
	r.bucket.closed++
	return errors.Join(r.closer.Close(), r.bucket.closeErr)
}

func TestProfileDumpList(t *testing.T) {
	tenant := "../tenant/a\\b"
	b := newDumpTestBucket()
	var wanted string
	for _, tc := range []struct {
		tenant string
		at     time.Time
		format profiledump.Format
		source profiledump.SourceProtocol
	}{
		{tenant, dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect},
		{"other", dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect},
		{tenant, dumpTestTime.Add(-24 * time.Hour), profiledump.FormatPprof, profiledump.SourceConnect},
		{tenant, dumpTestTime.Add(-time.Nanosecond), profiledump.FormatPprof, profiledump.SourceConnect},
		{tenant, dumpTestTime.Add(time.Hour), profiledump.FormatPprof, profiledump.SourceConnect},
	} {
		key, _, body := dumpFixture(t, tc.tenant, tc.at, tc.format, tc.source, []byte("secret native payload"))
		require.NoError(t, b.Upload(context.Background(), key, bytes.NewReader(body)))
		if wanted == "" {
			wanted = key
		}
	}
	p := parseDumpCommand(t, "list", "--tenant-id", tenant, "--from", dumpTestTime.Format(time.RFC3339Nano), "--to", dumpTestTime.Add(time.Hour).Format(time.RFC3339Nano), "--source", "connect", "--format", "pprof")
	var out bytes.Buffer
	require.NoError(t, runProfileDump(context.Background(), b, p, &out, io.Discard))
	var result dumpListResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &result))
	require.Len(t, result.Captures, 1)
	require.Equal(t, wanted, result.Captures[0].Key)
	require.False(t, result.Limited)
	require.Equal(t, []string{strings.Join(strings.Split(wanted, "/")[:5], "/") + "/"}, b.prefixes)
	require.Equal(t, 0, b.gets)
	require.Equal(t, 6, b.ranges) // Include boundary milliseconds before metadata filtering.
	require.Equal(t, b.opened, b.closed)
	require.NotContains(t, out.String(), "secret native payload")
}

func TestProfileDumpListBoundsAndMalformedKeys(t *testing.T) {
	for _, tc := range []struct {
		name        string
		work, limit int
		limited     bool
	}{
		{"results", 100, 1, true}, {"work", 4, 100, true}, {"complete", 100, 100, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newDumpTestBucket()
			for i := 0; i < 2; i++ {
				key, _, body := dumpFixture(t, "a", dumpTestTime.Add(time.Duration(i)*time.Second), profiledump.FormatPprof, profiledump.SourceConnect, []byte("p"))
				require.NoError(t, b.Upload(context.Background(), key, bytes.NewReader(body)))
			}
			b.extraKeys = []string{"profile-debug-dumps/../escape", "unrelated/key"}
			p := parseDumpCommand(t, "list", "--tenant-id=a", "--from=2026-09-18T00:00:00Z", "--to=2026-09-19T00:00:00Z")
			p.maxWork, p.limit = tc.work, tc.limit
			var out bytes.Buffer
			require.NoError(t, runProfileDump(context.Background(), b, p, &out, io.Discard))
			var result dumpListResult
			require.NoError(t, json.Unmarshal(out.Bytes(), &result))
			require.Equal(t, tc.limited, result.Limited)
			require.LessOrEqual(t, result.Work, tc.work)
			require.LessOrEqual(t, len(result.Captures), tc.limit)
			require.Equal(t, 2, result.SkippedInvalid)
		})
	}
	t.Run("empty partitions bounded", func(t *testing.T) {
		b := newDumpTestBucket()
		p := parseDumpCommand(t, "list", "--tenant-id=a", "--from=1970-01-01T00:00:00Z", "--to=9999-01-01T00:00:00Z", "--max-work=3")
		var out bytes.Buffer
		require.NoError(t, runProfileDump(context.Background(), b, p, &out, io.Discard))
		require.Len(t, b.prefixes, 3)
		require.Contains(t, out.String(), `"limited": true`)
	})
}

func TestProfileDumpInspectCompatibilityAndValidation(t *testing.T) {
	key, m, body := dumpFixture(t, "a", dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect, []byte("PAYLOAD-SECRET"))
	for _, version := range []uint16{1} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			b := newDumpTestBucket()
			m.SchemaVersion = version
			encoded := encodeDumpFixture(t, m, []byte("PAYLOAD-SECRET"))
			require.NoError(t, b.Upload(context.Background(), key, bytes.NewReader(encoded)))
			p := parseDumpCommand(t, "inspect", key)
			var out bytes.Buffer
			require.NoError(t, runProfileDump(context.Background(), b, p, &out, io.Discard))
			require.NotContains(t, out.String(), "PAYLOAD-SECRET")
			require.Contains(t, out.String(), "payload not read or verified")
			require.Contains(t, out.String(), `"schema_version": `+string(rune('0'+version)))
			require.Equal(t, int64(len(encoded)-len("PAYLOAD-SECRET")), b.rangeBytes)
			require.Equal(t, 0, b.gets)
			require.Equal(t, 2, b.closed)
		})
	}
	for _, tc := range []struct {
		name    string
		mutate  func([]byte) []byte
		maxSize int64
	}{
		{"bad magic", func(b []byte) []byte { b[0] = '!'; return b }, math.MaxInt64},
		{"unsupported", func(b []byte) []byte { b[9] = 99; return b }, math.MaxInt64},
		{"oversized metadata", func(b []byte) []byte { binary.BigEndian.PutUint32(b[10:14], profiledump.MaxMetadataSize+1); return b }, math.MaxInt64},
		{"short header", func(b []byte) []byte { return b[:5] }, math.MaxInt64},
		{"short metadata", func(b []byte) []byte { return b[:30] }, math.MaxInt64},
		{"short payload", func(b []byte) []byte { return b[:len(b)-1] }, math.MaxInt64},
		{"trailing payload", func(b []byte) []byte { return append(b, 1) }, math.MaxInt64},
		{"oversize object", func(b []byte) []byte { return b }, 32},
		{"unknown field", func(b []byte) []byte { return bytes.Replace(b, []byte("distributor_id"), []byte("unrecognized_x"), 1) }, math.MaxInt64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newDumpTestBucket()
			invalid := tc.mutate(bytes.Clone(body))
			require.NoError(t, b.Upload(context.Background(), key, bytes.NewReader(invalid)))
			p := parseDumpCommand(t, "inspect", key)
			p.maxSize = tc.maxSize
			var out bytes.Buffer
			require.ErrorIs(t, runProfileDump(context.Background(), b, p, &out, io.Discard), errDumpInvalid)
			require.Empty(t, out.String())
			require.Equal(t, b.opened, b.closed)
			p.operation, p.path = dumpExtract, filepath.Join(t.TempDir(), "native")
			require.ErrorIs(t, runProfileDump(context.Background(), b, p, &out, io.Discard), errDumpInvalid)
			files, err := os.ReadDir(filepath.Dir(p.path))
			require.NoError(t, err)
			require.Empty(t, files)
			require.Equal(t, b.opened, b.closed)
		})
	}
}

// S3 rejects a range starting at EOF instead of returning an empty reader like
// the in-memory bucket. Keep that distinction in the corrupt-envelope tests.
type dumpS3RangeBucket struct {
	*dumpTestBucket
	invalidRanges int
}

func (b *dumpS3RangeBucket) GetRange(ctx context.Context, key string, off, length int64) (io.ReadCloser, error) {
	attrs, err := b.Attributes(ctx, key)
	if err != nil {
		return nil, err
	}
	if off >= attrs.Size {
		b.invalidRanges++
		return nil, errors.New("InvalidRange: The requested range is not satisfiable")
	}
	return b.dumpTestBucket.GetRange(ctx, key, off, length)
}

func TestProfileDumpTruncatedMetadataDoesNotRequestInvalidRange(t *testing.T) {
	key, _, body := dumpFixture(t, "a", dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect, []byte("native"))
	validKey, _, validBody := dumpFixture(t, "a", dumpTestTime.Add(time.Second), profiledump.FormatPprof, profiledump.SourceConnect, []byte("valid"))
	for _, tc := range []struct {
		name string
		size int
	}{
		{"header only", profiledump.HeaderSize},
		{"partial metadata", profiledump.HeaderSize + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &dumpS3RangeBucket{dumpTestBucket: newDumpTestBucket()}
			require.NoError(t, b.Upload(context.Background(), key, bytes.NewReader(body[:tc.size])))
			require.NoError(t, b.Upload(context.Background(), validKey, bytes.NewReader(validBody)))
			p := parseDumpCommand(t, "inspect", key)
			var out bytes.Buffer
			err := runProfileDump(context.Background(), b, p, &out, io.Discard)
			require.ErrorIs(t, err, errDumpInvalid)
			require.NotErrorIs(t, err, errDumpMissing)
			require.NotContains(t, err.Error(), "storage failure")
			require.Empty(t, out.String())
			require.Equal(t, 1, b.ranges, "reject impossible metadata length after reading only the header")
			require.Zero(t, b.invalidRanges)
			require.Equal(t, b.opened, b.closed)

			p = parseDumpCommand(t, "list", "--tenant-id=a", "--from=2026-09-18T00:00:00Z", "--to=2026-09-19T00:00:00Z")
			require.NoError(t, runProfileDump(context.Background(), b, p, &out, io.Discard))
			var result dumpListResult
			require.NoError(t, json.Unmarshal(out.Bytes(), &result))
			require.False(t, result.Limited)
			require.Equal(t, 1, result.SkippedInvalid)
			require.Zero(t, result.SkippedMissing)
			require.Len(t, result.Captures, 1)
			require.Equal(t, validKey, result.Captures[0].Key, "continue listing after the corrupt capture")
			require.Equal(t, 4, b.ranges, "one header for each corrupt inspection, header and metadata for the valid capture")
			require.Zero(t, b.invalidRanges)
			require.Zero(t, b.gets)
			require.Equal(t, b.opened, b.closed)
		})
	}
}

func TestProfileDumpExtractRepresentations(t *testing.T) {
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, err := zw.Write([]byte("synthetic pprof bytes"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	for _, tc := range []struct {
		name                string
		encoding, extension string
		payload             []byte
	}{
		{"compressed pprof", "gzip", ".pprof.gz", gz.Bytes()},
		{"malformed pprof", "identity", ".pprof", []byte("malformed input")},
		{"unknown encoding", dumpEncodingUnknown, ".pprof.encoded", []byte{0xff}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newDumpTestBucket()
			key, m, _ := dumpFixture(t, "../../outside", dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect, tc.payload)
			m.PayloadEncoding = tc.encoding
			m.Labels = map[string]string{"__name__": "app"}
			require.NoError(t, b.Upload(context.Background(), key, bytes.NewReader(encodeDumpFixture(t, m, tc.payload))))
			require.Equal(t, tc.extension, dumpExtension(m))
			dir := t.TempDir()
			p := parseDumpCommand(t, dumpExtract, key, "--output", filepath.Join(dir, "native"+tc.extension))
			var out, warning bytes.Buffer
			require.NoError(t, runProfileDump(context.Background(), b, p, &out, &warning))
			got, err := os.ReadFile(p.path)
			require.NoError(t, err)
			require.Equal(t, tc.payload, got)
			meta, err := os.ReadFile(p.path + ".metadata.json")
			require.NoError(t, err)
			var restored profiledump.Metadata
			require.NoError(t, json.Unmarshal(meta, &restored))
			require.Equal(t, m, restored)
			require.Contains(t, warning.String(), "raw customer data")
			require.Contains(t, warning.String(), "cannot technically control")
			require.NotContains(t, out.String(), "WARNING")
			require.Contains(t, out.String(), "no checksum")
			require.Equal(t, b.opened, b.closed)
			stat, err := os.Stat(p.path)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0600), stat.Mode().Perm())
			require.ErrorContains(t, runProfileDump(context.Background(), b, p, io.Discard, io.Discard), "existing files")
			got, err = os.ReadFile(p.path)
			require.NoError(t, err)
			require.Equal(t, tc.payload, got)
			files, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, files, 2)
		})
	}
}

func TestProfileDumpErrorsAndCancellation(t *testing.T) {
	key, _, body := dumpFixture(t, "a", dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect, []byte("native"))
	for _, op := range []string{"inspect", dumpExtract} {
		t.Run(op, func(t *testing.T) {
			p := parseDumpCommand(t, op, key)
			p.path = filepath.Join(t.TempDir(), "native")
			b := newDumpTestBucket()
			require.ErrorIs(t, runProfileDump(context.Background(), b, p, io.Discard, io.Discard), errDumpMissing)
			denied := errors.New("permission denied")
			b.attrsErr, b.getErr = denied, denied
			err := runProfileDump(context.Background(), b, p, io.Discard, io.Discard)
			require.ErrorIs(t, err, denied)
			require.NotErrorIs(t, err, errDumpInvalid)
			require.NotErrorIs(t, err, errDumpMissing)
			b.attrsErr, b.getErr = nil, nil
			require.NoError(t, b.Upload(context.Background(), key, bytes.NewReader(body)))
			ctx, cancel := context.WithCancel(context.Background())
			b.onRead = cancel
			require.ErrorIs(t, runProfileDump(ctx, b, p, io.Discard, io.Discard), context.Canceled)
			require.Equal(t, b.opened, b.closed)
			files, err := os.ReadDir(filepath.Dir(p.path))
			require.NoError(t, err)
			require.Empty(t, files)
			b.onRead = nil
			b.closeErr = errors.New("close failed")
			require.ErrorContains(t, runProfileDump(context.Background(), b, p, io.Discard, io.Discard), "close failed")
			require.Equal(t, b.opened, b.closed)
		})
	}
	for _, key := range []string{"../outside", "profile-debug-dumps/YQ/2026/09/18/../../outside", "profile-debug-dumps/YR/2026/09/18/file.pyrdump", "/profile-debug-dumps/file"} {
		b := newDumpTestBucket()
		p := parseDumpCommand(t, "inspect", key)
		require.ErrorIs(t, runProfileDump(context.Background(), b, p, io.Discard, io.Discard), errDumpInvalid)
		require.Zero(t, b.ranges)
		require.Zero(t, b.gets)
	}
}

func TestProfileDumpPrefixAndOwnedClient(t *testing.T) {
	dir := t.TempDir()
	key, _, body := dumpFixture(t, "a", dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect, []byte("native"))
	path := filepath.Join(dir, "customer", key)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, body, 0600))
	for _, invalid := range []bool{false, true} {
		p := parseDumpCommand(t, "inspect", key, "--storage.filesystem.dir", dir, "--storage.prefix=customer")
		var tracked *dumpTestBucket
		p.objectStoreCfg.Middlewares = append(p.objectStoreCfg.Middlewares, func(b objstore.Bucket) (objstore.Bucket, error) {
			tracked = &dumpTestBucket{Bucket: b}
			return tracked, nil
		})
		var out bytes.Buffer
		if invalid {
			p.key = "../invalid"
		}
		err := profileDump(withOutput(context.Background(), &out), p)
		if invalid {
			require.ErrorIs(t, err, errDumpInvalid)
		} else {
			require.NoError(t, err)
			require.Contains(t, out.String(), key)
		}
		require.Equal(t, 1, tracked.clientClosed)
		require.Equal(t, tracked.opened, tracked.closed)
	}
	// The same prefix wrapper works with metadata reads, not only direct Get.
	b := phlareobj.NewPrefixedBucket(phlareobj.NewBucket(objstore.NewInMemBucket()), "customer")
	require.NoError(t, b.Upload(context.Background(), key, bytes.NewReader(body)))
	_, err := inspectProfileDump(context.Background(), b, key, math.MaxInt64)
	require.NoError(t, err)
}

func TestProfileDumpListDisappearingCorruptAndCancellation(t *testing.T) {
	b := newDumpTestBucket()
	key, _, body := dumpFixture(t, "a", dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect, []byte("native"))
	missing, _, _ := dumpFixture(t, "a", dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect, nil)
	corrupt, _, _ := dumpFixture(t, "a", dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect, nil)
	b.extraKeys = []string{missing}
	require.NoError(t, b.Upload(context.Background(), key, bytes.NewReader(body)))
	require.NoError(t, b.Upload(context.Background(), corrupt, strings.NewReader("corrupt")))
	// A directory entry from non-recursive Iter must not be descended into.
	require.NoError(t, b.Upload(context.Background(), key+"/child", strings.NewReader("nested")))
	p := parseDumpCommand(t, "list", "--tenant-id=a", "--from=2026-09-18T00:00:00Z", "--to=2026-09-19T00:00:00Z")
	var out bytes.Buffer
	require.NoError(t, runProfileDump(context.Background(), b, p, &out, io.Discard))
	var result dumpListResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &result))
	require.Len(t, result.Captures, 1)
	require.Equal(t, 1, result.SkippedMissing)
	require.Equal(t, 2, result.SkippedInvalid)
	require.Len(t, b.prefixes, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out.Reset()
	require.ErrorIs(t, runProfileDump(ctx, b, p, &out, io.Discard), context.Canceled)
	require.Contains(t, out.String(), `"limited": true`)
	require.Len(t, b.prefixes, 1)
	denied := errors.New("access denied")
	b.attrsErr = denied
	out.Reset()
	require.ErrorIs(t, runProfileDump(context.Background(), b, p, &out, io.Discard), denied)
	require.Contains(t, out.String(), `"limited": true`)
}

func TestProfileDumpListCancellationDuringFinalEmptyIteration(t *testing.T) {
	for _, tc := range []struct {
		name      string
		withPrior bool
	}{
		{"empty listing", false},
		{"preserve earlier results", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := newDumpTestBucket()
			p := parseDumpCommand(t, "list", "--tenant-id=a", "--from=2026-09-18T00:00:00Z", "--to=2026-09-19T00:00:00Z")
			partitions := 1
			var key string
			if tc.withPrior {
				var body []byte
				key, _, body = dumpFixture(t, "a", dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect, []byte("native"))
				require.NoError(t, b.Upload(ctx, key, bytes.NewReader(body)))
				p.to = "2026-09-20T00:00:00Z"
				partitions = 2
			}
			// InMemBucket, like Swift, ignores context in Iter. Cancel during
			// the final empty partition, which still returns nil without callbacks.
			b.onIter = func() {
				if len(b.prefixes) == partitions {
					cancel()
				}
			}
			var out bytes.Buffer
			require.ErrorIs(t, runProfileDump(ctx, b, p, &out, io.Discard), context.Canceled)
			var result dumpListResult
			require.NoError(t, json.Unmarshal(out.Bytes(), &result))
			require.True(t, result.Limited)
			require.Equal(t, context.Canceled.Error(), result.Reason)
			require.Len(t, b.prefixes, partitions)
			if tc.withPrior {
				require.Len(t, result.Captures, 1)
				require.Equal(t, key, result.Captures[0].Key)
			} else {
				require.Empty(t, result.Captures)
			}
			require.Equal(t, b.opened, b.closed)
			require.Zero(t, b.gets)
		})
	}
}

func TestProfileDumpExtractFailures(t *testing.T) {
	key, _, body := dumpFixture(t, "a", dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect, []byte("native"))
	for _, scenario := range []string{"short read", "output directory", "payload exists", "metadata exists", "payload symlink", "key mismatch", "warning failure", "result failure"} {
		t.Run(scenario, func(t *testing.T) {
			b := newDumpTestBucket()
			require.NoError(t, b.Upload(context.Background(), key, bytes.NewReader(body)))
			dir := t.TempDir()
			path := filepath.Join(dir, "native")
			p := parseDumpCommand(t, dumpExtract, key, "--output", path)
			warning := io.Discard
			result := io.Discard
			existing := ""
			switch scenario {
			case "short read":
				b.getBody = body[:len(body)-1]
			case "output directory":
				p.path = filepath.Join(dir, "absent", "native")
			case "payload exists":
				existing = path
			case "metadata exists":
				existing = path + ".metadata.json"
			case "payload symlink":
				require.NoError(t, os.Symlink("absent-target", path))
			case "key mismatch":
				other, _, _ := dumpFixture(t, "other", dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect, nil)
				p.key = other
				require.NoError(t, b.Upload(context.Background(), other, bytes.NewReader(body)))
			case "result failure":
				result = dumpFailWriter{}
			case "warning failure":
				warning = dumpFailWriter{}
			}
			if existing != "" {
				require.NoError(t, os.WriteFile(existing, []byte("preserved"), 0600))
			}
			require.Error(t, runProfileDump(context.Background(), b, p, result, warning))
			require.Equal(t, b.opened, b.closed)
			files, err := os.ReadDir(dir)
			require.NoError(t, err)
			if existing != "" {
				require.Len(t, files, 1)
				got, err := os.ReadFile(existing)
				require.NoError(t, err)
				require.Equal(t, "preserved", string(got))
			} else if scenario == "payload symlink" {
				require.Len(t, files, 1)
			} else {
				require.Empty(t, files)
			}
		})
	}
	// A failing destination and a short writer propagate errors from streaming Decode.
	for _, w := range []io.Writer{dumpFailWriter{}, dumpShortWriter{}} {
		tracked := &dumpPayloadWriter{w: w}
		_, err := profiledump.Decode(bytes.NewReader(body), tracked, math.MaxInt64)
		require.Error(t, err)
		require.Error(t, tracked.err)
	}
	// /dev/full (when available) exercises an actual filesystem write failure.
	if _, err := os.Stat("/dev/full"); err == nil {
		f, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
		require.NoError(t, err)
		_, err = profiledump.Decode(bytes.NewReader(body), &dumpPayloadWriter{w: f}, math.MaxInt64)
		require.Error(t, err)
		require.NoError(t, f.Close())
	}
}

type dumpFailWriter struct{}

func (dumpFailWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type dumpShortWriter struct{}

func (dumpShortWriter) Write(b []byte) (int, error) { return len(b) - 1, nil }

func TestProfileDumpCommandParsing(t *testing.T) {
	key, _, _ := dumpFixture(t, "a", dumpTestTime, profiledump.FormatPprof, profiledump.SourceConnect, nil)
	p := parseDumpCommand(t, "inspect", key, "--storage.backend=s3", "--storage.s3.region=test", "--storage.s3.insecure", "--storage.s3.sse.type=SSE-KMS", "--storage.s3.sse.kms-key-id=key", "--timeout=10s")
	require.Equal(t, "s3", p.objectStoreCfg.Backend)
	require.True(t, p.objectStoreCfg.S3.Insecure)
	require.Equal(t, "key", p.objectStoreCfg.S3.SSE.KMSKeyID)
	require.Equal(t, 10*time.Second, p.timeout)
	for _, args := range [][]string{
		{"profile-dump", "list"}, {"profile-dump", "inspect"}, {"profile-dump", dumpExtract},
		{"profile-dump", "list", "--tenant-id=a", "--from=2026-09-18T00:00:00Z", "--to=2026-09-19T00:00:00Z", "--source=bad"},
	} {
		app := kingpin.New("test", "test")
		addProfileDumpCommands(app)
		_, err := app.Parse(args)
		require.Error(t, err)
	}
}
