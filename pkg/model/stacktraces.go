package model

import (
	"bytes"
	"container/heap"
	"encoding/binary"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"github.com/cespare/xxhash/v2"

	"github.com/grafana/pyroscope/pkg/og/util/varint"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
)

func MergeBatchMergeStacktraces(responses ...*ingestv1.MergeProfilesStacktracesResult) *ingestv1.MergeProfilesStacktracesResult {
	var (
		result      *ingestv1.MergeProfilesStacktracesResult
		posByName   map[string]int32
		hasher      StacktracesHasher
		stacktraces = map[uint64]*ingestv1.StacktraceSample{}
	)

	largestNames := 0

	for _, resp := range responses {
		if resp != nil {
			if len(resp.FunctionNames) > largestNames {
				largestNames = len(resp.FunctionNames)
			}
		}
	}
	rewrite := make([]int32, largestNames)

	for _, resp := range responses {
		// skip empty results
		if resp == nil || len(resp.Stacktraces) == 0 {
			continue
		}

		// first non-empty result result
		if result == nil {
			result = resp
			for _, s := range result.Stacktraces {
				stacktraces[hasher.Hashes(s.FunctionIds)] = s
			}
			continue
		}

		// build up the lookup map the first time
		if posByName == nil {
			posByName = make(map[string]int32, len(result.FunctionNames))
			for idx, n := range result.FunctionNames {
				posByName[n] = int32(idx)
			}
		}

		// lookup and add missing functionNames
		var (
			rewrite = rewrite[:len(resp.FunctionNames)]
			ok      bool
		)
		for idx, n := range resp.FunctionNames {
			rewrite[idx], ok = posByName[n]
			if ok {
				continue
			}

			// need to add functionName to list
			rewrite[idx] = int32(len(result.FunctionNames))
			result.FunctionNames = append(result.FunctionNames, n)
		}

		// rewrite existing function ids, by building a list of unique slices
		functionIDsUniq := make(map[*int32][]int32)
		for _, sample := range resp.Stacktraces {
			if len(sample.FunctionIds) == 0 {
				continue
			}
			functionIDsUniq[&sample.FunctionIds[0]] = sample.FunctionIds

		}
		// now rewrite those ids in slices
		for _, slice := range functionIDsUniq {
			for idx, functionID := range slice {
				slice[idx] = rewrite[functionID]
			}
		}
		// if the stacktraces is missing add it or merge it.
		for _, sample := range resp.Stacktraces {
			if len(sample.FunctionIds) == 0 {
				continue
			}
			hash := hasher.Hashes(sample.FunctionIds)
			if existing, ok := stacktraces[hash]; ok {
				existing.Value += sample.Value
			} else {
				stacktraces[hash] = sample
				result.Stacktraces = append(result.Stacktraces, sample)
			}
		}
	}

	// ensure nil will always be the empty response
	if result == nil {
		result = &ingestv1.MergeProfilesStacktracesResult{}
	}

	// sort stacktraces by function name
	sortStacktraces(result)

	return result
}

type StacktracesHasher struct {
	hash *xxhash.Digest
	b    [4]byte
}

// todo we might want to reuse the results to avoid allocations
func (h StacktracesHasher) Hashes(fnIds []int32) uint64 {
	if h.hash == nil {
		h.hash = xxhash.New()
	} else {
		h.hash.Reset()
	}

	for _, locID := range fnIds {
		binary.LittleEndian.PutUint32(h.b[:], uint32(locID))
		if _, err := h.hash.Write(h.b[:]); err != nil {
			panic("unable to write hash")
		}
	}
	return h.hash.Sum64()
}

// sortStacktraces sorts the stacktraces by function name
func sortStacktraces(r *ingestv1.MergeProfilesStacktracesResult) {
	sort.Slice(r.Stacktraces, func(i, j int) bool {
		pos := 0
		for {
			// check slice lengths
			if pos >= len(r.Stacktraces[i].FunctionIds) {
				break
			}
			if pos >= len(r.Stacktraces[j].FunctionIds) {
				return false
			}

			if diff := strings.Compare(r.FunctionNames[r.Stacktraces[i].FunctionIds[pos]], r.FunctionNames[r.Stacktraces[j].FunctionIds[pos]]); diff < 0 {
				break
			} else if diff > 0 {
				return false
			}
			pos++
		}

		// when we get here, i is less than j
		return true
	})
}

type StacktraceMerger struct {
	mu sync.Mutex
	s  *StacktraceTree
	r  *functionsRewriter
}

// NewStackTraceMerger merges collections of StacktraceSamples.
// The result is a byte tree representation of the merged samples.
func NewStackTraceMerger() *StacktraceMerger {
	return new(StacktraceMerger)
}

