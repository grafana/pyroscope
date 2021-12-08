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
	IsDisabled   *bool  `gorm:"not null;default:false"`

	LastSeenAt        time.Time `gorm:"default:null"`
	PasswordChangedAt time.Time `gorm:"not null;default:null"`
}

type CreateUserParams struct {
	FullName *string
	Email    string
	Password string
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
	FullName   *string
	Email      *string
	Role       *Role
	Password   *string
	IsDisabled *bool
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
	if p.Password != nil {
		if pwdErr := ValidatePasswordRequirements(*p.Password); pwdErr != nil {
			err = multierror.Append(err, pwdErr)
		}
	}
	if p.Role != nil && !p.Role.IsValid() {
		err = multierror.Append(err, ErrRoleUnknown)
	}
	return err
}

func IsUserDisabled(u User) bool {
	if u.IsDisabled == nil {
		return false
	}
	return *u.IsDisabled
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
	if !govalidator.IsEmail(email) {
		return ErrUserEmailInvalid
	}
	return nil
}

func ValidatePasswordRequirements(p string) error {
	// TODO(kolesnikovae): should be configurable.
	if p == "" {
		return ErrUserPasswordEmpty
	}
	return nil
}

func MustPasswordHash(password string) []byte {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return h
}

func VerifyPassword(hashed []byte, password string) error {
	return bcrypt.CompareHashAndPassword(hashed, []byte(password))
}
