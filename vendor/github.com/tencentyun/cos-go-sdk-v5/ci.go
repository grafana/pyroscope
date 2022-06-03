package cos

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/crc64"
	"io"
	"net/http"
	"os"
	"strconv"
)

type CIService service

type PicOperations struct {
	IsPicInfo int                  `json:"is_pic_info,omitempty"`
	Rules     []PicOperationsRules `json:"rules,omitemtpy"`
}
type PicOperationsRules struct {
	Bucket string `json:"bucket,omitempty"`
	FileId string `json:"fileid"`
	Rule   string `json:"rule"`
}

func EncodePicOperations(pic *PicOperations) string {
	if pic == nil {
		return ""
	}
	bs, err := json.Marshal(pic)
	if err != nil {
		return ""
	}
	return string(bs)
}

type ImageProcessResult struct {
	XMLName        xml.Name          `xml:"UploadResult"`
	OriginalInfo   *PicOriginalInfo  `xml:"OriginalInfo,omitempty"`
	ProcessResults *PicProcessObject `xml:"ProcessResults>Object,omitempty"`
}
type PicOriginalInfo struct {
	Key       string        `xml:"Key,omitempty"`
	Location  string        `xml:"Location,omitempty"`
	ImageInfo *PicImageInfo `xml:"ImageInfo,omitempty"`
	ETag      string        `xml:"ETag,omitempty"`
}
type PicImageInfo struct {
	Format      string `xml:"Format,omitempty"`
	Width       int    `xml:"Width,omitempty"`
	Height      int    `xml:"Height,omitempty"`
	Quality     int    `xml:"Quality,omitempty"`
	Ave         string `xml:"Ave,omitempty"`
	Orientation int    `xml:"Orientation,omitempty"`
}
type PicProcessObject struct {
	Key             string       `xml:"Key,omitempty"`
	Location        string       `xml:"Location,omitempty"`
	Format          string       `xml:"Format,omitempty"`
	Width           int          `xml:"Width,omitempty"`
	Height          int          `xml:"Height,omitempty"`
	Size            int          `xml:"Size,omitempty"`
	Quality         int          `xml:"Quality,omitempty"`
	ETag            string       `xml:"ETag,omitempty"`
	WatermarkStatus int          `xml:"WatermarkStatus,omitempty"`
	CodeStatus      int          `xml:"CodeStatus,omitempty"`
	QRcodeInfo      []QRcodeInfo `xml:"QRcodeInfo,omitempty"`
}
type QRcodeInfo struct {
	CodeUrl      string        `xml:"CodeUrl,omitempty"`
	CodeLocation *CodeLocation `xml:"CodeLocation,omitempty"`
}
type CodeLocation struct {
	Point []string `xml:"Point,omitempty"`
}

type picOperationsHeader struct {
	PicOperations string `header:"Pic-Operations" xml:"-" url:"-"`
}

type ImageProcessOptions = PicOperations

