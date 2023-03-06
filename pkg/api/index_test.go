// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/api/handlers_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndexHandlerPrefix(t *testing.T) {
	c := NewIndexPageContent()
	c.AddLinks(DefaultWeight, "Store Gateway", []IndexPageLink{{Desc: "Ring status", Path: "/store-gateway/ring"}})

	for _, tc := range []struct {
		prefix    string
		toBeFound string
	}{
		{prefix: "", toBeFound: "<a href=\"/store-gateway/ring\">"},
		{prefix: "/test", toBeFound: "<a href=\"/test/store-gateway/ring\">"},
		// All the extra slashed are cleaned up in the result.
		{prefix: "///test///", toBeFound: "<a href=\"/test/store-gateway/ring\">"},
	} {
		h := IndexHandler(tc.prefix, c)

		req := httptest.NewRequest("GET", "/", nil)
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, req)

		require.Equal(t, 200, resp.Code)
		require.True(t, strings.Contains(resp.Body.String(), tc.toBeFound))
	}
}

func TestIndexPageContent(t *testing.T) {
	c := NewIndexPageContent()
	c.AddLinks(DefaultWeight, "Some group", []IndexPageLink{
		{Desc: "Some link", Path: "/store-gateway/ring"},
		{Dangerous: true, Desc: "Boom!", Path: "/store-gateway/boom"},
	})

	h := IndexHandler("", c)

	req := httptest.NewRequest("GET", "/", nil)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	require.Equal(t, 200, resp.Code)
	require.True(t, strings.Contains(resp.Body.String(), "Some group"))
	require.True(t, strings.Contains(resp.Body.String(), "Some link"))
	require.True(t, strings.Contains(resp.Body.String(), "Dangerous"))
	require.True(t, strings.Contains(resp.Body.String(), "Boom!"))
	require.True(t, strings.Contains(resp.Body.String(), "Dangerous"))
	require.True(t, strings.Contains(resp.Body.String(), "/store-gateway/ring"))
	require.True(t, strings.Contains(resp.Body.String(), "/store-gateway/boom"))
	require.False(t, strings.Contains(resp.Body.String(), "/compactor/ring"))
}
