package service

import (
	"context"
	"fmt"

	"github.com/golang-jwt/jwt"
	"gorm.io/gorm"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type AuthService struct {
	db *gorm.DB

	UserService
	APIKeyService

	// TODO(kolesnikovae): There should be a separate service
	//   responsible for JWT token generation, refresh, and validation.
	//   The service should also implement the standard Access/Refresh
	//   token flow.
	jwtSigningKey []byte
}

func NewAuthService(db *gorm.DB, jwtSigningKey []byte) AuthService {
	return AuthService{
		db: db,

		UserService:   NewUserService(db),
		APIKeyService: NewAPIKeyService(db, jwtSigningKey),
		jwtSigningKey: jwtSigningKey,
	}
}

func (svc AuthService) APIKeyFromToken(ctx context.Context, t string) (model.TokenAPIKey, error) {
	token, err := svc.parseJWTToken(t)
	if err != nil {
		return model.TokenAPIKey{}, fmt.Errorf("invalid jwt token")
	}
	keyToken, ok := model.APIKeyFromJWTToken(token)
	if !ok {
		return model.TokenAPIKey{}, fmt.Errorf("api key is invalid")
	}
	apiKey, err := svc.APIKeyService.FindAPIKeyByName(ctx, keyToken.Name)
	if err != nil {
		return model.TokenAPIKey{}, err
	}
	if !apiKey.VerifySignature(token) {
		return model.TokenAPIKey{}, fmt.Errorf("api key signature mismatch")
	}
	return keyToken, nil
}

func (svc AuthService) UserFromToken(_ context.Context, t string) (model.User, error) {
	token, err := svc.parseJWTToken(t)
	if err != nil {
		return model.User{}, fmt.Errorf("invalid jwt token")
	}
	userToken, ok := model.UserFromJWTToken(token)
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

func (svc AuthService) parseJWTToken(t string) (*jwt.Token, error) {
	return jwt.Parse(t, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return svc.jwtSigningKey, nil
	})
}
