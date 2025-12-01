package symdb

import (
	"bytes"
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/pprof"
)

func Test_stacktrace_tree_encoding(t *testing.T) {
	stacks := [][]uint64{
		{5, 4, 3, 2, 1},
		{6, 4, 3, 2, 1},
		{4, 3, 2, 1},
		{3, 2, 1},
		{4, 2, 1},
		{7, 2, 1},
		{2, 1},
		{1},
	}

	x := newStacktraceTree(10)
	var b bytes.Buffer

	for i := range stacks {
		x.insert(stacks[i])

		b.Reset()
		_, err := x.WriteTo(&b)
		require.NoError(t, err)

		ppt := newParentPointerTree(x.len())
		_, err = ppt.ReadFrom(bytes.NewBuffer(b.Bytes()))
		require.NoError(t, err)

		for j := range x.nodes {
			n, p := x.nodes[j], ppt.nodes[j]
			if n.p != p.p || n.r != p.r {
				t.Fatalf("tree mismatch on %v: n:%#v, p:%#v", stacks[i], n, p)
			}
		}
	}
}

func Test_stacktrace_avl_tree_encoding(t *testing.T) {
	stacks := [][]uint64{
		{5, 4, 3, 2, 1},
		{6, 4, 3, 2, 1},
		{4, 3, 2, 1},
		{3, 2, 1},
		{4, 2, 1},
		{7, 2, 1},
		{2, 1},
		{1},
	}

	x := newStacktraceAvlTree(10)
	var b bytes.Buffer

	for i := range stacks {
		x.insert(stacks[i])

		b.Reset()
		_, err := x.WriteTo(&b)
		require.NoError(t, err)

		ppt := newParentPointerTree(x.len())
		_, err = ppt.ReadFrom(bytes.NewBuffer(b.Bytes()))
		require.NoError(t, err)

		for j := range x.nodes {
			n, p := x.nodes[j], ppt.nodes[j]
			if n.p != p.p || n.r != p.r {
				t.Fatalf("tree mismatch on %v: n:%#v, p:%#v", stacks[i], n, p)
			}
		}
	}
}

func Test_stacktrace_hash_tree_encoding(t *testing.T) {
	stacks := [][]uint64{
		{5, 4, 3, 2, 1},
		{6, 4, 3, 2, 1},
		{4, 3, 2, 1},
		{3, 2, 1},
		{4, 2, 1},
		{7, 2, 1},
		{2, 1},
		{1},
	}

	x := newStacktraceHashTree(10)
	var b bytes.Buffer

	for i := range stacks {
		x.insert(stacks[i])

		b.Reset()
		_, err := x.WriteTo(&b)
		require.NoError(t, err)

		ppt := newParentPointerTree(x.len())
		_, err = ppt.ReadFrom(bytes.NewBuffer(b.Bytes()))
		require.NoError(t, err)

		for j := range x.nodes {
			n, p := x.nodes[j], ppt.nodes[j]
			if n.p != p.p || n.r != p.r {
				t.Fatalf("tree mismatch on %v: n:%#v, p:%#v", stacks[i], n, p)
			}
		}
	}
}

func Test_stacktrace_tree_encoding_group(t *testing.T) {
	stacks := [][]uint64{
		{5, 4, 3, 2, 1},
		{6, 4, 3, 2, 1},
		{4, 3, 2, 1},
		{3, 2, 1},
		{4, 2, 1},
		{7, 2, 1},
		{2, 1},
		{1},
	}

	x := newStacktraceTree(10)
	var b bytes.Buffer

	for i := range stacks {
		x.insert(stacks[i])

		b.Reset()
		e := treeEncoder{writeSize: 30}
		err := e.marshal(x, &b)
		require.NoError(t, err)

		ppt := newParentPointerTree(x.len())
		d := treeDecoder{
			bufSize:     64,
			peekSize:    20,
			groupBuffer: 12,
		}
		err = d.unmarshal(ppt, bytes.NewBuffer(b.Bytes()))
		require.NoError(t, err)

		for j := range x.nodes {
			n, p := x.nodes[j], ppt.nodes[j]
			if n.p != p.p || n.r != p.r {
				t.Fatalf("tree mismatch on %v: n:%#v, p:%#v", stacks[i], n, p)
			}
		}
	}
}

