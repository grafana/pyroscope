package pyroscope

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-kit/log/level"

	"github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/tenant"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/prometheus/prometheus/model/labels"
	"google.golang.org/protobuf/proto"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
)

type PushService interface {
	Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error)
	PushParsed(ctx context.Context, req *model.PushRequest) (*connect.Response[pushv1.PushResponse], error)
}

func NewPyroscopeIngestHandler(svc PushService, logger log.Logger) http.Handler {
	return NewIngestHandler(
		logger,
		&pyroscopeIngesterAdapter{svc: svc, log: logger},
	)
}

type pyroscopeIngesterAdapter struct {
	svc PushService
	log log.Logger
}

func (p *pyroscopeIngesterAdapter) Ingest(ctx context.Context, in *ingestion.IngestInput) error {
	pprofable, ok := in.Profile.(ingestion.ParseableToPprof)
	if ok {
		return p.parseToPprof(ctx, in, pprofable)
	} else {
		return in.Profile.Parse(ctx, p, p, in.Metadata)
	}
}

const (
	metricProcessCPU  = "process_cpu"
	metricMemory      = "memory"
	metricMutex       = "mutex"
	metricBlock       = "block"
	metricWall        = "wall"
	stUnitCount       = "count"
	stTypeSamples     = "samples"
	stTypeCPU         = "cpu"
	stTypeWall        = "wall"
	stUnitBytes       = "bytes"
	stTypeContentions = "contentions"
	stTypeDelay       = "delay"
	stUnitNanos       = "nanoseconds"
)

func (p *pyroscopeIngesterAdapter) Put(ctx context.Context, pi *storage.PutInput) error {
	if pi.Key.HasProfileID() {
		return nil
	}
	metric, stType, stUnit, app, err := convertMetadata(pi)
	if err != nil {
		return connect.NewError(
			connect.CodeInvalidArgument,
			fmt.Errorf("pyroscopeIngesterAdapter failed to convert metadata: %w", err),
		)
	}
	mdata := &tree.PprofMetadata{
		Type:      stType,
		Unit:      stUnit,
		StartTime: pi.StartTime,
	}
	if pi.SampleRate != 0 && (metric == metricWall || metric == metricProcessCPU) {
		period := time.Second.Nanoseconds() / int64(pi.SampleRate)
		mdata.Period = period
		mdata.PeriodType = stTypeCPU
		mdata.PeriodUnit = stUnitNanos
		if metric == metricWall {
			mdata.Type = stTypeWall
		} else {
			mdata.Type = stTypeCPU
		}
		mdata.Unit = stUnitNanos
		pi.Val.Scale(uint64(period))
	}
	pprof := pi.Val.Pprof(mdata)
	b, err := proto.Marshal(pprof)
	if err != nil {
		return connect.NewError(
			connect.CodeInvalidArgument,
			fmt.Errorf("pyroscopeIngesterAdapter failed to marshal pprof: %w", err),
		)
	}
	req := &pushv1.PushRequest{}
	series := &pushv1.RawProfileSeries{
		Labels: make([]*typesv1.LabelPair, 0, 3+len(pi.Key.Labels())),
	}
	series.Labels = append(series.Labels, &typesv1.LabelPair{
		Name:  labels.MetricName,
		Value: metric,
	}, &typesv1.LabelPair{
		Name:  phlaremodel.LabelNameDelta,
		Value: "false",
	})
	if pi.SpyName != "" {
		series.Labels = append(series.Labels, &typesv1.LabelPair{
			Name:  phlaremodel.LabelNamePyroscopeSpy,
			Value: pi.SpyName,
		})
	}
	hasServiceName := false
	for k, v := range pi.Key.Labels() {
		if !phlaremodel.IsLabelAllowedForIngestion(k) {
			continue
		}
		if k == "service_name" {
			hasServiceName = true
		}
		series.Labels = append(series.Labels, &typesv1.LabelPair{
			Name:  k,
			Value: v,
		})
	}
	// If service_name is not present, use app_name as the service_name.
	if !hasServiceName {
		series.Labels = append(series.Labels, &typesv1.LabelPair{
			Name:  "service_name",
			Value: app,
		})
	} else {
		series.Labels = append(series.Labels, &typesv1.LabelPair{
			Name:  "app_name",
			Value: app,
		})
	}
	series.Samples = []*pushv1.RawSample{{
		RawProfile: b,
		ID:         uuid.New().String(),
	}}
	req.Series = append(req.Series, series)
	_, err = p.svc.Push(ctx, connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("pyroscopeIngesterAdapter failed to push: %w", err)
	}
	return nil
}

