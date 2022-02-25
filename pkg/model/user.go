package model

import (
	"crypto/rand"
	"errors"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/crypto/bcrypt"
)

// revive:disable:max-public-structs domain complexity

var (
	ErrUserNotFound = NotFoundError{errors.New("user not found")}

	ErrUserNameExists      = ValidationError{errors.New("user with this name already exists")}
	ErrUserNameEmpty       = ValidationError{errors.New("user name can't be empty")}
	ErrUserNameTooLong     = ValidationError{errors.New("user name must not exceed 255 characters")}
	ErrUserFullNameTooLong = ValidationError{errors.New("user full name must not exceed 255 characters")}
	ErrUserEmailExists     = ValidationError{errors.New("user with this email already exists")}
	ErrUserEmailInvalid    = ValidationError{errors.New("user email is invalid")}
	ErrUserExternalChange  = ValidationError{errors.New("external users can't be modified")}
	ErrUserPasswordEmpty   = ValidationError{errors.New("user password can't be empty")}
	ErrUserPasswordTooLong = ValidationError{errors.New("user password must not exceed 255 characters")}
	ErrUserPasswordInvalid = ValidationError{errors.New("invalid password")}
	ErrUserDisabled        = ValidationError{errors.New("user disabled")}

	// ErrCredentialsInvalid should be returned when details of the authentication
	// failure should be hidden (e.g. when user or API key not found).
	ErrCredentialsInvalid = AuthenticationError{errors.New("invalid credentials")}
	// ErrPermissionDenied should be returned if the actor does not have
	// sufficient permissions for the action.
	ErrPermissionDenied = AuthorizationError{errors.New("permission denied")}
)

type User struct {
	ID           uint    `gorm:"primarykey"`
	Name         string  `gorm:"type:varchar(255);not null;default:null;index:,unique"`
	Email        *string `gorm:"type:varchar(255);default:null;index:,unique"`
	FullName     *string `gorm:"type:varchar(255);default:null"`
	PasswordHash []byte  `gorm:"type:varchar(255);not null;default:null"`
	Role         Role    `gorm:"not null;default:null"`
	IsDisabled   *bool   `gorm:"not null;default:false"`

	// IsExternal indicates that the user authenticity is confirmed by
	// an external authentication provider (such as OAuth) and thus,
	// only limited attributes of the user can be managed. In fact, only
	// FullName and Email can be altered by the user, and Role and IsDisabled
	// can be changed by an administrator. Name should never change.
	// TODO(kolesnikovae):
	//  Add an attribute indicating the provider (e.g OAuth/LDAP).
	//  Can it be a tagged union (sum type)?
	IsExternal *bool `gorm:"not null;default:false"`

	// TODO(kolesnikovae): Add an attribute indicating whether the email is confirmed.
	// IsEmailConfirmed *bool

	// TODO(kolesnikovae): Add an attribute forcing user to change its password.
	// IsPasswordChangeRequired *bool

	// TODO(kolesnikovae): Implemented LastSeenAt updating.
	LastSeenAt        *time.Time `gorm:"default:null"`
	PasswordChangedAt time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// TokenUser represents a user info retrieved from the validated JWT token.
type TokenUser struct {
	Name string
	Role Role
}

type CreateUserParams struct {
	Name       string
	Email      *string
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
	p.Role = &r // revive:disable:modifies-value-receiver returns by value
	return p
}

func (p UpdateUserParams) SetIsDisabled(d bool) UpdateUserParams {
	p.IsDisabled = &d // revive:disable:modifies-value-receiver returns by value
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
	if len(p) > 255 {
		return ErrUserPasswordTooLong
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
