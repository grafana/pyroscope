package load

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type App struct {
	Name            string
	SpyName         string
	SampleRate      uint32
	Units           string
	AggregationType string

	tags  *TagsGenerator
	trees *TreeGenerator
}

type AppConfig struct {
	Tags  []Tag
	Trees int
	TreeConfig
}

type Tag struct {
	Name        string
	Cardinality int
	MinLen      int
	MaxLen      int
}

func NewApp(seed int, name string, c AppConfig) *App {
	a := App{Name: name}
	a.trees = NewTreeGenerator(seed, c.Trees, c.TreeConfig)
	a.tags = NewTagGenerator(seed, name)
	for _, t := range c.Tags {
		a.tags.Add(t.Name, t.Cardinality, t.MinLen, t.MaxLen)
	}
	return &a
}

func (a *App) CreatePutInput(from, to time.Time) *storage.PutInput {
	return &storage.PutInput{
		StartTime:       from,
		EndTime:         to,
		Key:             storage.NewKey(a.tags.Next()),
		Val:             a.trees.Next(),
		SpyName:         a.SpyName,
		SampleRate:      a.SampleRate,
		Units:           a.Units,
		AggregationType: a.AggregationType,
	}
}
