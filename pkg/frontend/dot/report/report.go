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
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
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

	IntelSyntax bool // Whether or not to print assembly in Intel syntax.
}

// newTrimmedGraph creates a graph for this report, trimmed according
// to the report options.
func (rpt *Report) newTrimmedGraph() (g *graph.Graph, origCount, droppedNodes, droppedEdges int) {
	o := rpt.options

	// Build a graph and refine it. On each refinement step we must rebuild the graph from the samples,
	// as the graph itself doesn't contain enough information to preserve full precision.
	visualMode := true
	cumSort := o.CumSort

	// The call_tree option is only honored when generating visual representations of the callgraph.
	callTree := false

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
	g.SortNodes(cumSort, visualMode)
	if nodeCount := o.NodeCount; nodeCount > 0 {
		// Remove low frequency tags and edges as they affect selection.
		g.TrimLowFrequencyTags(nodeCutoff)
		g.TrimLowFrequencyEdges(edgeCutoff)
		if callTree {
			if nodesKept := g.SelectTopNodePtrs(nodeCount, visualMode); len(g.Nodes) != len(nodesKept) {
				g.TrimTree(nodesKept)
				g.SortNodes(cumSort, visualMode)
			}
		} else {
			if nodesKept := g.SelectTopNodes(nodeCount, visualMode); len(g.Nodes) != len(nodesKept) {
				g = rpt.newGraph(nodesKept)
				g.SortNodes(cumSort, visualMode)
			}
		}
	}

	// Final step: Filter out low frequency tags and edges, and remove redundant edges that clutter
	// the graph.
	g.TrimLowFrequencyTags(nodeCutoff)
	droppedEdges = g.TrimLowFrequencyEdges(edgeCutoff)
	if visualMode {
		g.RemoveRedundantEdges()
	}
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

// printProto writes the incoming proto via thw writer w.
// If the divide_by option has been specified, samples are scaled appropriately.
func printProto(w io.Writer, rpt *Report) error {
	p, o := rpt.prof, rpt.options

	// Apply the sample ratio to all samples before saving the profile.
	if r := o.Ratio; r > 0 && r != 1 {
		for _, sample := range p.Sample {
			for i, v := range sample.Value {
				sample.Value[i] = int64(float64(v) * r)
			}
		}
	}
	return p.Write(w)
}

// printTopProto writes a list of the hottest routines in a profile as a profile.proto.
func printTopProto(w io.Writer, rpt *Report) error {
	p := rpt.prof
	o := rpt.options
	g, _, _, _ := rpt.newTrimmedGraph()
	rpt.selectOutputUnit(g)

	out := profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "cum", Unit: o.OutputUnit},
			{Type: "flat", Unit: o.OutputUnit},
		},
		TimeNanos:     p.TimeNanos,
		DurationNanos: p.DurationNanos,
		PeriodType:    p.PeriodType,
		Period:        p.Period,
	}
	functionMap := make(functionMap)
	for i, n := range g.Nodes {
		f, added := functionMap.findOrAdd(n.Info)
		if added {
			out.Function = append(out.Function, f)
		}
		flat, cum := n.FlatValue(), n.CumValue()
		l := &profile.Location{
			ID:      uint64(i + 1),
			Address: n.Info.Address,
			Line: []profile.Line{
				{
					Line:     int64(n.Info.Lineno),
					Function: f,
				},
			},
		}

		fv, _ := measurement.Scale(flat, o.SampleUnit, o.OutputUnit)
		cv, _ := measurement.Scale(cum, o.SampleUnit, o.OutputUnit)
		s := &profile.Sample{
			Location: []*profile.Location{l},
			Value:    []int64{int64(cv), int64(fv)},
		}
		out.Location = append(out.Location, l)
		out.Sample = append(out.Sample, s)
	}

	return out.Write(w)
}

type functionMap map[string]*profile.Function

