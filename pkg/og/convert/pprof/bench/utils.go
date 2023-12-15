package bench

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"golang.org/x/exp/slices"
)

func ReadGzipFile(f string) ([]byte, error) {
	fd, err := os.Open(f)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	g, err := gzip.NewReader(fd)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(g)

}

func WriteGzipFile(f string, data []byte) error {
	fd, err := os.OpenFile(f, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer fd.Close()
	g := gzip.NewWriter(fd)
	_, err = g.Write(data)
	if err != nil {
		return err
	}
	return g.Close()
}

func StackCollapseProto(p *profilev1.Profile, valueIDX int, scale float64) []string {
	type stack struct {
		funcs string
		value int64
	}
	locMap := make(map[int64]*profilev1.Location)
	funcMap := make(map[int64]*profilev1.Function)
	for _, l := range p.Location {
		locMap[int64(l.Id)] = l
	}
	for _, f := range p.Function {
		funcMap[int64(f.Id)] = f
	}

	var ret []stack
	for _, s := range p.Sample {
		var funcs []string
		for i := range s.LocationId {
			locID := s.LocationId[len(s.LocationId)-1-i]
			loc := locMap[int64(locID)]
			for _, line := range loc.Line {
				f := funcMap[int64(line.FunctionId)]
				fname := p.StringTable[f.Name]
				funcs = append(funcs, fname)
			}
		}
		v := s.Value[valueIDX]
		if scale != 1 {
			v = int64(float64(v) * scale)
		}
		ret = append(ret, stack{
			funcs: strings.Join(funcs, ";"),
			value: v,
		})
	}
	slices.SortFunc(ret, func(i, j stack) int {
		return strings.Compare(i.funcs, j.funcs)
	})
	var unique []stack
	for _, s := range ret {
		if s.value == 0 {
			continue
		}
		if len(unique) == 0 {
			unique = append(unique, s)
			continue
		}

		if unique[len(unique)-1].funcs == s.funcs {
			unique[len(unique)-1].value += s.value
			continue
		}
		unique = append(unique, s)

	}

	res := []string{}
	for _, s := range unique {
		res = append(res, fmt.Sprintf("%s %d", s.funcs, s.value))
	}
	return res
}
