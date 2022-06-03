package linodego

import (
	"context"
	"encoding/json"
	"time"

	"github.com/linode/linodego/internal/parseabletime"
)

type VLAN struct {
	Label   string     `json:"label"`
	Linodes []int      `json:"linodes"`
	Region  string     `json:"region"`
	Created *time.Time `json:"-"`
}

// UnmarshalJSON for VLAN responses
func (v *VLAN) UnmarshalJSON(b []byte) error {
	type Mask VLAN

	p := struct {
		*Mask
		Created *parseabletime.ParseableTime `json:"created"`
	}{
		Mask: (*Mask)(v),
	}

	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}

	v.Created = (*time.Time)(p.Created)
	return nil
}

// VLANsPagedResponse represents a Linode API response for listing of VLANs
type VLANsPagedResponse struct {
	*PageOptions
	Data []VLAN `json:"data"`
}

func (VLANsPagedResponse) endpoint(c *Client) string {
	endpoint, err := c.VLANs.Endpoint()
	if err != nil {
		panic(err)
	}
	return endpoint
}

func (resp *VLANsPagedResponse) appendData(r *VLANsPagedResponse) {
	resp.Data = append(resp.Data, r.Data...)
}

// ListVLANs returns a paginated list of VLANs
func (c *Client) ListVLANs(ctx context.Context, opts *ListOptions) ([]VLAN, error) {
	response := VLANsPagedResponse{}

	err := c.listHelper(ctx, &response, opts)
	if err != nil {
		return nil, err
	}

	return response.Data, nil
}