// findOrAdd takes a node representing a function, adds the function
// represented by the node to the map if the function is not already present,
// and returns the function the node represents. This also returns a boolean,
// which is true if the function was added and false otherwise.
func (fm functionMap) findOrAdd(ni graph.NodeInfo) (*profile.Function, bool) {
	fName := fmt.Sprintf("%q%q%q%d", ni.Name, ni.OrigName, ni.File, ni.StartLine)

	if f := fm[fName]; f != nil {
		return f, false
	}

	f := &profile.Function{
		ID:         uint64(len(fm) + 1),
		Name:       ni.Name,
		SystemName: ni.OrigName,
		Filename:   ni.File,
		StartLine:  int64(ni.StartLine),
	}
	fm[fName] = f
	return f, true
}

type assemblyInstruction struct {
	address         uint64
	instruction     string
	function        string
	file            string
	line            int
	flat, cum       int64
	flatDiv, cumDiv int64
	startsBlock     bool
	inlineCalls     []callID
}

type callID struct {
	file string
	line int
}

func (a *assemblyInstruction) flatValue() int64 {
	if a.flatDiv != 0 {
		return a.flat / a.flatDiv
	}
	return a.flat
}

// valueOrDot formats a value according to a report, intercepting zero
// values.
func valueOrDot(value int64, rpt *Report) string {
	if value == 0 {
		return "."
	}
	return rpt.formatValue(value)
}

// printTags collects all tags referenced in the profile and prints
// them in a sorted table.
func printTags(w io.Writer, rpt *Report) error {
	p := rpt.prof

	o := rpt.options
	formatTag := func(v int64, key string) string {
		return measurement.ScaledLabel(v, key, o.OutputUnit)
	}

	// Hashtable to keep accumulate tags as key,value,count.
	tagMap := make(map[string]map[string]int64)
	for _, s := range p.Sample {
		for key, vals := range s.Label {
			for _, val := range vals {
				valueMap, ok := tagMap[key]
				if !ok {
					valueMap = make(map[string]int64)
					tagMap[key] = valueMap
				}
				valueMap[val] += o.SampleValue(s.Value)
			}
		}
		for key, vals := range s.NumLabel {
			unit := o.NumLabelUnits[key]
			for _, nval := range vals {
				val := formatTag(nval, unit)
				valueMap, ok := tagMap[key]
				if !ok {
					valueMap = make(map[string]int64)
					tagMap[key] = valueMap
				}
				valueMap[val] += o.SampleValue(s.Value)
			}
		}
	}

	tagKeys := make([]*graph.Tag, 0, len(tagMap))
	for key := range tagMap {
		tagKeys = append(tagKeys, &graph.Tag{Name: key})
	}
	tabw := tabwriter.NewWriter(w, 0, 0, 1, ' ', tabwriter.AlignRight)
	for _, tagKey := range graph.SortTags(tagKeys, true) {
		var total int64
		key := tagKey.Name
		tags := make([]*graph.Tag, 0, len(tagMap[key]))
		for t, c := range tagMap[key] {
			total += c
			tags = append(tags, &graph.Tag{Name: t, Flat: c})
		}

		f, u := measurement.Scale(total, o.SampleUnit, o.OutputUnit)
		fmt.Fprintf(tabw, "%s:\t Total %.1f%s\n", key, f, u)
		for _, t := range graph.SortTags(tags, true) {
			f, u := measurement.Scale(t.FlatValue(), o.SampleUnit, o.OutputUnit)
			if total > 0 {
				fmt.Fprintf(tabw, " \t%.1f%s (%s):\t %s\n", f, u, measurement.Percentage(t.FlatValue(), total), t.Name)
			} else {
				fmt.Fprintf(tabw, " \t%.1f%s:\t %s\n", f, u, t.Name)
			}
		}
		fmt.Fprintln(tabw)
	}
	return tabw.Flush()
}

// printComments prints all freeform comments in the profile.
func printComments(w io.Writer, rpt *Report) error {
	p := rpt.prof

	for _, c := range p.Comments {
		fmt.Fprintln(w, c)
	}
	return nil
}

// TextItem holds a single text report entry.
type TextItem struct {
	Name                  string
	InlineLabel           string // Not empty if inlined
	Flat, Cum             int64  // Raw values
	FlatFormat, CumFormat string // Formatted values
}

