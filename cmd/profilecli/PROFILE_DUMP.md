# profilecli profile-dump

Retrieve Connect pprof captures from the customer bucket. Supports capture format
v1 and preserves native payload bytes, including compression and malformed pprof.

## Synopsis

```text
profilecli profile-dump list [OPTIONS] --tenant-id=TENANT --from=TIME --to=TIME
profilecli profile-dump inspect [OPTIONS] KEY
profilecli profile-dump extract [OPTIONS] KEY [--output=PATH]
```

`KEY` is the exact `profile-debug-dumps/...` object key, relative to
`--storage.prefix`. Use the key returned by `list` or a capture trace.

## Common options

```text
--storage.*
    Existing object-store configuration and customer-cloud credentials.
    Run a subcommand with --help for provider-specific options.

--timeout=DURATION
    Default: 2m. Must be positive.
    Command deadline. Ctrl-C also cancels the command.
    Providers that ignore cancellation may exceed the deadline.

--max-object-size=BYTES
    Default: 134217728 (128 MiB). Minimum: 14 bytes.
    Maximum complete envelope size, including header, metadata and payload.
```

## list options

```text
--tenant-id=TENANT
    Required. Select one tenant within existing cloud permissions.
    This filter is not an authorization boundary.

--from=TIME
    Required. Inclusive server capture time in RFC3339, allowing fractional
    seconds. Must precede --to and fall in year 1970 or later.

--to=TIME
    Required. Exclusive server capture time in RFC3339, allowing fractional
    seconds. Must fall in year 9999 or earlier.

--source=connect
    Optional source filter. Only connect is supported.

--format=pprof
    Optional native-format filter. Only pprof is supported.

--limit=COUNT
    Default: 100. Must be positive.
    Maximum returned captures. Reaching the limit marks results incomplete.

--max-work=COUNT
    Default: 10000. Must be positive.
    Maximum sum of listing calls, visited entries and metadata storage calls.
    Each candidate inspection reserves three storage calls.
    Counts application work, excluding provider page prefetch.
```

## extract options

```text
--output=PATH, -o PATH
    Optional native payload destination.
    Default: CAPTURE_ID.pprof, CAPTURE_ID.pprof.gz or CAPTURE_ID.pprof.encoded
    in the current directory, according to the declared payload encoding.
    Original metadata is written to PATH.metadata.json.
    The directory must exist and support hard links.
    Existing files and symlinks are never overwritten.
```

## Output and errors

JSON goes to stdout. Diagnostics and the raw-customer-data warning go to stderr.

| Command | JSON result | Validation |
| --- | --- | --- |
| `list` | `captures`, `limited`, `reason`, `work`, `skipped_invalid`, `skipped_missing`, `validation` | Bounded metadata inspection within tenant/day prefixes |
| `inspect` | `key`, `metadata`, `validation` | Header, metadata, key agreement and declared object size, without reading the payload |
| `extract` | `payload`, `metadata`, `validation` | Complete envelope, key agreement and exact payload length, including EOF |

A limited list may omit matching captures. Invalid and vanished objects are
counted separately. Other listing failures can return partial JSON with an error
exit status. Inspect and extract distinguish missing objects, corrupt envelopes
and storage-access failures. A missing object may still be uploading or its
upload may have failed.

Extraction creates private mode-0600 files and cleans temporary or newly
published files on failure. The payload and sidecar are not crash-atomic, so an
abrupt process termination can leave the metadata file alone. Extracted data is
not sanitized. Envelope validation and filename extensions do not establish
native-format validity, integrity or authenticity. There is no checksum, profile
conversion or multipart part extraction.

## Example

```bash
STORAGE=(--storage.backend=filesystem --storage.filesystem.dir=/path/to/bucket)
profilecli profile-dump list "${STORAGE[@]}" --tenant-id=tenant-a \
  --from=2026-09-23T12:00:00Z --to=2026-09-23T13:00:00Z
profilecli profile-dump inspect "${STORAGE[@]}" "$KEY"
profilecli profile-dump extract "${STORAGE[@]}" "$KEY" --output=capture.pprof

# Requires a valid native pprof payload.
go tool pprof -http=127.0.0.1:8081 capture.pprof
```
