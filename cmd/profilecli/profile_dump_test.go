package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
)

var dumpTestTime = time.Date(2026, 9, 18, 12, 0, 0, 123456789, time.UTC)

func parseDumpCommand(t *testing.T, addParams func(commander) *profileDumpParams, args ...string) *profileDumpParams {
	t.Helper()
	app := kingpin.New("test", "test")
	p := addParams(app.Command(args[0], "test"))
	_, err := app.Parse(args)
	require.NoError(t, err)
	return p
}

func dumpFixture(t *testing.T, tenant string, at time.Time, payload []byte) (string, profiledump.Metadata, []byte) {
	t.Helper()
	key, id, err := profiledump.NewObjectKey(tenant, at)
	require.NoError(t, err)
	m := profiledump.Metadata{
		SchemaVersion: 1, CapturedAt: at, TenantID: tenant,
		SourceProtocol: "connect", Format: "pprof",
		DistributorID: "test",
		CaptureID:     id.String(), PayloadSize: int64(len(payload)),
		Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "first"}, {Name: "service_name", Value: "second"}},
	}
	body, err := json.Marshal(m)
	require.NoError(t, err)
	return key, m, body
}

func uploadDumpPair(t *testing.T, b objstore.Bucket, key string, metadata, payload []byte) {
	t.Helper()
	k, err := profiledump.ParseObjectKey(key)
	require.NoError(t, err)
	require.NoError(t, b.Upload(context.Background(), k.PayloadKey, bytes.NewReader(payload)))
	require.NoError(t, b.Upload(context.Background(), k.MetadataKey, bytes.NewReader(metadata)))
}

func listDumpParams(t *testing.T, from, to time.Time) *profileDumpParams {
	t.Helper()
	return &profileDumpParams{tenant: "3648", from: from.Format(time.RFC3339Nano), to: to.Format(time.RFC3339Nano), limit: 100}
}

func readDumpList(t *testing.T, ctx context.Context, b objstore.BucketReader, p *profileDumpParams) (dumpListResult, error) {
	t.Helper()
	var out bytes.Buffer
	err := listProfileDumps(ctx, b, p, &out)
	if err != nil {
		require.Empty(t, out.String())
		return dumpListResult{}, err
	}
	var result dumpListResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &result), "%s", out.String())
	return result, err
}

type dumpTestBucket struct {
	objstore.Bucket
	prefixes, gets                     []string
	opened, closed, clientClosed       int
	getErr, readErr, closeErr, iterErr error
	errorKey                           string
	extraKeys                          []string
}

func newDumpTestBucket() *dumpTestBucket { return &dumpTestBucket{Bucket: objstore.NewInMemBucket()} }
func (b *dumpTestBucket) Close() error   { b.clientClosed++; return b.Bucket.Close() }
func (b *dumpTestBucket) Iter(ctx context.Context, prefix string, f func(string) error, options ...objstore.IterOption) error {
	b.prefixes = append(b.prefixes, prefix)
	for _, k := range b.extraKeys {
		if err := f(k); err != nil {
			return err
		}
	}
	if err := b.Bucket.Iter(ctx, prefix, f, options...); err != nil {
		return err
	}
	return b.iterErr
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
	b.opened++
	return &dumpTestReader{ReadCloser: r, bucket: b, key: key}, nil
}

type dumpTestReader struct {
	io.ReadCloser
	bucket *dumpTestBucket
	key    string
}

func (r *dumpTestReader) Read(p []byte) (int, error) {
	if r.bucket.readErr != nil && (r.bucket.errorKey == "" || r.bucket.errorKey == r.key) {
		return 0, r.bucket.readErr
	}
	return r.ReadCloser.Read(p)
}
func (r *dumpTestReader) Close() error {
	r.bucket.closed++
	if r.bucket.errorKey == "" || r.bucket.errorKey == r.key {
		return errors.Join(r.ReadCloser.Close(), r.bucket.closeErr)
	}
	return r.ReadCloser.Close()
}

