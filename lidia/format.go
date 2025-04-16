package lidia

import (
	"bufio"
	"encoding/binary"
	"hash/crc32"
	"io"
)

type stringOffset uint64

type rangeEntry struct {
	length     uint64
	depth      uint64
	funcOffset stringOffset
	fileOffset stringOffset
	lineTable  lineTableRef
	callFile   stringOffset
	callLine   uint64
}

// sz 0x20
type vaTableHeader struct {
	entrySize uint64
	count     uint64
	offset    uint64
	crc       uint32
	_         uint32
}

// sz 0x20
type rangeTableHeader struct {
	fieldSize uint64
	count     uint64
	offset    uint64
	crc       uint32
	_         uint32
}

// sz 0x18
type stringsTableHeader struct {
	size   uint64
	offset uint64
	crc    uint32
	_      uint32
}

// sz 0x20
type lineTablesHeader struct {
	fieldSize uint64
	count     uint64
	offset    uint64
	crc       uint32
	_         uint32
}

type header struct {
	// 0x0
	magic   [4]byte
	version uint32
	// 0x8
	vaTableHeader vaTableHeader
	// 0x28
	rangeTableHeader rangeTableHeader
	// 0x48
	stringsTableHeader stringsTableHeader
	// 0x60
	lineTablesHeader lineTablesHeader
	// 0x80
}

func readHeader(file io.Reader) (header, error) {
	headerBuf := make([]byte, headerSize)
	if _, readErr := file.Read(headerBuf); readErr != nil {
		return header{}, readErr
	}
	hdr := header{}
	copy(hdr.magic[:], headerBuf[0:4])
	hdr.version = binary.LittleEndian.Uint32(headerBuf[4:])

	hdr.vaTableHeader.entrySize = binary.LittleEndian.Uint64(headerBuf[8:])
	hdr.vaTableHeader.count = binary.LittleEndian.Uint64(headerBuf[0x10:])
	hdr.vaTableHeader.offset = binary.LittleEndian.Uint64(headerBuf[0x18:])
	hdr.vaTableHeader.crc = binary.LittleEndian.Uint32(headerBuf[0x20:])

	hdr.rangeTableHeader.fieldSize = binary.LittleEndian.Uint64(headerBuf[0x28:])
	hdr.rangeTableHeader.count = binary.LittleEndian.Uint64(headerBuf[0x30:])
	hdr.rangeTableHeader.offset = binary.LittleEndian.Uint64(headerBuf[0x38:])
	hdr.rangeTableHeader.crc = binary.LittleEndian.Uint32(headerBuf[0x40:])

	hdr.stringsTableHeader.size = binary.LittleEndian.Uint64(headerBuf[0x48:])
	hdr.stringsTableHeader.offset = binary.LittleEndian.Uint64(headerBuf[0x50:])
	hdr.stringsTableHeader.crc = binary.LittleEndian.Uint32(headerBuf[0x58:])

	hdr.lineTablesHeader.fieldSize = binary.LittleEndian.Uint64(headerBuf[0x60:])
	hdr.lineTablesHeader.count = binary.LittleEndian.Uint64(headerBuf[0x68:])
	hdr.lineTablesHeader.offset = binary.LittleEndian.Uint64(headerBuf[0x70:])
	hdr.lineTablesHeader.crc = binary.LittleEndian.Uint32(headerBuf[0x78:])

	return hdr, nil
}

func readFields4(entryBuf []byte) rangeEntry {
	return rangeEntry{
		length:     uint64(binary.LittleEndian.Uint32(entryBuf[0:])),
		depth:      uint64(binary.LittleEndian.Uint32(entryBuf[4:])),
		funcOffset: stringOffset(binary.LittleEndian.Uint32(entryBuf[8:])),
		fileOffset: stringOffset(binary.LittleEndian.Uint32(entryBuf[12:])),
		lineTable: lineTableRef{
			idx:   uint64(binary.LittleEndian.Uint32(entryBuf[16:])),
			count: uint64(binary.LittleEndian.Uint32(entryBuf[20:])),
		},
		callFile: stringOffset(binary.LittleEndian.Uint32(entryBuf[24:])),
		callLine: uint64(binary.LittleEndian.Uint32(entryBuf[28:])),
	}
}

func readFields8(entryBuf []byte) rangeEntry {
	return rangeEntry{
		length:     binary.LittleEndian.Uint64(entryBuf[0:]),
		depth:      binary.LittleEndian.Uint64(entryBuf[8:]),
		funcOffset: stringOffset(binary.LittleEndian.Uint64(entryBuf[16:])),
		fileOffset: stringOffset(binary.LittleEndian.Uint64(entryBuf[24:])),
		lineTable: lineTableRef{
			idx:   binary.LittleEndian.Uint64(entryBuf[32:]),
			count: binary.LittleEndian.Uint64(entryBuf[40:]),
		},
		callFile: stringOffset(binary.LittleEndian.Uint64(entryBuf[48:])),
		callLine: binary.LittleEndian.Uint64(entryBuf[56:]),
	}
}

