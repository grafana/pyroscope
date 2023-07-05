package elf

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	name := NewName(0xef, 1)
	require.Equal(t, uint32(0xef), name.NameIndex())
	require.Equal(t, SectionLinkIndex(1), name.LinkIndex())
}