func TestProfileDumpListWindows(t *testing.T) {
	stored := objstore.NewInMemBucket()
	var entries []dumpListEntry
	for _, offset := range []time.Duration{-time.Millisecond, 0, time.Millisecond, time.Minute, 12 * time.Hour} {
		at := dumpTestTime.Add(offset)
		key, metadata, body := dumpFixture(t, "3648", at, nil)
		uploadDumpPair(t, stored, key, body, nil)
		entries = append(entries, dumpListEntry{Key: key, CapturedAt: at.Truncate(time.Millisecond), Labels: metadata.Labels})
	}
	for _, tc := range []struct {
		name, from, to string
		want           []int
	}{
		{"exclusive end", "2026-09-18T12:00:00.123Z", "2026-09-18T12:00:00.124Z", []int{1}},
		{"fractional endpoints", "2026-09-18T12:00:00.1239Z", "2026-09-18T12:00:00.1249Z", []int{1}},
		{"offset across minutes", "2026-09-18T05:00:00-07:00", "2026-09-18T12:01:01Z", []int{0, 1, 2, 3}},
		{"midnight", "2026-09-18T23:59:59Z", "2026-09-19T00:00:01Z", []int{4}},
		{"collapsed millisecond", "2026-09-18T12:00:00.1231Z", "2026-09-18T12:00:00.1239Z", nil},
		{"empty", "2026-09-18T12:00:00Z", "2026-09-18T12:00:00Z", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &dumpTestBucket{Bucket: stored}
			p := &profileDumpParams{tenant: "3648", from: tc.from, to: tc.to, limit: 100}
			result, err := readDumpList(t, context.Background(), b, p)
			require.NoError(t, err)
			var want []dumpListEntry
			for _, i := range tc.want {
				want = append(want, entries[i])
			}
			require.ElementsMatch(t, want, result.Captures)
			if len(tc.want) == 0 {
				require.Empty(t, b.prefixes, "empty windows must not access storage")
			}
			require.False(t, result.Limited)
		})
	}
}

func TestProfileDumpListLimits(t *testing.T) {
	b := newDumpTestBucket()
	from := dumpTestTime.Truncate(time.Minute)
	var keys []string
	for i := 0; i < 3; i++ {
		key, _, _ := dumpFixture(t, "3648", from.Add(time.Duration(i)*time.Second), nil)
		keys = append(keys, key)
	}
	b.extraKeys = []string{keys[2], keys[0], keys[1]}
	p := listDumpParams(t, from, from.Add(time.Minute))
	p.limit = 2
	result, err := readDumpList(t, context.Background(), b, p)
	require.NoError(t, err)
	require.True(t, result.Limited)
	require.Len(t, result.Captures, 2)
	require.Equal(t, keys[2], result.Captures[0].Key)
	require.Equal(t, keys[0], result.Captures[1].Key)
	require.Len(t, b.gets, p.limit)
	for _, entry := range result.Captures {
		require.Equal(t, "missing", entry.MetadataStatus)
	}
}

func TestProfileDumpInspectFailures(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	for _, tc := range []struct {
		name, body             string
		get, read, close, want error
	}{
		{name: "missing sidecar", want: errDumpMissing},
		{name: "malformed JSON", body: "{", want: errDumpInvalid},
		{name: "metadata GET", get: storageErr, want: storageErr},
		{name: "metadata read", body: "{}", read: storageErr, want: storageErr},
		{name: "metadata close", body: "{}", close: storageErr, want: storageErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newDumpTestBucket()
			key, _, _ := dumpFixture(t, "3648", dumpTestTime, nil)
			sidecar := strings.TrimSuffix(key, ".pprof") + ".json"
			if tc.body != "" {
				require.NoError(t, b.Upload(context.Background(), sidecar, strings.NewReader(tc.body)))
			}
			b.getErr, b.readErr, b.closeErr = tc.get, tc.read, tc.close
			var out bytes.Buffer
			err := printProfileDump(context.Background(), b, &profileDumpParams{key: key}, &out)
			require.ErrorIs(t, err, tc.want)
			require.Empty(t, out.String())
			require.Equal(t, []string{sidecar}, b.gets)
			require.Equal(t, b.opened, b.closed)
		})
	}
}

