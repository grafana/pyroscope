package cos

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"hash"
	"io/ioutil"
	math_rand "math/rand"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	sha1SignAlgorithm   = "sha1"
	privateHeaderPrefix = "x-cos-"
	defaultAuthExpire   = time.Hour
)

var (
	defaultCVMAuthExpire = int64(600)
	defaultCVMSchema     = "http"
	defaultCVMMetaHost   = "metadata.tencentyun.com"
	defaultCVMCredURI    = "latest/meta-data/cam/security-credentials"
	internalHost         = regexp.MustCompile(`^.*cos-internal\.[a-z-1]+\.tencentcos\.cn$`)
)

var DNSScatterDialContext = DNSScatterDialContextFunc

var DNSScatterTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           DNSScatterDialContext,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

func DNSScatterDialContextFunc(ctx context.Context, network string, addr string) (conn net.Conn, err error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}
	// DNS 打散
	math_rand.Seed(time.Now().UnixNano())
	start := math_rand.Intn(len(ips))
	for i := start; i < len(ips); i++ {
		conn, err = dialer.DialContext(ctx, network, net.JoinHostPort(ips[i].IP.String(), port))
		if err == nil {
			return
		}
	}
	for i := 0; i < start; i++ {
		conn, err = dialer.DialContext(ctx, network, net.JoinHostPort(ips[i].IP.String(), port))
		if err == nil {
			break
		}
	}
	return
}

// 需要校验的 Headers 列表
var NeedSignHeaders = map[string]bool{
	"host":                           true,
	"range":                          true,
	"x-cos-acl":                      true,
	"x-cos-grant-read":               true,
	"x-cos-grant-write":              true,
	"x-cos-grant-full-control":       true,
	"response-content-type":          true,
	"response-content-language":      true,
	"response-expires":               true,
	"response-cache-control":         true,
	"response-content-disposition":   true,
	"response-content-encoding":      true,
	"cache-control":                  true,
	"content-disposition":            true,
	"content-encoding":               true,
	"content-type":                   true,
	"content-length":                 true,
	"content-md5":                    true,
	"transfer-encoding":              true,
	"versionid":                      true,
	"expect":                         true,
	"expires":                        true,
	"x-cos-content-sha1":             true,
	"x-cos-storage-class":            true,
	"if-match":                       true,
	"if-modified-since":              true,
	"if-none-match":                  true,
	"if-unmodified-since":            true,
	"origin":                         true,
	"access-control-request-method":  true,
	"access-control-request-headers": true,
	"x-cos-object-type":              true,
}

// 非线程安全，只能在进程初始化（而不是Client初始化）时做设置
func SetNeedSignHeaders(key string, val bool) {
	NeedSignHeaders[key] = val
}

func safeURLEncode(s string) string {
	s = encodeURIComponent(s)
	s = strings.Replace(s, "!", "%21", -1)
	s = strings.Replace(s, "'", "%27", -1)
	s = strings.Replace(s, "(", "%28", -1)
	s = strings.Replace(s, ")", "%29", -1)
	s = strings.Replace(s, "*", "%2A", -1)
	return s
}

type valuesSignMap map[string][]string

func (vs valuesSignMap) Add(key, value string) {
	key = strings.ToLower(safeURLEncode(key))
	vs[key] = append(vs[key], value)
}

func (vs valuesSignMap) Encode() string {
	var keys []string
	for k := range vs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var pairs []string
	for _, k := range keys {
		items := vs[k]
		sort.Strings(items)
		for _, val := range items {
			pairs = append(
				pairs,
				fmt.Sprintf("%s=%s", k, safeURLEncode(val)))
		}
	}
	return strings.Join(pairs, "&")
}

// AuthTime 用于生成签名所需的 q-sign-time 和 q-key-time 相关参数
type AuthTime struct {
	SignStartTime time.Time
	SignEndTime   time.Time
	KeyStartTime  time.Time
	KeyEndTime    time.Time
}

// NewAuthTime 生成 AuthTime 的便捷函数
//
//   expire: 从现在开始多久过期.
func NewAuthTime(expire time.Duration) *AuthTime {
	signStartTime := time.Now()
	keyStartTime := signStartTime
	signEndTime := signStartTime.Add(expire)
	keyEndTime := signEndTime
	return &AuthTime{
		SignStartTime: signStartTime,
		SignEndTime:   signEndTime,
		KeyStartTime:  keyStartTime,
		KeyEndTime:    keyEndTime,
	}
}

