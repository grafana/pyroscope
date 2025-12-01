package symdb

import (
	"bufio"
	"io"
	"unsafe"

	"github.com/dgryski/go-groupvarint"
)

const (
	defaultStacktraceTreeSize = 0
	stacktraceTreeNodeSize    = int(unsafe.Sizeof(node{}))
	stacktraceAvlTreeNodeSize = int(unsafe.Sizeof(avlNode{}))
)

type stacktraceTreeOld struct {
	nodes []node
}

type node struct {
	p int32 // Parent index.
	r int32 // Reference the to stack frame data.
	// Auxiliary members only needed for insertion.
	fc int32 // First child index.
	ns int32 // Next sibling index.
}

func newStacktraceTree(size int) *stacktraceTreeOld {
	if size < 1 {
		size = 1
	}
	t := stacktraceTreeOld{nodes: make([]node, 1, size)}
	t.nodes[0] = node{
		p:  sentinel,
		fc: sentinel,
		ns: sentinel,
	}
	return &t
}

const sentinel = -1

func (t *stacktraceTreeOld) len() uint32 { return uint32(len(t.nodes)) }

func (t *stacktraceTreeOld) insert(refs []uint64) uint32 {
	var (
		n = &t.nodes[0]
		i = n.fc
		x int32
	)

	for j := len(refs) - 1; j >= 0; {
		r := int32(refs[j])
		if i == sentinel {
			ni := int32(len(t.nodes))
			n.fc = ni
			t.nodes = append(t.nodes, node{
				r:  r,
				p:  x,
				fc: sentinel,
				ns: sentinel,
			})
			x = ni
			n = &t.nodes[ni]
		} else {
			x = i
			n = &t.nodes[i]
		}
		if n.r == r {
			i = n.fc
			j--
			continue
		}
		if n.ns < 0 {
			n.ns = int32(len(t.nodes))
			t.nodes = append(t.nodes, node{
				r:  r,
				p:  n.p,
				fc: sentinel,
				ns: sentinel,
			})
		}
		i = n.ns
	}

	return uint32(x)
}

func (t *stacktraceTreeOld) resolve(dst []int32, id uint32) []int32 {
	dst = dst[:0]
	if id >= uint32(len(t.nodes)) {
		return dst
	}
	// Only node members are accessed, in order to avoid
	// race condition with insert: r and p are written once,
	// when the node is created.
	for i := int32(id); i > 0; i = t.nodes[i].p {
		dst = append(dst, t.nodes[i].r)
	}
	return dst
}

func (t *stacktraceTreeOld) resolveUint64(dst []uint64, id uint32) []uint64 {
	dst = dst[:0]
	if id >= uint32(len(t.nodes)) {
		return dst
	}
	// Only node members are accessed, in order to avoid
	// race condition with insert: r and p are written once,
	// when the node is created.
	for i := int32(id); i > 0; i = t.nodes[i].p {
		dst = append(dst, uint64(t.nodes[i].r))
	}
	return dst
}

func (t *stacktraceTreeOld) Nodes() []Node {
	dst := make([]Node, len(t.nodes))
	for i := 0; i < len(dst) && i < len(t.nodes); i++ { // BCE
		dst[i] = Node{Parent: t.nodes[i].p, Location: t.nodes[i].r}
	}
	return dst
}

const (
	maxGroupSize = 17 // 4 * uint32 + control byte
	// minGroupSize = 5  // 4 * byte + control byte
)

func (t *stacktraceTreeOld) WriteTo(dst io.Writer) (int64, error) {
	e := treeEncoder{
		writeSize: 4 << 10,
	}
	err := e.marshal(t, dst)
	return e.written, err
}

type stacktraceTree struct {
	nodes []avlNode
}

type avlNode struct {
	p int32 // parent index.
	r int32 // Reference the to stack frame data.

	// Auxiliary members only needed for insertion.
	cr int32 // children root (where to find next avl tree).
	ls int32 // left sibling.
	rs int32 // right sibling.
	h  int32 // height.
}

