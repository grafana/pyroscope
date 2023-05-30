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
	"github.com/pyroscope-io/pyroscope/pkg/util/varint"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
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
	s  *stacktraceTree
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
		m.s = newStacktraceTree(len(stacks) * 4 * 2)
		// We can use function IDs as is for the first batch.
		m.r = newFunctionsRewriter(names)
		for _, s := range stacks {
			m.s.insert(s.FunctionIds, s.Value)
		}
		m.mu.Unlock()
		return
	}
	m.r.union(names)
	for _, s := range stacks {
		m.r.rewrite(s.FunctionIds)
		m.s.insert(s.FunctionIds, s.Value)
	}
	m.mu.Unlock()
}

func (m *StacktraceMerger) TreeBytes(maxNodes int64) []byte {
	if m.s == nil || len(m.s.nodes) == 0 {
		return nil
	}
	// Reuse of the slice (or the whole StacktraceMerger) is possible,
	// but it is unlikely that the performance will be impacted.
	size := len(m.s.nodes)
	if mn := int(maxNodes); maxNodes > 0 && mn < size {
		size = mn
	}
	buf := make([]byte, 0, size*estimateBytesPerNode)
	b := bytes.NewBuffer(buf)
	m.s.bytes(b, maxNodes, m.r.names)
	return b.Bytes()
}

func (m *StacktraceMerger) Size() int {
	if m.s != nil {
		return len(m.s.nodes)
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

// stacktraceTree represents a profile built from a collection of StacktraceSamples.
type stacktraceTree struct{ nodes []stacktraceNode }

type stacktraceNode struct {
	i     int32
	fc    int32
	ns    int32
	fid   int32
	val   int64
	total int64
}

func newStacktraceTree(size int) *stacktraceTree {
	t := stacktraceTree{nodes: make([]stacktraceNode, 0, size)}
	t.newNode(0)
	return &t
}

func (t *stacktraceTree) newNode(fid int32) *stacktraceNode {
	n := stacktraceNode{
		fid: fid,
		i:   int32(len(t.nodes)),
		fc:  -1,
		ns:  -1,
	}
	t.nodes = append(t.nodes, n)
	return &t.nodes[n.i]
}

// stack of function IDs, the root it the last element.
func (t *stacktraceTree) insert(stack []int32, v int64) {
	var (
		i int32
		n = new(stacktraceNode)
	)
	// Iterate over the stack in reverse order.
	// Note that j is not decremented automatically.
	for j := len(stack) - 1; j >= 0; {
		r := stack[j]
		if i < 0 {
			// Next node is not found.
			x := t.newNode(r)
			n.fc = x.i
			t.nodes[n.i] = *n
			n = x
		} else {
			n = &t.nodes[i]
		}
		if n.fid == r && n.i != 0 {
			// There already is a node with this function ID.
			// Update it and go to the next level.
			n.total += v
			t.nodes[n.i] = *n
			i = n.fc
			j--
			continue
		}
		if n.i == 0 {
			i = n.fc
			continue
		}
		// No more siblings, insert one.
		if n.ns < 0 {
			x := t.newNode(r)
			n.ns = x.i
			t.nodes[n.i] = *n
		}
		// Go to the next sibling, without decrementing j,
		// so that the same function ID is evaluated.
		i = n.ns
	}
	// Reached end of the stack.
	n = &t.nodes[n.i]
	n.val += v
	t.nodes[n.i] = *n
}

// minValue returns the minimum "total" value a node in a tree has to have.
func (t *stacktraceTree) minValue(maxNodes int64) int64 {
	if maxNodes < 1 || maxNodes >= int64(len(t.nodes)) {
		return 0
	}
	s := make(minHeap, 0, maxNodes)
	h := &s
	for _, n := range t.nodes {
		if h.Len() >= int(maxNodes) {
			if n.total > (*h)[0] {
				heap.Pop(h)
			} else {
				continue
			}
		}
		heap.Push(h, n.total)
	}
	if h.Len() < int(maxNodes) {
		return 0
	}
	return (*h)[0]
}

const lostDuringSerializationNameReference = -1

var lostDuringSerializationNameBytes = []byte(lostDuringSerializationName)

func (t *stacktraceTree) bytes(dst io.Writer, maxNodes int64, funcs []string) {
	if len(t.nodes) == 0 || len(funcs) == 0 {
		return
	}
	min := t.minValue(maxNodes)
	vw := varint.NewWriter()
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
		n := &t.nodes[current]
		if n.fid == lostDuringSerializationNameReference {
			goto write
		}

		for x := n.fc; x > 0; {
			child := &t.nodes[x]
			if child.total >= min && child.fid != lostDuringSerializationNameReference {
				children = append(children, x)
			} else {
				truncated += child.total
			}
			x = child.ns
		}

		if truncated > 0 {
			// Create a stub for removed nodes.
			s := t.newNode(lostDuringSerializationNameReference)
			s.val = truncated
			t.nodes[s.i] = *s
			children = append(children, s.i)
		}

		if len(children) > 0 {
			nodes = append(nodes, children...)
		}

	write:
		var name []byte
		switch n.fid {
		default:
			// It is guaranteed that funcs slice and its contents are immutable,
			// and the byte slice backing capacity is managed by GC.
			name = unsafeStringBytes(funcs[n.fid])
		case lostDuringSerializationNameReference:
			name = lostDuringSerializationNameBytes
		}

		_, _ = vw.Write(dst, uint64(len(name)))
		_, _ = dst.Write(name)
		_, _ = vw.Write(dst, uint64(n.val))
		_, _ = vw.Write(dst, uint64(len(children)))
	}
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
