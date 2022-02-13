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

func WithAPIKey(ctx context.Context, key APIKeyToken) context.Context {
	return context.WithValue(ctx, ctxAPIKeyKey, key)
}

func APIKeyFromContext(ctx context.Context) (APIKeyToken, bool) {
	if key, ok := ctx.Value(ctxAPIKeyKey).(APIKeyToken); ok {
		return key, true
	}
	return APIKeyToken{}, false
}
