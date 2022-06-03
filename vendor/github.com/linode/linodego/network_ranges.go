package linodego

import (
	"context"
	"encoding/json"
	"fmt"
)

// IPv6RangesPagedResponse represents a paginated IPv6Range API response
type IPv6RangesPagedResponse struct {
	*PageOptions
	Data []IPv6Range `json:"data"`
}

// IPv6RangeCreateOptions fields are those accepted by CreateIPv6Range
type IPv6RangeCreateOptions struct {
	LinodeID     int    `json:"linode_id,omitempty"`
	PrefixLength int    `json:"prefix_length"`
	RouteTarget  string `json:"route_target,omitempty"`
}

// endpoint gets the endpoint URL for IPv6Range
func (IPv6RangesPagedResponse) endpoint(c *Client) string {
	endpoint, err := c.IPv6Ranges.Endpoint()
	if err != nil {
		panic(err)
	}
	return endpoint
}

// appendData appends IPv6Ranges when processing paginated IPv6Range responses
func (resp *IPv6RangesPagedResponse) appendData(r *IPv6RangesPagedResponse) {
	resp.Data = append(resp.Data, r.Data...)
}

// ListIPv6Ranges lists IPv6Ranges
func (c *Client) ListIPv6Ranges(ctx context.Context, opts *ListOptions) ([]IPv6Range, error) {
	response := IPv6RangesPagedResponse{}
	err := c.listHelper(ctx, &response, opts)
	if err != nil {
		return nil, err
	}
	return response.Data, nil
}

// GetIPv6Range gets details about an IPv6 range
func (c *Client) GetIPv6Range(ctx context.Context, ipRange string) (*IPv6Range, error) {
	e, err := c.IPv6Ranges.Endpoint()
	if err != nil {
		return nil, err
	}
	e = fmt.Sprintf("%s/%s", e, ipRange)
	r, err := coupleAPIErrors(c.R(ctx).SetResult(&IPv6Range{}).Get(e))
	if err != nil {
		return nil, err
	}
	return r.Result().(*IPv6Range), nil
}

// CreateIPv6Range creates an IPv6 Range and assigns it based on the provided Linode or route target IPv6 SLAAC address.
func (c *Client) CreateIPv6Range(ctx context.Context, createOpts IPv6RangeCreateOptions) (*IPv6Range, error) {
	var body string
	e, err := c.IPv6Ranges.Endpoint()
	if err != nil {
		return nil, err
	}

	req := c.R(ctx).SetResult(&IPv6Range{})

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
	return r.Result().(*IPv6Range), nil
}

// DeleteIPv6Range deletes an IPv6 Range.
func (c *Client) DeleteIPv6Range(ctx context.Context, ipRange string) error {
	e, err := c.IPv6Ranges.Endpoint()
	if err != nil {
		return err
	}

	req := c.R(ctx)

	e = fmt.Sprintf("%s/%s", e, ipRange)
	_, err = coupleAPIErrors(req.Delete(e))
	return err
}
