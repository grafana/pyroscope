package model

import (
	"errors"
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
	ID uint `gorm:"primarykey"`

	Name       string     `gorm:"type:varchar(255);not null;default:null;index:,unique"`
	Role       Role       `gorm:"not null;default:null"`
	ExpiresAt  *time.Time `gorm:"default:null"`
	LastSeenAt *time.Time `gorm:"default:null"`

	CreatedAt time.Time
	DeletedAt gorm.DeletedAt
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
	Role
}

const (
	jwtClaimName   = "name"
	jwtClaimRole   = "role"
	jwtClaimAPIKey = "iak"
)

// JWTToken creates a JWT token structure for the given API key params.
func (p CreateAPIKeyParams) JWTToken() *jwt.Token {
	iat := time.Now()
	claims := jwt.MapClaims{
		"iat":          iat.Unix(),
		jwtClaimName:   p.Name,
		jwtClaimRole:   p.Role.String(),
		jwtClaimAPIKey: true,
	}
	if p.ExpiresAt != nil && !p.ExpiresAt.IsZero() {
		claims["exp"] = p.ExpiresAt
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
}

// APIKeyFromJWTToken retrieves API key info from the given JWT token.
// 'name', 'role', and 'api_key' claims must be present and valid,
// otherwise the function returns false.
func APIKeyFromJWTToken(t *jwt.Token) (TokenAPIKey, bool) {
	var apiKey TokenAPIKey
	m, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		return apiKey, false
	}
	// Make sure the subject is an API Key.
	v, ok := m[jwtClaimAPIKey]
	if !ok {
		return apiKey, false
	}
	if iak, ok := v.(bool); !ok || !iak {
		return apiKey, false
	}

	v, ok = m[jwtClaimName]
	if !ok {
		return apiKey, false
	}
	apiKey.Name, ok = v.(string)
	if !ok {
		return apiKey, false
	}

	v, ok = m[jwtClaimRole]
	if !ok {
		return apiKey, false
	}
	s, ok := v.(string)
	if !ok {
		return apiKey, false
	}
	var err error
	apiKey.Role, err = ParseRole(s)
	if err != nil {
		return apiKey, false
	}

	return apiKey, true
}
