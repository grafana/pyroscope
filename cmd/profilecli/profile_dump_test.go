package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	phlareobj "github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/s3"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
)

var dumpTestTime = time.Date(2026, 9, 18, 12, 0, 0, 123456789, time.UTC)

func parseDumpCommand(t *testing.T, args ...string) *profileDumpParams {
	t.Helper()
	app := kingpin.New("test", "test")
	commands := addProfileDumpCommands(app.Command("admin", "Administrative tasks"))
	name, err := app.Parse(append([]string{"admin", "profile-dumps"}, args...))
	require.NoError(t, err)
	require.NotNil(t, commands[name])
	return commands[name]
}

func dumpFixture(t *testing.T, tenant string, at time.Time, payload []byte) (string, profiledump.NativeMetadata, []byte) {
	t.Helper()
	key, id, err := profiledump.NewNativeObjectKey(tenant, at)
	require.NoError(t, err)
	m := profiledump.NativeMetadata{
		SchemaVersion: profiledump.NativeSchemaVersion, CapturedAt: at, TenantID: tenant,
		SourceProtocol: profiledump.SourceConnect, NativeFormat: profiledump.FormatPprof,
		PayloadEncoding: "identity", DistributorID: "test",
		PolicyFingerprint: strings.Repeat("a", 64), CaptureID: id.String(), PayloadSize: int64(len(payload)),
	}
	body, err := profiledump.MarshalNativeMetadata(key, m)
	require.NoError(t, err)
	return key, m, body
}

func uploadDumpPair(t *testing.T, b objstore.Bucket, key string, metadata, payload []byte) {
	t.Helper()
	k, err := profiledump.ParseNativeObjectKey(key)
	require.NoError(t, err)
	require.NoError(t, b.Upload(context.Background(), k.PayloadKey, bytes.NewReader(payload)))
	require.NoError(t, b.Upload(context.Background(), k.MetadataKey, bytes.NewReader(metadata)))
}

func listDumpParams(t *testing.T, from, to time.Time) *profileDumpParams {
	t.Helper()
	return parseDumpCommand(t, "list", "--tenant-id=3648", "--from="+from.Format(time.RFC3339Nano), "--to="+to.Format(time.RFC3339Nano))
}

func readDumpList(t *testing.T, ctx context.Context, b objstore.BucketReader, p *profileDumpParams) (dumpListResult, error) {
	t.Helper()
	var out bytes.Buffer
	err := runProfileDump(ctx, b, p, &out, io.Discard)
	var result dumpListResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &result), "%s", out.String())
	return result, err
}

type dumpTestBucket struct {
	objstore.Bucket
	prefixes, gets, attrs                        []string
	ranges, opened, closed, clientClosed         int
	readBytes                                    int
	attrsErr, getErr, readErr, closeErr, iterErr error
	errorKey                                     string
	onRead, onIter                               func()
	getBody                                      []byte
	extraKeys                                    []string
	pages                                        [][]string
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
	if b.pages != nil {
		// Model callbacks spanning provider pages with deliberately unordered keys.
		for _, page := range b.pages {
			for _, key := range page {
				if err := f(key); err != nil {
					return err
				}
			}
		}
		return b.iterErr
	}
	if err := b.Bucket.Iter(ctx, prefix, f, options...); err != nil {
		return err
	}
	return b.iterErr
}
func (b *dumpTestBucket) Attributes(ctx context.Context, key string) (objstore.ObjectAttributes, error) {
	b.attrs = append(b.attrs, key)
	if b.attrsErr != nil && (b.errorKey == "" || key == b.errorKey) {
		return objstore.ObjectAttributes{}, b.attrsErr
	}
	return b.Bucket.Attributes(ctx, key)
}
func (b *dumpTestBucket) GetRange(context.Context, string, int64, int64) (io.ReadCloser, error) {
	b.ranges++
	return nil, errors.New("CLI must not use envelope range readers")
}
func (b *dumpTestBucket) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	b.gets = append(b.gets, key)
	if b.getErr != nil && (b.errorKey == "" || key == b.errorKey) {
		return nil, b.getErr
	}
	r, err := b.Bucket.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if b.getBody != nil && (b.errorKey == "" || key == b.errorKey) {
		if err := r.Close(); err != nil {
			return nil, err
		}
		r = io.NopCloser(bytes.NewReader(b.getBody))
	}
	b.opened++
	return &dumpTestReader{ReadCloser: r, bucket: b, key: key}, nil
}

