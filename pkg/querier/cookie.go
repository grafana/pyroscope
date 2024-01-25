package querier

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"

	"golang.org/x/oauth2"
)

var githubSessionSecret = os.Getenv("GITHUB_SESSION_SECRET")

func encryptAES256(token *oauth2.Token) (string, error) {
	keyBytes := []byte(githubSessionSecret)
	textBytes, err := json.Marshal(token)
	if err != nil {
		return "", err
	}

	// Create AES cipher block
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}

	// Generate a random IV
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}

	// Pad the plaintext if needed
	blockSize := block.BlockSize()
	textBytes = PKCS7Padding(textBytes, blockSize)

	// Create a new AES cipher block mode with IV
	blockMode := cipher.NewCBCEncrypter(block, iv)

	// Encrypt the plaintext
	ciphertext := make([]byte, len(textBytes))
	blockMode.CryptBlocks(ciphertext, textBytes)

	// Combine IV and ciphertext, then encode to base64
	encryptedData := append(iv, ciphertext...)
	encodedText := base64.StdEncoding.EncodeToString(encryptedData)

	return encodedText, nil
}

func decryptAES256(encodedText string) (*oauth2.Token, error) {
	keyBytes := []byte(githubSessionSecret)

	// Decode the base64-encoded string
	encryptedData, err := base64.StdEncoding.DecodeString(encodedText)
	if err != nil {
		return nil, err
	}

	// Extract IV from the first block
	iv := encryptedData[:aes.BlockSize]
	ciphertext := encryptedData[aes.BlockSize:]

	// Create AES cipher block
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}

	// Create a new AES cipher block mode with IV
	blockMode := cipher.NewCBCDecrypter(block, iv)

	// Decrypt the ciphertext
	plaintext := make([]byte, len(ciphertext))
	blockMode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS7 padding
	plaintext = PKCS7Unpadding(plaintext)

	var token oauth2.Token
	err = json.Unmarshal(plaintext, &token)

	return &token, err
}

// PKCS7Padding pads the input to be a multiple of blockSize using PKCS7 padding.
func PKCS7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// PKCS7Unpadding removes PKCS7 padding from the input.
func PKCS7Unpadding(data []byte) []byte {
	padding := int(data[len(data)-1])
	return data[:len(data)-padding]
}
