package spy

type Labels struct {
	m map[string]string
}

func NewLabels() *Labels {
	return &Labels{
		m: make(map[string]string),
	}
}

func (l *Labels) Set(key, val string) {
	l.m[key] = val
}

func (l *Labels) Tags() map[string]string {
	return l.m
}
