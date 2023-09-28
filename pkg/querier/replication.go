package querier

import (
	"context"

	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/ring"
	"golang.org/x/sync/errgroup"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	"github.com/grafana/pyroscope/pkg/util"
)

type ResponseFromReplica[T any] struct {
	addr     string
	response T
}

type QueryReplicaFn[T any, Querier any] func(ctx context.Context, q Querier) (T, error)

type QueryReplicaWithHintsFn[T any, Querier any] func(ctx context.Context, q Querier, hint *ingestv1.Hints) (T, error)

type Closer interface {
	CloseRequest() error
	CloseResponse() error
}

type ClientFactory[T any] func(addr string) (T, error)

// forGivenReplicationSet runs f, in parallel, for given replica set.
// Under the hood it returns only enough responses to satisfy the quorum.
func forGivenReplicationSet[Result any, Querier any](ctx context.Context, clientFactory func(string) (Querier, error), replicationSet ring.ReplicationSet, f QueryReplicaFn[Result, Querier]) ([]ResponseFromReplica[Result], error) {
	results, err := ring.DoUntilQuorumWithoutSuccessfulContextCancellation(
		ctx,
		replicationSet,
		ring.DoUntilQuorumConfig{
			MinimizeRequests: true,
		},
		func(ctx context.Context, ingester *ring.InstanceDesc, _ context.CancelFunc) (ResponseFromReplica[Result], error) {
			var res ResponseFromReplica[Result]
			client, err := clientFactory(ingester.Addr)
			if err != nil {
				return res, err
			}

			resp, err := f(ctx, client)
			if err != nil {
				return res, err
			}

			return ResponseFromReplica[Result]{ingester.Addr, resp}, nil
		},
		func(result ResponseFromReplica[Result]) {
			// If the result was streamed, we need to close the request and response
			if stream, ok := any(result.response).(interface {
				CloseRequest() error
			}); ok {
				if err := stream.CloseRequest(); err != nil {
					level.Warn(util.Logger).Log("msg", "failed to close request", "err", err)
				}
			}
			if stream, ok := any(result.response).(interface {
				CloseResponse() error
			}); ok {
				if err := stream.CloseResponse(); err != nil {
					level.Warn(util.Logger).Log("msg", "failed to close response", "err", err)
				}
			}
		})
	if err != nil {
		return nil, err
	}

	return results, err
}

// forGivenReplicationSet runs f, in parallel, for given replica set.
// Under the hood it returns only enough responses to satisfy the quorum.
// TODO: Implement cleanup
func forGivenPlan[Result any, Querier any](ctx context.Context, plan map[string]*ingestv1.BlockHints, clientFactory func(string) (Querier, error), replicationSet ring.ReplicationSet, f QueryReplicaWithHintsFn[Result, Querier]) ([]ResponseFromReplica[Result], error) {
	g, gctx := errgroup.WithContext(ctx)

	var (
		idx    = 0
		result = make([]ResponseFromReplica[Result], len(plan))
	)

	for replica, hints := range plan {
		// skip replicas not in the replica set
		if !replicationSet.Includes(replica) {
			continue
		}
		var (
			i = idx
			r = replica
			h = hints
		)
		idx++
		g.Go(func() error {

			client, err := clientFactory(r)
			if err != nil {
				return err
			}

			resp, err := f(gctx, client, &ingestv1.Hints{Block: h})
			if err != nil {
				return err
			}

			result[i] = ResponseFromReplica[Result]{r, resp}

			return nil

		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return result[:idx], nil
}
