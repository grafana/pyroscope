package vcs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/pkg/tenant"
)

func Test_getStringValueFrom(t *testing.T) {
	tests := []struct {
		Name       string
		Query      url.Values
		Key        string
		Want       string
		WantErrMsg string
	}{
		{
			Name: "key exists",
			Query: url.Values{
				"my_key": {"my_value"},
			},
			Key:  "my_key",
			Want: "my_value",
		},
		{
			Name: "key exists with multiple values",
			Query: url.Values{
				"my_key": {"my_value1", "my_value2"},
			},
			Key:  "my_key",
			Want: "my_value1",
		},
		{
			Name: "key is missing",
			Query: url.Values{
				"my_key": {"my_value"},
			},
			Key:        "my_missing_key",
			WantErrMsg: "missing key: my_missing_key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got, err := getStringValueFrom(tt.Query, tt.Key)
			if tt.WantErrMsg != "" {
				require.Error(t, err)
				require.EqualError(t, err, tt.WantErrMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.Want, got)
			}
		})
	}
}

func Test_getDurationValueFrom(t *testing.T) {
	tests := []struct {
		Name       string
		Query      url.Values
		Key        string
		Scalar     time.Duration
		Want       time.Duration
		WantErrMsg string
	}{
		{
			Name: "key exists",
			Query: url.Values{
				"my_key": {"100"},
			},
			Key:    "my_key",
			Scalar: time.Second,
			Want:   100 * time.Second,
		},
		{
			Name: "key exists with multiple values",
			Query: url.Values{
				"my_key": {"100", "200"},
			},
			Key:    "my_key",
			Scalar: time.Second,
			Want:   100 * time.Second,
		},
		{
			Name: "scalar less than 1",
			Query: url.Values{
				"my_key": {"100"},
			},
			Key:        "my_key",
			Scalar:     0,
			WantErrMsg: "cannot use scalar less than 1",
		},
		{
			Name: "value is not a duration",
			Query: url.Values{
				"my_key": {"not_a_number"},
			},
			Key:        "my_key",
			Scalar:     time.Second,
			WantErrMsg: "failed to parse my_key: strconv.Atoi: parsing \"not_a_number\": invalid syntax",
		},
		{
			Name: "key is missing",
			Query: url.Values{
				"my_key": {"my_value"},
			},
			Scalar:     time.Second,
			Key:        "my_missing_key",
			WantErrMsg: "missing key: my_missing_key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got, err := getDurationValueFrom(tt.Query, tt.Key, tt.Scalar)
			if tt.WantErrMsg != "" {
				require.Error(t, err)
				require.EqualError(t, err, tt.WantErrMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.Want, got)
			}
		})
	}
}

func Test_tokenFromRequest(t *testing.T) {
	ctx := newTestContext()

	t.Run("token exists in request", func(t *testing.T) {
		githubSessionSecret = []byte("16_byte_key_XXXX")

		derivedKey, err := deriveEncryptionKeyForContext(ctx)
		require.NoError(t, err)

		wantToken := &oauth2.Token{
			AccessToken:  "my_access_token",
			TokenType:    "my_token_type",
			RefreshToken: "my_refresh_token",
			Expiry:       time.Unix(1713298947, 0).UTC(), // 2024-04-16T20:22:27.346Z
		}

		// The type of request here doesn't matter.
		req := connect.NewRequest(&vcsv1.GetFileRequest{})
		req.Header().Add("Cookie", testEncodeCookie(t, derivedKey, wantToken).String())

		gotToken, err := tokenFromRequest(ctx, req)
		require.NoError(t, err)
		require.Equal(t, *wantToken, *gotToken)
	})

	t.Run("legacy token exists in request", func(t *testing.T) {
		githubSessionSecret = []byte("16_byte_key_XXXX")

		derivedKey, err := deriveEncryptionKeyForContext(ctx)
		require.NoError(t, err)

		wantToken := &oauth2.Token{
			AccessToken:  "my_access_token",
			TokenType:    "my_token_type",
			RefreshToken: "my_refresh_token",
			Expiry:       time.Unix(1713298947, 0).UTC(), // 2024-04-16T20:22:27.346Z
		}

		// The type of request here doesn't matter.
		req := connect.NewRequest(&vcsv1.GetFileRequest{})
		req.Header().Add("Cookie", testEncodeLegacyCookie(t, derivedKey, wantToken).String())

		gotToken, err := tokenFromRequest(ctx, req)
		require.NoError(t, err)
		require.Equal(t, *wantToken, *gotToken)
	})

	t.Run("token does not exist in request", func(t *testing.T) {
		githubSessionSecret = []byte("16_byte_key_XXXX")
		wantErr := "failed to read cookie pyroscope_git_session: http: named cookie not present"

		// The type of request here doesn't matter.
		req := connect.NewRequest(&vcsv1.GetFileRequest{})

		_, err := tokenFromRequest(ctx, req)
		require.Error(t, err)
		require.EqualError(t, err, wantErr)
	})
}

