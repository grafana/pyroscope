package featureflags

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parseClientCapabilities(t *testing.T) {
	tests := []struct {
		Name         string
		Header       http.Header
		Want         ClientCapabilities
		WantError    bool
		ErrorMessage string
	}{
		{
			Name:   "empty header returns default capabilities",
			Header: http.Header{},
			Want:   ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "no Accept header returns default capabilities",
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "empty Accept header value returns default capabilities",
			Header: http.Header{
				"Accept": []string{""},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "simple Accept header without capabilities",
			Header: http.Header{
				"Accept": []string{"application/json"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "Accept header with utf8 label names capability true",
			Header: http.Header{
				"Accept": []string{"*/*; allow-utf8-labelnames=true"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "Accept header with utf8 label names capability false",
			Header: http.Header{
				"Accept": []string{"*/*; allow-utf8-labelnames=false"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "Accept header with utf8 label names capability invalid value",
			Header: http.Header{
				"Accept": []string{"*/*; allow-utf8-labelnames=invalid"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "Accept header with unknown capability",
			Header: http.Header{
				"Accept": []string{"*/*; unknown-capability=true"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "Accept header with multiple capabilities",
			Header: http.Header{
				"Accept": []string{"*/*; allow-utf8-labelnames=true; unknown-capability=false"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "multiple Accept header values",
			Header: http.Header{
				"Accept": []string{"application/json", "*/*; allow-utf8-labelnames=true"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "multiple Accept header values with different capabilities",
			Header: http.Header{
				"Accept": []string{
					"application/json; allow-utf8-labelnames=false",
					"*/*; allow-utf8-labelnames=true",
				},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "Accept header with quality values",
			Header: http.Header{
				"Accept": []string{"text/html; q=0.9; allow-utf8-labelnames=true"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "complex Accept header",
			Header: http.Header{
				"Accept": []string{
					"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8;allow-utf8-labelnames=true",
				},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "multiple Accept header entries",
			Header: http.Header{
				"Accept": []string{
					"application/json",
					"text/plain; allow-utf8-labelnames=true",
					"*/*; q=0.1",
				},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "invalid mime type in Accept header",
			Header: http.Header{
				"Accept": []string{"invalid/mime/type/format"},
			},
			Want:         ClientCapabilities{},
			WantError:    true,
			ErrorMessage: "mime: unexpected content after media subtype",
		},
		{
			Name: "Accept header with invalid syntax",
			Header: http.Header{
				"Accept": []string{"text/html; invalid-parameter-syntax"},
			},
			Want:         ClientCapabilities{},
			WantError:    true,
			ErrorMessage: "mime: invalid media parameter",
		},
		{
			Name: "mixed valid and invalid Accept header values",
			Header: http.Header{
				"Accept": []string{
					"application/json",
					"invalid/mime/type/format",
				},
			},
			Want:         ClientCapabilities{},
			WantError:    true,
			ErrorMessage: "mime: unexpected content after media subtype",
		},
		{
			// Parameter names are case-insensitive in mime.ParseMediaType
			Name: "case sensitivity test for capability name",
			Header: http.Header{
				"Accept": []string{"*/*; Allow-Utf8-Labelnames=true"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "whitespace handling in Accept header",
			Header: http.Header{
				"Accept": []string{" application/json ; allow-utf8-labelnames=true "},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			got, err := parseClientCapabilities(tt.Header)

			if tt.WantError {
				require.Error(t, err)
				if tt.ErrorMessage != "" {
					require.Contains(t, err.Error(), tt.ErrorMessage)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.Want, got)
		})
	}
}

func Test_parseClientCapabilities_MultipleCapabilities(t *testing.T) {
	// This test specifically checks that when the same capability appears
	// multiple times with different values, the last "true" value wins
	tests := []struct {
		Name   string
		Header http.Header
		Want   ClientCapabilities
	}{
		{
			Name: "capability appears multiple times - last true wins",
			Header: http.Header{
				"Accept": []string{
					"application/json; allow-utf8-labelnames=false",
					"text/plain; allow-utf8-labelnames=true",
				},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "capability appears multiple times - last false loses to earlier true",
			Header: http.Header{
				"Accept": []string{
					"application/json; allow-utf8-labelnames=true",
					"text/plain; allow-utf8-labelnames=false",
				},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "capability appears multiple times - all false",
			Header: http.Header{
				"Accept": []string{
					"application/json; allow-utf8-labelnames=false",
					"text/plain; allow-utf8-labelnames=false",
				},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			got, err := parseClientCapabilities(tt.Header)
			require.NoError(t, err)
			require.Equal(t, tt.Want, got)
		})
	}
}