func TestProfileDumpListStorageFailureOmitsOutput(t *testing.T) {
	b := newDumpTestBucket()
	key, _, _ := dumpFixture(t, "3648", dumpTestTime, nil)
	b.extraKeys = []string{key}
	failure := errors.New("permission or network failure")
	b.iterErr = failure
	result, err := readDumpList(t, context.Background(), b, listDumpParams(t, dumpTestTime.Truncate(time.Millisecond), dumpTestTime.Add(time.Second)))
	require.ErrorIs(t, err, failure)
	require.Empty(t, result)
	require.Len(t, b.gets, 1)
}

func TestProfileDumpListMalformedKeysAndOrphans(t *testing.T) {
	b := newDumpTestBucket()
	key, metadata, _ := dumpFixture(t, "3648", dumpTestTime, nil)
	metadata.OriginalProfileID = "original-profile-id"
	body, err := json.Marshal(metadata)
	require.NoError(t, err)
	uploadDumpPair(t, b, key, body, nil)
	orphan, _, _ := dumpFixture(t, "3648", dumpTestTime, nil)
	require.NoError(t, b.Upload(context.Background(), orphan, strings.NewReader("orphan")))
	jsonOnly, _, jsonBody := dumpFixture(t, "3648", dumpTestTime, nil)
	jsonKeys, err := profiledump.ParseObjectKey(jsonOnly)
	require.NoError(t, err)
	require.NoError(t, b.Upload(context.Background(), jsonKeys.MetadataKey, bytes.NewReader(jsonBody)))
	require.NoError(t, b.Upload(context.Background(), key+"/child", strings.NewReader("nested")))
	other, _, _ := dumpFixture(t, "other", dumpTestTime, nil)
	earlier, _, _ := dumpFixture(t, "3648", dumpTestTime.Add(-time.Minute), nil)
	b.extraKeys = []string{
		"unrelated/key", other, earlier,
	}
	result, err := readDumpList(t, context.Background(), b, listDumpParams(t, dumpTestTime.Truncate(time.Minute), dumpTestTime.Truncate(time.Minute).Add(time.Minute)))
	require.NoError(t, err)
	require.ElementsMatch(t, []dumpListEntry{
		{Key: key, CapturedAt: dumpTestTime.Truncate(time.Millisecond), Labels: metadata.Labels, OriginalProfileID: metadata.OriginalProfileID},
		{Key: orphan, CapturedAt: dumpTestTime.Truncate(time.Millisecond), MetadataStatus: "missing"},
	}, result.Captures)
	require.ElementsMatch(t, []string{strings.TrimSuffix(key, ".pprof") + ".json", strings.TrimSuffix(orphan, ".pprof") + ".json"}, b.gets)
	require.Equal(t, b.opened, b.closed)
}

func TestProfileDumpListInvalidMetadata(t *testing.T) {
	b := newDumpTestBucket()
	key, _, _ := dumpFixture(t, "3648", dumpTestTime, nil)
	uploadDumpPair(t, b, key, []byte("{"), nil)
	later, valid, body := dumpFixture(t, "3648", dumpTestTime.Add(time.Minute), nil)
	uploadDumpPair(t, b, later, body, nil)
	result, err := readDumpList(t, context.Background(), b, listDumpParams(t, dumpTestTime, dumpTestTime.Add(2*time.Minute)))
	require.NoError(t, err)
	require.Equal(t, []dumpListEntry{
		{Key: key, CapturedAt: dumpTestTime.Truncate(time.Millisecond), MetadataStatus: "invalid"},
		{Key: later, CapturedAt: valid.CapturedAt.Truncate(time.Millisecond), Labels: valid.Labels},
	}, result.Captures)
	require.False(t, result.Limited)
	require.Equal(t, b.opened, b.closed)
}