func Test_encodeTokenInCookie(t *testing.T) {
	githubSessionSecret = []byte("16_byte_key_XXXX")
	ctx := newTestContext()

	derivedKey, err := deriveEncryptionKeyForContext(ctx)
	require.NoError(t, err)

	token := &oauth2.Token{
		AccessToken:  "my_access_token",
		TokenType:    "my_token_type",
		RefreshToken: "my_refresh_token",
		Expiry:       time.Unix(1713298947, 0).UTC(), // 2024-04-16T20:22:27.346Z
	}

	got, err := encodeTokenInCookie(token, derivedKey)
	require.NoError(t, err)
	require.Equal(t, sessionCookieName, got.Name)
	require.NotEmpty(t, got.Value)
	require.NotZero(t, got.Expires)
	require.True(t, got.Secure)
	require.Equal(t, http.SameSiteLaxMode, got.SameSite)
}

func Test_decodeToken(t *testing.T) {
	githubSessionSecret = []byte("16_byte_key_XXXX")

	ctx := newTestContext()
	derivedKey, err := deriveEncryptionKeyForContext(ctx)
	require.NoError(t, err)

	t.Run("valid legacy token", func(t *testing.T) {
		want := &oauth2.Token{
			AccessToken:  "my_access_token",
			TokenType:    "my_token_type",
			RefreshToken: "my_refresh_token",
			Expiry:       time.Unix(1713298947, 0).UTC(), // 2024-04-16T20:22:27.346Z
		}
		cookie := testEncodeCookie(t, derivedKey, want)

		got, err := decodeToken(cookie.Value, derivedKey)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("valid legacy token", func(t *testing.T) {
		want := &oauth2.Token{
			AccessToken:  "my_access_token",
			TokenType:    "my_token_type",
			RefreshToken: "my_refresh_token",
			Expiry:       time.Unix(1713298947, 0).UTC(), // 2024-04-16T20:22:27.346Z
		}
		cookie := testEncodeLegacyCookie(t, derivedKey, want)

		got, err := decodeToken(cookie.Value, derivedKey)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("invalid base64 encoding", func(t *testing.T) {
		illegalBase64Encoding := "xx==="

		_, err := decodeToken(illegalBase64Encoding, derivedKey)
		require.Error(t, err)
		require.EqualError(t, err, "illegal base64 data at input byte 4")
	})

	t.Run("invalid json encoding", func(t *testing.T) {
		illegalJSON := base64.StdEncoding.EncodeToString([]byte("illegal json value"))

		_, err := decodeToken(illegalJSON, derivedKey)
		require.Error(t, err)
		require.EqualError(t, err, "invalid character 'i' looking for beginning of value")
	})
}

func Test_tenantIsolation(t *testing.T) {
	githubSessionSecret = []byte("16_byte_key_XXXX")

	var (
		ctxA = newTestContextWithTenantID("tenant_a")
		ctxB = newTestContextWithTenantID("tenant_b")
	)

	derivedKeyA, err := deriveEncryptionKeyForContext(ctxA)
	require.NoError(t, err)

	encodedTokenA := testEncodeCookie(t, derivedKeyA, &oauth2.Token{
		AccessToken: "so_secret",
	})

	req := connect.NewRequest(&vcsv1.GetFileRequest{})
	req.Header().Add("Cookie", encodedTokenA.String())

	tA, err := tokenFromRequest(ctxA, req)
	require.NoError(t, err)
	require.Equal(t, "so_secret", tA.AccessToken)

	_, err = tokenFromRequest(ctxB, req)
	require.ErrorContains(t, err, "message authentication failed")

}

func Test_StillCompatible(t *testing.T) {
	githubSessionSecret = []byte("16_byte_key_XXXX")

	ctx := newTestContextWithTenantID("tenant_a")
	req := connect.NewRequest(&vcsv1.GetFileRequest{})
	req.Header().Add("Cookie", "pyroscope_git_session=eyJtZXRhZGF0YSI6Im12N0d1OHlIanZxdWdQMmF5TnJaYXd1SXNyQXFmUUVIMVhGS1RkejVlZWtob1NRV1JUM3hVZGRuMndUemhQZ05oWktRVkpjcVh5SVJDSnFmTTV3WTJyNmR3R21rZkRhL2FORjhRZ0lJcU1oa1hPbGFEdXNwcFE9PSJ9Cg==")

	realToken, err := tokenFromRequest(ctx, req)
	require.NoError(t, err)
	require.Equal(t, "so_secret", realToken.AccessToken)
}

func newTestContext() context.Context {
	return newTestContextWithTenantID("test_tenant_id")
}

func newTestContextWithTenantID(tenantID string) context.Context {
	return tenant.InjectTenantID(context.Background(), tenantID)
}

func testEncodeCookie(t *testing.T, key []byte, token *oauth2.Token) *http.Cookie {
	t.Helper()

	encrypted, err := encryptToken(token, key)
	require.NoError(t, err)

	cookieValue := gitSessionTokenCookie{
		Token: &encrypted,
	}

	jsonString, err := json.Marshal(cookieValue)
	require.NoError(t, err)

	encoded := base64.StdEncoding.EncodeToString(jsonString)
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    encoded,
		Expires:  token.Expiry,
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

func testEncodeLegacyCookie(t *testing.T, key []byte, token *oauth2.Token) *http.Cookie {
	t.Helper()

	encoded, err := encodeTokenInCookie(token, key)
	require.NoError(t, err)

	return encoded
}
