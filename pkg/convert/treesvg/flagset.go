package treesvg

type flagset struct{}

func (f *flagset) Bool(name string, def bool, usage string) *bool {
	// logrus.Info(name)
	b := name == "svg"
	return &b
}
func (f *flagset) Int(name string, def int, usage string) *int {
	v := def
	return &v
}
func (f *flagset) Float64(name string, def float64, usage string) *float64 {
	v := def
	return &v
}
func (f *flagset) String(name string, def string, usage string) *string {
	v := def
	if name == "output" {
		v = "out.svg"
	}
	return &v
}
func (f *flagset) StringList(name string, def string, usage string) *[]*string {
	v := []*string{}
	return &v
}
func (f *flagset) ExtraUsage() string {
	return ""
}
func (f *flagset) AddExtraUsage(eu string) {
}
func (f *flagset) Parse(usage func()) []string {
	return []string{
		"input.pprof",
	}
}
