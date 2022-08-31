package pprof

import "context"

const ctxPprofLineNumbers = "ctxPprofLineNumbers"

func WithLineNumbersEnabled(ctx context.Context, v bool) context.Context {
	return context.WithValue(ctx, ctxPprofLineNumbers, v)
}

func LineNumbersEnabledFromContext(ctx context.Context) bool {
	v, ok := ctx.Value(ctxPprofLineNumbers).(bool)
	return ok && v
}
