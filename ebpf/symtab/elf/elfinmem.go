package elf

import (
	"bytes"
	"debug/elf"
	"io"
	"strings"

	"github.com/ianlancetaylor/demangle"
)

type ElfSymbolReader interface {
	getString(start int, demangleOptions []demangle.Option) (string, bool)
}

type InMemElfFile struct {
	elf.FileHeader
	Sections    []elf.SectionHeader
	Progs       []elf.ProgHeader
	stringCache map[int]string

	reader io.ReaderAt
}

func NewInMemElfFile(r io.ReaderAt) (*InMemElfFile, error) {
	res := &InMemElfFile{
		reader: r,
	}
	elfFile, err := elf.NewFile(res.reader)
	if err != nil {
		return nil, err
	}
	progs := make([]elf.ProgHeader, 0, len(elfFile.Progs))
	sections := make([]elf.SectionHeader, 0, len(elfFile.Sections))
	for i := range elfFile.Progs {
		progs = append(progs, elfFile.Progs[i].ProgHeader)
	}
	for i := range elfFile.Sections {
		sections = append(sections, elfFile.Sections[i].SectionHeader)
	}
	res.FileHeader = elfFile.FileHeader
	res.Progs = progs
	res.Sections = sections
	return res, nil
}

func (f *InMemElfFile) Clear() {
	f.stringCache = nil
	f.Sections = nil
}

func (f *InMemElfFile) resetReader(r io.ReaderAt) {
	f.reader = r
}

func (f *InMemElfFile) Section(name string) *elf.SectionHeader {
	for i := range f.Sections {
		s := &f.Sections[i]
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (f *InMemElfFile) sectionByType(typ elf.SectionType) *elf.SectionHeader {
	for i := range f.Sections {
		s := &f.Sections[i]
		if s.Type == typ {
			return s
		}
	}
	return nil
}

func (f *InMemElfFile) SectionData(s *elf.SectionHeader) ([]byte, error) {
	res := make([]byte, s.Size)
	if _, err := f.reader.ReadAt(res, int64(s.Offset)); err != nil {
		return nil, err
	}
	return res, nil
}

// getString extracts a string from an ELF string table.
func (f *InMemElfFile) getString(start int, demangleOptions []demangle.Option) (string, bool) {
	if s, ok := f.stringCache[start]; ok {
		return s, true
	}
	const tmpBufSize = 128
	var tmpBuf [tmpBufSize]byte
	sb := strings.Builder{}
	for i := 0; i < 10; i++ {
		_, err := f.reader.ReadAt(tmpBuf[:], int64(start+i*tmpBufSize))
		if err != nil {
			return "", false
		}
		idx := bytes.IndexByte(tmpBuf[:], 0)
		if idx >= 0 {
			sb.Write(tmpBuf[:idx])
			s := sb.String()
			if len(demangleOptions) > 0 {
				s = demangle.Filter(s, demangleOptions...)
			}
			if f.stringCache == nil {
				f.stringCache = make(map[int]string)
			}
			f.stringCache[start] = s
			return s, true
		} else {
			sb.Write(tmpBuf[:])
		}
	}
	return "", false
}