type dumpTestReader struct {
	io.ReadCloser
	bucket *dumpTestBucket
	key    string
}

func (r *dumpTestReader) Read(p []byte) (int, error) {
	if r.bucket.onRead != nil {
		r.bucket.onRead()
	}
	if r.bucket.readErr != nil && (r.bucket.errorKey == "" || r.bucket.errorKey == r.key) {
		return 0, r.bucket.readErr
	}
	n, err := r.ReadCloser.Read(p)
	r.bucket.readBytes += n
	return n, err
}
func (r *dumpTestReader) Close() error {
	r.bucket.closed++
	return errors.Join(r.ReadCloser.Close(), r.bucket.closeErr)
}

func TestProfileDumpMinuteWindows(t *testing.T) {
	for _, tc := range []struct {
		name, from, to string
		minutes        []string
	}{
		{"aligned", "2026-09-18T12:00:00Z", "2026-09-18T12:01:00Z", []string{"2026-09-18/12/00"}},
		{"unaligned", "2026-09-18T12:00:30Z", "2026-09-18T12:01:30Z", []string{"2026-09-18/12/00", "2026-09-18/12/01"}},
		{"fractional", "2026-09-18T12:00:00.123456789Z", "2026-09-18T12:00:00.123456799Z", []string{"2026-09-18/12/00"}},
		{"offset", "2026-09-18T05:00:30-07:00", "2026-09-18T14:01:30+02:00", []string{"2026-09-18/12/00", "2026-09-18/12/01"}},
		{"midnight", "2026-09-18T23:59:30Z", "2026-09-19T00:00:30Z", []string{"2026-09-18/23/59", "2026-09-19/00/00"}},
		{"empty aligned", "2026-09-18T12:00:00Z", "2026-09-18T12:00:00Z", nil},
		{"empty fractional", "2026-09-18T12:00:00.123456789Z", "2026-09-18T12:00:00.123456789Z", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from, err := time.Parse(time.RFC3339Nano, tc.from)
			require.NoError(t, err)
			to, err := time.Parse(time.RFC3339Nano, tc.to)
			require.NoError(t, err)
			b := newDumpTestBucket()
			var wanted []string
			for _, at := range []time.Time{from.Add(-time.Nanosecond), from, to.Add(-time.Nanosecond), to} {
				key, _, body := dumpFixture(t, "3648", at, []byte("secret payload"))
				uploadDumpPair(t, b, key, body, []byte("secret payload"))
				if !at.Before(from) && at.Before(to) {
					wanted = append(wanted, key)
				}
			}
			other, _, body := dumpFixture(t, "other", from, nil)
			uploadDumpPair(t, b, other, body, nil)
			result, err := readDumpList(t, context.Background(), b, listDumpParams(t, from, to))
			require.NoError(t, err)
			var got []string
			for _, c := range result.Captures {
				got = append(got, c.Key)
			}
			require.ElementsMatch(t, wanted, got)
			var prefixes []string
			for _, minute := range tc.minutes {
				prefixes = append(prefixes, profiledump.NativeObjectPrefix+"3648/"+minute+"/")
			}
			require.Equal(t, prefixes, b.prefixes)
			require.False(t, result.Limited)
			require.Zero(t, result.SkippedInvalid)
			for _, key := range b.gets {
				require.True(t, strings.HasSuffix(key, ".json"))
			}
			require.Zero(t, b.ranges)
			require.Equal(t, b.opened, b.closed)
		})
	}
}

