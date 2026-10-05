# profilecli admin profile-dumps

Retrieve Connect pprof captures from the customer bucket. Reads native payloads with schema-v1 JSON
sidecars and preserves exact bytes, including compressed, empty and malformed pprof.

See [runtime activation](../../pkg/profiledump/RECORDER.md#activation)
and [admin retention](../../pkg/profiledump/CLEANER.md)
for enabling capture and running cleanup against the same bucket and storage prefix.

## Synopsis

```text
profilecli admin profile-dumps list [OPTIONS] --tenant-id=TENANT --from=TIME --to=TIME
profilecli admin profile-dumps inspect [OPTIONS] KEY
profilecli admin profile-dumps extract [OPTIONS] KEY [--output=PATH]
```

`KEY` is the exact `profile-debug-dumps/native/...` object key, relative to
`--storage.prefix`. Use the payload key returned by `list` or a capture trace.
Inspect and extract also accept the corresponding JSON sidecar key.

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
    Default: 134217728 (128 MiB). Must be positive.
    Maximum native payload size. Sidecar reads accept at most 64 KiB of JSON
    and read at most one additional byte to detect excess.
```

## list options

```text
--tenant-id=TENANT
    Required. Select one tenant within existing cloud permissions.
    This filter is not an authorization boundary.

--from=TIME
    Required. Inclusive server capture time in RFC3339, allowing fractional
    seconds. Must not follow --to and must fall in year 1970 or later.

--to=TIME
    Required. Exclusive server capture time in RFC3339, allowing fractional
    seconds. Must fall in year 9999 or earlier.

--limit=COUNT
    Default: 100. Must be positive.
    Maximum returned captures. Reaching the limit marks results incomplete.

--max-work=COUNT
    Default: 10000. Must be positive.
    Maximum sum of listing calls, visited entries and metadata storage calls.
    Each minute listing and each returned entry costs one unit.
    Each sidecar candidate reserves two additional units for its GET and
    the sibling payload Attributes call, even if inspection fails early.
    Counts application work, excluding provider page prefetch.
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
| `list` | `captures`, `limited`, `reason`, `work`, `skipped_invalid`, `skipped_missing`, `validation` | Bounded sidecar inspection and payload Attributes within tenant/minute directories |
| `inspect` | `key`, `metadata`, `validation` | Sidecar metadata, key agreement and declared payload size, without reading the payload |
| `extract` | `payload`, `metadata`, `validation` | Sidecar metadata, key agreement and exact payload length, including EOF |

Listing visits only UTC minute directories intersecting `[from,to)`, generated
sequentially without allocating a complete interval plan. Equal endpoints yield
an empty result without storage calls. Exact capture-time filtering preserves
fractional seconds and timezone offsets. Listing does not assume provider order
and never downloads payload bytes. Returned keys identify the native payloads.

A limited list may omit matching captures. Dense selected or boundary minutes can
exhaust the work budget even for a narrow interval. Missing or invalid sidecar
candidates are counted separately. Payload-only orphans are not sidecar candidates
and are not counted as missing, but ordinary object tools can retrieve them. Other listing failures can return partial JSON with an error
exit status. Inspect and extract distinguish missing objects, invalid metadata or mismatched payload sizes
and storage-access failures. A missing object may still be uploading or its
upload may have failed, or admin retention cleanup may have deleted it.

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

## Example

```bash
STORAGE=(--storage.backend=filesystem --storage.filesystem.dir=/path/to/bucket)
profilecli admin profile-dumps list "${STORAGE[@]}" --tenant-id=tenant-a \
  --from=2026-09-23T12:00:00Z --to=2026-09-23T13:00:00Z
profilecli admin profile-dumps inspect "${STORAGE[@]}" "$KEY"
profilecli admin profile-dumps extract "${STORAGE[@]}" "$KEY" --output=capture.pprof

# Requires a valid native pprof payload.
go tool pprof -http=127.0.0.1:8081 capture.pprof
```
