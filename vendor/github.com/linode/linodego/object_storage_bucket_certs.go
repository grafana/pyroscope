package linodego

import (
	"context"
	"encoding/json"
)

type ObjectStorageBucketCert struct {
	SSL bool `json:"ssl"`
}

type ObjectStorageBucketCertUploadOptions struct {
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"private_key"`
}

// UploadObjectStorageBucketCert uploads a TLS/SSL Cert to be used with an Object Storage Bucket.
func (c *Client) UploadObjectStorageBucketCert(ctx context.Context, clusterID, bucket string, uploadOpts ObjectStorageBucketCertUploadOptions) (*ObjectStorageBucketCert, error) {
	e, err := c.ObjectStorageBucketCerts.endpointWithParams(clusterID, bucket)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(uploadOpts)
	if err != nil {
		return nil, err
	}

	r, err := coupleAPIErrors(c.R(ctx).SetResult(&ObjectStorageBucketCert{}).SetBody(string(body)).Post(e))
	if err != nil {
		return nil, err
	}
	return r.Result().(*ObjectStorageBucketCert), nil
}

// GetObjectStorageBucketCert gets an ObjectStorageBucketCert
func (c *Client) GetObjectStorageBucketCert(ctx context.Context, clusterID, bucket string) (*ObjectStorageBucketCert, error) {
	e, err := c.ObjectStorageBucketCerts.endpointWithParams(clusterID, bucket)
	if err != nil {
		return nil, err
	}

	r, err := coupleAPIErrors(c.R(ctx).SetResult(&ObjectStorageBucketCert{}).Get(e))
	if err != nil {
		return nil, err
	}
	return r.Result().(*ObjectStorageBucketCert), nil
}

// DeleteObjectStorageBucketCert deletes an ObjectStorageBucketCert
func (c *Client) DeleteObjectStorageBucketCert(ctx context.Context, clusterID, bucket string) error {
	e, err := c.ObjectStorageBucketCerts.endpointWithParams(clusterID, bucket)
	if err != nil {
		return err
	}

	_, err = coupleAPIErrors(c.R(ctx).Delete(e))
	return err
}
