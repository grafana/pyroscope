// Copyright 2014 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package report summarizes a performance profile into a
// human-readable report.
package report

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/pprof/profile"

	"github.com/grafana/pyroscope/pkg/frontend/dot/graph"
	"github.com/grafana/pyroscope/pkg/frontend/dot/measurement"
)

// Options are the formatting and filtering options used to generate a
// profile.
type Options struct {
	OutputFormat int

	CumSort       bool
	CallTree      bool
	DropNegative  bool
	CompactLabels bool
	Ratio         float64
	Title         string
	ProfileLabels []string
	ActiveFilters []string
	NumLabelUnits map[string]string

	NodeCount    int
	NodeFraction float64
	EdgeFraction float64

	SampleValue       func(s []int64) int64
	SampleMeanDivisor func(s []int64) int64
	SampleType        string
	SampleUnit        string // Unit for the sample data from the profile.

	OutputUnit string // Units for data formatting in report.

	Symbol     *regexp.Regexp // Symbols to include on disassembly report.
	SourcePath string         // Search path for source files.
	TrimPath   string         // Paths to trim from source file paths.

	IntelSyntax bool // Whether to print assembly in Intel syntax.
}

// newTrimmedGraph creates a graph for this report, trimmed according
// to the report options.
func (rpt *Report) newTrimmedGraph() (g *graph.Graph, origCount, droppedNodes, droppedEdges int) {
	o := rpt.options

	// Build a graph and refine it. On each refinement step we must rebuild the graph from the samples,
	// as the graph itself doesn't contain enough information to preserve full precision.
	cumSort := o.CumSort

	// First step: Build complete graph to identify low frequency nodes, based on their cum weight.
	g = rpt.newGraph(nil)
	totalValue, _ := g.Nodes.Sum()
	nodeCutoff := abs64(int64(float64(totalValue) * o.NodeFraction))
	edgeCutoff := abs64(int64(float64(totalValue) * o.EdgeFraction))

	// Filter out nodes with cum value below nodeCutoff.
	if nodeCutoff > 0 {
		if nodesKept := g.DiscardLowFrequencyNodes(nodeCutoff); len(g.Nodes) != len(nodesKept) {
			droppedNodes = len(g.Nodes) - len(nodesKept)
			g = rpt.newGraph(nodesKept)
		}
	}
	origCount = len(g.Nodes)

	// Second step: Limit the total number of nodes. Apply specialized heuristics to improve
	// visualization when generating dot output.
	g.SortNodes(cumSort, true)
	if nodeCount := o.NodeCount; nodeCount > 0 {
		// Remove low frequency tags and edges as they affect selection.
		g.TrimLowFrequencyTags(nodeCutoff)
		g.TrimLowFrequencyEdges(edgeCutoff)
		if nodesKept := g.SelectTopNodes(nodeCount, true); len(g.Nodes) != len(nodesKept) {
			g = rpt.newGraph(nodesKept)
			g.SortNodes(cumSort, true)
		}
	}

	// Final step: Filter out low frequency tags and edges, and remove redundant edges that clutter
	// the graph.
	g.TrimLowFrequencyTags(nodeCutoff)
	droppedEdges = g.TrimLowFrequencyEdges(edgeCutoff)
	g.RemoveRedundantEdges()
	return
}

func (rpt *Report) selectOutputUnit(g *graph.Graph) {
	o := rpt.options

	// Select best unit for profile output.
	// Find the appropriate units for the smallest non-zero sample
	if o.OutputUnit != "minimum" || len(g.Nodes) == 0 {
		return
	}
	var minValue int64

	for _, n := range g.Nodes {
		nodeMin := abs64(n.FlatValue())
		if nodeMin == 0 {
			nodeMin = abs64(n.CumValue())
		}
		if nodeMin > 0 && (minValue == 0 || nodeMin < minValue) {
			minValue = nodeMin
		}
	}
	maxValue := rpt.total
	if minValue == 0 {
		minValue = maxValue
	}

	if r := o.Ratio; r > 0 && r != 1 {
		minValue = int64(float64(minValue) * r)
		maxValue = int64(float64(maxValue) * r)
	}

	_, minUnit := measurement.Scale(minValue, o.SampleUnit, "minimum")
	_, maxUnit := measurement.Scale(maxValue, o.SampleUnit, "minimum")

	unit := minUnit
	if minUnit != maxUnit && minValue*100 < maxValue {
		// Minimum and maximum values have different units. Scale
		// minimum by 100 to use larger units, allowing minimum value to
		// be scaled down to 0.01, except for callgrind reports since
		// they can only represent integer values.
		_, unit = measurement.Scale(100*minValue, o.SampleUnit, "minimum")
	}

	if unit != "" {
		o.OutputUnit = unit
	} else {
		o.OutputUnit = o.SampleUnit
	}
}

