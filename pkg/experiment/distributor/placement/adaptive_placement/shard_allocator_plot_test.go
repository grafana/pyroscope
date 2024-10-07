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
	name        string
	description string

	source []float64
	ewma   *ewma.Rate
	alloc  *shardAllocator

	statsUpdateInterval     time.Duration
	placementUpdateInterval time.Duration

	// Might be too expensive to draw.
	hideSource bool
}

const randSeed = 752383033

func Test_generate_plots(t *testing.T) {
	t.Skip()

	const defaultUnitSize uint64 = 512 << 10
	defaultTest := func(fn func(*testPlot)) testPlot {
		p := testPlot{
			ewma: ewma.NewHalfLife(time.Second * 180),
			alloc: &shardAllocator{
				unitSize:    defaultUnitSize,
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

	shortWindowTest := func(fn func(*testPlot)) testPlot {
		p := defaultTest(fn)
		p.alloc.burstWindow = (17 * time.Minute).Nanoseconds()
		p.alloc.decayWindow = (19 * time.Minute).Nanoseconds()
		return p
	}

	drawPlots := func(t *testing.T, plots ...testPlot) {
		for _, p := range plots {
			t.Run(p.name, func(t *testing.T) {
				t.Parallel()
				drawPlot(t, p)
			})
		}
	}

	t.Run("elephant_in_a_snake", func(t *testing.T) {
		steadyGrowthSource := generateData(
			rand.New(rand.NewSource(randSeed)),
			[]float64{
				100, 120, 130, 170, 200, 220, 240, 290,
				400, 560, 610, 640, 650, 670, 675, 645,
				560, 255, 175, 120, 100, 90, 100, 110,
			}, // Anchors.
			nil,   // Weights.
			1e4,   // Multiplier.
			86400, // 1s per point.
			80,    // Max variance.
		)

		steepGrowthSource := make([]float64, len(steadyGrowthSource))
		copy(steepGrowthSource, steadyGrowthSource)
		slices.Reverse(steepGrowthSource)

		drawPlots(t,
			defaultTest(func(t *testPlot) {
				t.name = "steady_front"
				t.source = steadyGrowthSource
			}),
			shortWindowTest(func(t *testPlot) {
				t.name = "steady_front_2"
				t.source = steadyGrowthSource
			}),
			defaultTest(func(t *testPlot) {
				t.name = "steep_front"
				t.source = steepGrowthSource
			}),
			shortWindowTest(func(t *testPlot) {
				t.name = "steep_front_2"
				t.source = steepGrowthSource
			}),
		)
	})

	t.Run("extreme_spikes", func(t *testing.T) {
		extremeSpikes := generateData(
			rand.New(rand.NewSource(randSeed)),
			[]float64{
				1, 1, 1000, 1, 300, 1, 1000, 1,
				1, 1, 1000, 1, 1, 1, 1, 1,
				1, 1, 1000, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
			},
			[]float64{
				1, .1, .1, .1, .1, .1, .1, .1,
				1, .1, .1, .1, 1, 1, 1, 1,
				1, .01, .01, .01, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
			},
			1e4,
			86400,
			80,
		)

		drawPlots(t,
			defaultTest(func(t *testPlot) {
				t.name = "extreme_spikes"
				t.source = extremeSpikes
			}),
			shortWindowTest(func(t *testPlot) {
				t.name = "extreme_spikes_2"
				t.source = extremeSpikes
			}),
			shortWindowTest(func(t *testPlot) {
				t.name = "extreme_spikes_3"
				t.source = extremeSpikes
				t.alloc.max = 12
			}),
		)
	})

	t.Run("low_rate_oscillations", func(t *testing.T) {
		rnd := rand.New(rand.NewSource(randSeed))
		oscillations := make([]float64, 144)
		for i := range oscillations {
			v := 3 * float64(rnd.Intn(int(defaultUnitSize))) * rnd.Float64()
			oscillations[i] = v
		}
		oscillationWeights := make([]float64, len(oscillations))
		for i := range oscillationWeights {
			oscillationWeights[i] = rnd.Float64()
		}
		lowRateOscillations := generateData(
			rnd,
			oscillations,
			oscillationWeights,
			1,
			86400,
			2*float64(defaultUnitSize),
		)

		drawPlots(t,
			defaultTest(func(t *testPlot) {
				t.name = "low_rate_oscillations"
				t.source = lowRateOscillations
				//	t.hideSource = true
			}),
			shortWindowTest(func(t *testPlot) {
				t.name = "low_rate_oscillations_2"
				t.source = lowRateOscillations
				//	t.hideSource = true
			}),
		)
	})
}

func drawPlot(t *testing.T, test testPlot) {
	t.Helper()

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
	sourceLine.Color = color.Gray{Y: 220}

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
	allocIndexLabels.Offset = vg.Point{X: 1, Y: -7}
	for i := range allocIndexLabels.TextStyle {
		allocIndexLabels.TextStyle[i].Font.Size = vg.Points(7)
	}

	allocShardsLabels, err := plotter.NewLabels(plotter.XYLabels{
		XYs:    allocatedPoints.XYs,
		Labels: allocShards,
	})
	allocShardsLabels.Offset = vg.Point{X: 1, Y: 3}
	for i := range allocShardsLabels.TextStyle {
		allocShardsLabels.TextStyle[i].Color = allocatedPoints.Color
	}

	// Plot.
	p := plot.New()
	p.Title.Text = "Shard Allocation"
	p.X.Label.Text = "Time (hours)"
	p.X.Tick.Marker = timeTicker(time.Hour)
	p.Y.Label.Text = "Data rate (MiB/s)"
	p.Y.Tick.Marker = dataRateTicker(test.alloc.unitSize)

	grid := plotter.NewGrid()
	grid.Horizontal.Color = color.Gray{Y: 210}
	grid.Vertical.Width = 0
	p.Add(grid)

	if test.description != "" {
		p.Legend.Add(test.description)
		p.Legend.Top = true
		p.Legend.Padding = vg.Millimeter
		p.Legend.TextStyle.Font.Size = vg.Points(10)
	}

	if !test.hideSource {
		p.Add(sourceLine)
	}
	p.Add(aggregatedLine)
	p.Add(allocatedLine, allocatedPoints, allocIndexLabels, allocShardsLabels)

	require.NoError(t, p.Save(12*vg.Inch, 4*vg.Inch, "testdata/plots/"+test.name+".png"))
}

func multiply(values []plotter.XY, m float64) plotter.XYs {
	for i, v := range values {
		values[i].Y = v.Y * m
	}
	return values
}

type timeTicker float64

func (t timeTicker) Ticks(min, max float64) []plot.Tick {
	h := float64(t)
	ticks := make([]plot.Tick, 0, int((max-min)/h))
	for i := min; i <= max; i += h {
		ticks = append(ticks, plot.Tick{
			Value: i,
			Label: fmt.Sprintf("%d", int(i/h)),
		})
	}
	return ticks
}

type dataRateTicker int

func (t dataRateTicker) Ticks(min, max float64) []plot.Tick {
	mib := 1 << 20
	h := float64(t)
	ticks := make([]plot.Tick, 0, int((max-min)/h))
	for i := min; i <= max; i += h {
		tick := plot.Tick{Value: i}
		if int(tick.Value)%mib == 0 {
			tick.Label = fmt.Sprint(int(tick.Value) / mib)
		}
		ticks = append(ticks, tick)
	}
	return ticks
}

func generateData(
	rnd *rand.Rand,
	anchors []float64,
	weights []float64,
	multiplier float64,
	n int,
	step float64,
) []float64 {
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
				ts[idx] = lower + (rnd.Float64()*2*step-step)/2
			} else {
				varStep := rnd.Float64()*2*step - step
				ts[idx] = ts[idx-1] + varStep
			}
			ts[idx] = (ts[idx] + mid) / 2
			idx++
		}
	}

	if idx < n {
		lower := anchors[ac-1] * multiplier
		for idx < n {
			ts[idx] = lower + (rnd.Float64()*2*step-step)/2
			idx++
		}
	}

	for i := range ts {
		ts[i] = max(0, ts[i])
	}

	return ts
}