func newStacktraceAvlTree(size int) *stacktraceTree {
	if size < 1 {
		size = 1
	}
	t := stacktraceTree{nodes: make([]avlNode, 1, size)}
	t.nodes[0] = avlNode{
		p:  sentinel,
		cr: sentinel,
		ls: sentinel,
		rs: sentinel,
	}
	return &t
}

func (t *stacktraceTree) len() uint32 { return uint32(len(t.nodes)) }

/*
	func (t *stacktraceTreeOld) insert2(refs []uint64) uint32 {
		var (
			n = &t.nodes[0]
			i = n.fc
			x int32
		)

		for j := len(refs) - 1; j >= 0; {
			r := int32(refs[j])
			if i == sentinel {
				ni := int32(len(t.nodes))
				n.fc = ni
				t.nodes = append(t.nodes, node{
					r:  r,
					p:  x,
					fc: sentinel,
					ns: sentinel,
				})
				x = ni
				n = &t.nodes[ni]
			} else {
				x = i
				n = &t.nodes[i]
			}
			if n.r == r {
				i = n.fc
				j--
				continue
			}
			if n.ns < 0 {
				n.ns = int32(len(t.nodes))
				t.nodes = append(t.nodes, node{
					r:  r,
					p:  n.p,
					fc: sentinel,
					ns: sentinel,
				})
			}
			i = n.ns
		}

		return uint32(x)
	}
*/
/*
func (t *stacktraceTree) insertIterative(refs []uint64) uint32 {
	curr := &t.nodes[0]
	var next_parent int32
	for j := len(refs) - 1; j >= 0; j-- {
		// find j-th ref in current node's children:
		r := int32(refs[j])
		childPointer2 := &curr.cr
		for *childPointer2 != sentinel && r != t.nodes[*childPointer2].r {
			curr = &t.nodes[*childPointer2]
			if r < t.nodes[*childPointer2].r {
				childPointer2 = &curr.ls
			} else {
				childPointer2 = &curr.rs
			}
		}
		if *childPointer2 == sentinel {
			// Node not found, let's add it
			newNodeIndex := int32(len(t.nodes))
			*childPointer2 = newNodeIndex
			t.nodes = append(t.nodes, avlNode{
				r:  r,
				p:  next_parent,
				cr: sentinel,
				ls: sentinel,
				rs: sentinel,
				// TODO UPDATE TOPS!!!
			})
			curr = &t.nodes[newNodeIndex]
			next_parent = newNodeIndex
		} else {
			// No insert, updating new current
			curr = &t.nodes[*childPointer2]
			next_parent = *childPointer2
		}
	}
	return uint32(next_parent)
}
*/

func (t *stacktraceTree) insertRecursive(refs []uint64) uint32 {
	var root = int32(0)
	for j := len(refs) - 1; j >= 0; j-- {
		r := int32(refs[j])
		inserted, newRoot := t.insertToNode(t.nodes[root].cr, root, r)
		t.nodes[root].cr = newRoot
		root = inserted
	}
	return uint32(root)
}

func (t *stacktraceTree) insertToNode(i, parent, r int32) (int32, int32) {
	var inserted int32
	if i == sentinel {
		inserted = int32(len(t.nodes))
		t.nodes = append(t.nodes, avlNode{
			r:  r,
			p:  parent,
			cr: sentinel,
			ls: sentinel,
			rs: sentinel,
		})
		return inserted, inserted
	}
	if r < t.nodes[i].r {
		inserted, t.nodes[i].ls = t.insertToNode(t.nodes[i].ls, parent, r)
	} else if r > t.nodes[i].r {
		inserted, t.nodes[i].rs = t.insertToNode(t.nodes[i].rs, parent, r)
	} else {
		inserted = i
	}
	t.updateHeight(i)
	return inserted, t.rebalance(i)
}

func (t *stacktraceTree) balanceFactor(i int32) int32 {
	left, right := int32(-1), int32(-1)
	if t.nodes[i].ls != sentinel {
		left = t.nodes[t.nodes[i].ls].h
	}
	if t.nodes[i].rs != sentinel {
		right = t.nodes[t.nodes[i].rs].h
	}
	return left - right
}

