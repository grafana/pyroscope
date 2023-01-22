package stackbuilder

type StackBuilder interface {
	Push(frame []byte)
	Pop() // bool
	Build() (stackID uint64)
	Reset()
}