// MergeStackTraces adds the stack traces to the resulting tree.
// The call is thread-safe, but the resulting tree bytes should
// be only build after all the samples are merged.
// Note that the function may reuse the capacity of the names slice.
func (m *StacktraceMerger) MergeStackTraces(stacks []*ingestv1.StacktraceSample, names []string) {
	m.mu.Lock()
	if m.s == nil {
		// Estimate resulting tree size: it's likely that the first
		// batch is small, therefore we can safely assume the tree
		// will grow (factor of 2 is quite conservative).
		// 4 here is the branching factor: how many new nodes per
		// a stack on average we expect.
		m.s = NewStacktraceTree(len(stacks) * 4 * 2)
		// We can use function IDs as is for the first batch.
		m.r = newFunctionsRewriter(names)
		for _, s := range stacks {
			m.s.Insert(s.FunctionIds, s.Value)
		}
		m.mu.Unlock()
		return
	}
	m.r.union(names)
	for _, s := range stacks {
		m.r.rewrite(s.FunctionIds)
		m.s.Insert(s.FunctionIds, s.Value)
	}
	m.mu.Unlock()
}

func (m *StacktraceMerger) TreeBytes(maxNodes int64) []byte {
	if m.s == nil || len(m.s.Nodes) == 0 {
		return nil
	}
	// Reuse of the slice (or the whole StacktraceMerger) is possible,
	// but it is unlikely that the performance will be impacted.
	size := len(m.s.Nodes)
	if mn := int(maxNodes); maxNodes > 0 && mn < size {
		size = mn
	}
	buf := make([]byte, 0, size*estimateBytesPerNode)
	b := bytes.NewBuffer(buf)
	m.s.Bytes(b, maxNodes, m.r.names)
	return b.Bytes()
}

func (m *StacktraceMerger) Size() int {
	if m.s != nil {
		return len(m.s.Nodes)
	}
	return 0
}

func newFunctionsRewriter(names []string) *functionsRewriter {
	p := make(map[string]int, 2*len(names))
	for i, v := range names {
		p[v] = i
	}
	return &functionsRewriter{
		positions: p,
		names:     names,
	}
}

type functionsRewriter struct {
	positions map[string]int
	names     []string
	tmp       []int32
}

func (r *functionsRewriter) union(names []string) {
	if cap(r.tmp) > len(names) {
		r.tmp = r.tmp[:len(names)]
	} else {
		r.tmp = make([]int32, len(names))
	}
	for i, name := range names {
		position, found := r.positions[name]
		if !found {
			position = len(r.names)
			r.names = append(r.names, name)
			r.positions[name] = position
		}
		r.tmp[i] = int32(position)
	}
}

func (r *functionsRewriter) rewrite(stack []int32) {
	for i := range stack {
		stack[i] = r.tmp[stack[i]]
	}
}

type StacktraceTree struct{ Nodes []StacktraceNode }

type StacktraceNode struct {
	FirstChild  int32
	NextSibling int32
	Parent      int32
	Location    int32
	Value       int64
	Total       int64
}

func NewStacktraceTree(size int) *StacktraceTree {
	if size < 1 {
		size = 1
	}
	t := StacktraceTree{Nodes: make([]StacktraceNode, 1, size)}
	t.Nodes[0] = StacktraceNode{
		FirstChild:  sentinel,
		NextSibling: sentinel,
	}
	return &t
}

const sentinel = -1

func (t *StacktraceTree) len() uint32 { return uint32(len(t.Nodes)) }

func (t *StacktraceTree) Insert(locations []int32, value int64) {
	var (
		n = &t.Nodes[0]
		i = n.FirstChild
		x int32
	)

	for j := len(locations) - 1; j >= 0; {
		r := locations[j]
		if i == sentinel {
			ni := int32(len(t.Nodes))
			n.FirstChild = ni
			t.Nodes = append(t.Nodes, StacktraceNode{
				Parent:      x,
				FirstChild:  sentinel,
				NextSibling: sentinel,
				Location:    r,
			})
			x = ni
			n = &t.Nodes[ni]
		} else {
			x = i
			n = &t.Nodes[i]
		}
		if n.Location == r {
			n.Total += value
			i = n.FirstChild
			j--
			continue
		}
		if n.NextSibling < 0 {
			n.NextSibling = int32(len(t.Nodes))
			t.Nodes = append(t.Nodes, StacktraceNode{
				Parent:      n.Parent,
				FirstChild:  sentinel,
				NextSibling: sentinel,
				Location:    r,
			})
		}
		i = n.NextSibling
	}

	t.Nodes[x].Value += value
}

