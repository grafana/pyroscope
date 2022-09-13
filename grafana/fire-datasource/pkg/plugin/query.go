package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	frame := responseToDataFrames(resp, qm.ProfileTypeID)

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

	// add the frames to the response.
	response.Frames = append(response.Frames, seriesToDataFrame(seriesResp, qm.ProfileTypeID))
	response.Frames = append(response.Frames, frame)

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

// responseToDataFrames turns fire response to data.Frame. We encode the data into a nested set format where we have
// [level, value, label] columns and by ordering the items in a depth first traversal order we can recreate the whole
// tree back.
func responseToDataFrames(resp *connect.Response[querierv1.SelectMergeStacktracesResponse], profileTypeID string) *data.Frame {
	for index, level := range resp.Msg.Flamegraph.Levels {
		values, _ := json.Marshal(level.Values)
		log.DefaultLogger.Debug(fmt.Sprintf("------- %d %s \n", index, values))
	}
	tree := levelsToTree(resp.Msg.Flamegraph.Levels, resp.Msg.Flamegraph.Names)
	return treeToNestedSetDataFrame(tree, profileTypeID)
}

// Offset of the bar relative to previous sibling
const START_OFFSET = 0

// Value or width of the bar
const VALUE_OFFSET = 1

// Self value of the bar, we don't use it at the moment but will add it to the metadata later.
// const SELF_OFFSET = 2
// Index into the names array
const NAME_OFFSET = 3

// Next bar. Each bar of the profile is represented by 4 number in a flat array.
const ITEM_OFFSET = 4

type ProfileTree struct {
	Start int64
	Value int64
	Level int
	Name  string
	Nodes []*ProfileTree
}

// levelsToTree converts flamebearer format into a tree. This is needed to then convert it into nested set format
// dataframe. This should be temporary, and ideally we should get some sort of tree struct directly from Fire API.
func levelsToTree(levels []*querierv1.Level, names []string) *ProfileTree {
	tree := &ProfileTree{
		Start: 0,
		Value: levels[0].Values[VALUE_OFFSET],
		Level: 0,
		Name:  names[levels[0].Values[NAME_OFFSET]],
	}

	parentsStack := []*ProfileTree{tree}
	currentLevel := 1

	// Cycle through each level
	for {
		if currentLevel >= len(levels) {
			break
		}

		// If we still have levels to go, this should not happen. Something is probably wrong with the flamebearer data.
		if len(parentsStack) == 0 {
			log.DefaultLogger.Error("parentsStack is empty but we are not at the the last level", "currentLevel", currentLevel)
			break
		}

		var nextParentsStack []*ProfileTree
		currentParent := parentsStack[:1][0]
		parentsStack = parentsStack[1:]
		itemIndex := 0
		// cumulative offset as items in flamebearer format have just relative to prev item
		offset := int64(0)

		// Cycle through bar in a level
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

				// Update offset for next item. This is changing relative offset to absolute one.
				offset = itemEnd
			} else {
				// We went out of parents bounds so lets move to next parent. We will evaluate the same item again, but
				// we will check if it is a child of the next parent item in line.
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

type CustomMeta struct {
	ProfileTypeID string
}

// treeToNestedSetDataFrame walks the tree depth first and adds items into the dataframe. This is a nested set format
// where by ordering the items in depth first order and knowing the level/depth of each item we can recreate the
// parent - child relationship without explicitly needing parent/child column and we can later just iterate over the
// dataFrame to again basically walking depth first over the tree/profile.
func treeToNestedSetDataFrame(tree *ProfileTree, profileTypeID string) *data.Frame {
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
	frame.Meta.Custom = CustomMeta{
		ProfileTypeID: profileTypeID,
	}
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

func seriesToDataFrame(seriesResp *connect.Response[querierv1.SelectSeriesResponse], profileTypeID string) *data.Frame {
	frame := data.NewFrame("series")
	frame.Meta = &data.FrameMeta{PreferredVisualization: "graph"}

	fields := data.Fields{}
	timeField := data.NewField("time", nil, []time.Time{})
	fields = append(fields, timeField)

	for index, series := range seriesResp.Msg.Series {
		label := ""
		if len(series.Labels) > 0 {
			label = series.Labels[0].Name
		} else {
			parts := strings.Split(profileTypeID, ":")
			if len(parts) == 5 {
				label = parts[1] // sample type e.g. cpu, goroutine, alloc_objects
			}
		}
		valueField := data.NewField(label, nil, []float64{})

		for _, point := range series.Points {
			if index == 0 {
				timeField.Append(time.UnixMilli(point.T))
			}
			valueField.Append(point.V)
		}

		fields = append(fields, valueField)
	}

	frame.Fields = fields
	return frame
}
