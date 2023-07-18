package shard

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
)

const (
	// ShardLabel is a reserved label referencing a cortex shard
	ShardLabel = "__cortex_shard__"
	// ShardLabelFmt is the fmt of the ShardLabel key.
	ShardLabelFmt = "%d_of_%d"
)

// ShardLabelRE matches a value in ShardLabelFmt
var ShardLabelRE = regexp.MustCompile("^[0-9]+_of_[0-9]+$")

// ParseShard will extract the shard information encoded in ShardLabelFmt
func ParseShard(input string) (parsed Annotation, err error) {
	if !ShardLabelRE.MatchString(input) {
		return parsed, errors.Errorf("Invalid ShardLabel value: [%s]", input)
	}

	matches := strings.Split(input, "_")
	x, err := strconv.Atoi(matches[0])
	if err != nil {
		return parsed, err
	}
	of, err := strconv.Atoi(matches[2])
	if err != nil {
		return parsed, err
	}

	if x >= of {
		return parsed, errors.Errorf("Shards out of bounds: [%d] >= [%d]", x, of)
	}
	return Annotation{
		Shard: x,
		Of:    of,
	}, err
}

// Annotation is a convenience struct which holds data from a parsed shard label
type Annotation struct {
	Shard int
	Of    int
}

func (shard Annotation) Match(fp model.Fingerprint) bool {
	return uint64(fp)%uint64(shard.Of) == uint64(shard.Shard)
}

// String encodes a shardAnnotation into a label value
func (shard Annotation) String() string {
	return fmt.Sprintf(ShardLabelFmt, shard.Shard, shard.Of)
}

// Label generates the ShardAnnotation as a label
func (shard Annotation) Label() labels.Label {
	return labels.Label{
		Name:  ShardLabel,
		Value: shard.String(),
	}
}

func (shard Annotation) TSDB() index.ShardAnnotation {
	return index.NewShard(uint32(shard.Shard), uint32(shard.Of))
}

// FromMatchers extracts a ShardAnnotation and the index it was pulled from in the matcher list
func FromMatchers(matchers []*labels.Matcher) (shard *Annotation, idx int, err error) {
	for i, matcher := range matchers {
		if matcher.Name == ShardLabel && matcher.Type == labels.MatchEqual {
			shard, err := ParseShard(matcher.Value)
			if err != nil {
				return nil, i, err
			}
			return &shard, i, nil
		}
	}
	return nil, 0, nil
}
