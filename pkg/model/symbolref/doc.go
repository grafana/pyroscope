// Package symbolref implements symbol-aware tree references: a
// model.LocationRefNameTree node name can refer to either a resolved frame
// name or an unresolved native {buildID, address} location, sharing a
// single integer reference space (model.LocationRefName) alongside a side
// table (queryv1.SymbolRefTable). This lets a partial or merged tree carry
// unresolved locations through query-plan merges without leaving the
// native tree representation, deferring resolution and truncation until
// after the final merge.
//
// Responsibility boundary:
//
// Owns: the symbol-reference table (interning and deduplication), ref-space
// partitioning (resolved vs. unresolved vs. the truncation "other"
// sentinel), merge-time ref rebasing, marshal-time compaction and
// ordering, deferred-truncation bookkeeping, grouping unresolved
// references into per-build-ID resolution jobs, and rebuilding/truncating
// the final resolved tree.
//
// Does NOT own: the generic tree node/marshal format (pkg/model's Tree,
// reused unchanged), building per-dataset trees from block data
// (pkg/phlaredb/symdb, pkg/querybackend), fetching or resolving debug info
// (pkg/symbolizer), or query orchestration (pkg/frontend/...).
package symbolref
