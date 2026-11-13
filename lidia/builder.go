package lidia

import (
	"encoding/binary"
	"hash/maphash"
	"io"
	"sort"
	"strings"
)

type lineTableRef struct {
	idx   uint64
	count uint64
}

type lineBuilder struct {
	entries []LineTableEntry
}

func newLineTableBuilder() *lineBuilder {
	return &lineBuilder{}
}

func (ltb *lineBuilder) add(lines LineTable) lineTableRef {
	o := len(ltb.entries)
	sz := len(lines)
	ltb.entries = append(ltb.entries, lines...)
	return lineTableRef{idx: uint64(o), count: uint64(sz)}
}

// stringChunkSize is the capacity of a string table chunk. A longer entry gets
// a chunk of its own.
const stringChunkSize = 1 << 20

// stringBuilder accumulates the deduplicated string table.
//
// The table is stored in chunks, so it is never copied as it grows. Strings
// are deduplicated by hash, so add does not retain the strings passed to it;
// a hash match is confirmed against the stored bytes.
type stringBuilder struct {
	// chunks hold the table contents in order. An entry never spans two
	// chunks, so a chunk may end before it is full.
	chunks      [][]byte
	chunkStarts []stringOffset
	size        stringOffset

	seed   maphash.Seed
	unique map[uint64]stringOffset
	// collisions holds strings whose hash is already taken by a different string.
	collisions map[string]stringOffset

	overflow stringOffset
	emptystr stringOffset
}

func newStringBuilder() *stringBuilder {
	sb := &stringBuilder{
		seed:   maphash.MakeSeed(),
		unique: make(map[uint64]stringOffset),
	}
	sb.emptystr = sb.add("")
	sb.overflow = sb.add("[overflow]")
	return sb
}

func (sb *stringBuilder) add(s string) stringOffset {
	return sb.addHashed(s, maphash.String(sb.seed, s))
}

// addHashed adds s, whose hash is h.
func (sb *stringBuilder) addHashed(s string, h uint64) stringOffset {
	prev, exists := sb.unique[h]
	if exists {
		if sb.equal(prev, s) {
			return prev
		}
		if prev, ok := sb.collisions[s]; ok {
			return prev
		}
	}

	strLen := len(s)
	if strLen >= int(^uint32(0)) {
		return sb.overflow
	}
	offset := sb.appendEntry(s)
	if !exists {
		sb.unique[h] = offset
	} else {
		if sb.collisions == nil {
			sb.collisions = make(map[string]stringOffset)
		}
		// s may alias a larger buffer, such as an ELF string table.
		sb.collisions[strings.Clone(s)] = offset
	}
	return offset
}

// appendEntry stores s, prefixed with its length, and returns its offset.
func (sb *stringBuilder) appendEntry(s string) stringOffset {
	n := 4 + len(s)
	last := len(sb.chunks) - 1
	if last < 0 || cap(sb.chunks[last])-len(sb.chunks[last]) < n {
		sb.chunks = append(sb.chunks, make([]byte, 0, max(n, stringChunkSize)))
		sb.chunkStarts = append(sb.chunkStarts, sb.size)
		last++
	}
	c := binary.LittleEndian.AppendUint32(sb.chunks[last], uint32(len(s)))
	sb.chunks[last] = append(c, s...)

	offset := sb.size
	sb.size = stringOffset(uint64(sb.size) + uint64(n))
	return offset
}

// equal reports whether the entry stored at offset is s.
func (sb *stringBuilder) equal(offset stringOffset, s string) bool {
	i := sort.Search(len(sb.chunkStarts), func(i int) bool { return sb.chunkStarts[i] > offset }) - 1
	b := sb.chunks[i][offset-sb.chunkStarts[i]:]
	return int(binary.LittleEndian.Uint32(b)) == len(s) && string(b[4:4+len(s)]) == s
}

// writeTo writes the string table to w.
func (sb *stringBuilder) writeTo(w io.Writer) error {
	for _, c := range sb.chunks {
		if _, err := w.Write(c); err != nil {
			return err
		}
	}
	return nil
}

// rangesBuilder
type rangesBuilder struct {
	entries []rangeEntry
	va      []uint64
}

func newRangesBuilder() *rangesBuilder {
	return &rangesBuilder{}
}

func (rb *rangesBuilder) add(va uint64, e rangeEntry) {
	rb.entries = append(rb.entries, e)
	rb.va = append(rb.va, va)
}

func (rb *rangesBuilder) sort() {
	sort.Stable(&sortByVADepth{rb})
}

// sortByVADepth sorts the ranges by VA and then by depth.
type sortByVADepth struct {
	b *rangesBuilder
}

func (s *sortByVADepth) Len() int {
	return len(s.b.entries)
}

func (s *sortByVADepth) Less(i, j int) bool {
	if s.b.va[i] == s.b.va[j] {
		return s.b.entries[i].depth < s.b.entries[j].depth
	}
	return s.b.va[i] < s.b.va[j]
}

func (s *sortByVADepth) Swap(i, j int) {
	s.b.entries[i], s.b.entries[j] = s.b.entries[j], s.b.entries[i]
	s.b.va[i], s.b.va[j] = s.b.va[j], s.b.va[i]
}

// rangeCollector
type rangeCollector struct {
	sb *stringBuilder
	rb *rangesBuilder
	lb *lineBuilder

	opt options
}

func (rc *rangeCollector) VisitRange(r *Range) {
	lt := lineTableRef{}
	funcOffset := rc.sb.add(r.Function)
	fileOffset := rc.sb.emptystr
	callFileOffset := rc.sb.emptystr
	if rc.opt.files {
		fileOffset = rc.sb.add(r.File)
		callFileOffset = rc.sb.add(r.CallFile)
	}

	if rc.opt.lines {
		lt = rc.lb.add(r.LineTable)
	}
	e := rangeEntry{
		length:     uint64(r.Length),
		depth:      uint64(r.Depth),
		funcOffset: funcOffset,
		fileOffset: fileOffset,
		lineTable:  lt,
		callFile:   callFileOffset,
		callLine:   uint64(r.CallLine),
	}
	rc.rb.add(r.VA, e)
}
