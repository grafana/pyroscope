package heapanalyzer

import (
	"fmt"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/core"
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/gocore"
)

type Dump struct {
	l        log.Logger
	exePath  string
	corePath string
	core     *core.Process
	gocore   *gocore.Process
	info     *HeapDump
}

func NewDump(l log.Logger, exePath string, corePath string, info *HeapDump) (*Dump, error) {
	c, err := core.Core(corePath, "", exePath)
	if err != nil {
		return nil, err
	}
	p, err := gocore.Core(c)
	if err != nil {
		return nil, err
	}
	d := &Dump{
		l:        l,
		exePath:  exePath,
		corePath: corePath,
		core:     c,
		gocore:   p,
		info:     info,
	}
	err = d.InitHeap()
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Dump) InitHeap() (err error) {
	defer func() {
		if r := recover(); r != nil {
			level.Error(d.l).Log("msg", "recovered from panic", "panic", r)
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	d.gocore.TypeHeap()
	return err
}

func (d *Dump) ObjectTypes() []ObjectTypeStats {
	return nil
}
