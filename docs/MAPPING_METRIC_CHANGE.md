# Adding Mapping Count Metric

This change adds a new metric to track the number of mappings per profile in the Pyroscope distributor.

## Changes Required

### 1. pkg/distributor/metrics.go

Already updated with:
- Added `receivedMappings` field to `metrics` struct
- Registered new histogram metric `pyroscope_distributor_received_mappings`
- Buckets: exponential range from 1 to 1000 with 20 buckets
- Labels: `type` and `tenant`

### 2. pkg/distributor/distributor.go

Need to add metric observation in `pushSeries` function after line 535 (original line, offset 934 with 399 offset):

```go
symbolsSize, samplesSize := profileSizeBytes(p.Profile)
d.metrics.receivedSamplesBytes.WithLabelValues(profName, tenantID).Observe(float64(samplesSize))
d.metrics.receivedSymbolsBytes.WithLabelValues(profName, tenantID).Observe(float64(symbolsSize))
// ADD THIS LINE:
d.metrics.receivedMappings.WithLabelValues(profName, tenantID).Observe(float64(len(p.Mapping)))
```

## Metric Details

**Name:** `pyroscope_distributor_received_mappings`

**Type:** Histogram

**Description:** The number of mappings per profile received by the distributor.

**Labels:**
- `type`: Profile type (e.g., "process_cpu", "memory", "goroutine")
- `tenant`: Tenant ID

**Buckets:** Exponential range from 1 to 1000 with 20 buckets

## Use Cases

1. Monitor mapping cardinality per profile type
2. Identify profiles with excessive mappings
3. Track changes in application deployment patterns
4. Capacity planning based on mapping counts

## Testing

Once deployed, the metric can be queried with:

```promql
# Average mappings per profile type
avg by (type) (pyroscope_distributor_received_mappings_sum / pyroscope_distributor_received_mappings_count)

# 95th percentile mappings by tenant
histogram_quantile(0.95, sum by (tenant, le) (rate(pyroscope_distributor_received_mappings_bucket[5m])))

# Rate of profiles with high mapping counts (>100)
sum(rate(pyroscope_distributor_received_mappings_bucket{le="+Inf"}[5m])) - sum(rate(pyroscope_distributor_received_mappings_bucket{le="100"}[5m]))
```
