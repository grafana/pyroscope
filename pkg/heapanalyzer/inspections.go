package heapanalyzer

import (
	"fmt"
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/core"
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/gocore"
	"unsafe"
)

type InspectionResult struct {
	InspectionName        string `json:"inspectionName"`
	InspectionDescription string `json:"inspectionDescription"`

	Findings []*InspectionFinding `json:"findings"`
}

type InspectionFinding struct {
	Value any `json:"value"`
}

type inspector interface {
	Consume(x gocore.Object) error
	GetResult() (*InspectionResult, error)
}

type duplicateStringInspector struct {
	gocore *gocore.Process

	itemCount map[string]*stringInstance
	topN      map[string]*stringInstance
	N         int
}

type stringInstance struct {
	count int64
	addr  []core.Address
}

func NewDuplicateStringInspector(gocore *gocore.Process) *duplicateStringInspector {
	d := &duplicateStringInspector{
		gocore:    gocore,
		itemCount: make(map[string]*stringInstance),
		topN:      make(map[string]*stringInstance),
		N:         10,
	}
	return d
}

func (d *duplicateStringInspector) Consume(x gocore.Object) error {
	tName := typeName(d.gocore, x)
	if tName != "string" {
		return nil
	}
	addr := d.gocore.Addr(x)
	n := d.gocore.Process().ReadInt(addr.Add(d.gocore.Process().PtrSize()))
	if n < 1 {
		return nil
	}
	// TODO: we could allocate a lot of memory here, maybe we want to only detect up to a certain size?
	b := make([]byte, n)
	d.gocore.Process().ReadAt(b, d.gocore.Process().ReadPtr(addr))
	str := *(*string)(unsafe.Pointer(&b))

	ins, ok := d.itemCount[str]
	if ok {
		d.itemCount[str].count += 1
		d.itemCount[str].addr = append(d.itemCount[str].addr, addr)
	} else {
		ins = &stringInstance{
			count: 1,
			addr:  []core.Address{addr},
		}
		d.itemCount[str] = ins
	}

	// maintain a short list of the worst offenders
	_, ok = d.topN[str]
	if !ok {
		if len(d.topN) < d.N {
			d.topN[str] = ins
		} else {
			for old, tN := range d.topN {
				if d.itemCount[str].count > tN.count {
					// we've found another entry with a lower count, swap them
					d.topN[str] = d.itemCount[str]
					delete(d.topN, old)
					break
				}
			}
		}
	}
	return nil
}

func (d *duplicateStringInspector) GetResult() (*InspectionResult, error) {
	findings := make([]*InspectionFinding, 0)
	for str, c := range d.topN {
		if c.count > 1 {
			objectIds := make([]string, len(c.addr))
			for i, a := range c.addr {
				objectIds[i] = fmt.Sprintf("%x", a)
			}
			findings = append(findings, &InspectionFinding{
				Value: map[string]any{
					"count":     fmt.Sprintf("%d", c.count),
					"string":    str,
					"objectIds": objectIds,
				},
			})
		}
	}
	ir := &InspectionResult{
		InspectionName:        "Duplicate strings",
		InspectionDescription: "Checks if memory is wasted with duplicate strings.",
		Findings:              findings,
	}
	return ir, nil
}