// 云上数据处理 https://cloud.tencent.com/document/product/460/18147
func (s *CIService) ImageProcess(ctx context.Context, name string, opt *ImageProcessOptions) (*ImageProcessResult, *Response, error) {
	header := &picOperationsHeader{
		PicOperations: EncodePicOperations(opt),
	}
	var res ImageProcessResult
	sendOpt := sendOptions{
		baseURL:   s.client.BaseURL.BucketURL,
		uri:       "/" + encodeURIComponent(name) + "?image_process",
		method:    http.MethodPost,
		optHeader: header,
		result:    &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// ImageRecognitionOptions is the option of ImageAuditing
type ImageRecognitionOptions struct {
	CIProcess        string `url:"ci-process,omitempty"`
	DetectType       string `url:"detect-type,omitempty"`
	DetectUrl        string `url:"detect-url,omitempty"`
	Interval         int    `url:"interval,omitempty"`
	MaxFrames        int    `url:"max-frames,omitempty"`
	BizType          string `url:"biz-type,omitempty"`
	LargeImageDetect int    `url:"large-image-detect,omitempty"`
}

// ImageRecognitionResult is the result of ImageRecognition/ImageAuditing
type ImageRecognitionResult struct {
	XMLName           xml.Name         `xml:"RecognitionResult"`
	JobId             string           `xml:"JobId,omitempty"`
	Text              string           `xml:"Text,omitempty"`
	Label             string           `xml:"Label,omitempty"`
	Result            int              `xml:"Result,omitempty"`
	Score             int              `xml:"Score,omitempty"`
	SubLabel          string           `xml:"SubLabel,omitempty"`
	PornInfo          *RecognitionInfo `xml:"PornInfo,omitempty"`
	TerroristInfo     *RecognitionInfo `xml:"TerroristInfo,omitempty"`
	PoliticsInfo      *RecognitionInfo `xml:"PoliticsInfo,omitempty"`
	AdsInfo           *RecognitionInfo `xml:"AdsInfo,omitempty"`
	TeenagerInfo      *RecognitionInfo `xml:"TeenagerInfo,omitempty"`
	TerrorismInfo     *RecognitionInfo `xml:"TerrorismInfo,omitempty"`
	CompressionResult int              `xml:"CompressionResult,omitempty"`
}

// RecognitionInfo is the result of auditing scene
type RecognitionInfo struct {
	Code          int            `xml:"Code,omitempty"`
	Msg           string         `xml:"Msg,omitempty"`
	HitFlag       int            `xml:"HitFlag,omitempty"`
	Score         int            `xml:"Score,omitempty"`
	Label         string         `xml:"Label,omitempty"`
	Count         int            `xml:"Count,omitempty"`
	SubLabel      string         `xml:"SubLabel,omitempty"`
	Keywords      []string       `xml:"Keywords,omitempty"`
	OcrResults    []OcrResult    `xml:"OcrResults,omitempty"`
	ObjectResults []ObjectResult `xml:"ObjectResults,omitempty"`
	LibResults    []LibResult    `xml:"LibResults,omitempty"`
}

// 图片审核 https://cloud.tencent.com/document/product/460/37318
func (s *CIService) ImageRecognition(ctx context.Context, name string, DetectType string) (*ImageRecognitionResult, *Response, error) {
	opt := &ImageRecognitionOptions{
		CIProcess:  "sensitive-content-recognition",
		DetectType: DetectType,
	}
	var res ImageRecognitionResult
	sendOpt := sendOptions{
		baseURL:  s.client.BaseURL.BucketURL,
		uri:      "/" + encodeURIComponent(name),
		method:   http.MethodGet,
		optQuery: opt,
		result:   &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// 图片审核 支持detect-url等全部参数
func (s *CIService) ImageAuditing(ctx context.Context, name string, opt *ImageRecognitionOptions) (*ImageRecognitionResult, *Response, error) {
	var res ImageRecognitionResult
	sendOpt := sendOptions{
		baseURL:  s.client.BaseURL.BucketURL,
		uri:      "/" + encodeURIComponent(name),
		method:   http.MethodGet,
		optQuery: opt,
		result:   &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// ImageAuditingInputOptions is the option of BatchImageAuditingOptions
type ImageAuditingInputOptions struct {
	DataId           string `xml:",omitempty"`
	Object           string `xml:",omitempty"`
	Url              string `xml:",omitempty"`
	Interval         int    `xml:",omitempty"`
	MaxFrames        int    `xml:",omitempty"`
	LargeImageDetect int    `xml:",omitempty"`
}

// ImageAuditingJobConf is the config of BatchImageAuditingOptions
type ImageAuditingJobConf struct {
	DetectType string `xml:",omitempty"`
	BizType    string `xml:",omitempty"`
}

// BatchImageAuditingOptions is the option of BatchImageAuditing
type BatchImageAuditingOptions struct {
	XMLName xml.Name                    `xml:"Request"`
	Input   []ImageAuditingInputOptions `xml:"Input,omitempty"`
	Conf    *ImageAuditingJobConf       `xml:"Conf"`
}

// ImageAuditingResult is the result of BatchImageAuditingJobResult
type ImageAuditingResult struct {
	Code              string           `xml:",omitempty"`
	Message           string           `xml:",omitempty"`
	JobId             string           `xml:"JobId,omitempty"`
	DataId            string           `xml:",omitempty"`
	Object            string           `xml:",omitempty"`
	Url               string           `xml:",omitempty"`
	Text              string           `xml:",omitempty"`
	Label             string           `xml:",omitempty"`
	Result            int              `xml:",omitempty"`
	Score             int              `xml:",omitempty"`
	SubLabel          string           `xml:",omitempty"`
	PornInfo          *RecognitionInfo `xml:",omitempty"`
	TerrorismInfo     *RecognitionInfo `xml:",omitempty"`
	PoliticsInfo      *RecognitionInfo `xml:",omitempty"`
	AdsInfo           *RecognitionInfo `xml:",omitempty"`
	TeenagerInfo      *RecognitionInfo `xml:",omitempty"`
	CompressionResult int              `xml:",omitempty"`
}

// BatchImageAuditingJobResult is the result of BatchImageAuditing
type BatchImageAuditingJobResult struct {
	XMLName    xml.Name              `xml:"Response"`
	JobsDetail []ImageAuditingResult `xml:",omitempty"`
	RequestId  string                `xml:",omitempty"`
}

// 图片批量审核接口
func (s *CIService) BatchImageAuditing(ctx context.Context, opt *BatchImageAuditingOptions) (*BatchImageAuditingJobResult, *Response, error) {
	var res BatchImageAuditingJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/image/auditing",
		method:  http.MethodPost,
		body:    opt,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// GetImageAuditingJobResult is the result of GetImageAuditingJob
type GetImageAuditingJobResult struct {
	XMLName    xml.Name             `xml:"Response"`
	JobsDetail *ImageAuditingResult `xml:",omitempty"`
	RequestId  string               `xml:",omitempty"`
}

// 图片审核-查询任务
func (s *CIService) GetImageAuditingJob(ctx context.Context, jobid string) (*GetImageAuditingJobResult, *Response, error) {
	var res GetImageAuditingJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/image/auditing/" + jobid,
		method:  http.MethodGet,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// PutVideoAuditingJobOptions is the option of PutVideoAuditingJob
type PutVideoAuditingJobOptions struct {
	XMLName     xml.Name              `xml:"Request"`
	InputObject string                `xml:"Input>Object,omitempty"`
	InputUrl    string                `xml:"Input>Url,omitempty"`
	InputDataId string                `xml:"Input>DataId,omitempty"`
	Conf        *VideoAuditingJobConf `xml:"Conf"`
}

// VideoAuditingJobConf is the config of PutVideoAuditingJobOptions
type VideoAuditingJobConf struct {
	DetectType      string                       `xml:",omitempty"`
	Snapshot        *PutVideoAuditingJobSnapshot `xml:",omitempty"`
	Callback        string                       `xml:",omitempty"`
	CallbackVersion string                       `xml:",omitempty"`
	BizType         string                       `xml:",omitempty"`
	DetectContent   int                          `xml:",omitempty"`
}

// PutVideoAuditingJobSnapshot is the snapshot config of VideoAuditingJobConf
type PutVideoAuditingJobSnapshot struct {
	Mode         string  `xml:",omitempty"`
	Count        int     `xml:",omitempty"`
	TimeInterval float32 `xml:",omitempty"`
}

// PutVideoAuditingJobResult is the result of PutVideoAuditingJob
type PutVideoAuditingJobResult struct {
	XMLName    xml.Name `xml:"Response"`
	JobsDetail struct {
		JobId        string `xml:"JobId,omitempty"`
		State        string `xml:"State,omitempty"`
		CreationTime string `xml:"CreationTime,omitempty"`
		Object       string `xml:"Object,omitempty"`
		Url          string `xml:"Url,omitempty"`
	} `xml:"JobsDetail,omitempty"`
	RequestId string `xml:"RequestId,omitempty"`
}

// 视频审核-创建任务 https://cloud.tencent.com/document/product/460/46427
func (s *CIService) PutVideoAuditingJob(ctx context.Context, opt *PutVideoAuditingJobOptions) (*PutVideoAuditingJobResult, *Response, error) {
	var res PutVideoAuditingJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/video/auditing",
		method:  http.MethodPost,
		body:    opt,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// GetVideoAuditingJobResult is the result of GetVideoAuditingJob
type GetVideoAuditingJobResult struct {
	XMLName    xml.Name           `xml:"Response"`
	JobsDetail *AuditingJobDetail `xml:",omitempty"`
	RequestId  string             `xml:",omitempty"`
}

// AuditingJobDetail is the detail of GetVideoAuditingJobResult
type AuditingJobDetail struct {
	Code          string                        `xml:",omitempty"`
	Message       string                        `xml:",omitempty"`
	JobId         string                        `xml:",omitempty"`
	State         string                        `xml:",omitempty"`
	CreationTime  string                        `xml:",omitempty"`
	Object        string                        `xml:",omitempty"`
	Url           string                        `xml:",omitempty"`
	DataId        string                        `xml:",omitempty"`
	SnapshotCount string                        `xml:",omitempty"`
	Label         string                        `xml:",omitempty"`
	Result        int                           `xml:",omitempty"`
	PornInfo      *RecognitionInfo              `xml:",omitempty"`
	TerrorismInfo *RecognitionInfo              `xml:",omitempty"`
	PoliticsInfo  *RecognitionInfo              `xml:",omitempty"`
	AdsInfo       *RecognitionInfo              `xml:",omitempty"`
	TeenagerInfo  *RecognitionInfo              `xml:",omitempty"`
	Snapshot      []GetVideoAuditingJobSnapshot `xml:",omitempty"`
	AudioSection  []AudioSectionResult          `xml:",omitempty"`
}

// GetVideoAuditingJobSnapshot is the snapshot result of AuditingJobDetail
type GetVideoAuditingJobSnapshot struct {
	Url           string           `xml:",omitempty"`
	Text          string           `xml:",omitempty"`
	SnapshotTime  int              `xml:",omitempty"`
	Label         string           `xml:",omitempty"`
	Result        int              `xml:",omitempty"`
	PornInfo      *RecognitionInfo `xml:",omitempty"`
	TerrorismInfo *RecognitionInfo `xml:",omitempty"`
	PoliticsInfo  *RecognitionInfo `xml:",omitempty"`
	AdsInfo       *RecognitionInfo `xml:",omitempty"`
	TeenagerInfo  *RecognitionInfo `xml:",omitempty"`
}

// AudioSectionResult is the audio section result of AuditingJobDetail/AudioAuditingJobDetail
type AudioSectionResult struct {
	Url           string           `xml:",omitempty"`
	Text          string           `xml:",omitempty"`
	OffsetTime    int              `xml:",omitempty"`
	Duration      int              `xml:",omitempty"`
	Label         string           `xml:",omitempty"`
	Result        int              `xml:",omitempty"`
	PornInfo      *RecognitionInfo `xml:",omitempty"`
	TerrorismInfo *RecognitionInfo `xml:",omitempty"`
	PoliticsInfo  *RecognitionInfo `xml:",omitempty"`
	AdsInfo       *RecognitionInfo `xml:",omitempty"`
}

// 视频审核-查询任务 https://cloud.tencent.com/document/product/460/46926
func (s *CIService) GetVideoAuditingJob(ctx context.Context, jobid string) (*GetVideoAuditingJobResult, *Response, error) {
	var res GetVideoAuditingJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/video/auditing/" + jobid,
		method:  http.MethodGet,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// PutAudioAuditingJobOptions is the option of PutAudioAuditingJob
type PutAudioAuditingJobOptions struct {
	XMLName     xml.Name              `xml:"Request"`
	InputObject string                `xml:"Input>Object,omitempty"`
	InputUrl    string                `xml:"Input>Url,omitempty"`
	InputDataId string                `xml:"Input>DataId,omitempty"`
	Conf        *AudioAuditingJobConf `xml:"Conf"`
}

// AudioAuditingJobConf is the config of PutAudioAuditingJobOptions
type AudioAuditingJobConf struct {
	DetectType      string `xml:",omitempty"`
	Callback        string `xml:",omitempty"`
	CallbackVersion string `xml:",omitempty"`
	BizType         string `xml:",omitempty"`
}

// PutAudioAuditingJobResult is the result of PutAudioAuditingJob
type PutAudioAuditingJobResult PutVideoAuditingJobResult

// 音频审核-创建任务 https://cloud.tencent.com/document/product/460/53395
func (s *CIService) PutAudioAuditingJob(ctx context.Context, opt *PutAudioAuditingJobOptions) (*PutAudioAuditingJobResult, *Response, error) {
	var res PutAudioAuditingJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/audio/auditing",
		method:  http.MethodPost,
		body:    opt,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// GetAudioAuditingJobResult is the result of GetAudioAuditingJob
type GetAudioAuditingJobResult struct {
	XMLName    xml.Name                `xml:"Response"`
	JobsDetail *AudioAuditingJobDetail `xml:",omitempty"`
	RequestId  string                  `xml:",omitempty"`
}

// AudioAuditingJobDetail is the detail of GetAudioAuditingJobResult
type AudioAuditingJobDetail struct {
	Code          string               `xml:",omitempty"`
	Message       string               `xml:",omitempty"`
	JobId         string               `xml:",omitempty"`
	State         string               `xml:",omitempty"`
	CreationTime  string               `xml:",omitempty"`
	Object        string               `xml:",omitempty"`
	Url           string               `xml:",omitempty"`
	DataId        string               `xml:",omitempty"`
	AudioText     string               `xml:",omitempty"`
	Label         string               `xml:",omitempty"`
	Result        int                  `xml:",omitempty"`
	PornInfo      *RecognitionInfo     `xml:",omitempty"`
	TerrorismInfo *RecognitionInfo     `xml:",omitempty"`
	PoliticsInfo  *RecognitionInfo     `xml:",omitempty"`
	AdsInfo       *RecognitionInfo     `xml:",omitempty"`
	Section       []AudioSectionResult `xml:",omitempty"`
}

// 音频审核-查询任务 https://cloud.tencent.com/document/product/460/53396
func (s *CIService) GetAudioAuditingJob(ctx context.Context, jobid string) (*GetAudioAuditingJobResult, *Response, error) {
	var res GetAudioAuditingJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/audio/auditing/" + jobid,
		method:  http.MethodGet,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// PutTextAuditingJobOptions is the option of PutTextAuditingJob
type PutTextAuditingJobOptions struct {
	XMLName      xml.Name             `xml:"Request"`
	InputObject  string               `xml:"Input>Object,omitempty"`
	InputUrl     string               `xml:"Input>Url,omitempty"`
	InputContent string               `xml:"Input>Content,omitempty"`
	InputDataId  string               `xml:"Input>DataId,omitempty"`
	Conf         *TextAuditingJobConf `xml:"Conf"`
}

// TextAuditingJobConf is the config of PutAudioAuditingJobOptions
type TextAuditingJobConf struct {
	DetectType      string `xml:",omitempty"`
	Callback        string `xml:",omitempty"`
	CallbackVersion string `xml:",omitempty"`
	BizType         string `xml:",omitempty"`
}

// PutTextAuditingJobResult is the result of PutTextAuditingJob
type PutTextAuditingJobResult GetTextAuditingJobResult

// 文本审核-创建任务 https://cloud.tencent.com/document/product/436/56289
func (s *CIService) PutTextAuditingJob(ctx context.Context, opt *PutTextAuditingJobOptions) (*PutTextAuditingJobResult, *Response, error) {
	var res PutTextAuditingJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/text/auditing",
		method:  http.MethodPost,
		body:    opt,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// GetTextAuditingJobResult is the result of GetTextAuditingJob
type GetTextAuditingJobResult struct {
	XMLName    xml.Name               `xml:"Response"`
	JobsDetail *TextAuditingJobDetail `xml:",omitempty"`
	RequestId  string                 `xml:",omitempty"`
}

// TextAuditingJobDetail is the detail of GetTextAuditingJobResult
type TextAuditingJobDetail struct {
	Code          string               `xml:",omitempty"`
	Message       string               `xml:",omitempty"`
	JobId         string               `xml:",omitempty"`
	State         string               `xml:",omitempty"`
	CreationTime  string               `xml:",omitempty"`
	Object        string               `xml:",omitempty"`
	Url           string               `xml:",omitempty"`
	DataId        string               `xml:",omitempty"`
	Content       string               `xml:",omitempty"`
	SectionCount  int                  `xml:",omitempty"`
	Label         string               `xml:",omitempty"`
	Result        int                  `xml:",omitempty"`
	PornInfo      *TextRecognitionInfo `xml:",omitempty"`
	TerrorismInfo *TextRecognitionInfo `xml:",omitempty"`
	PoliticsInfo  *TextRecognitionInfo `xml:",omitempty"`
	AdsInfo       *TextRecognitionInfo `xml:",omitempty"`
	IllegalInfo   *TextRecognitionInfo `xml:",omitempty"`
	AbuseInfo     *TextRecognitionInfo `xml:",omitempty"`
	Section       []TextSectionResult  `xml:",omitempty"`
}

// TextRecognitionInfo
type TextRecognitionInfo struct {
	Code     int    `xml:",omitempty"`
	HitFlag  int    `xml:",omitempty"`
	Score    int    `xml:",omitempty"`
	Count    int    `xml:",omitempty"`
	Keywords string `xml:",omitempty"`
}

// TextSectionResult is the section result of TextAuditingJobDetail
type TextSectionResult struct {
	StartByte     int                  `xml:",omitempty"`
	Label         string               `xml:",omitempty"`
	Result        int                  `xml:",omitempty"`
	PornInfo      *TextRecognitionInfo `xml:",omitempty"`
	TerrorismInfo *TextRecognitionInfo `xml:",omitempty"`
	PoliticsInfo  *TextRecognitionInfo `xml:",omitempty"`
	AdsInfo       *TextRecognitionInfo `xml:",omitempty"`
	IllegalInfo   *TextRecognitionInfo `xml:",omitempty"`
	AbuseInfo     *TextRecognitionInfo `xml:",omitempty"`
}

// 文本审核-查询任务 https://cloud.tencent.com/document/product/436/56288
func (s *CIService) GetTextAuditingJob(ctx context.Context, jobid string) (*GetTextAuditingJobResult, *Response, error) {
	var res GetTextAuditingJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/text/auditing/" + jobid,
		method:  http.MethodGet,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// PutDocumentAuditingJobOptions is the option of PutDocumentAuditingJob
type PutDocumentAuditingJobOptions struct {
	XMLName     xml.Name                 `xml:"Request"`
	InputObject string                   `xml:"Input>Object,omitempty"`
	InputUrl    string                   `xml:"Input>Url,omitempty"`
	InputType   string                   `xml:"Input>Type,omitempty"`
	InputDataId string                   `xml:"Input>DataId,omitempty"`
	Conf        *DocumentAuditingJobConf `xml:"Conf"`
}

// DocumentAuditingJobConf is the config of PutDocumentAuditingJobOptions
type DocumentAuditingJobConf struct {
	DetectType string `xml:",omitempty"`
	Callback   string `xml:",omitempty"`
	BizType    string `xml:",omitempty"`
}

// PutDocumentAuditingJobResult is the result of PutDocumentAuditingJob
type PutDocumentAuditingJobResult PutVideoAuditingJobResult

// 文档审核-创建任务 https://cloud.tencent.com/document/product/436/59381
func (s *CIService) PutDocumentAuditingJob(ctx context.Context, opt *PutDocumentAuditingJobOptions) (*PutDocumentAuditingJobResult, *Response, error) {
	var res PutDocumentAuditingJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/document/auditing",
		method:  http.MethodPost,
		body:    opt,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// GetDocumentAuditingJobResult is the result of GetDocumentAuditingJob
type GetDocumentAuditingJobResult struct {
	XMLName    xml.Name                   `xml:"Response"`
	JobsDetail *DocumentAuditingJobDetail `xml:",omitempty"`
	RequestId  string                     `xml:",omitempty"`
}

// DocumentAuditingJobDetail is the detail of GetDocumentAuditingJobResult
type DocumentAuditingJobDetail struct {
	Code         string                   `xml:",omitempty"`
	Message      string                   `xml:",omitempty"`
	JobId        string                   `xml:",omitempty"`
	State        string                   `xml:",omitempty"`
	CreationTime string                   `xml:",omitempty"`
	Object       string                   `xml:",omitempty"`
	Url          string                   `xml:",omitempty"`
	DataId       string                   `xml:",omitempty"`
	PageCount    int                      `xml:",omitempty"`
	Label        string                   `xml:",omitempty"`
	Suggestion   int                      `xml:",omitempty"`
	Labels       *DocumentResultInfo      `xml:",omitempty"`
	PageSegment  *DocumentPageSegmentInfo `xml:",omitempty"`
}

// DocumentResultInfo
type DocumentResultInfo struct {
	PornInfo      *RecognitionInfo `xml:",omitempty"`
	TerrorismInfo *RecognitionInfo `xml:",omitempty"`
	PoliticsInfo  *RecognitionInfo `xml:",omitempty"`
	AdsInfo       *RecognitionInfo `xml:",omitempty"`
}

// DocumentPageSegmentInfo
type DocumentPageSegmentInfo struct {
	Results []DocumentPageSegmentResultResult `xml:",omitempty"`
}

// DocumentPageSegmentResultResult
type DocumentPageSegmentResultResult struct {
	Url           string           `xml:",omitempty"`
	Text          string           `xml:",omitempty"`
	PageNumber    int              `xml:",omitempty"`
	SheetNumber   int              `xml:",omitempty"`
	Label         string           `xml:",omitempty"`
	Suggestion    int              `xml:",omitempty"`
	PornInfo      *RecognitionInfo `xml:",omitempty"`
	TerrorismInfo *RecognitionInfo `xml:",omitempty"`
	PoliticsInfo  *RecognitionInfo `xml:",omitempty"`
	AdsInfo       *RecognitionInfo `xml:",omitempty"`
}

// OcrResult
type OcrResult struct {
	Text     string    `xml:"Text,omitempty"`
	Keywords []string  `xml:"Keywords,omitempty"`
	Location *Location `xml:"Location,omitempty"`
}

// ObjectResult
type ObjectResult struct {
	Name     string    `xml:"Name,omitempty"`
	Location *Location `xml:"Location,omitempty"`
}

// LibResult
type LibResult struct {
	ImageId string `xml:"ImageId"`
	Score   uint32 `xml:"Score"`
}

// Location
type Location struct {
	X      float64 `xml:"X,omitempty"`      // 左上角横坐标
	Y      float64 `xml:"Y,omitempty"`      // 左上角纵坐标
	Width  float64 `xml:"Width,omitempty"`  // 宽度
	Height float64 `xml:"Height,omitempty"` // 高度
	Rotate float64 `xml:"Rotate,omitempty"` // 检测框的旋转角度
}

// 文档审核-查询任务 https://cloud.tencent.com/document/product/436/59382
func (s *CIService) GetDocumentAuditingJob(ctx context.Context, jobid string) (*GetDocumentAuditingJobResult, *Response, error) {
	var res GetDocumentAuditingJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/document/auditing/" + jobid,
		method:  http.MethodGet,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// PutWebpageAuditingJobOptions is the option of PutWebpageAuditingJob
type PutWebpageAuditingJobOptions struct {
	XMLName     xml.Name                `xml:"Request"`
	InputUrl    string                  `xml:"Input>Url,omitempty"`
	InputDataId string                  `xml:"Input>DataId,omitempty"`
	Conf        *WebpageAuditingJobConf `xml:"Conf"`
}

// WebpageAuditingJobConf is the config of PutWebpageAuditingJobOptions
type WebpageAuditingJobConf struct {
	DetectType          string `xml:",omitempty"`
	Callback            string `xml:",omitempty"`
	ReturnHighlightHtml bool   `xml:",omitempty"`
}

// PutWebpageAuditingJobResult is the result of PutWebpageAuditingJob
type PutWebpageAuditingJobResult PutVideoAuditingJobResult

// 网页审核-创建任务 https://cloud.tencent.com/document/product/436/63958
func (s *CIService) PutWebpageAuditingJob(ctx context.Context, opt *PutWebpageAuditingJobOptions) (*PutWebpageAuditingJobResult, *Response, error) {
	var res PutWebpageAuditingJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/webpage/auditing",
		method:  http.MethodPost,
		body:    opt,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// GetWebpageAuditingJobResult is the result of GetWebpageAuditingJob
type GetWebpageAuditingJobResult struct {
	XMLName    xml.Name                  `xml:"Response"`
	JobsDetail *WebpageAuditingJobDetail `xml:",omitempty"`
}

// WebpageAuditingJobDetail is the detail of GetWebpageAuditingJobResult
type WebpageAuditingJobDetail struct {
	Code          string               `xml:",omitempty"`
	Message       string               `xml:",omitempty"`
	JobId         string               `xml:",omitempty"`
	State         string               `xml:",omitempty"`
	CreationTime  string               `xml:",omitempty"`
	Url           string               `xml:",omitempty"`
	Labels        *WebpageResultInfo   `xml:",omitempty"`
	PageCount     int                  `xml:",omitempty"`
	Suggestion    int                  `xml:",omitempty"`
	ImageResults  *WebpageImageResults `xml:",omitempty"`
	TextResults   *WebpageTextResults  `xml:",omitempty"`
	HighlightHtml string               `xml:",omitempty"`
	DataId        string               `xml:",omitempty"`
}

// WebpageResultInfo
type WebpageResultInfo struct {
	PornInfo      *RecognitionInfo `xml:",omitempty"`
	TerrorismInfo *RecognitionInfo `xml:",omitempty"`
	PoliticsInfo  *RecognitionInfo `xml:",omitempty"`
	AdsInfo       *RecognitionInfo `xml:",omitempty"`
}

// WebpageImageResults
type WebpageImageResults struct {
	Results []WebpageImageResult `xml:",omitempty"`
}

// WebpageImageResult
type WebpageImageResult struct {
	Url           string           `xml:",omitempty"`
	Text          string           `xml:",omitempty"`
	Label         string           `xml:",omitempty"`
	PageNumber    int              `xml:",omitempty"`
	SheetNumber   int              `xml:",omitempty"`
	Suggestion    int              `xml:",omitempty"`
	PornInfo      *RecognitionInfo `xml:",omitempty"`
	TerrorismInfo *RecognitionInfo `xml:",omitempty"`
	PoliticsInfo  *RecognitionInfo `xml:",omitempty"`
	AdsInfo       *RecognitionInfo `xml:",omitempty"`
}

// WebpageTextResults
type WebpageTextResults struct {
	Results []WebpageTextResult `xml:",omitempty"`
}

// WebpageTextResult
type WebpageTextResult struct {
	Text          string               `xml:",omitempty"`
	Label         string               `xml:",omitempty"`
	Result        int                  `xml:",omitempty"`
	PageNumber    int                  `xml:",omitempty"`
	SheetNumber   int                  `xml:",omitempty"`
	Suggestion    int                  `xml:",omitempty"`
	PornInfo      *TextRecognitionInfo `xml:",omitempty"`
	TerrorismInfo *TextRecognitionInfo `xml:",omitempty"`
	PoliticsInfo  *TextRecognitionInfo `xml:",omitempty"`
	AdsInfo       *TextRecognitionInfo `xml:",omitempty"`
	IllegalInfo   *TextRecognitionInfo `xml:",omitempty"`
	AbuseInfo     *TextRecognitionInfo `xml:",omitempty"`
}

// 网页审核-查询任务 https://cloud.tencent.com/document/product/436/63959
func (s *CIService) GetWebpageAuditingJob(ctx context.Context, jobid string) (*GetWebpageAuditingJobResult, *Response, error) {
	var res GetWebpageAuditingJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/webpage/auditing/" + jobid,
		method:  http.MethodGet,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// PutVirusDetectJobOptions is the option of PutVirusDetectJob
type PutVirusDetectJobOptions struct {
	XMLName     xml.Name            `xml:"Request"`
	InputObject string              `xml:"Input>Object,omitempty"`
	InputUrl    string              `xml:"Input>Url,omitempty"`
	Conf        *VirusDetectJobConf `xml:"Conf"`
}

// VirusDetectJobConf is the config of PutVirusDetectJobOptions
type VirusDetectJobConf struct {
	DetectType string `xml:",omitempty"`
	Callback   string `xml:",omitempty"`
}

// PutVirusDetectJobResult is the result of PutVirusDetectJob
type PutVirusDetectJobResult PutVideoAuditingJobResult

// 云查毒接口-提交病毒检测任务 https://cloud.tencent.com/document/product/436/63961
func (s *CIService) PutVirusDetectJob(ctx context.Context, opt *PutVirusDetectJobOptions) (*PutVirusDetectJobResult, *Response, error) {
	var res PutVirusDetectJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/virus/detect",
		method:  http.MethodPost,
		body:    opt,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// GetVirusDetectJobResult is the result of GetVirusDetectJob
type GetVirusDetectJobResult struct {
	XMLName    xml.Name              `xml:"Response"`
	JobsDetail *VirusDetectJobDetail `xml:",omitempty"`
	RequestId  string                `xml:",omitempty"`
}

// VirusDetectJobDetail is the detail of GetVirusDetectJobResult
type VirusDetectJobDetail struct {
	Code         string        `xml:",omitempty"`
	Message      string        `xml:",omitempty"`
	JobId        string        `xml:",omitempty"`
	State        string        `xml:",omitempty"`
	CreationTime string        `xml:",omitempty"`
	Object       string        `xml:",omitempty"`
	Url          string        `xml:",omitempty"`
	Suggestion   string        `xml:",omitempty"`
	DetectDetail *VirusResults `xml:",omitempty"`
}

// VirusResults
type VirusResults struct {
	Result []VirusInfo `xml:",omitempty"`
}

// VirusInfo
type VirusInfo struct {
	FileName  string `xml:",omitempty"`
	VirusName string `xml:",omitempty"`
}

// 云查毒接口-查询病毒检测任务结果 https://cloud.tencent.com/document/product/436/63962
func (s *CIService) GetVirusDetectJob(ctx context.Context, jobid string) (*GetVirusDetectJobResult, *Response, error) {
	var res GetVirusDetectJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/virus/detect/" + jobid,
		method:  http.MethodGet,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// 图片持久化处理-上传时处理 https://cloud.tencent.com/document/product/460/18147
// 盲水印-上传时添加 https://cloud.tencent.com/document/product/460/19017
// 二维码识别-上传时识别 https://cloud.tencent.com/document/product/460/37513
func (s *CIService) Put(ctx context.Context, name string, r io.Reader, uopt *ObjectPutOptions) (*ImageProcessResult, *Response, error) {
	if r == nil {
		return nil, nil, fmt.Errorf("reader is nil")
	}
	if err := CheckReaderLen(r); err != nil {
		return nil, nil, err
	}
	opt := CloneObjectPutOptions(uopt)
	totalBytes, err := GetReaderLen(r)
	if err != nil && opt != nil && opt.Listener != nil {
		if opt.ContentLength == 0 {
			return nil, nil, err
		}
		totalBytes = opt.ContentLength
	}
	if err == nil {
		// 与 go http 保持一致, 非bytes.Buffer/bytes.Reader/strings.Reader由用户指定ContentLength, 或使用 Chunk 上传
		if opt != nil && opt.ContentLength == 0 && IsLenReader(r) {
			opt.ContentLength = totalBytes
		}
	}
	reader := TeeReader(r, nil, totalBytes, nil)
	if s.client.Conf.EnableCRC {
		reader.writer = crc64.New(crc64.MakeTable(crc64.ECMA))
	}
	if opt != nil && opt.Listener != nil {
		reader.listener = opt.Listener
	}

	var res ImageProcessResult
	sendOpt := sendOptions{
		baseURL:   s.client.BaseURL.BucketURL,
		uri:       "/" + encodeURIComponent(name),
		method:    http.MethodPut,
		body:      reader,
		optHeader: opt,
		result:    &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)

	return &res, resp, err
}

// ci put object from local file
func (s *CIService) PutFromFile(ctx context.Context, name string, filePath string, opt *ObjectPutOptions) (*ImageProcessResult, *Response, error) {
	fd, err := os.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	defer fd.Close()

	return s.Put(ctx, name, fd, opt)
}

// 基本图片处理 https://cloud.tencent.com/document/product/460/36540
// 盲水印-下载时添加 https://cloud.tencent.com/document/product/460/19017
func (s *CIService) Get(ctx context.Context, name string, operation string, opt *ObjectGetOptions, id ...string) (*Response, error) {
	var u string
	if len(id) == 1 {
		u = fmt.Sprintf("/%s?versionId=%s&%s", encodeURIComponent(name), id[0], encodeURIComponent(operation))
	} else if len(id) == 0 {
		u = fmt.Sprintf("/%s?%s", encodeURIComponent(name), encodeURIComponent(operation))
	} else {
		return nil, errors.New("wrong params")
	}

	sendOpt := sendOptions{
		baseURL:          s.client.BaseURL.BucketURL,
		uri:              u,
		method:           http.MethodGet,
		optQuery:         opt,
		optHeader:        opt,
		disableCloseBody: true,
	}
	resp, err := s.client.send(ctx, &sendOpt)

	if opt != nil && opt.Listener != nil {
		if err == nil && resp != nil {
			if totalBytes, e := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64); e == nil {
				resp.Body = TeeReader(resp.Body, nil, totalBytes, opt.Listener)
			}
		}
	}
	return resp, err
}

func (s *CIService) GetToFile(ctx context.Context, name, localpath, operation string, opt *ObjectGetOptions, id ...string) (*Response, error) {
	resp, err := s.Get(ctx, name, operation, opt, id...)
	if err != nil {
		return resp, err
	}
	defer resp.Body.Close()

	// If file exist, overwrite it
	fd, err := os.OpenFile(localpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660)
	if err != nil {
		return resp, err
	}

	_, err = io.Copy(fd, resp.Body)
	fd.Close()
	if err != nil {
		return resp, err
	}

	return resp, nil
}

type GetQRcodeResult struct {
	XMLName     xml.Name    `xml:"Response"`
	CodeStatus  int         `xml:"CodeStatus,omitempty"`
	QRcodeInfo  *QRcodeInfo `xml:"QRcodeInfo,omitempty"`
	ResultImage string      `xml:"ResultImage,omitempty"`
}

// 二维码识别-下载时识别 https://cloud.tencent.com/document/product/436/54070
func (s *CIService) GetQRcode(ctx context.Context, name string, cover int, opt *ObjectGetOptions, id ...string) (*GetQRcodeResult, *Response, error) {
	var u string
	if len(id) == 1 {
		u = fmt.Sprintf("/%s?versionId=%s&ci-process=QRcode&cover=%v", encodeURIComponent(name), id[0], cover)
	} else if len(id) == 0 {
		u = fmt.Sprintf("/%s?ci-process=QRcode&cover=%v", encodeURIComponent(name), cover)
	} else {
		return nil, nil, errors.New("wrong params")
	}

	var res GetQRcodeResult
	sendOpt := sendOptions{
		baseURL:   s.client.BaseURL.BucketURL,
		uri:       u,
		method:    http.MethodGet,
		optQuery:  opt,
		optHeader: opt,
		result:    &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

type GenerateQRcodeOptions struct {
	QRcodeContent string `url:"qrcode-content,omitempty"`
	Mode          int    `url:"mode,omitempty"`
	Width         int    `url:"width,omitempty"`
}
type GenerateQRcodeResult struct {
	XMLName     xml.Name `xml:"Response"`
	ResultImage string   `xml:"ResultImage,omitempty"`
}

// 二维码生成 https://cloud.tencent.com/document/product/436/54071
func (s *CIService) GenerateQRcode(ctx context.Context, opt *GenerateQRcodeOptions) (*GenerateQRcodeResult, *Response, error) {
	var res GenerateQRcodeResult
	sendOpt := &sendOptions{
		baseURL:  s.client.BaseURL.BucketURL,
		uri:      "/?ci-process=qrcode-generate",
		method:   http.MethodGet,
		optQuery: opt,
		result:   &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

func (s *CIService) GenerateQRcodeToFile(ctx context.Context, filePath string, opt *GenerateQRcodeOptions) (*GenerateQRcodeResult, *Response, error) {
	res, resp, err := s.GenerateQRcode(ctx, opt)
	if err != nil {
		return res, resp, err
	}

	// If file exist, overwrite it
	fd, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660)
	if err != nil {
		return res, resp, err
	}
	defer fd.Close()

	bs, err := base64.StdEncoding.DecodeString(res.ResultImage)
	if err != nil {
		return res, resp, err
	}
	fb := bytes.NewReader(bs)
	_, err = io.Copy(fd, fb)

	return res, resp, err
}

// 开通 Guetzli 压缩 https://cloud.tencent.com/document/product/460/30112
func (s *CIService) PutGuetzli(ctx context.Context) (*Response, error) {
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/?guetzli",
		method:  http.MethodPut,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return resp, err
}

type GetGuetzliResult struct {
	XMLName       xml.Name `xml:"GuetzliStatus"`
	GuetzliStatus string   `xml:",chardata"`
}

// 查询 Guetzli 状态 https://cloud.tencent.com/document/product/460/30111
func (s *CIService) GetGuetzli(ctx context.Context) (*GetGuetzliResult, *Response, error) {
	var res GetGuetzliResult
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/?guetzli",
		method:  http.MethodGet,
		result:  &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

// 关闭 Guetzli 压缩 https://cloud.tencent.com/document/product/460/30113
func (s *CIService) DeleteGuetzli(ctx context.Context) (*Response, error) {
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/?guetzli",
		method:  http.MethodDelete,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return resp, err
}