func TestProfileDumpListMetadataFailures(t *testing.T) {
	b := newDumpTestBucket()
	prior, _, _ := dumpFixture(t, "3648", dumpTestTime, nil)
	key, _, _ := dumpFixture(t, "3648", dumpTestTime, nil)
	failure := errors.New("storage unavailable")
	b.getErr = failure
	b.errorKey = strings.TrimSuffix(key, ".pprof") + ".json"
	b.extraKeys = []string{prior, key}
	result, err := readDumpList(t, context.Background(), b, listDumpParams(t, dumpTestTime, dumpTestTime.Add(time.Second)))
	require.ErrorIs(t, err, failure)
	require.Empty(t, result)
	require.Len(t, b.gets, 2)
}

func TestProfileDumpListInvalidParameters(t *testing.T) {
	for _, tc := range []struct {
		name, tenant, from, to string
		limit                  int
	}{
		{"tenant", "../other", "2026-09-18T12:00:00Z", "2026-09-18T12:01:00Z", 100},
		{"from", "3648", "invalid", "2026-09-18T12:01:00Z", 100},
		{"to", "3648", "2026-09-18T12:00:00Z", "invalid", 100},
		{"before epoch", "3648", "1969-12-31T23:59:59Z", "2026-09-18T12:00:00Z", 100},
		{"after year 9999", "3648", "2026-09-18T12:00:00Z", "253402300800000", 100},
		{"reversed", "3648", "2026-09-18T12:01:00Z", "2026-09-18T12:00:00Z", 100},
		{"reversed within millisecond", "3648", "2026-09-18T12:00:00.1239Z", "2026-09-18T12:00:00.1231Z", 100},
		{"limit", "3648", "2026-09-18T12:00:00Z", "2026-09-18T12:01:00Z", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newDumpTestBucket()
			p := &profileDumpParams{tenant: tc.tenant, from: tc.from, to: tc.to, limit: tc.limit}
			require.Error(t, listProfileDumps(context.Background(), b, p, io.Discard))
			require.Empty(t, b.prefixes)
		})
	}
}

func TestProfileDumpExtractRepresentations(t *testing.T) {
	compressed, err := os.ReadFile("../../pkg/pprof/testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	for _, tc := range []struct {
		name    string
		payload []byte
	}{
		{"gzip", compressed},
		{"empty", []byte{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newDumpTestBucket()
			key, m, body := dumpFixture(t, "team-a", dumpTestTime, tc.payload)
			uploadDumpPair(t, b, key, body, tc.payload)
			k, err := profiledump.ParseObjectKey(key)
			require.NoError(t, err)
			t.Chdir(t.TempDir())
			p := &profileDumpParams{key: k.MetadataKey}
			var out bytes.Buffer
			require.NoError(t, extractProfileDump(context.Background(), b, p, &out))
			path := m.CaptureID + ".pprof"
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, tc.payload, got)
			metadata, err := os.ReadFile(path + ".metadata.json")
			require.NoError(t, err)
			require.Equal(t, body, metadata)
			for _, name := range []string{path, path + ".metadata.json"} {
				stat, err := os.Stat(name)
				require.NoError(t, err)
				require.Equal(t, os.FileMode(0600), stat.Mode().Perm())
			}
			var result map[string]string
			require.NoError(t, json.Unmarshal(out.Bytes(), &result))
			require.Equal(t, map[string]string{"payload": path, "metadata": path + ".metadata.json"}, result)
			require.Equal(t, []string{k.PayloadKey, k.MetadataKey}, b.gets)
			require.Equal(t, b.opened, b.closed)
		})
	}
}

