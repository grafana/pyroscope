package model

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Benchmark_NewFlamegraphDiff(b *testing.B) {
	leftTreeBytes, err := os.ReadFile("testdata/diff_left_tree.bin")
	require.NoError(b, err)
	rightTreeBytes, err := os.ReadFile("testdata/diff_right_tree.bin")
	require.NoError(b, err)

	leftTree, err := UnmarshalTree(leftTreeBytes)
	require.NoError(b, err)

	rightTree, err := UnmarshalTree(rightTreeBytes)
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		diff, err := NewFlamegraphDiff(leftTree, rightTree, 163840)
		require.NoError(b, err)
		require.NotNil(b, diff)
	}
}
