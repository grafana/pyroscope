# Profile capture runtime policy

This checkpoint provides the configuration and immutable policy used by the
Connect + pprof capture path. The recorder and ingestion hook are separate work.
Configuring this policy alone does not yet capture or upload profiles.

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
must be finite and in `(0, 1]`. The selector defaults to `{}` and is compiled through
`model.ParseMetricSelector` when configuration loads. For Connect it matches the
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

`Overrides.ProfileDebugDump` returns an immutable policy by value. It exposes no
matcher slices or mutable configuration pointers. Selector evaluation uses the
compiled matchers, with no per-sample parsing. Input configuration changes after
validation do not change the published policy. Validate only unpublished limits.

A recorder must check `ActiveAt` on every new admission, even after reload failure.
Removing or expiring a policy stops new admissions after local propagation.
Already-admitted captures may finish through the normal bounded queue. Policy
changes neither revoke queued captures nor delete existing objects.
