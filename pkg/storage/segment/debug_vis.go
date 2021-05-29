package segment

import (
	"encoding/json"
	"math/big"
	"os"
	"text/template"
	"time"
)

var visDebuggingEnabled = false

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

// This is here for debugging
func newVis() *vis {
	return &vis{nodes: []*visualizeNode2{}}
}

func (v *vis) add(n *streeNode, r *big.Rat, used bool) {
	if !visDebuggingEnabled {
		return
	}
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

func (v *vis) print(name string) {
	if !visDebuggingEnabled {
		return
	}
	vizTmplt, _ := template.New("viz").Parse(vizTmplt)

	jsonBytes, _ := json.MarshalIndent(v.nodes, "", "  ")
	jsonStr := string(jsonBytes)
	w, _ := os.Create(name)
	vizTmplt.Execute(w, TmpltVars{Data: jsonStr})
}

var vizTmplt = `
<html>

<script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/3.5.1/jquery.min.js"></script>
<script src="http://code.highcharts.com/highcharts.js"></script>
<script src="http://code.highcharts.com/highcharts-more.js"></script>
<script src="http://code.highcharts.com/modules/exporting.js"></script>
<script src="https://cdn.jsdelivr.net/npm/underscore@1.11.0/underscore-min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/moment.js/2.27.0/moment.min.js"></script>

<div id="container" style="min-width: 310px; height: 400px; margin: 0 auto"></div>

<script>
const data = {{ .Data }};

let lookup = {};

let formattedData = data;

formattedData.forEach(function(x){
  let k = [x.Depth, moment(x.T1).valueOf()].join("-");
  lookup[k] = x;
});

formattedData = formattedData.map(function(x){return [
  x.Depth, moment(x.T1).valueOf(), moment(x.T2).valueOf()
]});

$(function () {
  $('#container').highcharts({
		chart: {
			type: 'columnrange',
			inverted: true
		},

		title: {
			text: 'Segments'
		},

		xAxis: {
			categories: ['0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '10', '11']
		},

		yAxis: {
			title: {
				text: 'time'
			}
		},

		tooltip: {
			formatter: function (a,b,c) {
				let key = [this.x,this.y].join("-");
				let obj = lookup[key];
				return JSON.stringify(obj, null, 2);
			}
		},

		plotOptions: {
			columnrange: {
				dataLabels: {
					enabled: true,
					formatter: function () {
						return this.y;
					}
				}
			}
		},

		legend: {
			enabled: false
		},

		series: [{
			name: 'Segment',
			data: formattedData
		}]
  });
});
</script>
</html>
`
