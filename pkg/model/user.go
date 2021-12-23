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
	ErrUserNotFound        = NotFoundError{errors.New("user not found")}
	ErrUserNameEmpty       = ValidationError{errors.New("user name can't be empty")}
	ErrUserNameExists      = ValidationError{errors.New("user with this name already exists")}
	ErrUserNameTooLong     = ValidationError{errors.New("user name must not exceed 255 characters")}
	ErrUserFullNameTooLong = ValidationError{errors.New("user full name must not exceed 255 characters")}
	ErrUserEmailExists     = ValidationError{errors.New("user with this email already exists")}
	ErrUserEmailInvalid    = ValidationError{errors.New("user email is invalid")}
	ErrUserPasswordEmpty   = ValidationError{errors.New("user password can't be empty")}
)

type User struct {
	ID           uint    `gorm:"primarykey"`
	Name         string  `gorm:"type:varchar(255);not null;default:null;index:,unique"`
	Email        string  `gorm:"type:varchar(255);not null;default:null;index:,unique"`
	FullName     *string `gorm:"type:varchar(255);default:null"`
	PasswordHash []byte  `gorm:"type:varchar(255);not null;default:null"`
	IsDisabled   *bool   `gorm:"not null;default:false"`
	IsAdmin      *bool   `gorm:"not null;default:false"`

	LastSeenAt        *time.Time `gorm:"default:null"`
	PasswordChangedAt time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt
}

type CreateUserParams struct {
	Name     string
	Email    string // TODO(kolesnikovae): Make optional.
	FullName *string
	Password string
	IsAdmin  bool
}

func (p CreateUserParams) Validate() error {
	var err error
	if nameErr := ValidateUserName(p.Name); nameErr != nil {
		err = multierror.Append(err, nameErr)
	}
	if emailErr := ValidateEmail(p.Email); emailErr != nil {
		err = multierror.Append(err, emailErr)
	}
	if p.FullName != nil {
		if nameErr := ValidateUserFullName(*p.FullName); nameErr != nil {
			err = multierror.Append(err, nameErr)
		}
	}
	if pwdErr := ValidatePasswordRequirements(p.Password); pwdErr != nil {
		err = multierror.Append(err, pwdErr)
	}
	return err
}

type UpdateUserParams struct {
	FullName   *string
	Name       *string
	Email      *string
	Password   *string
	IsDisabled *bool
	IsAdmin    *bool
}

func (p UpdateUserParams) Validate() error {
	var err error
	if p.Name != nil {
		if nameErr := ValidateUserName(*p.Name); nameErr != nil {
			err = multierror.Append(err, nameErr)
		}
	}
	if p.Email != nil {
		if emailErr := ValidateEmail(*p.Email); emailErr != nil {
			err = multierror.Append(err, emailErr)
		}
	}
	if p.FullName != nil {
		if nameErr := ValidateUserFullName(*p.FullName); nameErr != nil {
			err = multierror.Append(err, nameErr)
		}
	}
	if p.Password != nil {
		if pwdErr := ValidatePasswordRequirements(*p.Password); pwdErr != nil {
			err = multierror.Append(err, pwdErr)
		}
	}
	return err
}

func (p UpdateUserParams) SetIsDisabled(d bool) UpdateUserParams {
	p.IsDisabled = &d
	return p
}

func IsUserDisabled(u User) bool {
	if u.IsDisabled == nil {
		return false
	}
	return *u.IsDisabled
}

func (p UpdateUserParams) SetIsAdmin(d bool) UpdateUserParams {
	p.IsAdmin = &d
	return p
}

func IsUserAdmin(u User) bool {
	if u.IsAdmin == nil {
		return false
	}
	return *u.IsAdmin
}

func ValidateUserName(userName string) error {
	if len(userName) == 0 {
		return ErrUserNameEmpty
	}
	if len(userName) > 255 {
		return ErrUserNameTooLong
	}
	return nil
}

func ValidateUserFullName(fullName string) error {
	if len(fullName) > 255 {
		return ErrUserFullNameTooLong
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
