package int

type Map interface {
	Update(k, v any, flags int)
}
