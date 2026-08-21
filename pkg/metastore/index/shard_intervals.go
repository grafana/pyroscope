package index

import (
	"cmp"
	"sync"

	"go.etcd.io/bbolt"

	indexstore "github.com/grafana/pyroscope/v2/pkg/metastore/index/store"
)

// shardIntervalIndex narrows metadata queries to shard summaries whose data
// range may overlap the request. Its contents are derived from ShardIndex and
// are therefore safe to rebuild from Bolt at any time.
type shardIntervalIndex struct {
	mu      sync.RWMutex
	tenants map[string]*tenantShardIntervals
	removed map[shardIntervalKey]int
	ready   bool
}

type tenantShardIntervals struct {
	root    *intervalNode
	entries map[shardIntervalKey]intervalShard
	legacy  map[shardIntervalKey]intervalShard
}

type shardIntervalKey struct {
	partition indexstore.Partition
	tenant    string
	shard     uint32
}

type intervalShard struct {
	shard indexstore.Shard
	min   int64
	max   int64
}

type intervalNode struct {
	item   intervalShard
	left   *intervalNode
	right  *intervalNode
	height int
	maxEnd int64
}

func newShardIntervalIndex() *shardIntervalIndex {
	return &shardIntervalIndex{
		tenants: make(map[string]*tenantShardIntervals),
		removed: make(map[shardIntervalKey]int),
		ready:   true,
	}
}

func (i *shardIntervalIndex) disable() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.ready = false
}

func (i *shardIntervalIndex) replace(replacement *shardIntervalIndex) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.tenants = replacement.tenants
	i.removed = replacement.removed
	i.ready = true
}

func (i *shardIntervalIndex) upsert(shard indexstore.Shard) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.upsertUnsafe(shard)
}

func (i *shardIntervalIndex) upsertUnsafe(shard indexstore.Shard) {
	tree := i.tenants[shard.Tenant]
	if tree == nil {
		tree = &tenantShardIntervals{
			entries: make(map[shardIntervalKey]intervalShard),
			legacy:  make(map[shardIntervalKey]intervalShard),
		}
		i.tenants[shard.Tenant] = tree
	}
	key := shardIntervalKey{partition: shard.Partition, tenant: shard.Tenant, shard: shard.Shard}
	if previous, ok := tree.entries[key]; ok && !previous.matchesAll() {
		tree.root = deleteIntervalNode(tree.root, previous)
	}
	delete(tree.legacy, key)
	entry := intervalShard{
		shard: indexstore.Shard{
			Partition:  shard.Partition,
			Tenant:     shard.Tenant,
			Shard:      shard.Shard,
			ShardIndex: shard.ShardIndex,
		},
		min: shard.ShardIndex.MinTime,
		max: shard.ShardIndex.MaxTime,
	}
	tree.entries[key] = entry
	delete(i.removed, key)
	if entry.matchesAll() {
		tree.legacy[key] = entry
	} else {
		tree.root = insertIntervalNode(tree.root, entry)
	}
}

func (i *shardIntervalIndex) remove(txID int, partition indexstore.Partition, tenant string, shard uint32) {
	i.mu.Lock()
	defer i.mu.Unlock()
	tree := i.tenants[tenant]
	if tree == nil {
		return
	}
	key := shardIntervalKey{partition: partition, tenant: tenant, shard: shard}
	entry, ok := tree.entries[key]
	if !ok {
		return
	}
	if !entry.matchesAll() {
		tree.root = deleteIntervalNode(tree.root, entry)
	}
	delete(tree.entries, key)
	delete(tree.legacy, key)
	if len(tree.entries) == 0 {
		delete(i.tenants, tenant)
	}
	i.removed[key] = txID
}

// candidates returns false when this in-memory index has observed a deletion
// newer than the caller's Bolt snapshot. The caller must then scan Bolt to
// avoid excluding a shard that still exists in that older snapshot. Deletions
// are reconciled against newer snapshots because Bolt reuses IDs after a
// rolled-back write transaction.
func (i *shardIntervalIndex) candidates(tx *bbolt.Tx, store Store, start, end int64, tenants ...string) ([]indexstore.Shard, bool, error) {
	i.mu.RLock()
	if !i.ready {
		i.mu.RUnlock()
		return nil, false, nil
	}
	if len(i.removed) == 0 {
		shards := i.collectCandidates(start, end, tenants...)
		i.mu.RUnlock()
		return shards, true, nil
	}
	i.mu.RUnlock()

	i.mu.Lock()
	defer i.mu.Unlock()
	if !i.ready {
		return nil, false, nil
	}
	for key, removedAt := range i.removed {
		if tx.ID() < removedAt {
			return nil, false, nil
		}
		shard, err := store.LoadShard(tx, key.partition, key.tenant, key.shard)
		if err != nil {
			return nil, false, err
		}
		if shard != nil {
			i.upsertUnsafe(*shard)
		}
		delete(i.removed, key)
	}
	return i.collectCandidates(start, end, tenants...), true, nil
}

