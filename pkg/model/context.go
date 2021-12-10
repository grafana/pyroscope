package model

import "context"

var ctxUserKey struct{}

func UserFromContext(ctx context.Context) (User, bool) {
	if user, ok := ctx.Value(ctxUserKey).(User); ok {
		return user, true
	}
	return User{}, false
}

func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, ctxUserKey, user)
}