func (p *pyroscopeIngesterAdapter) Evaluate(input *storage.PutInput) (storage.SampleObserver, bool) {
	return nil, false // noop
}

func (p *pyroscopeIngesterAdapter) parseToPprof(ctx context.Context, in *ingestion.IngestInput, pprofable ingestion.ParseableToPprof) error {
	plainReq, err := pprofable.ParseToPprof(ctx, in.Metadata)
	// ParseToPprof allocates pprof.Profile that have to be closed after use.
	defer func() {
		if plainReq == nil {
			return
		}
		for _, s := range plainReq.Series {
			if s == nil {
				continue
			}
			for _, x := range s.Samples {
				if x != nil && x.Profile != nil {
					x.Profile.Close()
				}
			}
		}
	}()
	if err != nil {
		return fmt.Errorf("parsing IngestInput-pprof failed %w", err)
	}
	if len(plainReq.Series) == 0 {
		tenantID, _ := tenant.ExtractTenantIDFromContext(ctx)
		_ = level.Debug(p.log).Log("msg", "empty profile",
			"application", in.Metadata.Key.AppName(),
			"orgID", tenantID)
		return nil
	}
	_, err = p.svc.PushParsed(ctx, plainReq)
	if err != nil {
		return fmt.Errorf("pushing IngestInput-pprof failed %w", err)
	}
	return nil
}

func convertMetadata(pi *storage.PutInput) (metricName, stType, stUnit, app string, err error) {
	app = pi.Key.AppName()
	parts := strings.Split(app, ".")
	if len(parts) <= 1 {
		stType = stTypeCPU
	} else {
		stType = parts[len(parts)-1]
		app = strings.Join(parts[:len(parts)-1], ".")
	}
	switch stType {
	case stTypeCPU:
		metricName = metricProcessCPU
		stType = stTypeSamples
		stUnit = stUnitCount
	case "wall":
		metricName = metricWall
		stType = stTypeSamples
		stUnit = stUnitCount
	case "inuse_objects":
		metricName = metricMemory
		stUnit = stUnitCount
	case "inuse_space":
		metricName = metricMemory
		stUnit = stUnitBytes
	case "alloc_objects":
		metricName = metricMemory
		stUnit = stUnitCount
	case "alloc_space":
		metricName = metricMemory
		stUnit = stUnitBytes
	case "goroutines":
		metricName = "goroutine"
		stUnit = stUnitCount
	case "mutex_count":
		stType = stTypeContentions
		stUnit = stUnitCount
		metricName = metricMutex
	case "mutex_duration":
		stType = stTypeDelay
		stUnit = stUnitNanos
		metricName = metricMutex
	case "block_count":
		stType = stTypeContentions
		stUnit = stUnitCount
		metricName = metricBlock
	case "block_duration":
		stType = stTypeDelay
		stUnit = stUnitNanos
		metricName = metricBlock
	case "itimer":
		metricName = metricProcessCPU
		stType = stTypeSamples
		stUnit = stUnitCount
	case "alloc_in_new_tlab_objects":
		metricName = metricMemory
		stType = "alloc_in_new_tlab_objects"
		stUnit = stUnitCount
	case "alloc_in_new_tlab_bytes":
		metricName = metricMemory
		stType = "alloc_in_new_tlab_bytes"
		stUnit = stUnitBytes
	case "alloc_outside_tlab_objects":
		metricName = metricMemory
		stType = "alloc_outside_tlab_objects"
		stUnit = stUnitCount
	case "alloc_outside_tlab_bytes":
		metricName = metricMemory
		stType = "alloc_outside_tlab_bytes"
		stUnit = stUnitBytes
	case "lock_count":
		stType = stTypeContentions
		stUnit = stUnitCount
		metricName = metricBlock
	case "lock_duration":
		stType = stTypeDelay
		stUnit = stUnitNanos
		metricName = metricBlock
	case "live":
		metricName = metricMemory
		stType = "live"
		stUnit = stUnitCount
	case "exceptions":
		metricName = "exceptions"
		stType = stTypeSamples
		stUnit = stUnitCount
	default:
		err = fmt.Errorf("unknown profile type: %s", stType)
	}

	return metricName, stType, stUnit, app, err
}