// TextItems returns a list of text items from the report and a list
// of labels that describe the report.
func TextItems(rpt *Report) ([]TextItem, []string) {
	g, origCount, droppedNodes, _ := rpt.newTrimmedGraph()
	rpt.selectOutputUnit(g)
	labels := reportLabels(rpt, g, origCount, droppedNodes, 0, false)

	var items []TextItem
	var flatSum int64
	for _, n := range g.Nodes {
		name, flat, cum := n.Info.PrintableName(), n.FlatValue(), n.CumValue()

		var inline, noinline bool
		for _, e := range n.In {
			if e.Inline {
				inline = true
			} else {
				noinline = true
			}
		}

		var inl string
		if inline {
			if noinline {
				inl = "(partial-inline)"
			} else {
				inl = "(inline)"
			}
		}

		flatSum += flat
		items = append(items, TextItem{
			Name:        name,
			InlineLabel: inl,
			Flat:        flat,
			Cum:         cum,
			FlatFormat:  rpt.formatValue(flat),
			CumFormat:   rpt.formatValue(cum),
		})
	}
	return items, labels
}

// printText prints a flat text report for a profile.
func printText(w io.Writer, rpt *Report) error {
	items, labels := TextItems(rpt)
	fmt.Fprintln(w, strings.Join(labels, "\n"))
	fmt.Fprintf(w, "%10s %5s%% %5s%% %10s %5s%%\n",
		"flat", "flat", "sum", "cum", "cum")
	var flatSum int64
	for _, item := range items {
		inl := item.InlineLabel
		if inl != "" {
			inl = " " + inl
		}
		flatSum += item.Flat
		fmt.Fprintf(w, "%10s %s %s %10s %s  %s%s\n",
			item.FlatFormat, measurement.Percentage(item.Flat, rpt.total),
			measurement.Percentage(flatSum, rpt.total),
			item.CumFormat, measurement.Percentage(item.Cum, rpt.total),
			item.Name, inl)
	}
	return nil
}

// printTraces prints all traces from a profile.
func printTraces(w io.Writer, rpt *Report) error {
	fmt.Fprintln(w, strings.Join(ProfileLabels(rpt), "\n"))

	prof := rpt.prof
	o := rpt.options

	const separator = "-----------+-------------------------------------------------------"

	_, locations := graph.CreateNodes(prof, &graph.Options{})
	for _, sample := range prof.Sample {
		type stk struct {
			*graph.NodeInfo
			inline bool
		}
		var stack []stk
		for _, loc := range sample.Location {
			nodes := locations[loc.ID]
			for i, n := range nodes {
				// The inline flag may be inaccurate if 'show' or 'hide' filter is
				// used. See https://github.com/google/pprof/issues/511.
				inline := i != len(nodes)-1
				stack = append(stack, stk{&n.Info, inline})
			}
		}

		if len(stack) == 0 {
			continue
		}

		fmt.Fprintln(w, separator)
		// Print any text labels for the sample.
		var labels []string
		for s, vs := range sample.Label {
			labels = append(labels, fmt.Sprintf("%10s:  %s\n", s, strings.Join(vs, " ")))
		}
		sort.Strings(labels)
		fmt.Fprint(w, strings.Join(labels, ""))

		// Print any numeric labels for the sample
		var numLabels []string
		for key, vals := range sample.NumLabel {
			unit := o.NumLabelUnits[key]
			numValues := make([]string, len(vals))
			for i, vv := range vals {
				numValues[i] = measurement.Label(vv, unit)
			}
			numLabels = append(numLabels, fmt.Sprintf("%10s:  %s\n", key, strings.Join(numValues, " ")))
		}
		sort.Strings(numLabels)
		fmt.Fprint(w, strings.Join(numLabels, ""))

		var d, v int64
		v = o.SampleValue(sample.Value)
		if o.SampleMeanDivisor != nil {
			d = o.SampleMeanDivisor(sample.Value)
		}
		// Print call stack.
		if d != 0 {
			v = v / d
		}
		for i, s := range stack {
			var vs, inline string
			if i == 0 {
				vs = rpt.formatValue(v)
			}
			if s.inline {
				inline = " (inline)"
			}
			fmt.Fprintf(w, "%10s   %s%s\n", vs, s.PrintableName(), inline)
		}
	}
	fmt.Fprintln(w, separator)
	return nil
}

