# Pyroscope API Reference

This documentation is auto-generated from OpenAPI v3 specifications.

<!-- TOC -->
## Table of Contents

- [Push/V1](#push/v1)
  - [push.v1](#pushv1)
- [Querier/V1](#querier/v1)
  - [querier.v1](#querierv1)

## API Scopes

- **scope/internal**: This operation is considered part of the interal API scope. There are no stability guaraentees when using those APIs.
- **scope/public**: This operation is considered part of the public API scope

## Push/V1

### push.v1

**Version:** v1.0.0  
**OpenAPI:** 3.1.0  

**API Scopes:**
- **scope/public**: This operation is considered part of the public API scope
- **scope/internal**: This operation is considered part of the interal API scope. There are no stability guaraentees when using those APIs.
- **push.v1.PusherService**

**Endpoints:**

#### `/push.v1.PusherService/Push`

##### POST

**Summary:** Push

**Operation ID:** `push.v1.PusherService.Push`

**Tags:** `scope/public`, `push.v1.PusherService`

**Request Body:**

*Required*

- **Content-Type:** `application/json`
- **Schema:** `push.v1.PushRequest`

  **Fields:**

  | Name | Type | Description |
  |------|------|-----------|
  | `series` | `array` | series is a set raw pprof profiles and accompanying labels |


**Responses:**

- **200**: Success
  - Content-Type: `application/json`
  - Schema: `push.v1.PushResponse`
- **default**: Error
  - Content-Type: `application/json`
  - Schema: `connect.error`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `code` | `string` (enum) | The status code, which should be an enum value of [google.rpc.Code][google.rpc.Code]. |
    | `details` | `array` | A list of messages that carry the error details. There is no limit on the number of messages. |
    | `message` | `string` | A developer-facing error message, which should be in English. Any user-facing error message should be localized and sent in the [google.rpc.Status.details][google.rpc.Status.details] field, or localized by the client. |


**Curl Example:**

```bash
curl -X POST "http://localhost:4040/push.v1.PusherService/Push" \
  -H "Connect-Protocol-Version: 1" \
  -H "Content-Type: application/json" \
  -d '{ "series": [] }'
```

---


## Querier/V1

### querier.v1

**Version:** v1.0.0  
**OpenAPI:** 3.1.0  

**API Scopes:**
- **scope/public**: This operation is considered part of the public API scope
- **scope/internal**: This operation is considered part of the interal API scope. There are no stability guaraentees when using those APIs.
- **querier.v1.QuerierService**

**Endpoints:**

#### `/querier.v1.QuerierService/Diff`

##### POST

**Summary:** Diff returns a diff of two profiles

**Description:** Diff returns a diff of two profiles

**Operation ID:** `querier.v1.QuerierService.Diff`

**Tags:** `scope/public`, `querier.v1.QuerierService`

**Request Body:**

*Required*

- **Content-Type:** `application/json`
- **Schema:** `querier.v1.DiffRequest`

  **Fields:**

  | Name | Type | Description |
  |------|------|-----------|
  | `left` | `querier.v1.SelectMergeStacktracesRequest` | - |
  | `right` | `querier.v1.SelectMergeStacktracesRequest` | - |


**Responses:**

- **200**: Success
  - Content-Type: `application/json`
  - Schema: `querier.v1.DiffResponse`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `flamegraph` | `querier.v1.FlameGraphDiff` | - |

- **default**: Error
  - Content-Type: `application/json`
  - Schema: `connect.error`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `code` | `string` (enum) | The status code, which should be an enum value of [google.rpc.Code][google.rpc.Code]. |
    | `details` | `array` | A list of messages that carry the error details. There is no limit on the number of messages. |
    | `message` | `string` | A developer-facing error message, which should be in English. Any user-facing error message should be localized and sent in the [google.rpc.Status.details][google.rpc.Status.details] field, or localized by the client. |


**Curl Example:**

```bash
curl -X POST "http://localhost:4040/querier.v1.QuerierService/Diff" \
  -H "Connect-Protocol-Version: 1" \
  -H "Content-Type: application/json" \
  -d '{ "left": { /* querier.v1.SelectMergeStacktracesRequest object */ }, "right": { /* querier.v1.SelectMergeStacktracesRequest object */ } }'
```

---

#### `/querier.v1.QuerierService/LabelNames`

##### POST

**Summary:** LabelNames returns a list of the existing label names.

**Description:** LabelNames returns a list of the existing label names.

**Operation ID:** `querier.v1.QuerierService.LabelNames`

**Tags:** `scope/public`, `querier.v1.QuerierService`

**Request Body:**

*Required*

- **Content-Type:** `application/json`
- **Schema:** `types.v1.LabelNamesRequest`

  **Fields:**

  | Name | Type | Description |
  |------|------|-----------|
  | `end` | `integer:int64` | - |
  | `matchers` | `array` | - |
  | `start` | `integer:int64` | - |


**Responses:**

- **200**: Success
  - Content-Type: `application/json`
  - Schema: `types.v1.LabelNamesResponse`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `names` | `array` | - |

- **default**: Error
  - Content-Type: `application/json`
  - Schema: `connect.error`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `code` | `string` (enum) | The status code, which should be an enum value of [google.rpc.Code][google.rpc.Code]. |
    | `details` | `array` | A list of messages that carry the error details. There is no limit on the number of messages. |
    | `message` | `string` | A developer-facing error message, which should be in English. Any user-facing error message should be localized and sent in the [google.rpc.Status.details][google.rpc.Status.details] field, or localized by the client. |


**Curl Example:**

```bash
curl -X POST "http://localhost:4040/querier.v1.QuerierService/LabelNames" \
  -H "Connect-Protocol-Version: 1" \
  -H "Content-Type: application/json" \
  -d '{ "end": 1234567890, "matchers": [], "start": 1234567890 }'
```

---

#### `/querier.v1.QuerierService/LabelValues`

##### POST

**Summary:** LabelValues returns the existing label values for the provided label names.

**Description:** LabelValues returns the existing label values for the provided label names.

**Operation ID:** `querier.v1.QuerierService.LabelValues`

**Tags:** `scope/public`, `querier.v1.QuerierService`

**Request Body:**

*Required*

- **Content-Type:** `application/json`
- **Schema:** `types.v1.LabelValuesRequest`

  **Fields:**

  | Name | Type | Description |
  |------|------|-----------|
  | `end` | `integer:int64` | - |
  | `matchers` | `array` | - |
  | `name` | `string` | - |
  | `start` | `integer:int64` | - |


**Responses:**

- **200**: Success
  - Content-Type: `application/json`
  - Schema: `types.v1.LabelValuesResponse`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `names` | `array` | - |

- **default**: Error
  - Content-Type: `application/json`
  - Schema: `connect.error`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `code` | `string` (enum) | The status code, which should be an enum value of [google.rpc.Code][google.rpc.Code]. |
    | `details` | `array` | A list of messages that carry the error details. There is no limit on the number of messages. |
    | `message` | `string` | A developer-facing error message, which should be in English. Any user-facing error message should be localized and sent in the [google.rpc.Status.details][google.rpc.Status.details] field, or localized by the client. |


**Curl Example:**

```bash
curl -X POST "http://localhost:4040/querier.v1.QuerierService/LabelValues" \
  -H "Connect-Protocol-Version: 1" \
  -H "Content-Type: application/json" \
  -d '{ "end": 1234567890, "matchers": [], "name": "example", "start": 1234567890 }'
```

---

#### `/querier.v1.QuerierService/ProfileTypes`

##### POST

**Summary:** ProfileType returns a list of the existing profile types.

**Description:** ProfileType returns a list of the existing profile types.

**Operation ID:** `querier.v1.QuerierService.ProfileTypes`

**Tags:** `scope/public`, `querier.v1.QuerierService`

**Request Body:**

*Required*

- **Content-Type:** `application/json`
- **Schema:** `querier.v1.ProfileTypesRequest`

  **Fields:**

  | Name | Type | Description |
  |------|------|-----------|
  | `end` | `integer:int64` | Milliseconds since epoch. If missing or zero, only the ingesters will be
 queried. |
  | `start` | `integer:int64` | Milliseconds since epoch. If missing or zero, only the ingesters will be
 queried. |


**Responses:**

- **200**: Success
  - Content-Type: `application/json`
  - Schema: `querier.v1.ProfileTypesResponse`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `profileTypes` | `array` | - |

- **default**: Error
  - Content-Type: `application/json`
  - Schema: `connect.error`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `code` | `string` (enum) | The status code, which should be an enum value of [google.rpc.Code][google.rpc.Code]. |
    | `details` | `array` | A list of messages that carry the error details. There is no limit on the number of messages. |
    | `message` | `string` | A developer-facing error message, which should be in English. Any user-facing error message should be localized and sent in the [google.rpc.Status.details][google.rpc.Status.details] field, or localized by the client. |


**Curl Example:**

```bash
curl -X POST "http://localhost:4040/querier.v1.QuerierService/ProfileTypes" \
  -H "Connect-Protocol-Version: 1" \
  -H "Content-Type: application/json" \
  -d '{ "end": 1234567890, "start": 1234567890 }'
```

---

#### `/querier.v1.QuerierService/SelectMergeProfile`

##### POST

**Summary:** SelectMergeProfile returns matching profiles aggregated in pprof format. It  will contain all information stored (so including filenames and line  number, if ingested).

**Description:** SelectMergeProfile returns matching profiles aggregated in pprof format. It
 will contain all information stored (so including filenames and line
 number, if ingested).

**Operation ID:** `querier.v1.QuerierService.SelectMergeProfile`

**Tags:** `scope/public`, `querier.v1.QuerierService`

**Request Body:**

*Required*

- **Content-Type:** `application/json`
- **Schema:** `querier.v1.SelectMergeProfileRequest`

  **Fields:**

  | Name | Type | Description |
  |------|------|-----------|
  | `end` | `integer:int64` | Milliseconds since epoch. |
  | `labelSelector` | `string` | - |
  | `maxNodes` | `integer:int64` (nullable) | Limit the nodes returned to only show the node with the max_node's biggest
 total |
  | `profileTypeID` | `string` | - |
  | `stackTraceSelector` | `types.v1.StackTraceSelector` | Select stack traces that match the provided selector. |
  | `start` | `integer:int64` | Milliseconds since epoch. |


**Responses:**

- **200**: Success
  - Content-Type: `application/json`
  - Schema: `google.v1.Profile`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `comment` | `array` | Freeform text associated to the profile. Indices into string table. |
    | `defaultSampleType` | `integer:int64` | Index into the string table of the type of the preferred sample
 value. If unset, clients should default to the last sample value. |
    | `dropFrames` | `integer:int64` | frames with Function.function_name fully matching the following
 regexp will be dropped from the samples, along with their successors. Index into string table. |
    | `durationNanos` | `integer:int64` | Duration of the profile, if a duration makes sense. |
    | `function` | `array` | Functions referenced by locations |
    | `keepFrames` | `integer:int64` | frames with Function.function_name fully matching the following
 regexp will be kept, even if it matches drop_frames. Index into string table. |
    | `location` | `array` | Useful program location |
    | `mapping` | `array` | Mapping from address ranges to the image/binary/library mapped
 into that address range.  mapping[0] will be the main binary. |
    | `period` | `integer:int64` | The number of events between sampled occurrences. |
    | `periodType` | `google.v1.ValueType` | The kind of events between sampled ocurrences.
 e.g [ "cpu","cycles" ] or [ "heap","bytes" ] |
    | `sample` | `array` | The set of samples recorded in this profile. |
    | `sampleType` | `array` | A description of the samples associated with each Sample.value.
 For a cpu profile this might be:
   [["cpu","nanoseconds"]] or [["wall","seconds"]] or [["syscall","count"]]
 For a heap profile, this might be:
   [["allocations","count"], ["space","bytes"]],
 If one of the values represents the number of events represented
 by the sample, by convention it should be at index 0 and use
 sample_type.unit == "count". |
    | `stringTable` | `array` | A common table for strings referenced by various messages.
 string_table[0] must always be "". |
    | `timeNanos` | `integer:int64` | Time of collection (UTC) represented as nanoseconds past the epoch. |

- **default**: Error
  - Content-Type: `application/json`
  - Schema: `connect.error`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `code` | `string` (enum) | The status code, which should be an enum value of [google.rpc.Code][google.rpc.Code]. |
    | `details` | `array` | A list of messages that carry the error details. There is no limit on the number of messages. |
    | `message` | `string` | A developer-facing error message, which should be in English. Any user-facing error message should be localized and sent in the [google.rpc.Status.details][google.rpc.Status.details] field, or localized by the client. |


**Curl Example:**

```bash
curl -X POST "http://localhost:4040/querier.v1.QuerierService/SelectMergeProfile" \
  -H "Connect-Protocol-Version: 1" \
  -H "Content-Type: application/json" \
  -d '{ "end": 1234567890, "labelSelector": "example", "maxNodes": 1234567890, "profileTypeID": "example", "stackTraceSelector": { /* types.v1.StackTraceSelector object */ }, "start": 1234567890 }'
```

---

#### `/querier.v1.QuerierService/SelectMergeSpanProfile`

##### POST

**Summary:** SelectMergeSpanProfile returns matching profiles aggregated in a flamegraph  format. It will combine samples from within the same callstack, with each  element being grouped by its function name.

**Description:** SelectMergeSpanProfile returns matching profiles aggregated in a flamegraph
 format. It will combine samples from within the same callstack, with each
 element being grouped by its function name.

**Operation ID:** `querier.v1.QuerierService.SelectMergeSpanProfile`

**Tags:** `scope/public`, `querier.v1.QuerierService`

**Request Body:**

*Required*

- **Content-Type:** `application/json`
- **Schema:** `querier.v1.SelectMergeSpanProfileRequest`

  **Fields:**

  | Name | Type | Description |
  |------|------|-----------|
  | `end` | `integer:int64` | Milliseconds since epoch. |
  | `format` | `querier.v1.ProfileFormat` | Profile format specifies the format of profile to be returned.
 If not specified, the profile will be returned in flame graph format. |
  | `labelSelector` | `string` | - |
  | `maxNodes` | `integer:int64` (nullable) | Limit the nodes returned to only show the node with the max_node's biggest
 total |
  | `profileTypeID` | `string` | - |
  | `spanSelector` | `array` | - |
  | `start` | `integer:int64` | Milliseconds since epoch. |


**Responses:**

- **200**: Success
  - Content-Type: `application/json`
  - Schema: `querier.v1.SelectMergeSpanProfileResponse`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `flamegraph` | `querier.v1.FlameGraph` | - |
    | `tree` | `string:byte` | Pyroscope tree bytes. |

- **default**: Error
  - Content-Type: `application/json`
  - Schema: `connect.error`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `code` | `string` (enum) | The status code, which should be an enum value of [google.rpc.Code][google.rpc.Code]. |
    | `details` | `array` | A list of messages that carry the error details. There is no limit on the number of messages. |
    | `message` | `string` | A developer-facing error message, which should be in English. Any user-facing error message should be localized and sent in the [google.rpc.Status.details][google.rpc.Status.details] field, or localized by the client. |


**Curl Example:**

```bash
curl -X POST "http://localhost:4040/querier.v1.QuerierService/SelectMergeSpanProfile" \
  -H "Connect-Protocol-Version: 1" \
  -H "Content-Type: application/json" \
  -d '{ "end": 1234567890, "format": { /* querier.v1.ProfileFormat object */ }, "labelSelector": "example", "maxNodes": 1234567890, "profileTypeID": "example", "spanSelector": [], "start": 1234567890 }'
```

---

#### `/querier.v1.QuerierService/SelectMergeStacktraces`

##### POST

**Summary:** SelectMergeStacktraces returns matching profiles aggregated in a flamegraph  format. It will combine samples from within the same callstack, with each  element being grouped by its function name.

**Description:** SelectMergeStacktraces returns matching profiles aggregated in a flamegraph
 format. It will combine samples from within the same callstack, with each
 element being grouped by its function name.

**Operation ID:** `querier.v1.QuerierService.SelectMergeStacktraces`

**Tags:** `scope/public`, `querier.v1.QuerierService`

**Request Body:**

*Required*

- **Content-Type:** `application/json`
- **Schema:** `querier.v1.SelectMergeStacktracesRequest`

  **Fields:**

  | Name | Type | Description |
  |------|------|-----------|
  | `end` | `integer:int64` | Milliseconds since epoch. |
  | `format` | `querier.v1.ProfileFormat` | Profile format specifies the format of profile to be returned.
 If not specified, the profile will be returned in flame graph format. |
  | `labelSelector` | `string` | - |
  | `maxNodes` | `integer:int64` (nullable) | Limit the nodes returned to only show the node with the max_node's biggest
 total |
  | `profileTypeID` | `string` | - |
  | `start` | `integer:int64` | Milliseconds since epoch. |


**Responses:**

- **200**: Success
  - Content-Type: `application/json`
  - Schema: `querier.v1.SelectMergeStacktracesResponse`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `flamegraph` | `querier.v1.FlameGraph` | - |
    | `tree` | `string:byte` | Pyroscope tree bytes. |

- **default**: Error
  - Content-Type: `application/json`
  - Schema: `connect.error`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `code` | `string` (enum) | The status code, which should be an enum value of [google.rpc.Code][google.rpc.Code]. |
    | `details` | `array` | A list of messages that carry the error details. There is no limit on the number of messages. |
    | `message` | `string` | A developer-facing error message, which should be in English. Any user-facing error message should be localized and sent in the [google.rpc.Status.details][google.rpc.Status.details] field, or localized by the client. |


**Curl Example:**

```bash
curl -X POST "http://localhost:4040/querier.v1.QuerierService/SelectMergeStacktraces" \
  -H "Connect-Protocol-Version: 1" \
  -H "Content-Type: application/json" \
  -d '{ "end": 1234567890, "format": { /* querier.v1.ProfileFormat object */ }, "labelSelector": "example", "maxNodes": 1234567890, "profileTypeID": "example", "start": 1234567890 }'
```

---

#### `/querier.v1.QuerierService/SelectSeries`

##### POST

**Summary:** SelectSeries returns a time series for the total sum of the requested  profiles.

**Description:** SelectSeries returns a time series for the total sum of the requested
 profiles.

**Operation ID:** `querier.v1.QuerierService.SelectSeries`

**Tags:** `scope/public`, `querier.v1.QuerierService`

**Request Body:**

*Required*

- **Content-Type:** `application/json`
- **Schema:** `querier.v1.SelectSeriesRequest`

  **Fields:**

  | Name | Type | Description |
  |------|------|-----------|
  | `aggregation` | `types.v1.TimeSeriesAggregationType` | Query resolution step width in seconds |
  | `end` | `integer:int64` | Milliseconds since epoch. |
  | `groupBy` | `array` | - |
  | `labelSelector` | `string` | - |
  | `limit` | `integer:int64` (nullable) | Select the top N series by total value. |
  | `profileTypeID` | `string` | - |
  | `stackTraceSelector` | `types.v1.StackTraceSelector` | Select stack traces that match the provided selector. |
  | `start` | `integer:int64` | Milliseconds since epoch. |
  | `step` | `number:double` | - |


**Responses:**

- **200**: Success
  - Content-Type: `application/json`
  - Schema: `querier.v1.SelectSeriesResponse`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `series` | `array` | - |

- **default**: Error
  - Content-Type: `application/json`
  - Schema: `connect.error`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `code` | `string` (enum) | The status code, which should be an enum value of [google.rpc.Code][google.rpc.Code]. |
    | `details` | `array` | A list of messages that carry the error details. There is no limit on the number of messages. |
    | `message` | `string` | A developer-facing error message, which should be in English. Any user-facing error message should be localized and sent in the [google.rpc.Status.details][google.rpc.Status.details] field, or localized by the client. |


**Curl Example:**

```bash
curl -X POST "http://localhost:4040/querier.v1.QuerierService/SelectSeries" \
  -H "Connect-Protocol-Version: 1" \
  -H "Content-Type: application/json" \
  -d '{ "aggregation": { /* types.v1.TimeSeriesAggregationType object */ }, "end": 1234567890, "groupBy": [], "labelSelector": "example", "limit": 1234567890, "profileTypeID": "example", "stackTraceSelector": { /* types.v1.StackTraceSelector object */ }, "start": 1234567890, "step": 123.45 }'
```

---

#### `/querier.v1.QuerierService/Series`

##### POST

**Summary:** Series returns profiles series matching the request. A series is a unique  label set.

**Description:** Series returns profiles series matching the request. A series is a unique
 label set.

**Operation ID:** `querier.v1.QuerierService.Series`

**Tags:** `scope/public`, `querier.v1.QuerierService`

**Request Body:**

*Required*

- **Content-Type:** `application/json`
- **Schema:** `querier.v1.SeriesRequest`

  **Fields:**

  | Name | Type | Description |
  |------|------|-----------|
  | `end` | `integer:int64` | Milliseconds since epoch. If missing or zero, only the ingesters will be
 queried. |
  | `labelNames` | `array` | - |
  | `matchers` | `array` | - |
  | `start` | `integer:int64` | Milliseconds since epoch. If missing or zero, only the ingesters will be
 queried. |


**Responses:**

- **200**: Success
  - Content-Type: `application/json`
  - Schema: `querier.v1.SeriesResponse`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `labelsSet` | `array` | - |

- **default**: Error
  - Content-Type: `application/json`
  - Schema: `connect.error`

    **Fields:**

    | Name | Type | Description |
    |------|------|-----------|
    | `code` | `string` (enum) | The status code, which should be an enum value of [google.rpc.Code][google.rpc.Code]. |
    | `details` | `array` | A list of messages that carry the error details. There is no limit on the number of messages. |
    | `message` | `string` | A developer-facing error message, which should be in English. Any user-facing error message should be localized and sent in the [google.rpc.Status.details][google.rpc.Status.details] field, or localized by the client. |


**Curl Example:**

```bash
curl -X POST "http://localhost:4040/querier.v1.QuerierService/Series" \
  -H "Connect-Protocol-Version: 1" \
  -H "Content-Type: application/json" \
  -d '{ "end": 1234567890, "labelNames": [], "matchers": [], "start": 1234567890 }'
```

---


