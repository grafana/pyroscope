package kuberesolver

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTokenFileClient(t *testing.T, token string, readAt time.Time) (*k8sClient, string) {
	t.Helper()
	tokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte(token), 0o600))
	return &k8sClient{
		host:        "https://kubernetes.default.svc",
		token:       token,
		tokenFile:   tokenFile,
		tokenReadAt: readAt,
		httpClient:  http.DefaultClient,
	}, tokenFile
}

func bearerToken(t *testing.T, c *k8sClient) string {
	t.Helper()
	req, err := c.GetRequest("/api/v1/namespaces/default/endpoints/foo")
	require.NoError(t, err)
	return req.Header.Get("Authorization")
}

// The kubelet rotates the projected service account token, but if the
// fsnotify event is missed the client must still pick up the new token:
// the file is re-read once the cached copy is older than the refresh
// period.
func TestK8sClient_TokenRefreshedAfterRefreshPeriod(t *testing.T) {
	client, tokenFile := newTokenFileClient(t, "token-a", time.Now().Add(-2*tokenRefreshPeriod))
	require.NoError(t, os.WriteFile(tokenFile, []byte("token-b"), 0o600))
	require.Equal(t, "Bearer token-b", bearerToken(t, client))
}

func TestK8sClient_TokenKeptWithinRefreshPeriod(t *testing.T) {
	client, tokenFile := newTokenFileClient(t, "token-a", time.Now())
	require.NoError(t, os.WriteFile(tokenFile, []byte("token-b"), 0o600))
	require.Equal(t, "Bearer token-a", bearerToken(t, client))
}

func TestK8sClient_FailedRefreshKeepsCachedToken(t *testing.T) {
	client, tokenFile := newTokenFileClient(t, "token-a", time.Now().Add(-2*tokenRefreshPeriod))
	require.NoError(t, os.Remove(tokenFile))
	require.Equal(t, "Bearer token-a", bearerToken(t, client))
}

func TestK8sClient_NoTokenFileNoRefresh(t *testing.T) {
	client := &k8sClient{host: "https://localhost", httpClient: http.DefaultClient}
	require.Empty(t, bearerToken(t, client))
}