func (i *shardIntervalIndex) collectCandidates(start, end int64, tenants ...string) []indexstore.Shard {
	shards := make([]indexstore.Shard, 0)
	for _, tenant := range tenants {
		tree := i.tenants[tenant]
		if tree == nil {
			continue
		}
		for _, entry := range tree.legacy {
			shards = append(shards, entry.shard)
		}
		collectIntervalOverlaps(tree.root, start, end, &shards)
	}
	return shards
}

func (s intervalShard) matchesAll() bool { return s.min == 0 || s.max == 0 }

func collectIntervalOverlaps(node *intervalNode, start, end int64, shards *[]indexstore.Shard) {
	if node == nil {
		return
	}
	if node.left != nil && node.left.maxEnd >= start {
		collectIntervalOverlaps(node.left, start, end, shards)
	}
	if node.item.min <= end && node.item.max >= start {
		*shards = append(*shards, node.item.shard)
	}
	if node.item.min <= end {
		collectIntervalOverlaps(node.right, start, end, shards)
	}
}

func insertIntervalNode(node *intervalNode, item intervalShard) *intervalNode {
	if node == nil {
		return &intervalNode{item: item, height: 1, maxEnd: item.max}
	}
	if compareIntervalShards(item, node.item) < 0 {
		node.left = insertIntervalNode(node.left, item)
	} else {
		node.right = insertIntervalNode(node.right, item)
	}
	return balanceIntervalNode(node)
}

func deleteIntervalNode(node *intervalNode, item intervalShard) *intervalNode {
	if node == nil {
		return nil
	}
	switch cmp := compareIntervalShards(item, node.item); {
	case cmp < 0:
		node.left = deleteIntervalNode(node.left, item)
	case cmp > 0:
		node.right = deleteIntervalNode(node.right, item)
	default:
		if node.left == nil {
			return node.right
		}
		if node.right == nil {
			return node.left
		}
		successor := node.right
		for successor.left != nil {
			successor = successor.left
		}
		node.item = successor.item
		node.right = deleteIntervalNode(node.right, successor.item)
	}
	return balanceIntervalNode(node)
}

func compareIntervalShards(a, b intervalShard) int {
	if c := cmp.Compare(a.min, b.min); c != 0 {
		return c
	}
	if c := a.shard.Partition.Timestamp.Compare(b.shard.Partition.Timestamp); c != 0 {
		return c
	}
	if c := cmp.Compare(a.shard.Partition.Duration, b.shard.Partition.Duration); c != 0 {
		return c
	}
	return cmp.Compare(a.shard.Shard, b.shard.Shard)
}

func balanceIntervalNode(node *intervalNode) *intervalNode {
	updateIntervalNode(node)
	balance := intervalNodeHeight(node.left) - intervalNodeHeight(node.right)
	if balance > 1 {
		if intervalNodeHeight(node.left.left) < intervalNodeHeight(node.left.right) {
			node.left = rotateIntervalLeft(node.left)
		}
		return rotateIntervalRight(node)
	}
	if balance < -1 {
		if intervalNodeHeight(node.right.right) < intervalNodeHeight(node.right.left) {
			node.right = rotateIntervalRight(node.right)
		}
		return rotateIntervalLeft(node)
	}
	return node
}

func rotateIntervalLeft(node *intervalNode) *intervalNode {
	right := node.right
	node.right = right.left
	right.left = node
	updateIntervalNode(node)
	updateIntervalNode(right)
	return right
}

func rotateIntervalRight(node *intervalNode) *intervalNode {
	left := node.left
	node.left = left.right
	left.right = node
	updateIntervalNode(node)
	updateIntervalNode(left)
	return left
}

func updateIntervalNode(node *intervalNode) {
	node.height = max(intervalNodeHeight(node.left), intervalNodeHeight(node.right)) + 1
	node.maxEnd = max(node.item.max, max(intervalNodeMaxEnd(node.left), intervalNodeMaxEnd(node.right)))
}

func intervalNodeHeight(node *intervalNode) int {
	if node == nil {
		return 0
	}
	return node.height
}

func intervalNodeMaxEnd(node *intervalNode) int64 {
	if node == nil {
		return -1 << 63
	}
	return node.maxEnd
}
