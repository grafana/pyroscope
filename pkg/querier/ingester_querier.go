package querier

// import (
// 	"context"
// 	"time"

// 	"github.com/grafana/dskit/ring"
// 	"github.com/grafana/dskit/services"
// 	"github.com/grafana/loki/pkg/distributor/clientpool"
// 	"github.com/grafana/loki/pkg/logproto"
// 	"github.com/pkg/errors"
// 	"go.etcd.io/etcd/client"
// )

// type responseFromIngesters struct {
// 	addr     string
// 	response interface{}
// }

// // IngesterQuerier helps with querying the ingesters.
// type IngesterQuerier struct {
// 	ring            ring.ReadRing
// 	pool            *ring_client.Pool
// 	extraQueryDelay time.Duration
// }

// func NewIngesterQuerier(clientCfg client.Config, ring ring.ReadRing, extraQueryDelay time.Duration) (*IngesterQuerier, error) {
// 	factory := func(addr string) (ring_client.PoolClient, error) {
// 		return client.New(clientCfg, addr)
// 	}

// 	return newIngesterQuerier(clientCfg, ring, extraQueryDelay, factory)
// }

// // newIngesterQuerier creates a new IngesterQuerier and allows to pass a custom ingester client factory
// // used for testing purposes
// func newIngesterQuerier(clientCfg client.Config, ring ring.ReadRing, extraQueryDelay time.Duration, clientFactory ring_client.PoolFactory) (*IngesterQuerier, error) {
// 	iq := IngesterQuerier{
// 		ring:            ring,
// 		pool:            clientpool.NewPool(clientCfg.PoolConfig, ring, clientFactory, util_log.Logger),
// 		extraQueryDelay: extraQueryDelay,
// 	}

// 	err := services.StartAndAwaitRunning(context.Background(), iq.pool)
// 	if err != nil {
// 		return nil, errors.Wrap(err, "querier pool")
// 	}

// 	return &iq, nil
// }

// // forAllIngesters runs f, in parallel, for all ingesters
// // TODO taken from Cortex, see if we can refactor out an usable interface.
// func (q *IngesterQuerier) forAllIngesters(ctx context.Context, f func(logproto.QuerierClient) (interface{}, error)) ([]responseFromIngesters, error) {
// 	replicationSet, err := q.ring.GetReplicationSetForOperation(ring.Read)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return q.forGivenIngesters(ctx, replicationSet, f)
// }

// // forGivenIngesters runs f, in parallel, for given ingesters
// // TODO taken from Cortex, see if we can refactor out an usable interface.
// func (q *IngesterQuerier) forGivenIngesters(ctx context.Context, replicationSet ring.ReplicationSet, f func(QuerierClient) (interface{}, error)) ([]responseFromIngesters, error) {
// 	results, err := replicationSet.Do(ctx, q.extraQueryDelay, func(ctx context.Context, ingester *ring.InstanceDesc) (interface{}, error) {
// 		client, err := q.pool.GetClientFor(ingester.Addr)
// 		if err != nil {
// 			return nil, err
// 		}

// 		resp, err := f(client.(QuerierClient))
// 		if err != nil {
// 			return nil, err
// 		}

// 		return responseFromIngesters{ingester.Addr, resp}, nil
// 	})
// 	if err != nil {
// 		return nil, err
// 	}

// 	responses := make([]responseFromIngesters, 0, len(results))
// 	for _, result := range results {
// 		responses = append(responses, result.(responseFromIngesters))
// 	}

// 	return responses, err
// }

// // func (q *IngesterQuerier) Label(ctx context.Context, req *logproto.LabelRequest) ([][]string, error) {
// // 	resps, err := q.forAllIngesters(ctx, func(client logproto.QuerierClient) (interface{}, error) {
// // 		return client.Label(ctx, req)
// // 	})
// // 	if err != nil {
// // 		return nil, err
// // 	}

// // 	results := make([][]string, 0, len(resps))
// // 	for _, resp := range resps {
// // 		results = append(results, resp.response.(*logproto.LabelResponse).Values)
// // 	}

// // 	return results, nil
// // }
