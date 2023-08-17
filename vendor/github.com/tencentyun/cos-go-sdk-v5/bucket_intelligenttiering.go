package cos

import (
	"context"
	"encoding/xml"
	"net/http"
)

type BucketIntelligentTieringTransition struct {
	Days            int `xml:"Days,omitempty" header:"-"`
	RequestFrequent int `xml:"RequestFrequent,omitempty" header:"-"`
}

type BucketPutIntelligentTieringOptions struct {
	XMLName       xml.Name                            `xml:"IntelligentTieringConfiguration" header:"-"`
	Status        string                              `xml:"Status,omitempty" header:"-"`
	Transition    *BucketIntelligentTieringTransition `xml:"Transition,omitempty" header:"-"`
	XOptionHeader *http.Header                        `header:"-,omitempty" url:"-" xml:"-"`
}

type BucketGetIntelligentTieringResult BucketPutIntelligentTieringOptions

type BucketGetIntelligentTieringOptions struct {
	XOptionHeader *http.Header `header:"-,omitempty" url:"-" xml:"-"`
}

func (s *BucketService) PutIntelligentTiering(ctx context.Context, opt *BucketPutIntelligentTieringOptions) (*Response, error) {
	if opt != nil && opt.Transition != nil {
		opt.Transition.RequestFrequent = 1
	}
	sendOpt := sendOptions{
		baseURL:   s.client.BaseURL.BucketURL,
		uri:       "/?intelligenttiering",
		method:    http.MethodPut,
		optHeader: opt,
		body:      opt,
	}
	resp, err := s.client.doRetry(ctx, &sendOpt)
	return resp, err
}

func (s *BucketService) GetIntelligentTiering(ctx context.Context, opt ...*BucketGetIntelligentTieringOptions) (*BucketGetIntelligentTieringResult, *Response, error) {
	var optHeader *BucketGetIntelligentTieringOptions
	if len(opt) > 0 {
		optHeader = opt[0]
	}
	var res BucketGetIntelligentTieringResult
	sendOpt := sendOptions{
		baseURL:   s.client.BaseURL.BucketURL,
		uri:       "/?intelligenttiering",
		method:    http.MethodGet,
		optHeader: optHeader,
		result:    &res,
	}
	resp, err := s.client.doRetry(ctx, &sendOpt)
	return &res, resp, err

}