func Test_stacktrace_avl_tree_encoding_group(t *testing.T) {
	stacks := [][]uint64{
		{5, 4, 3, 2, 1},
		{6, 4, 3, 2, 1},
		{4, 3, 2, 1},
		{3, 2, 1},
		{4, 2, 1},
		{7, 2, 1},
		{2, 1},
		{1},
	}

	x := newStacktraceAvlTree(10)
	var b bytes.Buffer

	for i := range stacks {
		x.insert(stacks[i])

		b.Reset()
		e := treeEncoder{writeSize: 30}
		err := e.marshalAvl(x, &b)
		require.NoError(t, err)

		ppt := newParentPointerTree(x.len())
		d := treeDecoder{
			bufSize:     64,
			peekSize:    20,
			groupBuffer: 12,
		}
		err = d.unmarshal(ppt, bytes.NewBuffer(b.Bytes()))
		require.NoError(t, err)

		for j := range x.nodes {
			n, p := x.nodes[j], ppt.nodes[j]
			if n.p != p.p || n.r != p.r {
				t.Fatalf("tree mismatch on %v: n:%#v, p:%#v", stacks[i], n, p)
			}
		}
	}
}

func Test_stacktrace_hash_tree_encoding_group(t *testing.T) {
	stacks := [][]uint64{
		{5, 4, 3, 2, 1},
		{6, 4, 3, 2, 1},
		{4, 3, 2, 1},
		{3, 2, 1},
		{4, 2, 1},
		{7, 2, 1},
		{2, 1},
		{1},
	}

	x := newStacktraceHashTree(10)
	var b bytes.Buffer

	for i := range stacks {
		x.insert(stacks[i])

		b.Reset()
		e := treeEncoder{writeSize: 30}
		err := e.marshalHash(x, &b)
		require.NoError(t, err)

		ppt := newParentPointerTree(x.len())
		d := treeDecoder{
			bufSize:     64,
			peekSize:    20,
			groupBuffer: 12,
		}
		err = d.unmarshal(ppt, bytes.NewBuffer(b.Bytes()))
		require.NoError(t, err)

		for j := range x.nodes {
			n, p := x.nodes[j], ppt.nodes[j]
			if n.p != p.p || n.r != p.r {
				t.Fatalf("tree mismatch on %v: n:%#v, p:%#v", stacks[i], n, p)
			}
		}
	}
}

func Test_stacktrace_tree_encoding_rand(t *testing.T) {
	nodes := make([]node, 1<<20)
	for i := range nodes {
		nodes[i] = node{
			fc: 2,
			ns: 3,
			p:  int32(rand.Intn(10 << 10)),
			r:  int32(rand.Intn(10 << 10)),
		}
	}

	x := &stacktraceTreeOld{nodes: nodes}
	var b bytes.Buffer
	_, err := x.WriteTo(&b)
	require.NoError(t, err)

	ppt := newParentPointerTree(x.len())
	_, err = ppt.ReadFrom(bytes.NewBuffer(b.Bytes()))
	require.NoError(t, err)

	for j := range x.nodes {
		n, p := x.nodes[j], ppt.nodes[j]
		if n.p != p.p || n.r != p.r {
			t.Fatalf("tree mismatch at %d: n:%#v. p:%#v", j, n, p)
		}
	}
}

func Test_stacktrace_avl_tree_encoding_rand(t *testing.T) {
	nodes := make([]avlNode, 1<<20)
	for i := range nodes {
		nodes[i] = avlNode{
			cr: 2,
			ls: 3,
			rs: 4,
			p:  int32(rand.Intn(10 << 10)),
			r:  int32(rand.Intn(10 << 10)),
		}
	}

	x := &stacktraceTree{nodes: nodes}
	var b bytes.Buffer
	_, err := x.WriteTo(&b)
	require.NoError(t, err)

	ppt := newParentPointerTree(x.len())
	_, err = ppt.ReadFrom(bytes.NewBuffer(b.Bytes()))
	require.NoError(t, err)

	for j := range x.nodes {
		n, p := x.nodes[j], ppt.nodes[j]
		if n.p != p.p || n.r != p.r {
			t.Fatalf("tree mismatch at %d: n:%#v. p:%#v", j, n, p)
		}
	}
}

