package service

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type UserService struct{ db *gorm.DB }

func NewUserService(db *gorm.DB) UserService { return UserService{db} }

func (svc UserService) CreateUser(ctx context.Context, params model.CreateUserParams) (model.User, error) {
	if err := params.Validate(); err != nil {
		return model.User{}, err
	}
	user := model.User{
		Name:              params.Name,
		Email:             params.Email,
		Role:              params.Role,
		PasswordHash:      model.MustPasswordHash(params.Password),
		PasswordChangedAt: time.Now(),
	}
	if params.FullName != nil {
		user.FullName = params.FullName
	}
	err := svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := findUserByEmail(tx, params.Email)
		switch {
		case errors.Is(err, model.ErrUserNotFound):
		case err == nil:
			return model.ErrUserEmailExists
		default:
			return err
		}
		return tx.Create(&user).Error
	})
	if err != nil {
		return model.User{}, err
	}
	return user, nil
}

func (svc UserService) FindUserByName(ctx context.Context, name string) (model.User, error) {
	return findUserByName(svc.db.WithContext(ctx), name)
}

func (svc UserService) FindUserByEmail(ctx context.Context, email string) (model.User, error) {
	if err := model.ValidateEmail(email); err != nil {
		return model.User{}, err
	}
	return findUserByEmail(svc.db.WithContext(ctx), email)
}

func (svc UserService) FindUserByID(ctx context.Context, id uint) (model.User, error) {
	return findUserByID(svc.db.WithContext(ctx), id)
}

func findUserByName(tx *gorm.DB, name string) (model.User, error) {
	return findUser(tx, model.User{Name: name})
}

func findUserByEmail(tx *gorm.DB, email string) (model.User, error) {
	return findUser(tx, model.User{Email: email})
}

func findUserByID(tx *gorm.DB, id uint) (model.User, error) {
	return findUser(tx, model.User{ID: id})
}

func findUser(tx *gorm.DB, user model.User) (model.User, error) {
	var u model.User
	r := tx.Where(user).First(&u)
	switch {
	case r.Error == nil:
		return u, nil
	case errors.Is(r.Error, gorm.ErrRecordNotFound):
		return model.User{}, model.ErrUserNotFound
	default:
		return model.User{}, r.Error
	}
}

func (svc UserService) GetAllUsers(ctx context.Context) ([]model.User, error) {
	var users []model.User
	db := svc.db.WithContext(ctx)
	if err := db.Order("full_name").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (svc UserService) UpdateUserByID(ctx context.Context, id uint, params model.UpdateUserParams) (model.User, error) {
	if err := params.Validate(); err != nil {
		return model.User{}, err
	}
	var updated model.User
	err := svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := findUserByID(tx, id)
		if err != nil {
			return err
		}
		// We only skip update if params are not specified.
		// Otherwise, even if the values match the current ones,
		// the user is to be updated.
		if (model.UpdateUserParams{}) == params {
			updated = user
			return nil
		}
		var columns model.User
		// If the new email matches the current one, ignore.
		if params.Email != nil && user.Email != *params.Email {
			// Make sure it is not in use.
			// Note that we can't rely on the constraint violation error
			// that should occur: underlying database driver errors are
			// not standardized, but service consumers expect friendly
			// typed errors.
			switch _, err = findUserByEmail(tx, *params.Email); {
			case errors.Is(err, model.ErrUserNotFound):
				columns.Email = *params.Email
			case err == nil:
				return model.ErrUserEmailExists
			default:
				return err
			}
		}
		// Same for user name.
		if params.Name != nil && user.Name != *params.Name {
			switch _, err = findUserByName(tx, *params.Name); {
			case errors.Is(err, model.ErrUserNotFound):
				columns.Name = *params.Name
			case err == nil:
				return model.ErrUserNameExists
			default:
				return err
			}
		}
		if params.FullName != nil {
			columns.FullName = params.FullName
		}
		if params.Role != nil {
			columns.Role = *params.Role
		}
		if params.Password != nil {
			columns.PasswordHash = model.MustPasswordHash(*params.Password)
			columns.PasswordChangedAt = time.Now()
		}
		if params.IsDisabled != nil {
			columns.IsDisabled = params.IsDisabled
		}
		return tx.Model(user).Updates(columns).Error
	})
	if err != nil {
		return model.User{}, err
	}
	return updated, nil
}

// DeleteUserByID removes user from the database with "hard" delete.
// This can not be reverted.
func (svc UserService) DeleteUserByID(ctx context.Context, id uint) error {
	return svc.db.WithContext(ctx).Unscoped().Delete(&model.User{}, id).Error
}
