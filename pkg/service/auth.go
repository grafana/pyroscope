package service

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type AuthService struct {
	db *gorm.DB

	userService     UserService
	apiKeyService   APIKeyService
	jwtTokenService JWTTokenService
}

func NewAuthService(db *gorm.DB, jwtTokenService JWTTokenService) AuthService {
	return AuthService{
		db: db,

		userService:     NewUserService(db),
		apiKeyService:   NewAPIKeyService(db, jwtTokenService),
		jwtTokenService: jwtTokenService,
	}
}

func (svc AuthService) APIKeyFromJWTToken(ctx context.Context, t string) (model.TokenAPIKey, error) {
	token, err := svc.jwtTokenService.Parse(t)
	if err != nil {
		return model.TokenAPIKey{}, fmt.Errorf("invalid jwt token")
	}
	keyToken, ok := svc.jwtTokenService.APIKeyFromJWTToken(token)
	if !ok {
		return model.TokenAPIKey{}, fmt.Errorf("api key is invalid")
	}
	apiKey, err := svc.apiKeyService.FindAPIKeyByName(ctx, keyToken.Name)
	if err != nil {
		return model.TokenAPIKey{}, err
	}
	if !apiKey.VerifySignature(token) {
		return model.TokenAPIKey{}, fmt.Errorf("api key signature mismatch")
	}
	return keyToken, nil
}

func (svc AuthService) UserFromJWTToken(_ context.Context, t string) (model.User, error) {
	token, err := svc.jwtTokenService.Parse(t)
	if err != nil {
		return model.User{}, fmt.Errorf("invalid jwt token")
	}
	userToken, ok := svc.jwtTokenService.UserFromJWTToken(token)
	if !ok {
		return model.User{}, fmt.Errorf("user token is invalid")
	}
	// TODO(kolesnikovae): Fetch user via UserService.
	// user, err := svc.UserService.FindUserByName(ctx, userToken.Name)
	// if err != nil {
	// 	return model.User{}, err
	// }
	// if model.IsUserDisabled(user) {
	// 	return model.User{}, fmt.Errorf("user disabled")
	// }
	user := model.User{
		Name: userToken.Name,
		Role: model.AdminRole,
	}
	return user, nil
}
