package model

import "context"

var (
	ctxUserKey   struct{}
	ctxAPIKeyKey struct{}
)

func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, ctxUserKey, user)
}

func UserFromContext(ctx context.Context) (User, bool) {
	if user, ok := ctx.Value(ctxUserKey).(User); ok {
		return user, true
	}
	return User{}, false
}

func WithTokenAPIKey(ctx context.Context, key TokenAPIKey) context.Context {
	return context.WithValue(ctx, ctxAPIKeyKey, key)
}

func APIKeyFromContext(ctx context.Context) (TokenAPIKey, bool) {
	if key, ok := ctx.Value(ctxAPIKeyKey).(TokenAPIKey); ok {
		return key, true
	}
	return TokenAPIKey{}, false
}
