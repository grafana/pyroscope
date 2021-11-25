// Copyright 2013 The Prometheus Authors
// Copyright 2021 The Pyroscope Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package targetgroup

import (
	"bytes"
	"encoding/json"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/model"
)

// Group is a set of targets with a common label set(production , test, staging etc.).
type Group struct {
	// Targets is a list of targets identified by a label set. Each target is
	// uniquely identifiable in the group by its address label.
	Targets []model.LabelSet
	// Labels is a set of labels that is common across all targets in the group.
	Labels model.LabelSet
	// Source is an identifier that describes a group of targets.
	Source string
}

func (tg Group) String() string {
	return tg.Source
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (tg *Group) UnmarshalYAML(unmarshal func(interface{}) error) error {
	g := struct {
		AppName string         `yaml:"application"`
		Targets []string       `yaml:"targets"`
		Labels  model.LabelSet `yaml:"labels"`
	}{}
	if err := unmarshal(&g); err != nil {
		return err
	}
	if err := flameql.ValidateAppName(g.AppName); err != nil {
		return err
	}
	tg.Targets = make([]model.LabelSet, 0, len(g.Targets))
	for _, t := range g.Targets {
		tg.Targets = append(tg.Targets, model.LabelSet{
			model.AddressLabel:    model.LabelValue(t),
			model.MetricNameLabel: model.LabelValue(g.AppName),
		})
	}
	tg.Labels = g.Labels
	return nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (tg *Group) UnmarshalJSON(b []byte) error {
	g := struct {
		AppName string         `json:"application"`
		Targets []string       `json:"targets"`
		Labels  model.LabelSet `json:"labels"`
	}{}

	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&g); err != nil {
		return err
	}
	if err := flameql.ValidateAppName(g.AppName); err != nil {
		return err
	}
	tg.Targets = make([]model.LabelSet, 0, len(g.Targets))
	for _, t := range g.Targets {
		tg.Targets = append(tg.Targets, model.LabelSet{
			model.AddressLabel:    model.LabelValue(t),
			model.MetricNameLabel: model.LabelValue(g.AppName),
		})
	}
	tg.Labels = g.Labels
	return nil
}
