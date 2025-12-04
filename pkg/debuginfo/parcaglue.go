package debuginfo

import (
	"github.com/go-kit/log"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/parca/debuginfo"
	debuginfogrpc "github.com/grafana/pyroscope/pkg/parca/gen/proto/go/parca/debuginfo/v1alpha1"
	"github.com/grafana/pyroscope/pkg/symbolizer"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
)

func NewParcaDebugInfo(l log.Logger, bucket objstore.Bucket, cfg symbolizer.Config, server *grpc.Server) *debuginfo.Store {
	t := noop.Tracer{}
	l = log.With(l, "component", "debug-info")
	bucket = objstore.NewPrefixedBucket(bucket, symbolizer.BucketPrefixParcaDebugInfo)
	md := debuginfo.NewObjectStoreMetadata(l, bucket)
	debuginfod := debuginfo.NewParallelDebuginfodClients(nil)
	store, _ := debuginfo.NewStore(t, l, md, bucket, debuginfod,
		debuginfo.SignedUpload{Enabled: false},
		cfg.MaxUploadDuration, cfg.MaxUploadSize)

	debuginfogrpc.RegisterDebuginfoServiceServer(server, store)
	return store
}