// printCallgrind prints a graph for a profile on callgrind format.
func printCallgrind(w io.Writer, rpt *Report) error {
	o := rpt.options
	rpt.options.NodeFraction = 0
	rpt.options.EdgeFraction = 0
	rpt.options.NodeCount = 0

	g, _, _, _ := rpt.newTrimmedGraph()
	rpt.selectOutputUnit(g)

	nodeNames := getDisambiguatedNames(g)

	fmt.Fprintln(w, "positions: instr line")
	fmt.Fprintln(w, "events:", o.SampleType+"("+o.OutputUnit+")")

	objfiles := make(map[string]int)
	files := make(map[string]int)
	names := make(map[string]int)

	// prevInfo points to the previous NodeInfo.
	// It is used to group cost lines together as much as possible.
	var prevInfo *graph.NodeInfo
	for _, n := range g.Nodes {
		if prevInfo == nil || n.Info.Objfile != prevInfo.Objfile || n.Info.File != prevInfo.File || n.Info.Name != prevInfo.Name {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "ob="+callgrindName(objfiles, n.Info.Objfile))
			fmt.Fprintln(w, "fl="+callgrindName(files, n.Info.File))
			fmt.Fprintln(w, "fn="+callgrindName(names, n.Info.Name))
		}

		addr := callgrindAddress(prevInfo, n.Info.Address)
		sv, _ := measurement.Scale(n.FlatValue(), o.SampleUnit, o.OutputUnit)
		fmt.Fprintf(w, "%s %d %d\n", addr, n.Info.Lineno, int64(sv))

		// Print outgoing edges.
		for _, out := range n.Out.Sort() {
			c, _ := measurement.Scale(out.Weight, o.SampleUnit, o.OutputUnit)
			callee := out.Dest
			fmt.Fprintln(w, "cfl="+callgrindName(files, callee.Info.File))
			fmt.Fprintln(w, "cfn="+callgrindName(names, nodeNames[callee]))
			// pprof doesn't have a flat weight for a call, leave as 0.
			fmt.Fprintf(w, "calls=0 %s %d\n", callgrindAddress(prevInfo, callee.Info.Address), callee.Info.Lineno)
			// TODO: This address may be in the middle of a call
			// instruction. It would be best to find the beginning
			// of the instruction, but the tools seem to handle
			// this OK.
			fmt.Fprintf(w, "* * %d\n", int64(c))
		}

		prevInfo = &n.Info
	}

	return nil
}

// getDisambiguatedNames returns a map from each node in the graph to
// the name to use in the callgrind output. Callgrind merges all
// functions with the same [file name, function name]. Add a [%d/n]
// suffix to disambiguate nodes with different values of
// node.Function, which we want to keep separate. In particular, this
// affects graphs created with --call_tree, where nodes from different
// contexts are associated to different Functions.
func getDisambiguatedNames(g *graph.Graph) map[*graph.Node]string {
	nodeName := make(map[*graph.Node]string, len(g.Nodes))

	type names struct {
		file, function string
	}

	// nameFunctionIndex maps the callgrind names (filename, function)
	// to the node.Function values found for that name, and each
	// node.Function value to a sequential index to be used on the
	// disambiguated name.
	nameFunctionIndex := make(map[names]map[*graph.Node]int)
	for _, n := range g.Nodes {
		nm := names{n.Info.File, n.Info.Name}
		p, ok := nameFunctionIndex[nm]
		if !ok {
			p = make(map[*graph.Node]int)
			nameFunctionIndex[nm] = p
		}
		if _, ok := p[n.Function]; !ok {
			p[n.Function] = len(p)
		}
	}

	for _, n := range g.Nodes {
		nm := names{n.Info.File, n.Info.Name}
		nodeName[n] = n.Info.Name
		if p := nameFunctionIndex[nm]; len(p) > 1 {
			// If there is more than one function, add suffix to disambiguate.
			nodeName[n] += fmt.Sprintf(" [%d/%d]", p[n.Function]+1, len(p))
		}
	}
	return nodeName
}