func (t *stacktraceTree) rebalance(i int32) int32 {
	bf := t.balanceFactor(i)
	if bf > 1 {
		// LR
		if t.nodes[i].ls != sentinel && t.balanceFactor(t.nodes[i].ls) < 0 {
			t.nodes[i].ls = t.leftRotation(t.nodes[i].ls)
		}
		// LL
		i = t.rightRotation(i)
	} else if bf < -1 {
		// RL
		if t.nodes[i].rs != sentinel && t.balanceFactor(t.nodes[i].rs) > 0 {
			t.nodes[i].rs = t.rightRotation(t.nodes[i].rs)
		}
		// RR
		i = t.leftRotation(i)
	}
	return i
}

func (t *stacktraceTree) updateHeight(i int32) {
	left, right := int32(-1), int32(-1)
	if t.nodes[i].ls != sentinel {
		left = t.nodes[t.nodes[i].ls].h
	}
	if t.nodes[i].rs != sentinel {
		right = t.nodes[t.nodes[i].rs].h
	}
	t.nodes[i].h = 1 + max(left, right)
}

func (t *stacktraceTree) rightRotation(i int32) int32 {
	x := t.nodes[i].ls
	t.nodes[i].ls = t.nodes[x].rs
	t.nodes[x].rs = i
	t.updateHeight(i)
	t.updateHeight(x)
	return x
}

func (t *stacktraceTree) leftRotation(i int32) int32 {
	x := t.nodes[i].rs
	t.nodes[i].rs = t.nodes[x].ls
	t.nodes[x].ls = i
	t.updateHeight(i)
	t.updateHeight(x)
	return x
}

func (t *stacktraceTree) resolve(dst []int32, id uint32) []int32 {
	dst = dst[:0]
	if id >= uint32(len(t.nodes)) {
		return dst
	}
	// Only node members are accessed, in order to avoid
	// race condition with insert: r and p are written once,
	// when the node is created.
	for i := int32(id); i > 0; i = t.nodes[i].p {
		dst = append(dst, t.nodes[i].r)
	}
	return dst
}

func (t *stacktraceTree) resolveUint64(dst []uint64, id uint32) []uint64 {
	dst = dst[:0]
	if id >= uint32(len(t.nodes)) {
		return dst
	}
	// Only node members are accessed, in order to avoid
	// race condition with insert: r and p are written once,
	// when the node is created.
	for i := int32(id); i > 0; i = t.nodes[i].p {
		dst = append(dst, uint64(t.nodes[i].r))
	}
	return dst
}

func (t *stacktraceTree) Nodes() []Node {
	dst := make([]Node, len(t.nodes))
	for i := 0; i < len(dst) && i < len(t.nodes); i++ { // BCE
		dst[i] = Node{Parent: t.nodes[i].p, Location: t.nodes[i].r}
	}
	return dst
}

func (t *stacktraceTree) WriteTo(dst io.Writer) (int64, error) {
	e := treeEncoder{
		writeSize: 4 << 10,
	}
	err := e.marshalAvl(t, dst)
	return e.written, err
}

type stacktraceHashTree struct {
	nodes []hashNode
}

type hashNode struct {
	p int32 // parent index.
	r int32 // Reference the to stack frame data.

	c map[int32]int
}

func newStacktraceHashTree(size int) *stacktraceHashTree {
	if size < 1 {
		size = 1
	}
	t := stacktraceHashTree{nodes: make([]hashNode, 1, size)}
	t.nodes[0] = hashNode{
		p: sentinel,
		c: make(map[int32]int),
	}
	return &t
}

func (t *stacktraceHashTree) len() uint32 { return uint32(len(t.nodes)) }

func (t *stacktraceHashTree) insert(refs []uint64) uint32 {
	var n = int32(0)
	for j := len(refs) - 1; j >= 0; j-- {
		r := int32(refs[j])
		next, ok := t.nodes[n].c[r]
		if !ok {
			next = len(t.nodes)
			t.nodes = append(t.nodes, hashNode{
				p: n,
				r: r,
				c: make(map[int32]int),
			})
			t.nodes[n].c[r] = next
		}
		n = int32(next)
	}
	return uint32(n)
}

