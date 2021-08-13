package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Decode", func() {
	Context("simple case", func() {
		It("should decode correctly", func() {
			names := strings.Split("total,a,b,c", ",")
			levels := [][]int{
				{0, 3, 0, 0},             // total
				{0, 3, 0, 1},             // a
				{0, 1, 1, 3, 2, 2, 2, 2}, // b, c
			}
			in := &Input{
				Flamebearer: &tree.Flamebearer{
					Names:  names,
					Levels: levels,
				},
			}
			out := decodeLevels(in)
			outJSON := marshal(out)
			expected := `{
  "flamebearer": {
    "levels": [
      [
        {
          "_row": 0,
          "_col": 0,
          "name": "total",
          "total": 3,
          "self": 0,
          "offset": 0
        }
      ],
      [
        {
          "_row": 1,
          "_col": 0,
          "name": "a",
          "total": 3,
          "self": 0,
          "offset": 0
        }
      ],
      [
        {
          "_row": 2,
          "_col": 0,
          "name": "c",
          "total": 1,
          "self": 1,
          "offset": 0
        },
        {
          "_row": 2,
          "_col": 1,
          "name": "b",
          "total": 2,
          "self": 2,
          "offset": 2
        }
      ]
    ]
  }
}`
 			fmt.Println(outJSON)
			Expect(outJSON).To(Equal(expected))
		})
	})
})

func marshal(out interface{}) string {
	data, _ := json.MarshalIndent(out, "", "  ")
	return strings.TrimSpace(string(data))
}
