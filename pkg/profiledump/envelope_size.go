package profiledump

import (
	"strconv"
	"time"
)

// encodedMetadataSize counts JSON bytes without building a buffer. Validate first.
// Keep in sync with Metadata and the JSON parity tests.
func encodedMetadataSize(m Metadata) int64 {
	n := 2 // braces
	fields := 0
	field := func(name string, size int64) {
		if fields > 0 {
			n++
		}
		fields++
		n += len(name) + 3 + int(size) // quoted field name and colon
	}
	str := func(name, value string) { field(name, jsonStringSize(value)) }
	var tmp [64]byte
	field("schema_version", int64(len(strconv.AppendUint(tmp[:0], uint64(m.SchemaVersion), 10))))
	str("captured_at", string(m.CapturedAt.AppendFormat(tmp[:0], time.RFC3339Nano)))
	str("tenant_id", m.TenantID)
	str("source_protocol", string(m.SourceProtocol))
	str("native_format", string(m.NativeFormat))
	if m.Incoming != nil {
		field("incoming", representationSize(*m.Incoming))
	}
	field("stored", representationSize(m.Stored))
	if m.Legacy != nil {
		field("legacy", legacyMetadataSize(*m.Legacy))
	}
	if m.HTTP != nil {
		field("http", httpMetadataSize(*m.HTTP))
	}
	if len(m.Labels) > 0 {
		size := int64(2)
		for k, v := range m.Labels {
			size += jsonStringSize(k) + 1 + jsonStringSize(v) + 1
		}
		field("labels", size-1)
	}
	if m.OriginalProfileID != "" {
		str("original_profile_id", m.OriginalProfileID)
	}
	str("distributor_id", m.DistributorID)
	str("activation_source", m.ActivationSource)
	str("policy_fingerprint", m.PolicyFingerprint)
	str("capture_id", m.CaptureID)
	field("payload_size", int64(len(strconv.AppendInt(tmp[:0], m.PayloadSize, 10))))
	return int64(n)
}

func representationSize(r Representation) int64 {
	return int64(len(`{"content_type":,"encoding":,"syntax":}`)) + jsonStringSize(r.ContentType) + jsonStringSize(r.Encoding) + jsonStringSize(r.Syntax)
}

func jsonStringSize(s string) int64 {
	n := int64(len(s) + 2)
	for _, c := range s {
		switch c {
		case '"', '\\':
			n++
		case '<', '>', '&':
			n += 5 // encoding/json escapes HTML by default
		case '\u2028', '\u2029':
			n += 3
		}
	}
	return n
}

func legacyMetadataSize(m LegacyMetadata) int64 {
	var tmp [64]byte
	n := int64(len(`{"name":,"start_time":,"end_time":,"sample_rate":,"spy":,"units":,"aggregation":,"declared_format":}`))
	for _, s := range []string{m.Name, m.Spy, m.Units, m.Aggregation, m.DeclaredFormat} {
		n += jsonStringSize(s)
	}
	n += jsonStringSize(string(m.StartTime.AppendFormat(tmp[:0], time.RFC3339Nano)))
	n += jsonStringSize(string(m.EndTime.AppendFormat(tmp[:0], time.RFC3339Nano)))
	return n + int64(len(strconv.AppendUint(tmp[:0], uint64(m.SampleRate), 10)))
}