func (rc *rangeCollector) write(f io.WriteSeeker) error {
	buf := bufio.NewWriter(f)
	hdr := &header{
		version: version,
	}

	copy(hdr.magic[:], magic)

	if err := writeHeader(buf, hdr); err != nil {
		return err
	}

	hdr.vaTableHeader.offset = headerSize
	hdr.rangeTableHeader.offset = headerSize
	hdr.stringsTableHeader.offset = headerSize
	hdr.stringsTableHeader.size = uint64(len(rc.sb.buf))

	crc := crc32.New(castagnoli)
	_, _ = crc.Write(rc.sb.buf)
	hdr.stringsTableHeader.crc = crc.Sum32()

	if err := writeRangeEntries(rc.rb, hdr, buf); err != nil {
		return err
	}
	if err := buf.Flush(); err != nil {
		return err
	}

	if _, err := f.Write(rc.sb.buf); err != nil {
		return err
	}

	if err := writeLineTableEntries(rc.lb, hdr, buf); err != nil {
		return err
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	if err := writeHeader(f, hdr); err != nil {
		return err
	}
	return nil
}

func writeHeader(output io.Writer, hdr *header) error {
	headerBuf := make([]byte, headerSize)
	copy(headerBuf[0:4], hdr.magic[:])
	binary.LittleEndian.PutUint32(headerBuf[4:], hdr.version)

	binary.LittleEndian.PutUint64(headerBuf[0x8:], hdr.vaTableHeader.entrySize)
	binary.LittleEndian.PutUint64(headerBuf[0x10:], hdr.vaTableHeader.count)
	binary.LittleEndian.PutUint64(headerBuf[0x18:], hdr.vaTableHeader.offset)
	binary.LittleEndian.PutUint32(headerBuf[0x20:], hdr.vaTableHeader.crc)

	binary.LittleEndian.PutUint64(headerBuf[0x28:], hdr.rangeTableHeader.fieldSize)
	binary.LittleEndian.PutUint64(headerBuf[0x30:], hdr.rangeTableHeader.count)
	binary.LittleEndian.PutUint64(headerBuf[0x38:], hdr.rangeTableHeader.offset)
	binary.LittleEndian.PutUint32(headerBuf[0x40:], hdr.rangeTableHeader.crc)

	binary.LittleEndian.PutUint64(headerBuf[0x48:], hdr.stringsTableHeader.size)
	binary.LittleEndian.PutUint64(headerBuf[0x50:], hdr.stringsTableHeader.offset)
	binary.LittleEndian.PutUint32(headerBuf[0x58:], hdr.stringsTableHeader.crc)

	binary.LittleEndian.PutUint64(headerBuf[0x60:], hdr.lineTablesHeader.fieldSize)
	binary.LittleEndian.PutUint64(headerBuf[0x68:], hdr.lineTablesHeader.count)
	binary.LittleEndian.PutUint64(headerBuf[0x70:], hdr.lineTablesHeader.offset)
	binary.LittleEndian.PutUint32(headerBuf[0x78:], hdr.lineTablesHeader.crc)

	if _, err := output.Write(headerBuf); err != nil {
		return err
	}
	return nil
}

func writeRangeEntries(rb *rangesBuilder, hdr *header, buf *bufio.Writer) error {
	hdr.vaTableHeader.count = uint64(len(rb.va))
	hdr.rangeTableHeader.count = uint64(len(rb.entries))
	calculateSizes(rb, hdr)
	vaBuf := make([]byte, hdr.vaTableHeader.entrySize)
	{
		crc := crc32.New(castagnoli)
		ww := io.MultiWriter(crc, buf)
		if hdr.vaTableHeader.entrySize == 4 {
			for i := range rb.va {
				binary.LittleEndian.PutUint32(vaBuf, uint32(rb.va[i]))
				if _, err := ww.Write(vaBuf); err != nil {
					return err
				}
			}
		} else {
			for i := range rb.va {
				binary.LittleEndian.PutUint64(vaBuf, rb.va[i])
				if _, err := ww.Write(vaBuf); err != nil {
					return err
				}
			}
		}
		hdr.vaTableHeader.crc = crc.Sum32()
	}
	bsWritten := len(rb.va) * int(hdr.vaTableHeader.entrySize)
	hdr.rangeTableHeader.offset += uint64(bsWritten)

	{
		crc := crc32.New(castagnoli)
		ww := io.MultiWriter(crc, buf)
		if hdr.rangeTableHeader.fieldSize == 4 {
			entryBuf := make([]byte, fieldsEntrySize4)
			for i := range rb.entries {
				writeFields4(entryBuf, rb.entries[i])
				if _, err := ww.Write(entryBuf); err != nil {
					return err
				}
			}
			bsWritten += len(rb.entries) * fieldsEntrySize4
		} else {
			entryBuf := make([]byte, fieldsEntrySize8)
			for i := range rb.entries {
				writeFields8(entryBuf, rb.entries[i])
				if _, err := ww.Write(entryBuf); err != nil {
					return err
				}
			}
			bsWritten += len(rb.entries) * fieldsEntrySize8
		}
		hdr.rangeTableHeader.crc = crc.Sum32()
	}

	hdr.stringsTableHeader.offset += uint64(bsWritten)

	return nil
}

func writeLineTableEntries(lb *lineBuilder, hdr *header, buf *bufio.Writer) error {
	hdr.lineTablesHeader.offset = hdr.stringsTableHeader.offset + hdr.stringsTableHeader.size
	calculateLineTableFieldSize(lb, hdr)
	hdr.lineTablesHeader.count = uint64(len(lb.entries))
	crc := crc32.New(castagnoli)
	ww := io.MultiWriter(crc, buf)
	if hdr.lineTablesHeader.fieldSize == 2 {
		lineTableBuf := make([]byte, 4)
		for i := range lb.entries {
			binary.LittleEndian.PutUint16(lineTableBuf[0:], uint16(lb.entries[i].Offset))
			binary.LittleEndian.PutUint16(lineTableBuf[2:], uint16(lb.entries[i].LineNumber))
			if _, err := ww.Write(lineTableBuf); err != nil {
				return err
			}
		}
	} else {
		lineTableBuf := make([]byte, 8)
		for i := range lb.entries {
			binary.LittleEndian.PutUint32(lineTableBuf[0:], lb.entries[i].Offset)
			binary.LittleEndian.PutUint32(lineTableBuf[4:], lb.entries[i].LineNumber)
			if _, err := ww.Write(lineTableBuf); err != nil {
				return err
			}
		}
	}
	hdr.lineTablesHeader.crc = crc.Sum32()
	return buf.Flush()
}

func calculateSizes(rb *rangesBuilder, hdr *header) {
	const maxUint32 = uint64(^uint32(0))
	hdr.vaTableHeader.entrySize = 4
	hdr.rangeTableHeader.fieldSize = 4
	for _, va := range rb.va {
		if va > maxUint32 {
			hdr.vaTableHeader.entrySize = 8
			break
		}
	}

	for _, e := range rb.entries {
		if e.length > maxUint32 || e.depth > maxUint32 || uint64(e.funcOffset) > maxUint32 ||
			uint64(e.fileOffset) > maxUint32 || e.lineTable.idx > maxUint32 ||
			e.lineTable.count > maxUint32 {
			hdr.rangeTableHeader.fieldSize = 8
			break
		}
	}
}

func writeFields8(entryBuf []byte, e rangeEntry) {
	binary.LittleEndian.PutUint64(entryBuf[0:], e.length)
	binary.LittleEndian.PutUint64(entryBuf[8:], e.depth)
	binary.LittleEndian.PutUint64(entryBuf[0x10:], uint64(e.funcOffset))
	binary.LittleEndian.PutUint64(entryBuf[0x18:], uint64(e.fileOffset))
	binary.LittleEndian.PutUint64(entryBuf[0x20:], e.lineTable.idx)
	binary.LittleEndian.PutUint64(entryBuf[0x28:], e.lineTable.count)
	binary.LittleEndian.PutUint64(entryBuf[0x30:], uint64(e.callFile))
	binary.LittleEndian.PutUint64(entryBuf[0x38:], e.callLine)
}

func writeFields4(entryBuf []byte, e rangeEntry) {
	binary.LittleEndian.PutUint32(entryBuf[0:], uint32(e.length))
	binary.LittleEndian.PutUint32(entryBuf[4:], uint32(e.depth))
	binary.LittleEndian.PutUint32(entryBuf[8:], uint32(e.funcOffset))
	binary.LittleEndian.PutUint32(entryBuf[0xc:], uint32(e.fileOffset))
	binary.LittleEndian.PutUint32(entryBuf[0x10:], uint32(e.lineTable.idx))
	binary.LittleEndian.PutUint32(entryBuf[0x14:], uint32(e.lineTable.count))
	binary.LittleEndian.PutUint32(entryBuf[0x18:], uint32(e.callFile))
	binary.LittleEndian.PutUint32(entryBuf[0x1c:], uint32(e.callLine))
}

func calculateLineTableFieldSize(lb *lineBuilder, hdr *header) {
	const maxUint16 = uint32(^uint16(0))
	hdr.lineTablesHeader.fieldSize = 2
	for i := range lb.entries {
		if lb.entries[i].Offset > maxUint16 || lb.entries[i].LineNumber > maxUint16 {
			hdr.lineTablesHeader.fieldSize = 4
			break
		}
	}
}