// newGraph creates a new graph for this report. If nodes is non-nil,
// only nodes whose info matches are included. Otherwise, all nodes
// are included, without trimming.
func (rpt *Report) newGraph(nodes graph.NodeSet) *graph.Graph {
	o := rpt.options

	// Clean up file paths using heuristics.
	prof := rpt.prof
	for _, f := range prof.Function {
		f.Filename = trimPath(f.Filename, o.TrimPath, o.SourcePath)
	}
	// Removes all numeric tags except for the bytes tag prior
	// to making graph.
	// TODO: modify to select first numeric tag if no bytes tag
	for _, s := range prof.Sample {
		numLabels := make(map[string][]int64, len(s.NumLabel))
		numUnits := make(map[string][]string, len(s.NumLabel))
		for k, vs := range s.NumLabel {
			if k == "bytes" {
				unit := o.NumLabelUnits[k]
				numValues := make([]int64, len(vs))
				numUnit := make([]string, len(vs))
				for i, v := range vs {
					numValues[i] = v
					numUnit[i] = unit
				}
				numLabels[k] = append(numLabels[k], numValues...)
				numUnits[k] = append(numUnits[k], numUnit...)
			}
		}
		s.NumLabel = numLabels
		s.NumUnit = numUnits
	}

	// Remove label marking samples from the base profiles, so it does not appear
	// as a nodelet in the graph view.
	prof.RemoveLabel("pprof::base")

	formatTag := func(v int64, key string) string {
		return measurement.ScaledLabel(v, key, o.OutputUnit)
	}

	gopt := &graph.Options{
		SampleValue:       o.SampleValue,
		SampleMeanDivisor: o.SampleMeanDivisor,
		FormatTag:         formatTag,
		CallTree:          false,
		DropNegative:      o.DropNegative,
		KeptNodes:         nodes,
	}

	return graph.New(rpt.prof, gopt)
}

// TextItem holds a single text report entry.
type TextItem struct {
	Name                  string
	InlineLabel           string // Not empty if inlined
	Flat, Cum             int64  // Raw values
	FlatFormat, CumFormat string // Formatted values
}

// GetDOT returns a graph suitable for dot processing along with some
// configuration information.
func GetDOT(rpt *Report) (*graph.Graph, *graph.DotConfig) {
	g, origCount, droppedNodes, droppedEdges := rpt.newTrimmedGraph()
	rpt.selectOutputUnit(g)
	labels := reportLabels(rpt, g, origCount, droppedNodes, droppedEdges, true)

	c := &graph.DotConfig{
		Title:       rpt.options.Title,
		Labels:      labels,
		FormatValue: rpt.formatValue,
		Total:       rpt.total,
	}
	return g, c
}

// ProfileLabels returns printable labels for a profile.
func ProfileLabels(rpt *Report) []string {
	var label []string
	prof := rpt.prof
	o := rpt.options
	if len(prof.Mapping) > 0 {
		if prof.Mapping[0].File != "" {
			label = append(label, "File: "+filepath.Base(prof.Mapping[0].File))
		}
		if prof.Mapping[0].BuildID != "" {
			label = append(label, "Build ID: "+prof.Mapping[0].BuildID)
		}
	}
	// Only include comments that do not start with '#'.
	for _, c := range prof.Comments {
		if !strings.HasPrefix(c, "#") {
			label = append(label, c)
		}
	}
	if o.SampleType != "" {
		label = append(label, "Type: "+o.SampleType)
	}
	if prof.TimeNanos != 0 {
		const layout = "Jan 2, 2006 at 3:04pm (MST)"
		label = append(label, "Time: "+time.Unix(0, prof.TimeNanos).Format(layout))
	}
	if prof.DurationNanos != 0 {
		duration := measurement.Label(prof.DurationNanos, "nanoseconds")
		totalNanos, totalUnit := measurement.Scale(rpt.total, o.SampleUnit, "nanoseconds")
		var ratio string
		if totalUnit == "ns" && totalNanos != 0 {
			ratio = "(" + measurement.Percentage(int64(totalNanos), prof.DurationNanos) + ")"
		}
		label = append(label, fmt.Sprintf("Duration: %s, Total samples = %s %s", duration, rpt.formatValue(rpt.total), ratio))
	}
	return label
}

// reportLabels returns printable labels for a report. Includes
// profileLabels.
func reportLabels(rpt *Report, g *graph.Graph, origCount, droppedNodes, droppedEdges int, fullHeaders bool) []string {
	nodeFraction := rpt.options.NodeFraction
	edgeFraction := rpt.options.EdgeFraction
	nodeCount := len(g.Nodes)

	var label []string
	if len(rpt.options.ProfileLabels) > 0 {
		label = append(label, rpt.options.ProfileLabels...)
	} else if fullHeaders || !rpt.options.CompactLabels {
		label = ProfileLabels(rpt)
	}

	var flatSum int64
	for _, n := range g.Nodes {
		flatSum = flatSum + n.FlatValue()
	}

	if len(rpt.options.ActiveFilters) > 0 {
		activeFilters := legendActiveFilters(rpt.options.ActiveFilters)
		label = append(label, activeFilters...)
	}

	label = append(label, fmt.Sprintf("Showing nodes accounting for %s, %s of %s total", rpt.formatValue(flatSum), strings.TrimSpace(measurement.Percentage(flatSum, rpt.total)), rpt.formatValue(rpt.total)))

	if rpt.total != 0 {
		if droppedNodes > 0 {
			label = append(label, genLabel(droppedNodes, "node", "cum",
				rpt.formatValue(abs64(int64(float64(rpt.total)*nodeFraction)))))
		}
		if droppedEdges > 0 {
			label = append(label, genLabel(droppedEdges, "edge", "freq",
				rpt.formatValue(abs64(int64(float64(rpt.total)*edgeFraction)))))
		}
		if nodeCount > 0 && nodeCount < origCount {
			label = append(label, fmt.Sprintf("Showing top %d nodes out of %d",
				nodeCount, origCount))
		}
	}

	// Help new users understand the graph.
	// A new line is intentionally added here to better show this message.
	if fullHeaders {
		label = append(label, "\nSee https://git.io/JfYMW for how to read the graph")
	}

	return label
}

