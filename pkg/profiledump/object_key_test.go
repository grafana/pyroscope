package profiledump

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

func TestTenantEncoding(t *testing.T) {
	t.Parallel()
	for _, tenant := range []string{"tenant-a", ".", "..", "../other", `a\b`, "/absolute/path", "%2f%2e%2e", "租户/é", strings.Repeat("t", MaxTenantBytes)} {
		t.Run(tenant, func(t *testing.T) {
			encoded, err := EncodeTenant(tenant)
			require.NoError(t, err)
			require.NotEmpty(t, encoded)
			require.NotContains(t, encoded, "/")
			require.NotContains(t, encoded, `\`)
			require.NotContains(t, encoded, ".")
			decoded, err := DecodeTenant(encoded)
			require.NoError(t, err)
			require.Equal(t, tenant, decoded)
		})
	}
	encoded, err := EncodeTenant("tenant-a")
	require.NoError(t, err)
	require.Equal(t, "dGVuYW50LWE", encoded, "pin the encoding convention")
	for _, tenant := range []string{"", "\xff", "a\nb", "\x00", strings.Repeat("t", MaxTenantBytes+1)} {
		_, err := EncodeTenant(tenant)
		require.Error(t, err)
		_, err = DecodeTenant(base64.RawURLEncoding.EncodeToString([]byte(tenant)))
		require.Error(t, err)
	}
	for _, segment := range []string{"", ".", "..", "../YQ", `YQ\`, "YQ==", "YQ=", "YR", "YQ\n", "YQ\r\n", "%59Q", "YQ/", "YQ+", "a", strings.Repeat("A", 1000)} {
		_, err := DecodeTenant(segment)
		require.Error(t, err, "accepted %q", segment)
	}
}

func TestObjectKeyRoundTrip(t *testing.T) {
	t.Parallel()
	for _, format := range []Format{FormatPprof, FormatJFR, FormatOTLP, FormatTrie, FormatTree, FormatLines, FormatGroups, FormatSpeedscope} {
		t.Run(string(format), func(t *testing.T) {
			capturedAt := time.Date(2026, 9, 16, 12, 34, 56, 123456789, time.UTC)
			key, id, err := newObjectKey("../tenant/with\\separators", capturedAt, format, bytes.NewReader(make([]byte, 10)))
			require.NoError(t, err)
			parsed, err := ParseObjectKey(key)
			require.NoError(t, err)
			require.Equal(t, "../tenant/with\\separators", parsed.TenantID)
			require.Equal(t, id, parsed.CaptureID)
			require.Equal(t, capturedAt.Truncate(time.Millisecond), parsed.CaptureTime)
			require.Equal(t, format, parsed.Format)
			require.Equal(t, 6, len(strings.Split(key, "/")))
		})
	}
}

func TestObjectKeyCaptureTime(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ timestamp, partition string }{
		{"2026-09-16T23:59:59.999999999-07:00", "2026/09/17"},
		{"2026-09-16T00:00:00+07:00", "2026/09/15"},
		{"2026-12-31T23:59:59.999999999Z", "2026/12/31"},
		{"2027-01-01T00:00:00Z", "2027/01/01"},
		{"2024-02-29T00:00:00Z", "2024/02/29"},
		{"1970-01-01T00:00:00Z", "1970/01/01"},
		{"9999-12-31T23:59:59.999999999Z", "9999/12/31"},
	} {
		t.Run(tc.timestamp, func(t *testing.T) {
			capturedAt, err := time.Parse(time.RFC3339Nano, tc.timestamp)
			require.NoError(t, err)
			key, id, err := newObjectKey("a", capturedAt, FormatPprof, bytes.NewReader(make([]byte, 10)))
			require.NoError(t, err)
			require.Equal(t, ObjectPrefix+"YQ/"+tc.partition+"/"+id.String()+"-pprof.pyrdump", key)
			require.Equal(t, ulid.Timestamp(capturedAt), id.Time())
			parsed, err := ParseObjectKey(key)
			require.NoError(t, err)
			require.True(t, capturedAt.Truncate(time.Millisecond).Equal(parsed.CaptureTime))
			require.Equal(t, time.UTC, parsed.CaptureTime.Location())
		})
	}
	for _, capturedAt := range []time.Time{{}, time.Unix(-1, 999999999), time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)} {
		_, _, err := newObjectKey("a", capturedAt, FormatPprof, bytes.NewReader(make([]byte, 10)))
		require.Error(t, err)
	}
}

func TestObjectKeyFreshIDs(t *testing.T) {
	t.Parallel()
	capturedAt := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	// Deterministic entropy yields distinct IDs for the same capture millisecond.
	entropy := bytes.NewReader(append(make([]byte, 10), bytes.Repeat([]byte{1}, 10)...))
	key1, id1, err := newObjectKey("a", capturedAt, FormatJFR, entropy)
	require.NoError(t, err)
	key2, id2, err := newObjectKey("a", capturedAt, FormatJFR, entropy)
	require.NoError(t, err)
	require.NotEqual(t, id1, id2)
	require.NotEqual(t, key1, key2)
	require.Equal(t, id1.Time(), id2.Time())
	_, _, err = newObjectKey("a", capturedAt, FormatJFR, entropy)
	require.Error(t, err, "entropy failures propagate")
	// Exercise the production entropy source without probabilistic assertions.
	key, id, err := NewObjectKey("a", capturedAt, FormatJFR)
	require.NoError(t, err)
	parsed, err := ParseObjectKey(key)
	require.NoError(t, err)
	require.Equal(t, id, parsed.CaptureID)
	_, _, err = NewObjectKey("", capturedAt, FormatJFR)
	require.Error(t, err)
	_, _, err = NewObjectKey("a", capturedAt, Format("../pprof"))
	require.Error(t, err)
}

func TestParseObjectKeyRejectsMalformed(t *testing.T) {
	t.Parallel()
	capturedAt := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	key, id, err := newObjectKey("a", capturedAt, FormatPprof, bytes.NewReader(bytes.Repeat([]byte{255}, 10)))
	require.NoError(t, err)
	for name, bad := range map[string]string{
		"empty":                       "",
		"unrelated":                   "blocks/" + strings.TrimPrefix(key, ObjectPrefix),
		"prefix lookalike":            "profile-debug-dumps-evil/" + strings.TrimPrefix(key, ObjectPrefix),
		"absolute":                    "/" + key,
		"traversal prefix":            "../" + key,
		"empty tenant":                strings.Replace(key, "/YQ/", "//", 1),
		"tenant traversal":            strings.Replace(key, "/YQ/", "/../", 1),
		"tenant backslash":            strings.Replace(key, "/YQ/", `/..\evil/`, 1),
		"tenant percent escaped":      strings.Replace(key, "/YQ/", "/%2e%2e/", 1),
		"tenant padding":              strings.Replace(key, "/YQ/", "/YQ==/", 1),
		"tenant unused bits":          strings.Replace(key, "/YQ/", "/YR/", 1),
		"extra segment":               strings.Replace(key, "/2026/", "/extra/2026/", 1),
		"date traversal":              strings.Replace(key, "/09/", "/../", 1),
		"invalid month":               strings.Replace(key, "/09/", "/13/", 1),
		"invalid day":                 strings.Replace(key, "/16/", "/31/", 1),
		"non leap day":                strings.Replace(key, "/09/16/", "/02/29/", 1),
		"date padding":                strings.Replace(key, "/09/", "/9/", 1),
		"date mismatch":               strings.Replace(key, "/16/", "/17/", 1),
		"short ID":                    strings.Replace(key, id.String(), id.String()[1:], 1),
		"lowercase ID":                strings.Replace(key, id.String(), strings.ToLower(id.String()), 1),
		"ambiguous ID alphabet":       strings.Replace(key, id.String(), strings.Repeat("I", 26), 1),
		"ULID overflow":               strings.Replace(key, id.String(), "8"+strings.Repeat("0", 25), 1),
		"ULID outside supported date": strings.Replace(key, id.String(), "7"+strings.Repeat("Z", 25), 1),
		"unknown format":              strings.Replace(key, "-pprof.", "-unknown.", 1),
		"uppercase format":            strings.Replace(key, "-pprof.", "-PPROF.", 1),
		"missing format":              strings.Replace(key, "-pprof.", "-.", 1),
		"missing hyphen":              strings.Replace(key, "-pprof.", "pprof.", 1),
		"wrong suffix":                strings.TrimSuffix(key, ".pyrdump") + ".gz",
		"trailing slash":              key + "/",
		"trailing bytes":              key + "\x00",
		"query":                       key + "?x=1",
		"too long":                    key + strings.Repeat("x", 1024),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseObjectKey(bad)
			require.Error(t, err, "accepted %q", bad)
		})
	}
}

func FuzzObjectKey(f *testing.F) {
	f.Add("profile-debug-dumps/YQ/1970/01/01/00000000000000000000000000-pprof.pyrdump")
	f.Add("profile-debug-dumps/../2026/09/16/invalid-jfr.pyrdump")
	f.Add("")
	f.Fuzz(func(t *testing.T, key string) {
		parsed, err := ParseObjectKey(key)
		if err != nil {
			return
		}
		tenant, err := EncodeTenant(parsed.TenantID)
		require.NoError(t, err)
		canonical := fmt.Sprintf("%s%s/%s/%s-%s.pyrdump", ObjectPrefix, tenant, parsed.CaptureTime.Format("2006/01/02"), parsed.CaptureID, parsed.Format)
		require.Equal(t, key, canonical)
	})
}

func FuzzTenantEncoding(f *testing.F) {
	for _, tenant := range []string{"a", "../x", `a\b`, "租户", "\xff"} {
		f.Add(tenant)
	}
	f.Fuzz(func(t *testing.T, tenant string) {
		encoded, err := EncodeTenant(tenant)
		if err != nil {
			return
		}
		require.False(t, strings.ContainsAny(encoded, `/\.`))
		decoded, err := DecodeTenant(encoded)
		require.NoError(t, err)
		require.Equal(t, tenant, decoded)
	})
}
