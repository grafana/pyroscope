// Copyright 2021 The Parca Authors
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

package scrape

import (
	"context"

	"github.com/prometheus/prometheus/model/labels"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	profilepb "github.com/parca-dev/parca/gen/proto/go/parca/profilestore/v1alpha1"
	pb "github.com/parca-dev/parca/gen/proto/go/parca/scrape/v1alpha1"
)

// Targets implements the Targets RCP.
func (m *Manager) Targets(ctx context.Context, req *pb.TargetsRequest) (*pb.TargetsResponse, error) {
	var targets map[string][]*Target
	switch req.State {
	case pb.TargetsRequest_STATE_ACTIVE:
		targets = m.TargetsActive()
	case pb.TargetsRequest_STATE_DROPPED:
		targets = m.TargetsDropped()
	case pb.TargetsRequest_STATE_ANY_UNSPECIFIED:
		fallthrough
	default:
		targets = m.TargetsAll()
	}

	resp := &pb.TargetsResponse{
		Targets: make(map[string]*pb.Targets, len(targets)),
	}

	// Convert the targets into proto format
	for k, ts := range targets {
		tgts := make([]*pb.Target, 0, len(ts))
		for _, t := range ts {
			lastError := ""
			lerr := t.LastError()
			if lerr != nil {
				lastError = lerr.Error()
			}

			tgts = append(tgts, &pb.Target{
				DiscoveredLabels:   ProtoLabelsFromLabels(t.DiscoveredLabels()),
				Labels:             ProtoLabelsFromLabels(t.Labels()),
				LastError:          lastError,
				LastScrape:         timestamppb.New(t.LastScrape()),
				LastScrapeDuration: durationpb.New(t.LastScrapeDuration()),
				Url:                t.URL().String(),
				Health:             HealthProto(t.Health()),
			})
		}
		resp.Targets[k] = &pb.Targets{
			Targets: tgts,
		}
	}

	return resp, nil
}

// ProtoLabelsFromLabels converts labels.Labels into a proto label set.
func ProtoLabelsFromLabels(l labels.Labels) *profilepb.LabelSet {
	ls := &profilepb.LabelSet{
		Labels: make([]*profilepb.Label, 0, len(l)),
	}

	for _, lbl := range l {
		ls.Labels = append(ls.Labels, &profilepb.Label{
			Name:  lbl.Name,
			Value: lbl.Value,
		})
	}
	return ls
}

// HealthProto converts a target health string into a Target_Health proto.
func HealthProto(s TargetHealth) pb.Target_Health {
	switch s {
	case HealthGood:
		return pb.Target_HEALTH_GOOD
	case HealthBad:
		return pb.Target_HEALTH_BAD
	case HealthUnknown:
		fallthrough
	default:
		return pb.Target_HEALTH_UNKNOWN_UNSPECIFIED
	}
}
