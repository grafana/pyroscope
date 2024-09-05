package collection

import (
	"fmt"
	"strings"
	"unsafe"

	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/prometheus/model/relabel"
	"gopkg.in/yaml.v3"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

func init() {
	jsoniter.RegisterTypeEncoder("settingsv1.CollectionRuleAction", actionCodec{})
}

type actionCodec struct{}

func (actionCodec) IsEmpty(ptr unsafe.Pointer) bool {
	// handle action (which is a protobuf enum) as string and as the decoder expects it
	return *(*settingsv1.CollectionRuleAction)(ptr) == 0
}

func (actionCodec) Encode(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	action := (*settingsv1.CollectionRuleAction)(ptr).String()
	action = strings.TrimPrefix(action, "COLLECTION_RULE_ACTION_")
	action = strings.ToLower(action)
	stream.WriteString(action)
}

func collectionRuleToRelabelConfig(in *settingsv1.CollectionRule, out *relabel.Config) error {
	bytes, err := jsoniter.Marshal(in)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(bytes, out)
	if err != nil {
		return err
	}
	return err
}

func CollectionRuleToRelabelConfig(in *settingsv1.CollectionRule) (*relabel.Config, error) {
	var out relabel.Config
	if err := collectionRuleToRelabelConfig(in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func ValidateCollectionRule(rule *settingsv1.CollectionRule) error {
	out, err := CollectionRuleToRelabelConfig(rule)
	if err != nil {
		return err
	}
	return out.Validate()
}

func CollectionRulesToRelabelConfigs(rules []*settingsv1.CollectionRule) ([]*relabel.Config, error) {
	if len(rules) == 0 {
		return nil, nil
	}

	configs := make([]relabel.Config, len(rules))
	result := make([]*relabel.Config, len(rules))
	for idx := range rules {
		if err := collectionRuleToRelabelConfig(rules[idx], &configs[idx]); err != nil {
			return nil, fmt.Errorf("error validating rule %d: %w", idx, err)
		}
		if err := configs[idx].Validate(); err != nil {
			return nil, fmt.Errorf("error validating rule %d: %w", idx, err)
		}
		result[idx] = &configs[idx]
	}

	return result, nil
}
