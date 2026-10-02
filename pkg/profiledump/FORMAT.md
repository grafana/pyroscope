# Profile dump format, version 1

## Format overview

Each capture is stored as one self-contained object containing bounded metadata
followed by the native payload bytes unchanged. Keeping both in one object avoids
the coordination and failure modes of separate metadata and payload objects.

The fixed header records the format version and metadata length:

```text
magic | version | metadata length | metadata | untouched payload
```

This makes the metadata independently inspectable while leaving the payload opaque
and preserving the original bytes. Representing binary payloads compactly in JSON
would require a text encoding such as base64, adding encoding overhead and increasing
the object size. A protobuf wrapper could provide a similar storage model, but the same
access pattern would still require a way to determine the metadata boundary before reading
the payload.

## Layout

Each object contains a 14-byte header, UTF-8 JSON metadata, and opaque native payload.

| Offset | Width | Value |
| --- | --- | --- |
| 0 | 8 bytes | ASCII `PYRDUMP` + LF (`50 59 52 44 55 4d 50 0a`) |
| 8 | 2 bytes | Unsigned big-endian envelope version |
| 10 | 4 bytes | Unsigned big-endian metadata byte length |
| 14 | metadata length | JSON object with matching `schema_version` |
| 14 + metadata length | `payload_size` | Unmodified payload |

## Compatibility and limits

The envelope and metadata schema both use version 1. Unreleased development formats
are unsupported. Unsupported header versions are rejected before parsing JSON.
The header version identifies the framing, while `schema_version` keeps decoded
metadata self-describing. Metadata validation requires the schema version to match.

Unknown JSON fields are rejected. Missing scalars decode to Go zero values and
undergo validation. `payload_size` must be explicitly present and non-null. JSON
field names use the documented lowercase spelling. Incidental case-folding by
`encoding/json` is not a compatibility guarantee.

Metadata must be between 1 and 65,536 bytes. Required `payload_size` is a nonnegative
int64. The complete object must fit int64 and the caller's positive `maxObjectSize`.
Empty payloads are valid. Encoding and decoding require EOF immediately after
the declared payload, rejecting short payloads, trailing bytes, and concatenated
objects. The EOF check may consume an extra byte. Do not hide trailing bytes with
a reader limited to the declared size.

`Encode` and `Decode` stream payloads with bounded buffering. They retain or close
no inputs. Keep inputs stable during calls and own any asynchronous copies.
Discard partial output on error. Readers/writers supply cancellation and deadlines.
Use `io.Discard` to validate without retaining the decoded payload.

`Inspect` reads only header and metadata, returning offsets, declared sizes, and
owned metadata for range reads. `HeaderSize` and `MaxMetadataSize` bound inspection.
It cannot confirm payload completeness. The format has no checksum, authentication,
or native-payload validation, so even full decoding cannot detect same-length corruption.

## Metadata

Metadata identifies capture time, tenant, capture ID, source protocol (`connect`),
native format (`pprof`), distributor instance, original profile ID when available,
selected request-series labels, activation source (`runtime_override`), policy
fingerprint, and payload size. Capture time and ID must agree to the millisecond.

`payload_encoding` describes the native bytes as `gzip`, `identity`, or `unknown`.
It is a declaration by the capture adapter, not a codec validation result. The
codec never parses or decompresses pprof. Malformed pprof and malformed gzip are
valid opaque payloads. Compressed bytes are preserved exactly. No HTTP headers,
legacy fields, or OTLP-specific metadata are part of this schema.

Text must be valid UTF-8 without control characters. Limits count bytes:

| Field | Limit |
| --- | --- |
| Tenant | 256 |
| Ordinary text / label value | 1024 |
| Label name | 128 |
| Selected labels | 32 entries, 8192 combined name/value bytes |

The policy fingerprint is the policy's 64-character lowercase SHA-256 identifier.
It does not anonymize labels or payloads. The Connect adapter selects valid series
labels in wire order within the limits above, omitting invalid, duplicate and
over-budget entries. Values are preserved without sanitization and may contain
credentials. The adapter does not collect transport headers.

## Object keys

```
profile-debug-dumps/<tenant>/<YYYY>/<MM>/<DD>/<ULID>-<format>.pyrdump
```

Tenant is canonical, unpadded RFC 4648 base64url of its UTF-8 ID, with zero unused
bits. Separators and traversal strings encode to one safe segment. Parsing never
unescapes or cleans paths. Keys contain no service names or labels.

The only current native format is `pprof`.
ULIDs must be canonical uppercase. `NewObjectKey` uses server capture time from
1970 through 9999 UTC for both the date partition and a ULID with fresh cryptographic
entropy. Parsed time has millisecond precision. Metadata may retain finer precision
but must match the ULID's millisecond. Date/ULID mismatches are rejected.
Use capture time, never client profile time. IDs do not guarantee deduplication.
These helpers perform no storage operations.

## Encoded object size

The encoder first validates field lengths and collection counts, then calls
`json.Marshal` once. The input bounds constrain that initial allocation. It checks
the actual encoded length against the 64 KiB metadata limit and calculates
`HeaderSize + len(metadataJSON) + payload_size` without int64 overflow. JSON
escaping expansion is included in the complete object size.

JSON metadata uses standard serialization and can be read with ordinary JSON tools.
For recorder allocation, reservation and release rules, see
[Memory and ownership](RECORDER.md#memory-and-ownership).
