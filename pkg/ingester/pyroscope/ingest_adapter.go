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
		Labels: make([]*typesv1.LabelPair, 0, 1+len(pi.Key.Labels())),
	}
	series.Labels = append(series.Labels, &typesv1.LabelPair{
		Name:  labels.MetricName,
		Value: metric,
	}, &typesv1.LabelPair{
		Name:  "pyroscope_app",
		Value: app,
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
		ID:         uuid.New().String(), //todo generate on a client
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
	app = pi.Key.AppName()
	parts := strings.Split(app, ".")
	if len(parts) > 1 {
		stType = parts[len(parts)-1]
		app = strings.Join(parts[:len(parts)-1], ".")
	}
	switch stType {
	case "cpu":
		metricName = "process_cpu"
		stType = "samples"
		stUnit = "count"
	case "wall":
		metricName = "wall"
		stType = "samples"
		stUnit = "count"
	case "inuse_objects":
		metricName = "memory"
		stUnit = "count"
	case "inuse_space":
		metricName = "memory"
		stUnit = "bytes"
	case "alloc_objects":
		metricName = "memory"
		stUnit = "count"
	case "alloc_space":
		metricName = "memory"
		stUnit = "bytes"
	case "goroutines":
		metricName = "goroutine"
		stUnit = "count"
	case "mutex_count":
		stType = "contentions"
		stUnit = "count"
		metricName = "mutex"
	case "mutex_duration":
		stType = "delay"
		stUnit = "nanoseconds"
		metricName = "mutex"
	case "block_count":
		stType = "contentions"
		stUnit = "count"
		metricName = "block"
	case "block_duration":
		stType = "delay"
		stUnit = "nanoseconds"
		metricName = "block"
	case "itimer":
		metricName = "process_cpu"
		stType = "samples"
		stUnit = "count"
	case "alloc_in_new_tlab_objects":
		metricName = "memory"
		stType = "alloc_in_new_tlab_objects"
		stUnit = "count"
	case "alloc_in_new_tlab_bytes":
		metricName = "memory"
		stType = "alloc_in_new_tlab_bytes"
		stUnit = "bytes"
	case "alloc_outside_tlab_objects":
		metricName = "memory"
		stType = "alloc_outside_tlab_objects"
		stUnit = "count"
	case "alloc_outside_tlab_bytes":
		metricName = "memory"
		stType = "alloc_outside_tlab_bytes"
		stUnit = "bytes"
	case "lock_count":
		stType = "contentions"
		stUnit = "count"
		metricName = "block"
	case "lock_duration":
		stType = "delay"
		stUnit = "nanoseconds"
		metricName = "block"
	case "live":
		metricName = "memory"
		stType = "live"
		stUnit = "count"
	}

	return metricName, stType, stUnit, app
}
