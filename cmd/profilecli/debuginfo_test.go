package main

import (
	"bytes"
	"context"
	"debug/elf"
	"encoding/binary"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	debuginfov1alpha1connect "github.com/grafana/pyroscope/api/gen/proto/go/debuginfo/v1alpha1/debuginfov1alpha1connect"
	"github.com/grafana/pyroscope/v2/pkg/debuginfo"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

// makeTestELF writes a minimal 64-bit ELF with a .note.gnu.build-id section
// containing buildID, and returns its path. Uses debug/elf's exported header
// types so we don't have to lay out raw bytes by hand.
func makeTestELF(t *testing.T, buildID []byte) string {
	t.Helper()

	// Note payload: namesz | descsz | type | "GNU\0" | buildID | pad to 4.
	// Errors from binary.Write to a bytes.Buffer are impossible, so we ignore them.
	var note bytes.Buffer
	_ = binary.Write(&note, binary.LittleEndian, uint32(4))            // namesz: len("GNU\0")
	_ = binary.Write(&note, binary.LittleEndian, uint32(len(buildID))) // descsz
	_ = binary.Write(&note, binary.LittleEndian, uint32(3))            // NT_GNU_BUILD_ID
	note.WriteString("GNU\x00")
	note.Write(buildID)
	for note.Len()%4 != 0 {
		note.WriteByte(0)
	}

	shstrtab := []byte("\x00.note.gnu.build-id\x00.shstrtab\x00")
	const ehdrSz, phdrSz, shdrSz = 64, 56, 64
	noteOff := uint64(ehdrSz + phdrSz)
	shstrOff := noteOff + uint64(note.Len())
	shdrOff := shstrOff + uint64(len(shstrtab))

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, elf.Header64{
		Ident:     [16]byte{0x7f, 'E', 'L', 'F', 2 /*64-bit*/, 1 /*LE*/, 1 /*ver*/},
		Type:      uint16(elf.ET_EXEC),
		Machine:   uint16(elf.EM_X86_64),
		Version:   1,
		Phoff:     ehdrSz,
		Shoff:     shdrOff,
		Ehsize:    ehdrSz,
		Phentsize: phdrSz,
		Phnum:     1,
		Shentsize: shdrSz,
		Shnum:     3, // null, .note.gnu.build-id, .shstrtab
		Shstrndx:  2,
	})
	_ = binary.Write(&buf, binary.LittleEndian, elf.Prog64{
		Type:   uint32(elf.PT_NOTE),
		Flags:  uint32(elf.PF_R),
		Off:    noteOff,
		Filesz: uint64(note.Len()),
		Memsz:  uint64(note.Len()),
		Align:  4,
	})
	buf.Write(note.Bytes())
	buf.Write(shstrtab)

	// Three section headers: null, .note.gnu.build-id, .shstrtab.
	_ = binary.Write(&buf, binary.LittleEndian, elf.Section64{})
	_ = binary.Write(&buf, binary.LittleEndian, elf.Section64{
		Name: 1, Type: uint32(elf.SHT_NOTE),
		Off: noteOff, Size: uint64(note.Len()), Addralign: 4,
	})
	_ = binary.Write(&buf, binary.LittleEndian, elf.Section64{
		Name: 20, Type: uint32(elf.SHT_STRTAB),
		Off: shstrOff, Size: uint64(len(shstrtab)), Addralign: 1,
	})

	path := filepath.Join(t.TempDir(), "test.elf")
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
	return path
}

func startDebuginfoTestServer(t *testing.T, enabled bool) *httptest.Server {
	t.Helper()
	store, err := debuginfo.NewStore(log.NewNopLogger(), memory.NewInMemBucket(), debuginfo.Config{
		Enabled:           enabled,
		MaxUploadSize:     100 * 1024 * 1024,
		UploadStalePeriod: time.Minute,
	})
	require.NoError(t, err)

	router := mux.NewRouter()
	debuginfov1alpha1connect.RegisterDebuginfoServiceHandler(
		router, store,
		connect.WithInterceptors(tenant.NewAuthInterceptor(true)),
	)
	router.Handle(
		"/debuginfo.v1alpha1.DebuginfoService/Upload/{gnu_build_id}",
		util.AuthenticateUser(true).Wrap(store.UploadHTTPHandler()),
	).Methods("POST")

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func TestExtractGnuBuildId(t *testing.T) {
	t.Parallel()

	id := []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
		0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	got, err := extractGnuBuildId(makeTestELF(t, id))
	require.NoError(t, err)
	assert.Equal(t, "deadbeef0102030405060708090a0b0c0d0e0f10", got)

	_, err = extractGnuBuildId("/nonexistent")
	require.Error(t, err)
}

// TestExtractGnuBuildId_Truncated checks that a .note.gnu.build-id section
// with a descsz larger than the actual data returns an error instead of
// panicking on out-of-bounds slicing.
func TestExtractGnuBuildId_Truncated(t *testing.T) {
	t.Parallel()

	// Build an ELF with a valid build ID, then patch the note's descsz field
	// to claim a much larger payload than is actually present.
	path := makeTestELF(t, []byte{0xaa, 0xbb, 0xcc, 0xdd})
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	// Note payload starts at ehdrSz+phdrSz = 64+56 = 120; descsz is at offset +4.
	const noteOff = 120
	binary.LittleEndian.PutUint32(data[noteOff+4:], 0xffff)
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, err = extractGnuBuildId(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "truncated")
}

func TestUploadDebuginfo(t *testing.T) {
	t.Parallel()
	srv := startDebuginfoTestServer(t, true)
	id := []byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

	params := &debuginfoUploadParams{
		path:         makeTestELF(t, id),
		fileType:     "executable-full",
		phlareClient: &phlareClient{URL: srv.URL, TenantID: "t1"},
	}

	// First upload succeeds, second is a no-op (server says it already has it).
	require.NoError(t, uploadDebuginfo(context.Background(), params))
	require.NoError(t, uploadDebuginfo(context.Background(), params))
}

func TestUploadDebuginfo_Disabled(t *testing.T) {
	t.Parallel()
	srv := startDebuginfoTestServer(t, false)
	id := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}

	err := uploadDebuginfo(context.Background(), &debuginfoUploadParams{
		path:         makeTestELF(t, id),
		fileType:     "executable-full",
		phlareClient: &phlareClient{URL: srv.URL, TenantID: "t2"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}

func TestUploadDebuginfo_NotAnELF(t *testing.T) {
	t.Parallel()
	srv := startDebuginfoTestServer(t, true)

	bogus := filepath.Join(t.TempDir(), "not-an-elf")
	require.NoError(t, os.WriteFile(bogus, []byte("not elf"), 0o644))

	err := uploadDebuginfo(context.Background(), &debuginfoUploadParams{
		path:         bogus,
		fileType:     "executable-full",
		phlareClient: &phlareClient{URL: srv.URL, TenantID: "t3"},
	})
	require.Error(t, err)
}
