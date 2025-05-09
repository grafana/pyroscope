package memdb

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"go.uber.org/atomic"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/labels"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/validation"
)

type FlushedHead struct {
	Index        []byte
	Profiles     []byte
	Symbols      []byte
	Unsymbolized bool
	Meta         struct {
		ProfileTypeNames []string
		MinTimeNanos     int64
		MaxTimeNanos     int64
		NumSamples       uint64
		NumProfiles      uint64
		NumSeries        uint64
	}
}

type Head struct {
	symbols      *symdb.PartitionWriter
	metaLock     sync.RWMutex
	minTimeNanos int64
	maxTimeNanos int64
	totalSamples *atomic.Uint64
	profiles     *profilesIndex
	metrics      *HeadMetrics
	logger       log.Logger
	limits       validation.LabelValidationLimits
}

func NewHead(metrics *HeadMetrics, logger log.Logger, limits validation.LabelValidationLimits) *Head {
	h := &Head{
		metrics: metrics,
		logger:  logger,
		limits:  limits,
		symbols: symdb.NewPartitionWriter(0, &symdb.Config{
			Version: symdb.FormatV3,
		}),
		totalSamples: atomic.NewUint64(0),
		minTimeNanos: math.MaxInt64,
		maxTimeNanos: 0,
		profiles:     newProfileIndex(metrics),
	}

	return h
}

func (h *Head) Ingest(tenantID string, p *profilev1.Profile, id uuid.UUID, externalLabels []*typesv1.LabelPair, annotations []*typesv1.ProfileAnnotation) {
	if len(p.Sample) == 0 {
		return
	}

	if err := validation.ValidateLabels(h.limits, tenantID, externalLabels); err != nil {
		level.Warn(h.logger).Log("msg", "labels validation failed", "err", err)
		// TODO aleks-p: propagate the error upward
		return
	}

	// Delta not supported.
	externalLabels = phlaremodel.Labels(externalLabels).Delete(phlaremodel.LabelNameDelta)
	// Label order is enforced to ensure that __profile_type__ and __service_name__ always
	// come first in the label set. This is important for spatial locality: profiles are
	// stored in the label series order.
	externalLabels = phlaremodel.Labels(externalLabels).Delete(phlaremodel.LabelNameOrder)

	lbls, seriesFingerprints := labels.CreateProfileLabels(true, p, externalLabels...)
	metricName := phlaremodel.Labels(externalLabels).Get(model.MetricNameLabel)

	var profileIngested bool
	memProfiles := h.symbols.WriteProfileSymbols(p)
	for idxType := range memProfiles {
		profile := &memProfiles[idxType]
		profile.ID = id
		profile.SeriesFingerprint = seriesFingerprints[idxType]
		profile.Samples = profile.Samples.Compact(false)

		profile.TotalValue = profile.Samples.Sum()

		profile.Annotations.Keys = make([]string, 0, len(annotations))
		profile.Annotations.Values = make([]string, 0, len(annotations))
		for _, annotation := range annotations {
			profile.Annotations.Keys = append(profile.Annotations.Keys, annotation.Key)
			profile.Annotations.Values = append(profile.Annotations.Values, annotation.Value)
		}

		if profile.Samples.Len() == 0 {
			continue
		}

		h.profiles.Add(profile, lbls[idxType], metricName)

		profileIngested = true
		h.totalSamples.Add(uint64(profile.Samples.Len()))
		h.metrics.sampleValuesIngested.WithLabelValues(metricName).Add(float64(profile.Samples.Len()))
		h.metrics.sampleValuesReceived.WithLabelValues(metricName).Add(float64(len(p.Sample)))
	}

	if !profileIngested {
		return
	}

	h.metaLock.Lock()
	if p.TimeNanos < h.minTimeNanos {
		h.minTimeNanos = p.TimeNanos
	}
	if p.TimeNanos > h.maxTimeNanos {
		h.maxTimeNanos = p.TimeNanos
	}
	h.metaLock.Unlock()
}

func (h *Head) Flush(ctx context.Context) (res *FlushedHead, err error) {
	t := prometheus.NewTimer(h.metrics.flushedBlockDurationSeconds)
	defer t.ObserveDuration()

	if res, err = h.flush(ctx); err != nil {
		h.metrics.flushedBlocks.WithLabelValues("failed").Inc()
		return nil, err
	}

	blockSize := len(res.Index) + len(res.Profiles) + len(res.Symbols)
	h.metrics.flushedBlocks.WithLabelValues("success").Inc()
	h.metrics.flushedBlockSamples.Observe(float64(res.Meta.NumSamples))
	h.metrics.flusehdBlockProfiles.Observe(float64(res.Meta.NumProfiles))
	h.metrics.flushedBlockSeries.Observe(float64(res.Meta.NumSeries))
	h.metrics.flushedBlockSizeBytes.Observe(float64(blockSize))
	h.metrics.flushedFileSizeBytes.WithLabelValues("tsdb").Observe(float64(len(res.Index)))
	h.metrics.flushedFileSizeBytes.WithLabelValues("profiles.parquet").Observe(float64(len(res.Profiles)))
	h.metrics.flushedFileSizeBytes.WithLabelValues("symbols.symdb").Observe(float64(len(res.Symbols)))
	return res, nil
}

func (h *Head) flush(ctx context.Context) (*FlushedHead, error) {
	var (
		err      error
		profiles []schemav1.InMemoryProfile
	)
	res := new(FlushedHead)
	res.Meta.MinTimeNanos = h.minTimeNanos
	res.Meta.MaxTimeNanos = h.maxTimeNanos
	res.Meta.NumSamples = h.totalSamples.Load()
	res.Meta.NumSeries = uint64(h.profiles.totalSeries.Load())

	if res.Meta.NumSamples == 0 {
		return res, nil
	}

	res.Unsymbolized = HasUnsymbolizedProfiles(h.symbols.Symbols())

	symbolsBuffer := bytes.NewBuffer(nil)
	if err := symdb.WritePartition(h.symbols, symbolsBuffer); err != nil {
		return nil, err
	}
	res.Symbols = symbolsBuffer.Bytes()

	if res.Meta.ProfileTypeNames, err = h.profiles.profileTypeNames(); err != nil {
		return nil, fmt.Errorf("failed to get profile type names: %w", err)
	}

	if res.Index, profiles, err = h.profiles.Flush(ctx); err != nil {
		return nil, fmt.Errorf("failed to flush profiles: %w", err)
	}
	res.Meta.NumProfiles = uint64(len(profiles))

	if res.Profiles, err = WriteProfiles(h.metrics, profiles); err != nil {
		return nil, fmt.Errorf("failed to write profiles parquet: %w", err)
	}
	return res, nil
}

// TODO: move into the symbolizer package when available
func HasUnsymbolizedProfiles(symbols *symdb.Symbols) bool {
	locations := symbols.Locations
	mappings := symbols.Mappings
	for _, loc := range locations {
		if !mappings[loc.MappingId].HasFunctions {
			return true
		}
	}
	return false
}
