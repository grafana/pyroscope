package model

import (
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUserNotFound    = NotFoundError{errors.New("user not found")}
	ErrUserEmailExists = ValidationError{errors.New("user with this email already exists")}
	ErrUserNilPassword = ValidationError{errors.New("user password can't be empty")}
)

type User struct {
	gorm.Model

	FullName     string `gorm:"type:varchar(255);default:null"`
	Email        string `gorm:"type:varchar(255);not null;default:null;index:,unique"`
	PasswordHash []byte `gorm:"type:varchar(255);not null;default:null"`
	Role         Role   `gorm:"not null;default:null"`

	LastSeenAt        time.Time
	PasswordChangedAt time.Time
}

type CreateUserParams struct {
	FullName *string
	Email    string
	Password []byte
	Role     Role // TODO: default role?
}

func (p CreateUserParams) Validate() error {
	// TODO: use multi-err.
	if !p.Role.IsValid() {
		return ErrRoleUnknown
	}
	if len(p.Password) == 0 {
		return ErrUserNilPassword
	}
	// TODO: email validation.
	return nil
}

type UpdateUserParams struct {
	FullName *string
	Email    *string
	Role     *Role
}

type ChangeUserPassword struct {
	Password []byte
}

func (p ChangeUserPassword) Validate() error {
	if len(p.Password) == 0 {
		return ErrUserNilPassword
	}
	// TODO: password requirement.
	return nil
}

func (p UpdateUserParams) Validate() error {
	if p.Role != nil && !p.Role.IsValid() {
		return ErrRoleUnknown
	}
	// TODO: email validation.
	return nil
}

func MustPasswordHash(password []byte) []byte {
	h, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return h
}

func VerifyPassword(hashed, password []byte) error {
	return bcrypt.CompareHashAndPassword(hashed, password)
}
