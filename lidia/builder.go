package lidia

import (
	"encoding/binary"
	"sort"
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

type stringBuilder struct {
	buf      []byte
	unique   map[string]stringOffset
	offset   stringOffset
	overflow stringOffset
	emptystr stringOffset
}

func newStringBuilder() *stringBuilder {
	sb := &stringBuilder{
		buf:    make([]byte, 0),
		unique: make(map[string]stringOffset),
	}
	sb.emptystr = sb.add("")
	sb.overflow = sb.add("[overflow]")
	return sb
}

func (sb *stringBuilder) add(s string) stringOffset {
	if prev, exists := sb.unique[s]; exists {
		return prev
	}

	strLen := len(s)
	if strLen >= int(^uint32(0)) {
		return sb.overflow
	}
	sb.buf = binary.LittleEndian.AppendUint32(sb.buf, uint32(strLen))
	sb.buf = append(sb.buf, s...)

	offset := sb.offset
	sb.unique[s] = offset
	sb.offset = stringOffset(uint64(sb.offset) + uint64(4+strLen))

	return offset
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
	sort.Sort(&sortByVADepth{rb})
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
	sb  *stringBuilder
	rb  *rangesBuilder
	lb  *lineBuilder
	blb *binaryLayoutBuilder

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

type binaryLayoutBuilder struct {
	buf []byte
}

func newBinaryLayoutBuilder() *binaryLayoutBuilder {
	return &binaryLayoutBuilder{
		buf: make([]byte, 0, 256),
	}
}

func (blb *binaryLayoutBuilder) write(layout *BinaryLayoutInfo) {
	// Reset buffer
	blb.buf = blb.buf[:0]

	blb.buf = binary.LittleEndian.AppendUint16(blb.buf, layout.Type)

	count := uint32(len(layout.ProgramHeaders))
	blb.buf = binary.LittleEndian.AppendUint32(blb.buf, count)

	for _, ph := range layout.ProgramHeaders {
		blb.buf = binary.LittleEndian.AppendUint32(blb.buf, ph.Type)
		blb.buf = binary.LittleEndian.AppendUint32(blb.buf, ph.Flags)
		blb.buf = binary.LittleEndian.AppendUint64(blb.buf, ph.Offset)
		blb.buf = binary.LittleEndian.AppendUint64(blb.buf, ph.VirtualAddr)
		blb.buf = binary.LittleEndian.AppendUint64(blb.buf, ph.PhysAddr)
		blb.buf = binary.LittleEndian.AppendUint64(blb.buf, ph.FileSize)
		blb.buf = binary.LittleEndian.AppendUint64(blb.buf, ph.MemSize)
		blb.buf = binary.LittleEndian.AppendUint64(blb.buf, ph.Align)
	}
}
