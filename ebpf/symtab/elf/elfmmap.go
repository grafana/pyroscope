package elf

import (
	"debug/elf"
	"fmt"
	"os"
	"runtime"

	"github.com/ianlancetaylor/demangle"
)

type MMapedElfFile struct {
	InMemElfFile
	fpath string
	err   error
	fd    *os.File
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
	f, err := NewInMemElfFile(res.fd)
	if err != nil {
		res.Close()
		return nil, err
	}
	res.InMemElfFile = *f
	runtime.SetFinalizer(res, (*MMapedElfFile).Finalize)
	return res, nil
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
	f.InMemElfFile.Clear()
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
	f.InMemElfFile.resetReader(f.fd)
	return nil
}

func (f *MMapedElfFile) SectionData(s *elf.SectionHeader) ([]byte, error) {
	if err := f.ensureOpen(); err != nil {
		return nil, err
	}
	return f.InMemElfFile.SectionData(s)
}

func (f *MMapedElfFile) FilePath() string {
	return f.fpath
}

// getString extracts a string from an ELF string table.
func (f *MMapedElfFile) getString(start int, demangleOptions []demangle.Option) (string, bool) {
	if err := f.ensureOpen(); err != nil {
		return "", false
	}
	return f.InMemElfFile.getString(start, demangleOptions)
}
