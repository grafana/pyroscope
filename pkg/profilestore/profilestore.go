package profilestore

import (
	"bytes"
	"context"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/status"
	"github.com/google/pprof/profile"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/parca-dev/parca/pkg/metastore"
	"github.com/parca-dev/parca/pkg/parcacol"
	"github.com/polarsignals/arcticdb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"

	pushv1 "github.com/grafana/fire/pkg/gen/push/v1"
)

type ProfileStore struct {
	logger log.Logger
	tracer trace.Tracer

	metaStore metastore.ProfileMetaStore
	col       *arcticdb.ColumnStore
	table     *arcticdb.Table
}

func New(logger log.Logger, reg prometheus.Registerer, tracerProvider trace.TracerProvider) (*ProfileStore, error) {
	var (
		granuleSize         = 8 * 1024
		storageActiveMemory = int64(512 * 1024 * 1024)
	)

	// initialize metastore
	metaStore := metastore.NewBadgerMetastore(
		logger,
		reg,
		tracerProvider.Tracer("badgerinmemory"),
		metastore.NewRandomUUIDGenerator(),
	)

	col := arcticdb.New(
		reg,
		granuleSize,
		storageActiveMemory,
	)

	colDB, err := col.DB("fire")
	if err != nil {
		level.Error(logger).Log("msg", "failed to load database", "err", err)
		return nil, err
	}

	table, err := colDB.Table("stacktraces", arcticdb.NewTableConfig(
		parcacol.Schema(),
	), logger)
	if err != nil {
		level.Error(logger).Log("msg", "create table", "err", err)
		return nil, err
	}

	return &ProfileStore{
		logger:    logger,
		tracer:    tracerProvider.Tracer("profilestore"),
		col:       col,
		table:     table,
		metaStore: metaStore,
	}, nil
}

func (ps *ProfileStore) Close() error {
	ps.table.Sync()

	var result error

	if err := ps.col.Close(); err != nil {
		result = multierror.Append(result, err)
	}

	if err := ps.metaStore.Close(); err != nil {
		result = multierror.Append(result, err)
	}

	return result
}

func (ps *ProfileStore) Ingest(ctx context.Context, req *connect.Request[pushv1.PushRequest]) error {
	ingester := parcacol.NewIngester(ps.logger, ps.metaStore, ps.table)

	for _, series := range req.Msg.Series {
		ls := make(labels.Labels, 0, len(series.Labels))
		for _, l := range series.Labels {
			if valid := model.LabelName(l.Name).IsValid(); !valid {
				return status.Errorf(codes.InvalidArgument, "invalid label name: %v", l.Name)
			}

			ls = append(ls, labels.Label{
				Name:  l.Name,
				Value: l.Value,
			})
		}

		for _, sample := range series.Samples {
			p, err := profile.Parse(bytes.NewBuffer(sample.RawProfile))
			if err != nil {
				return status.Errorf(codes.InvalidArgument, "failed to parse profile: %v", err)
			}

			if err := p.CheckValid(); err != nil {
				return status.Errorf(codes.InvalidArgument, "invalid profile: %v", err)
			}

			// TODO: Support normalized
			normalized := false
			if err := ingester.Ingest(ctx, ls, p, normalized); err != nil {
				return status.Errorf(codes.Internal, "failed to ingest profile: %v", err)
			}
		}
	}
	return nil
}

func (ps *ProfileStore) Table() *arcticdb.Table {
	return ps.table
}
