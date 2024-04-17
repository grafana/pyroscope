package vcs

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
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
	t.Run("token exists in request", func(t *testing.T) {
		gitSessionSecret = []byte("16_byte_key_XXXX")
		wantToken := &oauth2.Token{
			AccessToken:  "my_access_token",
			TokenType:    "my_token_type",
			RefreshToken: "my_refresh_token",
			Expiry:       time.Unix(1713298947, 0).UTC(), // 2024-04-16T20:22:27.346Z
		}

		// The type of request here doesn't matter.
		req := connect.NewRequest(&vcsv1.GetFileRequest{})
		req.Header().Add("Cookie", testEncodeCookie(t, wantToken).String())

		gotToken, err := tokenFromRequest(req)
		require.NoError(t, err)
		require.Equal(t, *wantToken, *gotToken)
	})

	t.Run("token does not exist in request", func(t *testing.T) {
		gitSessionSecret = []byte("16_byte_key_XXXX")
		wantErr := "failed to read cookie GitSession: http: named cookie not present"

		// The type of request here doesn't matter.
		req := connect.NewRequest(&vcsv1.GetFileRequest{})

		_, err := tokenFromRequest(req)
		require.Error(t, err)
		require.EqualError(t, err, wantErr)
	})
}

func Test_encodeToken(t *testing.T) {
	gitSessionSecret = []byte("16_byte_key_XXXX")
	token := &oauth2.Token{
		AccessToken:  "my_access_token",
		TokenType:    "my_token_type",
		RefreshToken: "my_refresh_token",
		Expiry:       time.Unix(1713298947, 0).UTC(), // 2024-04-16T20:22:27.346Z
	}

	got, err := encodeToken(token)
	require.NoError(t, err)
	require.Equal(t, sessionCookieName, got.Name)
	require.NotEmpty(t, got.Value)
	require.NotZero(t, got.Expires)
	require.True(t, got.Secure)
	require.Equal(t, http.SameSiteLaxMode, got.SameSite)
}

func Test_decodeToken(t *testing.T) {
	gitSessionSecret = []byte("16_byte_key_XXXX")

	t.Run("valid token", func(t *testing.T) {
		want := &oauth2.Token{
			AccessToken:  "my_access_token",
			TokenType:    "my_token_type",
			RefreshToken: "my_refresh_token",
			Expiry:       time.Unix(1713298947, 0).UTC(), // 2024-04-16T20:22:27.346Z
		}
		cookie := testEncodeCookie(t, want)

		got, err := decodeToken(cookie.Value)
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
		cookie := testEncodeLegacyCookie(t, want)

		got, err := decodeToken(cookie.Value)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("invalid base64 encoding", func(t *testing.T) {
		illegalBase64Encoding := "xx==="

		_, err := decodeToken(illegalBase64Encoding)
		require.Error(t, err)
		require.EqualError(t, err, "illegal base64 data at input byte 4")
	})

	t.Run("invalid json encoding", func(t *testing.T) {
		illegalJSON := base64.StdEncoding.EncodeToString([]byte("illegal json value"))

		_, err := decodeToken(illegalJSON)
		require.Error(t, err)
		require.EqualError(t, err, "invalid character 'i' looking for beginning of value")
	})
}

func testEncodeCookie(t *testing.T, token *oauth2.Token) *http.Cookie {
	t.Helper()

	encoded, err := encodeToken(token)
	require.NoError(t, err)

	return encoded
}

func testEncodeLegacyCookie(t *testing.T, token *oauth2.Token) *http.Cookie {
	t.Helper()

	encrypted, err := encryptToken(token, gitSessionSecret)
	require.NoError(t, err)

	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    encrypted,
		Expires:  token.Expiry,
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}
