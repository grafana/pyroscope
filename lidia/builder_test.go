package lidia

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/lidia/gosym"
)

// mapStringBuilder deduplicates strings by keying a map with them, the
// reference output for stringBuilder.
type mapStringBuilder struct {
	buf    []byte
	unique map[string]stringOffset
	offset stringOffset
}

func (sb *mapStringBuilder) add(s string) stringOffset {
	if prev, exists := sb.unique[s]; exists {
		return prev
	}
	sb.buf = binary.LittleEndian.AppendUint32(sb.buf, uint32(len(s)))
	sb.buf = append(sb.buf, s...)
	offset := sb.offset
	sb.unique[s] = offset
	sb.offset = stringOffset(uint64(sb.offset) + uint64(4+len(s)))
	return offset
}

func TestStringBuilderMatchesMap(t *testing.T) {
	type input struct {
		name    string
		strings []string
	}
	inputs := []input{{"generated", generatedStrings()}}
	binaries := []testBinary{
		{"test-binary", unzipTestBinary(t)},
		{"libfib.so", "testdata/libfib.so"},
	}
	if runtime.GOOS == "linux" {
		binaries = append(binaries, testBinary{"self", "/proc/self/exe"})
	}
	for _, b := range binaries {
		inputs = append(inputs, input{b.name, binaryStrings(t, b.path)})
	}

	hashes := []struct {
		name string
		hash func(string) uint64
	}{
		{"maphash", nil},
		{"constant", func(string) uint64 { return 0 }},
		{"by-length", func(s string) uint64 { return uint64(len(s)) }},
	}

	for _, in := range inputs {
		for _, h := range hashes {
			t.Run(in.name+"/"+h.name, func(t *testing.T) {
				want := &mapStringBuilder{unique: make(map[string]stringOffset)}
				want.add("")
				want.add("[overflow]")

				sb := newStringBuilder()
				add := sb.add
				if h.hash != nil {
					sb = &stringBuilder{unique: make(map[uint64]stringOffset)}
					add = func(s string) stringOffset { return sb.addHashed(s, h.hash(s)) }
					sb.emptystr = add("")
					sb.overflow = add("[overflow]")
				}

				for i, s := range in.strings {
					require.Equal(t, want.add(s), add(s), "offset of string %d %q", i, s)
				}

				var got bytes.Buffer
				require.NoError(t, sb.writeTo(&got))
				require.Equal(t, uint64(want.offset), uint64(sb.size))
				require.True(t, bytes.Equal(want.buf, got.Bytes()), "string table differs")
			})
		}
	}
}

// generatedStrings returns strings with many repeats, including ones larger
// than a chunk and ones that end exactly on a chunk boundary.
func generatedStrings() []string {
	rnd := rand.New(rand.NewSource(1))
	var res []string
	for range 20000 {
		res = append(res, fmt.Sprintf("func_%d", rnd.Intn(5000)))
	}
	res = append(res, "", "[overflow]", "func_1")
	for _, n := range []int{stringChunkSize - 4, stringChunkSize - 3, stringChunkSize, 3 * stringChunkSize} {
		res = append(res, strings.Repeat("x", n), strings.Repeat("y", n), strings.Repeat("x", n))
	}
	for range 1000 {
		res = append(res, strings.Repeat("z", rnd.Intn(64<<10)))
	}
	return res
}

// binaryStrings returns the function names in the symbol tables and Go
// pclntab of path, each followed by the two empty file names that
// rangeCollector adds for it with WithFiles.
func binaryStrings(t *testing.T, path string) []string {
	t.Helper()
	f, err := elf.Open(path)
	require.NoError(t, err)
	defer f.Close()

	var res []string
	for _, load := range []func() ([]elf.Symbol, error){f.Symbols, f.DynamicSymbols} {
		symbols, err := load()
		if err != nil {
			continue
		}
		for _, s := range symbols {
			if elf.ST_TYPE(s.Info) == elf.STT_FUNC && s.Name != "" {
				res = append(res, s.Name, "", "")
			}
		}
	}
	functions, err := gosym.GoFunctions(f)
	require.NoError(t, err)
	for _, fn := range functions {
		res = append(res, fn.Name, "", "")
	}
	require.NotEmpty(t, res)
	return res
}
