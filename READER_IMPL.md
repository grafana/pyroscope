# Query-backend reader/merger split — implementation notes

Paper trail for splitting the single `query-backend` into two roles:

- **query-backend-merger** — delegates data collection to children and merges
  the results (dedup + business logic). Stateless w.r.t. object storage; can run
  with less memory.
- **query-backend-reader** ("leaf") — reads blocks from object storage. Benefits
  from lots of memory to buffer data fetched from the object store.

The query plan is unchanged: a DAG of `MERGE` and `READ` nodes
(`pkg/querybackend/queryplan/query_plan.go`). The split is purely about *which
process* executes each node type and *how children are dispatched*.

This file is the rationale/"just in case" log that we deliberately keep out of
the code. Each section names the stage and the commit subject so it can be
traced back to the diff.

---

## Stage 1 — dual-client dispatch in the query-backend (full back-compat)

Commit: `feat(querybackend): route query plan children by node type`

### What changed

`QueryBackend` (`pkg/querybackend/backend.go`) previously held a single
`backendClient` used to delegate *every* child node, plus a `blockReader` for
local `READ` execution. Role was decided per-request by node type, but all
delegation went to one round-robin pool (`query-backend.address`), so a single
pool served both roles.

Now the backend holds two downstream clients:

- `mergerClient` — where `MERGE` children are sent.
- `readerClient` — where `READ` children are sent.
- `blockReader`  — local object-storage read (nil on a pure merger).

Dispatch is decided by `clientForNode` in `merge()`: `READ` -> `readerClient`,
`MERGE` -> `mergerClient`.

### Why this is a no-op for existing deployments

`initQueryBackend` (`pkg/pyroscope/modules_experimental.go`) passes the same
`f.queryBackendClient` for *both* `mergerClient` and `readerClient`. So a
combined `query-backend` still dispatches every child to one pool exactly as
before. The split only becomes real in stage 2 when the two clients point at
different pools.

### `read()` forwarding branch — why it exists now but is dead code until stage 2

`read()` gained a `q.blockReader == nil` branch that forwards a `READ` node to
`readerClient` instead of executing locally. Rationale:

- A **pure merger** has no object-storage access (`blockReader == nil`). It must
  still handle a `READ` node that arrives as the *root* of a plan. This happens
  for single-block queries where `queryplan.Build` returns a lone `READ` root
  (see `pkg/querybackend/queryplan/query_plan.go`) and the query-frontend sends
  that root straight to a merger.
- For a combined backend (`blockReader != nil`) the branch is never taken, so
  behavior is unchanged. The branch is therefore additive and only active in a
  configuration that did not exist before stage 2.

Decision: keep the frontend "dumb" (always talk to the merger pool) and let a
merger forward `READ` roots, rather than teaching the frontend to pick a pool by
root type. Revisit in a later optimization if the extra hop for single-block
queries matters.

### Why mergers need neither Storage nor Overrides

Verified the merge path (`merge()` -> `report_aggregator.go`) touches neither
`Overrides` nor the object store. `sanitize_on_merge` is carried in
`InvokeOptions` precisely because "query workers might not have access to tenant
overrides" (see `api/query/v1/query.proto`). This is what makes a pure merger a
cheap, stateless compute tier and is the core justification for the split.

### Test topology notes

The existing tests already modeled a two-tier split (e.g. a "root" backend
delegating to a "leaf" backend), so updates were mechanical:

- reader-only backend: `New(cfg, log, reg, nil, nil, reader)`.
- merger with `READ` children: `New(cfg, log, reg, nil, handler, nil)`
  (`readerClient = handler`).
- `backend_analysis_test.go` overrides the delegate for recursion; it now sets
  **both** `mergerClient` and `readerClient` so the "nested merges" plan routes
  its inner `MERGE` child and its `READ` children back through the same handler.

---

## Stage 2 — config + new targets/modules

Commit: `feat(querybackend): add query-backend-merger and -reader targets`

### What changed

- `query-backend.reader-address` added to `Config` (`pkg/querybackend/backend.go`);
  default empty => the reader client reuses `query-backend.address`
  (combined/back-compat). Regenerated the config reference via
  `go run ./tools/doc-generator/ ...` (Makefile lines 79-80).
- New targets registered in `pkg/pyroscope/pyroscope.go`:
  - `query-backend-merger` (`initQueryBackendMerger`): `blockReader == nil`,
    `mergerClient = queryBackendClient` (peer merger pool),
    `readerClient = queryBackendReaderClient` (reader pool).
  - `query-backend-reader` (`initQueryBackendReader`): `blockReader` set, both
    outbound clients nil (a reader is a leaf and never delegates).
  - `query-backend-reader-client` (invisible): dials `reader-address`, or
    `address` when `reader-address` is empty.
- `query-backend` (combined) and `-target=all` are unchanged: `all` still pulls
  in `query-backend`, not the split targets.
- Deps: `QueryBackendMerger: {API, QueryBackendClient, QueryBackendReaderClient}`
  (deliberately no `Storage`/`Overrides` — see stage 1 rationale);
  `QueryBackendReader: {Overrides, API, Storage}`.
- Concurrency interceptor (`pkg/pyroscope/modules.go`) now also fires for the
  merger and reader targets.

### Decisions / gotchas

- **Reader-client config copy**: `initQueryBackendReaderClient` copies
  `GRPCClientConfig` by value before setting `.Middleware`, so it does not stomp
  the merger client's config when both init in one process.
- **UI catch-all not extended**: the merger/reader are microservice-only and the
  query-frontend already serves the UI, so they are intentionally left out of
  the `RegisterCatchAll` list in `pkg/pyroscope/pyroscope.go`. The combined
  `query-backend` keeps registering it (single-binary/debug convenience).
- **Do not co-locate roles in one process**: both init paths call
  `API.RegisterQueryBackend`, which registers the gRPC service. Running e.g.
  `-target=query-backend-merger,query-backend-reader` (or either alongside
  `query-backend`) would double-register and panic. Use the combined
  `query-backend` for a single process; the split targets are for separate
  deployments. Mirrors the existing single-registration assumption.
- **Reader receiving a MERGE node** would hit a nil `mergerClient`. By design a
  reader only ever receives `READ` nodes (frontend -> merger -> reader), so this
  is left as an invariant rather than a guarded error for the POC.
- **`query-backend.address` in split mode** = the merger pool (the frontend and
  mergers both dial it); `query-backend.reader-address` = the reader pool. Set
  by helm in stage 3.
- **v1 storage**: the split targets are not added to the `v2Modules`/`All`
  pruning logic because they are never in `All`; a `-target=query-backend-*`
  under `architecture.storage=v1` is an operator error, same as the combined
  target today.

## Stage 3 — helm chart (planned)

- Add `query-backend-merger` (low memory) and `query-backend-reader` (high
  memory, more replicas) under `architecture.microservices.v2`.
- Point `-query-backend.address` at the merger headless service and
  `-query-backend.reader-address` at the reader headless service; the
  query-frontend targets the merger pool.
- Regenerate `operations/pyroscope/helm/pyroscope/rendered/*`.

## Stage 4 — integration test (planned)

- Add a split variant to `pkg/test/integration/cluster/cluster_v2.go`.

## Selection strategy (future)

Reader selection is gRPC client-side `round_robin` today (see `grpcServiceConfig`
in `pkg/querybackend/client/client.go`), which is the POC. Load-aware or
cache-affinity routing (e.g. consistent hashing `READ` nodes by block id) is a
separable follow-up requiring an app-level picker instead of the built-in
balancer.
