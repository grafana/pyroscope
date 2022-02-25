package model

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math/rand"
	"time"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrAPIKeyNotFound    = NotFoundError{errors.New("api key not found")}
	ErrAPIKeyNameExists  = ValidationError{errors.New("api key with this name already exists")}
	ErrAPIKeyNameEmpty   = ValidationError{errors.New("api key name can't be empty")}
	ErrAPIKeyNameTooLong = ValidationError{errors.New("api key name must not exceed 255 characters")}

	ErrAPIKeyInvalid = AuthenticationError{errors.New("API key invalid")}
	ErrAPIKeyExpired = AuthenticationError{errors.New("API key expired")}
)

const (
	apiKeyMagic     = "psx-"
	apiKeySecretLen = 32
)

type APIKey struct {
	ID         uint       `gorm:"primarykey"`
	Name       string     `gorm:"type:varchar(255);not null;default:null;index:,unique"`
	Hash       []byte     `gorm:"type:varchar(255);not null;default:null"`
	Role       Role       `gorm:"not null;default:null"`
	ExpiresAt  *time.Time `gorm:"default:null"`
	LastSeenAt *time.Time `gorm:"default:null"`

	CreatedAt time.Time
}

func (k APIKey) Verify(secret []byte) error {
	if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
		return ErrAPIKeyExpired
	}
	return bcrypt.CompareHashAndPassword(k.Hash, secret)
}

type CreateAPIKeyParams struct {
	Name      string
	Role      Role
	ExpiresAt *time.Time
}

func (p CreateAPIKeyParams) Validate() error {
	var err error
	if nameErr := ValidateAPIKeyName(p.Name); nameErr != nil {
		err = multierror.Append(err, nameErr)
	}
	if !p.Role.IsValid() {
		err = multierror.Append(err, ErrRoleUnknown)
	}
	return err
}

func ValidateAPIKeyName(apiKeyName string) error {
	// TODO(kolesnikovae): restrict allowed chars.
	if len(apiKeyName) == 0 {
		return ErrAPIKeyNameEmpty
	}
	if len(apiKeyName) > 255 {
		return ErrAPIKeyNameTooLong
	}
	return nil
}

// GenerateAPIKey produces an API key and returns the secret bcrypt hash
// to be persisted.
//
// The key format:
//   [4 byte magic][payload]
//
// Currently, the function generates 'psx' key, the payload structure is
// defined as follows: base64(id + secret), where:
//   - id      A var-len encoded uint64 ID of the API key.
//   - secret  A random string of the defined length (32).
//
// The call encodes base64 using raw URL encoding (unpadded alternate base64
// encoding defined in RFC 4648).
func GenerateAPIKey(id uint) (key string, hashed []byte, err error) {
	b := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(b, uint64(id))
	secret := make([]byte, apiKeySecretLen)
	if _, err = rand.Read(secret); err != nil {
		return "", nil, err
	}
	h, err := bcrypt.GenerateFromPassword(secret, bcrypt.DefaultCost)
	if err != nil {
		return "", nil, err
	}
	payload := append(b[:n], secret...)
	return apiKeyMagic + base64.RawURLEncoding.EncodeToString(payload), h, nil
}

// DecodeAPIKey retrieves API key ID and the secret from the given key
// generated with GenerateAPIKey.
func DecodeAPIKey(key string) (id uint, secret []byte, err error) {
	if len(key) < (len(apiKeyMagic) + apiKeySecretLen) {
		// A basic check that does not account for anything
		// specific to the payload format.
		return 0, nil, ErrAPIKeyInvalid
	}
	magic, payload := key[:len(apiKeyMagic)], []byte(key[len(apiKeyMagic):])
	if magic != apiKeyMagic {
		return 0, nil, ErrAPIKeyInvalid
	}
	// Specific to apiKeyMagic ("psx-"); later there can be multiple
	// supported formats.
	dst := make([]byte, binary.MaxVarintLen64+apiKeySecretLen)
	pl, err := base64.RawURLEncoding.Decode(dst, payload)
	if err != nil {
		return 0, nil, err
	}
	x, n := binary.Uvarint(dst[:binary.MaxVarintLen64])
	if n <= 0 || n >= len(dst) {
		return 0, nil, ErrAPIKeyInvalid
	}
	secret = dst[n:pl]
	if len(secret) != apiKeySecretLen {
		return 0, nil, ErrAPIKeyInvalid
	}
	return uint(x), secret, nil
}