func Test_stacktrace_hash_tree_encoding_rand(t *testing.T) {
	nodes := make([]hashNode, 1<<20)
	for i := range nodes {
		nodes[i] = hashNode{
			c: make(map[int32]int),
			p: int32(rand.Intn(10 << 10)),
			r: int32(rand.Intn(10 << 10)),
		}
	}

	x := &stacktraceHashTree{nodes: nodes}
	var b bytes.Buffer
	_, err := x.WriteTo(&b)
	require.NoError(t, err)

	ppt := newParentPointerTree(x.len())
	_, err = ppt.ReadFrom(bytes.NewBuffer(b.Bytes()))
	require.NoError(t, err)

	for j := range x.nodes {
		n, p := x.nodes[j], ppt.nodes[j]
		if n.p != p.p || n.r != p.r {
			t.Fatalf("tree mismatch at %d: n:%#v. p:%#v", j, n, p)
		}
	}
}

func Test_stacktrace_tree_pprof_locations_(t *testing.T) {
	x := newStacktraceTree(0)
	assert.Len(t, x.resolve([]int32{0, 1, 2, 3}, 42), 0)
	assert.Len(t, x.resolveUint64([]uint64{0, 1, 2, 3}, 42), 0)

	p := newParentPointerTree(0)
	assert.Len(t, p.resolve([]int32{0, 1, 2, 3}, 42), 0)
	assert.Len(t, p.resolveUint64([]uint64{0, 1, 2, 3}, 42), 0)
}

func Test_stacktrace_avl_tree_pprof_locations_(t *testing.T) {
	x := newStacktraceAvlTree(0)
	assert.Len(t, x.resolve([]int32{0, 1, 2, 3}, 42), 0)
	assert.Len(t, x.resolveUint64([]uint64{0, 1, 2, 3}, 42), 0)

	p := newParentPointerTree(0)
	assert.Len(t, p.resolve([]int32{0, 1, 2, 3}, 42), 0)
	assert.Len(t, p.resolveUint64([]uint64{0, 1, 2, 3}, 42), 0)
}

func Test_stacktrace_hash_tree_pprof_locations_(t *testing.T) {
	x := newStacktraceHashTree(0)
	assert.Len(t, x.resolve([]int32{0, 1, 2, 3}, 42), 0)
	assert.Len(t, x.resolveUint64([]uint64{0, 1, 2, 3}, 42), 0)

	p := newParentPointerTree(0)
	assert.Len(t, p.resolve([]int32{0, 1, 2, 3}, 42), 0)
	assert.Len(t, p.resolveUint64([]uint64{0, 1, 2, 3}, 42), 0)
}