// signString return q-sign-time string
func (a *AuthTime) signString() string {
	return fmt.Sprintf("%d;%d", a.SignStartTime.Unix(), a.SignEndTime.Unix())
}

// keyString return q-key-time string
func (a *AuthTime) keyString() string {
	return fmt.Sprintf("%d;%d", a.KeyStartTime.Unix(), a.KeyEndTime.Unix())
}

// newAuthorization 通过一系列步骤生成最终需要的 Authorization 字符串
func newAuthorization(secretID, secretKey string, req *http.Request, authTime *AuthTime, signHost bool) string {
	signTime := authTime.signString()
	keyTime := authTime.keyString()
	signKey := calSignKey(secretKey, keyTime)

	if signHost {
		req.Header.Set("Host", req.Host)
	}
	formatHeaders := *new(string)
	signedHeaderList := *new([]string)
	formatHeaders, signedHeaderList = genFormatHeaders(req.Header)
	formatParameters, signedParameterList := genFormatParameters(req.URL.Query())
	formatString := genFormatString(req.Method, *req.URL, formatParameters, formatHeaders)

	stringToSign := calStringToSign(sha1SignAlgorithm, keyTime, formatString)
	signature := calSignature(signKey, stringToSign)

	return genAuthorization(
		secretID, signTime, keyTime, signature, signedHeaderList,
		signedParameterList,
	)
}

// AddAuthorizationHeader 给 req 增加签名信息
func AddAuthorizationHeader(secretID, secretKey string, sessionToken string, req *http.Request, authTime *AuthTime) {
	if secretID == "" {
		return
	}

	auth := newAuthorization(secretID, secretKey, req,
		authTime, true,
	)
	if len(sessionToken) > 0 {
		req.Header.Set("x-cos-security-token", sessionToken)
	}
	req.Header.Set("Authorization", auth)
}

// calSignKey 计算 SignKey
func calSignKey(secretKey, keyTime string) string {
	digest := calHMACDigest(secretKey, keyTime, sha1SignAlgorithm)
	return fmt.Sprintf("%x", digest)
}

// calStringToSign 计算 StringToSign
func calStringToSign(signAlgorithm, signTime, formatString string) string {
	h := sha1.New()
	h.Write([]byte(formatString))
	return fmt.Sprintf("%s\n%s\n%x\n", signAlgorithm, signTime, h.Sum(nil))
}

// calSignature 计算 Signature
func calSignature(signKey, stringToSign string) string {
	digest := calHMACDigest(signKey, stringToSign, sha1SignAlgorithm)
	return fmt.Sprintf("%x", digest)
}

// genAuthorization 生成 Authorization
func genAuthorization(secretID, signTime, keyTime, signature string, signedHeaderList, signedParameterList []string) string {
	return strings.Join([]string{
		"q-sign-algorithm=" + sha1SignAlgorithm,
		"q-ak=" + secretID,
		"q-sign-time=" + signTime,
		"q-key-time=" + keyTime,
		"q-header-list=" + strings.Join(signedHeaderList, ";"),
		"q-url-param-list=" + strings.Join(signedParameterList, ";"),
		"q-signature=" + signature,
	}, "&")
}

// genFormatString 生成 FormatString
func genFormatString(method string, uri url.URL, formatParameters, formatHeaders string) string {
	formatMethod := strings.ToLower(method)
	formatURI := uri.Path

	return fmt.Sprintf("%s\n%s\n%s\n%s\n", formatMethod, formatURI,
		formatParameters, formatHeaders,
	)
}

// genFormatParameters 生成 FormatParameters 和 SignedParameterList
// instead of the url.Values{}
func genFormatParameters(parameters url.Values) (formatParameters string, signedParameterList []string) {
	ps := valuesSignMap{}
	for key, values := range parameters {
		for _, value := range values {
			ps.Add(key, value)
			signedParameterList = append(signedParameterList, strings.ToLower(safeURLEncode(key)))
		}
	}
	//formatParameters = strings.ToLower(ps.Encode())
	formatParameters = ps.Encode()
	sort.Strings(signedParameterList)
	return
}

