package linodego

import (
	"context"
	"encoding/json"
)

type GrantPermissionLevel string

const (
	AccessLevelReadOnly  GrantPermissionLevel = "read_only"
	AccessLevelReadWrite GrantPermissionLevel = "read_write"
)

type GlobalUserGrants struct {
	AccountAccess        *GrantPermissionLevel `json:"account_access"`
	AddDomains           bool                  `json:"add_domains"`
	AddFirewalls         bool                  `json:"add_firewalls"`
	AddImages            bool                  `json:"add_images"`
	AddLinodes           bool                  `json:"add_linodes"`
	AddLongview          bool                  `json:"add_longview"`
	AddNodeBalancers     bool                  `json:"add_nodebalancers"`
	AddStackScripts      bool                  `json:"add_stackscripts"`
	AddVolumes           bool                  `json:"add_volumes"`
	CancelAccount        bool                  `json:"cancel_account"`
	LongviewSubscription bool                  `json:"longview_subscription"`
}

type EntityUserGrant struct {
	ID          int                   `json:"id"`
	Permissions *GrantPermissionLevel `json:"permissions"`
}

type GrantedEntity struct {
	ID          int                  `json:"id"`
	Label       string               `json:"label"`
	Permissions GrantPermissionLevel `json:"permissions"`
}

type UserGrants struct {
	Domain       []GrantedEntity `json:"domain"`
	Firewall     []GrantedEntity `json:"firewall"`
	Image        []GrantedEntity `json:"image"`
	Linode       []GrantedEntity `json:"linode"`
	Longview     []GrantedEntity `json:"longview"`
	NodeBalancer []GrantedEntity `json:"nodebalancer"`
	StackScript  []GrantedEntity `json:"stackscript"`
	Volume       []GrantedEntity `json:"volume"`

	Global GlobalUserGrants `json:"global"`
}

type UserGrantsUpdateOptions struct {
	Domain       []EntityUserGrant `json:"domain,omitempty"`
	Firewall     []EntityUserGrant `json:"firewall,omitempty"`
	Image        []EntityUserGrant `json:"image,omitempty"`
	Linode       []EntityUserGrant `json:"linode,omitempty"`
	Longview     []EntityUserGrant `json:"longview,omitempty"`
	NodeBalancer []EntityUserGrant `json:"nodebalancer,omitempty"`
	StackScript  []EntityUserGrant `json:"stackscript,omitempty"`
	Volume       []EntityUserGrant `json:"volume,omitempty"`

	Global GlobalUserGrants `json:"global"`
}

func (c *Client) GetUserGrants(ctx context.Context, username string) (*UserGrants, error) {
	e, err := c.UserGrants.endpointWithParams(username)
	if err != nil {
		return nil, err
	}

	r, err := coupleAPIErrors(c.R(ctx).SetResult(&UserGrants{}).Get(e))
	if err != nil {
		return nil, err
	}

	return r.Result().(*UserGrants), nil
}

func (c *Client) UpdateUserGrants(ctx context.Context, username string, updateOpts UserGrantsUpdateOptions) (*UserGrants, error) {
	var body string

	e, err := c.UserGrants.endpointWithParams(username)
	if err != nil {
		return nil, err
	}

	req := c.R(ctx).SetResult(&UserGrants{})

	if bodyData, err := json.Marshal(updateOpts); err == nil {
		body = string(bodyData)
	} else {
		return nil, NewError(err)
	}

	r, err := coupleAPIErrors(req.SetBody(body).Put(e))
	if err != nil {
		return nil, err
	}

	return r.Result().(*UserGrants), nil
}