func Test_stacktrace_tree_pprof_locations(t *testing.T) {
	p, err := pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(t, err)

	x := newStacktraceTree(defaultStacktraceTreeSize)
	m := make(map[uint32]int)
	for i := range p.Sample {
		m[x.insert(p.Sample[i].LocationId)] = i
	}

	tmp := stacktraceLocations.get()
	defer stacktraceLocations.put(tmp)
	for sid, i := range m {
		tmp = x.resolve(tmp, sid)
		locs := p.Sample[i].LocationId
		for j := range locs {
			if tmp[j] != int32(locs[j]) {
				t.Log("resolved:", tmp)
				t.Log("locations:", locs)
				t.Fatalf("ST: tmp[j] != locs[j]")
			}
		}
	}

	var b bytes.Buffer
	n, err := x.WriteTo(&b)
	require.NoError(t, err)
	assert.Equal(t, b.Len(), int(n))

	ppt := newParentPointerTree(x.len())
	n, err = ppt.ReadFrom(bytes.NewReader(b.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, b.Len(), int(n))

	tmp = stacktraceLocations.get()
	defer stacktraceLocations.put(tmp)
	for sid, i := range m {
		tmp = ppt.resolve(tmp, sid)
		locs := p.Sample[i].LocationId
		for j := range locs {
			if tmp[j] != int32(locs[j]) {
				t.Log("resolved:", tmp)
				t.Log("locations:", locs)
				t.Fatalf("PPT: tmp[j] != locs[j]")
			}
		}
	}
}

func Test_stacktrace_avl_tree_pprof_locations(t *testing.T) {
	p, err := pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(t, err)

	x := newStacktraceAvlTree(defaultStacktraceTreeSize)
	m := make(map[uint32]int)
	for i := range p.Sample {
		m[x.insert(p.Sample[i].LocationId)] = i
	}

	tmp := stacktraceLocations.get()
	defer stacktraceLocations.put(tmp)
	for sid, i := range m {
		tmp = x.resolve(tmp, sid)
		locs := p.Sample[i].LocationId
		for j := range locs {
			if tmp[j] != int32(locs[j]) {
				t.Log("resolved:", tmp)
				t.Log("locations:", locs)
				t.Fatalf("ST: tmp[j] != locs[j]")
			}
		}
	}

	var b bytes.Buffer
	n, err := x.WriteTo(&b)
	require.NoError(t, err)
	assert.Equal(t, b.Len(), int(n))

	ppt := newParentPointerTree(x.len())
	n, err = ppt.ReadFrom(bytes.NewReader(b.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, b.Len(), int(n))

	tmp = stacktraceLocations.get()
	defer stacktraceLocations.put(tmp)
	for sid, i := range m {
		tmp = ppt.resolve(tmp, sid)
		locs := p.Sample[i].LocationId
		for j := range locs {
			if tmp[j] != int32(locs[j]) {
				t.Log("resolved:", tmp)
				t.Log("locations:", locs)
				t.Fatalf("PPT: tmp[j] != locs[j]")
			}
		}
	}
}

func Test_stacktrace_hash_tree_pprof_locations(t *testing.T) {
	p, err := pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(t, err)

	x := newStacktraceHashTree(defaultStacktraceTreeSize)
	m := make(map[uint32]int)
	for i := range p.Sample {
		m[x.insert(p.Sample[i].LocationId)] = i
	}

	tmp := stacktraceLocations.get()
	defer stacktraceLocations.put(tmp)
	for sid, i := range m {
		tmp = x.resolve(tmp, sid)
		locs := p.Sample[i].LocationId
		for j := range locs {
			if tmp[j] != int32(locs[j]) {
				t.Log("resolved:", tmp)
				t.Log("locations:", locs)
				t.Fatalf("ST: tmp[j] != locs[j]")
			}
		}
	}

	var b bytes.Buffer
	n, err := x.WriteTo(&b)
	require.NoError(t, err)
	assert.Equal(t, b.Len(), int(n))

	ppt := newParentPointerTree(x.len())
	n, err = ppt.ReadFrom(bytes.NewReader(b.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, b.Len(), int(n))

	tmp = stacktraceLocations.get()
	defer stacktraceLocations.put(tmp)
	for sid, i := range m {
		tmp = ppt.resolve(tmp, sid)
		locs := p.Sample[i].LocationId
		for j := range locs {
			if tmp[j] != int32(locs[j]) {
				t.Log("resolved:", tmp)
				t.Log("locations:", locs)
				t.Fatalf("PPT: tmp[j] != locs[j]")
			}
		}
	}
}

// The test is helpful for debugging.
func Test_parentPointerTree_toStacktraceTree(t *testing.T) {
	x := newStacktraceTree(10)
	for _, stack := range [][]uint64{
		{5, 4, 3, 2, 1},
		{6, 4, 3, 2, 1},
		{4, 3, 2, 1},
		{3, 2, 1},
		{4, 2, 1},
		{7, 2, 1},
		{2, 1},
		{1},
	} {
		x.insert(stack)
	}
	assertRestoredStacktraceTree(t, x)
}

// The test is helpful for debugging.
func Test_compareResultingParentPointerTree(t *testing.T) {
	x := newStacktraceAvlTree(10)
	y := newStacktraceTree(10)
	for _, stack := range [][]uint64{
		{5, 4, 3, 2, 1},
		{6, 4, 3, 2, 1},
		{4, 3, 2, 1},
		{3, 2, 1},
		{4, 2, 1},
		{7, 2, 1},
		{2, 1},
		{1},
	} {
		x.insertIt(stack)
		y.insert(stack)
	}
	assertResultingParentPointerTrees(t, x, y)
}

// The test is helpful for debugging.
func Test_compareResultingParentPointerTree2(t *testing.T) {
	x := newStacktraceHashTree(10)
	y := newStacktraceTree(10)
	for _, stack := range [][]uint64{
		{5, 4, 3, 2, 1},
		{6, 4, 3, 2, 1},
		{4, 3, 2, 1},
		{3, 2, 1},
		{4, 2, 1},
		{7, 2, 1},
		{2, 1},
		{1},
	} {
		x.insert(stack)
		y.insert(stack)
	}
	assertResultingParentPointerTrees2(t, x, y)
}

func Test_parentPointerTree_toStacktraceTree_profile(t *testing.T) {
	p, err := pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(t, err)
	x := newStacktraceTree(defaultStacktraceTreeSize)
	for _, s := range p.Sample {
		x.insert(s.LocationId)
	}
	assertRestoredStacktraceTree(t, x)
}

func Test_compareResultingParentPointerTree_profile(t *testing.T) {
	p, err := pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(t, err)
	x := newStacktraceAvlTree(defaultStacktraceTreeSize)
	y := newStacktraceTree(defaultStacktraceTreeSize)
	for _, s := range p.Sample {
		x.insert(s.LocationId)
		y.insert(s.LocationId)
	}
	assertResultingParentPointerTrees(t, x, y)
}

func Test_compareResultingParentPointerTree_profile3(t *testing.T) {
	p, err := pprof.OpenFile("testdata/big-profile.pb.gz")
	require.NoError(t, err)
	x := newStacktraceAvlTree(defaultStacktraceTreeSize)
	y := newStacktraceAvlTree(defaultStacktraceTreeSize)
	for _, s := range p.Sample {
		x.insertItFull(s.LocationId)
		y.insertIt(s.LocationId)
	}
	for i := range x.nodes {
		assert.Equal(t, x.nodes[i].p, y.nodes[i].p)
	}
	// TODO ref
}

func Test_compareResultingParentPointerTree_profile2(t *testing.T) {
	p, err := pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(t, err)
	x := newStacktraceAvlTree(defaultStacktraceTreeSize)
	y := newStacktraceTree(defaultStacktraceTreeSize)
	for _, s := range p.Sample {
		x.insertIt(s.LocationId)
		y.insert(s.LocationId)
	}
	assertResultingParentPointerTrees(t, x, y)
}

func Test_compareResultingParentPointerTree2_profile(t *testing.T) {
	p, err := pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(t, err)
	x := newStacktraceHashTree(defaultStacktraceTreeSize)
	y := newStacktraceTree(defaultStacktraceTreeSize)
	for _, s := range p.Sample {
		x.insert(s.LocationId)
		y.insert(s.LocationId)
	}
	assertResultingParentPointerTrees2(t, x, y)
}

func assertRestoredStacktraceTree(t *testing.T, x *stacktraceTreeOld) {
	var b bytes.Buffer
	_, _ = x.WriteTo(&b)
	ppt := newParentPointerTree(x.len())
	_, err := ppt.ReadFrom(bytes.NewBuffer(b.Bytes()))
	require.NoError(t, err)
	restored := ppt.toStacktraceTree()
	assert.Equal(t, x.nodes, restored.nodes)
}

func assertResultingParentPointerTrees(t *testing.T, x *stacktraceTree, y *stacktraceTreeOld) {
	var bx bytes.Buffer
	_, _ = x.WriteTo(&bx)
	pptx := newParentPointerTree(x.len())
	_, err := pptx.ReadFrom(bytes.NewBuffer(bx.Bytes()))
	require.NoError(t, err)
	var by bytes.Buffer
	_, _ = y.WriteTo(&by)
	ppty := newParentPointerTree(y.len())
	_, err = ppty.ReadFrom(bytes.NewBuffer(by.Bytes()))
	require.NoError(t, err)

	assert.Equal(t, pptx.nodes, ppty.nodes)
}

func assertResultingParentPointerTrees2(t *testing.T, x *stacktraceHashTree, y *stacktraceTreeOld) {
	var bx bytes.Buffer
	_, _ = x.WriteTo(&bx)
	pptx := newParentPointerTree(x.len())
	_, err := pptx.ReadFrom(bytes.NewBuffer(bx.Bytes()))
	require.NoError(t, err)
	var by bytes.Buffer
	_, _ = y.WriteTo(&by)
	ppty := newParentPointerTree(y.len())
	_, err = ppty.ReadFrom(bytes.NewBuffer(by.Bytes()))
	require.NoError(t, err)

	assert.Equal(t, pptx.nodes, ppty.nodes)
}

func assertResultingParentPointerTrees3(t *testing.T, x *stacktraceTree, y *stacktraceTree) {
	for i := range x.nodes {
		assert.Equal(t, x.nodes[i].p, y.nodes[i].p)
	}
}

func Benchmark_stacktrace_tree_insert(b *testing.B) {
	p, err := pprof.OpenFile("testdata/big-profile.pb.gz")
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		x := newStacktraceTree(defaultStacktraceTreeSize)
		for j := range p.Sample {
			x.insert(p.Sample[j].LocationId)
		}
	}
}

func Benchmark_stacktrace_avl_tree_insert(b *testing.B) {
	p, err := pprof.OpenFile("testdata/big-profile.pb.gz")
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		x := newStacktraceAvlTree(defaultStacktraceTreeSize)
		for j := range p.Sample {
			x.insert(p.Sample[j].LocationId)
		}
	}
}

func Benchmark_stacktrace_avl_tree_insert_it(b *testing.B) {
	p, err := pprof.OpenFile("testdata/big-profile.pb.gz")
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		x := newStacktraceAvlTree(defaultStacktraceTreeSize)
		for j := range p.Sample {
			x.insertIt(p.Sample[j].LocationId)
		}
	}
}

