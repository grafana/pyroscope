package inout

type InOut struct {
	bodyCreator bodyCreator
}

func NewInOut() *InOut {
	return &InOut{
		bodyCreator: bodyCreator{},
	}
}
