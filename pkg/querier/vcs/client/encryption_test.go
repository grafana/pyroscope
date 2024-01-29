package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestEncodeOAuth(t *testing.T) {
	key := []byte("0123456789abcdef")
	token := &oauth2.Token{
		AccessToken:  "a1b2c3d4e5f6",
		TokenType:    "access_token",
		RefreshToken: "a1b2c3d4e5f6",
		Expiry:       time.Unix(200, 0).UTC(),
	}
	enc, err := encryptToken(token, key)
	require.NoError(t, err)
	require.NotEmpty(t, enc)
	actual, err := decryptToken(enc, key)
	require.NoError(t, err)
	require.Equal(t, token, actual)
}