func (t *stacktraceHashTree) resolve(dst []int32, id uint32) []int32 {
	dst = dst[:0]
	if id >= uint32(len(t.nodes)) {
		return dst
	}
	// Only node members are accessed, in order to avoid
	// race condition with insert: r and p are written once,
	// when the node is created.
	for i := int32(id); i > 0; i = t.nodes[i].p {
		dst = append(dst, t.nodes[i].r)
	}
	return dst
}

func (t *stacktraceHashTree) resolveUint64(dst []uint64, id uint32) []uint64 {
	dst = dst[:0]
	if id >= uint32(len(t.nodes)) {
		return dst
	}
	// Only node members are accessed, in order to avoid
	// race condition with insert: r and p are written once,
	// when the node is created.
	for i := int32(id); i > 0; i = t.nodes[i].p {
		dst = append(dst, uint64(t.nodes[i].r))
	}
	return dst
}

func (t *stacktraceHashTree) Nodes() []Node {
	dst := make([]Node, len(t.nodes))
	for i := 0; i < len(dst) && i < len(t.nodes); i++ { // BCE
		dst[i] = Node{Parent: t.nodes[i].p, Location: t.nodes[i].r}
	}
	return dst
}

func (t *stacktraceHashTree) WriteTo(dst io.Writer) (int64, error) {
	e := treeEncoder{
		writeSize: 4 << 10,
	}
	err := e.marshalHash(t, dst)
	return e.written, err
}

type parentPointerTree struct {
	nodes []pptNode
}

type pptNode struct {
	p int32 // Parent index.
	r int32 // Reference the to stack frame data.
}

func newParentPointerTree(size uint32) *parentPointerTree {
	return &parentPointerTree{
		nodes: make([]pptNode, size),
	}
}

func (t *parentPointerTree) resolve(dst []int32, id uint32) []int32 {
	dst = dst[:0]
	if id >= uint32(len(t.nodes)) {
		return dst
	}
	n := t.nodes[id]
	for n.p >= 0 {
		dst = append(dst, n.r)
		n = t.nodes[n.p]
	}
	return dst
}

func (t *parentPointerTree) resolveUint64(dst []uint64, id uint32) []uint64 {
	dst = dst[:0]
	if id >= uint32(len(t.nodes)) {
		return dst
	}
	n := t.nodes[id]
	for n.p >= 0 {
		dst = append(dst, uint64(n.r))
		n = t.nodes[n.p]
	}
	return dst
}

func (t *parentPointerTree) Nodes() []Node {
	dst := make([]Node, len(t.nodes))
	for i := 0; i < len(dst) && i < len(t.nodes); i++ { // BCE
		dst[i] = Node{Parent: t.nodes[i].p, Location: t.nodes[i].r}
	}
	return dst
}

func (t *parentPointerTree) toStacktraceTree() *stacktraceTreeOld {
	l := int32(len(t.nodes))
	x := stacktraceTreeOld{nodes: make([]node, l)}
	x.nodes[0] = node{
		p:  sentinel,
		fc: sentinel,
		ns: sentinel,
	}
	lc := make([]int32, len(t.nodes))
	var s int32
	for i := int32(1); i < l; i++ {
		n := t.nodes[i]
		x.nodes[i] = node{
			p:  n.p,
			r:  n.r,
			fc: sentinel,
			ns: sentinel,
		}
		// Swap the last child of the parent with self.
		// If this is the first child, update the parent.
		// Otherwise, update the sibling.
		s, lc[n.p] = lc[n.p], i
		if s == 0 {
			x.nodes[n.p].fc = i
		} else {
			x.nodes[s].ns = i
		}
	}
	return &x
}

// ReadFrom decodes parent pointer tree from the reader.
// The tree must have enough nodes.
func (t *parentPointerTree) ReadFrom(r io.Reader) (int64, error) {
	d := treeDecoder{
		bufSize:     4 << 10,
		peekSize:    4 << 10,
		groupBuffer: 1 << 10,
	}
	err := d.unmarshal(t, r)
	return d.read, err
}

