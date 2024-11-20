package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_matchGitHubAPIRoute(t *testing.T) {
	tests := []struct {
		Name string
		Path string
		Want string
	}{
		{
			Name: "GetContents",
			Path: "/repos/grafana/pyroscope/contents/pkg/querier/querier.go",
			Want: "/repos/{owner}/{repo}/contents/{path}",
		},
		{
			Name: "GetContents with dash",
			Path: "/repos/connectrpc/connect-go/contents/protocol.go",
			Want: "/repos/{owner}/{repo}/contents/{path}",
		},
		{
			Name: "GetContents without path",
			Path: "/repos/grafana/pyroscope/contents/",
			Want: "unknown_route",
		},
		{
			Name: "GetContents with whitespace in path",
			Path: "/repos/grafana/pyroscope/contents/path with spaces",
			Want: "unknown_route",
		},
		{
			Name: "GetCommit",
			Path: "/repos/grafana/pyroscope/commits/abcdef1234567890",
			Want: "/repos/{owner}/{repo}/commits/{ref}",
		},
		{
			Name: "GetCommit with lowercase and uppercase ref",
			Path: "/repos/grafana/pyroscope/commits/abcdefABCDEF1234567890",
			Want: "/repos/{owner}/{repo}/commits/{ref}",
		},
		{
			Name: "GetCommit with non-hexadecimal ref",
			Path: "/repos/grafana/pyroscope/commits/HEAD",
			Want: "/repos/{owner}/{repo}/commits/{ref}",
		},
		{
			Name: "GetCommit without commit",
			Path: "/repos/grafana/pyroscope/commits/",
			Want: "unknown_route",
		},
		{
			Name: "Refresh",
			Path: "/login/oauth/access_token",
			Want: "/login/oauth/access_token",
		},
		{
			Name: "empty path",
			Path: "",
			Want: "unknown_route",
		},
		{
			Name: "unmapped path",
			Path: "/some/random/path",
			Want: "unknown_route",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got := matchGitHubAPIRoute(tt.Path)
			require.Equal(t, tt.Want, got)
		})
	}
}
