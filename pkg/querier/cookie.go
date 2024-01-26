package querier

import (
	"encoding/base64"
	"encoding/json"
	"os"

	encryption "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/encryption"
	"golang.org/x/oauth2"
)

var githubSessionSecret = []byte(os.Getenv("GITHUB_SESSION_SECRET"))

func encrypt(token *oauth2.Token) (string, error) {
	cipher, err := encryption.NewGCMCipher(githubSessionSecret)
	if err != nil {
		return "", err
	}
	textBytes, err := json.Marshal(token)
	if err != nil {
		return "", err
	}
	enc, err := cipher.Encrypt(textBytes)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(enc), nil
}

func decrypt(encodedText string) (*oauth2.Token, error) {
	// Decode the base64-encoded string
	encryptedData, err := base64.StdEncoding.DecodeString(encodedText)
	if err != nil {
		return nil, err
	}
	cipher, err := encryption.NewGCMCipher(githubSessionSecret)
	if err != nil {
		return nil, err
	}

	plaintext, err := cipher.Decrypt(encryptedData)
	if err != nil {
		return nil, err
	}

	var token oauth2.Token
	err = json.Unmarshal(plaintext, &token)
	return &token, err
}