type treeEncoder struct {
	writeSize int
	written   int64
}

func (tc *treeEncoder) marshal(t *stacktraceTreeOld, w io.Writer) (err error) {
	// Writes go through a staging buffer.
	// Make sure it is allocated on stack.
	ws := tc.writeSize
	b := make([]byte, ws)
	g := make([]uint32, 4)
	var n, s int
	// For delta zig-zag.
	var p, c node
	var v int32

	for i := 0; i < len(t.nodes); i += 2 {
		// First node of the pair.
		c = t.nodes[i]
		v = c.p - p.p
		g[0] = uint32((v << 1) ^ (v >> 31))
		g[1] = uint32(c.r)
		p = c
		if sn := i + 1; sn < len(t.nodes) {
			// Second node.
			c = t.nodes[sn]
			v = c.p - p.p
			g[2] = uint32((v << 1) ^ (v >> 31))
			g[3] = uint32(c.r)
			p = c
		} else {
			// A stub node is added to complete the group.
			g[2] = 0
			g[3] = 0
		}
		groupvarint.Encode4(b[n:], g)
		n += groupvarint.BytesUsed[b[n]]
		if n+maxGroupSize > ws || i >= len(t.nodes)-2 {
			s, err = w.Write(b[:n])
			if err != nil {
				return err
			}
			tc.written += int64(s)
			n = 0
		}
	}
	return nil
}

func (tc *treeEncoder) marshalAvl(t *stacktraceTree, w io.Writer) (err error) {
	// Writes go through a staging buffer.
	// Make sure it is allocated on stack.
	ws := tc.writeSize
	b := make([]byte, ws)
	g := make([]uint32, 4)
	var n, s int
	// For delta zig-zag.
	var p, c avlNode
	var v int32

	for i := 0; i < len(t.nodes); i += 2 {
		// First node of the pair.
		c = t.nodes[i]
		v = c.p - p.p
		g[0] = uint32((v << 1) ^ (v >> 31))
		g[1] = uint32(c.r)
		p = c
		if sn := i + 1; sn < len(t.nodes) {
			// Second node.
			c = t.nodes[sn]
			v = c.p - p.p
			g[2] = uint32((v << 1) ^ (v >> 31))
			g[3] = uint32(c.r)
			p = c
		} else {
			// A stub node is added to complete the group.
			g[2] = 0
			g[3] = 0
		}
		groupvarint.Encode4(b[n:], g)
		n += groupvarint.BytesUsed[b[n]]
		if n+maxGroupSize > ws || i >= len(t.nodes)-2 {
			s, err = w.Write(b[:n])
			if err != nil {
				return err
			}
			tc.written += int64(s)
			n = 0
		}
	}
	return nil
}

func (tc *treeEncoder) marshalHash(t *stacktraceHashTree, w io.Writer) (err error) {
	// Writes go through a staging buffer.
	// Make sure it is allocated on stack.
	ws := tc.writeSize
	b := make([]byte, ws)
	g := make([]uint32, 4)
	var n, s int
	// For delta zig-zag.
	var p, c hashNode
	var v int32

	for i := 0; i < len(t.nodes); i += 2 {
		// First node of the pair.
		c = t.nodes[i]
		v = c.p - p.p
		g[0] = uint32((v << 1) ^ (v >> 31))
		g[1] = uint32(c.r)
		p = c
		if sn := i + 1; sn < len(t.nodes) {
			// Second node.
			c = t.nodes[sn]
			v = c.p - p.p
			g[2] = uint32((v << 1) ^ (v >> 31))
			g[3] = uint32(c.r)
			p = c
		} else {
			// A stub node is added to complete the group.
			g[2] = 0
			g[3] = 0
		}
		groupvarint.Encode4(b[n:], g)
		n += groupvarint.BytesUsed[b[n]]
		if n+maxGroupSize > ws || i >= len(t.nodes)-2 {
			s, err = w.Write(b[:n])
			if err != nil {
				return err
			}
			tc.written += int64(s)
			n = 0
		}
	}
	return nil
}

type treeDecoder struct {
	bufSize     int
	peekSize    int
	groupBuffer int // %4 == 0
	read        int64
}

