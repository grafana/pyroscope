---
description: Enable experimental QueryAnomalies so Pyroscope confirms candidate profile IDs from an external anomaly source against ingested data.
menuTitle: Query anomalies
title: Query profile anomalies
weight: 320
keywords:
  - Pyroscope
  - query-frontend
  - anomalies
  - profilecli
---

# Query profile anomalies

Grafana Pyroscope can confirm profile IDs that an external anomaly source flagged against data that was actually ingested. Use this when a detector points at profiles that sampling or ingestion might have dropped, so a follow-up flame graph query doesn't come back empty.

The `QueryAnomalies` RPC is experimental. It runs only on the v2 query-frontend. The default is to leave `-query-frontend.anomaly-api.url` empty, which disables the lookup.

## Before you begin

- Run the v2 query-frontend. The v1 frontend returns an unimplemented error.
- Send requests with a single tenant ID. The stacktrace anomaly type rejects requests that carry more than one tenant.
- Provide an HTTP service that implements the anomaly lookup described below.

## Configure the anomaly source URL

Set the query-frontend `anomaly_api.url` to the base URL of your anomaly source. Pyroscope appends the lookup path.

```yaml
frontend:
  anomaly_api:
    url: https://<ANOMALY_SOURCE>
```

The equivalent flag is `-query-frontend.anomaly-api.url`. For the generated field listing, refer to [Configuration parameters](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-server/reference-configuration-parameters/).

## Implement the anomaly lookup

For anomaly type `stacktrace`, the query-frontend resolves distinct `service_name` values for the request's label selector and time range, then calls the anomaly source once.

The request is:

- Method: `GET`
- Path: `/api/v1/anomalydetection/anomalies`
- Query parameters: one or more `service_name` values, plus `start` and `end` as Unix timestamps in milliseconds
- Header: `X-Scope-OrgID` set to the request tenant

Example:

```http
GET /api/v1/anomalydetection/anomalies?service_name=checkout&start=1717200000000&end=1717203600000 HTTP/1.1
Host: <ANOMALY_SOURCE>
X-Scope-OrgID: <TENANT_ID>
```

A successful response is HTTP 200 with JSON of this shape:

```json
{
  "anomalies": [
    {
      "profile_uuid": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "score": 0.92
    }
  ]
}
```

`profile_uuid` must parse as a UUID. The query-frontend copies `score` onto the matching RPC result. The RPC and CLI call the identifier a profile ID. If the label selector resolves to more than 200 distinct `service_name` values, the query-frontend rejects the request.

## Confirm candidates against ingested data

After the anomaly source returns candidate profile IDs, the query-frontend checks which of those IDs exist in ingested data for the full label selector and time range.

Only `process_cpu:cpu:nanoseconds:cpu:nanoseconds` is queried for the stacktrace type. Other profile types return an empty result.

Sampled-out profiles don't count as present. For how sampled-out profiles are stored, refer to [Write-path sampling](/docs/pyroscope/<PYROSCOPE_VERSION>/reference-pyroscope-v2-architecture/sampling/).

Each returned anomaly includes the ingested profile ID, the profile timestamp, the profile labels, and the score from the anomaly source.

## Query anomalies from the CLI

Use `profilecli query anomalies` to call this RPC. Refer to [Profile CLI](/docs/pyroscope/<PYROSCOPE_VERSION>/view-and-analyze-profile-data/profile-cli/#query-profile-anomalies).
