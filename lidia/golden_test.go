package lidia

import (
	"archive/zip"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var optionSets = []struct {
	name string
	opts []Option
}{
	{"default", nil},
	{"files+lines", []Option{WithFiles(), WithLines()}},
	{"symtab-only", []Option{WithParseGoPclntab(false)}},
}

type testBinary struct {
	name, path string
}

// goldenSHA256 is the SHA-256 of CreateLidiaFromELF output, keyed by binary
// and option set. Regenerate it when the file format changes on purpose.
var goldenSHA256 = map[string]string{
	"test-binary/default":     "2901002e63fcf49983e67dfb9950029d88b16a50f47fd3597cc81eac20068b8c",
	"test-binary/files+lines": "2901002e63fcf49983e67dfb9950029d88b16a50f47fd3597cc81eac20068b8c",
	"test-binary/symtab-only": "773ee683932b0e4084dec16c2f427442b6c7d366229f47a960f01c35f50fe9ca",
	"libfib.so/default":       "b1827a94251231bcd99d415a6ca0bbd30a44921533205b028acc8cbb69471fbe",
	"libfib.so/files+lines":   "b1827a94251231bcd99d415a6ca0bbd30a44921533205b028acc8cbb69471fbe",
	"libfib.so/symtab-only":   "b1827a94251231bcd99d415a6ca0bbd30a44921533205b028acc8cbb69471fbe",
}

func TestCreateLidiaFromELFGolden(t *testing.T) {
	testGolden(t, []testBinary{
		{"test-binary", unzipTestBinary(t)},
		{"libfib.so", "testdata/libfib.so"},
	}, goldenSHA256)
}

func testGolden(t *testing.T, binaries []testBinary, golden map[string]string) {
	t.Helper()
	for _, b := range binaries {
		for _, o := range optionSets {
			t.Run(b.name+"/"+o.name, func(t *testing.T) {
				want, ok := golden[b.name+"/"+o.name]
				require.True(t, ok, "no golden hash")

				f, err := elf.Open(b.path)
				require.NoError(t, err)
				defer f.Close()

				got, err := createBytes(f, CreateLidiaFromELF, o.opts)
				require.NoError(t, err)
				sum := sha256.Sum256(got)
				require.Equal(t, want, hex.EncodeToString(sum[:]))
			})
		}
	}
}

func createBytes(f *elf.File, create func(*elf.File, io.WriteSeeker, ...Option) error, opts []Option) ([]byte, error) {
	out, err := os.CreateTemp("", "lidia")
	if err != nil {
		return nil, err
	}
	defer os.Remove(out.Name())
	defer out.Close()
	if err := create(f, out, opts...); err != nil {
		return nil, err
	}
	return os.ReadFile(out.Name())
}

func unzipTestBinary(t *testing.T) string {
	t.Helper()
	zr, err := zip.OpenReader("testdata/test-binary.zip")
	require.NoError(t, err)
	defer zr.Close()

	r, err := zr.Open("test-binary")
	require.NoError(t, err)
	defer r.Close()
	b, err := io.ReadAll(r)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "test-binary")
	require.NoError(t, os.WriteFile(path, b, 0o755))
	return path
}