func TestProfileDumpListSkipsEarlierMinutes(t *testing.T) {
	b := newDumpTestBucket()
	target := dumpTestTime.Add(45 * time.Minute).Truncate(time.Minute)
	for i := 0; i < 10001; i++ {
		at := target.Add(-time.Duration(1+i%45) * time.Minute)
		key, _, body := dumpFixture(t, "3648", at, nil)
		uploadDumpPair(t, b, key, body, nil)
	}
	var wanted []string
	for i := 0; i < 3; i++ {
		key, _, body := dumpFixture(t, "3648", target.Add(time.Duration(i)*time.Second), nil)
		uploadDumpPair(t, b, key, body, nil)
		wanted = append(wanted, key)
	}
	result, err := readDumpList(t, context.Background(), b, listDumpParams(t, target, target.Add(time.Minute)))
	require.NoError(t, err)
	require.False(t, result.Limited)
	require.Len(t, result.Captures, 3)
	require.Equal(t, 13, result.Work) // One minute, six entries, and two calls reserved for each JSON.
	require.Equal(t, []string{profiledump.NativeObjectPrefix + "3648/2026-09-18/12/45/"}, b.prefixes)
	var got []string
	for _, c := range result.Captures {
		got = append(got, c.Key)
	}
	require.ElementsMatch(t, wanted, got)
}

func TestProfileDumpListUnorderedPages(t *testing.T) {
	b := newDumpTestBucket()
	from := dumpTestTime.Truncate(time.Minute).Add(10 * time.Second)
	to := from.Add(10 * time.Second)
	keys := make([]string, 4)
	for i, at := range []time.Time{to, from, from.Add(-time.Second), to.Add(-time.Second)} {
		key, _, body := dumpFixture(t, "3648", at, nil)
		uploadDumpPair(t, b, key, body, nil)
		k, err := profiledump.ParseNativeObjectKey(key)
		require.NoError(t, err)
		keys[i] = k.MetadataKey
	}
	b.pages = [][]string{{keys[0], keys[1]}, {keys[2], keys[3]}}
	result, err := readDumpList(t, context.Background(), b, listDumpParams(t, from, to))
	require.NoError(t, err)
	require.Len(t, result.Captures, 2)
	require.False(t, result.Limited)
	require.Equal(t, 9, result.Work)
}

func TestProfileDumpListLimits(t *testing.T) {
	for _, tc := range []struct {
		name        string
		work, limit int
		boundary    bool
		reason      string
	}{
		{"results", 100, 1, false, "result limit"},
		{"selected minute", 10, 100, false, "work limit"},
		{"boundary minute", 10, 100, true, "work limit"},
		{"reserve both calls", 3, 100, false, "work limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newDumpTestBucket()
			from := dumpTestTime.Truncate(time.Minute)
			for i := 0; i < 20; i++ {
				key, _, body := dumpFixture(t, "3648", from.Add(time.Duration(i)*time.Second), nil)
				uploadDumpPair(t, b, key, body, nil)
			}
			if tc.boundary {
				from = from.Add(30 * time.Second)
			}
			p := listDumpParams(t, from, from.Truncate(time.Minute).Add(time.Minute))
			p.maxWork, p.limit = tc.work, tc.limit
			result, err := readDumpList(t, context.Background(), b, p)
			require.NoError(t, err)
			require.True(t, result.Limited)
			require.Contains(t, result.Reason, tc.reason)
			require.LessOrEqual(t, result.Work, p.maxWork)
			require.LessOrEqual(t, len(result.Captures), p.limit)
			if tc.boundary {
				require.Empty(t, result.Captures)
			}
			if tc.name == "selected minute" {
				require.NotEmpty(t, result.Captures)
			}
			if tc.name == "reserve both calls" {
				require.Empty(t, b.gets)
				require.Empty(t, b.attrs)
			}
		})
	}
	t.Run("huge interval generated lazily", func(t *testing.T) {
		b := newDumpTestBucket()
		p := parseDumpCommand(t, "list", "--tenant-id=3648", "--from=1970-01-01T00:00:00Z", "--to=9999-12-31T23:59:59Z", "--max-work=3")
		result, err := readDumpList(t, context.Background(), b, p)
		require.NoError(t, err)
		require.Len(t, b.prefixes, 3)
		require.Equal(t, 3, result.Work)
		require.True(t, result.Limited)
	})
}

