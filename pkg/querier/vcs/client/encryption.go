package client

import (
	"encoding/base64"
	"encoding/json"

	encryption "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/encryption"
	"golang.org/x/oauth2"
)

func encryptToken(token *oauth2.Token, key []byte) (string, error) {
	cipher, err := encryption.NewGCMCipher(key)
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

func decryptToken(encodedText string, key []byte) (*oauth2.Token, error) {
	encryptedData, err := base64.StdEncoding.DecodeString(encodedText)
	if err != nil {
		return nil, err
	}
	cipher, err := encryption.NewGCMCipher(key)
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