// minValue returns the minimum "total" value a node in a tree has to have.
func (t *StacktraceTree) minValue(maxNodes int64) int64 {
	if maxNodes < 1 || maxNodes >= int64(len(t.Nodes)) {
		return 0
	}
	s := make(minHeap, 0, maxNodes)
	h := &s
	for _, n := range t.Nodes {
		if h.Len() >= int(maxNodes) {
			if n.Total > (*h)[0] {
				heap.Pop(h)
			} else {
				continue
			}
		}
		heap.Push(h, n.Total)
	}
	if h.Len() < int(maxNodes) {
		return 0
	}
	return (*h)[0]
}

type StacktraceTreeTraverseFn = func(index int32, children []int32) error

func (t *StacktraceTree) Traverse(maxNodes int64, fn StacktraceTreeTraverseFn) error {
	min := t.minValue(maxNodes)
	children := make([]int32, 0, 128) // Children per node.
	nodesSize := maxNodes             // Depth search buffer.
	if nodesSize < 1 || nodesSize > 10<<10 {
		nodesSize = 1 << 10 // Sane default.
	}
	nodes := make([]int32, 1, nodesSize)
	var current int32
	for len(nodes) > 0 {
		current, nodes, children = nodes[len(nodes)-1], nodes[:len(nodes)-1], children[:0]
		var truncated int64
		n := &t.Nodes[current]
		if n.Location == StacktraceTreeNodeTruncated {
			goto call
		}

		for x := n.FirstChild; x > 0; {
			child := &t.Nodes[x]
			if child.Total >= min && child.Location != StacktraceTreeNodeTruncated {
				children = append(children, x)
			} else {
				truncated += child.Total
			}
			x = child.NextSibling
		}

		if truncated > 0 {
			// Create a stub for removed nodes.
			i := len(t.Nodes)
			t.Nodes = append(t.Nodes, StacktraceNode{
				Location: StacktraceTreeNodeTruncated,
				Value:    truncated,
			})
			children = append(children, int32(i))
		}

		if len(children) > 0 {
			nodes = append(nodes, children...)
		}

	call:
		if err := fn(current, children); err != nil {
			return err
		}
	}

	return nil
}

const StacktraceTreeNodeTruncated = -1

type LocationUnfoldFn func(location int32) []string

func (t *StacktraceTree) Tree(maxNodes int64, unfold LocationUnfoldFn) *Tree {
	// TODO: Allow custom node allocator.
	nodes := make([]*node, len(t.Nodes))

	_ = t.Traverse(maxNodes, func(index int32, children []int32) error {
		n := t.Nodes[index]
		x := nodes[index]
		if x == nil {
			x = new(node)
			if n.Parent > 0 {
				x.parent = nodes[n.Parent]
			}
			nodes[index] = x
		}
		x.total = n.Total
		lines := unfold(n.Location)
		x.name = lines[0]
		for _, line := range lines[1:] {
			m := &node{
				parent: x,
				total:  x.total,
				name:   line,
			}
			x.children = []*node{m}
			x = m
		}
		nodes[index] = x
		x.self = n.Value
		for i := range x.children {
			nodes[children[i]] = &node{
				parent: x,
			}
		}
		return nil
	})

	dst := Tree{root: make([]*node, 0, 64)}
	for _, n := range nodes {
		if n.parent == nil {
			dst.root = append(dst.root, n)
		}
		sort.Slice(n.children, func(i, j int) bool {
			return n.children[i].name < n.children[j].name
		})
	}

	return &dst
}

var lostDuringSerializationNameBytes = []byte(lostDuringSerializationName)

func (t *StacktraceTree) Bytes(dst io.Writer, maxNodes int64, funcs []string) {
	if len(t.Nodes) == 0 || len(funcs) == 0 {
		return
	}
	vw := varint.NewWriter()
	_ = t.Traverse(maxNodes, func(index int32, children []int32) error {
		n := t.Nodes[index]
		var name []byte
		switch n.Location {
		default:
			// It is guaranteed that funcs slice and its contents are immutable,
			// and the byte slice backing capacity is managed by GC.
			name = unsafeStringBytes(funcs[n.Location])
		case StacktraceTreeNodeTruncated:
			name = lostDuringSerializationNameBytes
		}

		_, _ = vw.Write(dst, uint64(len(name)))
		_, _ = dst.Write(name)
		_, _ = vw.Write(dst, uint64(n.Value))
		_, _ = vw.Write(dst, uint64(len(children)))
		return nil
	})
}

func unsafeStringBytes(s string) []byte {
	p := unsafe.Pointer((*reflect.StringHeader)(unsafe.Pointer(&s)).Data)
	var b []byte
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	hdr.Data = uintptr(p)
	hdr.Cap = len(s)
	hdr.Len = len(s)
	return b
}