func TestProfileDumpInspectAndListInvalidPairs(t *testing.T) {
	for _, scenario := range []string{"missing sidecar", "missing payload", "invalid JSON", "oversize metadata", "identity mismatch", "time mismatch", "size mismatch", "oversize payload", "redirect", "unknown schema"} {
		t.Run(scenario, func(t *testing.T) {
			b := newDumpTestBucket()
			key, m, body := dumpFixture(t, "3648", dumpTestTime, []byte("native"))
			k, err := profiledump.ParseNativeObjectKey(key)
			require.NoError(t, err)
			switch scenario {
			case "invalid JSON":
				body = []byte("{")
			case "oversize metadata":
				body = bytes.Repeat([]byte(" "), profiledump.MaxMetadataSize+100)
			case "identity mismatch":
				m.TenantID = "other"
				body, err = json.Marshal(m)
			case "time mismatch":
				m.CapturedAt = m.CapturedAt.Add(time.Second)
				body, err = json.Marshal(m)
			case "size mismatch":
				m.PayloadSize++
				body, err = json.Marshal(m)
			case "redirect":
				body = append(body[:len(body)-1], []byte(`,"payload_key":"outside"}`)...)
			case "unknown schema":
				m.SchemaVersion++
				body, err = json.Marshal(m)
			}
			require.NoError(t, err)
			uploadDumpPair(t, b, key, body, []byte("native"))
			expected := errDumpInvalid
			if scenario == "missing sidecar" {
				require.NoError(t, b.Delete(context.Background(), k.MetadataKey))
				expected = errDumpMissing
			}
			if scenario == "missing payload" {
				require.NoError(t, b.Delete(context.Background(), k.PayloadKey))
				expected = errDumpMissing
			}
			p := parseDumpCommand(t, "inspect", key)
			if scenario == "oversize payload" {
				p.maxSize = 5
			}
			var out bytes.Buffer
			err = runProfileDump(context.Background(), b, p, &out, io.Discard)
			require.ErrorIs(t, err, expected)
			if scenario == "missing sidecar" {
				require.Contains(t, err.Error(), "metadata sidecar")
			}
			if scenario == "missing payload" {
				require.Contains(t, err.Error(), "native payload")
			}
			require.Empty(t, out.String())
			if scenario == "oversize metadata" {
				require.Equal(t, profiledump.MaxMetadataSize+1, b.readBytes)
			}
			// A sidecar can disappear after listing. Payload-only orphans are otherwise not candidates.
			valid, _, validBody := dumpFixture(t, "3648", dumpTestTime.Add(time.Second), nil)
			uploadDumpPair(t, b, valid, validBody, nil)
			validKeys, err := profiledump.ParseNativeObjectKey(valid)
			require.NoError(t, err)
			b.pages = [][]string{{k.MetadataKey}, {validKeys.MetadataKey}}
			lp := listDumpParams(t, dumpTestTime.Truncate(time.Minute), dumpTestTime.Truncate(time.Minute).Add(time.Minute))
			lp.maxSize = p.maxSize
			result, err := readDumpList(t, context.Background(), b, lp)
			require.NoError(t, err)
			require.False(t, result.Limited)
			require.Len(t, result.Captures, 1)
			require.Equal(t, valid, result.Captures[0].Key)
			if expected == errDumpMissing {
				require.Equal(t, 1, result.SkippedMissing)
			} else {
				require.Equal(t, 1, result.SkippedInvalid)
			}
			p.operation, p.path = dumpExtract, filepath.Join(t.TempDir(), "native")
			require.ErrorIs(t, runProfileDump(context.Background(), b, p, io.Discard, io.Discard), expected)
			files, err := os.ReadDir(filepath.Dir(p.path))
			require.NoError(t, err)
			require.Empty(t, files)
			require.Equal(t, b.opened, b.closed)
			for _, accessed := range append(b.gets, b.attrs...) {
				require.Contains(t, []string{k.MetadataKey, k.PayloadKey, validKeys.MetadataKey, validKeys.PayloadKey}, accessed)
			}
		})
	}
}

