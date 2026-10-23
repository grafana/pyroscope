# Stacktrace tree insertion benchmarks

These benchmarks measure `StacktraceTree.Insert`. Compare changes to the
implementation using the same
fixtures, Go version, hardware, CPU settings, and benchmark commands.

## Synthetic workloads

`stacktraces_bench_test.go` supplies deterministic, leaf-first stacks:

- Single chains with depths 16, 64, and 256.
- Fanouts 8, 64, 512, and 4096, both at the root and after a 32-frame prefix.
- Balanced trees with branching 2 / depth 12 and branching 8 / depth 4.
- A mixed tree with variable depths, mostly narrow branches, and a few wide
  parents. Location IDs are reused under different parents and at different
  depths to catch incorrectly keyed child indexes.

One operation processes an entire batch:

| Mode | Timed work | Batch |
| --- | --- | --- |
| `BuildFresh/SmallCapacity` | Constructor, growth, and insertion | Each distinct stack once, shuffled with a fixed seed |
| `BuildFresh/ExactCapacity` | Constructor and insertion, with exact node capacity | Same build batch |
| `ResetBuild` | Reset and insertion into a previously warmed tree | Same build batch |
| `LookupExisting/Uniform` | Value updates on an already populated tree | A second deterministic permutation, repeated to at least 256 accesses |
| `LookupExisting/Skewed` | Value updates on an already populated wide tree | Approximately 80% first-two/last-two inserted children, 20% uniform |

The skewed hot set includes late siblings deliberately. Only wide workloads use
this mode. Exact capacity is a diagnostic control, not a production sizing
recommendation. ResetBuild has one capacity case because warming has already
established sufficient capacity; reset/index cleanup remains inside timing.

Input generation, access-order selection, reference construction, and validation
are outside timing. A unit test checks the fixtures against an independent
map-per-node trie, including self/total values, child/parent links, returned leaf
paths, repeated lookups, slice growth, and rebuilds after reset.

## Parent-pointer replay

`../phlaredb/symdb/resolver_tree_bench_test.go` uses the checked-in
`testdata/big-profile.pb.gz` fixture and its first sample type. Setup normalizes
and indexes the profile, aggregates samples by stack ID with `SampleAppender`,
and creates the block reader's parent-pointer representation. It does not time
parsing, indexing, sample aggregation, block IO, or a network request.

- `BenchmarkInsertStacktraces`: precomputes values and truncation, then times
  stack reconstruction, function-name expansion, and insertion into a fresh
  tree. Initial capacity equals `maxNodes`, matching the production caller.
- `BenchmarkBuildTreeForStacktraceIDRange`: times the full range conversion,
  including copying source nodes, value propagation, truncation, insertion,
  and output-tree conversion. Every operation starts from immutable source
  nodes. It excludes multi-range scheduling and merging.

Both use `maxNodes` of 8K, 16K, and 64K. They call the parent-pointer path
explicitly: the existing in-memory Resolver benchmarks instead dispatch through
per-stack resolution. The full range benchmark checks the output fingerprint
against an untimed warm-up and verifies total weight. A small-fixture unit test
also compares intermediate output against independent string-tree insertion and
checks that neither prepared nor source nodes are mutated by replay.

This corpus is a realistic regression fixture, not a capture of the query inputs
that produced the production CPU hotspot. Production CPU stacks describe the
backend's execution, not the trees its queries were building.

## Metrics

- `ns/op`, `B/op`, `allocs/op`: per batch or full range conversion.
- `ns/stack`, `ns/frame`: normalized batch time for synthetic insertion and
  `insertStacktraces` replay. Replay includes scanning source nodes and resolving
  frames, so these are not isolated per-frame instruction costs.
- `stacks/op`: insert calls per batch; `samples/op`: aggregated samples supplied
  to the full range builder before truncation.
- `nodes/tree`: resulting node count (includes the virtual root for intermediate
  StacktraceTrees, but not for final function trees).
- `source-nodes/op`: source nodes scanned/copied by the parent-pointer path.
- `node-cap-B`: node slice capacity times node size, **not total retained heap**
  or allocator-rounded bytes. Synthetic benchmarks also report `edge-cap-B` for
  the index slots and `retained-cap-B` for the sum of both backing arrays. Replay
  benchmarks report node capacity only; their `B/op` includes index allocation.
  Zero allocations in a warmed run do not imply low memory retention.

