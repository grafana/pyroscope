package model

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
)

func TestStackTraceMerger(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in       []*ingestv1.MergeProfilesStacktracesResult
		maxNodes int64
		expected *Tree
	}{
		{
			name:     "empty",
			in:       []*ingestv1.MergeProfilesStacktracesResult{},
			expected: newTree(nil),
		},
		{
			name: "single",
			in: []*ingestv1.MergeProfilesStacktracesResult{
				{
					Stacktraces: []*ingestv1.StacktraceSample{
						{
							FunctionIds: []int32{1, 0},
							Value:       1,
						},
						{
							FunctionIds: []int32{2, 1, 0},
							Value:       3,
						},
					},
					FunctionNames: []string{"my", "other", "stack"},
				},
			},
			expected: newTree([]stacktraces{
				{locations: []string{"other", "my"}, value: 1},
				{locations: []string{"stack", "other", "my"}, value: 3},
			}),
		},
		{
			name: "multiple",
			in: []*ingestv1.MergeProfilesStacktracesResult{
				{
					Stacktraces: []*ingestv1.StacktraceSample{
						{
							FunctionIds: []int32{1, 0},
							Value:       1,
						},
						{
							FunctionIds: []int32{2, 1, 0},
							Value:       3,
						},
						{
							FunctionIds: []int32{3},
							Value:       4,
						},
					},
					FunctionNames: []string{"my", "other", "stack", "foo"},
				},
				{
					Stacktraces: []*ingestv1.StacktraceSample{
						{
							FunctionIds: []int32{1, 0},
							Value:       1,
						},
						{
							FunctionIds: []int32{2, 1, 0},
							Value:       3,
						},
						{
							FunctionIds: []int32{3},
							Value:       5,
						},
					},
					FunctionNames: []string{"my", "other", "stack", "bar"},
				},
			},
			expected: newTree([]stacktraces{
				{locations: []string{"bar"}, value: 5},
				{locations: []string{"foo"}, value: 4},
				{locations: []string{"other", "my"}, value: 2},
				{locations: []string{"stack", "other", "my"}, value: 6},
			}),
		},
		{
			name:     "multiple with truncation",
			maxNodes: 3,
			in: []*ingestv1.MergeProfilesStacktracesResult{
				{
					Stacktraces: []*ingestv1.StacktraceSample{
						{
							FunctionIds: []int32{1, 0},
							Value:       1,
						},
						{
							FunctionIds: []int32{2, 1, 0},
							Value:       3,
						},
						{
							FunctionIds: []int32{3},
							Value:       4,
						},
					},
					FunctionNames: []string{"my", "qux", "stack", "foo"},
				},
				{
					Stacktraces: []*ingestv1.StacktraceSample{
						{
							FunctionIds: []int32{1, 0},
							Value:       1,
						},
						{
							FunctionIds: []int32{2, 1, 0},
							Value:       3,
						},
						{
							FunctionIds: []int32{3},
							Value:       5,
						},
					},
					FunctionNames: []string{"my", "qux", "stack", "bar"},
				},
			},
			expected: newTree([]stacktraces{
				{locations: []string{"other"}, value: 9},
				{locations: []string{"qux", "my"}, value: 2},
				{locations: []string{"stack", "qux", "my"}, value: 6},
			}),
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := NewStackTraceMerger()
			for _, x := range tc.in {
				m.MergeStackTraces(x.Stacktraces, x.FunctionNames)
			}
			yn := m.TreeBytes(tc.maxNodes)
			actual, err := UnmarshalTree(yn)
			require.NoError(t, err)
			require.Equal(t, tc.expected.String(), actual.String())
		})
	}
}