func TestProfileDumpListStorageFailuresPreserveOutput(t *testing.T) {
	for _, operation := range []string{"get", "attributes", "read", "list"} {
		t.Run(operation, func(t *testing.T) {
			b := newDumpTestBucket()
			keys := make([]profiledump.NativeObjectKey, 2)
			for i := range keys {
				key, _, body := dumpFixture(t, "3648", dumpTestTime.Add(time.Duration(i)*time.Second), nil)
				uploadDumpPair(t, b, key, body, nil)
				var err error
				keys[i], err = profiledump.ParseNativeObjectKey(key)
				require.NoError(t, err)
			}
			b.pages = [][]string{{keys[0].MetadataKey}, {keys[1].MetadataKey}}
			failure := errors.New("permission or network failure")
			b.errorKey = keys[1].MetadataKey
			switch operation {
			case "get":
				b.getErr = failure
			case "read":
				b.readErr = failure
			case "attributes":
				b.attrsErr, b.errorKey = failure, keys[1].PayloadKey
			case dumpList:
				b.iterErr = failure
			}
			result, err := readDumpList(t, context.Background(), b, listDumpParams(t, dumpTestTime, dumpTestTime.Add(time.Second*3)))
			require.ErrorIs(t, err, failure)
			require.NotErrorIs(t, err, errDumpInvalid)
			require.NotErrorIs(t, err, errDumpMissing)
			require.True(t, result.Limited)
			require.NotEmpty(t, result.Captures)
			require.Equal(t, keys[0].PayloadKey, result.Captures[0].Key)
			require.Zero(t, result.SkippedInvalid)
			require.Zero(t, result.SkippedMissing)
			require.Equal(t, b.opened, b.closed)
		})
	}
}

func TestProfileDumpListMalformedKeysAndOrphans(t *testing.T) {
	b := newDumpTestBucket()
	key, _, body := dumpFixture(t, "3648", dumpTestTime, nil)
	uploadDumpPair(t, b, key, body, nil)
	orphan, _, _ := dumpFixture(t, "3648", dumpTestTime, nil)
	require.NoError(t, b.Upload(context.Background(), orphan, strings.NewReader("orphan")))
	require.NoError(t, b.Upload(context.Background(), key+"/child", strings.NewReader("nested")))
	b.extraKeys = []string{"profile-debug-dumps/../escape", "unrelated/key"}
	result, err := readDumpList(t, context.Background(), b, listDumpParams(t, dumpTestTime, dumpTestTime.Truncate(time.Minute).Add(time.Minute)))
	require.NoError(t, err)
	require.Len(t, result.Captures, 1)
	require.Equal(t, 3, result.SkippedInvalid)
	require.Zero(t, result.SkippedMissing)
	require.Len(t, b.gets, 1)
}

func TestProfileDumpCancellation(t *testing.T) {
	for _, op := range []string{"inspect", dumpExtract} {
		t.Run(op, func(t *testing.T) {
			b := newDumpTestBucket()
			key, _, body := dumpFixture(t, "3648", dumpTestTime, []byte("native"))
			uploadDumpPair(t, b, key, body, []byte("native"))
			p := parseDumpCommand(t, op, key)
			p.path = filepath.Join(t.TempDir(), "native")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b.onRead = cancel
			require.ErrorIs(t, runProfileDump(ctx, b, p, io.Discard, io.Discard), context.Canceled)
			require.Equal(t, b.opened, b.closed)
			files, err := os.ReadDir(filepath.Dir(p.path))
			require.NoError(t, err)
			require.Empty(t, files)
		})
	}
	for _, prior := range []bool{false, true} {
		t.Run(fmt.Sprintf("final empty minute prior=%t", prior), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := newDumpTestBucket()
			from := dumpTestTime.Truncate(time.Minute)
			p := listDumpParams(t, from, from.Add(time.Minute))
			partitions := 1
			if prior {
				key, _, body := dumpFixture(t, "3648", dumpTestTime, nil)
				uploadDumpPair(t, b, key, body, nil)
				p.to = from.Add(2 * time.Minute).Format(time.RFC3339Nano)
				partitions = 2
			}
			// In-memory Iter ignores cancellation, including an empty final partition.
			b.onIter = func() {
				if len(b.prefixes) == partitions {
					cancel()
				}
			}
			result, err := readDumpList(t, ctx, b, p)
			require.ErrorIs(t, err, context.Canceled)
			require.True(t, result.Limited)
			require.Equal(t, context.Canceled.Error(), result.Reason)
			if prior {
				require.Len(t, result.Captures, 1)
			} else {
				require.Empty(t, result.Captures)
			}
			require.Len(t, b.prefixes, partitions)
		})
	}
	b := newDumpTestBucket()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := readDumpList(t, ctx, b, listDumpParams(t, dumpTestTime, dumpTestTime.Add(time.Minute)))
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, result.Limited)
	require.Empty(t, b.prefixes)
}