## Running and comparing

From the repository root:

```sh
# Correctness, including the new fixture tests.
go test ./pkg/model ./pkg/phlaredb/symdb

# Smoke-test every benchmark case; these timings are not a stable baseline.
go test ./pkg/model -run '^$' -bench '^BenchmarkStacktraceTreeInsert$' \
  -benchmem -benchtime=1x -cpu=1
go test ./pkg/phlaredb/symdb -run '^$' \
  -bench '^(BenchmarkInsertStacktraces|BenchmarkBuildTreeForStacktraceIDRange)$' \
  -benchmem -benchtime=1x -cpu=1

# Repeated baseline. Run sequentially, not alongside another benchmark process.
go test ./pkg/model -run '^$' -bench '^BenchmarkStacktraceTreeInsert$' \
  -benchmem -benchtime=1s -count=10 -cpu=1 > /tmp/tree-model-before.txt
go test ./pkg/phlaredb/symdb -run '^$' \
  -bench '^(BenchmarkInsertStacktraces|BenchmarkBuildTreeForStacktraceIDRange)$' \
  -benchmem -benchtime=1s -count=10 -cpu=1 > /tmp/tree-replay-before.txt

# Repeat after the candidate change, using -after filenames, then compare:
benchstat /tmp/tree-model-before.txt /tmp/tree-model-after.txt
benchstat /tmp/tree-replay-before.txt /tmp/tree-replay-after.txt

# Profile a representative replay separately from timing comparisons.
go test ./pkg/phlaredb/symdb -run '^$' \
  -bench '^BenchmarkInsertStacktraces$/^MaxNodes16384$' \
  -benchtime=5s -cpu=1 -cpuprofile=/tmp/tree-insert.cpu.pprof \
  -o /tmp/tree-symdb.test
go tool pprof -top -focus 'insertStacktraces|StacktraceTree.*Insert' \
  /tmp/tree-insert.cpu.pprof
```

`-cpu=1` sets GOMAXPROCS, not OS CPU affinity. Repeat on production-like Linux
amd64 hardware before extrapolating local results. Prefer realistic fresh-tree
and full-range improvements over isolated wide-tree wins, and check narrow-tree
regressions and memory growth before choosing an index design.

## Edge-index experiment

The candidate adapts the open-addressed `(parent, location)` table from PR #5433,
retaining child/sibling links for traversal. Slots contain only child indices and
rehash at 75% occupancy. The first four children of each parent remain unindexed;
new indexed children are linked after that stable prefix. This changes sibling
traversal order beyond the prefix, not node IDs, stack weights, or sorted output
names. Reset clears the index while retaining its capacity. Location and Parent
must remain immutable while inserting; finalization can rewrite locations, but
insertion after those rewrites requires Reset.

An initial first-child-only fast path regressed binary-tree builds by 23–34%
locally and did not consistently help replay. The bounded four-child scan reduced
those regressions substantially. This is a candidate, not a demonstrated overall
query speedup or an exhaustively tuned threshold.

Initial comparison on Apple M2 Max, darwin/arm64, Go 1.26.8, GOMAXPROCS=1:

| Replay | Baseline median | Hybrid median | benchstat result |
| --- | --- | --- | --- |
| Insert, 8K limit | 36.02 ms | 36.11 ms | No significant difference |
| Insert, 16K limit | 59.53 ms | 55.94 ms | -6.03% |
| Insert, 64K limit | 159.0 ms | 142.9 ms | -10.13% |
| Full range, 8K limit | 48.65 ms | 47.30 ms | No significant difference |
| Full range, 16K limit | 71.34 ms | 71.56 ms | No significant difference |
| Full range, 64K limit | 171.8 ms | 168.9 ms | No significant difference |

Six repetitions used 1s per replay case and 200ms per synthetic case. Exact-capacity
wide-root builds with 4096 children improved from 14.34 ms to 122.3 us. However,
binary-tree builds remained approximately 3% slower, deep-chain existing lookups
approximately 7% slower, and some tiny fanout-8 cases regressed 10–35%. These
tradeoffs and the absence of a significant full-range win warrant production-like
amd64 measurement before rollout. Allocations include additional index storage;
the storage-side PR's node-size savings do not apply here.
