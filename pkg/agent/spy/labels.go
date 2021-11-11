package spy

type Labels struct {
	m map[string]string
	s string
}

func NewLabels() *Labels {
	return &Labels{
		m: make(map[string]string),
		s: "",
	}
}

func (l *Labels) Set(key, val string) {
	l.m[key] = val
	l.s += key
	l.s += ":"
	l.s += val
	l.s += "\n"
}

func (l *Labels) Tags() map[string]string {
	return l.m
}

func (l *Labels) ID() string {
	return l.s
}
