package querier

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestEncodeOAuth(t *testing.T) {
	token := &oauth2.Token{
		AccessToken:  "a1b2c3d4e5f6",
		TokenType:    "access_token",
		RefreshToken: "a1b2c3d4e5f6",
		Expiry:       time.Unix(200, 0),
	}
	enc, err := encrypt(token)
	require.NoError(t, err)
	require.NotEmpty(t, enc)
	actual, err := decrypt(enc)
	require.NoError(t, err)
	require.Equal(t, token, actual)
}
