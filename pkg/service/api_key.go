package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type APIKeyService struct{ db *gorm.DB }

func NewAPIKeyService(db *gorm.DB) APIKeyService { return APIKeyService{db: db} }

func (svc APIKeyService) CreateAPIKey(ctx context.Context, params model.CreateAPIKeyParams) (model.APIKey, string, error) {
	if err := params.Validate(); err != nil {
		return model.APIKey{}, "", err
	}
	apiKey := model.APIKey{
		Name:      params.Name,
		Hash:      []byte{},
		Role:      params.Role,
		ExpiresAt: params.ExpiresAt,
	}
	var secret string
	err := svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := findAPIKeyByName(tx, params.Name)
		switch {
		case errors.Is(err, model.ErrAPIKeyNotFound):
		case err == nil:
			return model.ErrAPIKeyNameExists
		default:
			return err
		}
		if err = tx.Create(&apiKey).Error; err != nil {
			return err
		}
		var hash []byte
		if secret, hash, err = model.GenerateAPIKey(apiKey.ID); err != nil {
			return err
		}
		apiKey.Hash = hash
		return tx.Updates(&model.APIKey{ID: apiKey.ID, Hash: hash}).Error
	})
	if err != nil {
		return model.APIKey{}, "", err
	}
	return apiKey, secret, nil
}

func (svc APIKeyService) FindAPIKeyByID(ctx context.Context, id uint) (model.APIKey, error) {
	return findAPIKeyByID(svc.db.WithContext(ctx), id)
}

func (svc APIKeyService) FindAPIKeyByName(ctx context.Context, name string) (model.APIKey, error) {
	return findAPIKeyByName(svc.db.WithContext(ctx), name)
}

func findAPIKeyByID(tx *gorm.DB, id uint) (model.APIKey, error) {
	return findAPIKey(tx, model.APIKey{ID: id})
}

func findAPIKeyByName(tx *gorm.DB, apiKeyName string) (model.APIKey, error) {
	return findAPIKey(tx, model.APIKey{Name: apiKeyName})
}

func findAPIKey(tx *gorm.DB, k model.APIKey) (model.APIKey, error) {
	var apiKey model.APIKey
	r := tx.Where(k).First(&apiKey)
	switch {
	case r.Error == nil:
		return apiKey, nil
	case errors.Is(r.Error, gorm.ErrRecordNotFound):
		return model.APIKey{}, model.ErrAPIKeyNotFound
	default:
		return model.APIKey{}, r.Error
	}
}

func (svc APIKeyService) GetAllAPIKeys(ctx context.Context) ([]model.APIKey, error) {
	var apiKeys []model.APIKey
	return apiKeys, svc.db.WithContext(ctx).Find(&apiKeys).Error
}

func (svc APIKeyService) DeleteAPIKeyByID(ctx context.Context, id uint) error {
	return svc.db.WithContext(ctx).Delete(&model.APIKey{}, id).Error
}
