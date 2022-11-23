package server

import "context"

type maxNodesKeyType int

const currentMaxNodes maxNodesKeyType = iota

func ContextWithMaxNodes(parent context.Context, val int) context.Context {
	return context.WithValue(parent, currentMaxNodes, val)
}

func MaxNodesFromContext(ctx context.Context) (int, bool) {
	v, ok := ctx.Value(currentMaxNodes).(int)
	return v, ok
}
