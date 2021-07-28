package main

import (
	"encoding/json"
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
				{0, 1, 1, 3, 2, 0, 2, 2}, // b, c
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
          "name": "total",
          "total": 3,
          "self": 0
        }
      ],
      [
        {
          "name": "a",
          "total": 3,
          "self": 0
        }
      ],
      [
        {
          "name": "c",
          "total": 1,
          "self": 1
        },
        {
          "name": "b",
          "total": 2,
          "self": 2
        }
      ]
    ]
  }
}`
			Expect(outJSON).To(Equal(expected))
		})
	})
})

func marshal(out interface{}) string {
	data, _ := json.MarshalIndent(out, "", "  ")
	return strings.TrimSpace(string(data))
}
