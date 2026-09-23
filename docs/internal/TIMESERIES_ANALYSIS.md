# Time-Series Analysis

## Goal

`QuerierService.AnalyzeSeries` investigates profiling time series and returns
an event log instead of the source points. The initial event types are spikes,
drops, and increases. Flat series produce no events.

The endpoint requires the V2 query backend for the entire requested range. V1
and hybrid ranges return `Unimplemented`: detecting events independently on
both sides of a storage split would produce incorrect results around the split.

## API

The public request follows `SelectSeries`: profile type, label selector, time
range, resolution step, grouping labels, and an optional series limit. It also
accepts an analysis configuration:

- Baseline window: observations used to establish normal behavior.
- Confirmation window: observations inspected after a candidate deviation.
- Minimum sustained points: elevated observations required for a sustained
  event.
- Minimum relative change and robust score: both thresholds must pass before
  an event starts.
- Recovery threshold and flatness threshold: hysteresis and flat-series
  filtering.

The response is a chronological event log. Each event includes its type and
duration, labels, start/end/peak timestamps, baseline and peak values, relative
change, and robust score. New event types and fields can be added without
changing the event-log shape.

## Detector

The detector runs independently per labeled series after all block and
query-plan results have been merged and bucketed to the requested step.

1. Split points into contiguous runs. A timestamp gap larger than 1.5 times the
   requested step resets the baseline; missing buckets are never interpreted as
   zero.
2. Omit runs that cannot fill the baseline and confirmation windows, then omit
   flat series based on normalized range.
3. For each candidate point, calculate the median and MAD of its preceding
   baseline window. The candidate starts an event only when its relative change
   and robust score both exceed their configured thresholds.
4. Hold that pre-event baseline fixed while examining subsequent points. This
   prevents an event from changing the baseline used to classify itself.
5. Mark an event sustained when enough observations in the confirmation window
   remain outside the recovery threshold. Positive short events are spikes;
   sustained positive events are increases. Negative short events are drops;
   sustained negative events are decreases.
6. End an event after values recover inside the lower recovery threshold. This
   hysteresis prevents noisy values from splitting one event into many events.

Median and MAD are chosen over mean and standard deviation because an outlier
does not materially distort its own baseline. The simple fixed-size rolling
window has predictable cost of O(n * w log w), where n is point count and w is
the baseline window. The first implementation intentionally defers PELT, whose
practical cost can be near linear but has an O(n^2) worst case.

## Query Backend

`QUERY_TIME_SERIES_ANALYSIS` uses the same profile-total extraction as a
standard time-series query. Its report carries intermediate series between
query-plan nodes. Intermediate aggregators only merge and bucket those series;
they do not run detection or apply the event limit. The frontend sets
`InvokeOptions.finalize` on the root invocation, and query backends clear it
when dispatching child invocations. After the root finishes merging (or reading,
for a single read-node plan), that query backend runs detection and applies the
event limit once. The final report contains events rather than source series.
This preserves correctness across block and plan boundaries without repeatedly
analyzing partial results.

## Validation and Tests

The frontend validates profile type, time range, millisecond step, windows,
thresholds, and finite numeric values. Zero-valued optional configuration
fields select documented defaults.

Tests cover flat and all-zero series, spikes, short drops,
sustained increases and decreases, recovery hysteresis, zero baselines, missing-bucket gaps,
block-boundary events, deterministic ordering, V2 routing, and V1/hybrid
rejection.
