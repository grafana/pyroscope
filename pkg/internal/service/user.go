package service

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/pyroscope-io/pyroscope/pkg/internal/model"
)

type UserService struct{ db *gorm.DB }

func NewUserService(db *gorm.DB) UserService { return UserService{db} }

func (svc UserService) CreateUser(ctx context.Context, params model.CreateUserParams) (user *model.User, err error) {
	if err = params.Validate(); err != nil {
		return nil, err
	}
	return user, svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err = findUserByEmail(tx, params.Email)
		switch {
		case errors.Is(err, model.ErrUserNotFound):
		case err == nil:
			return model.ErrUserEmailExists
		default:
			return err
		}
		user = &model.User{
			Email:        params.Email,
			Role:         params.Role,
			PasswordHash: model.MustPasswordHash(params.Password),
		}
		if params.FullName != nil {
			user.FullName = *params.FullName
		}
		return tx.Create(user).Error
	})
}

func (svc UserService) FindUserByEmail(ctx context.Context, email string) (*model.User, error) {
	return findUserByEmail(svc.db.WithContext(ctx), email)
}

func (svc UserService) FindUserByID(ctx context.Context, id uint) (*model.User, error) {
	return findUserByID(svc.db.WithContext(ctx), id)
}

func findUserByEmail(tx *gorm.DB, email string) (*model.User, error) {
	if err := model.ValidateEmail(email); err != nil {
		return nil, err
	}
	return findUser(tx, model.User{Email: email})
}

func findUserByID(tx *gorm.DB, id uint) (*model.User, error) {
	return findUser(tx, model.User{Model: gorm.Model{ID: id}})
}

func findUser(tx *gorm.DB, user model.User) (*model.User, error) {
	var u model.User
	r := tx.Where(user).First(&u)
	switch {
	case errors.Is(r.Error, gorm.ErrRecordNotFound):
		return nil, model.ErrUserNotFound
	case r.Error == nil:
		return &u, nil
	default:
		return nil, r.Error
	}
}

func (svc UserService) GetAllUsers(ctx context.Context) ([]*model.User, error) {
	var users []*model.User
	db := svc.db.WithContext(ctx)
	if err := db.Order("full_name").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (svc UserService) UpdateUserByID(ctx context.Context, id uint, params model.UpdateUserParams) (user *model.User, err error) {
	if err = params.Validate(); err != nil {
		return nil, err
	}
	return user, svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err = findUserByID(tx, id)
		if err != nil {
			return err
		}
		// If the new email matches the current one, ignore.
		if params.Email != nil && user.Email != *params.Email {
			// Make sure it is not in use.
			switch _, err = findUserByEmail(tx, *params.Email); {
			case errors.Is(err, model.ErrUserNotFound):
				user.Email = *params.Email
			case err == nil:
				return model.ErrUserEmailExists
			default:
				return err
			}
		}
		if params.FullName != nil {
			user.FullName = *params.FullName
		}
		if params.Role != nil {
			user.Role = *params.Role
		}
		return tx.Save(user).Error
	})
}

func (svc UserService) ChangeUserPasswordByID(ctx context.Context, id uint, params model.ChangeUserPasswordParams) (user *model.User, err error) {
	if err = params.Validate(); err != nil {
		return nil, err
	}
	return user, svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err = findUserByID(tx, id)
		if err != nil {
			return err
		}
		user.PasswordHash = model.MustPasswordHash(params.Password)
		user.PasswordChangedAt = time.Now()
		return tx.Save(user).Error
	})
}

func (svc UserService) DisableUserByID(ctx context.Context, id uint) error {
	// TODO(kolesnikovae)
	return nil
}

func (svc UserService) EnableUserByID(ctx context.Context, id uint) error {
	// TODO(kolesnikovae)
	return nil
}

// DeleteUserByID removes user from the database with "hard" delete.
// This can not be reverted.
func (svc UserService) DeleteUserByID(ctx context.Context, id uint) error {
	return svc.db.Unscoped().WithContext(ctx).Delete(&model.User{}, id).Error
}