func TestProfileDumpInspectMetadata(t *testing.T) {
	key, metadata, body := dumpFixture(t, "team-a", dumpTestTime, []byte("payload"))
	for _, tc := range []struct {
		name, body string
		want       profiledump.Metadata
	}{
		{"matching", string(body), metadata},
		{"mismatched identity and size", `{"schema_version":2,"tenant_id":"other","capture_id":"unknown","payload_size":-1,"extra":true}`, profiledump.Metadata{SchemaVersion: 2, TenantID: "other", CaptureID: "unknown", PayloadSize: -1}},
		{"null", "null", profiledump.Metadata{}},
		{"empty object", "{}", profiledump.Metadata{}},
		{"null size", `{"payload_size":null}`, profiledump.Metadata{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newDumpTestBucket()
			sidecar := strings.TrimSuffix(key, ".pprof") + ".json"
			// Inspection accepts a sidecar without a payload and ignores trailing whitespace.
			require.NoError(t, b.Upload(context.Background(), sidecar, strings.NewReader(tc.body+" \n\t")))
			var out bytes.Buffer
			require.NoError(t, printProfileDump(context.Background(), b, &profileDumpParams{key: key}, &out))
			var inspected struct {
				Key      string               `json:"key"`
				Metadata profiledump.Metadata `json:"metadata"`
			}
			decoder := json.NewDecoder(&out)
			decoder.DisallowUnknownFields()
			require.NoError(t, decoder.Decode(&inspected))
			require.Equal(t, key, inspected.Key)
			require.Equal(t, tc.want, inspected.Metadata)
			require.Equal(t, []string{sidecar}, b.gets)
			require.Equal(t, b.opened, b.closed)
		})
	}
}

func TestProfileDumpExtractRefusesOverwrite(t *testing.T) {
	for _, suffix := range []string{"", ".metadata.json"} {
		for _, symlink := range []bool{false, true} {
			t.Run(fmt.Sprintf("suffix=%q/symlink=%t", suffix, symlink), func(t *testing.T) {
				b := newDumpTestBucket()
				key, _, _ := dumpFixture(t, "3648", dumpTestTime, nil)
				path := filepath.Join(t.TempDir(), "payload")
				existing := path + suffix
				target := filepath.Join(t.TempDir(), "absent")
				if symlink {
					require.NoError(t, os.Symlink(target, existing))
				} else {
					require.NoError(t, os.WriteFile(existing, []byte("original"), 0600))
				}
				p := &profileDumpParams{key: key, path: path}
				var out bytes.Buffer
				err := extractProfileDump(context.Background(), b, p, &out)
				require.EqualError(t, err, fmt.Sprintf("output file %s already exists (use --force to overwrite)", existing))
				require.Empty(t, b.gets)
				require.Empty(t, out.String())
				for _, name := range []string{path, path + ".metadata.json"} {
					if name != existing {
						require.NoFileExists(t, name)
					}
				}
				if symlink {
					got, err := os.Readlink(existing)
					require.NoError(t, err)
					require.Equal(t, target, got)
					require.NoFileExists(t, target)
				} else {
					got, err := os.ReadFile(existing)
					require.NoError(t, err)
					require.Equal(t, "original", string(got))
				}
			})
		}
	}
}
func TestProfileDumpExtractRefusesExistingDirectory(t *testing.T) {
	b := newDumpTestBucket()
	key, _, _ := dumpFixture(t, "3648", dumpTestTime, nil)
	path := filepath.Join(t.TempDir(), "payload")
	require.NoError(t, os.Mkdir(path+".metadata.json", 0700))
	require.ErrorContains(t, extractProfileDump(context.Background(), b, &profileDumpParams{key: key, path: path}, io.Discard), "already exists")
	require.Empty(t, b.gets)
	require.NoFileExists(t, path)
	require.DirExists(t, path+".metadata.json")
}

func TestProfileDumpExtractForceOverwritesWithoutMetadataValidation(t *testing.T) {
	b := newDumpTestBucket()
	key, _, _ := dumpFixture(t, "3648", dumpTestTime, nil)
	sidecar := []byte("not JSON")
	uploadDumpPair(t, b, key, sidecar, []byte("payload"))
	path := filepath.Join(t.TempDir(), "payload")
	for _, name := range []string{path, path + ".metadata.json"} {
		require.NoError(t, os.WriteFile(name, bytes.Repeat([]byte("x"), 100), 0600))
	}
	require.NoError(t, extractProfileDump(context.Background(), b, &profileDumpParams{key: key, path: path, force: true}, io.Discard))
	payload, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "payload", string(payload))
	metadata, err := os.ReadFile(path + ".metadata.json")
	require.NoError(t, err)
	require.Equal(t, sidecar, metadata)
	require.Equal(t, b.opened, b.closed)
}

