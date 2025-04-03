package gtbl

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"sort"
)

const (
	magic   uint32 = 0x6c627467 // "gtbl"
	version uint32 = 1
)

type entry struct {
	va uint64
	rangeEntry
}

func (e entry) String() string {
	return fmt.Sprintf("va: %x, length: %d depth: %d", e.va, e.length, e.depth)
}

type ReaderAtCloser interface {
	io.ReadCloser
	io.ReaderAt
}

type Table struct {
	file ReaderAtCloser
	hdr  header
	opt  options

	vaTable []byte

	fieldsBuffer []byte
}

func OpenReader(f ReaderAtCloser, opt ...Option) (*Table, error) {
	var err error
	res := new(Table)

	for _, o := range opt {
		o(&res.opt)
	}

	res.file = f

	hdr, err := readHeader(f)
	if err != nil {
		res.Close()
		return nil, err
	}

	if hdr.magic != magic {
		res.Close()
		return nil, errors.New("invalid magic number")
	}
	if hdr.version != version {
		res.Close()
		return nil, errors.New("unsupported version")
	}
	if hdr.vaTableHeader.entrySize != 4 && hdr.vaTableHeader.entrySize != 8 {
		res.Close()
		return nil, errors.New("invalid vaSize")
	}
	if hdr.rangeTableHeader.fieldSize != 4 && hdr.rangeTableHeader.fieldSize != 8 {
		res.Close()
		return nil, errors.New("invalid fieldSize")
	}
	if hdr.rangeTableHeader.count != hdr.vaTableHeader.count {
		res.Close()
		return nil, errors.New("invalid count")
	}
	res.hdr = hdr

	res.fieldsBuffer = make([]byte, int(hdr.rangeTableHeader.fieldSize)*fieldsCount)
	// all functions addresses sorted.
	res.vaTable = make([]byte, int(hdr.vaTableHeader.entrySize)*int(hdr.vaTableHeader.count))

	if _, err = f.ReadAt(res.vaTable, int64(hdr.vaTableHeader.offset)); err != nil {
		res.Close()
		return nil, err
	}
	if res.opt.crc {
		if err = res.CheckCRC(); err != nil {
			res.Close()
			return nil, err
		}
	}

	return res, nil
}

func (st *Table) Lookup(addr64 uint64) ([]SourceInfoFrame, error) {
	var result []SourceInfoFrame

	addr := addr64
	idx := sort.Search(int(st.hdr.vaTableHeader.count), func(i int) bool {
		return st.getEntryVA(i) > addr
	})
	idx--
	var prev entry
	_ = prev
	for idx >= 0 {
		it, err := st.getEntry(idx) // todo: prefetch multiple entries to minimize io calls
		if err != nil {
			return result[:0], err
		}

		covered := it.va <= addr && addr < it.va+it.length
		if covered {
			name := st.str(it.funcOffset)
			res := SourceInfoFrame{
				FunctionName: name,
			}
			result = append(result, res)
			prev = it
		}
		if it.depth == 0 {
			break
		}
		idx--
	}
	return result, nil
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

func (st *Table) CheckCRC() error {
	if err := st.CheckCRCVA(); err != nil {
		return err
	}
	if err := st.CheckCRCStrings(); err != nil {
		return err
	}
	if err := st.CheckCRCFields(); err != nil {
		return err
	}
	if err := st.CheckCRCLineTables(); err != nil {
		return err
	}
	return nil
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

func (st *Table) Close() {
	if st.file != nil {
		_ = st.file.Close()
	}
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
