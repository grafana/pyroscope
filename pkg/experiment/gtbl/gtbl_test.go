package gtbl

import (
	"bytes"
	"debug/elf"
	"fmt"
	"github.com/google/pprof/profile"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const alloy = "/home/korniltsev/alloy"
const alloyDebug = "/home/korniltsev/alloy.debug"

// const alloy = "/Users/marcsanmi/alloy"
//const alloy = "/Users/marcsanmi/work/96921110873602d0036300b40f9c61e9fad3d69d"

func TestName(t *testing.T) {
	e, err := elf.Open(alloy)
	require.NoError(t, err)
	symbols, err := e.Symbols()
	require.NoError(t, err)
	i := 0
	for _, symbol := range symbols {
		fmt.Printf("%16x | %6x %s\n", symbol.Value, symbol.Size, symbol.Name)
		i++
		if i > 100 {
			break
		}
	}
}

const gtblfile = "alloy.gtbl"

func TestCreateGtbl(t *testing.T) {
	executable, err := os.Open(alloy)
	require.NoError(t, err)
	defer executable.Close()
	output, err := os.Create(gtblfile)
	require.NoError(t, err)
	defer output.Close()
	err = createTable(t, executable, output)
	require.NoError(t, err)
}

//40b560 |     66 internal/abi.(*IntArgRegBitmap).Get

//
// 1. download debug file from debuginfod
// 2. convert it to gtbl << not sure how to do this once among multiple query-fronted
// 3. cache gtbl to objstore

// for the poc we measure FAT profile (from database, strip line symbols) lookup locally

// todo do not reinvent the wheel
type bufferCloser struct {
	bs  []byte
	off int64
}

func (b *bufferCloser) Read(p []byte) (n int, err error) {
	res, err := b.ReadAt(p, b.off)
	b.off += int64(res)
	return res, err
}

func (b *bufferCloser) ReadAt(p []byte, off int64) (n int, err error) {
	copy(p, b.bs[off:])
	return len(p), nil
}

func (b *bufferCloser) Close() error {
	return nil
}

func TestReadGtbl(t *testing.T) {
	bs, err := os.ReadFile(gtblfile)
	require.NoError(t, err)
	var f ReaderAtCloser = &bufferCloser{bs, 0}
	path, err := OpenReader(f)
	require.NoError(t, err)
	//lookup, err := path.Lookup(0x408ed0)
	lookup, err := path.Lookup(0x40b560)

	require.NoError(t, err)
	require.Len(t, lookup, 1)
	require.Equal(t, "internal/abi.(*IntArgRegBitmap).Get", lookup[0].FunctionName)
	defer path.Close()

}

func TestCreateRead(t *testing.T) {
	srcFiles := []string{
		alloy,
		alloyDebug,
	}
	expected := []struct {
		VA           uint64
		FunctionName string
	}{
		{0x48bfff, ""},
		{0x40b560, "internal/abi.(*IntArgRegBitmap).Get"},
		{0x408ed0, "_ZN8smallvec17SmallVec$LT$A$GT$21reserve_one_unchecked17h38e8e94dce0a375aE"},
		{0x408ed0, "_ZN8smallvec17SmallVec$LT$A$GT$21reserve_one_unchecked17h38e8e94dce0a375aE"},
		{0x8d46d38, "go.opentelemetry.io/ebpf-profiler/process.(*systemProcess).GetMappings"},
	}
	for _, file := range srcFiles {
		gtblPath := t.TempDir() + "/tem.gtbl"
		func() {
			dstf, err := os.Create(gtblPath)
			require.NoError(t, err)
			defer dstf.Close()

			srcf, err := os.Open(file)
			require.NoError(t, err)
			defer srcf.Close()

			err = createTable(t, srcf, dstf)
			require.NoError(t, err)
		}()
		tblf, err := os.Open(gtblPath)
		require.NoError(t, err)
		tbl, err := OpenReader(tblf)
		t.Cleanup(func() {
			tbl.Close()
		})
		require.NoError(t, err)

		t.Run(file, func(t *testing.T) {
			for _, e := range expected {
				t.Run(fmt.Sprintf("%x %s", e.VA, e.FunctionName), func(t *testing.T) {
					res, err := tbl.Lookup(e.VA)
					fname := ""
					if len(res) > 0 {
						fname = res[0].FunctionName
					}
					require.NoError(t, err)
					require.Equal(t, e.FunctionName, fname)
				})
			}
		})
	}
}

func createTable(t *testing.T, executable, output *os.File, opt ...Option) error {
	sb := newStringBuilder()
	rb := newRangesBuilder()
	lb := newLineTableBuilder()
	rc := &rangeCollector{sb: sb, rb: rb, lb: lb}
	for _, o := range opt {
		o(&rc.opt)
	}
	e, err := elf.NewFile(executable)
	require.NoError(t, err)

	symbols, err := e.Symbols()
	require.NoError(t, err)
	for _, symbol := range symbols {
		rc.VisitRange(&GoRange{
			VA:        symbol.Value,
			Length:    uint32(symbol.Size),
			Function:  symbol.Name,
			File:      "",
			CallFile:  "",
			CallLine:  0,
			Depth:     0,
			LineTable: nil,
		})
	}
	fmt.Printf("number of symbols %d\n", len(symbols))
	rb.sort()

	err2 := rc.write(output)
	if err2 != nil {
		return err2
	}

	return nil
}

func TestSymbolizeProfile(t *testing.T) {
	// Make sure you have a valid GTBL file first
	// If not, run TestCreateGtbl with the correct ELF file path

	// Open the GTBL file
	bs, err := os.ReadFile(gtblfile)
	require.NoError(t, err)
	var f ReaderAtCloser = &bufferCloser{bs, 0}
	symTable, err := OpenReader(f)
	require.NoError(t, err)
	defer symTable.Close()

	// Read the pprof file
	profileData, err := os.ReadFile("cleaned_profile.pb")
	require.NoError(t, err)

	// Parse the pprof file
	prof, err := profile.Parse(bytes.NewReader(profileData))
	require.NoError(t, err)

	// Loop through the samples and symbolize addresses
	for _, sample := range prof.Sample {
		for _, loc := range sample.Location {
			if loc.Mapping.File != "alloy" {
				continue
			}
			addr := loc.Address
			symbols, err := symTable.Lookup(addr)
			if err == nil && len(symbols) > 0 {
				//t.Logf("Symbolized 0x%x to %s", addr, symbols[0].FunctionName)
				loc.Line = []profile.Line{{Function: &profile.Function{Name: symbols[0].FunctionName}}}
			} else {
				t.Logf("Could not symbolize 0x%x: %v", addr, err)
			}
		}
	}
	fmt.Printf("locations %d\n", len(prof.Location))

	var buf bytes.Buffer
	err = prof.Write(&buf)
	require.NoError(t, err)
	err = os.WriteFile("symbolized_profile.pb", buf.Bytes(), 0644)
	require.NoError(t, err)

	err = os.WriteFile("symbolized_profile.txt", []byte(prof.String()), 0644)
	require.NoError(t, err)
}