// callgrindName implements the callgrind naming compression scheme.
// For names not previously seen returns "(N) name", where N is a
// unique index. For names previously seen returns "(N)" where N is
// the index returned the first time.
func callgrindName(names map[string]int, name string) string {
	if name == "" {
		return ""
	}
	if id, ok := names[name]; ok {
		return fmt.Sprintf("(%d)", id)
	}
	id := len(names) + 1
	names[name] = id
	return fmt.Sprintf("(%d) %s", id, name)
}

// callgrindAddress implements the callgrind subposition compression scheme if
// possible. If prevInfo != nil, it contains the previous address. The current
// address can be given relative to the previous address, with an explicit +/-
// to indicate it is relative, or * for the same address.
func callgrindAddress(prevInfo *graph.NodeInfo, curr uint64) string {
	abs := fmt.Sprintf("%#x", curr)
	if prevInfo == nil {
		return abs
	}

	prev := prevInfo.Address
	if prev == curr {
		return "*"
	}

	diff := int64(curr - prev)
	relative := fmt.Sprintf("%+d", diff)

	// Only bother to use the relative address if it is actually shorter.
	if len(relative) < len(abs) {
		return relative
	}

	return abs
}

// printTree prints a tree-based report in text form.
func printTree(w io.Writer, rpt *Report) error {
	const separator = "----------------------------------------------------------+-------------"
	const legend = "      flat  flat%   sum%        cum   cum%   calls calls% + context 	 	 "

	g, origCount, droppedNodes, _ := rpt.newTrimmedGraph()
	rpt.selectOutputUnit(g)

	fmt.Fprintln(w, strings.Join(reportLabels(rpt, g, origCount, droppedNodes, 0, false), "\n"))

	fmt.Fprintln(w, separator)
	fmt.Fprintln(w, legend)
	var flatSum int64

	rx := rpt.options.Symbol
	matched := 0
	for _, n := range g.Nodes {
		name, flat, cum := n.Info.PrintableName(), n.FlatValue(), n.CumValue()

		// Skip any entries that do not match the regexp (for the "peek" command).
		if rx != nil && !rx.MatchString(name) {
			continue
		}
		matched++

		fmt.Fprintln(w, separator)
		// Print incoming edges.
		inEdges := n.In.Sort()
		for _, in := range inEdges {
			var inline string
			if in.Inline {
				inline = " (inline)"
			}
			fmt.Fprintf(w, "%50s %s |   %s%s\n", rpt.formatValue(in.Weight),
				measurement.Percentage(in.Weight, cum), in.Src.Info.PrintableName(), inline)
		}

		// Print current node.
		flatSum += flat
		fmt.Fprintf(w, "%10s %s %s %10s %s                | %s\n",
			rpt.formatValue(flat),
			measurement.Percentage(flat, rpt.total),
			measurement.Percentage(flatSum, rpt.total),
			rpt.formatValue(cum),
			measurement.Percentage(cum, rpt.total),
			name)

		// Print outgoing edges.
		outEdges := n.Out.Sort()
		for _, out := range outEdges {
			var inline string
			if out.Inline {
				inline = " (inline)"
			}
			fmt.Fprintf(w, "%50s %s |   %s%s\n", rpt.formatValue(out.Weight),
				measurement.Percentage(out.Weight, cum), out.Dest.Info.PrintableName(), inline)
		}
	}
	if len(g.Nodes) > 0 {
		fmt.Fprintln(w, separator)
	}
	if rx != nil && matched == 0 {
		return fmt.Errorf("no matches found for regexp: %s", rx)
	}
	return nil
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

// printDOT prints an annotated callgraph in DOT format.
func printDOT(w io.Writer, rpt *Report) error {
	g, c := GetDOT(rpt)
	graph.ComposeDot(w, g, &graph.DotAttributes{}, c)
	return nil
}

// ProfileLabels returns printable labels for a profile.
func ProfileLabels(rpt *Report) []string {
	label := []string{}
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
