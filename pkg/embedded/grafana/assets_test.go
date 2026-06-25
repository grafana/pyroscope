package grafana

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClearPath(t *testing.T) {
	t.Parallel()

	destPath := t.TempDir()

	tests := []struct {
		name            string
		entryName       string
		stripComponents int
		wantSuffix      string // expected suffix relative to destPath
		wantErr         bool
		errContains     string
	}{
		{
			name:            "simple file",
			entryName:       "dir/file.txt",
			stripComponents: 0,
			wantSuffix:      "dir/file.txt",
		},
		{
			name:            "strip one component",
			entryName:       "prefix/dir/file.txt",
			stripComponents: 1,
			wantSuffix:      "dir/file.txt",
		},
		{
			// When stripComponents >= len(parts), len(list) > stripComponents is false
			// so the list is left untouched and the full path is appended.
			name:            "strip equal to component count keeps full path",
			entryName:       "a/b",
			stripComponents: 2,
			wantSuffix:      "a/b",
		},
		{
			name:            "path traversal via ..",
			entryName:       "../../etc/passwd",
			stripComponents: 0,
			wantErr:         true,
			errContains:     "escapes destination directory",
		},
		{
			name:            "path traversal after strip",
			entryName:       "prefix/../../../etc/passwd",
			stripComponents: 1,
			wantErr:         true,
			errContains:     "escapes destination directory",
		},
		{
			// /etc/passwd is split by FieldsFunc into ["etc","passwd"] (leading separator
			// consumed), then joined under destPath — result is destPath/etc/passwd.
			name:            "absolute path is confined to destPath",
			entryName:       "/etc/passwd",
			stripComponents: 0,
			wantSuffix:      "etc/passwd",
		},
		{
			name:            "nested directory within dest",
			entryName:       "a/b/c/file.txt",
			stripComponents: 0,
			wantSuffix:      "a/b/c/file.txt",
		},
		{
			name:            "traversal with mixed separators",
			entryName:       "good/../../../evil",
			stripComponents: 0,
			wantErr:         true,
			errContains:     "escapes destination directory",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := clearPath(tt.entryName, destPath, tt.stripComponents)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			want := filepath.Join(destPath, filepath.FromSlash(tt.wantSuffix))
			assert.Equal(t, want, got)
		})
	}
}

type zipEntry struct {
	name    string
	content string
}

func buildZip(t *testing.T, entries []zipEntry) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		if strings.HasSuffix(e.name, "/") || e.content == "" && strings.Contains(e.name, "/") && !strings.Contains(filepath.Base(e.name), ".") {
			_, err := w.Create(e.name)
			require.NoError(t, err)
		} else {
			f, err := w.Create(e.name)
			require.NoError(t, err)
			_, err = f.Write([]byte(e.content))
			require.NoError(t, err)
		}
	}
	require.NoError(t, w.Close())
	return bytes.NewReader(buf.Bytes())
}

func TestExtractZip_Normal(t *testing.T) {
	dest := t.TempDir()
	r := buildZip(t, []zipEntry{
		{name: "prefix/subdir/hello.txt", content: "hello"},
		{name: "prefix/world.txt", content: "world"},
	})
	err := extractZip(r, int64(r.Len()), dest, 1 /* strip "prefix" */)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dest, "subdir", "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))

	data, err = os.ReadFile(filepath.Join(dest, "world.txt"))
	require.NoError(t, err)
	assert.Equal(t, "world", string(data))
}

func TestExtractZip_NoStrip(t *testing.T) {
	dest := t.TempDir()
	r := buildZip(t, []zipEntry{
		{name: "file.txt", content: "content"},
	})
	err := extractZip(r, int64(r.Len()), dest, 0)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dest, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(data))
}

func TestExtractZip_PathTraversal(t *testing.T) {
	dest := t.TempDir()
	escaped := filepath.Join(dest, "..", "..", "evil.txt")
	t.Cleanup(func() { os.Remove(escaped) })

	r := buildZip(t, []zipEntry{
		{name: "../../evil.txt", content: "malicious"},
	})
	err := extractZip(r, int64(r.Len()), dest, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes destination directory")

	_, statErr := os.Stat(escaped)
	assert.True(t, os.IsNotExist(statErr), "evil.txt must not be written outside destPath")
}

type tarEntry struct {
	name    string
	content string
	isDir   bool
}

func buildTarGz(t *testing.T, entries []tarEntry) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for _, e := range entries {
		if e.isDir {
			hdr := &tar.Header{
				Name:     e.name,
				Typeflag: tar.TypeDir,
				Mode:     0755,
			}
			require.NoError(t, tw.WriteHeader(hdr))
		} else {
			hdr := &tar.Header{
				Name:     e.name,
				Typeflag: tar.TypeReg,
				Mode:     0644,
				Size:     int64(len(e.content)),
			}
			require.NoError(t, tw.WriteHeader(hdr))
			_, err := tw.Write([]byte(e.content))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return bytes.NewReader(buf.Bytes())
}

func TestExtractTarGz_Normal(t *testing.T) {
	dest := t.TempDir()
	r := buildTarGz(t, []tarEntry{
		{name: "prefix/", isDir: true},
		{name: "prefix/hello.txt", content: "hello"},
	})
	err := extractTarGz(r, dest, 1 /* strip "prefix" */)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dest, "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestExtractTarGz_NestedDirs(t *testing.T) {
	dest := t.TempDir()
	r := buildTarGz(t, []tarEntry{
		{name: "a/", isDir: true},
		{name: "a/b/", isDir: true},
		{name: "a/b/file.txt", content: "deep"},
	})
	err := extractTarGz(r, dest, 0)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dest, "a", "b", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "deep", string(data))
}

func TestExtractTarGz_PathTraversal(t *testing.T) {
	dest := t.TempDir()
	escaped := filepath.Join(dest, "..", "..", "evil.txt")
	t.Cleanup(func() { os.Remove(escaped) })

	r := buildTarGz(t, []tarEntry{
		{name: "../../evil.txt", content: "malicious"},
	})
	err := extractTarGz(r, dest, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes destination directory")

	_, statErr := os.Stat(escaped)
	assert.True(t, os.IsNotExist(statErr), "evil.txt must not be written outside destPath")
}

func TestExtractTarGz_SlipAfterStrip(t *testing.T) {
	dest := t.TempDir()
	r := buildTarGz(t, []tarEntry{
		// After stripping "prefix", the effective path becomes ../../evil.txt
		{name: "prefix/../../evil.txt", content: "bad"},
	})
	err := extractTarGz(r, dest, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes destination directory")
}
