---
title: Capture inflight profiles
menuTitle: Profile capture
description: Activate temporary per-tenant Connect pprof capture and configure retention.
weight: 310
---

# Capture inflight profiles

The external Connect pprof path uses this immutable per-tenant runtime policy.
Distributors with a configured customer bucket capture and upload admitted samples.
Process bounds alone do not activate capture.

## Activation

Activation uses the existing per-tenant runtime overrides file:

```yaml
overrides:
  tenant-a:
    profile_debug_dump:
      active_until: "2026-09-23T23:10:00Z"
      selector: '{service_name="checkout"}'
      probability: 0.01
      max_captures_per_second: 1
```

Replace the illustrative timestamp with the intended absolute deadline. An absent
or null block disables capture. A deadline in the past is valid but inactive.
`active_until` and `probability` are required when the block is present. Probability
must be finite and in `(0, 1]`. The selector defaults to `{}`. For Connect it matches the
external request-series labels before relabeling or profile parsing. An absent
label has the same empty-string matching semantics as other repository selectors.

Process configuration supplies finite bounds:

```yaml
profile_dump:
  max_activation_window: 1h
  default_captures_per_second: 1
  max_captures_per_second: 10
```

Corresponding flags use the `profile-dump.` prefix with hyphenated names. The
activation ceiling must be positive and is measured at configuration load. The
rate default and ceiling must be finite and positive, and the default cannot
exceed the ceiling. Tenant rates may be fractional but must be positive, finite,
and no greater than the process ceiling. These rates are per tenant per
distributor, not fleet-wide quotas. The values above are provisional development
defaults. Production sizing remains open.

The block is runtime-only and cannot be set in global `limits`. Each tenant must
opt in explicitly. Invalid deadlines, selectors, probabilities, or rates reject
the entire reload. The existing runtime manager retains the last valid snapshot.
Reloads and restarts never renew its absolute deadline.

Policies are validated and compiled when configuration loads. Every new admission
checks the current policy and its absolute deadline, including after reload failure.
Removing or expiring a policy stops new admissions after local propagation.
Already-admitted captures may finish through the normal bounded queue. Policy
changes neither revoke queued captures nor delete existing objects.

Recorder limits are configured in the same `profile_dump` block. See the
[configuration reference](../reference-configuration-parameters/) for `max_object_bytes`, `max_retained_bytes`, queue, worker,
rate, upload-timeout and shutdown-drain settings. The provisional defaults are
development bounds. They are validated at process startup.

## Monitoring

Check the overrides exporter after changing a policy:

- `pyroscope_limits_overrides{limit_name="profile_debug_dump_active_until_timestamp_seconds",tenant="tenant-a"}`
  reports the configured absolute deadline as Unix seconds.
- `pyroscope_limits_overrides{limit_name="profile_debug_dump_active",tenant="tenant-a"}`
  is 1 before that deadline and 0 at or after it.

Status uses one current time per scrape, so expiry needs no reload. An expired
policy retains its configured deadline. An absent, null or removed policy reports
0 for both values while the tenant override entry remains. Removing the entire
tenant entry stops exporting its series, following normal Prometheus staleness.
No tenant history is retained. These values have no global-default series.
With the exporter ring enabled, only its leader exports them. A rejected reload
leaves the last valid deadline in effect, with status still evaluated at scrape time.

Recorder metrics are process aggregates without tenant labels. Check
`pyroscope_profile_dump_dropped_total` for local drops and
`pyroscope_profile_dump_uploads_total` by `result` for success, error, timeout
and canceled uploads. Aggregate unsuccessful uploads with:

```promql
sum by (source) (pyroscope_profile_dump_uploads_total{result=~"error|timeout|canceled"})
```

An enqueued candidate or a `Profile.Capture` span's `capture.object_key` identifies
intended storage and does not establish successful persistence.

## Admin retention

Run an **admin target** using the same customer bucket and `storage.prefix` as the
capturing distributors. Distributor-only deployments do not run retention cleanup.
Configure retention under `profile_dump`. Expiry or policy removal stops new
admissions, while cleanup continues independently for stored captures.
See [Cleaner contracts](https://github.com/grafana/pyroscope/blob/main/pkg/profiledump/CLEANER.md)
for retention settings, progress monitoring and provider limits, and the
[CLI reference](https://github.com/grafana/pyroscope/blob/main/cmd/profilecli/PROFILE_DUMP.md)
for listing, inspecting and extracting captures.
