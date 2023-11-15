package python

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGlibcVersions(t *testing.T) {
	testdata := []struct {
		path    string
		version Version
	}{
		{"../testdata/glibc-arm64/glibc-2.31/libc.so.6", Version{2, 31, 0}},
		{"../testdata/glibc-arm64/glibc-2.36/libc.so.6", Version{2, 36, 0}},
		{"../testdata/glibc-arm64/glibc-2.29/libc.so.6", Version{2, 29, 0}},
		{"../testdata/glibc-arm64/glibc-2.37/libc.so.6", Version{2, 37, 0}},
		{"../testdata/glibc-arm64/glibc-2.34/libc.so.6", Version{2, 34, 0}},
		{"../testdata/glibc-arm64/glibc-2.32/libc.so.6", Version{2, 32, 0}},
		{"../testdata/glibc-arm64/glibc-2.27/libc.so.6", Version{2, 27, 0}},
		{"../testdata/glibc-arm64/glibc-2.28/libc.so.6", Version{2, 28, 0}},
		{"../testdata/glibc-arm64/glibc-2.35/libc.so.6", Version{2, 35, 0}},
		{"../testdata/glibc-arm64/glibc-2.30/libc.so.6", Version{2, 30, 0}},
		{"../testdata/glibc-arm64/glibc-2.33/libc.so.6", Version{2, 33, 0}},
		{"../testdata/glibc-arm64/glibc-2.38/libc.so.6", Version{2, 38, 0}},
		{"../testdata/glibc-x64/glibc-2.31/libc.so.6", Version{2, 31, 0}},
		{"../testdata/glibc-x64/glibc-2.36/libc.so.6", Version{2, 36, 0}},
		{"../testdata/glibc-x64/glibc-2.29/libc.so.6", Version{2, 29, 0}},
		{"../testdata/glibc-x64/glibc-2.37/libc.so.6", Version{2, 37, 0}},
		{"../testdata/glibc-x64/glibc-2.34/libc.so.6", Version{2, 34, 0}},
		{"../testdata/glibc-x64/glibc-2.32/libc.so.6", Version{2, 32, 0}},
		{"../testdata/glibc-x64/glibc-2.27/libc.so.6", Version{2, 27, 0}},
		{"../testdata/glibc-x64/glibc-2.28/libc.so.6", Version{2, 28, 0}},
		{"../testdata/glibc-x64/glibc-2.35/libc.so.6", Version{2, 35, 0}},
		{"../testdata/glibc-x64/glibc-2.30/libc.so.6", Version{2, 30, 0}},
		{"../testdata/glibc-x64/glibc-2.33/libc.so.6", Version{2, 33, 0}},
		{"../testdata/glibc-x64/glibc-2.38/libc.so.6", Version{2, 38, 0}},
	}
	for _, testdatum := range testdata {
		version, err := GetGlibcVersionFromFile(testdatum.path)
		assert.NoError(t, err)
		assert.Equal(t, testdatum.version, version)

	}
}

func TestGlibcGuess(t *testing.T) {
	orig := glibcOffsets
	defer func() {
		glibcOffsets = orig
	}()
	glibcOffsets = map[Version]*GlibcOffsets{}
	v30 := Version{2, 30, 0}
	glibcOffsets[v30] = orig[v30]
	expected := PerfGlibc{
		PthreadSize:             orig[v30].PthreadSize,
		PthreadSpecific1stblock: orig[v30].PthreadSpecific1stblock,
	}
	offsets, guess, err := GetGlibcOffsets(v30)
	assert.NoError(t, err)
	assert.False(t, guess)
	assert.Equal(t, expected, offsets)

	offsets, guess, err = GetGlibcOffsets(Version{2, 31, 0})
	assert.NoError(t, err)
	assert.True(t, guess)
	assert.Equal(t, expected, offsets)
	_, _, err = GetGlibcOffsets(Version{2, 27, 0})
	assert.Error(t, err)
}

func TestKeyData(t *testing.T) {
	for version, offsets := range glibcOffsets {
		assert.Equal(t, int16(8), offsets.PthreadKeyDataData, version.String())
		assert.Equal(t, int16(16), offsets.PthreadKeyDataSize, version.String())
	}
}