func TestTree(t *testing.T) {
	tree2 := NewStacktraceTree(10)
	tree2.Insert([]int32{7}, 2)
	tree2.Insert([]int32{7, 3}, 100)
	tree2.Insert([]int32{3, 7}, 1000)
	tree2.Insert([]int32{3, 7}, 50)
	tree2.Insert([]int32{1, 2, 3}, 10)
	tree2.Insert([]int32{4, 2, 3}, 5)

	tree2.Traverse(1000, func(index int32, children []int32) error {
		fmt.Println(tree2.Nodes[index].Location, tree2.Nodes[index].Value, tree2.Nodes[index].Total)
		return nil
	})
	NN := int64(2)
	t2 := tree2.Tree(NN, []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"})
	fmt.Println(t2.String())

	tree := AVLTree{}
	tree.Insert([]int32{1, 2, 3}, 10)
	tree.Insert([]int32{4, 2, 3}, 5)
	tree.Insert([]int32{7}, 2)
	tree.Insert([]int32{7, 3}, 100)
	tree.Insert([]int32{3, 7}, 1000)
	tree.Insert([]int32{3, 7}, 50)
	fmt.Println(tree.Len())
	i := NewAVLIterator(tree)
	n := 0
	for i.Next() {
		n++
		i.At()
	}
	fmt.Println(n)
	tr := tree.Tree(NN, []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"})
	fmt.Println(tr.String())

	require.Equal(t, true, t2.Equal(tr))
	require.Equal(t, t2.String(), tr.String())

}

type fileline struct {
	stack []int32
	value int64
}

var names []string
var lines []fileline

func init() {
	names = make([]string, 0, 20000000)
	for i := 0; i < 20000000; i++ {
		names = append(names, strconv.Itoa(i))
	}

	lines, _ = readLines("./testdata/stacktraces.log")
}

func readLines(path string) ([]fileline, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	lines := make([]fileline, 0)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "]")

		stackStr := strings.Trim(parts[0], "[")
		valStr := strings.TrimSpace(parts[1])

		var stack []int32
		if len(stackStr) > 0 {
			for _, s := range strings.Split(stackStr, " ") {
				i, _ := strconv.Atoi(s)
				stack = append(stack, int32(i))
			}
		}
		value, _ := strconv.ParseInt(valStr, 10, 64)

		lines = append(lines, fileline{stack, value})
	}
	return lines, nil
}

func TestStacktraceTree_InsertFromFile(t *testing.T) {
	tree := NewStacktraceTree(0)
	start := time.Now()
	for line := range lines {
		tree.Insert(lines[line].stack, lines[line].value)
	}
	fmt.Println("trie:", time.Since(start))

	tree2 := AVLTree{}
	start = time.Now()
	for line := range lines {
		tree2.Insert(lines[line].stack, lines[line].value)
	}
	fmt.Println("avl:", time.Since(start))

	start = time.Now()
	resultingTree1 := tree.Tree(20000000, names)
	fmt.Println("trie:", time.Since(start))
	start = time.Now()
	resultingTree2 := tree.Tree(20000000, names)
	fmt.Println("avl:", time.Since(start))

	require.Equal(t, true, resultingTree1.Equal(resultingTree2))
}

func TestStacktraceTree_Iterator(t *testing.T) {
	tree := AVLTree{}
	start := time.Now()
	for line := range lines {
		tree.Insert(lines[line].stack, lines[line].value)
	}
	fmt.Println("avl:", time.Since(start))

	i := NewAVLIterator(tree)
	n := int32(0)
	for i.Next() {
		n++
		i.At()
	}
	require.Equal(t, n, tree.Len())
}

func TestStacktraceTree_MinValue(t *testing.T) {
	tree := NewStacktraceTree(0)
	for line := range lines {
		tree.Insert(lines[line].stack, lines[line].value)
	}
	start := time.Now()
	mv1 := tree.MinValue(int64(len(tree.Nodes) - 1))
	fmt.Println("trie:", time.Since(start))

	tree2 := AVLTree{}
	for line := range lines {
		tree2.Insert(lines[line].stack, lines[line].value)
	}

	start = time.Now()
	mv2 := tree2.MinValue(int64(tree2.Len() - 1))
	fmt.Println("avl:", time.Since(start))

	require.Equal(t, mv1, mv2)
}

func TestStacktraceTree_MinValueRandom(t *testing.T) {
	tree := NewStacktraceTree(0)
	for line := range lines {
		tree.Insert(lines[line].stack, lines[line].value)
	}

	tree2 := AVLTree{}
	for line := range lines {
		tree2.Insert(lines[line].stack, lines[line].value)
	}

	totalTrie := time.Duration(0)
	totalAVL := time.Duration(0)
	l := len(tree.Nodes)
	rand.Seed(time.Now().UnixNano())
	for i := 0; i <= l; i += rand.Intn(1000000) + 1 {
		start := time.Now()
		mv1 := tree.MinValue(int64(i))
		totalTrie += time.Since(start)
		start = time.Now()
		mv2 := tree2.MinValue(int64(i))
		totalAVL += time.Since(start)
		fmt.Println(i, "trie:", mv1, "avl:", mv2)
		require.Equal(t, mv1, mv2)
	}
	fmt.Println("total trie:", totalTrie)
	fmt.Println("total avl:", totalAVL)
}
