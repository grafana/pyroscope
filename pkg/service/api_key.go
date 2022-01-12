package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type APIKeyService struct {
	db         *gorm.DB
	signingKey []byte
}

func NewAPIKeyService(db *gorm.DB, signingKey []byte) APIKeyService {
	return APIKeyService{
		db:         db,
		signingKey: signingKey,
	}
}

func (svc APIKeyService) CreateAPIKey(ctx context.Context, params model.CreateAPIKeyParams) (model.APIKey, string, error) {
	if err := params.Validate(); err != nil {
		return model.APIKey{}, "", err
	}
	token, signature, err := model.SignJWTToken(params.JWTToken(), svc.signingKey)
	if err != nil {
		return model.APIKey{}, "", err
	}
	apiKey := model.APIKey{
		Name:      params.Name,
		Signature: signature,
		Role:      params.Role,
		ExpiresAt: params.ExpiresAt,
	}
	err = svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err = findAPIKeyByName(tx, params.Name)
		switch {
		case errors.Is(err, model.ErrAPIKeyNotFound):
		case err == nil:
			return model.ErrAPIKeyNameExists
		default:
			return err
		}
		return tx.Create(&apiKey).Error
	})
	if err != nil {
		return model.APIKey{}, "", err
	}
	return apiKey, token, nil
}

func (svc APIKeyService) FindAPIKeyByName(ctx context.Context, apiKeyName string) (model.APIKey, error) {
	return findAPIKeyByName(svc.db.WithContext(ctx), apiKeyName)
}

func findAPIKeyByName(tx *gorm.DB, apiKeyName string) (model.APIKey, error) {
	var apiKey model.APIKey
	r := tx.Where("name = ?", apiKeyName).First(&apiKey)
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
	db := svc.db.WithContext(ctx)
	if err := db.Find(&apiKeys).Error; err != nil {
		return nil, err
	}
	return apiKeys, nil
}

func (svc APIKeyService) DeleteAPIKeyByID(ctx context.Context, id uint) error {
	return svc.db.WithContext(ctx).Unscoped().Delete(&model.APIKey{}, id).Error
}
