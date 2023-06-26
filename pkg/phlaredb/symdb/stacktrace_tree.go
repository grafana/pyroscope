package symdb

import (
	"bufio"
	"io"

	"github.com/dgryski/go-groupvarint"

	"github.com/grafana/phlare/pkg/util/math"
)

const defaultStacktraceTreeSize = 10 << 10

type stacktraceTree struct {
	nodes []node
}

type node struct {
	p int32 // Parent index.
	r int32 // Reference the to stack frame data.
	// Auxiliary members only needed for insertion.
	fc int32 // First child index.
	ns int32 // Next sibling index.
}

func newStacktraceTree(size int) *stacktraceTree {
	if size < 1 {
		size = 1
	}
	t := stacktraceTree{nodes: make([]node, 1, size)}
	t.nodes[0] = node{
		p:  sentinel,
		fc: sentinel,
		ns: sentinel,
	}
	return &t
}

const sentinel = -1

func (t *stacktraceTree) len() uint32 { return uint32(len(t.nodes)) }

func (t *stacktraceTree) insert(refs []uint64) uint32 {
	var (
		n = &t.nodes[0]
		i = n.fc
		x int32
	)

	for j := len(refs) - 1; j >= 0; {
		r := int32(refs[j])
		if i == sentinel {
			ni := int32(len(t.nodes))
			t.nodes = append(t.nodes, node{
				r:  r,
				p:  x,
				fc: sentinel,
				ns: sentinel,
			})
			n.fc = ni
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

// TODO(kolesnikovae): Implement tree merge.
//
// func (t *stacktraceTree) merge(*stacktraceTree) {}

const (
	maxGroupSize = 17 // 4 * uint32 + control byte
	// minGroupSize = 5  // 4 * byte + control byte
)

func (t *stacktraceTree) WriteTo(dst io.Writer) (int64, error) {
	e := treeEncoder{
		writeSize: 4 << 10,
	}
	err := e.marshal(t, dst)
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
	if id >= uint32(len(t.nodes)) {
		return dst
	}
	dst = dst[:0]
	n := t.nodes[id]
	for n.p >= 0 {
		dst = append(dst, n.r)
		n = t.nodes[n.p]
	}
	return dst
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

func (tc *treeEncoder) marshal(t *stacktraceTree, w io.Writer) (err error) {
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
			g = g[:math.Min((xn+xn%2)*2, d.groupBuffer)]
			var gp int

			// Check if there is a remainder. If this is the case,
			// decode the group and advance gp.
			if len(rb) > 0 {
				// It's expected that r contains a single complete group.
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
			read += n + rn // Mark remainder bytes as read, we copy them.
			if rn > 0 {
				// If there is a remainder, it is copied and decoded on
				// the next Peek. This should not be possible with eof.
				rb = append(rb, b[len(b)-rn:]...)
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
