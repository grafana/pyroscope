# profilecli admin profile-dumps

Discover and retrieve Connect pprof captures from the customer bucket. Inspection
validates schema-v1 JSON sidecars. Extraction preserves exact native bytes,
including compressed, empty and malformed pprof.

Storage must be configured explicitly to match the capturing deployment's backend,
bucket and storage prefix. The CLI does not discover storage from the tenant ID.
Filesystem storage also requires an explicit directory.

See [runtime activation](../../pkg/profiledump/RECORDER.md#activation)
and [admin retention](../../pkg/profiledump/CLEANER.md)
for enabling capture and running cleanup against the same bucket and storage prefix.

## Synopsis

```text
profilecli admin profile-dumps list --storage.backend=BACKEND [OPTIONS] --tenant-id=TENANT --from=TIME --to=TIME
profilecli admin profile-dumps inspect --storage.backend=BACKEND [OPTIONS] KEY
profilecli admin profile-dumps extract --storage.backend=BACKEND [OPTIONS] KEY [--output=PATH]
```

`KEY` is the exact `__pyroscope_cluster/profile-debug-dumps/native/...` object key,
relative to `--storage.prefix`. Use the payload key returned by `list` or a capture trace.
Inspect and extract also accept the corresponding JSON sidecar key.

## Common options

```text
--storage.backend=BACKEND
    Required. Select the backend used by the capturing deployment.
    There is no default backend for these commands.

--storage.filesystem.dir=PATH
    Required when --storage.backend=filesystem. No default directory.

--storage.*
    Configure the matching bucket, storage prefix and customer-cloud credentials.
    Run a subcommand with --help for provider-specific options.

--timeout=DURATION
    Default: 2m. Must be positive.
    Command deadline. Ctrl-C also cancels the command.
    Providers that ignore cancellation may exceed the deadline.
```

## inspect and extract options

```text
--max-object-size=BYTES
    Default: 134217728 (128 MiB). Must be positive.
    Maximum native payload size. Available only for inspect/extract.
    Sidecar reads accept at most 64 KiB of JSON
    and read at most one additional byte to detect excess.
```

## list options

```text
--tenant-id=TENANT
    Required. Select one tenant within existing cloud permissions.
    This filter is not an authorization boundary.

--from=TIME
    Required. Inclusive key-derived millisecond capture time in RFC3339,
    allowing fractional seconds. Must be earlier than or equal to the
    --to timestamp and fall in year 1970 or later.

--to=TIME
    Required. Exclusive key-derived millisecond capture time in RFC3339, allowing fractional
    seconds. Must fall in year 9999 or earlier.

--limit=COUNT
    Default: 100. Must be positive.
    Maximum returned captures. Reaching the limit marks results incomplete.
```

## extract options

```text
--output=PATH, -o PATH
    Optional native payload destination.
    Default: CAPTURE_ID.pprof, CAPTURE_ID.pprof.gz or CAPTURE_ID.pprof.encoded
    in the current directory, according to the declared payload encoding.
    Decoded metadata is written as formatted JSON to PATH.metadata.json.
    The directory must exist and support hard links.
    Existing files and symlinks are never overwritten.
```

## Output and errors

JSON goes to stdout. Diagnostics and the raw-customer-data warning go to stderr.

| Command | JSON result | Validation |
| --- | --- | --- |
| `list` | `captures` (each with `key` and `captured_at`), `limited`, optional `reason`, `skipped_invalid` | Validated native payload keys within tenant/minute directories |
| `inspect` | `key`, `metadata`, `validation` | Sidecar metadata, key agreement and declared payload size, without reading the payload |
| `extract` | `payload`, `metadata`, `validation` | Sidecar metadata, key agreement and exact payload length, including EOF |

Listing visits only UTC minute directories intersecting `[from,to)`, generated
sequentially without allocating a complete interval plan. Equal endpoints yield
an empty result without storage calls. Listing does not assume provider order and
makes no object GET or Attributes calls. Returned keys identify native `.pprof`
payloads, including payload-only orphans. JSON-only orphans are not results.

The reported `captured_at` comes from the ULID at millisecond resolution and is
compared directly against `[from,to)`, including fractional endpoints and timezone
offsets. For example, a capture made at `2026-09-18T12:00:00.123456789Z` has key time
`2026-09-18T12:00:00.123Z`. A range starting at `.1234Z` excludes that key. A range
from `.123Z` to `.1234Z` includes it, even though the full capture time is later than
the endpoint. Listing does not read metadata to recover sub-millisecond precision.

A limited list may omit matching captures. Result limits, timeout and cancellation
bound the command, but broad queries may perform many storage operations before
timeout. Invalid keys are counted in `skipped_invalid`. Listing failures return
partial JSON marked `limited` with a `reason` and an error exit status.

Discovery does not certify complete payload/sidecar pairs. A listed payload may
fail subsequent inspection. Use ordinary object tooling to retrieve a payload-only
orphan. Inspect and extract distinguish missing objects, invalid metadata or
mismatched payload sizes, and storage-access failures. A missing object may still
be uploading, its upload may have failed, or retention cleanup may have deleted it.

Sidecar inspection ignores unknown JSON fields. It still requires one valid UTF-8
JSON value within 64 KiB, supported schema, an explicit non-null nonnegative
`payload_size`, and identity/time agreement with the key. Zero-byte payloads are
valid captures.

Extraction creates private mode-0600 files and attempts to remove temporary and
newly published files on failure, reporting cleanup errors. The payload and sidecar
are not crash-atomic, so abrupt process termination can leave the metadata file
alone. Extracted data is
not sanitized. Metadata validation and filename extensions do not establish
native-format validity, integrity or authenticity. There is no checksum or profile
conversion.

Cancellation remains cooperative. Providers may exceed the deadline, and early
listing termination can leave a provider's listing producer blocked. Minute
directories reduce historical scanning but do not change producer teardown.

## Examples

### S3

```bash
STORAGE=(
  --storage.backend=s3
  --storage.s3.bucket-name=my-pyroscope-bucket
  --storage.s3.region=us-west-2
  --storage.s3.endpoint=s3.us-west-2.amazonaws.com
  --storage.s3.native-aws-auth-enabled=true
  --storage.prefix=production
)

profilecli admin profile-dumps list "${STORAGE[@]}" \
  --tenant-id=3648 \
  --from=2026-10-05T12:00:00Z \
  --to=2026-10-05T12:05:00Z
```

Replace the bucket, region, endpoint and prefix with the capturing deployment's
settings, and choose the tenant and capture-time window to search. Omit
`--storage.prefix` if the deployment has no prefix. Do not append
`__pyroscope_cluster/profile-debug-dumps/native` to it.

`--storage.s3.native-aws-auth-enabled=true` uses the AWS SDK's credential discovery
from environment variables and AWS configuration files. Reuse `"${STORAGE[@]}"`
for `inspect` and `extract`, as in the filesystem example below.

### Filesystem

```bash
STORAGE=(--storage.backend=filesystem --storage.filesystem.dir=/path/to/bucket)
profilecli admin profile-dumps list "${STORAGE[@]}" --tenant-id=tenant-a \
  --from=2026-09-23T12:00:00Z --to=2026-09-23T13:00:00Z
profilecli admin profile-dumps inspect "${STORAGE[@]}" "$KEY"
profilecli admin profile-dumps extract "${STORAGE[@]}" "$KEY" --output=capture.pprof

# Requires a valid native pprof payload.
go tool pprof -http=127.0.0.1:8081 capture.pprof
```