// genFormatHeaders 生成 FormatHeaders 和 SignedHeaderList
func genFormatHeaders(headers http.Header) (formatHeaders string, signedHeaderList []string) {
	hs := valuesSignMap{}
	for key, values := range headers {
		if isSignHeader(strings.ToLower(key)) {
			for _, value := range values {
				hs.Add(key, value)
				signedHeaderList = append(signedHeaderList, strings.ToLower(safeURLEncode(key)))
			}
		}
	}
	formatHeaders = hs.Encode()
	sort.Strings(signedHeaderList)
	return
}

// HMAC 签名
func calHMACDigest(key, msg, signMethod string) []byte {
	var hashFunc func() hash.Hash
	switch signMethod {
	case "sha1":
		hashFunc = sha1.New
	default:
		hashFunc = sha1.New
	}
	h := hmac.New(hashFunc, []byte(key))
	h.Write([]byte(msg))
	return h.Sum(nil)
}

func isSignHeader(key string) bool {
	for k, v := range NeedSignHeaders {
		if key == k && v {
			return true
		}
	}
	return strings.HasPrefix(key, privateHeaderPrefix)
}

// AuthorizationTransport 给请求增加 Authorization header
type AuthorizationTransport struct {
	SecretID     string
	SecretKey    string
	SessionToken string
	rwLocker     sync.RWMutex
	// 签名多久过期
	Expire    time.Duration
	Transport http.RoundTripper
}

// SetCredential update the SecretID(ak), SercretKey(sk), sessiontoken
func (t *AuthorizationTransport) SetCredential(ak, sk, token string) {
	t.rwLocker.Lock()
	defer t.rwLocker.Unlock()
	t.SecretID = ak
	t.SecretKey = sk
	t.SessionToken = token
}

// GetCredential get the ak, sk, token
func (t *AuthorizationTransport) GetCredential() (string, string, string) {
	t.rwLocker.RLock()
	defer t.rwLocker.RUnlock()
	return t.SecretID, t.SecretKey, t.SessionToken
}

// RoundTrip implements the RoundTripper interface.
func (t *AuthorizationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = cloneRequest(req) // per RoundTrip contract

	ak, sk, token := t.GetCredential()
	if strings.HasPrefix(ak, " ") || strings.HasSuffix(ak, " ") {
		return nil, fmt.Errorf("SecretID is invalid")
	}
	if strings.HasPrefix(sk, " ") || strings.HasSuffix(sk, " ") {
		return nil, fmt.Errorf("SecretKey is invalid")
	}

	// 增加 Authorization header
	authTime := NewAuthTime(defaultAuthExpire)
	AddAuthorizationHeader(ak, sk, token, req, authTime)

	resp, err := t.transport(req).RoundTrip(req)
	return resp, err
}

func (t *AuthorizationTransport) transport(req *http.Request) http.RoundTripper {
	if t.Transport != nil {
		return t.Transport
	}
	// 内部域名默认使用DNS打散
	if rc := internalHost.MatchString(req.URL.Hostname()); rc {
		return DNSScatterTransport
	}
	return http.DefaultTransport
}

type CVMSecurityCredentials struct {
	TmpSecretId  string `json:",omitempty"`
	TmpSecretKey string `json:",omitempty"`
	ExpiredTime  int64  `json:",omitempty"`
	Expiration   string `json:",omitempty"`
	Token        string `json:",omitempty"`
	Code         string `json:",omitempty"`
}

type CVMCredentialTransport struct {
	RoleName     string
	Transport    http.RoundTripper
	secretID     string
	secretKey    string
	sessionToken string
	expiredTime  int64
	rwLocker     sync.RWMutex
}

func (t *CVMCredentialTransport) GetRoles() ([]string, error) {
	urlname := fmt.Sprintf("%s://%s/%s", defaultCVMSchema, defaultCVMMetaHost, defaultCVMCredURI)
	resp, err := http.Get(urlname)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		bs, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("get cvm security-credentials role failed, StatusCode: %v, Body: %v", resp.StatusCode, string(bs))
	}
	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	roles := strings.Split(strings.TrimSpace(string(bs)), "\n")
	if len(roles) == 0 {
		return nil, fmt.Errorf("get cvm security-credentials role failed, No valid cam role was found")
	}
	return roles, nil
}

