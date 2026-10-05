# Native profile dumps, schema version 1

## Native representation

Each capture has two logical object names with the same stem:

```text
__pyroscope_cluster/profile-debug-dumps/native/<actual-tenant>/YYYY-MM-DD/HH/MM/<ULID>.pprof
__pyroscope_cluster/profile-debug-dumps/native/<actual-tenant>/YYYY-MM-DD/HH/MM/<ULID>.json
```

`ValidateNativeTenant` requires a nonempty ID accepted by
`dskit/tenant.ValidTenantID`.
Valid nonnumeric IDs are supported. There is no escaping, normalization, numeric
restriction, or new character policy. The shared validator currently limits IDs
to 150 bytes. Storage-prefix and tenant-encryption wrappers continue to apply
outside these logical keys, with no additional tenant prefix or storage client.

`NewNativeObjectKey` uses the same server capture time for a random ULID and the
UTC minute partition. Supported capture years are 1970 through 9999 UTC. Keys use
canonical uppercase ULIDs, exact lowercase suffixes, and zero-padded date, hour,
and minute components. `ParseNativeObjectKey` validates the complete namespace,
tenant, shape, suffix, calendar/time components, and ULID/partition agreement.
It returns both sibling names only after validation, without cleaning or
unescaping paths. Parsed capture time has millisecond precision.

The `.pprof` object contains the exact incoming Connect `RawProfile` bytes.
Compressed, uncompressed, empty, and malformed payloads are all valid captures.
Neither key nor metadata helpers parse, validate, decompress, or rewrite native
payloads. The suffix does not assert that pprof can parse the capture. Uploads must
not set content encoding that causes download-time transformation. Valid native
captures can be downloaded directly and opened with ordinary pprof tools.

## JSON sidecar

The sidecar is one UTF-8 JSON object with schema version 1.

| Field | Description |
| --- | --- |
| `schema_version` | Required, `1` |
| `tenant_id` | Required, exactly the validated key's actual tenant |
| `capture_id` | Required, exactly the key's canonical ULID |
| `captured_at` | Required, RFC 3339 time matching the ULID millisecond and partition |
| `payload_size` | Required, non-null, nonnegative int64 byte count, including zero |
| `payload_encoding` | Required, `identity`, `gzip`, or `unknown` |
| `source_protocol` | Required, `connect` |
| `native_format` | Required, `pprof` |
| `distributor_id` | Required, nonempty, at most 1024 bytes |
| `labels` | Optional, at most 32 entries and 8192 combined name/value bytes |
| `original_profile_id` | Optional, at most 1024 bytes |

Label names are nonempty and at most 128 bytes. Label values are at most 1024
bytes. Text must be valid UTF-8 without control characters. Existing bounded
Connect label selection remains applicable. Labels and payloads can contain
credentials. Transport headers are not collected.

Sidecar capture time may retain sub-millisecond precision. Listing filters the
key-derived millisecond time and does not read this field.
Sidecar serialization emits it in UTC. Equivalent RFC 3339 offsets are accepted
on read when the instant agrees with the key. Encoding is a declaration from the
capture adapter, not a validation result. No payload path, activation source,
binary framing fields, checksum, transaction state, or extension map is included.

Before allocating JSON, capture preparation validates field bounds, nonnegative
payload size, and identity/time agreement with the complete native key. It then
calls ordinary `encoding/json.Marshal` once and checks the actual serialized length.
The sidecar limit is 65,536 bytes, including JSON escaping expansion. There is no
manual serialized-size prediction.

`ReadNativeMetadata(reader, key)` reads at most 65,537 bytes before rejecting an
oversized sidecar, then decodes the buffered input with `json.Unmarshal`. Unknown
JSON fields are ignored and are not preserved. The reader rejects empty input,
invalid UTF-8, malformed JSON, unsupported schema versions, missing/null
`payload_size`, invalid fields, and identity/time mismatch. Exactly one JSON object
is accepted, with optional surrounding JSON whitespace counted toward the limit. A second JSON value or any
other trailing non-whitespace data is rejected. Field names use the documented
lowercase spelling. Incidental decoder behavior is not a compatibility promise.

A storage read error remains an error even if valid JSON preceded it.

## Storage and retrieval

The capture size limit covers payload plus serialized JSON. The payload is uploaded
first and the sidecar second under one deadline through the existing bucket
wrappers. Success requires both uploads. An uncertain failure can leave either
file or both, with no rollback, repair, or atomicity guarantee.

Listing discovers validated native `.pprof` keys in intersecting UTC minute
directories without GET or Attributes calls. It reports payload keys and ULID
capture timestamps at millisecond resolution, filtering those timestamps directly
against `[from,to)`. It counts invalid keys and reports partial results on limits
or errors. Payload-only orphans are discoverable. JSON-only orphans are not results.

Inspection validates the bounded sidecar and checks payload existence and size
through Attributes without reading payload bytes. Storage paths are derived by
`ParseNativeObjectKey`. `payload_size` must match the stored payload and, on
extraction, the streamed byte count. Identity/time checks do not detect same-length
substitution. A listed payload may fail inspection. Ordinary object tools can
recover payload-only orphans independently of validated metadata.

Cleanup deletes individually validated native keys from eligible hours, including
either orphan form, without reading sidecar contents. A recent ULID placed in an
old partition fails key validation. There is no directory or broad-prefix delete.

Use `profilecli admin profile-dumps list`, `inspect`, and `extract` for discovery and validated
retrieval. Extraction preserves payload bytes exactly and refuses to overwrite
existing files or symlinks. See the [CLI reference](../../cmd/profilecli/PROFILE_DUMP.md)
for explicit output paths, partial listing results, and cooperative cancellation
limits, [recorder contracts](RECORDER.md) for activation and bounds, and
[cleaner contracts](CLEANER.md) for eventual retention.
