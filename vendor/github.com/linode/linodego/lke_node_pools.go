package linodego

import (
	"context"
	"encoding/json"
	"fmt"
)

// LKELinodeStatus constants start with LKELinode and include
// Linode API LKENodePool Linode Status values
type LKELinodeStatus string

// LKENodePoolStatus constants reflect the current status of an LKENodePool
const (
	LKELinodeReady    LKELinodeStatus = "ready"
	LKELinodeNotReady LKELinodeStatus = "not_ready"
)

// LKENodePoolDisk represents a Node disk in an LKENodePool object
type LKENodePoolDisk struct {
	Size int    `json:"size"`
	Type string `json:"type"`
}

type LKENodePoolAutoscaler struct {
	Enabled bool `json:"enabled"`
	Min     int  `json:"min"`
	Max     int  `json:"max"`
}

// LKENodePoolLinode represents a LKENodePoolLinode object
type LKENodePoolLinode struct {
	ID         string          `json:"id"`
	InstanceID int             `json:"instance_id"`
	Status     LKELinodeStatus `json:"status"`
}

// LKENodePool represents a LKENodePool object
type LKENodePool struct {
	ID      int                 `json:"id"`
	Count   int                 `json:"count"`
	Type    string              `json:"type"`
	Disks   []LKENodePoolDisk   `json:"disks"`
	Linodes []LKENodePoolLinode `json:"nodes"`
	Tags    []string            `json:"tags"`

	Autoscaler LKENodePoolAutoscaler `json:"autoscaler"`
}

// LKENodePoolCreateOptions fields are those accepted by CreateLKENodePool
type LKENodePoolCreateOptions struct {
	Count int               `json:"count"`
	Type  string            `json:"type"`
	Disks []LKENodePoolDisk `json:"disks"`
	Tags  []string          `json:"tags"`

	Autoscaler *LKENodePoolAutoscaler `json:"autoscaler,omitempty"`
}

// LKENodePoolUpdateOptions fields are those accepted by UpdateLKENodePoolUpdate
type LKENodePoolUpdateOptions struct {
	Count int       `json:"count,omitempty"`
	Tags  *[]string `json:"tags,omitempty"`

	Autoscaler *LKENodePoolAutoscaler `json:"autoscaler,omitempty"`
}

// GetCreateOptions converts a LKENodePool to LKENodePoolCreateOptions for
// use in CreateLKENodePool
func (l LKENodePool) GetCreateOptions() (o LKENodePoolCreateOptions) {
	o.Count = l.Count
	o.Disks = l.Disks
	o.Tags = l.Tags
	o.Autoscaler = &l.Autoscaler
	return
}

// GetUpdateOptions converts a LKENodePool to LKENodePoolUpdateOptions for use in UpdateLKENodePoolUpdate
func (l LKENodePool) GetUpdateOptions() (o LKENodePoolUpdateOptions) {
	o.Count = l.Count
	o.Tags = &l.Tags
	o.Autoscaler = &l.Autoscaler
	return
}

// LKENodePoolsPagedResponse represents a paginated LKENodePool API response
type LKENodePoolsPagedResponse struct {
	*PageOptions
	Data []LKENodePool `json:"data"`
}

// endpointWithID gets the endpoint URL for InstanceConfigs of a given Instance
func (LKENodePoolsPagedResponse) endpointWithID(c *Client, id int) string {
	endpoint, err := c.LKENodePools.endpointWithParams(id)
	if err != nil {
		panic(err)
	}
	return endpoint
}

// appendData appends LKENodePools when processing paginated LKENodePool responses
func (resp *LKENodePoolsPagedResponse) appendData(r *LKENodePoolsPagedResponse) {
	resp.Data = append(resp.Data, r.Data...)
}

// ListLKENodePools lists LKENodePools
func (c *Client) ListLKENodePools(ctx context.Context, clusterID int, opts *ListOptions) ([]LKENodePool, error) {
	response := LKENodePoolsPagedResponse{}
	err := c.listHelperWithID(ctx, &response, clusterID, opts)
	if err != nil {
		return nil, err
	}

	return response.Data, nil
}

// GetLKENodePool gets the LKENodePool with the provided ID
func (c *Client) GetLKENodePool(ctx context.Context, clusterID, id int) (*LKENodePool, error) {
	e, err := c.LKENodePools.endpointWithParams(clusterID)
	if err != nil {
		return nil, err
	}
	e = fmt.Sprintf("%s/%d", e, id)
	r, err := coupleAPIErrors(c.R(ctx).SetResult(&LKENodePool{}).Get(e))
	if err != nil {
		return nil, err
	}
	return r.Result().(*LKENodePool), nil
}

// CreateLKENodePool creates a LKENodePool
func (c *Client) CreateLKENodePool(ctx context.Context, clusterID int, createOpts LKENodePoolCreateOptions) (*LKENodePool, error) {
	var body string
	e, err := c.LKENodePools.endpointWithParams(clusterID)
	if err != nil {
		return nil, err
	}

	req := c.R(ctx).SetResult(&LKENodePool{})

	if bodyData, err := json.Marshal(createOpts); err == nil {
		body = string(bodyData)
	} else {
		return nil, NewError(err)
	}

	r, err := coupleAPIErrors(req.
		SetBody(body).
		Post(e))
	if err != nil {
		return nil, err
	}
	return r.Result().(*LKENodePool), nil
}

// UpdateLKENodePool updates the LKENodePool with the specified id
func (c *Client) UpdateLKENodePool(ctx context.Context, clusterID, id int, updateOpts LKENodePoolUpdateOptions) (*LKENodePool, error) {
	var body string
	e, err := c.LKENodePools.endpointWithParams(clusterID)
	if err != nil {
		return nil, err
	}
	e = fmt.Sprintf("%s/%d", e, id)

	req := c.R(ctx).SetResult(&LKENodePool{})

	if bodyData, err := json.Marshal(updateOpts); err == nil {
		body = string(bodyData)
	} else {
		return nil, NewError(err)
	}

	r, err := coupleAPIErrors(req.
		SetBody(body).
		Put(e))
	if err != nil {
		return nil, err
	}
	return r.Result().(*LKENodePool), nil
}

// DeleteLKENodePool deletes the LKENodePool with the specified id
func (c *Client) DeleteLKENodePool(ctx context.Context,
	clusterID, id int) error {
	e, err := c.LKENodePools.endpointWithParams(clusterID)
	if err != nil {
		return err
	}
	e = fmt.Sprintf("%s/%d", e, id)

	_, err = coupleAPIErrors(c.R(ctx).Delete(e))
	return err
}

// DeleteLKENodePoolNode deletes a given node from a node pool
func (c *Client) DeleteLKENodePoolNode(ctx context.Context, clusterID int, id string) error {
	e, err := c.LKEClusters.Endpoint()
	if err != nil {
		return err
	}
	e = fmt.Sprintf("%s/%d/nodes/%s", e, clusterID, id)

	_, err = coupleAPIErrors(c.R(ctx).Delete(e))
	return err
}