func TestProfileDumpExtractRepresentations(t *testing.T) {
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, err := zw.Write([]byte("synthetic pprof bytes"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	for _, tc := range []struct {
		name, encoding, extension string
		payload                   []byte
	}{
		{"gzip", "gzip", ".pprof.gz", gz.Bytes()},
		{"malformed", "identity", ".pprof", []byte("malformed input")},
		{"empty", "identity", ".pprof", []byte{}},
		{"unknown", dumpEncodingUnknown, ".pprof.encoded", []byte{0xff}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newDumpTestBucket()
			key, m, _ := dumpFixture(t, "team-a", dumpTestTime, tc.payload)
			m.PayloadEncoding = tc.encoding
			m.Labels = map[string]string{"__name__": "app"}
			body, err := profiledump.MarshalNativeMetadata(key, m)
			require.NoError(t, err)
			uploadDumpPair(t, b, key, body, tc.payload)
			k, err := profiledump.ParseNativeObjectKey(key)
			require.NoError(t, err)
			require.Equal(t, tc.extension, dumpExtension(m))
			var inspection bytes.Buffer
			require.NoError(t, runProfileDump(context.Background(), b, parseDumpCommand(t, "inspect", k.MetadataKey), &inspection, io.Discard))
			require.Contains(t, inspection.String(), "payload not read or verified")
			require.NotContains(t, b.gets, key)
			dir := t.TempDir()
			p := parseDumpCommand(t, dumpExtract, k.MetadataKey, "--output", filepath.Join(dir, "native"+tc.extension))
			var out, warning bytes.Buffer
			require.NoError(t, runProfileDump(context.Background(), b, p, &out, &warning))
			got, err := os.ReadFile(p.path)
			require.NoError(t, err)
			require.Equal(t, tc.payload, got)
			metadata, err := os.ReadFile(p.path + ".metadata.json")
			require.NoError(t, err)
			var restored profiledump.NativeMetadata
			require.NoError(t, json.Unmarshal(metadata, &restored))
			require.Equal(t, m, restored)
			require.Contains(t, warning.String(), "raw customer data")
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

func TestProfileDumpExtractFailures(t *testing.T) {
	for _, scenario := range []string{"short read", "trailing bytes", "payload read failure", "payload GET failure", "close failure", "output directory", "payload exists", "metadata exists", "payload symlink", "warning failure", "result failure"} {
		t.Run(scenario, func(t *testing.T) {
			b := newDumpTestBucket()
			key, _, body := dumpFixture(t, "3648", dumpTestTime, []byte("native"))
			uploadDumpPair(t, b, key, body, []byte("native"))
			dir := t.TempDir()
			path := filepath.Join(dir, "native")
			p := parseDumpCommand(t, dumpExtract, key, "--output", path)
			warning, result := io.Discard, io.Discard
			existing := ""
			b.errorKey = key
			switch scenario {
			case "short read":
				b.getBody = []byte("nativ")
			case "trailing bytes":
				b.getBody = []byte("native excess")
			case "payload read failure":
				b.readErr = errors.New("network read failed")
			case "payload GET failure":
				b.getErr = errors.New("permission denied")
			case "close failure":
				b.closeErr = errors.New("close failed")
			case "output directory":
				p.path = filepath.Join(dir, "absent", "native")
			case "payload exists":
				existing = path
			case "metadata exists":
				existing = path + ".metadata.json"
			case "payload symlink":
				require.NoError(t, os.Symlink("absent-target", path))
			case "warning failure":
				warning = dumpFailWriter{}
			case "result failure":
				result = dumpFailWriter{}
			}
			if existing != "" {
				require.NoError(t, os.WriteFile(existing, []byte("preserved"), 0600))
			}
			err := runProfileDump(context.Background(), b, p, result, warning)
			require.Error(t, err)
			if scenario == "short read" || scenario == "trailing bytes" {
				require.ErrorIs(t, err, errDumpInvalid)
			}
			if scenario == "payload read failure" {
				require.ErrorIs(t, err, b.readErr)
				require.NotErrorIs(t, err, errDumpInvalid)
			}
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
	for _, w := range []io.Writer{dumpFailWriter{}, dumpShortWriter{}} {
		tracked := &dumpPayloadWriter{w: w}
		_, err := io.Copy(tracked, strings.NewReader("native"))
		require.Error(t, err)
		require.Error(t, tracked.err)
	}
}

type dumpFailWriter struct{}

func (dumpFailWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type dumpShortWriter struct{}

func (dumpShortWriter) Write(b []byte) (int, error) { return len(b) - 1, nil }

func TestProfileDumpPrefixAndOwnedClient(t *testing.T) {
	dir := t.TempDir()
	key, _, body := dumpFixture(t, "3648", dumpTestTime, []byte("native"))
	k, err := profiledump.ParseNativeObjectKey(key)
	require.NoError(t, err)
	path := filepath.Join(dir, "customer", key)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("native"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "customer", k.MetadataKey), body, 0600))
	for _, invalid := range []bool{false, true} {
		p := parseDumpCommand(t, "inspect", key, "--storage.filesystem.dir", dir, "--storage.prefix=customer")
		var tracked *dumpTestBucket
		p.objectStoreCfg.Middlewares = append(p.objectStoreCfg.Middlewares, func(b objstore.Bucket) (objstore.Bucket, error) {
			tracked = &dumpTestBucket{Bucket: b}
			return tracked, nil
		})
		if invalid {
			p.key = "../invalid"
		}
		var out bytes.Buffer
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
	b := phlareobj.NewPrefixedBucket(phlareobj.NewBucket(objstore.NewInMemBucket()), "customer")
	uploadDumpPair(t, b, key, body, []byte("native"))
	_, err = inspectProfileDump(context.Background(), b, key, math.MaxInt64)
	require.NoError(t, err)
	result, err := readDumpList(t, context.Background(), b, listDumpParams(t, dumpTestTime, dumpTestTime.Add(time.Second)))
	require.NoError(t, err)
	require.Len(t, result.Captures, 1)
}

func TestProfileDumpCommandParsing(t *testing.T) {
	key, _, _ := dumpFixture(t, "a", dumpTestTime, nil)
	p := parseDumpCommand(t, "inspect", key, "--storage.backend=s3", "--storage.s3.region=test", "--storage.s3.insecure", "--storage.s3.sse.type=SSE-KMS", "--storage.s3.sse.kms-key-id=key", "--timeout=10s")
	require.Equal(t, "s3", p.objectStoreCfg.Backend)
	require.True(t, p.objectStoreCfg.S3.Insecure)
	require.Equal(t, "key", p.objectStoreCfg.S3.SSE.KMSKeyID)
	require.Equal(t, 10*time.Second, p.timeout)
	for _, args := range [][]string{
		{"profile-dump", "inspect", key}, {"profile-dumps", "inspect", key},
		{"admin", "profile-dump", "inspect", key},
		{"admin", "profile-dumps", "list"}, {"admin", "profile-dumps", "inspect"}, {"admin", "profile-dumps", dumpExtract},
	} {
		app := kingpin.New("test", "test")
		addProfileDumpCommands(app.Command("admin", "Administrative tasks"))
		_, err := app.Parse(args)
		require.Error(t, err)
	}
	app := kingpin.New("profilecli", "test")
	var help bytes.Buffer
	app.UsageWriter(&help).Terminate(func(int) {})
	addProfileDumpCommands(app.Command("admin", "Administrative tasks"))
	_, _ = app.Parse([]string{"admin", "profile-dumps", "list", "--help"})
	require.Contains(t, help.String(), "admin profile-dumps list")
	require.Contains(t, help.String(), "minute listing")
	require.NotContains(t, help.String(), "envelope")
	for _, invalid := range []string{"../outside", "profile-debug-dumps/old.pyrdump", "/profile-debug-dumps/file"} {
		b := newDumpTestBucket()
		_, err := inspectProfileDump(context.Background(), b, invalid, math.MaxInt64)
		require.ErrorIs(t, err, errDumpInvalid)
		require.Empty(t, b.gets)
		require.Empty(t, b.attrs)
	}
}

// Exercise pagination through the pinned S3 provider, using the same local HTTP
// fixture pattern as the storage and cleaner tests. No request-count guarantee is assumed.
func TestProfileDumpListS3Pagination(t *testing.T) {
	from := dumpTestTime.Truncate(time.Minute).Add(10 * time.Second)
	to := from.Add(10 * time.Second)
	prefix := profiledump.NativeObjectPrefix + "3648/2026-09-18/12/00/"
	metadata := make(map[string][]byte)
	payloads := make(map[string]bool)
	keys := make([]string, 2)
	for i, at := range []time.Time{to, from} {
		key, _, body := dumpFixture(t, "3648", at, nil)
		k, err := profiledump.ParseNativeObjectKey(key)
		require.NoError(t, err)
		keys[i] = k.MetadataKey
		metadata[k.MetadataKey] = body
		payloads[k.PayloadKey] = true
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") == "2" {
			if r.URL.Query().Get("prefix") != prefix {
				t.Errorf("unexpected prefix %q", r.URL.Query().Get("prefix"))
			}
			w.Header().Set("Content-Type", "application/xml")
			key := keys[0]
			continuation := "<IsTruncated>true</IsTruncated><NextContinuationToken>next</NextContinuationToken>"
			if r.URL.Query().Get("continuation-token") == "next" {
				key = keys[1]
				continuation = "<IsTruncated>false</IsTruncated>"
			}
			_, _ = fmt.Fprintf(w, `<ListBucketResult><Name>captures</Name>%s<Contents><Key>%s</Key><Size>%d</Size><LastModified>2026-09-18T12:00:00.000Z</LastModified></Contents></ListBucketResult>`, continuation, key, len(metadata[key]))
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/captures/")
		if body, ok := metadata[key]; ok && r.Method == http.MethodGet {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.Header().Set("Last-Modified", "Fri, 18 Sep 2026 12:00:00 GMT")
			_, _ = w.Write(body)
			return
		}
		if payloads[key] && r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "0")
			w.Header().Set("Last-Modified", "Fri, 18 Sep 2026 12:00:00 GMT")
			return
		}
		t.Errorf("unexpected storage access %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	var cfg s3.Config
	cfg.RegisterFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	cfg.Endpoint = strings.TrimPrefix(srv.URL, "http://")
	cfg.BucketName, cfg.Region, cfg.Insecure = "captures", "us-east-1", true
	cfg.AccessKeyID = "local-test-key"
	require.NoError(t, cfg.SecretAccessKey.Set("local-test-secret"))
	cfg.BucketLookupType = s3.PathStyleLookup
	cfg.HTTP.Transport = srv.Client().Transport
	bucket, err := s3.NewBucketClient(cfg, "test", log.NewNopLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, bucket.Close()) })
	result, err := readDumpList(t, context.Background(), bucket, listDumpParams(t, from, to))
	require.NoError(t, err)
	require.False(t, result.Limited)
	require.Len(t, result.Captures, 1)
	require.Equal(t, from, result.Captures[0].CapturedAt)
}
