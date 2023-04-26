package pyroscope

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"google.golang.org/protobuf/proto"

	pushv1 "github.com/grafana/phlare/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/phlare/pkg/model"
)

type PushService interface {
	Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error)
}

func NewPyroscopeIngestHandler(svc PushService, logger log.Logger) http.Handler {
	return server.NewIngestHandler(
		logger,
		&pyroscopeIngesterAdapter{svc: svc},
		func(input *ingestion.IngestInput) {},
		httputils.NewLogKitErrorUtils(logger),
	)
}

type pyroscopeIngesterAdapter struct {
	svc PushService
}

func (p *pyroscopeIngesterAdapter) Ingest(ctx context.Context, in *ingestion.IngestInput) error {
	return in.Profile.Parse(ctx, p, p, in.Metadata)
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
		return fmt.Errorf("pyroscopeIngesterAdapter failed to convert metadata: %w", err)
	}
	mdata := &tree.PprofMetadata{
		Type:      stType,
		Unit:      stUnit,
		StartTime: pi.StartTime,
	}
	if pi.SampleRate != 0 && (metric == metricWall || metric == metricProcessCPU) {
		period := time.Second.Nanoseconds() / int64(pi.SampleRate)
		mdata.Period = period
		mdata.PeriodType = "cpu"
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
		return fmt.Errorf("pyroscopeIngesterAdapter failed to marshal pprof: %w", err)
	}
	req := &pushv1.PushRequest{}
	series := &pushv1.RawProfileSeries{
		Labels: make([]*typesv1.LabelPair, 0, 3+len(pi.Key.Labels())),
	}
	series.Labels = append(series.Labels, &typesv1.LabelPair{
		Name:  labels.MetricName,
		Value: metric,
	}, &typesv1.LabelPair{
		Name:  "pyroscope_app",
		Value: app,
	}, &typesv1.LabelPair{
		Name:  phlaremodel.LabelNameDelta,
		Value: "false",
	})
	if pi.SpyName != "" {
		series.Labels = append(series.Labels, &typesv1.LabelPair{
			Name:  "pyroscope_spy",
			Value: pi.SpyName,
		})
	}
	for k, v := range pi.Key.Labels() {
		if strings.HasPrefix(k, "__") {
			continue
		}
		series.Labels = append(series.Labels, &typesv1.LabelPair{
			Name:  k,
			Value: v,
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

func convertMetadata(pi *storage.PutInput) (metricName, stType, stUnit, app string, err error) {
	app = pi.Key.AppName()
	parts := strings.Split(app, ".")
	if len(parts) <= 1 {
		err = fmt.Errorf("app name is not in the format of <app>.<profile_type> - %s", app)
		return metricName, stType, stUnit, app, err
	}
	stType = parts[len(parts)-1]
	app = strings.Join(parts[:len(parts)-1], ".")
	switch stType {
	case "cpu":
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