func Benchmark_stacktrace_avl_tree_insert_it_full(b *testing.B) {
	p, err := pprof.OpenFile("testdata/big-profile.pb.gz")
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		x := newStacktraceAvlTree(defaultStacktraceTreeSize)
		for j := range p.Sample {
			x.insertItFull(p.Sample[j].LocationId)
		}
	}
}

func Benchmark_stacktrace_tree_insert_default_sizes(b *testing.B) {
	p, err := pprof.OpenFile("testdata/big-profile.pb.gz")
	require.NoError(b, err)

	b.ResetTimer()

	for _, size := range []int{0, 10, 1024, 2048, 4096, 8192} {
		b.Run("size="+strconv.Itoa(size), func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				x := newStacktraceTree(size)
				for j := range p.Sample {
					x.insert(p.Sample[j].LocationId)
				}

				if testing.Verbose() {
					c := float64(cap(x.nodes))
					b.ReportMetric(c, "cap")
					b.ReportMetric(c*float64(stacktraceTreeNodeSize), "size")
					b.ReportMetric(float64(x.len())/float64(c)*100, "fill")
				}
			}
		})
	}
}

func Benchmark_stacktrace_avl_tree_insert_default_sizes(b *testing.B) {
	p, err := pprof.OpenFile("testdata/big-profile.pb.gz")
	require.NoError(b, err)

	b.ResetTimer()

	for _, size := range []int{0, 10, 1024, 2048, 4096, 8192} {
		b.Run("size="+strconv.Itoa(size), func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				x := newStacktraceAvlTree(size)
				for j := range p.Sample {
					x.insert(p.Sample[j].LocationId)
				}

				if testing.Verbose() {
					c := float64(cap(x.nodes))
					b.ReportMetric(c, "cap")
					b.ReportMetric(c*float64(stacktraceAvlTreeNodeSize), "size")
					b.ReportMetric(float64(x.len())/float64(c)*100, "fill")
				}
			}
		})
	}
}