func TestProfileDumpExtractStorageFailures(t *testing.T) {
	failure := errors.New("storage unavailable")
	for _, tc := range []struct {
		name             string
		get, read, close error
		sidecar, missing bool
		want             error
	}{
		{name: "missing payload", missing: true, want: errDumpMissing},
		{name: "payload GET", get: failure, want: failure},
		{name: "payload read", read: failure, want: failure},
		{name: "sidecar close", close: failure, sidecar: true, want: failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newDumpTestBucket()
			key, _, body := dumpFixture(t, "3648", dumpTestTime, []byte("payload"))
			uploadDumpPair(t, b, key, body, []byte("payload"))
			if tc.missing {
				require.NoError(t, b.Delete(context.Background(), key))
			}
			b.errorKey = key
			if tc.sidecar {
				b.errorKey = strings.TrimSuffix(key, ".pprof") + ".json"
			}
			b.getErr, b.readErr, b.closeErr = tc.get, tc.read, tc.close
			p := &profileDumpParams{key: key, path: filepath.Join(t.TempDir(), "payload")}
			require.ErrorIs(t, extractProfileDump(context.Background(), b, p, io.Discard), tc.want)
			require.Equal(t, b.opened, b.closed)
		})
	}
}

func TestProfileDumpExtractOutputCreationFailure(t *testing.T) {
	b := newDumpTestBucket()
	key, _, _ := dumpFixture(t, "3648", dumpTestTime, nil)
	p := &profileDumpParams{key: key, path: filepath.Join(t.TempDir(), "absent", "payload")}
	require.ErrorContains(t, extractProfileDump(context.Background(), b, p, io.Discard), "open payload output")
	require.Empty(t, b.gets)
}

func TestProfileDumpExtractMissingSidecarPreservesPayload(t *testing.T) {
	b := newDumpTestBucket()
	key, _, _ := dumpFixture(t, "3648", dumpTestTime, nil)
	require.NoError(t, b.Upload(context.Background(), key, strings.NewReader("payload")))
	p := &profileDumpParams{key: key, path: filepath.Join(t.TempDir(), "payload")}
	require.ErrorIs(t, extractProfileDump(context.Background(), b, p, io.Discard), errDumpMissing)
	payload, err := os.ReadFile(p.path)
	require.NoError(t, err)
	require.Equal(t, "payload", string(payload))
	require.Equal(t, b.opened, b.closed)
}

func TestProfileDumpExtractResultFailurePreservesFiles(t *testing.T) {
	b := newDumpTestBucket()
	key, _, body := dumpFixture(t, "3648", dumpTestTime, []byte("payload"))
	uploadDumpPair(t, b, key, body, []byte("payload"))
	p := &profileDumpParams{key: key, path: filepath.Join(t.TempDir(), "payload")}
	require.ErrorContains(t, extractProfileDump(context.Background(), b, p, dumpFailWriter{}), "write failed")
	payload, err := os.ReadFile(p.path)
	require.NoError(t, err)
	require.Equal(t, "payload", string(payload))
	sidecar, err := os.ReadFile(p.path + ".metadata.json")
	require.NoError(t, err)
	require.Equal(t, body, sidecar)
	require.Equal(t, b.opened, b.closed)
}

type dumpFailWriter struct{}

