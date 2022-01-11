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

func MustUserFromContext(ctx context.Context) User {
	u, ok := UserFromContext(ctx)
	if !ok {
		panic("user not found in context")
	}
	return u
}

func WithAPIKey(ctx context.Context, key APIKey) context.Context {
	return context.WithValue(ctx, ctxAPIKeyKey, key)
}

func APIKeyFromContext(ctx context.Context) (APIKey, bool) {
	if key, ok := ctx.Value(ctxAPIKeyKey).(APIKey); ok {
		return key, true
	}
	return APIKey{}, false
}
