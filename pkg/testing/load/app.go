package load

import (
	"math/big"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type App struct {
	Name            string
	SpyName         string
	SampleRate      uint32
	Units           metadata.Units
	AggregationType metadata.AggregationType

	tags  *TagsGenerator
	trees *TreeGenerator

	m          sync.Mutex
	mergedTree *tree.Tree
}

type AppConfig struct {
	SpyName         string                   `yaml:"spyName"`
	SampleRate      uint32                   `yaml:"sampleRate"`
	Units           metadata.Units           `yaml:"units"`
	AggregationType metadata.AggregationType `yaml:"aggregationType"`

	Tags       []Tag `yaml:"tags"`
	Trees      int   `yaml:"trees"`
	TreeConfig `yaml:"treeConfig"`
}

type Tag struct {
	Name        string `yaml:"name"`
	Cardinality int    `yaml:"cardinality"`
	MinLen      int    `yaml:"minLen"`
	MaxLen      int    `yaml:"maxLen"`
}

func NewApp(seed int, name string, c AppConfig) *App {
	a := App{
		Name:            name,
		SpyName:         c.SpyName,
		SampleRate:      c.SampleRate,
		Units:           c.Units,
		AggregationType: c.AggregationType,
		mergedTree:      tree.New(),
	}
	a.trees = NewTreeGenerator(seed, c.Trees, c.TreeConfig)
	a.tags = NewTagGenerator(seed, name)
	for _, t := range c.Tags {
		a.tags.Add(t.Name, t.Cardinality, t.MinLen, t.MaxLen)
	}
	return &a
}

type Input struct {
	StartTime       time.Time
	EndTime         time.Time
	Key             *segment.Key
	Val             *tree.Tree
	SpyName         string
	SampleRate      uint32
	Units           metadata.Units
	AggregationType metadata.AggregationType
}

func (a *App) CreateInput(from, to time.Time) Input {
	t := a.trees.Next()

	a.m.Lock()
	a.mergedTree.Merge(t.Clone(big.NewRat(1, 1)))
	a.m.Unlock()

	return Input{
		StartTime:       from,
		EndTime:         to,
		Key:             segment.NewKey(a.tags.Next()),
		Val:             t,
		SpyName:         a.SpyName,
		SampleRate:      a.SampleRate,
		Units:           a.Units,
		AggregationType: a.AggregationType,
	}
}

func (a *App) MergedTree() *tree.Tree { return a.mergedTree }
