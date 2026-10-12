package querybackend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

// resultCacheCompactionLevel is the only block level eligible for caching.
// Level-2 blocks are fully compacted and immutable, and a request must cover
// the block's complete time range for its result to be complete and stable.
const resultCacheCompactionLevel = uint32(2)

// Incomplete reads retain the normal response semantics but must not be cached.
var errResultCacheIncomplete = errors.New("block execution skipped missing data")

// executeBlock is called only by READ leaves, after tenant dataset filtering.
// It never changes the request or its query plan.
func (q *resultCache) executeBlock(ctx context.Context, req *queryv1.InvokeRequest, block *metastorev1.BlockMeta, execute func(context.Context) (*queryv1.InvokeResponse, error)) (*queryv1.InvokeResponse, error) {
	if !q.blockCacheEligible(req, block) {
		return execute(ctx)
	}
	return q.raceBlockResultCache(ctx, req, block, execute)
}

func (q *resultCache) blockCacheEligible(req *queryv1.InvokeRequest, block *metastorev1.BlockMeta) bool {
	return q.resultCacheEligible(req) && block.CompactionLevel == resultCacheCompactionLevel && req.StartTime <= block.MinTime && req.EndTime >= block.MaxTime
}

// Include the selected datasets as well as the block ID: metastore plans may
// select different tenant/service datasets from the same physical block.
func blockResultCacheIdentity(req *queryv1.InvokeRequest, block *metastorev1.BlockMeta) (*queryv1.ResultCacheKey, error) {
	query, err := cacheQuery(req, block.MinTime, block.MaxTime)
	if err != nil {
		return nil, err
	}
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(block)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	return &queryv1.ResultCacheKey{Query: query, BlockIds: []string{block.Id, hex.EncodeToString(digest[:])}}, nil
}

func blockResultCacheKey(tenant string, generation uint, identity *queryv1.ResultCacheKey) (string, error) {
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(identity)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return fmt.Sprintf("results-cache/blocks/%s/%04d-%s", tenant, generation, hex.EncodeToString(digest[:])), nil
}

func (q *resultCache) raceBlockResultCache(ctx context.Context, req *queryv1.InvokeRequest, block *metastorev1.BlockMeta, execute func(context.Context) (*queryv1.InvokeResponse, error)) (*queryv1.InvokeResponse, error) {
	identity, err := blockResultCacheIdentity(req, block)
	if err != nil {
		return execute(ctx)
	}
	key, err := blockResultCacheKey(req.Tenant[0], q.resultCacheOverrides.ResultCacheGeneration(req.Tenant[0]), identity)
	if err != nil {
		return execute(ctx)
	}
	queryType, _ := resultCacheQueryType(req.Query)
	raceCtx, cancel := context.WithCancel(ctx)
	// Speculative execution owns its collectors and object readers. A cache
	// hit can cancel it without waiting for its readers to drain.
	defer cancel()
	type lookupResult struct {
		response *queryv1.InvokeResponse
		hit      bool
	}
	type executionResult struct {
		response *queryv1.InvokeResponse
		err      error
	}
	lookup := make(chan lookupResult, 1)
	execution := make(chan executionResult, 1)
	go func() {
		aggregator := newAggregator(req)
		var bytesFetched uint64
		hit, err := q.readResultCacheWithCost(raceCtx, queryType, key, identity, aggregator, &bytesFetched)
		resp := aggregator.response()
		resp.Diagnostics = &queryv1.Diagnostics{ExecutionNode: &queryv1.ExecutionNode{Stats: &queryv1.ExecutionStats{BytesFetched: bytesFetched}}}
		lookup <- lookupResult{response: resp, hit: hit && err == nil}
	}()
	go func() {
		timer := time.NewTimer(q.resultCacheExecutionDelay)
		defer timer.Stop()
		select {
		case <-raceCtx.Done():
			return
		case <-timer.C:
		}
		var resp *queryv1.InvokeResponse
		err := util.RecoverPanic(func() error {
			var err error
			resp, err = execute(raceCtx)
			return err
		})()
		execution <- executionResult{response: resp, err: err}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-lookup:
			if result.hit {
				cancel()
				return result.response, nil
			}
			lookup = nil
		case result := <-execution:
			cancel()
			if result.err == nil && ctx.Err() == nil {
				q.enqueueResultCacheWrite(resultCacheWriteJob{queryType: queryType, key: key, identity: identity, reports: cloneReports(result.response.Reports), bytesFetched: result.response.GetDiagnostics().GetExecutionNode().GetStats().GetBytesFetched()})
			}
			if errors.Is(result.err, errResultCacheIncomplete) {
				return result.response, nil
			}
			return result.response, result.err
		}
	}
}
