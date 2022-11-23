package service

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

const (
	jwtClaimUserName = "name"
	jwtClaimRole     = "role"
)

type JWTTokenService struct {
	signingKey []byte
	tokenTTL   time.Duration
}

func NewJWTTokenService(signingKey []byte, tokenTTL time.Duration) JWTTokenService {
	return JWTTokenService{
		signingKey: signingKey,
		tokenTTL:   tokenTTL,
	}
}

func (svc JWTTokenService) GenerateUserJWTToken(name string, role model.Role) *jwt.Token {
	var exp time.Time
	if svc.tokenTTL > 0 {
		exp = time.Now().Add(svc.tokenTTL)
	}
	return generateToken(exp, jwt.MapClaims{
		jwtClaimUserName: name,
		jwtClaimRole:     role.String(),
	})
}

func generateToken(exp time.Time, claims jwt.MapClaims) *jwt.Token {
	claims["iat"] = time.Now().Unix()
	if !exp.IsZero() {
		claims["exp"] = exp.Unix()
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
}

// UserFromJWTToken retrieves user info from the given JWT token.
// 'name' claim must be present and valid, otherwise the function returns
// false. The function does not validate the token.
func (JWTTokenService) UserFromJWTToken(t *jwt.Token) (model.TokenUser, bool) {
	var user model.TokenUser
	m, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		return user, false
	}
	if user.Name, ok = m[jwtClaimUserName].(string); !ok {
		return user, false
	}
	// Parse role.
	s, ok := m[jwtClaimRole].(string)
	if !ok {
		return user, false
	}
	var err error
	user.Role, err = model.ParseRole(s)
	return user, err == nil
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

func (svc JWTTokenService) Sign(t *jwt.Token) (string, error) {
	return t.SignedString(svc.signingKey)
}
