package v1

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
)

// This test ensures that the structs that are stored and the used schema matches
func TestSchemaMatch(t *testing.T) {

	// TODO: Unfortunately the upstream schema doesn't correctly produce a
	// schema of a List of a struct pointer. This replaces this in the schema
	// comparison, because this has no affect to our construct/reconstruct code
	// we can simply replace the string in the schema.
	profilesStructSchema := strings.ReplaceAll(
		parquet.SchemaOf(&Profile{}).String(),
		"optional group element",
		"required group element",
	)

	require.Equal(t, profilesStructSchema, profilesSchema.String())

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

func newProfiles() []*Profile {
	return []*Profile{
		{
			ID:         uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			TimeNanos:  1001,
			SeriesRefs: []model.Fingerprint{0xaa, 0xab},
			Samples: []*Sample{
				{
					StacktraceID: 0xba,
					Values:       []int64{0xca, 0xcc},
					Labels:       []*profilev1.Label{},
				},
				{
					StacktraceID: 0xbb,
					Values:       []int64{0xca, 0xcc},
					Labels: []*profilev1.Label{
						{Key: 0xda, Str: 0xea},
					},
				},
			},
			Comments: []int64{},
		},
		{
			ID:         uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			SeriesRefs: []model.Fingerprint{0xab, 0xac},
			TimeNanos:  1002,
			Samples: []*Sample{
				{
					StacktraceID: 0xbc,
					Values:       []int64{0xcd, 0xce},
					Labels:       []*profilev1.Label{},
				},
			},
			Comments: []int64{},
		},
	}
}

func TestProfilesRoundTrip(t *testing.T) {
	var (
		p   = newProfiles()
		w   = &ReadWriter[*Profile, *ProfilePersister]{}
		buf bytes.Buffer
	)

	require.NoError(t, w.WriteParquetFile(&buf, p))

	sRead, err := w.ReadParquetFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, newProfiles(), sRead)
}
