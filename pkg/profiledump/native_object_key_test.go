package profiledump

import (
	"strings"
	"testing"
	"time"

	"github.com/grafana/dskit/tenant"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

func TestNativeObjectKeyTenants(t *testing.T) {
	t.Parallel()
	capturedAt := time.Date(2026, 9, 25, 12, 34, 56, 123456789, time.UTC)
	for _, tenantID := range []string{"3648", "tenant-a", "Team_A.42", "!-_.*'()", "...", strings.Repeat("t", tenant.MaxTenantIDLength)} {
		t.Run(tenantID, func(t *testing.T) {
			key, id, err := NewNativeObjectKey(tenantID, capturedAt)
			require.NoError(t, err)
			require.Equal(t, NativeObjectPrefix+tenantID+"/2026-09-25/12/34/"+id.String()+".pprof", key)
			parsed, err := ParseNativeObjectKey(key)
			require.NoError(t, err)
			require.Equal(t, tenantID, parsed.TenantID)
			require.Equal(t, id, parsed.CaptureID)
			require.Equal(t, capturedAt.Truncate(time.Millisecond), parsed.CaptureTime)
			require.Equal(t, key, parsed.PayloadKey)
			require.Equal(t, strings.TrimSuffix(key, ".pprof")+".json", parsed.MetadataKey)
			sibling, err := ParseNativeObjectKey(parsed.MetadataKey)
			require.NoError(t, err)
			require.Equal(t, parsed, sibling)
		})
	}
	for _, tenantID := range []string{"", ".", "..", "../a", "a/b", `a\b`, "a|b", "%2e%2e", "a b", "a\nb", "a\x00b", "租户", "\xff", strings.Repeat("t", tenant.MaxTenantIDLength+1)} {
		t.Run("invalid "+tenantID, func(t *testing.T) {
			_, _, err := NewNativeObjectKey(tenantID, capturedAt)
			require.Error(t, err)
			_, err = ParseNativeObjectKey(NativeObjectPrefix + tenantID + "/2026-09-25/12/34/01M3CAE8HV0000000000000000.pprof")
			require.Error(t, err)
		})
	}
}

func TestNativeObjectKeyTimeBoundaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ timestamp, partition string }{
		{"2026-09-25T12:34:59.999999999Z", "2026-09-25/12/34"},
		{"2026-09-25T12:35:00Z", "2026-09-25/12/35"},
		{"2026-09-25T12:59:59.999999999Z", "2026-09-25/12/59"},
		{"2026-09-25T13:00:00Z", "2026-09-25/13/00"},
		{"2026-12-31T23:59:59.999999999Z", "2026-12-31/23/59"},
		{"2027-01-01T00:00:00Z", "2027-01-01/00/00"},
		{"2026-09-25T23:59:59.999999999-07:00", "2026-09-26/06/59"},
		{"2026-09-25T00:00:00+07:00", "2026-09-24/17/00"},
		{"2024-02-29T00:00:00Z", "2024-02-29/00/00"},
		{"1970-01-01T00:00:00Z", "1970-01-01/00/00"},
		{"9999-12-31T23:59:59.999999999Z", "9999-12-31/23/59"},
	} {
		t.Run(tc.timestamp, func(t *testing.T) {
			capturedAt, err := time.Parse(time.RFC3339Nano, tc.timestamp)
			require.NoError(t, err)
			key, id, err := NewNativeObjectKey("3648", capturedAt)
			require.NoError(t, err)
			require.Equal(t, NativeObjectPrefix+"3648/"+tc.partition+"/"+id.String()+".pprof", key)
			require.Equal(t, ulid.Timestamp(capturedAt), id.Time())
			parsed, err := ParseNativeObjectKey(key)
			require.NoError(t, err)
			require.True(t, capturedAt.Truncate(time.Millisecond).Equal(parsed.CaptureTime))
			require.Equal(t, time.UTC, parsed.CaptureTime.Location())
		})
	}
	for _, capturedAt := range []time.Time{{}, time.Unix(-1, 999999999), time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)} {
		_, _, err := NewNativeObjectKey("3648", capturedAt)
		require.Error(t, err)
	}
}

func TestNativeObjectKeyRejectsNearMatches(t *testing.T) {
	t.Parallel()
	key, id, err := NewNativeObjectKey("3648", time.Date(2026, 9, 25, 12, 34, 0, 0, time.UTC))
	require.NoError(t, err)
	for name, bad := range map[string]string{
		"empty":            "",
		"namespace":        strings.Replace(key, "native/", "native-other/", 1),
		"root":             strings.Replace(key, ObjectPrefix, "profile-debug-dumps-other/", 1),
		"old namespace":    strings.Replace(key, "native/", "", 1),
		"absolute":         "/" + key,
		"traversal":        "../" + key,
		"extra segment":    strings.Replace(key, "/3648/", "/3648/extra/", 1),
		"duplicate slash":  strings.Replace(key, "/3648/", "/3648//", 1),
		"invalid month":    strings.Replace(key, "2026-09-25", "2026-13-25", 1),
		"invalid day":      strings.Replace(key, "2026-09-25", "2026-09-31", 1),
		"non leap day":     strings.Replace(key, "2026-09-25", "2026-02-29", 1),
		"date padding":     strings.Replace(key, "2026-09-25", "2026-9-25", 1),
		"hour padding":     strings.Replace(key, "/12/", "/1/", 1),
		"minute padding":   strings.Replace(key, "/34/", "/4/", 1),
		"hour range":       strings.Replace(key, "/12/", "/24/", 1),
		"minute range":     strings.Replace(key, "/34/", "/60/", 1),
		"date mismatch":    strings.Replace(key, "2026-09-25", "2026-09-24", 1),
		"hour mismatch":    strings.Replace(key, "/12/", "/11/", 1),
		"minute mismatch":  strings.Replace(key, "/34/", "/33/", 1),
		"short ID":         strings.Replace(key, id.String(), id.String()[1:], 1),
		"lowercase ID":     strings.Replace(key, id.String(), strings.ToLower(id.String()), 1),
		"invalid alphabet": strings.Replace(key, id.String(), strings.Repeat("I", 26), 1),
		"overflow":         strings.Replace(key, id.String(), "8"+strings.Repeat("0", 25), 1),
		"unsupported date": strings.Replace(key, id.String(), "7"+strings.Repeat("Z", 25), 1),
		"uppercase suffix": strings.TrimSuffix(key, ".pprof") + ".PPROF",
		"wrong suffix":     strings.TrimSuffix(key, ".pprof") + ".pyrdump",
		"empty suffix":     strings.TrimSuffix(key, ".pprof"),
		"extra suffix":     key + ".json",
		"trailing slash":   key + "/",
		"query":            key + "?x=1",
		"trailing control": key + "\x00",
		"too long":         key + strings.Repeat("x", 1000),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseNativeObjectKey(bad)
			require.Error(t, err, "accepted %q", bad)
		})
	}
}
