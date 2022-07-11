package v1

import (
	"bytes"
	"strings"
	"testing"

	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/require"
)

// This test ensures that the structs that are stored and the used schema matches
func TestSchemaMatch(t *testing.T) {
	profilesStructSchema := parquet.SchemaOf(&Profile{})
	require.Equal(t, profilesStructSchema.String(), profilesSchema.String())

	stacktracesStructSchema := parquet.SchemaOf(&storedStacktrace{})
	require.Equal(t, strings.Replace(stacktracesStructSchema.String(), "message storedStacktrace", "message Stacktrace", 1), stacktracesSchema.String())

}

func newStacktraces() []*Stacktrace {
	return []*Stacktrace{
		{LocationIDs: []uint64{0x11}},
		{LocationIDs: []uint64{}},
		{LocationIDs: []uint64{12, 13}},
		{LocationIDs: []uint64{}},
		{LocationIDs: []uint64{14, 15}},
	}
}

func TestStacktracesRoundTrip(t *testing.T) {
	var (
		s   = newStacktraces()
		w   = &Writer[*Stacktrace, *StacktracePersister]{}
		buf bytes.Buffer
	)

	require.NoError(t, w.WriteParquetFile(&buf, s))

	/*
		n, err := buf.WriteRows(s.ToRows())
		require.NoError(t, err)
		assert.Equal(t, 5, n)

		var rows = make([]parquet.Row, len(s))
		n, err = buf.Rows().ReadRows(rows)
		require.NoError(t, err)
		assert.Equal(t, 5, n)

		sRoundTrip, err := StacktracesFromRows(rows)
		require.NoError(t, err)
		assert.Equal(t, s, sRoundTrip)
	*/
}

func newStrings() []string {
	return []string{
		"",
		"foo",
		"bar",
		"baz",
		"",
	}
}

func TestStringsRoundTrip(t *testing.T) {
	var (
		s   = newStrings()
		w   = &Writer[string, *StringPersister]{}
		buf bytes.Buffer
	)

	require.NoError(t, w.WriteParquetFile(&buf, s))

}
