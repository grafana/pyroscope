package main

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_AcceptHeader(t *testing.T) {
	tests := []struct {
		Name               string
		Header             http.Header
		ClientCapabilities []string
		Want               []string
	}{
		{
			Name:   "empty header adds capability",
			Header: http.Header{},
			ClientCapabilities: []string{
				"allow-utf8-labelnames=true",
			},
			Want: []string{"*/*;allow-utf8-labelnames=true"},
		},
		{
			Name: "existing header appends capability",
			Header: http.Header{
				"Accept": []string{"application/json"},
			},
			ClientCapabilities: []string{
				"allow-utf8-labelnames=true",
			},
			Want: []string{"application/json", "*/*;allow-utf8-labelnames=true"},
		},
		{
			Name: "multiple existing values appends capability",
			Header: http.Header{
				"Accept": []string{"application/json", "text/plain"},
			},
			ClientCapabilities: []string{
				"allow-utf8-labelnames=true",
			},
			Want: []string{"application/json", "text/plain", "*/*;allow-utf8-labelnames=true"},
		},
		{
			Name: "existing capability is not duplicated",
			Header: http.Header{
				"Accept": []string{"*/*;allow-utf8-labelnames=true"},
			},
			ClientCapabilities: []string{
				"allow-utf8-labelnames=true",
			},
			Want: []string{"*/*;allow-utf8-labelnames=true"},
		},
		{
			Name: "multiple client capabilities appends capability",
			Header: http.Header{
				"Accept": []string{"*/*;allow-utf8-labelnames=true"},
			},
			ClientCapabilities: []string{
				"allow-utf8-labelnames=true",
				"capability2=false",
			},
			Want: []string{"*/*;allow-utf8-labelnames=true", "*/*;capability2=false"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			req, _ := http.NewRequest("GET", "example.com", nil)
			req.Header = tt.Header
			clientCapabilities := tt.ClientCapabilities

			addClientCapabilitiesHeader(req, acceptHeaderMimeType, clientCapabilities)
			require.Equal(t, tt.Want, req.Header.Values("Accept"))
		})
	}
}

func TestAddTraceparentHeader(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest("GET", "http://example.com", nil)
	require.NoError(t, err)

	addTraceparentHeader(req)

	traceparent := req.Header.Get(traceparentHeader)
	require.Regexp(t, regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-01$`), traceparent)
}

func TestAddTraceparentHeader_PreservesExistingHeader(t *testing.T) {
	t.Parallel()

	const existingTraceparent = "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"
	req, err := http.NewRequest("GET", "http://example.com", nil)
	require.NoError(t, err)
	req.Header.Set(traceparentHeader, existingTraceparent)

	addTraceparentHeader(req)

	require.Equal(t, existingTraceparent, req.Header.Get(traceparentHeader))
}