func Benchmark_stacktrace_avl_tree_insert_it_default_sizes(b *testing.B) {
	p, err := pprof.OpenFile("testdata/big-profile.pb.gz")
	require.NoError(b, err)

	b.ResetTimer()

	for _, size := range []int{0, 10, 1024, 2048, 4096, 8192} {
		b.Run("size="+strconv.Itoa(size), func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				x := newStacktraceAvlTree(size)
				for j := range p.Sample {
					x.insertIt(p.Sample[j].LocationId)
				}

				if testing.Verbose() {
					c := float64(cap(x.nodes))
					b.ReportMetric(c, "cap")
					b.ReportMetric(c*float64(stacktraceAvlTreeNodeSize), "size")
					b.ReportMetric(float64(x.len())/float64(c)*100, "fill")
				}
			}
		})
	}
}
func Benchmark_stacktrace_avl_tree_insert_itFull_default_sizes(b *testing.B) {
	p, err := pprof.OpenFile("testdata/big-profile.pb.gz")
	require.NoError(b, err)

	b.ResetTimer()

	for _, size := range []int{0, 10, 1024, 2048, 4096, 8192} {
		b.Run("size="+strconv.Itoa(size), func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				x := newStacktraceAvlTree(size)
				for j := range p.Sample {
					x.insertItFull(p.Sample[j].LocationId)
				}

				if testing.Verbose() {
					c := float64(cap(x.nodes))
					b.ReportMetric(c, "cap")
					b.ReportMetric(c*float64(stacktraceAvlTreeNodeSize), "size")
					b.ReportMetric(float64(x.len())/float64(c)*100, "fill")
				}
			}
		})
	}
}