func (d *treeDecoder) unmarshal(t *parentPointerTree, r io.Reader) error {
	buf, ok := r.(*bufio.Reader)
	if !ok || buf.Size() < d.peekSize {
		buf = bufio.NewReaderSize(r, d.bufSize)
	}

	g := make([]uint32, d.groupBuffer)
	rb := make([]byte, 0, maxGroupSize)
	var p, c pptNode // Previous and current nodes.
	var np int
	var eof bool

	for !eof {
		// Load the next peekSize bytes.
		// Must not exceed Reader's buffer size.
		b, err := buf.Peek(d.peekSize)
		if err != nil {
			if err != io.EOF {
				return err
			}
			eof = true
		}
		// len(b) is always >= b.Buffered(),
		// therefore Discard does not invalidate
		// the buffer.
		if _, err = buf.Discard(len(b)); err != nil {
			return err
		}
		d.read += int64(len(b))

		// Read b into g and decode.
		for read := 0; read < len(b); {
			// We need to read remaining_nodes * 2 uints or the whole
			// group buffer, whichever is smaller.
			xn := len(t.nodes) - np // remaining nodes
			// Note that g should always be a multiple of 4.
			g = g[:min((xn+xn%2)*2, d.groupBuffer)]
			if len(g)%4 != 0 {
				return io.ErrUnexpectedEOF
			}
			// Check if there is a remainder. If this is the case,
			// decode the group and advance gp.
			var gp int
			if len(rb) > 0 {
				// It's expected that rb contains a single complete group.
				m := groupvarint.BytesUsed[rb[0]] - len(rb)
				if m >= (len(b) + len(rb)) {
					return io.ErrUnexpectedEOF
				}
				rb = append(rb, b[:m]...)
				i, n, rn := decodeU32Groups(g[:4], rb)
				if i != 4 || n != len(rb) || rn > 0 {
					return io.ErrUnexpectedEOF
				}
				read += m // Part is read from rb.
				rb = rb[:0]
				gp += 4
			}

			// Re-fill g.
			gi, n, rn := decodeU32Groups(g[gp:], b[read:])
			gp += gi
			read += n + rn // Mark the remaining bytes as read; we copy them.
			if rn > 0 {
				// If there is a remainder, it is copied and decoded on
				// the next Peek. This should not be possible with eof.
				rb = append(rb, b[len(b)-rn:]...)
			}
			if len(g) == 0 && len(rb) == 0 {
				break
			}

			// g is full, or no more data in buf.
			for i := 0; i < len(g[:gp])-1; i += 2 {
				if np >= len(t.nodes) {
					// g may contain an empty node at the end.
					return nil
				}
				v := int32(g[i])
				c.p = (v>>1 ^ ((v << 31) >> 31)) + p.p
				c.r = int32(g[i+1])
				t.nodes[np] = c
				np++
				p = c
			}
		}
	}

	return nil
}

// decodeU32Groups decodes len(dst)/4 groups from src and
// returns: dst offset, bytes read, bytes remaining in src.
func decodeU32Groups(dst []uint32, src []byte) (i, j, rm int) {
	var n int
	for i < len(dst) && j < len(src) {
		n = groupvarint.BytesUsed[src[j]]
		if rm = len(src[j:]); rm < n {
			return i, j, rm
		}
		groupvarint.Decode4(dst[i:], src[j:])
		i += 4
		j += n
	}
	return i, j, 0
}

/*

func (t *stacktraceTree) insert(refs []uint64) uint32 {
	var root = int32(0)
	for j := len(refs) - 1; j >= 0; j-- {
		r := int32(refs[j])
		inserted, newRoot := t.insertToNode(t.nodes[root].cr, root, r)
		t.nodes[root].cr = newRoot
		root = inserted
	}
	return uint32(root)
}
*/

