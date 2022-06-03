package cos

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
)

// ErrorResponse 包含 API 返回的错误信息
//
// https://www.qcloud.com/document/product/436/7730
type ErrorResponse struct {
	XMLName   xml.Name       `xml:"Error"`
	Response  *http.Response `xml:"-"`
	Code      string
	Message   string
	Resource  string
	RequestID string `header:"x-cos-request-id,omitempty" url:"-" xml:"RequestId,omitempty"`
	TraceID   string `xml:"TraceId,omitempty"`
}

// Error returns the error msg
func (r *ErrorResponse) Error() string {
	RequestID := r.RequestID
	if RequestID == "" {
		RequestID = r.Response.Header.Get("X-Cos-Request-Id")
	}
	TraceID := r.TraceID
	if TraceID == "" {
		TraceID = r.Response.Header.Get("X-Cos-Trace-Id")
	}
	decodeURL, err := decodeURIComponent(r.Response.Request.URL.String())
	if err != nil {
		decodeURL = r.Response.Request.URL.String()
	}
	return fmt.Sprintf("%v %v: %d %v(Message: %v, RequestId: %v, TraceId: %v)",
		r.Response.Request.Method, decodeURL,
		r.Response.StatusCode, r.Code, r.Message, RequestID, TraceID)
}

type jsonError struct {
	Code      int    `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// 检查 response 是否是出错时的返回的 response
func checkResponse(r *http.Response) error {
	if c := r.StatusCode; 200 <= c && c <= 299 {
		return nil
	}
	errorResponse := &ErrorResponse{Response: r}
	data, err := ioutil.ReadAll(r.Body)
	if err == nil && data != nil {
		xml.Unmarshal(data, errorResponse)
	}
	// 是否为 json 格式
	if errorResponse.Code == "" {
		ctype := strings.TrimLeft(r.Header.Get("Content-Type"), " ")
		if strings.HasPrefix(ctype, "application/json") {
			var jerror jsonError
			json.Unmarshal(data, &jerror)
			errorResponse.Code = strconv.Itoa(jerror.Code)
			errorResponse.Message = jerror.Message
			errorResponse.RequestID = jerror.RequestID
		}

	}
	return errorResponse
}

func IsNotFoundError(e error) bool {
	if e == nil {
		return false
	}
	err, ok := e.(*ErrorResponse)
	if !ok {
		return false
	}
	if err.Response != nil && err.Response.StatusCode == 404 {
		return true
	}
	return false
}

func IsCOSError(e error) (*ErrorResponse, bool) {
	if e == nil {
		return nil, false
	}
	err, ok := e.(*ErrorResponse)
	return err, ok
}
