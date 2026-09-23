# Profile dump format, versions 1–3

Each object contains a 14-byte header, UTF-8 JSON metadata, and opaque native payload.

| Offset | Width | Value |
| --- | --- | --- |
| 0 | 8 bytes | ASCII `PYRDUMP` + LF (`50 59 52 44 55 4d 50 0a`) |
| 8 | 2 bytes | Unsigned big-endian envelope version |
| 10 | 4 bytes | Unsigned big-endian metadata byte length |
| 14 | metadata length | JSON object with matching `schema_version` |
| 14 + metadata length | `payload_size` | Unmodified payload |

## Compatibility and limits

The codec supports versions 1–3. The recorder uses version 3 for all sources.
Version 2 added `legacy`. Version 3 added `http` and representation encoding `unknown`.
Version 1 rejects `legacy`, and versions 1–2 reject `http`, even null or case-folded
field names. Older readers reject newer versions before parsing JSON.
Unknown versions and JSON fields, including nested fields, are rejected.
Absent or null optional blocks mean unavailable metadata. Legacy blocks require
an ingest source, and missing scalars decode to Go zero values.

Metadata must occupy 1–65536 bytes. Required `payload_size` is a nonnegative int64.
The complete object must fit int64 and the caller's positive `maxObjectSize`.
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
It cannot confirm payload completeness. No version has a checksum, authentication,
or native-payload validation, so even full decoding cannot detect same-length corruption.

## Metadata

Metadata identifies the source protocol, native format, and incoming/stored
representations. Each representation includes content type with parameters
(including multipart boundary), encoding, and syntax. Incoming may be unavailable
to typed handlers. Stored describes the actual bytes, such as identity encoding
after removing one gzip layer, `unknown` for ambiguous remaining encodings, or
protobuf for decoded OTLP received as JSON. The codec does not infer these fields.

Text must be valid UTF-8 without control characters. Limits count bytes:

| Field | Limit |
| --- | --- |
| Tenant | 256 |
| Ordinary text / label value | 1024 |
| Label name | 128 |
| Selected labels | 32 entries, 8192 combined name/value bytes |

The policy fingerprint is the policy's 64-character lowercase SHA-256 identifier.
It does not anonymize labels or payloads. Adapters must exclude credentials and
arbitrary request data, supplying only selected identity labels and explicit fields.

## Object keys

```
profile-debug-dumps/<tenant>/<YYYY>/<MM>/<DD>/<ULID>-<format>.pyrdump
```

Tenant is canonical, unpadded RFC 4648 base64url of its UTF-8 ID, with zero unused
bits. Separators and traversal strings encode to one safe segment. Parsing never
unescapes or cleans paths. Keys contain no service names or labels.

Formats are `pprof`, `jfr`, `otlp`, `trie`, `tree`, `lines`, `groups`, and `speedscope`.
ULIDs must be canonical uppercase. `NewObjectKey` uses server capture time from
1970 through 9999 UTC for both the date partition and a ULID with fresh cryptographic
entropy. Parsed time has millisecond precision. Metadata may retain finer precision
but must match the ULID's millisecond. Date/ULID mismatches are rejected.
Use capture time, never client profile time. IDs do not guarantee deduplication.
These helpers perform no storage operations.

## Legacy `/ingest`

The `legacy` block contains `name`, `start_time`, `end_time`, `sample_rate` (uint32),
`spy`, `units`, `aggregation`, and `declared_format`. The handler writes every scalar
using effective parser values, including sample-rate fallback and default times.
Name is the parsed application name. Profile times are UTC RFC 3339 timestamps,
separate from capture time used for keys and retention. Omitted declared format is
empty. `native_format` records the adapter's effective format, including multipart pprof.

Top-level `labels` contains the complete parsed map, including `__name__`, before
conversion or normalization. Selection uses the same map without service-name mapping.
The label limits apply to the whole map. Legacy strings allow 1024 bytes each and
4096 combined. The 64 KiB wire limit includes escaping, labels, and extensions.
Invalid metadata drops only capture. Label preflight precedes selector allocation,
sorting, and recorder admission, so failures produce no per-candidate metrics or spans.
Object limits and recorder reservations include encoded metadata. Encoding workspace is bounded.

Stored content type retains valid supplied parameters, including multipart boundary.
Missing types use `application/octet-stream`. Unparseable types or multipart types
without a boundary use `application/octet-stream` with binary syntax. Invalid declarations are discarded,
while oversized content types drop capture. Stored encoding is `gzip` for a gzip
signature, otherwise `identity`. This sniffing does not validate compression.
Syntax follows the effective format, or `multipart` for multipart content types.
Compression within multipart sections does not affect the outer representation.
Invalid representation metadata drops only capture.

Incoming is omitted. No HTTP Content-Encoding or unrelated headers are copied.
The full body, including all multipart sections, is copied once into recorder-owned
storage before conversion. Neither this capture path nor the codec decompresses
or rewrites it.

## OTLP HTTP encoding metadata

Version 3's optional `http` block contains `content_encoding_base64`, an ordered
array of base64-encoded Content-Encoding values, and `gzip_decompressed`, recording
one handler decompression. It preserves duplicates, comma-separated declarations,
unfamiliar values, and non-UTF-8 bytes. Base64 protects header bytes from JSON
normalization and does not describe body compression. No other headers are included.
Only HTTP sources may use this block. The legacy adapter currently omits it.

The OTLP hook neither parses encoding chains nor adds decompression. A recognized
single declaration supplies incoming encoding. Ambiguous or unfamiliar declarations
use `unknown`. Failed-decode bodies retain that encoding unless a single gzip
declaration was decompressed, making stored encoding `identity`. Repeated declarations
remain `unknown` even after one decompression. Successful decoding stores per-profile
protobuf with identity encoding, retaining raw declarations and the transformation flag.

Unfamiliar declarations alone do not drop capture. Raw header bytes bypass ordinary
text validation, but the 64 KiB metadata limit includes their base64 overhead.
Object and retained-memory limits still apply. Fields are borrowed only until
synchronous recorder serialization returns.