func (dumpFailWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestProfileDumpPrefixAndOwnedClient(t *testing.T) {
	dir := t.TempDir()
	key, _, body := dumpFixture(t, "3648", dumpTestTime, []byte("payload"))
	k, err := profiledump.ParseObjectKey(key)
	require.NoError(t, err)
	path := filepath.Join(dir, "customer", key)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("payload"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "customer", k.MetadataKey), body, 0600))
	for _, invalid := range []bool{false, true} {
		p := parseDumpCommand(t, addProfileDumpInspectParams, dumpInspect, key, "--storage.backend=filesystem", "--storage.filesystem.dir", dir, "--storage.prefix=customer")
		var tracked *dumpTestBucket
		p.objectStoreCfg.Middlewares = append(p.objectStoreCfg.Middlewares, func(b objstore.Bucket) (objstore.Bucket, error) {
			tracked = &dumpTestBucket{Bucket: b}
			return tracked, nil
		})
		if invalid {
			p.key = "../invalid"
		}
		var out bytes.Buffer
		err := profileDump(withOutput(context.Background(), &out), p, printProfileDump)
		if invalid {
			require.ErrorIs(t, err, errDumpInvalid)
		} else {
			require.NoError(t, err)
			require.Contains(t, out.String(), key)
		}
		require.Equal(t, 1, tracked.clientClosed)
		require.Equal(t, tracked.opened, tracked.closed)
	}
}

func TestProfileDumpCommandParsing(t *testing.T) {
	key, _, _ := dumpFixture(t, "a", dumpTestTime, nil)
	for operation, addParams := range map[string]func(commander) *profileDumpParams{dumpList: addProfileDumpListParams, dumpInspect: addProfileDumpInspectParams, dumpExtract: addProfileDumpExtractParams} {
		t.Run(operation, func(t *testing.T) {
			args := []string{operation}
			if operation == dumpList {
				args = append(args, "--tenant-id=a")
			} else {
				args = append(args, key)
			}
			p := parseDumpCommand(t, addParams, args...)
			require.Equal(t, "filesystem", p.objectStoreCfg.Backend)
			require.Equal(t, "./data/v2/shared", p.objectStoreCfg.Filesystem.Directory)
			if operation == dumpList {
				require.Equal(t, "now-1h", p.from)
				require.Equal(t, "now", p.to)
				require.Equal(t, 100, p.limit)
			}
			app := kingpin.New("test", "test")
			addParams(app.Command(operation, "test"))
			_, err := app.Parse([]string{operation})
			require.Error(t, err)
		})
	}
	for _, flags := range [][]string{{"--force", "--output", "capture"}, {"-f", "-o", "capture"}} {
		p := parseDumpCommand(t, addProfileDumpExtractParams, append([]string{dumpExtract, key}, flags...)...)
		require.True(t, p.force)
		require.Equal(t, "capture", p.path)
	}
	p := parseDumpCommand(t, addProfileDumpExtractParams, dumpExtract, key)
	require.False(t, p.force)
	require.Empty(t, p.path)
}

func TestProfileDumpInvalidKey(t *testing.T) {
	b := newDumpTestBucket()
	p := &profileDumpParams{key: "../outside"}
	require.ErrorIs(t, printProfileDump(context.Background(), b, p, io.Discard), errDumpInvalid)
	require.ErrorIs(t, extractProfileDump(context.Background(), b, p, io.Discard), errDumpInvalid)
	require.Empty(t, b.gets)
}

func TestProfileDumpListUnorderedIteration(t *testing.T) {
	from := dumpTestTime.Truncate(time.Minute).Add(10 * time.Second)
	to := from.Add(10 * time.Second)
	b := newDumpTestBucket()
	var want string
	for _, at := range []time.Time{to, from.Add(-time.Millisecond), from} {
		key, _, body := dumpFixture(t, "3648", at, nil)
		b.extraKeys = append(b.extraKeys, key)
		if at.Equal(from) {
			want = key
			// Only sidecars are stored so iteration uses the deliberately unordered keys.
			require.NoError(t, b.Upload(context.Background(), strings.TrimSuffix(key, ".pprof")+".json", bytes.NewReader(body)))
		}
	}
	result, err := readDumpList(t, context.Background(), b, listDumpParams(t, from, to))
	require.NoError(t, err)
	require.False(t, result.Limited)
	require.Len(t, result.Captures, 1)
	require.Equal(t, want, result.Captures[0].Key)
	require.Equal(t, from, result.Captures[0].CapturedAt)
	require.Equal(t, []string{strings.TrimSuffix(want, ".pprof") + ".json"}, b.gets)
}
