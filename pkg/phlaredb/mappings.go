// nolint unused
package phlaredb

import (
	"unsafe"

	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
)

type mappingsHelper struct{}

const mappingSize = uint64(unsafe.Sizeof(schemav1.InMemoryMapping{}))

type mappingsKey struct {
	MemoryStart     uint64
	MemoryLimit     uint64
	FileOffset      uint64
	Filename        uint32 // Index into string table
	BuildId         uint32 // Index into string table
	HasFunctions    bool
	HasFilenames    bool
	HasLineNumbers  bool
	HasInlineFrames bool
}

func (*mappingsHelper) key(m *schemav1.InMemoryMapping) mappingsKey {
	return mappingsKey{
		MemoryStart:     m.MemoryStart,
		MemoryLimit:     m.MemoryLimit,
		FileOffset:      m.FileOffset,
		Filename:        m.Filename,
		BuildId:         m.BuildId,
		HasFunctions:    m.HasFunctions,
		HasFilenames:    m.HasFilenames,
		HasLineNumbers:  m.HasFunctions,
		HasInlineFrames: m.HasInlineFrames,
	}
}

func (*mappingsHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.mappings = elemRewriter
}

// nolint unparam
func (*mappingsHelper) rewrite(r *rewriter, m *schemav1.InMemoryMapping) error {
	r.strings.rewriteUint32(&m.Filename)
	r.strings.rewriteUint32(&m.BuildId)
	return nil
}

func (*mappingsHelper) setID(_, newID uint64, m *schemav1.InMemoryMapping) uint64 {
	oldID := m.Id
	m.Id = newID
	return oldID
}

func (*mappingsHelper) size(_ *schemav1.InMemoryMapping) uint64 {
	return mappingSize
}

func (*mappingsHelper) clone(m *schemav1.InMemoryMapping) *schemav1.InMemoryMapping {
	return &(*m)
}
