package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type APIKeyService struct {
	db              *gorm.DB
	jwtTokenService JWTTokenService
}

func NewAPIKeyService(db *gorm.DB, jwtTokenService JWTTokenService) APIKeyService {
	return APIKeyService{
		db:              db,
		jwtTokenService: jwtTokenService,
	}
}

func (svc APIKeyService) CreateAPIKey(ctx context.Context, params model.CreateAPIKeyParams) (model.APIKey, string, error) {
	if err := params.Validate(); err != nil {
		return model.APIKey{}, "", err
	}
	t := svc.jwtTokenService.GenerateAPIKeyToken(params)
	tokenString, signature, err := svc.jwtTokenService.Sign(t)
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
	return apiKey, tokenString, nil
}

func (svc APIKeyService) FindAPIKeyByName(ctx context.Context, apiKeyName string) (model.APIKey, error) {
	return findAPIKeyByName(svc.db.WithContext(ctx), apiKeyName)
}

func findAPIKeyByName(tx *gorm.DB, apiKeyName string) (model.APIKey, error) {
	var apiKey model.APIKey
	r := tx.Where(model.APIKey{Name: apiKeyName}).First(&apiKey)
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
