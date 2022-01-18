package model

import (
	"crypto/subtle"
	"errors"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/hashicorp/go-multierror"
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
}

func (k APIKey) VerifySignature(t *jwt.Token) bool {
	return subtle.ConstantTimeCompare([]byte(k.Signature), []byte(t.Signature)) == 1
}

// TokenAPIKey represents an API key retrieved from the validated JWT token.
type TokenAPIKey struct {
	Name string
	Role Role
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
