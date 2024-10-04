package adaptive_placement

import (
	"fmt"
	"image/color"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/ewma"
)

type testPlot struct {
	name   string
	source []float64

	ewma  *ewma.Rate
	alloc *shardAllocator

	statsUpdateInterval     time.Duration
	placementUpdateInterval time.Duration
}

func Test_generate_plots(t *testing.T) {
	t.Skip()

	steadyGrowthSource := generateData(
		[]float64{
			100, 120, 130, 170, 200, 220, 240, 290,
			400, 560, 610, 640, 650, 670, 675, 645,
			560, 255, 175, 120, 100, 90, 100, 110,
		},
		nil,   // Weights.
		1e4,   // Multiplier.
		86400, // 1s per point.
		80,    // Max variance.
	)

	steadyDecaySource := make([]float64, len(steadyGrowthSource))
	copy(steadyDecaySource, steadyGrowthSource)
	slices.Reverse(steadyDecaySource)

	defaultTest := func(fn func(*testPlot)) testPlot {
		p := testPlot{
			name:   "",
			source: nil,
			ewma:   ewma.NewHalfLife(time.Second * 180),
			alloc: &shardAllocator{
				unitSize:    uint64(256 << 10),
				min:         1,
				max:         32,
				burstWindow: (63 * time.Minute).Nanoseconds(),
				decayWindow: (67 * time.Minute).Nanoseconds(),
			},
			statsUpdateInterval:     time.Second * 10,
			placementUpdateInterval: time.Second * 30,
		}
		fn(&p)
		return p
	}

	for _, test := range []testPlot{
		defaultTest(func(t *testPlot) {
			t.name = "elephant_in_a_snake"
			t.source = steadyGrowthSource
		}),
		defaultTest(func(t *testPlot) {
			t.name = "elephant_in_a_snake_backwards"
			t.source = steadyDecaySource
		}),
		defaultTest(func(t *testPlot) {
			t.name = "extreme_spikes"
			t.alloc.max = 64
			t.source = generateData(
				[]float64{
					1, 1, 1000, 1, 1, 1000, 1, 1,
					1, 1, 1000, 1, 1, 1, 1, 1,
					1, 1, 1000, 1, 1, 1, 1, 1,
				},
				[]float64{
					1, .16, .16, 1, .16, .16, 1, 1,
					1, .16, .16, 1, 1, 1, 1, 1,
					1, .05, .05, 1, 1, 1, 1, 1,
				},
				1e4,
				86400,
				80,
			)
		}),
	} {
		buildPlot(t, test)
	}
}

