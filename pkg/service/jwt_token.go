package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

const (
	jwtClaimAPIKeyName = "akn"
	jwtClaimUserName   = "name"
	jwtClaimRole       = "role"
)

// TODO(kolesnikovae): Move to AuthService.

type JWTTokenService struct {
	signingKey               []byte
	userTokenMaxLifetimeDays int
}

func NewJWTTokenService(signingKey []byte, userTokenMaxLifetimeDays int) JWTTokenService {
	return JWTTokenService{
		signingKey:               signingKey,
		userTokenMaxLifetimeDays: userTokenMaxLifetimeDays,
	}
}

func (svc JWTTokenService) GenerateUserToken(name string, role model.Role) *jwt.Token {
	var exp time.Time
	if svc.userTokenMaxLifetimeDays > 0 {
		exp = time.Now().Add(time.Hour * 24 * time.Duration(svc.userTokenMaxLifetimeDays))
	}
	return svc.generateToken(exp, jwt.MapClaims{
		jwtClaimUserName: name,
		jwtClaimRole:     role.String(),
	})
}

func (svc JWTTokenService) GenerateAPIKeyToken(params model.CreateAPIKeyParams) *jwt.Token {
	var exp time.Time
	if params.ExpiresAt != nil {
		exp = *params.ExpiresAt
	}
	return svc.generateToken(exp, jwt.MapClaims{
		jwtClaimAPIKeyName: params.Name,
		jwtClaimRole:       params.Role.String(),
	})
}

func (svc JWTTokenService) generateToken(exp time.Time, claims jwt.MapClaims) *jwt.Token {
	claims["iat"] = time.Now().Unix()
	if !exp.IsZero() {
		claims["exp"] = exp.Unix()
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
}

// UserFromJWTToken retrieves user info from the given JWT token.
// 'name' claim must be present and valid, otherwise the function returns
// false. The function does not validate the token.
func (svc JWTTokenService) UserFromJWTToken(t *jwt.Token) (model.TokenUser, bool) {
	var user model.TokenUser
	m, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		return user, false
	}
	if user.Name, ok = m[jwtClaimUserName].(string); !ok {
		return user, false
	}
	return user, true
}

// APIKeyFromJWTToken retrieves API key info from the given JWT token.
// 'akn' and 'role' claims must be present and valid, otherwise the
// function returns false. The function does not validate the token.
func (svc JWTTokenService) APIKeyFromJWTToken(t *jwt.Token) (model.TokenAPIKey, bool) {
	var apiKey model.TokenAPIKey
	m, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		return apiKey, false
	}
	// Make sure the subject is an API Key.
	if apiKey.Name, ok = m[jwtClaimAPIKeyName].(string); !ok {
		return apiKey, false
	}
	// Parse role.
	s, ok := m[jwtClaimRole].(string)
	if !ok {
		return apiKey, false
	}
	var err error
	apiKey.Role, err = model.ParseRole(s)
	return apiKey, err == nil
}

// Parse parses the token and validates it using the signing key.
func (svc JWTTokenService) Parse(t string) (*jwt.Token, error) {
	return jwt.Parse(t, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return svc.signingKey, nil
	})
}

func (svc JWTTokenService) Sign(t *jwt.Token) (jwtToken, signature string, err error) {
	var sig, sstr string
	if sstr, err = t.SigningString(); err != nil {
		return "", "", err
	}
	if sig, err = t.Method.Sign(sstr, svc.signingKey); err != nil {
		return "", "", err
	}
	return strings.Join([]string{sstr, sig}, "."), sig, nil
}
