package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bufbuild/connect-go"
	querierv1 "github.com/grafana/fire/pkg/gen/querier/v1"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/grafana/grafana-plugin-sdk-go/live"
)

type queryModel struct {
	WithStreaming bool
	ProfileTypeID string `json:"profileTypeId"`
	LabelSelector string `json:"labelSelector"`
}

// query processes single Fire query transforming the response to data.Frame packaged in DataResponse
func (d *FireDatasource) query(ctx context.Context, pCtx backend.PluginContext, query backend.DataQuery) backend.DataResponse {
	var qm queryModel
	response := backend.DataResponse{}

	err := json.Unmarshal(query.JSON, &qm)
	if err != nil {
		response.Error = err
		return response
	}

	log.DefaultLogger.Debug("Querying SelectMergeStacktraces()", "queryModel", qm)

	resp, err := d.client.SelectMergeStacktraces(ctx, makeRequest(qm, query))
	if err != nil {
		response.Error = err
		return response
	}
	frame := responseToDataFrames(resp)

	// If query called with streaming on then return a channel
	// to subscribe on a client-side and consume updates from a plugin.
	// Feel free to remove this if you don't need streaming for your datasource.
	if qm.WithStreaming {
		channel := live.Channel{
			Scope:     live.ScopeDatasource,
			Namespace: pCtx.DataSourceInstanceSettings.UID,
			Path:      "stream",
		}
		frame.SetMeta(&data.FrameMeta{Channel: channel.String()})
	}

	// add the frames to the response.
	response.Frames = append(response.Frames, frame)

	log.DefaultLogger.Debug("Querying SelectSeries()", "queryModel", qm)

	seriesResp, err := d.client.SelectSeries(ctx, connect.NewRequest(&querierv1.SelectSeriesRequest{
		ProfileTypeID: qm.ProfileTypeID,
		LabelSelector: qm.LabelSelector,
		Start:         query.TimeRange.From.UnixMilli(),
		End:           query.TimeRange.To.UnixMilli(),
		Step:          query.Interval.Seconds(),
		// todo add one or more group bys
		GroupBy: []string{},
	}))
	if err != nil {
		response.Error = err
		return response
	}
	// todo remove me and add the series to the frame.
	log.DefaultLogger.Debug("Series", seriesResp.Msg.Series)
	return response
}

func makeRequest(qm queryModel, query backend.DataQuery) *connect.Request[querierv1.SelectMergeStacktracesRequest] {
	return &connect.Request[querierv1.SelectMergeStacktracesRequest]{
		Msg: &querierv1.SelectMergeStacktracesRequest{
			ProfileTypeID: qm.ProfileTypeID,
			LabelSelector: qm.LabelSelector,
			Start:         query.TimeRange.From.UnixMilli(),
			End:           query.TimeRange.To.UnixMilli(),
		},
	}
}

type CustomMeta struct {
	Names   []string
	Total   int64
	MaxSelf int64
}

// responseToDataFrames turns fire response to data.Frame. At this point this transform is very simple, each
// level being encoded as json string and set as a single value in a single column. Reason for this is that each level
// can have variable number of values but in data.Frame each column needs to have the same number of values.
// In addition, Names, Total, MaxSelf is added to Meta.Custom which may not be the best practice so needs to be
// evaluated later on
func responseToDataFrames(resp *connect.Response[querierv1.SelectMergeStacktracesResponse]) *data.Frame {
	for index, level := range resp.Msg.Flamegraph.Levels {
		values, _ := json.Marshal(level.Values)
		log.DefaultLogger.Debug(fmt.Sprintf("------- %d %s \n", index, values))
	}
	tree := levelsToTree(resp.Msg.Flamegraph.Levels, resp.Msg.Flamegraph.Names)
	return treeToNestedSetDataFrame(tree)
}

const START_OFFSET = 0
const VALUE_OFFSET = 1
const NAME_OFFSET = 3
const ITEM_OFFSET = 4

type ProfileTree struct {
	Start int64
	Value int64
	Level int
	Name  string
	Nodes []*ProfileTree
}

func levelsToTree(levels []*querierv1.Level, names []string) *ProfileTree {
	tree := &ProfileTree{
		Start: 0,
		Value: levels[0].Values[VALUE_OFFSET],
		Level: 0,
		Name:  names[levels[0].Values[NAME_OFFSET]],
	}

	parentsStack := []*ProfileTree{tree}
	currentLevel := 1

	for {
		if currentLevel >= len(levels) {
			break
		}

		// If we still have levels to go this should not happen
		if len(parentsStack) == 0 {
			log.DefaultLogger.Error("parentsStack is empty but we are not at the the last level", "currentLevel", currentLevel)
			break
		}

		var nextParentsStack []*ProfileTree
		currentParent := parentsStack[:1][0]
		parentsStack = parentsStack[1:]
		itemIndex := 0
		// cumulative offset as items have just relative to prev item
		offset := int64(0)

		for {
			if itemIndex >= len(levels[currentLevel].Values) {
				break
			}

			itemStart := levels[currentLevel].Values[itemIndex+START_OFFSET] + offset
			itemValue := levels[currentLevel].Values[itemIndex+VALUE_OFFSET]
			itemEnd := itemStart + itemValue
			parentEnd := currentParent.Start + currentParent.Value

			if itemStart >= currentParent.Start && itemEnd <= parentEnd {
				// We have an item that is in the bounds of current parent item, so it should be its child
				treeItem := &ProfileTree{
					Start: itemStart,
					Value: itemValue,
					Level: currentLevel,
					Name:  names[levels[currentLevel].Values[itemIndex+NAME_OFFSET]],
				}
				// Add to parent
				currentParent.Nodes = append(currentParent.Nodes, treeItem)
				// Add this item as parent for the next level
				nextParentsStack = append(nextParentsStack, treeItem)
				itemIndex += ITEM_OFFSET

				// Update offset for next item
				offset = itemEnd
			} else {
				// We went out of parents bounds so lets move to next parent
				if len(parentsStack) == 0 {
					log.DefaultLogger.Error("parentsStack is empty but there are still items in current level", "currentLevel", currentLevel, "itemIndex", itemIndex)
					break
				}
				currentParent = parentsStack[:1][0]
				parentsStack = parentsStack[1:]
				continue
			}
		}
		parentsStack = nextParentsStack
		currentLevel++
	}

	return tree
}

func treeToNestedSetDataFrame(tree *ProfileTree) *data.Frame {
	frame := data.NewFrame("response")
	frame.Meta = &data.FrameMeta{PreferredVisualization: "flamegraph"}

	levelField := data.NewField("level", nil, []int64{})
	valueField := data.NewField("value", nil, []int64{})
	labelField := data.NewField("label", nil, []string{})
	frame.Fields = data.Fields{levelField, valueField, labelField}

	walkTree(tree, func(tree *ProfileTree) {
		levelField.Append(int64(tree.Level))
		valueField.Append(tree.Value)
		labelField.Append(tree.Name)
	})
	return frame
}

func walkTree(tree *ProfileTree, fn func(tree *ProfileTree)) {
	fn(tree)
	stack := tree.Nodes

	for {
		if len(stack) == 0 {
			break
		}

		fn(stack[0])
		if stack[0].Nodes != nil {
			stack = append(stack[0].Nodes, stack[1:]...)
		} else {
			stack = stack[1:]
		}
	}
}
