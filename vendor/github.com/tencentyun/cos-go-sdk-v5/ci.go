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
	"net/url"
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
	XMLName        xml.Name           `xml:"UploadResult"`
	OriginalInfo   *PicOriginalInfo   `xml:"OriginalInfo,omitempty"`
	ProcessResults []PicProcessObject `xml:"ProcessResults>Object,omitempty"`
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
	DataId           string `url:"dataid,omitempty"`
	Async            int    `url:"async,omitempty"`
	Callback         string `url:"callback,omitempty"`
}

type UserListInfo struct {
	ListResults []UserListResults `xml:",omitempty"`
}

//UserListResults 命中账号黑白名单信息
type UserListResults struct {
	ListType *int   `xml:",omitempty"`
	ListName string `xml:",omitempty"`
	Entity   string `xml:",omitempty"`
}

// ImageRecognitionResult is the result of ImageRecognition/ImageAuditing
type ImageRecognitionResult struct {
	XMLName           xml.Name         `xml:"RecognitionResult"`
	JobId             string           `xml:"JobId,omitempty"`
	State             string           `xml:"State,omitempty"`
	Object            string           `xml:"Object,omitempty"`
	Url               string           `xml:"Url,omitempty"`
	Text              string           `xml:"Text,omitempty"`
	Label             string           `xml:"Label,omitempty"`
	Result            int              `xml:"Result,omitempty"`
	Score             int              `xml:"Score,omitempty"`
	Category          string           `xml:"Category,omitempty"`
	SubLabel          string           `xml:"SubLabel,omitempty"`
	PornInfo          *RecognitionInfo `xml:"PornInfo,omitempty"`
	TerroristInfo     *RecognitionInfo `xml:"TerroristInfo,omitempty"`
	PoliticsInfo      *RecognitionInfo `xml:"PoliticsInfo,omitempty"`
	AdsInfo           *RecognitionInfo `xml:"AdsInfo,omitempty"`
	TeenagerInfo      *RecognitionInfo `xml:"TeenagerInfo,omitempty"`
	TerrorismInfo     *RecognitionInfo `xml:"TerrorismInfo,omitempty"`
	CompressionResult int              `xml:"CompressionResult,omitempty"`
	DataId            string           `xml:"DataId,omitempty"`
}

