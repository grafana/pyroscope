package debuginfo

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/parca/debuginfo"
	debuginfogrpc "github.com/grafana/pyroscope/pkg/parca/gen/proto/go/parca/debuginfo/v1alpha1"
	"github.com/grafana/pyroscope/pkg/symbolizer"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
)

type Config struct {
	Enabled           bool          `yaml:"-"`
	MaxUploadSize     int64         `yaml:"-"`
	MaxUploadDuration time.Duration `yaml:"-"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&cfg.Enabled, "debug-info.enabled", true, "Enable debug info.")
	f.Int64Var(&cfg.MaxUploadSize, "debug-info.max-upload-size", 100*1024*1024, "Maximum size of a single debug info upload in bytes.")
	f.DurationVar(&cfg.MaxUploadDuration, "debug-info.max-upload-duration", time.Minute, "Maximum duration of a single debug info upload.")
}

func NewParcaDebugInfo(l log.Logger, bucket objstore.Bucket, cfg Config, server *grpc.Server) error {
	if !cfg.Enabled {
		debuginfogrpc.RegisterDebuginfoServiceServer(server, disabled{})
		return nil
	}
	if bucket == nil {
		return fmt.Errorf("storage bucket is required for debug-info")
	}

	t := noop.Tracer{}
	l = log.With(l, "component", "debug-info")
	bucket = objstore.NewPrefixedBucket(bucket, symbolizer.BucketPrefixParcaDebugInfo)
	md := debuginfo.NewObjectStoreMetadata(l, bucket)
	debuginfod := debuginfo.NewParallelDebuginfodClients(nil)
	store, _ := debuginfo.NewStore(t, l, md, bucket, debuginfod,
		debuginfo.SignedUpload{Enabled: false},
		cfg.MaxUploadDuration, cfg.MaxUploadSize)

	debuginfogrpc.RegisterDebuginfoServiceServer(server, store)
	return nil
}

type disabled struct {
	debuginfogrpc.UnimplementedDebuginfoServiceServer
}

func (d disabled) ShouldInitiateUpload(context.Context, *debuginfogrpc.ShouldInitiateUploadRequest) (*debuginfogrpc.ShouldInitiateUploadResponse, error) {
	return &debuginfogrpc.ShouldInitiateUploadResponse{ShouldInitiateUpload: false, Reason: "debug info upload is disabled"}, nil
}
