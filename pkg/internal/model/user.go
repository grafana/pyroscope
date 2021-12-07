package model

import (
	"errors"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUserNotFound      = NotFoundError{errors.New("user not found")}
	ErrUserEmailExists   = ValidationError{errors.New("user with this email already exists")}
	ErrUserEmailInvalid  = ValidationError{errors.New("user email is invalid")}
	ErrUserNameEmpty     = ValidationError{errors.New("user name can't be empty")}
	ErrUserNameTooLong   = ValidationError{errors.New("user name can't be longer than 255 symbols")}
	ErrUserPasswordEmpty = ValidationError{errors.New("user password can't be empty")}
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
	Role     Role
}

func (p CreateUserParams) Validate() error {
	var err error
	if p.FullName != nil {
		if nameErr := ValidateUserFullName(*p.FullName); nameErr != nil {
			err = multierror.Append(err, nameErr)
		}
	}
	if pwdErr := ValidatePasswordRequirements(p.Password); pwdErr != nil {
		err = multierror.Append(err, pwdErr)
	}
	if emailErr := ValidateEmail(p.Email); emailErr != nil {
		err = multierror.Append(err, emailErr)
	}
	if !p.Role.IsValid() {
		err = multierror.Append(err, ErrRoleUnknown)
	}
	return err
}

type UpdateUserParams struct {
	FullName *string
	Email    *string
	Role     *Role
}

func (p UpdateUserParams) Validate() error {
	var err error
	if p.FullName != nil {
		if nameErr := ValidateUserFullName(*p.FullName); nameErr != nil {
			err = multierror.Append(err, nameErr)
		}
	}
	if p.Email != nil {
		if emailErr := ValidateEmail(*p.Email); emailErr != nil {
			err = multierror.Append(err, emailErr)
		}
	}
	if p.Role != nil && !p.Role.IsValid() {
		err = multierror.Append(err, ErrRoleUnknown)
	}
	return err
}

type ChangeUserPasswordParams struct {
	Password []byte
}

func (p ChangeUserPasswordParams) Validate() error {
	return ValidatePasswordRequirements(p.Password)
}

func ValidateUserFullName(fullName string) error {
	if len(fullName) == 0 {
		return ErrUserNameEmpty
	}
	if len(fullName) > 255 {
		return ErrUserNameTooLong
	}
	return nil
}

func ValidateEmail(email string) error {
	// TODO(kolesnikovae): can also force specific domain(s).
	if !govalidator.IsEmail(email) {
		return ErrUserEmailInvalid
	}
	return nil
}

func ValidatePasswordRequirements(p []byte) error {
	// TODO(kolesnikovae): should be configurable.
	if len(p) == 0 {
		return ErrUserPasswordEmpty
	}
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