// RecognitionInfo is the result of auditing scene
type RecognitionInfo struct {
	Code               int              `xml:"Code,omitempty"`
	Msg                string           `xml:"Msg,omitempty"`
	HitFlag            int              `xml:"HitFlag,omitempty"`
	Score              int              `xml:"Score,omitempty"`
	Label              string           `xml:"Label,omitempty"`
	Count              int              `xml:"Count,omitempty"`
	Category           string           `xml:"Category,omitempty"`
	SubLabel           string           `xml:"SubLabel,omitempty"`
	Keywords           []string         `xml:"Keywords,omitempty"`
	OcrResults         []OcrResult      `xml:"OcrResults,omitempty"`
	ObjectResults      []ObjectResult   `xml:"ObjectResults,omitempty"`
	LibResults         []LibResult      `xml:"LibResults,omitempty"`
	SpeakerResults     []LanguageResult `xml:"SpeakerResults,omitempty"`
	RecognitionResults []LanguageResult `xml:"RecognitionResults,omitempty"`
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

// UserExtraInfo is user defined information
type UserExtraInfo struct {
	TokenId        string `xml:",omitempty"`
	Nickname       string `xml:",omitempty"`
	DeviceId       string `xml:",omitempty"`
	AppId          string `xml:",omitempty"`
	Room           string `xml:",omitempty"`
	IP             string `xml:",omitempty"`
	Type           string `xml:",omitempty"`
	ReceiveTokenId string `xml:",omitempty"`
	Gender         string `xml:",omitempty"`
	Level          string `xml:",omitempty"`
	Role           string `xml:",omitempty"`
}

// ImageAuditingInputOptions is the option of BatchImageAuditingOptions
type ImageAuditingInputOptions struct {
	DataId           string         `xml:",omitempty"`
	Object           string         `xml:",omitempty"`
	Url              string         `xml:",omitempty"`
	Content          string         `xml:",omitempty"`
	Interval         int            `xml:",omitempty"`
	MaxFrames        int            `xml:",omitempty"`
	LargeImageDetect int            `xml:",omitempty"`
	UserInfo         *UserExtraInfo `xml:",omitempty"`
}

// ImageAuditingJobConf is the config of BatchImageAuditingOptions
type ImageAuditingJobConf struct {
	DetectType string `xml:",omitempty"`
	BizType    string `xml:",omitempty"`
	Async      int    `xml:",omitempty"`
	Callback   string `xml:",omitempty"`
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
	JobId             string           `xml:",omitempty"`
	State             string           `xml:",omitempty"`
	DataId            string           `xml:",omitempty"`
	Object            string           `xml:",omitempty"`
	Url               string           `xml:",omitempty"`
	Text              string           `xml:",omitempty"`
	Label             string           `xml:",omitempty"`
	Result            int              `xml:",omitempty"`
	Score             int              `xml:",omitempty"`
	Category          string           `xml:",omitempty"`
	SubLabel          string           `xml:",omitempty"`
	PornInfo          *RecognitionInfo `xml:",omitempty"`
	TerrorismInfo     *RecognitionInfo `xml:",omitempty"`
	PoliticsInfo      *RecognitionInfo `xml:",omitempty"`
	AdsInfo           *RecognitionInfo `xml:",omitempty"`
	TeenagerInfo      *RecognitionInfo `xml:",omitempty"`
	CompressionResult int              `xml:",omitempty"`
	UserInfo          *UserExtraInfo   `xml:",omitempty"`
	ListInfo          *UserListInfo    `xml:",omitempty"`
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
	XMLName       xml.Name              `xml:"Request"`
	InputObject   string                `xml:"Input>Object,omitempty"`
	InputUrl      string                `xml:"Input>Url,omitempty"`
	InputDataId   string                `xml:"Input>DataId,omitempty"`
	InputUserInfo *UserExtraInfo        `xml:"Input>UserInfo,omitempty"`
	Conf          *VideoAuditingJobConf `xml:"Conf"`
	Type          string                `xml:"Type,omitempty"`
}

// VideoAuditingJobConf is the config of PutVideoAuditingJobOptions
type VideoAuditingJobConf struct {
	DetectType      string                       `xml:",omitempty"`
	Snapshot        *PutVideoAuditingJobSnapshot `xml:",omitempty"`
	Callback        string                       `xml:",omitempty"`
	CallbackVersion string                       `xml:",omitempty"`
	CallbackType    int                          `xml:",omitempty"`
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
	UserInfo      *UserExtraInfo                `xml:",omitempty"`
	Type          string                        `xml:",omitempty"`
	ListInfo      *UserListInfo                 `xml:",omitempty"`
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
	Url             string           `xml:",omitempty"`
	Text            string           `xml:",omitempty"`
	OffsetTime      int              `xml:",omitempty"`
	Duration        int              `xml:",omitempty"`
	Label           string           `xml:",omitempty"`
	Result          int              `xml:",omitempty"`
	PornInfo        *RecognitionInfo `xml:",omitempty"`
	TerrorismInfo   *RecognitionInfo `xml:",omitempty"`
	PoliticsInfo    *RecognitionInfo `xml:",omitempty"`
	AdsInfo         *RecognitionInfo `xml:",omitempty"`
	TeenagerInfo    *RecognitionInfo `xml:",omitempty"`
	LanguageResults []LanguageResult `xml:",omitempty"`
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

// 视频审核-取消直播流审核任务
func (s *CIService) PostVideoAuditingCancelJob(ctx context.Context, jobid string) (*PutVideoAuditingJobResult, *Response, error) {
	var res PutVideoAuditingJobResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/video/cancel_auditing" + jobid,
		method:  http.MethodPost,
		result:  &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return &res, resp, err
}

// PutAudioAuditingJobOptions is the option of PutAudioAuditingJob
type PutAudioAuditingJobOptions struct {
	XMLName       xml.Name              `xml:"Request"`
	InputObject   string                `xml:"Input>Object,omitempty"`
	InputUrl      string                `xml:"Input>Url,omitempty"`
	InputDataId   string                `xml:"Input>DataId,omitempty"`
	InputUserInfo *UserExtraInfo        `xml:"Input>UserInfo,omitempty"`
	Conf          *AudioAuditingJobConf `xml:"Conf"`
}

// AudioAuditingJobConf is the config of PutAudioAuditingJobOptions
type AudioAuditingJobConf struct {
	DetectType      string `xml:",omitempty"`
	Callback        string `xml:",omitempty"`
	CallbackVersion string `xml:",omitempty"`
	CallbackType    int    `xml:",omitempty"`
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
	Code            string               `xml:",omitempty"`
	Message         string               `xml:",omitempty"`
	JobId           string               `xml:",omitempty"`
	State           string               `xml:",omitempty"`
	CreationTime    string               `xml:",omitempty"`
	Object          string               `xml:",omitempty"`
	Url             string               `xml:",omitempty"`
	DataId          string               `xml:",omitempty"`
	AudioText       string               `xml:",omitempty"`
	Label           string               `xml:",omitempty"`
	Result          int                  `xml:",omitempty"`
	PornInfo        *RecognitionInfo     `xml:",omitempty"`
	TerrorismInfo   *RecognitionInfo     `xml:",omitempty"`
	PoliticsInfo    *RecognitionInfo     `xml:",omitempty"`
	AdsInfo         *RecognitionInfo     `xml:",omitempty"`
	TeenagerInfo    *RecognitionInfo     `xml:",omitempty"`
	LanguageResults []LanguageResult     `xml:",omitempty"`
	Section         []AudioSectionResult `xml:",omitempty"`
	UserInfo        *UserExtraInfo       `xml:",omitempty"`
	ListInfo        *UserListInfo        `xml:",omitempty"`
}

// LanguageResult 语种识别结果
type LanguageResult struct {
	Label     string `xml:"Label"`
	Score     uint32 `xml:"Score"`
	StartTime *int64 `xml:"StartTime,omitempty"`
	EndTime   *int64 `xml:"EndTime,omitempty"`
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
	XMLName       xml.Name             `xml:"Request"`
	InputObject   string               `xml:"Input>Object,omitempty"`
	InputUrl      string               `xml:"Input>Url,omitempty"`
	InputContent  string               `xml:"Input>Content,omitempty"`
	InputDataId   string               `xml:"Input>DataId,omitempty"`
	InputUserInfo *UserExtraInfo       `xml:"Input>UserInfo,omitempty"`
	Conf          *TextAuditingJobConf `xml:"Conf"`
}

// TextAuditingJobConf is the config of PutAudioAuditingJobOptions
type TextAuditingJobConf struct {
	DetectType      string `xml:",omitempty"`
	Callback        string `xml:",omitempty"`
	CallbackVersion string `xml:",omitempty"`
	BizType         string `xml:",omitempty"`
	CallbackType    int    `xml:",omitempty"`
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
	UserInfo      *UserExtraInfo       `xml:",omitempty"`
	ListInfo      *UserListInfo        `xml:",omitempty"`
}

// TextLibResult
type TextLibResult struct {
	LibType  int32    `xml:"LibType,omitempty"`
	LibName  string   `xml:"LibName,omitempty"`
	Keywords []string `xml:"Keywords,omitempty"`
}

// TextRecognitionInfo
type TextRecognitionInfo struct {
	Code       int             `xml:",omitempty"`
	HitFlag    int             `xml:",omitempty"`
	Score      int             `xml:",omitempty"`
	Count      int             `xml:",omitempty"`
	Keywords   string          `xml:",omitempty"`
	LibResults []TextLibResult `xml:",omitempty"`
	SubLabel   string          `xml:",omitempty"`
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
	XMLName       xml.Name                 `xml:"Request"`
	InputObject   string                   `xml:"Input>Object,omitempty"`
	InputUrl      string                   `xml:"Input>Url,omitempty"`
	InputType     string                   `xml:"Input>Type,omitempty"`
	InputDataId   string                   `xml:"Input>DataId,omitempty"`
	InputUserInfo *UserExtraInfo           `xml:"Input>UserInfo,omitempty"`
	Conf          *DocumentAuditingJobConf `xml:"Conf"`
}

// DocumentAuditingJobConf is the config of PutDocumentAuditingJobOptions
type DocumentAuditingJobConf struct {
	DetectType   string `xml:",omitempty"`
	Callback     string `xml:",omitempty"`
	BizType      string `xml:",omitempty"`
	CallbackType int    `xml:",omitempty"`
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
	UserInfo     *UserExtraInfo           `xml:",omitempty"`
	ListInfo     *UserListInfo            `xml:",omitempty"`
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
	ImageId string `xml:"ImageId,omitempty"`
	Score   uint32 `xml:"Score,omitempty"`
	TextLibResult
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
	XMLName       xml.Name                `xml:"Request"`
	InputUrl      string                  `xml:"Input>Url,omitempty"`
	InputDataId   string                  `xml:"Input>DataId,omitempty"`
	InputUserInfo *UserExtraInfo          `xml:"Input>UserInfo,omitempty"`
	Conf          *WebpageAuditingJobConf `xml:"Conf"`
}

// WebpageAuditingJobConf is the config of PutWebpageAuditingJobOptions
type WebpageAuditingJobConf struct {
	DetectType          string `xml:",omitempty"`
	Callback            string `xml:",omitempty"`
	ReturnHighlightHtml bool   `xml:",omitempty"`
	BizType             string `xml:",omitempty"`
	CallbackType        int    `xml:",omitempty"`
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
	UserInfo      *UserExtraInfo       `xml:",omitempty"`
	ListInfo      *UserListInfo        `xml:",omitempty"`
	Label         string               `xml:",omitempty"`
}

// WebpageResultInfo
type WebpageResultInfo struct {
	PornInfo      *RecognitionInfo `xml:",omitempty"`
	TerrorismInfo *RecognitionInfo `xml:",omitempty"`
	PoliticsInfo  *RecognitionInfo `xml:",omitempty"`
	AdsInfo       *RecognitionInfo `xml:",omitempty"`
	IllegalInfo   *RecognitionInfo `xml:",omitempty"`
	AbuseInfo     *RecognitionInfo `xml:",omitempty"`
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

// ReportBadcaseOptions
type ReportBadcaseOptions struct {
	XMLName        xml.Name `xml:"Request"`
	ContentType    int      `xml:",omitempty"`
	Text           string   `xml:",omitempty"`
	Url            string   `xml:",omitempty"`
	Label          string   `xml:",omitempty"`
	SuggestedLabel string   `xml:",omitempty"`
	JobId          string   `xml:",omitempty"`
	ModerationTime string   `xml:",omitempty"`
}

// ReportBadcaseResult
type ReportBadcaseResult struct {
	XMLName   xml.Name `xml:"Response"`
	RequestId string   `xml:",omitempty"`
}

// 提交Badcase
func (s *CIService) ReportBadcase(ctx context.Context, opt *ReportBadcaseOptions) (*ReportBadcaseResult, *Response, error) {
	var res ReportBadcaseResult
	sendOpt := sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		uri:     "/report/badcase",
		method:  http.MethodPost,
		body:    opt,
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
		// if opt != nil && opt.ContentLength == 0 && IsLenReader(r) {
		// 	opt.ContentLength = totalBytes
		// }
		// lilang : 2022-07-04
		// 图片cgi不设置content-length的话，读不到body。图片处理cgi暂时不支持chunked，后面会修复。
		if opt != nil && opt.ContentLength == 0 {
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

type AddStyleOptions struct {
	XMLName   xml.Name `xml:"AddStyle"`
	StyleName string   `xml:"StyleName,omitempty"`
	StyleBody string   `xml:"StyleBody,omitempty"`
}

type GetStyleOptions struct {
	XMLName   xml.Name `xml:"GetStyle"`
	StyleName string   `xml:"StyleName,omitempty"`
}

type GetStyleResult struct {
	XMLName   xml.Name    `xml:"StyleList"`
	StyleRule []StyleRule `xml:"StyleRule,omitempty"`
}

type StyleRule struct {
	StyleName string `xml:"StyleName,omitempty"`
	StyleBody string `xml:"StyleBody,omitempty"`
}

type DeleteStyleOptions struct {
	XMLName   xml.Name `xml:"DeleteStyle"`
	StyleName string   `xml:"StyleName,omitempty"`
}

func (s *CIService) AddStyle(ctx context.Context, opt *AddStyleOptions) (*Response, error) {
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		method:  http.MethodPut,
		uri:     "/?style",
		body:    opt,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return resp, err
}

func (s *CIService) GetStyle(ctx context.Context, opt *GetStyleOptions) (*GetStyleResult, *Response, error) {
	var res GetStyleResult
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		method:  http.MethodGet,
		uri:     "/?style",
		body:    opt,
		result:  &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

func (s *CIService) DeleteStyle(ctx context.Context, opt *DeleteStyleOptions) (*Response, error) {
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		method:  http.MethodDelete,
		uri:     "/?style",
		body:    opt,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return resp, err
}

type ImageQualityResult struct {
	XMLName        xml.Name `xml:"Response"`
	LongImage      bool     `xml:"LongImage,omitempty"`
	BlackAndWhite  bool     `xml:"BlackAndWhite,omitempty"`
	SmallImage     bool     `xml:"SmallImage,omitempty"`
	BigImage       bool     `xml:"BigImage,omitempty"`
	PureImage      bool     `xml:"PureImage,omitempty"`
	ClarityScore   int      `xml:"ClarityScore,omitempty"`
	AestheticScore int      `xml:"AestheticScore,omitempty"`
	RequestId      string   `xml:"RequestId,omitempty"`
}

// ImageQuality 图片质量评估
func (s *CIService) ImageQuality(ctx context.Context, obj string) (*ImageQualityResult, *Response, error) {
	var res ImageQualityResult
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.BucketURL,
		uri:     "/" + encodeURIComponent(obj) + "?ci-process=AssessQuality",
		method:  http.MethodGet,
		result:  &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

type OcrRecognitionOptions struct {
	Type              string `url:"type,omitempty"`
	LanguageType      string `url:"language-type,omitempty"`
	Ispdf             bool   `url:"ispdf,omitempty"`
	PdfPageNumber     int    `url:"pdf-pagenumber,omitempty"`
	Isword            bool   `url:"isword,omitempty"`
	EnableWordPolygon bool   `url:"enable-word-polygon,omitempty"`
}

type OcrRecognitionResult struct {
	XMLName        xml.Name         `xml:"Response"`
	TextDetections []TextDetections `xml:"TextDetections,omitempty"`
	Language       string           `xml:"Language,omitempty"`
	Angel          float64          `xml:"Angel,omitempty"`
	PdfPageSize    int              `xml:"PdfPageSize,omitempty"`
	RequestId      string           `xml:"RequestId,omitempty"`
}

type TextDetections struct {
	DetectedText string        `xml:"DetectedText,omitempty"`
	Confidence   int           `xml:"Confidence,omitempty"`
	Polygon      []Polygon     `xml:"Polygon,omitempty"`
	ItemPolygon  []ItemPolygon `xml:"ItemPolygon,omitempty"`
	Words        []Words       `xml:"Words,omitempty"`
	WordPolygon  []WordPolygon `xml:"WordPolygon,omitempty"`
}

type Polygon struct {
	X int `xml:"X,omitempty"`
	Y int `xml:"Y,omitempty"`
}

// ItemPolygon TODO
type ItemPolygon struct {
	X      int `xml:"X,omitempty"`
	Y      int `xml:"Y,omitempty"`
	Width  int `xml:"Width,omitempty"`
	Height int `xml:"Height,omitempty"`
}

type Words struct {
	Confidence     int             `xml:"Confidence,omitempty"`
	Character      string          `xml:"Character,omitempty"`
	WordCoordPoint *WordCoordPoint `xml:"WordCoordPoint,omitempty"`
}

type WordCoordPoint struct {
	WordCoordinate []Polygon `xml:"WordCoordinate,omitempty"`
}

type WordPolygon struct {
	LeftTop     *Polygon `xml:"LeftTop,omitempty"`
	RightTop    *Polygon `xml:"RightTop,omitempty"`
	RightBottom *Polygon `xml:"RightBottom,omitempty"`
	LeftBottom  *Polygon `xml:"LeftBottom,omitempty"`
}

// OcrRecognition OCR通用文字识别
func (s *CIService) OcrRecognition(ctx context.Context, obj string, opt *OcrRecognitionOptions) (*OcrRecognitionResult, *Response, error) {
	var res OcrRecognitionResult
	sendOpt := &sendOptions{
		baseURL:  s.client.BaseURL.BucketURL,
		uri:      "/" + encodeURIComponent(obj) + "?ci-process=OCR",
		method:   http.MethodGet,
		optQuery: opt,
		result:   &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

type DetectCarResult struct {
	XMLName   xml.Name  `xml:"Response"`
	RequestId string    `xml:"RequestId,omitempty"`
	CarTags   []CarTags `xml:"CarTags,omitempty"`
}

type CarTags struct {
	Serial       string         `xml:"Serial,omitempty"`
	Brand        string         `xml:"Brand,omitempty"`
	Type         string         `xml:"Type,omitempty"`
	Color        string         `xml:"Color,omitempty"`
	Confidence   int            `xml:"Confidence,omitempty"`
	Year         int            `xml:"Year,omitempty"`
	CarLocation  []CarLocation  `xml:"CarLocation,omitempty"`
	PlateContent []PlateContent `xml:"PlateContent,omitempty"`
}

type CarLocation struct {
	X int `xml:"X,omitempty"`
	Y int `xml:"Y,omitempty"`
}

type PlateContent struct {
	Plate         string         `xml:"Plate,omitempty"`
	Color         string         `xml:"Color,omitempty"`
	Type          string         `xml:"Type,omitempty"`
	PlateLocation *PlateLocation `xml:"PlateLocation,omitempty"`
}

type PlateLocation struct {
	X int `xml:"X,omitempty"`
	Y int `xml:"Y,omitempty"`
}

// DetectCar 车辆车牌检测
func (s *CIService) DetectCar(ctx context.Context, obj string) (*DetectCarResult, *Response, error) {
	var res DetectCarResult
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.BucketURL,
		uri:     "/" + encodeURIComponent(obj) + "?ci-process=DetectCar",
		method:  http.MethodGet,
		result:  &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

type CIServiceResult struct {
	XMLName  xml.Name `xml:"CIStatus"`
	CIStatus string   `xml:",chardata"`
}

func (s *CIService) OpenCIService(ctx context.Context) (*Response, error) {
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		method:  http.MethodPut,
		uri:     "/",
	}
	resp, err := s.client.send(ctx, sendOpt)
	return resp, err
}

func (s *CIService) GetCIService(ctx context.Context) (*CIServiceResult, *Response, error) {
	var res CIServiceResult
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		method:  http.MethodGet,
		uri:     "/",
		result:  &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

func (s *CIService) CloseCIService(ctx context.Context) (*Response, error) {
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		method:  http.MethodPut,
		uri:     "/?unbind",
	}
	resp, err := s.client.send(ctx, sendOpt)
	return resp, err
}

type HotLinkOptions struct {
	XMLName xml.Name `xml:"Hotlink"`
	Url     []string `xml:"Url,omitempty"`
	Type    string   `xml:"Type,omitempty"`
}

type HotLinkResult struct {
	XMLName xml.Name `xml:"Hotlink"`
	Status  string   `xml:"Status,omitempty"`
	Type    string   `xml:"Type,omitempty"`
	Url     []string `xml:"Url,omitempty"`
}

func (s *CIService) SetHotLink(ctx context.Context, opt *HotLinkOptions) (*Response, error) {
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		method:  http.MethodPut,
		uri:     "/?hotlink",
		body:    opt,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return resp, err
}

func (s *CIService) GetHotLink(ctx context.Context) (*HotLinkResult, *Response, error) {
	var res HotLinkResult
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		method:  http.MethodGet,
		uri:     "/?hotlink",
		result:  &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

type OriginProtectResult struct {
	XMLName             xml.Name `xml:"OriginProtectStatus"`
	OriginProtectStatus string   `xml:",chardata"`
}

func (s *CIService) OpenOriginProtect(ctx context.Context) (*Response, error) {
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		method:  http.MethodPut,
		uri:     "/?origin-protect",
	}
	resp, err := s.client.send(ctx, sendOpt)
	return resp, err
}

func (s *CIService) GetOriginProtect(ctx context.Context) (*OriginProtectResult, *Response, error) {
	var res OriginProtectResult
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		method:  http.MethodGet,
		uri:     "/?origin-protect",
		result:  &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

func (s *CIService) CloseOriginProtect(ctx context.Context) (*Response, error) {
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		method:  http.MethodDelete,
		uri:     "/?origin-protect",
	}
	resp, err := s.client.send(ctx, sendOpt)
	return resp, err
}

type PicTagResult struct {
	XMLName xml.Name `xml:"RecognitionResult"`
	Labels  []PicTag `xml:"Labels,omitempty"`
}

type PicTag struct {
	Confidence int    `xml:"Confidence,omitempty"`
	Name       string `xml:"Name,omitempty"`
}

func (s *CIService) PicTag(ctx context.Context, obj string) (*PicTagResult, *Response, error) {
	var res PicTagResult
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.CIURL,
		method:  http.MethodGet,
		uri:     "/" + encodeURIComponent(obj) + "?ci-process=detect-label",
		result:  &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

type DetectFaceOptions struct {
	MaxFaceNum int `url:"max-face-num,omitempty"`
}

type DetectFaceResult struct {
	XMLName          xml.Name    `xml:"Response"`
	ImageWidth       int         `xml:"ImageWidth,omitempty"`
	ImageHeight      int         `xml:"ImageHeight,omitempty"`
	FaceModelVersion string      `xml:"FaceModelVersion,omitempty"`
	RequestId        string      `xml:"RequestId,omitempty"`
	FaceInfos        []FaceInfos `xml:"FaceInfos,omitempty"`
}

type FaceInfos struct {
	X      int `xml:"X,omitempty"`
	Y      int `xml:"Y,omitempty"`
	Width  int `xml:"Width,omitempty"`
	Height int `xml:"Height,omitempty"`
}

func (s *CIService) DetectFace(ctx context.Context, obj string, opt *DetectFaceOptions) (*DetectFaceResult, *Response, error) {
	var res DetectFaceResult
	sendOpt := &sendOptions{
		baseURL:  s.client.BaseURL.BucketURL,
		method:   http.MethodGet,
		uri:      "/" + encodeURIComponent(obj) + "?ci-process=DetectFace",
		optQuery: opt,
		result:   &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

type FaceEffectOptions struct {
	Type         string `url:"type,omitempty"`
	Whitening    int    `url:"whitening,omitempty"`
	Smoothing    int    `url:"smoothing,omitempty"`
	FaceLifting  int    `url:"faceLifting,omitempty"`
	EyeEnlarging int    `url:"eyeEnlarging,omitempty"`
	Gender       int    `url:"gender,omitempty"`
	Age          int    `url:"age,omitempty"`
}

type FaceEffectResult struct {
	XMLName     xml.Name `xml:"Response"`
	ResultImage string   `xml:"ResultImage,omitempty"`
	ResultMask  string   `xml:"ResultMask,omitempty"`
}

func (s *CIService) FaceEffect(ctx context.Context, obj string, opt *FaceEffectOptions) (*FaceEffectResult, *Response, error) {
	var res FaceEffectResult
	sendOpt := &sendOptions{
		baseURL:  s.client.BaseURL.BucketURL,
		method:   http.MethodGet,
		uri:      "/" + encodeURIComponent(obj) + "?ci-process=face-effect",
		optQuery: opt,
		result:   &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

type IdCardOCROptions struct {
	CardSide string                  `url:"CardSide,omitempty"`
	Config   *IdCardOCROptionsConfig `url:"Config,omitempty"`
}

type IdCardOCROptionsConfig struct {
	CropIdCard      bool `json:"CropIdCard,omitempty"`
	CropPortrait    bool `json:"CropPortrait,omitempty"`
	CopyWarn        bool `json:"CopyWarn,omitempty"`
	BorderCheckWarn bool `json:"BorderCheckWarn,omitempty"`
	ReshootWarn     bool `json:"ReshootWarn,omitempty"`
	DetectPsWarn    bool `json:"DetectPsWarn,omitempty"`
	TempIdWarn      bool `json:"TempIdWarn,omitempty"`
	InvalidDateWarn bool `json:"InvalidDateWarn,omitempty"`
	Quality         bool `json:"Quality,omitempty"`
	MultiCardDetect bool `json:"MultiCardDetect,omitempty"`
}

func (c *IdCardOCROptionsConfig) EncodeValues(key string, v *url.Values) error {
	config, err := json.Marshal(c)
	if err != nil {
		return err
	}
	v.Add("Config", string(config))

	return nil
}

type IdCardOCRResult struct {
	XMLName      xml.Name            `xml:"Response"`
	IdInfo       *IdCardInfo         `xml:"IdInfo,omitempty"`
	AdvancedInfo *IdCardAdvancedInfo `xml:"AdvancedInfo,omitempty"`
}

type IdCardInfo struct {
	Name      string `xml:"Name,omitempty"`
	Sex       string `xml:"Sex,omitempty"`
	Nation    string `xml:"Nation,omitempty"`
	Birth     string `xml:"Birth,omitempty"`
	Address   string `xml:"Address,omitempty"`
	IdNum     string `xml:"IdNum,omitempty"`
	Authority string `xml:"Authority,omitempty"`
	ValidDate string `xml:"ValidDate,omitempty"`
}

type IdCardAdvancedInfo struct {
	IdCard          string   `xml:"IdCard,omitempty"`
	Portrait        string   `xml:"Portrait,omitempty"`
	Quality         string   `xml:"Quality,omitempty"`
	BorderCodeValue string   `xml:"BorderCodeValue,omitempty"`
	WarnInfos       []string `xml:"WarnInfos,omitempty"`
}

func (s *CIService) IdCardOCRWhenCloud(ctx context.Context, obj string, query *IdCardOCROptions) (*IdCardOCRResult, *Response, error) {
	var res IdCardOCRResult
	sendOpt := &sendOptions{
		baseURL:  s.client.BaseURL.BucketURL,
		method:   http.MethodGet,
		uri:      "/" + encodeURIComponent(obj) + "?ci-process=IDCardOCR",
		optQuery: query,
		result:   &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

func (s *CIService) IdCardOCRWhenUpload(ctx context.Context, obj, filePath string, query *IdCardOCROptions, header *ObjectPutOptions) (*IdCardOCRResult, *Response, error) {
	fd, err := os.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	defer fd.Close()

	if err := CheckReaderLen(fd); err != nil {
		return nil, nil, err
	}
	opt := CloneObjectPutOptions(header)
	totalBytes, err := GetReaderLen(fd)
	if err != nil && opt != nil && opt.Listener != nil {
		if opt.ContentLength == 0 {
			return nil, nil, err
		}
		totalBytes = opt.ContentLength
	}
	if err == nil {
		// 与 go http 保持一致, 非bytes.Buffer/bytes.Reader/strings.Reader由用户指定ContentLength, 或使用 Chunk 上传
		// if opt != nil && opt.ContentLength == 0 && IsLenReader(r) {
		// 	opt.ContentLength = totalBytes
		// }
		// lilang : 2022-07-04
		// 图片cgi不设置content-length的话，读不到body。图片处理cgi暂时不支持chunked，后面会修复。
		if opt != nil && opt.ContentLength == 0 {
			opt.ContentLength = totalBytes
		}

	}
	reader := TeeReader(fd, nil, totalBytes, nil)
	if s.client.Conf.EnableCRC {
		reader.writer = crc64.New(crc64.MakeTable(crc64.ECMA))
	}
	if opt != nil && opt.Listener != nil {
		reader.listener = opt.Listener
	}

	var res IdCardOCRResult
	sendOpt := sendOptions{
		baseURL:   s.client.BaseURL.BucketURL,
		uri:       "/" + encodeURIComponent(obj) + "?ci-process=IDCardOCR",
		method:    http.MethodPut,
		optQuery:  query,
		body:      reader,
		optHeader: opt,
		result:    &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)

	return &res, resp, err
}

type GetLiveCodeResult struct {
	XMLName  xml.Name `xml:"Response"`
	LiveCode string   `xml:"LiveCode,omitempty"`
}

func (s *CIService) GetLiveCode(ctx context.Context) (*GetLiveCodeResult, *Response, error) {
	var res GetLiveCodeResult
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.BucketURL,
		method:  http.MethodGet,
		uri:     "/?ci-process=GetLiveCode",
		result:  &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

type GetActionSequenceResult struct {
	XMLName        xml.Name `xml:"Response"`
	ActionSequence string   `xml:"ActionSequence,omitempty"`
}

func (s *CIService) GetActionSequence(ctx context.Context) (*GetActionSequenceResult, *Response, error) {
	var res GetActionSequenceResult
	sendOpt := &sendOptions{
		baseURL: s.client.BaseURL.BucketURL,
		method:  http.MethodGet,
		uri:     "/?ci-process=GetActionSequence",
		result:  &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

type LivenessRecognitionOptions struct {
	IdCard       string `url:"IdCard,omitempty"`
	Name         string `url:"Name,omitempty"`
	LivenessType string `url:"LivenessType,omitempty"`
	ValidateData string `url:"ValidateData,omitempty"`
	BestFrameNum int    `url:"BestFrameNum,omitempty"`
}

type LivenessRecognitionResult struct {
	XMLName         xml.Name `xml:"Response"`
	BestFrameBase64 string   `xml:"BestFrameBase64,omitempty"`
	Sim             float64  `xml:"Sim,omitempty"`
	BestFrameList   []string `xml:"BestFrameList,omitempty"`
}

func (s *CIService) LivenessRecognitionWhenCloud(ctx context.Context, obj string, query *LivenessRecognitionOptions) (*LivenessRecognitionResult, *Response, error) {
	var res LivenessRecognitionResult
	sendOpt := &sendOptions{
		baseURL:  s.client.BaseURL.BucketURL,
		method:   http.MethodGet,
		uri:      "/" + encodeURIComponent(obj) + "?ci-process=LivenessRecognition",
		optQuery: query,
		result:   &res,
	}
	resp, err := s.client.send(ctx, sendOpt)
	return &res, resp, err
}

func (s *CIService) LivenessRecognitionWhenUpload(ctx context.Context, obj, filePath string, query *LivenessRecognitionOptions, header *ObjectPutOptions) (*LivenessRecognitionResult, *Response, error) {
	fd, err := os.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	defer fd.Close()

	if err := CheckReaderLen(fd); err != nil {
		return nil, nil, err
	}
	opt := CloneObjectPutOptions(header)
	totalBytes, err := GetReaderLen(fd)
	if err != nil && opt != nil && opt.Listener != nil {
		if opt.ContentLength == 0 {
			return nil, nil, err
		}
		totalBytes = opt.ContentLength
	}
	if err == nil {
		// 与 go http 保持一致, 非bytes.Buffer/bytes.Reader/strings.Reader由用户指定ContentLength, 或使用 Chunk 上传
		// if opt != nil && opt.ContentLength == 0 && IsLenReader(r) {
		// 	opt.ContentLength = totalBytes
		// }
		// lilang : 2022-07-04
		// 图片cgi不设置content-length的话，读不到body。图片处理cgi暂时不支持chunked，后面会修复。
		if opt != nil && opt.ContentLength == 0 {
			opt.ContentLength = totalBytes
		}

	}
	reader := TeeReader(fd, nil, totalBytes, nil)
	if s.client.Conf.EnableCRC {
		reader.writer = crc64.New(crc64.MakeTable(crc64.ECMA))
	}
	if opt != nil && opt.Listener != nil {
		reader.listener = opt.Listener
	}

	var res LivenessRecognitionResult
	sendOpt := sendOptions{
		baseURL:   s.client.BaseURL.BucketURL,
		uri:       "/" + encodeURIComponent(obj) + "?ci-process=LivenessRecognition",
		method:    http.MethodPut,
		optQuery:  query,
		body:      reader,
		optHeader: opt,
		result:    &res,
	}
	resp, err := s.client.send(ctx, &sendOpt)

	return &res, resp, err
}

// GoodsMatting 商品抠图
func (s *CIService) GoodsMatting(ctx context.Context, key string) (*Response, error) {
	sendOpt := sendOptions{
		baseURL:          s.client.BaseURL.BucketURL,
		uri:              "/" + encodeURIComponent(key) + "?ci-process=GoodsMatting",
		method:           http.MethodGet,
		disableCloseBody: true,
	}
	resp, err := s.client.send(ctx, &sendOpt)
	return resp, err
}
