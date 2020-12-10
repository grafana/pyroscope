package segment

import (
	"encoding/json"
	"io/ioutil"
	"math/big"
	"os"
	"text/template"
	"time"
)

type visualizeNode2 struct {
	T1      time.Time
	T2      time.Time
	Depth   int
	HasTrie bool
	Samples uint64
	M       int
	D       int
	Used    bool
}

type vis struct {
	nodes []*visualizeNode2
}

func newVis() *vis {
	return &vis{nodes: []*visualizeNode2{}}
}

func (v *vis) add(n *streeNode, r *big.Rat, used bool) {
	v.nodes = append(v.nodes, &visualizeNode2{
		T1:      n.time.UTC(),
		T2:      n.time.Add(durations[n.depth]).UTC(),
		Depth:   n.depth,
		HasTrie: n.present,
		Samples: n.samples,
		M:       int(r.Num().Int64()),
		D:       int(r.Denom().Int64()),
		Used:    used,
	})
}

type TmpltVars struct {
	Data string
}

// This is here for debugging
func (s *vis) print(name string) {
	f, _ := os.Open("/Users/dmitry/Dev/ps/other/viz.html")
	b, _ := ioutil.ReadAll(f)
	vizTmplt, _ := template.New("viz").Parse(string(b))

	jsonBytes, _ := json.MarshalIndent(s.nodes, "", "  ")
	jsonStr := string(jsonBytes)
	w, _ := os.Create(name)
	vizTmplt.Execute(w, TmpltVars{Data: jsonStr})
	f.Close()
}
