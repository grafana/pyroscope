package pyroscope

import (
	"context"
	"fmt"
	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/google/uuid"
	pushv1 "github.com/grafana/phlare/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/ingester"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"net/http"
	"strings"
)

func NewPyroscopeIngestHandler(svc *ingester.Ingester, logger log.Logger) http.Handler {
	var ll = logrus.StandardLogger() //todo adapter to logkit
	return server.NewIngestHandler(
		ll,
		&pyroscopeIngesterAdapter{svc: svc},
		func(input *ingestion.IngestInput) {},
		httputils.NewDefaultHelper(ll),
	)
}

type pyroscopeIngesterAdapter struct {
	svc *ingester.Ingester
}

func (p *pyroscopeIngesterAdapter) Ingest(ctx context.Context, in *ingestion.IngestInput) error {
	return in.Profile.Parse(ctx, p, p, in.Metadata)
}

func (p *pyroscopeIngesterAdapter) Put(ctx context.Context, pi *storage.PutInput) error {
	if pi.Key.HasProfileID() {
		return nil
	}
	metric, stType, stUnit, app := recoverMetadata(pi)
	pprof := pi.Val.Pprof(&tree.PprofMetadata{
		Type:      stType,
		Unit:      stUnit,
		StartTime: pi.StartTime,
	})
	// a fake mapping
	pprof.Mapping = []*tree.Mapping{{Id: 0}}

	b, err := proto.Marshal(pprof)
	if err != nil {
		return fmt.Errorf("pyroscopeIngesterAdapter failed to marshal pprof: %w", err)
	}
	req := &pushv1.PushRequest{}
	series := &pushv1.RawProfileSeries{
		Labels: make([]*typesv1.LabelPair, 0, 2+len(pi.Key.Labels())),
	}
	series.Labels = append(series.Labels, &typesv1.LabelPair{
		Name:  labels.MetricName,
		Value: metric,
	}, &typesv1.LabelPair{
		Name:  "pyroscope_app",
		Value: app,
	},
		&typesv1.LabelPair{
			Name:  phlaremodel.LabelNameDelta,
			Value: "false",
		})
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

func recoverMetadata(pi *storage.PutInput) (metricName, stType, stUnit, app string) {
	const (
		stUnitCount       = "count"
		stTypeSamples     = "samples"
		metricMemory      = "memory"
		stUnitBytes       = "bytes"
		stTypeContentions = "contentions"
		metricMutex       = "mutex"
		stTypeDelay       = "delay"
		stUniNanos        = "nanoseconds"
		metricBlock       = "block"
	)
	app = pi.Key.AppName()
	parts := strings.Split(app, ".")
	if len(parts) > 1 {
		stType = parts[len(parts)-1]
		app = strings.Join(parts[:len(parts)-1], ".")
	}
	switch stType {
	case "cpu":
		metricName = "process_cpu"
		stType = stTypeSamples
		stUnit = stUnitCount
	case "wall":
		metricName = "wall"
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
		stUnit = stUniNanos
		metricName = metricMutex
	case "block_count":
		stType = stTypeContentions
		stUnit = stUnitCount
		metricName = metricBlock
	case "block_duration":
		stType = stTypeDelay
		stUnit = stUniNanos
		metricName = metricBlock
	case "itimer":
		metricName = "process_cpu"
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
		stUnit = stUniNanos
		metricName = metricBlock
	case "live":
		metricName = metricMemory
		stType = "live"
		stUnit = stUnitCount
	}

	return metricName, stType, stUnit, app
}
