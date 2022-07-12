package v1

import (
	"bytes"
	"strings"
	"testing"

	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test ensures that the structs that are stored and the used schema matches
func TestSchemaMatch(t *testing.T) {
	profilesStructSchema := parquet.SchemaOf(&Profile{})
	require.Equal(t, profilesStructSchema.String(), profilesSchema.String())

	stacktracesStructSchema := parquet.SchemaOf(&storedStacktrace{})
	require.Equal(t, strings.Replace(stacktracesStructSchema.String(), "message storedStacktrace", "message Stacktrace", 1), stacktracesSchema.String())

	stringsStructSchema := parquet.SchemaOf(&storedString{})
	require.Equal(t, strings.Replace(stringsStructSchema.String(), "message storedString", "message String", 1), stringsSchema.String())
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
		w   = &ReadWriter[*Stacktrace, *StacktracePersister]{}
		buf bytes.Buffer
	)

	require.NoError(t, w.WriteParquetFile(&buf, s))

	sRead, err := w.ReadParquetFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, newStacktraces(), sRead)
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
		w   = &ReadWriter[string, *StringPersister]{}
		buf bytes.Buffer
	)

	require.NoError(t, w.WriteParquetFile(&buf, s))

	sRead, err := w.ReadParquetFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, newStrings(), sRead)
}
