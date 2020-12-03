package spy

import "fmt"

type Spy interface {
	Stop() error
	Snapshot(cb func([]byte, error))
}

type spyIntitializer func(pid int) (Spy, error)

var supportedSpiesMap map[string]spyIntitializer
var SupportedSpies []string

func init() {
	supportedSpiesMap = make(map[string]spyIntitializer)
}

func RegisterSpy(name string, cb spyIntitializer) {
	SupportedSpies = append(SupportedSpies, name)
	supportedSpiesMap[name] = cb
}

func SpyFromName(name string, pid int) (Spy, error) {
	if s, ok := supportedSpiesMap[name]; ok {
		return s(pid)
	}
	return nil, fmt.Errorf("unknown spy name %s", name)
}
