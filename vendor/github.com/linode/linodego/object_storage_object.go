package linodego

import (
	"context"
	"encoding/json"
	"fmt"
)

type ObjectStorageObjectURLCreateOptions struct {
	Name               string `json:"name"`
	Method             string `json:"method"`
	ContentType        string `json:"content_type,omit_empty"`
	ContentDisposition string `json:"content_disposition,omit_empty"`
	ExpiresIn          *int   `json:"expires_in,omit_empty"`
}

type ObjectStorageObjectURL struct {
	URL    string `json:"url"`
	Exists bool   `json:"exists"`
}

type ObjectStorageObjectACLConfig struct {
	ACL    string `json:"acl"`
	ACLXML string `json:"acl_xml"`
}

type ObjectStorageObjectACLConfigUpdateOptions struct {
	Name string `json:"name"`
	ACL  string `json:"acl"`
}

func (c *Client) CreateObjectStorageObjectURL(ctx context.Context, clusterID, label string, options ObjectStorageObjectURLCreateOptions) (*ObjectStorageObjectURL, error) {
	var body string
	e, err := c.ObjectStorageBuckets.Endpoint()
	if err != nil {
		return nil, err
	}

	req := c.R(ctx).SetResult(&ObjectStorageObjectURL{})
	e = fmt.Sprintf("%s/%s/%s/object-url", e, clusterID, label)

	if bodyData, err := json.Marshal(options); err == nil {
		body = string(bodyData)
	} else {
		return nil, NewError(err)
	}

	r, err := coupleAPIErrors(req.SetBody(body).Post(e))
	return r.Result().(*ObjectStorageObjectURL), err
}

func (c *Client) GetObjectStorageObjectACLConfig(ctx context.Context, clusterID, label, object string) (*ObjectStorageObjectACLConfig, error) {
	e, err := c.ObjectStorageBuckets.Endpoint()
	if err != nil {
		return nil, err
	}

	req := c.R(ctx).SetResult(&ObjectStorageObjectACLConfig{})
	e = fmt.Sprintf("%s/%s/%s/object-acl?name=%s", e, clusterID, label, object)

	r, err := coupleAPIErrors(req.Get(e))
	return r.Result().(*ObjectStorageObjectACLConfig), err
}

func (c *Client) UpdateObjectStorageObjectACLConfig(ctx context.Context, clusterID, label string, options ObjectStorageObjectACLConfigUpdateOptions) (*ObjectStorageObjectACLConfig, error) {
	var body string
	e, err := c.ObjectStorageBuckets.Endpoint()
	if err != nil {
		return nil, err
	}

	req := c.R(ctx).SetResult(&ObjectStorageObjectACLConfig{})
	e = fmt.Sprintf("%s/%s/%s/object-acl", e, clusterID, label)

	if bodyData, err := json.Marshal(options); err == nil {
		body = string(bodyData)
	} else {
		return nil, NewError(err)
	}

	r, err := coupleAPIErrors(req.SetBody(body).Put(e))
	return r.Result().(*ObjectStorageObjectACLConfig), err
}
