// nolint unused
package phlaredb

import (
	"unsafe"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
)

type mappingsHelper struct{}

const mappingSize = uint64(unsafe.Sizeof(profilev1.Mapping{}))

type mappingsKey struct {
	MemoryStart     uint64
	MemoryLimit     uint64
	FileOffset      uint64
	Filename        int64 // Index into string table
	BuildId         int64 //nolint // Index into string table
	HasFunctions    bool
	HasFilenames    bool
	HasLineNumbers  bool
	HasInlineFrames bool
}

func (*mappingsHelper) key(m *profilev1.Mapping) mappingsKey {
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
func (*mappingsHelper) rewrite(r *rewriter, m *profilev1.Mapping) error {
	r.strings.rewrite(&m.Filename)
	r.strings.rewrite(&m.BuildId)
	return nil
}

func (*mappingsHelper) setID(_, newID uint64, m *profilev1.Mapping) uint64 {
	oldID := m.Id
	m.Id = newID
	return oldID
}

func (*mappingsHelper) size(_ *profilev1.Mapping) uint64 {
	return mappingSize
}

func (*mappingsHelper) clone(m *profilev1.Mapping) *profilev1.Mapping {
	return m
}