/*

func (t *stacktraceTree) insertToNode(i, parent, r int32) (int32, int32) {
	var inserted int32
	if i == sentinel {
		inserted = int32(len(t.nodes))
		t.nodes = append(t.nodes, avlNode{
			r:  r,
			p:  parent,
			cr: sentinel,
			ls: sentinel,
			rs: sentinel,
		})
		return inserted, inserted
	}
	if r < t.nodes[i].r {
		inserted, t.nodes[i].ls = t.insertToNode(t.nodes[i].ls, parent, r)
	} else if r > t.nodes[i].r {
		inserted, t.nodes[i].rs = t.insertToNode(t.nodes[i].rs, parent, r)
	} else {
		inserted = i
	}
	t.updateHeight(i)
	return inserted, t.rebalance(i)
}
*/

func (t *stacktraceTree) insertIt(refs []uint64) uint32 {
	var root = int32(0)
	path := make([]int32, 0, 32)
	for j := len(refs) - 1; j >= 0; j-- {
		path = path[:0]
		r := int32(refs[j])
		p := int32(0)

		// calcular inserted i new root:
		i := t.nodes[root].cr
		for i != sentinel && t.nodes[i].r != r {
			path = append(path, i)
			p++
			if r < t.nodes[i].r {
				i = t.nodes[i].ls
			} else {
				i = t.nodes[i].rs
			}
		}
		if i != sentinel {
			// No new node, no need to complicate things, let's skip to the next one
			root = i
			continue
		}

		// new node, se vienen cositas
		inserted := int32(len(t.nodes))
		t.nodes = append(t.nodes, avlNode{
			r:  r,
			p:  root,
			cr: sentinel,
			ls: sentinel,
			rs: sentinel,
		})
		newRoot := inserted
		for k := p - 1; k >= 0; k-- {
			p = path[k]
			if r < t.nodes[p].r {
				t.nodes[p].ls = newRoot
			} else {
				t.nodes[p].rs = newRoot
			}
			t.updateHeight(p)
			newRoot = t.rebalance(p)
		}
		// assignar t.nodes[root].cr = newRoot i root = inserted
		t.nodes[root].cr = newRoot
		root = inserted
	}

	return uint32(root)
}

