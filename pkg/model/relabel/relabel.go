// Provenance-includes-location: https://github.com/prometheus/prometheus/blob/v2.51.2/model/relabel/relabel.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Prometheus Authors

package relabel

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/relabel"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

type Config = relabel.Config

// Process returns a relabeled version of the given label set. The relabel configurations
// are applied in order of input.
// There are circumstances where Process will modify the input label.
// If you want to avoid issues with the input label set being modified, at the cost of
// higher memory usage, you can use lbls.Copy().
// If a label set is dropped, EmptyLabels and false is returned.
func Process(lbls phlaremodel.Labels, cfgs ...*relabel.Config) (ret phlaremodel.Labels, keep bool) {
	lb := phlaremodel.NewLabelsBuilder(lbls)
	if !ProcessBuilder(lb, cfgs...) {
		return phlaremodel.EmptyLabels(), false
	}
	return lb.Labels(), true
}

// ProcessBuilder is like Process, but the caller passes a labels.Builder
// containing the initial set of labels, which is mutated by the rules.
func ProcessBuilder(lb *phlaremodel.LabelsBuilder, cfgs ...*Config) (keep bool) {
	for _, cfg := range cfgs {
		keep = relabelBuilder(cfg, lb)
		if !keep {
			return false
		}
	}
	return true
}

func relabelBuilder(cfg *Config, lb *phlaremodel.LabelsBuilder) (keep bool) {
	var va [16]string
	values := va[:0]
	if len(cfg.SourceLabels) > cap(values) {
		values = make([]string, 0, len(cfg.SourceLabels))
	}
	for _, ln := range cfg.SourceLabels {
		values = append(values, lb.Get(string(ln)))
	}
	val := strings.Join(values, cfg.Separator)

	switch cfg.Action {
	case relabel.Drop:
		if cfg.Regex.MatchString(val) {
			return false
		}
	case relabel.Keep:
		if !cfg.Regex.MatchString(val) {
			return false
		}
	case relabel.DropEqual:
		if lb.Get(cfg.TargetLabel) == val {
			return false
		}
	case relabel.KeepEqual:
		if lb.Get(cfg.TargetLabel) != val {
			return false
		}
	case relabel.Replace:
		indexes := cfg.Regex.FindStringSubmatchIndex(val)
		// If there is no match no replacement must take place.
		if indexes == nil {
			break
		}
		target := model.LabelName(cfg.Regex.ExpandString([]byte{}, cfg.TargetLabel, val, indexes))
		if !model.UTF8Validation.IsValidLabelName(string(target)) {
			break
		}
		res := cfg.Regex.ExpandString([]byte{}, cfg.Replacement, val, indexes)
		if len(res) == 0 {
			lb.Del(string(target))
			break
		}
		lb.Set(string(target), string(res))
	case relabel.Lowercase:
		lb.Set(cfg.TargetLabel, strings.ToLower(val))
	case relabel.Uppercase:
		lb.Set(cfg.TargetLabel, strings.ToUpper(val))
	case relabel.HashMod:
		hash := md5.Sum([]byte(val))
		// Use only the last 8 bytes of the hash to give the same result as earlier versions of this code.
		mod := binary.BigEndian.Uint64(hash[8:]) % cfg.Modulus
		lb.Set(cfg.TargetLabel, strconv.FormatUint(mod, 10))
	case relabel.LabelMap:
		lb.Range(func(l *typesv1.LabelPair) {
			if cfg.Regex.MatchString(l.Name) {
				res := cfg.Regex.ReplaceAllString(l.Name, cfg.Replacement)
				lb.Set(res, l.Value)
			}
		})
	case relabel.LabelDrop:
		lb.Range(func(l *typesv1.LabelPair) {
			if cfg.Regex.MatchString(l.Name) {
				lb.Del(l.Name)
			}
		})
	case relabel.LabelKeep:
		lb.Range(func(l *typesv1.LabelPair) {
			if !cfg.Regex.MatchString(l.Name) {
				lb.Del(l.Name)
			}
		})
	default:
		panic(fmt.Errorf("relabel: unknown relabel action type %q", cfg.Action))
	}

	return true
}
