package model

import (
	"crypto/rand"
	"errors"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound        = NotFoundError{errors.New("user not found")}
	ErrUserNameExists      = ValidationError{errors.New("user with this name already exists")}
	ErrUserNameEmpty       = ValidationError{errors.New("user name can't be empty")}
	ErrUserNameTooLong     = ValidationError{errors.New("user name must not exceed 255 characters")}
	ErrUserFullNameTooLong = ValidationError{errors.New("user full name must not exceed 255 characters")}
	ErrUserEmailExists     = ValidationError{errors.New("user with this email already exists")}
	ErrUserEmailInvalid    = ValidationError{errors.New("user email is invalid")}
	ErrUserExternal        = ValidationError{errors.New("external users can't be modified")}
	ErrUserPasswordEmpty   = ValidationError{errors.New("user password can't be empty")}
	ErrUserDisabled        = ValidationError{errors.New("user disabled")}
	ErrInvalidCredentials  = ValidationError{errors.New("invalid credentials")}
)

type User struct {
	ID           uint    `gorm:"primarykey"`
	Name         string  `gorm:"type:varchar(255);not null;default:null;index:,unique"`
	Email        string  `gorm:"type:varchar(255);not null;default:null;index:,unique"`
	FullName     *string `gorm:"type:varchar(255);default:null"`
	PasswordHash []byte  `gorm:"type:varchar(255);not null;default:null"`
	Role         Role    `gorm:"not null;default:null"`
	IsDisabled   *bool   `gorm:"not null;default:false"`
	IsExternal   *bool   `gorm:"not null;default:false"`

	LastSeenAt        *time.Time `gorm:"default:null"`
	PasswordChangedAt time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// TokenUser represents a user info retrieved from the validated JWT token.
type TokenUser struct {
	Name string
}

type CreateUserParams struct {
	Name       string
	Email      string
	FullName   *string
	Password   string
	Role       Role
	IsExternal bool
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
	if !p.Role.IsValid() {
		err = multierror.Append(err, ErrRoleUnknown)
	}
	return err
}

type UpdateUserParams struct {
	FullName   *string
	Name       *string
	Email      *string
	Password   *string
	Role       *Role
	IsDisabled *bool
}

func (p UpdateUserParams) SetRole(r Role) UpdateUserParams {
	p.Role = &r
	return p
}

func (p UpdateUserParams) SetIsDisabled(d bool) UpdateUserParams {
	p.IsDisabled = &d
	return p
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
	if p.Role != nil && !p.Role.IsValid() {
		err = multierror.Append(err, ErrRoleUnknown)
	}
	return err
}

type UpdateUserPasswordParams struct {
	OldPassword string
	NewPassword string
}

func (p UpdateUserPasswordParams) Validate() error {
	return ValidatePasswordRequirements(p.NewPassword)
}

func IsUserDisabled(u User) bool {
	if u.IsDisabled == nil {
		return false
	}
	return *u.IsDisabled
}

func IsUserExternal(u User) bool {
	if u.IsExternal == nil {
		return false
	}
	return *u.IsExternal
}

func ValidateUserName(userName string) error {
	// TODO(kolesnikovae): restrict allowed chars?
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

func MustRandomPassword() string {
	// TODO(kolesnikovae): should be compliant with the requirements.
	p := make([]byte, 32)
	if _, err := rand.Read(p); err != nil {
		panic(err)
	}
	return string(p)
}
