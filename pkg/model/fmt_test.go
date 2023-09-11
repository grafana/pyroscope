package model

import (
	"bufio"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_DropGoTypeParameters(t *testing.T) {
	ef, err := os.Open("testdata/go_type_parameters.expected.txt")
	require.NoError(t, err)
	defer ef.Close()

	in, err := os.Open("testdata/go_type_parameters.txt")
	require.NoError(t, err)
	defer in.Close()

	input := bufio.NewScanner(in)
	expected := bufio.NewScanner(ef)
	for input.Scan() {
		expected.Scan()
		require.Equal(t, expected.Text(), DropGoTypeParameters(input.Text()))
	}

	require.NoError(t, input.Err())
	require.NoError(t, expected.Err())
	require.False(t, expected.Scan())
}
