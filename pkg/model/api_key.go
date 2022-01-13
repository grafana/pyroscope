package model

import (
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/hashicorp/go-multierror"
	"gorm.io/gorm"
)

var (
	ErrAPIKeyNotFound    = NotFoundError{errors.New("api key not found")}
	ErrAPIKeyNameExists  = ValidationError{errors.New("api key with this name already exists")}
	ErrAPIKeyNameEmpty   = ValidationError{errors.New("api key name can't be empty")}
	ErrAPIKeyNameTooLong = ValidationError{errors.New("api key name must not exceed 255 characters")}
)

type APIKey struct {
	ID         uint       `gorm:"primarykey"`
	Name       string     `gorm:"type:varchar(255);not null;default:null;index:,unique"`
	Signature  string     `gorm:"type:varchar(255);not null;default:null"`
	Role       Role       `gorm:"not null;default:null"`
	ExpiresAt  *time.Time `gorm:"default:null"`
	LastSeenAt *time.Time `gorm:"default:null"`

	CreatedAt time.Time
	DeletedAt gorm.DeletedAt
}

func (k APIKey) VerifySignature(t *jwt.Token) bool {
	return subtle.ConstantTimeCompare([]byte(k.Signature), []byte(t.Signature)) == 1
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

// TokenAPIKey represents an API key retrieved from the validated JWT token.
type TokenAPIKey struct {
	Name string
	Role Role
}

const (
	jwtClaimAPIKeyName = "akn"
	jwtClaimRole       = "role"
)

// JWTToken creates a new JWT token structure for the given API
// key params. The token needs to be signed: use SignJWTToken to
// get the signature along with with the signed JWT string.
func (p CreateAPIKeyParams) JWTToken() *jwt.Token {
	iat := time.Now()
	claims := jwt.MapClaims{
		"iat":              iat.Unix(),
		jwtClaimAPIKeyName: p.Name,
		jwtClaimRole:       p.Role.String(),
	}
	if p.ExpiresAt != nil && !p.ExpiresAt.IsZero() {
		claims["exp"] = p.ExpiresAt
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
}

// SignJWTToken returns the complete, signed token and its signature.
func SignJWTToken(t *jwt.Token, key []byte) (jwtToken, signature string, err error) {
	var sig, sstr string
	if sstr, err = t.SigningString(); err != nil {
		return "", "", err
	}
	if sig, err = t.Method.Sign(sstr, key); err != nil {
		return "", "", err
	}
	return strings.Join([]string{sstr, sig}, "."), sig, nil
}

// APIKeyFromJWTToken retrieves API key info from the given JWT token.
// 'akn' and 'role' claims must be present and valid, otherwise the
// function returns false. The function does not validate the token.
func APIKeyFromJWTToken(t *jwt.Token) (TokenAPIKey, bool) {
	var apiKey TokenAPIKey
	m, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		return apiKey, false
	}
	// Make sure the subject is an API Key.
	if apiKey.Name, ok = m[jwtClaimAPIKeyName].(string); !ok {
		return apiKey, false
	}
	// Parse role.
	s, ok := m[jwtClaimRole].(string)
	if !ok {
		return apiKey, false
	}
	var err error
	apiKey.Role, err = ParseRole(s)
	return apiKey, err == nil
}