// https://cloud.tencent.com/document/product/213/4934
func (t *CVMCredentialTransport) UpdateCredential(now int64) (string, string, string, error) {
	t.rwLocker.Lock()
	defer t.rwLocker.Unlock()
	if t.expiredTime > now+defaultCVMAuthExpire {
		return t.secretID, t.secretKey, t.sessionToken, nil
	}
	roleName := t.RoleName
	if roleName == "" {
		roles, err := t.GetRoles()
		if err != nil {
			return t.secretID, t.secretKey, t.sessionToken, err
		}
		roleName = roles[0]
	}
	urlname := fmt.Sprintf("%s://%s/%s/%s", defaultCVMSchema, defaultCVMMetaHost, defaultCVMCredURI, roleName)
	resp, err := http.Get(urlname)
	if err != nil {
		return t.secretID, t.secretKey, t.sessionToken, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		bs, _ := ioutil.ReadAll(resp.Body)
		return t.secretID, t.secretKey, t.sessionToken, fmt.Errorf("call cvm security-credentials failed, StatusCode: %v, Body: %v", resp.StatusCode, string(bs))
	}
	var cred CVMSecurityCredentials
	err = json.NewDecoder(resp.Body).Decode(&cred)
	if err != nil {
		return t.secretID, t.secretKey, t.sessionToken, err
	}
	if cred.Code != "Success" {
		return t.secretID, t.secretKey, t.sessionToken, fmt.Errorf("call cvm security-credentials failed, Code:%v", cred.Code)
	}
	t.secretID, t.secretKey, t.sessionToken, t.expiredTime = cred.TmpSecretId, cred.TmpSecretKey, cred.Token, cred.ExpiredTime
	return t.secretID, t.secretKey, t.sessionToken, nil
}

func (t *CVMCredentialTransport) GetCredential() (string, string, string, error) {
	now := time.Now().Unix()
	t.rwLocker.RLock()
	// 提前 defaultCVMAuthExpire 获取重新获取临时密钥
	if t.expiredTime <= now+defaultCVMAuthExpire {
		expiredTime := t.expiredTime
		t.rwLocker.RUnlock()
		secretID, secretKey, secretToken, err := t.UpdateCredential(now)
		// 获取临时密钥失败但密钥未过期
		if err != nil && now < expiredTime {
			err = nil
		}
		return secretID, secretKey, secretToken, err
	}
	defer t.rwLocker.RUnlock()
	return t.secretID, t.secretKey, t.sessionToken, nil
}

func (t *CVMCredentialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ak, sk, token, err := t.GetCredential()
	if err != nil {
		return nil, err
	}
	req = cloneRequest(req)
	// 增加 Authorization header
	authTime := NewAuthTime(defaultAuthExpire)
	AddAuthorizationHeader(ak, sk, token, req, authTime)

	resp, err := t.transport().RoundTrip(req)
	return resp, err
}

func (t *CVMCredentialTransport) transport() http.RoundTripper {
	if t.Transport != nil {
		return t.Transport
	}
	return http.DefaultTransport
}

type CredentialTransport struct {
	Transport  http.RoundTripper
	Credential CredentialIface
}

func (t *CredentialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ak, sk, token := t.Credential.GetSecretId(), t.Credential.GetSecretKey(), t.Credential.GetToken()

	req = cloneRequest(req)
	// 增加 Authorization header
	authTime := NewAuthTime(defaultAuthExpire)
	AddAuthorizationHeader(ak, sk, token, req, authTime)

	resp, err := t.transport().RoundTrip(req)
	return resp, err
}

func (t *CredentialTransport) transport() http.RoundTripper {
	if t.Transport != nil {
		return t.Transport
	}
	return http.DefaultTransport
}

type CredentialIface interface {
	GetSecretId() string
	GetToken() string
	GetSecretKey() string
}

func NewTokenCredential(secretId, secretKey, token string) *Credential {
	return &Credential{
		SecretID:     secretId,
		SecretKey:    secretKey,
		SessionToken: token,
	}
}

func (c *Credential) GetSecretKey() string {
	return c.SecretKey
}

func (c *Credential) GetSecretId() string {
	return c.SecretID
}

func (c *Credential) GetToken() string {
	return c.SessionToken
}
