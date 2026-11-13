package recording

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

func TestEffectiveRules(t *testing.T) {
	configWithID := RandomRule()
	configWithoutID := RandomRule()
	configWithoutID.Id = ""
	overridden := &settingsv1.RecordingRuleStore{Id: configWithID.Id, MetricName: "profiles_recorded_overridden"}
	extra := &settingsv1.RecordingRuleStore{Id: "extra", MetricName: "profiles_recorded_extra"}

	config := []*settingsv1.RecordingRule{configWithID, configWithoutID}
	snapshot := []*settingsv1.RecordingRule{proto.Clone(configWithID).(*settingsv1.RecordingRule), proto.Clone(configWithoutID).(*settingsv1.RecordingRule)}

	rules := EffectiveRules(config, []*settingsv1.RecordingRuleStore{overridden, extra})

	assert.Len(t, rules, 3)
	assert.Equal(t, configWithID.Id, rules[0].Id)
	assert.True(t, rules[0].Provisioned)
	assert.Equal(t, configWithID.MetricName, rules[0].MetricName)
	assert.Equal(t, idForRule(proto.Clone(snapshot[1]).(*settingsv1.RecordingRule)), rules[1].Id)
	assert.True(t, rules[1].Provisioned)
	assert.Equal(t, "extra", rules[2].Id)
	assert.False(t, rules[2].Provisioned)

	assert.True(t, proto.Equal(snapshot[0], config[0]))
	assert.True(t, proto.Equal(snapshot[1], config[1]))
}

func TestEffectiveRules_Empty(t *testing.T) {
	assert.Empty(t, EffectiveRules(nil, nil))
}
