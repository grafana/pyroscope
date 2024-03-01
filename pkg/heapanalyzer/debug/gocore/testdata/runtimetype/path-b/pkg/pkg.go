package pkg

type T1 struct {
	f1 uint64
	f2 uint64
	f3 uint64
	f4 uint64
}

type T2 struct {
	f1 uint64
	f2 uint64
	f3 uint64
}

type IfaceDirect interface {
	M1()
}

type IfaceInDirect interface {
	M2()
}

func (t *T1) M1() {
}

func (t T2) M2() {
}

func NewIfaceDirect() IfaceDirect {
	return &T1{1000, 0, 0, 0}
}

func NewIfaceInDirect() IfaceInDirect {
	return T2{2000, 0, 0}
}
