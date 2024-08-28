package read_path

import (
	"context"
	"math/rand"

	"github.com/go-kit/log"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	metastoreclient "github.com/grafana/pyroscope/pkg/experiment/metastore/client"
	"github.com/grafana/pyroscope/pkg/experiment/querybackend"
	querybackendclient "github.com/grafana/pyroscope/pkg/experiment/querybackend/client"
	"github.com/grafana/pyroscope/pkg/experiment/querybackend/queryplan"
	"github.com/grafana/pyroscope/pkg/frontend"
)

type Overrides interface {
	ReadPathOverrides(tenantID string) Config
}

type Router struct {
	logger    log.Logger
	overrides Overrides
	limits    frontend.Limits

	frontend *frontend.Frontend
	backend  *QueryBackend
}

type QueryBackend struct {
	logger       log.Logger
	limits       frontend.Limits
	metastore    *metastoreclient.Client
	querybackend *querybackendclient.Client
}

var xrand = rand.New(rand.NewSource(4349676827832284783))

func (q *QueryBackend) Query(
	ctx context.Context,
	startTime int64,
	endTime int64,
	tenants []string,
	labelSelector string,
	query *querybackendv1.Query,
) (*querybackendv1.Report, error) {
	md, err := q.metastore.QueryMetadata(ctx, &metastorev1.QueryMetadataRequest{
		TenantId:  tenants,
		StartTime: startTime,
		EndTime:   endTime,
		Query:     labelSelector,
	})
	if err != nil {
		return nil, err
	}
	if len(md.Blocks) == 0 {
		return nil, nil
	}

	// TODO(kolesnikovae): Improve distribution.
	// Randomize the order of blocks to avoid hotspots.
	xrand.Shuffle(len(md.Blocks), func(i, j int) {
		md.Blocks[i], md.Blocks[j] = md.Blocks[j], md.Blocks[i]
	})
	p := queryplan.Build(md.Blocks, 2, 10)

	resp, err := q.querybackend.Invoke(ctx, &querybackendv1.InvokeRequest{
		Tenant:        tenants,
		StartTime:     startTime,
		EndTime:       endTime,
		LabelSelector: labelSelector,
		Options:       &querybackendv1.InvokeOptions{},
		QueryPlan:     p.Proto(),
		Query:         []*querybackendv1.Query{query},
	})
	if err != nil {
		return nil, err
	}

	return findReport(querybackend.QueryReportType(query.QueryType), resp.Reports), nil
}
