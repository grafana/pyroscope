package plugin

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/grafana/fire/pkg/gen/querier/v1/querierv1connect"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/grafana/grafana-plugin-sdk-go/live"
	"github.com/prometheus/common/model"

	querierv1 "github.com/grafana/fire/pkg/gen/querier/v1"
)

// Make sure FireDatasource implements required interfaces. This is important to do
// since otherwise we will only get a not implemented error response from plugin in
// runtime. In this example datasource instance implements backend.QueryDataHandler,
// backend.CheckHealthHandler, backend.StreamHandler interfaces. Plugin should not
// implement all these interfaces - only those which are required for a particular task.
// For example if plugin does not need streaming functionality then you are free to remove
// methods that implement backend.StreamHandler. Implementing instancemgmt.InstanceDisposer
// is useful to clean up resources used by previous datasource instance when a new datasource
// instance created upon datasource settings changed.
var (
	_ backend.QueryDataHandler      = (*FireDatasource)(nil)
	_ backend.CallResourceHandler   = (*FireDatasource)(nil)
	_ backend.CheckHealthHandler    = (*FireDatasource)(nil)
	_ backend.StreamHandler         = (*FireDatasource)(nil)
	_ instancemgmt.InstanceDisposer = (*FireDatasource)(nil)
)

// FireDatasource is an datasource for querying application performance profiles.
type FireDatasource struct {
	client querierv1connect.QuerierServiceClient
}

// NewFireDatasource creates a new datasource instance.
func NewFireDatasource(settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	log.DefaultLogger.Info("Created DataSource", "settings", settings)

	opt, err := settings.HTTPClientOptions()
	if err != nil {
		return nil, err
	}
	httpClient, err := httpclient.New(opt)
	if err != nil {
		return nil, err
	}

	return &FireDatasource{
		client: querierv1connect.NewQuerierServiceClient(httpClient, settings.URL),
	}, nil
}

// Dispose here tells plugin SDK that plugin wants to clean up resources when a new instance
// created. As soon as datasource settings change detected by SDK old datasource instance will
// be disposed and a new one will be created using NewSampleDatasource factory function.
func (d *FireDatasource) Dispose() {
	// Clean up datasource instance resources.
}

func (d *FireDatasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	log.DefaultLogger.Info("CallResource", "req", req)
	if req.Path == "profileTypes" {
		return d.callProfileTypes(ctx, req, sender)
	}
	return sender.Send(&backend.CallResourceResponse{
		Status: 404,
	})
}

func (d *FireDatasource) callProfileTypes(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	res, err := d.client.ProfileTypes(ctx, connect.NewRequest(&querierv1.ProfileTypesRequest{}))
	if err != nil {
		return err
	}
	data, err := json.Marshal(res.Msg.ProfileTypes)
	if err != nil {
		return err
	}
	err = sender.Send(&backend.CallResourceResponse{Body: data, Headers: req.Headers, Status: 200})
	if err != nil {
		return err
	}
	return nil
}

// QueryData handles multiple queries and returns multiple responses.
// req contains the queries []DataQuery (where each query contains RefID as a unique identifier).
// The QueryDataResponse contains a map of RefID to the response for each query, and each response
// contains Frames ([]*Frame).
func (d *FireDatasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	log.DefaultLogger.Info("QueryData called", "request", req)

	// create response struct
	response := backend.NewQueryDataResponse()

	// loop over queries and execute them individually.
	for _, q := range req.Queries {
		res := d.query(ctx, req.PluginContext, q)

		// save the response in a hashmap
		// based on with RefID as identifier
		response.Responses[q.RefID] = res
	}

	return response, nil
}

type queryModel struct {
	WithStreaming bool `json:"withStreaming"`
}

