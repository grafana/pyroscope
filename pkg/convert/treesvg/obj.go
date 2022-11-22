package treesvg

import (
	"errors"

	"github.com/google/pprof/driver"
)

type obj struct{}
type objFile struct{}

// Name() string
// ObjAddr(addr uint64) (uint64, error)
// BuildID() string
// SourceLine(addr uint64) ([]Frame, error)
// Symbols(r *regexp.Regexp, addr uint64) ([]*Sym, error)
// Close() error

func (o *obj) Open(file string, start, limit, offset uint64, relocationSymbol string) (driver.ObjFile, error) {
	// return &objFile{}, errors.New("not implemented")
	return nil, errors.New("not implemented")
}

func (o *obj) Disasm(file string, start, end uint64, intelSyntax bool) ([]driver.Inst, error) {
	return nil, nil
}
