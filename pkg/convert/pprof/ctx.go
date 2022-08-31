package pprof

import "context"

type ctxPprofLineNumbersKey string

const ctxPprofLineNumbers ctxPprofLineNumbersKey = "ctxPprofLineNumbers"

func WithLineNumbersEnabled(ctx context.Context, v bool) context.Context {
	return context.WithValue(ctx, ctxPprofLineNumbers, v)
}

func LineNumbersEnabledFromContext(ctx context.Context) bool {
	v, ok := ctx.Value(ctxPprofLineNumbers).(bool)
	return ok && v
}