func buildPlot(t *testing.T, test testPlot) {
	u := int(test.statsUpdateInterval.Seconds())
	a := int(test.placementUpdateInterval.Seconds())

	var now int64
	sourcePoints := make(plotter.XYs, len(test.source))
	aggregated := make(plotter.XYs, len(test.source)/u)
	allocated := make(plotter.XYs, len(test.source)/a)

	for i, v := range test.source {
		sourcePoints[i].X = float64(i * int(time.Second))
		sourcePoints[i].Y = v

		test.ewma.UpdateAt(v, now)
		if i%u == 0 {
			j := i / u
			aggregated[j].X = float64(now)
			aggregated[j].Y = test.ewma.ValueAt(now)
		}

		if i%a == 0 {
			j := i / a
			allocated[j].X = float64(now)
			x := uint64(test.ewma.ValueAt(now))
			allocated[j].Y = float64(test.alloc.observe(x, now))
		}

		now += int64(time.Second)
	}

	sourceLine, err := plotter.NewLine(sourcePoints)
	require.NoError(t, err)
	sourceLine.Width = vg.Points(0.1)
	sourceLine.Color = color.Gray{Y: 200}
	sourceLine.Dashes = []vg.Length{vg.Points(0.1), vg.Points(0.1)}

	aggregatedLine, err := plotter.NewLine(aggregated)
	require.NoError(t, err)
	aggregatedLine.Color = plotutil.Color(2)
	aggregatedLine.Width = vg.Points(0.5)

	multiply(allocated, float64(test.alloc.unitSize))
	allocatedLine, err := plotter.NewLine(allocated)
	require.NoError(t, err)
	allocatedLine.Color = plotutil.Color(7)
	allocatedLine.Width = vg.Points(1)

	allocatedPoints, err := plotter.NewScatter(allocated)
	require.NoError(t, err)
	allocatedPoints.Shape = draw.CircleGlyph{}
	allocatedPoints.Color = plotutil.Color(7)
	allocatedPoints.XYs = slices.CompactFunc(allocatedPoints.XYs, func(a, b plotter.XY) bool {
		return a.Y == b.Y
	})

	// Alloc labels.
	allocIndices := make([]string, len(allocatedPoints.XYs))
	allocShards := make([]string, len(allocatedPoints.XYs))
	for i := range allocIndices {
		allocIndices[i] = fmt.Sprint(i)
		allocShards[i] = fmt.Sprint(int(allocatedPoints.XYs[i].Y / float64(test.alloc.unitSize)))
	}

	allocIndexLabels, err := plotter.NewLabels(plotter.XYLabels{
		XYs:    allocatedPoints.XYs,
		Labels: allocIndices,
	})
	allocIndexLabels.XOffset = vg.Points(1)
	allocIndexLabels.YOffset = vg.Points(-7)
	for i := range allocIndexLabels.TextStyle {
		allocIndexLabels.TextStyle[i].Font.Size = vg.Points(7)
	}

	allocShardsLabels, err := plotter.NewLabels(plotter.XYLabels{
		XYs:    allocatedPoints.XYs,
		Labels: allocShards,
	})
	allocShardsLabels.XOffset = vg.Points(1)
	allocShardsLabels.YOffset = vg.Points(3)
	for i := range allocShardsLabels.TextStyle {
		allocShardsLabels.TextStyle[i].Color = allocatedPoints.Color
	}

	// Plot.
	p, err := plot.New()
	require.NoError(t, err)

	p.Add(sourceLine)
	p.Add(aggregatedLine)
	p.Add(allocatedLine, allocatedPoints, allocIndexLabels, allocShardsLabels)

	p.Title.Text = "Shard Allocation"
	p.X.Label.Text = "Time (hours)"
	p.X.Tick.Marker = ticker(3600 * int(time.Second))
	p.Y.Label.Text = "Data rate (MiB/s)"
	p.Y.Tick.Marker = ticker(1 << 20)

	require.NoError(t, p.Save(12*vg.Inch, 4*vg.Inch, "testdata/output/"+test.name+".png"))
}

func multiply(values []plotter.XY, m float64) plotter.XYs {
	for i, v := range values {
		values[i].Y = v.Y * m
	}
	return values
}

type ticker float64

func (t ticker) Ticks(min, max float64) []plot.Tick {
	h := float64(t)
	ticks := make([]plot.Tick, 0, int((max-min)/h))
	for i := min; i <= max; i += h {
		ticks = append(ticks, plot.Tick{
			Value: i,
			Label: fmt.Sprintf("%d", int(i/h)),
		},
		)
	}
	return ticks
}

func generateData(anchors []float64, weights []float64, multiplier float64, n int, step float64) []float64 {
	r := rand.New(rand.NewSource(123))
	ts := make([]float64, n)
	step *= multiplier
	ac := len(anchors)

	if weights == nil || len(weights) == 0 {
		weights = make([]float64, ac)
		for i := range weights {
			weights[i] = 1.0
		}
	}

	total := 0.0
	for _, w := range weights {
		total += w
	}

	points := make([]int, ac)
	for i := 0; i < ac; i++ {
		points[i] = int(weights[i] / total * float64(n))
	}

	if points[0] == 0 {
		points[0] = 1
	}

	var sum int
	for _, v := range points {
		sum += v
	}

	points[ac-1] += n - sum
	idx := 0
	for i := 0; i < ac-1; i++ {
		lower := anchors[i] * multiplier
		upper := anchors[i+1] * multiplier
		ps := points[i]

		for j := 0; j < ps; j++ {
			if idx >= n {
				break
			}
			t := float64(j) / float64(ps)
			mid := lower + t*(upper-lower)
			if idx == 0 {
				ts[idx] = lower + (r.Float64()*2*step-step)/2
			} else {
				varStep := r.Float64()*2*step - step
				ts[idx] = ts[idx-1] + varStep
			}
			ts[idx] = (ts[idx] + mid) / 2
			idx++
		}
	}

	if idx < n {
		lower := anchors[ac-1] * multiplier
		for idx < n {
			ts[idx] = lower + (r.Float64()*2*step-step)/2
			idx++
		}
	}

	return ts
}