func (t *stacktraceTree) insert(refs []uint64) uint32 {
	var root = int32(0)
	path := make([]int32, 0, 32)
	for j := len(refs) - 1; j >= 0; j-- {
		path = path[:0]
		r := int32(refs[j])
		p := int32(0)

		// calcular inserted i new root:
		i := t.nodes[root].cr
		for i != sentinel && t.nodes[i].r != r {
			path = append(path, i)
			p++
			if r < t.nodes[i].r {
				i = t.nodes[i].ls
			} else {
				i = t.nodes[i].rs
			}
		}
		if i != sentinel {
			// No new node, no need to complicate things, let's skip to the next one
			root = i
			continue
		}

		// new node, se vienen cositas
		inserted := int32(len(t.nodes))
		t.nodes = append(t.nodes, avlNode{
			r:  r,
			p:  root,
			cr: sentinel,
			ls: sentinel,
			rs: sentinel,
		})
		newRoot := inserted
		for k := p - 1; k >= 0; k-- {
			p = path[k]
			if r < t.nodes[p].r {
				t.nodes[p].ls = newRoot
			} else {
				t.nodes[p].rs = newRoot
			}
			left, right := int32(-1), int32(-1)
			if t.nodes[p].ls != sentinel {
				left = t.nodes[t.nodes[p].ls].h
			}
			if t.nodes[p].rs != sentinel {
				right = t.nodes[t.nodes[p].rs].h
			}
			t.nodes[p].h = 1 + max(left, right)

			bf := left - right
			if bf > 1 {
				// LR
				if t.nodes[p].ls != sentinel {
					left, right = int32(-1), int32(-1)
					if t.nodes[t.nodes[p].ls].ls != sentinel {
						left = t.nodes[t.nodes[t.nodes[p].ls].ls].h
					}
					if t.nodes[t.nodes[p].ls].rs != sentinel {
						right = t.nodes[t.nodes[t.nodes[p].ls].rs].h
					}
					if left-right < 0 {
						x := t.nodes[t.nodes[p].ls].rs
						t.nodes[t.nodes[p].ls].rs = t.nodes[x].ls
						t.nodes[x].ls = t.nodes[p].ls

						left, right = int32(-1), int32(-1)
						if t.nodes[t.nodes[p].ls].ls != sentinel {
							left = t.nodes[t.nodes[t.nodes[p].ls].ls].h
						}
						if t.nodes[t.nodes[p].ls].rs != sentinel {
							right = t.nodes[t.nodes[t.nodes[p].ls].rs].h
						}
						t.nodes[t.nodes[p].ls].h = 1 + max(left, right)

						left, right = int32(-1), int32(-1)
						if t.nodes[x].ls != sentinel {
							left = t.nodes[t.nodes[x].ls].h
						}
						if t.nodes[x].rs != sentinel {
							right = t.nodes[t.nodes[x].rs].h
						}
						t.nodes[x].h = 1 + max(left, right)
						t.nodes[p].ls = x
					}
				}
				// LL
				x := t.nodes[p].ls
				t.nodes[p].ls = t.nodes[x].rs
				t.nodes[x].rs = p

				left, right = int32(-1), int32(-1)
				if t.nodes[p].ls != sentinel {
					left = t.nodes[t.nodes[p].ls].h
				}
				if t.nodes[p].rs != sentinel {
					right = t.nodes[t.nodes[p].rs].h
				}
				t.nodes[p].h = 1 + max(left, right)

				left, right = int32(-1), int32(-1)
				if t.nodes[x].ls != sentinel {
					left = t.nodes[t.nodes[x].ls].h
				}
				if t.nodes[x].rs != sentinel {
					right = t.nodes[t.nodes[x].rs].h
				}
				t.nodes[x].h = 1 + max(left, right)
				newRoot = x
			} else if bf < -1 {
				// RL
				if t.nodes[p].rs != sentinel {
					left, right = int32(-1), int32(-1)
					if t.nodes[t.nodes[p].rs].ls != sentinel {
						left = t.nodes[t.nodes[t.nodes[p].rs].ls].h
					}
					if t.nodes[t.nodes[p].rs].rs != sentinel {
						right = t.nodes[t.nodes[t.nodes[p].rs].rs].h
					}
					if left-right > 0 {
						x := t.nodes[t.nodes[p].rs].ls
						t.nodes[t.nodes[p].rs].ls = t.nodes[x].rs
						t.nodes[x].rs = t.nodes[p].rs

						left, right = int32(-1), int32(-1)
						if t.nodes[t.nodes[p].rs].ls != sentinel {
							left = t.nodes[t.nodes[t.nodes[p].rs].ls].h
						}
						if t.nodes[t.nodes[p].rs].rs != sentinel {
							right = t.nodes[t.nodes[t.nodes[p].rs].rs].h
						}
						t.nodes[t.nodes[p].rs].h = 1 + max(left, right)

						left, right = int32(-1), int32(-1)
						if t.nodes[x].ls != sentinel {
							left = t.nodes[t.nodes[x].ls].h
						}
						if t.nodes[x].rs != sentinel {
							right = t.nodes[t.nodes[x].rs].h
						}
						t.nodes[x].h = 1 + max(left, right)

						t.nodes[p].rs = x
					}
				}
				// RR
				x := t.nodes[p].rs
				t.nodes[p].rs = t.nodes[x].ls
				t.nodes[x].ls = p

				left, right = int32(-1), int32(-1)
				if t.nodes[p].ls != sentinel {
					left = t.nodes[t.nodes[p].ls].h
				}
				if t.nodes[p].rs != sentinel {
					right = t.nodes[t.nodes[p].rs].h
				}
				t.nodes[p].h = 1 + max(left, right)

				left, right = int32(-1), int32(-1)
				if t.nodes[x].ls != sentinel {
					left = t.nodes[t.nodes[x].ls].h
				}
				if t.nodes[x].rs != sentinel {
					right = t.nodes[t.nodes[x].rs].h
				}
				t.nodes[x].h = 1 + max(left, right)

				newRoot = x
			} else {
				newRoot = p
			}
		}
		// assignar t.nodes[root].cr = newRoot i root = inserted
		t.nodes[root].cr = newRoot
		root = inserted
	}

	return uint32(root)
}
