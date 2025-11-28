package golang

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsStandardLibraryPath(t *testing.T) {
	for _, tt := range []struct {
		input           string
		expectedPath    string
		expectedVersion string
		expectedOk      bool
	}{
		{
			input:      "github.com/grafana/grafana/pkg/frontend/vcs.go",
			expectedOk: false,
		},
		{
			input:        "/usr/local/go/src/bufio/bufio.go",
			expectedPath: "bufio/bufio.go",
			expectedOk:   true,
		},
		{
			input:        "$GOROOT/src/unicode/utf8/utf8.go",
			expectedPath: "unicode/utf8/utf8.go",
			expectedOk:   true,
		},
		{
			input:        "fmt/scan.go",
			expectedPath: "fmt/scan.go",
			expectedOk:   true,
		},
		{
			input:        "$GOROOT/src/vendor/golang.org/x/crypto/cryptobyte/asn1.go",
			expectedPath: "vendor/golang.org/x/crypto/cryptobyte/asn1.go",
			expectedOk:   true,
		},
		{
			input:        "/usr/local/go/src/vendor/golang.org/x/net/http2/hpack/tables.go",
			expectedPath: "vendor/golang.org/x/net/http2/hpack/tables.go",
			expectedOk:   true,
		},
		{
			input:           "/usr/local/Cellar/go/1.21.3/libexec/src/runtime/netpoll_kqueue.go",
			expectedPath:    "runtime/netpoll_kqueue.go",
			expectedVersion: "1.21.3",
			expectedOk:      true,
		},
		{
			input:           "/opt/hostedtoolcache/go/1.21.6/x64/src/runtime/mgc.go",
			expectedPath:    "runtime/mgc.go",
			expectedVersion: "1.21.6",
			expectedOk:      true,
		},
		{
			input:           "/Users/pyroscope/.golang/packages/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.6.darwin-arm64/src/runtime/proc.go",
			expectedPath:    "runtime/proc.go",
			expectedVersion: "1.24.6",
			expectedOk:      true,
		},
		{
			input:           "/Users/christian/.golang/packages/pkg/mod/golang.org/toolchain@v0.0.1-go1.25rc1.darwin-arm64/src/runtime/type.go",
			expectedPath:    "runtime/type.go",
			expectedVersion: "1.25rc1",
			expectedOk:      true,
		},
	} {
		t.Run(tt.input, func(t *testing.T) {
			actualPath, actualVersion, ok := IsStandardLibraryPath(tt.input)
			if !tt.expectedOk {
				require.False(t, ok)
			}
			require.Equal(t, tt.expectedPath, actualPath)
			require.Equal(t, tt.expectedVersion, actualVersion)
		})
	}
}

func TestVendorRelativePath(t *testing.T) {
	for _, tt := range []struct {
		in         string
		expected   string
		expectedOk bool
	}{
		{
			in:         "/drone/src/vendor/google.golang.org/protobuf/proto/merge.go",
			expected:   "/vendor/google.golang.org/protobuf/proto/merge.go",
			expectedOk: true,
		},
		{
			in:         "google.golang.org/protobuf/proto/merge.go",
			expected:   "",
			expectedOk: false,
		},
	} {
		t.Run(tt.in, func(t *testing.T) {
			actual, ok := VendorRelativePath(tt.in)
			if !tt.expectedOk {
				require.False(t, ok)
			}
			require.Equal(t, tt.expected, actual)
		})
	}
}
