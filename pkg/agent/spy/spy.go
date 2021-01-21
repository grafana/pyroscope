// Package spy contains an interface (Spy) and functionality to register new spies
package spy

import (
	"fmt"
)

type Spy interface {
	Stop() error
	Snapshot(cb func([]byte, error))
}

type spyIntitializer func(pid int) (Spy, error)

var supportedSpiesMap map[string]spyIntitializer
var SupportedSpies []string

var autoDetectionMapping = map[string]string{
	"python":  "pyspy",
	"python2": "pyspy",
	"python3": "pyspy",
	"uwsgi":   "pyspy",
	"pipenv":  "pyspy",

	"ruby":   "rbspy",
	"bundle": "rbspy",
	"rails":  "rbspy",
	"rake":   "rbspy",
}

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
	return nil, fmt.Errorf("unknown spy \"%s\". Make sure it's supported (run `pyroscope version` to check if your version supports it)", name)
}

func ResolveAutoName(s string) string {
	return autoDetectionMapping[s]
}

func SupportedExecSpies() []string {
	supportedSpies := []string{}
	for _, s := range SupportedSpies {
		if s != "gospy" {
			supportedSpies = append(supportedSpies, s)
		}
	}

	return supportedSpies
}
