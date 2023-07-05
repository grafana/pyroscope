package elf

import (
	"bytes"
	"debug/elf"
	"fmt"
	"os"
	"runtime"
	"strings"
)

type MMapedElfFile struct {
	elf.FileHeader
	Sections []elf.SectionHeader
	Progs    []elf.ProgHeader

	fpath string
	err   error
	fd    *os.File

	stringCache map[int]string
}

func NewMMapedElfFile(fpath string) (*MMapedElfFile, error) {
	res := &MMapedElfFile{
		fpath: fpath,
	}
	err := res.ensureOpen()
	if err != nil {
		res.Close()
		return nil, err
	}
	elfFile, err := elf.NewFile(res.fd)
	if err != nil {
		res.Close()
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

	runtime.SetFinalizer(res, (*MMapedElfFile).Finalize)
	return res, nil
}

func (f *MMapedElfFile) Section(name string) *elf.SectionHeader {
	for i := range f.Sections {
		s := &f.Sections[i]
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (f *MMapedElfFile) sectionByType(typ elf.SectionType) *elf.SectionHeader {
	for i := range f.Sections {
		s := &f.Sections[i]
		if s.Type == typ {
			return s
		}
	}
	return nil
}

func (f *MMapedElfFile) ensureOpen() error {
	if f.fd != nil {
		return nil
	}
	return f.open()
}

func (f *MMapedElfFile) Finalize() {
	if f.fd != nil {
		println("ebpf mmaped elf not closed")
	}
	f.Close()
}
func (f *MMapedElfFile) Close() {
	if f.fd != nil {
		f.fd.Close()
		f.fd = nil
	}
	f.stringCache = nil
	f.Sections = nil
}
func (f *MMapedElfFile) open() error {
	if f.err != nil {
		return fmt.Errorf("failed previously %w", f.err)
	}
	fd, err := os.OpenFile(f.fpath, os.O_RDONLY, 0)
	if err != nil {
		f.err = err
		return fmt.Errorf("open elf file %s %w", f.fpath, err)
	}
	f.fd = fd
	return nil
}

func (f *MMapedElfFile) SectionData(s *elf.SectionHeader) ([]byte, error) {
	if err := f.ensureOpen(); err != nil {
		return nil, err
	}
	res := make([]byte, s.Size)
	if _, err := f.fd.ReadAt(res, int64(s.Offset)); err != nil {
		return nil, err
	}
	return res, nil
}

func (f *MMapedElfFile) FilePath() string {
	return f.fpath
}

// getString extracts a string from an ELF string table.
func (f *MMapedElfFile) getString(start int) (string, bool) {
	if err := f.ensureOpen(); err != nil {
		return "", false
	}
	if s, ok := f.stringCache[start]; ok {
		return s, true
	}
	const tmpBufSize = 128
	var tmpBuf [tmpBufSize]byte
	sb := strings.Builder{}
	for i := 0; i < 10; i++ {
		_, err := f.fd.ReadAt(tmpBuf[:], int64(start+i*tmpBufSize))
		if err != nil {
			return "", false
		}
		idx := bytes.IndexByte(tmpBuf[:], 0)
		if idx >= 0 {
			sb.Write(tmpBuf[:idx])
			s := sb.String()
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
