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
	user, ok := ctx.Value(ctxUserKey).(User)
	return user, ok
}

func WithAPIKey(ctx context.Context, key APIKey) context.Context {
	return context.WithValue(ctx, ctxAPIKeyKey, key)
}

func APIKeyFromContext(ctx context.Context) (APIKey, bool) {
	key, ok := ctx.Value(ctxAPIKeyKey).(APIKey)
	return key, ok
}
