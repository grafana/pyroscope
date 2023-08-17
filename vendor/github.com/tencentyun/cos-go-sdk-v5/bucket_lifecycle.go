package cos

import (
	"context"
	"encoding/xml"
	"net/http"
)

type BucketLifecycleAndOperator struct {
	Prefix string             `xml:"Prefix,omitempty" header:"-"`
	Tag    []BucketTaggingTag `xml:"Tag,omitempty" header:"-"`
}

// BucketLifecycleFilter is the param of BucketLifecycleRule
type BucketLifecycleFilter struct {
	Prefix string                      `xml:"Prefix,omitempty" header:"-"`
	Tag    *BucketTaggingTag           `xml:"Tag,omitempty" header:"-"`
	And    *BucketLifecycleAndOperator `xml:"And,omitempty" header:"-"`
}

// BucketLifecycleExpiration is the param of BucketLifecycleRule
type BucketLifecycleExpiration struct {
	Date                      string `xml:"Date,omitempty" header:"-"`
	Days                      int    `xml:"Days,omitempty" header:"-"`
	ExpiredObjectDeleteMarker bool   `xml:"ExpiredObjectDeleteMarker,omitempty" header:"-"`
}

// BucketLifecycleTransition is the param of BucketLifecycleRule
type BucketLifecycleTransition struct {
	Date         string `xml:"Date,omitempty" header:"-"`
	Days         int    `xml:"Days,omitempty" header:"-"`
	StorageClass string `xml:"StorageClass,omitempty" header:"-"`
}

type BucketLifecycleNoncurrentVersion struct {
	NoncurrentDays int    `xml:"NoncurrentDays,omitempty" header:"-"`
	StorageClass   string `xml:"StorageClass,omitempty" header:"-"`
}

// BucketLifecycleAbortIncompleteMultipartUpload is the param of BucketLifecycleRule
type BucketLifecycleAbortIncompleteMultipartUpload struct {
	DaysAfterInitiation int `xml:"DaysAfterInitiation,omitempty" header:"-"`
}

// BucketLifecycleRule is the rule of BucketLifecycle
type BucketLifecycleRule struct {
	ID                             string                                         `xml:"ID,omitempty" header:"-"`
	Status                         string                                         `xml:"Status,omitempty" header:"-"`
	Filter                         *BucketLifecycleFilter                         `xml:"Filter,omitempty" header:"-"`
	Transition                     []BucketLifecycleTransition                    `xml:"Transition,omitempty" header:"-"`
	Expiration                     *BucketLifecycleExpiration                     `xml:"Expiration,omitempty" header:"-"`
	AbortIncompleteMultipartUpload *BucketLifecycleAbortIncompleteMultipartUpload `xml:"AbortIncompleteMultipartUpload,omitempty" header:"-"`
	NoncurrentVersionTransition    []BucketLifecycleNoncurrentVersion             `xml:"NoncurrentVersionTransition,omitempty" header:"-"`
	NoncurrentVersionExpiration    *BucketLifecycleNoncurrentVersion              `xml:"NoncurrentVersionExpiration,omitempty" header:"-"`
}

// BucketGetLifecycleResult is the result of BucketGetLifecycle
type BucketGetLifecycleResult struct {
	XMLName xml.Name              `xml:"LifecycleConfiguration" header:"-"`
	Rules   []BucketLifecycleRule `xml:"Rule,omitempty" header:"-"`
}

type BucketGetLifecycleOptions struct {
	XOptionHeader *http.Header `header:"-,omitempty" url:"-" xml:"-"`
}

// GetLifecycle 请求实现读取生命周期管理的配置。当配置不存在时，返回404 Not Found。
// https://www.qcloud.com/document/product/436/8278
func (s *BucketService) GetLifecycle(ctx context.Context, opt ...*BucketGetLifecycleOptions) (*BucketGetLifecycleResult, *Response, error) {
	var optHeader *BucketGetLifecycleOptions
	if len(opt) > 0 {
		optHeader = opt[0]
	}
	var res BucketGetLifecycleResult
	sendOpt := sendOptions{
		baseURL:   s.client.BaseURL.BucketURL,
		uri:       "/?lifecycle",
		method:    http.MethodGet,
		optHeader: optHeader,
		result:    &res,
	}
	resp, err := s.client.doRetry(ctx, &sendOpt)
	return &res, resp, err
}

// BucketPutLifecycleOptions is the option of PutBucketLifecycle
type BucketPutLifecycleOptions struct {
	XMLName       xml.Name              `xml:"LifecycleConfiguration" header:"-"`
	Rules         []BucketLifecycleRule `xml:"Rule,omitempty" header:"-"`
	XOptionHeader *http.Header          `header:"-,omitempty" url:"-" xml:"-"`
}

// PutLifecycle 请求实现设置生命周期管理的功能。您可以通过该请求实现数据的生命周期管理配置和定期删除。
// 此请求为覆盖操作，上传新的配置文件将覆盖之前的配置文件。生命周期管理对文件和文件夹同时生效。
// https://www.qcloud.com/document/product/436/8280
func (s *BucketService) PutLifecycle(ctx context.Context, opt *BucketPutLifecycleOptions) (*Response, error) {
	sendOpt := sendOptions{
		baseURL:   s.client.BaseURL.BucketURL,
		uri:       "/?lifecycle",
		method:    http.MethodPut,
		optHeader: opt,
		body:      opt,
	}
	resp, err := s.client.doRetry(ctx, &sendOpt)
	return resp, err
}

type BucketDeleteLifecycleOptions struct {
	XOptionHeader *http.Header `header:"-,omitempty" url:"-" xml:"-"`
}

// DeleteLifecycle 请求实现删除生命周期管理。
// https://www.qcloud.com/document/product/436/8284
func (s *BucketService) DeleteLifecycle(ctx context.Context, opt ...*BucketDeleteLifecycleOptions) (*Response, error) {
	var optHeader *BucketDeleteLifecycleOptions
	if len(opt) > 0 {
		optHeader = opt[0]
	}
	sendOpt := sendOptions{
		baseURL:   s.client.BaseURL.BucketURL,
		uri:       "/?lifecycle",
		optHeader: optHeader,
		method:    http.MethodDelete,
	}
	resp, err := s.client.doRetry(ctx, &sendOpt)
	return resp, err
}