func (d *FireDatasource) query(ctx context.Context, pCtx backend.PluginContext, query backend.DataQuery) backend.DataResponse {
	response := backend.DataResponse{}

	// Unmarshal the JSON into our queryModel.
	var qm queryModel

	response.Error = json.Unmarshal(query.JSON, &qm)
	if response.Error != nil {
		return response
	}

	// create data frame response.
	frame := data.NewFrame("response")
	frame.Meta = &data.FrameMeta{PreferredVisualization: "profile"}

	// todo parse the query from queryModel
	res, err := d.client.SelectMergeStacktraces(ctx, connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: "memory:inuse_space:bytes:space:bytes",
		Start:         int64(model.TimeFromUnixNano(time.Now().Add(-1 * time.Hour).UnixNano())),
		End:           int64(model.TimeFromUnixNano(time.Now().UnixNano())),
		LabelSelector: "{}",
	}))
	if err != nil {
		response.Error = err
		return response
	}

	// todo create data frame response.
	b, err := json.Marshal(res.Msg)
	if err != nil {
		response.Error = err
		return response
	}
	log.DefaultLogger.Info("SelectMergeStacktraces", "result", string(b))

	// add fields.
	frame.Fields = append(frame.Fields,
		data.NewField("levels.0", nil, []string{`[0, 4862950000000, 0, 0]`}),
		data.NewField("levels.1", nil, []string{`[0, 75210000000, 70000000, 6112, 0, 884550000000, 490000000, 5601]`}),
	)

	// new frame for names
	// frame.a
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

	return response
}

// CheckHealth handles health checks sent from Grafana to the plugin.
// The main use case for these health checks is the test button on the
// datasource configuration page which allows users to verify that
// a datasource is working as expected.
func (d *FireDatasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	log.DefaultLogger.Info("CheckHealth called", "request", req)

	status := backend.HealthStatusOk
	message := "Data source is working"

	if _, err := d.client.ProfileTypes(ctx, connect.NewRequest(&querierv1.ProfileTypesRequest{})); err != nil {
		status = backend.HealthStatusError
		message = err.Error()
	}

	return &backend.CheckHealthResult{
		Status:  status,
		Message: message,
	}, nil
}

// SubscribeStream is called when a client wants to connect to a stream. This callback
// allows sending the first message.
func (d *FireDatasource) SubscribeStream(_ context.Context, req *backend.SubscribeStreamRequest) (*backend.SubscribeStreamResponse, error) {
	log.DefaultLogger.Info("SubscribeStream called", "request", req)

	status := backend.SubscribeStreamStatusPermissionDenied
	if req.Path == "stream" {
		// Allow subscribing only on expected path.
		status = backend.SubscribeStreamStatusOK
	}
	return &backend.SubscribeStreamResponse{
		Status: status,
	}, nil
}

// RunStream is called once for any open channel.  Results are shared with everyone
// subscribed to the same channel.
func (d *FireDatasource) RunStream(ctx context.Context, req *backend.RunStreamRequest, sender *backend.StreamSender) error {
	log.DefaultLogger.Info("RunStream called", "request", req)

	// Create the same data frame as for query data.
	frame := data.NewFrame("response")

	// Add fields (matching the same schema used in QueryData).
	frame.Fields = append(frame.Fields,
		data.NewField("time", nil, make([]time.Time, 1)),
		data.NewField("values", nil, make([]int64, 1)),
	)

	counter := 0

	// Stream data frames periodically till stream closed by Grafana.
	for {
		select {
		case <-ctx.Done():
			log.DefaultLogger.Info("Context done, finish streaming", "path", req.Path)
			return nil
		case <-time.After(time.Second):
			// Send new data periodically.
			frame.Fields[0].Set(0, time.Now())
			frame.Fields[1].Set(0, int64(10*(counter%2+1)))

			counter++

			err := sender.SendFrame(frame, data.IncludeAll)
			if err != nil {
				log.DefaultLogger.Error("Error sending frame", "error", err)
				continue
			}
		}
	}
}

// PublishStream is called when a client sends a message to the stream.
func (d *FireDatasource) PublishStream(_ context.Context, req *backend.PublishStreamRequest) (*backend.PublishStreamResponse, error) {
	log.DefaultLogger.Info("PublishStream called", "request", req)

	// Do not allow publishing at all.
	return &backend.PublishStreamResponse{
		Status: backend.PublishStreamStatusPermissionDenied,
	}, nil
}
