package golang

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStandardLibraryURL(t *testing.T) {
	for _, tt := range []struct {
		input      string
		expected   string
		expectedOk bool
	}{
		{
			input:      "github.com/grafana/grafana/pkg/querier/vcs.go",
			expected:   "",
			expectedOk: false,
		},
		{
			input:      "/usr/local/go/src/bufio/bufio.go",
			expected:   "https://raw.githubusercontent.com/golang/go/master/src/bufio/bufio.go",
			expectedOk: true,
		},
		{
			input:      "$GOROOT/src/unicode/utf8/utf8.go",
			expected:   "https://raw.githubusercontent.com/golang/go/master/src/unicode/utf8/utf8.go",
			expectedOk: true,
		},
		{
			input:      "fmt/scan.go",
			expected:   "https://raw.githubusercontent.com/golang/go/master/src/fmt/scan.go",
			expectedOk: true,
		},
		{
			input:      "$GOROOT/src/vendor/golang.org/x/crypto/cryptobyte/asn1.go",
			expected:   "https://raw.githubusercontent.com/golang/go/master/src/vendor/golang.org/x/crypto/cryptobyte/asn1.go",
			expectedOk: true,
		},
		{
			input:      "/usr/local/go/src/vendor/golang.org/x/net/http2/hpack/tables.go",
			expected:   "https://raw.githubusercontent.com/golang/go/master/src/vendor/golang.org/x/net/http2/hpack/tables.go",
			expectedOk: true,
		},
		{
			input:      "/usr/local/Cellar/go/1.21.3/libexec/src/runtime/netpoll_kqueue.go",
			expected:   "https://raw.githubusercontent.com/golang/go/go1.21.3/src/runtime/netpoll_kqueue.go",
			expectedOk: true,
		},
		{
			input:      "/opt/hostedtoolcache/go/1.21.6/x64/src/runtime/mgc.go",
			expected:   "https://raw.githubusercontent.com/golang/go/go1.21.6/src/runtime/mgc.go",
			expectedOk: true,
		},
	} {
		t.Run(tt.input, func(t *testing.T) {
			actual, ok := StandardLibraryURL(tt.input)
			if !tt.expectedOk {
				require.False(t, ok)
			}
			require.Equal(t, tt.expected, actual)
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
