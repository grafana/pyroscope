package linodego

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/linode/linodego/internal/parseabletime"
)

// ImageStatus represents the status of an Image.
type ImageStatus string

// ImageStatus options start with ImageStatus and include all Image statuses
const (
	ImageStatusCreating      ImageStatus = "creating"
	ImageStatusPendingUpload ImageStatus = "pending_upload"
	ImageStatusAvailable     ImageStatus = "available"
)

// Image represents a deployable Image object for use with Linode Instances
type Image struct {
	ID          string      `json:"id"`
	CreatedBy   string      `json:"created_by"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
	Type        string      `json:"type"`
	Vendor      string      `json:"vendor"`
	Status      ImageStatus `json:"status"`
	Size        int         `json:"size"`
	IsPublic    bool        `json:"is_public"`
	Deprecated  bool        `json:"deprecated"`
	Created     *time.Time  `json:"-"`
	Expiry      *time.Time  `json:"-"`
}

// ImageCreateOptions fields are those accepted by CreateImage
type ImageCreateOptions struct {
	DiskID      int    `json:"disk_id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// ImageUpdateOptions fields are those accepted by UpdateImage
type ImageUpdateOptions struct {
	Label       string  `json:"label,omitempty"`
	Description *string `json:"description,omitempty"`
}

// ImageCreateUploadResponse fields are those returned by CreateImageUpload
type ImageCreateUploadResponse struct {
	Image    *Image `json:"image"`
	UploadTo string `json:"upload_to"`
}

// ImageCreateUploadOptions fields are those accepted by CreateImageUpload
type ImageCreateUploadOptions struct {
	Region      string `json:"region"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// ImageUploadOptions fields are those accepted by UploadImage
type ImageUploadOptions struct {
	Region      string `json:"region"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Image       io.Reader
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (i *Image) UnmarshalJSON(b []byte) error {
	type Mask Image

	p := struct {
		*Mask
		Created *parseabletime.ParseableTime `json:"created"`
		Expiry  *parseabletime.ParseableTime `json:"expiry"`
	}{
		Mask: (*Mask)(i),
	}

	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}

	i.Created = (*time.Time)(p.Created)
	i.Expiry = (*time.Time)(p.Expiry)

	return nil
}

// GetUpdateOptions converts an Image to ImageUpdateOptions for use in UpdateImage
func (i Image) GetUpdateOptions() (iu ImageUpdateOptions) {
	iu.Label = i.Label
	iu.Description = copyString(&i.Description)
	return
}

// ImagesPagedResponse represents a linode API response for listing of images
type ImagesPagedResponse struct {
	*PageOptions
	Data []Image `json:"data"`
}

func (ImagesPagedResponse) endpoint(c *Client) string {
	endpoint, err := c.Images.Endpoint()
	if err != nil {
		panic(err)
	}
	return endpoint
}

func (resp *ImagesPagedResponse) appendData(r *ImagesPagedResponse) {
	resp.Data = append(resp.Data, r.Data...)
}

// ListImages lists Images
func (c *Client) ListImages(ctx context.Context, opts *ListOptions) ([]Image, error) {
	response := ImagesPagedResponse{}
	err := c.listHelper(ctx, &response, opts)
	if err != nil {
		return nil, err
	}
	return response.Data, nil
}

// GetImage gets the Image with the provided ID
func (c *Client) GetImage(ctx context.Context, id string) (*Image, error) {
	e, err := c.Images.Endpoint()
	if err != nil {
		return nil, err
	}

	e = fmt.Sprintf("%s/%s", e, id)
	r, err := coupleAPIErrors(c.Images.R(ctx).Get(e))
	if err != nil {
		return nil, err
	}
	return r.Result().(*Image), nil
}

// CreateImage creates a Image
func (c *Client) CreateImage(ctx context.Context, createOpts ImageCreateOptions) (*Image, error) {
	var body string

	e, err := c.Images.Endpoint()
	if err != nil {
		return nil, err
	}

	req := c.R(ctx).SetResult(&Image{})

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
	return r.Result().(*Image), nil
}

// UpdateImage updates the Image with the specified id
func (c *Client) UpdateImage(ctx context.Context, id string, updateOpts ImageUpdateOptions) (*Image, error) {
	var body string

	e, err := c.Images.Endpoint()
	if err != nil {
		return nil, err
	}

	e = fmt.Sprintf("%s/%s", e, id)

	req := c.R(ctx).SetResult(&Image{})

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
	return r.Result().(*Image), nil
}

// DeleteImage deletes the Image with the specified id
func (c *Client) DeleteImage(ctx context.Context, id string) error {
	e, err := c.Images.Endpoint()
	if err != nil {
		return err
	}

	e = fmt.Sprintf("%s/%s", e, id)

	_, err = coupleAPIErrors(c.R(ctx).Delete(e))
	return err
}

// CreateImageUpload creates an Image and an upload URL
func (c *Client) CreateImageUpload(ctx context.Context, createOpts ImageCreateUploadOptions) (image *Image, uploadURL string, err error) {
	var body string

	e, err := c.Images.Endpoint()
	if err != nil {
		return nil, "", err
	}

	e = fmt.Sprintf("%s/upload", e)

	req := c.R(ctx).SetResult(&ImageCreateUploadResponse{})

	if bodyData, err := json.Marshal(createOpts); err == nil {
		body = string(bodyData)
	} else {
		return nil, "", NewError(err)
	}

	r, err := coupleAPIErrors(req.
		SetBody(body).
		Post(e))
	if err != nil {
		return nil, "", err
	}

	result, ok := r.Result().(*ImageCreateUploadResponse)
	if !ok {
		return nil, "", fmt.Errorf("failed to parse result")
	}

	return result.Image, result.UploadTo, nil
}

// UploadImageToURL uploads the given image to the given upload URL
func (c *Client) UploadImageToURL(ctx context.Context, uploadURL string, image io.Reader) error {
	// Linode-specific headers do not need to be sent to this endpoint
	req := resty.New().SetDebug(c.resty.Debug).R().
		SetContext(ctx).
		SetContentLength(true).
		SetHeader("Content-Type", "application/octet-stream").
		SetBody(image)

	_, err := coupleAPIErrors(req.
		Put(uploadURL))

	return err
}

// UploadImage creates and uploads an image
func (c *Client) UploadImage(ctx context.Context, options ImageUploadOptions) (*Image, error) {
	image, uploadURL, err := c.CreateImageUpload(ctx, ImageCreateUploadOptions{
		Label:       options.Label,
		Region:      options.Region,
		Description: options.Description,
	})
	if err != nil {
		return nil, err
	}

	return image, c.UploadImageToURL(ctx, uploadURL, options.Image)
}