func legendActiveFilters(activeFilters []string) []string {
	legendActiveFilters := make([]string, len(activeFilters)+1)
	legendActiveFilters[0] = "Active filters:"
	for i, s := range activeFilters {
		if len(s) > 80 {
			s = s[:80] + "…"
		}
		legendActiveFilters[i+1] = "   " + s
	}
	return legendActiveFilters
}

func genLabel(d int, n, l, f string) string {
	if d > 1 {
		n = n + "s"
	}
	return fmt.Sprintf("Dropped %d %s (%s <= %s)", d, n, l, f)
}

// New builds a new report indexing the sample values interpreting the
// samples with the provided function.
func New(prof *profile.Profile, o *Options) *Report {
	format := func(v int64) string {
		if r := o.Ratio; r > 0 && r != 1 {
			fv := float64(v) * r
			v = int64(fv)
		}
		return measurement.ScaledLabel(v, o.SampleUnit, o.OutputUnit)
	}
	return &Report{prof, computeTotal(prof, o.SampleValue, o.SampleMeanDivisor),
		o, format}
}

// NewDefault builds a new report indexing the last sample value
// available.
func NewDefault(prof *profile.Profile, options Options) *Report {
	index := len(prof.SampleType) - 1
	o := &options
	if o.Title == "" && len(prof.Mapping) > 0 && prof.Mapping[0].File != "" {
		o.Title = filepath.Base(prof.Mapping[0].File)
	}
	o.SampleType = prof.SampleType[index].Type
	o.SampleUnit = strings.ToLower(prof.SampleType[index].Unit)
	o.SampleValue = func(v []int64) int64 {
		return v[index]
	}
	return New(prof, o)
}

// computeTotal computes the sum of the absolute value of all sample values.
// If any samples have label indicating they belong to the diff base, then the
// total will only include samples with that label.
func computeTotal(prof *profile.Profile, value, meanDiv func(v []int64) int64) int64 {
	var div, total, diffDiv, diffTotal int64
	for _, sample := range prof.Sample {
		var d, v int64
		v = value(sample.Value)
		if meanDiv != nil {
			d = meanDiv(sample.Value)
		}
		if v < 0 {
			v = -v
		}
		total += v
		div += d
		if sample.DiffBaseSample() {
			diffTotal += v
			diffDiv += d
		}
	}
	if diffTotal > 0 {
		total = diffTotal
		div = diffDiv
	}
	if div != 0 {
		return total / div
	}
	return total
}

// Report contains the data and associated routines to extract a
// report from a profile.
type Report struct {
	prof        *profile.Profile
	total       int64
	options     *Options
	formatValue func(int64) string
}

// Total returns the total number of samples in a report.
func (rpt *Report) Total() int64 { return rpt.total }

func abs64(i int64) int64 {
	if i < 0 {
		return -i
	}
	return i
}

func trimPath(path, trimPath, searchPath string) string {
	// Keep path variable intact as it's used below to form the return value.
	sPath, searchPath := filepath.ToSlash(path), filepath.ToSlash(searchPath)
	if trimPath == "" {
		// If the trim path is not configured, try to guess it heuristically:
		// search for basename of each search path in the original path and, if
		// found, strip everything up to and including the basename. So, for
		// example, given original path "/some/remote/path/my-project/foo/bar.c"
		// and search path "/my/local/path/my-project" the heuristic will return
		// "/my/local/path/my-project/foo/bar.c".
		for _, dir := range filepath.SplitList(searchPath) {
			want := "/" + filepath.Base(dir) + "/"
			if found := strings.Index(sPath, want); found != -1 {
				return path[found+len(want):]
			}
		}
	}
	// Trim configured trim prefixes.
	trimPaths := append(filepath.SplitList(filepath.ToSlash(trimPath)), "/proc/self/cwd/./", "/proc/self/cwd/")
	for _, trimPath := range trimPaths {
		if !strings.HasSuffix(trimPath, "/") {
			trimPath += "/"
		}
		if strings.HasPrefix(sPath, trimPath) {
			return path[len(trimPath):]
		}
	}
	return path
}
