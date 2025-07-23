package lidia

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

type entry struct {
	va uint64
	rangeEntry
}

func (e entry) String() string {
	return fmt.Sprintf("va: %x, length: %d depth: %d", e.va, e.length, e.depth)
}

func (st *Table) getEntry(i int) (entry, error) {
	if i < 0 || i >= int(st.hdr.vaTableHeader.count) {
		return entry{}, errors.New("index out of bounds")
	}
	offset := int64(st.hdr.rangeTableHeader.offset) + int64(i)*int64(len(st.fieldsBuffer))

	if _, err := st.file.ReadAt(st.fieldsBuffer, offset); err != nil {
		return entry{}, err
	}
	e := entry{}
	if st.hdr.rangeTableHeader.fieldSize == 4 {
		e.rangeEntry = readFields4(st.fieldsBuffer)
	} else {
		e.rangeEntry = readFields8(st.fieldsBuffer)
	}
	e.va = st.getEntryVA(i)
	return e, nil
}

func (st *Table) CheckCRCVA() error {
	crc := crc32.New(castagnoli)
	_, _ = crc.Write(st.vaTable)
	if crc.Sum32() != st.hdr.vaTableHeader.crc {
		return errors.New("crc mismatch in va table")
	}
	return nil
}

func (st *Table) CheckCRCStrings() error {
	return checkCRC(st.file,
		int64(st.hdr.stringsTableHeader.offset),
		int64(st.hdr.stringsTableHeader.size),
		st.hdr.stringsTableHeader.crc,
		"strings")
}

func (st *Table) CheckCRCFields() error {
	elementSize := int64(st.hdr.rangeTableHeader.fieldSize) * fieldsCount
	sz := elementSize * int64(st.hdr.rangeTableHeader.count)
	return checkCRC(st.file,
		int64(st.hdr.rangeTableHeader.offset),
		sz, st.hdr.rangeTableHeader.crc,
		"fields")
}

func (st *Table) CheckCRCLineTables() error {
	elementSize := int64(st.hdr.lineTablesHeader.fieldSize) * lineTableFieldsCount
	return checkCRC(st.file,
		int64(st.hdr.lineTablesHeader.offset),
		elementSize*int64(st.hdr.lineTablesHeader.count),
		st.hdr.lineTablesHeader.crc,
		"linetable")
}

func (st *Table) getEntryVA(i int) uint64 {
	offset := int64(i) * int64(st.hdr.vaTableHeader.entrySize)
	it := st.vaTable[offset : offset+int64(st.hdr.vaTableHeader.entrySize)]
	if st.hdr.vaTableHeader.entrySize == 4 {
		return uint64(binary.LittleEndian.Uint32(it))
	}
	return binary.LittleEndian.Uint64(it)
}

func (st *Table) str(offset stringOffset) string {
	if offset == 0 {
		return ""
	}
	var strLen uint32
	buf := st.fieldsBuffer[:4]
	if _, err := st.readStrData(buf, uint64(offset)); err != nil {
		return ""
	}
	strLen = binary.LittleEndian.Uint32(buf)
	strData := make([]byte, strLen)
	if _, err := st.readStrData(strData, uint64(offset)+4); err != nil {
		if err != io.EOF {
			return ""
		}
	}
	return string(strData)
}

func (st *Table) readStrData(buf []byte, o uint64) (int, error) {
	return st.file.ReadAt(buf, int64(st.hdr.stringsTableHeader.offset+o))
}

func checkCRC(f ReaderAtCloser, offset, size int64, expected uint32, name string) error {
	crc := crc32.New(castagnoli)
	n, err := io.Copy(crc, io.NewSectionReader(f, offset, size))
	if err != nil {
		return err
	}
	if n != size {
		return errors.New("unexpected end of " + name)
	}
	if crc.Sum32() != expected {
		return errors.New("crc mismatch in " + name)
	}
	return nil
}
