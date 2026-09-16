package pan115

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/httpx"
)

const (
	webAPIHost   = "https://webapi.115.com"
	proAPIHost   = "https://proapi.115.com"
	myHost       = "https://my.115.com"
	qrcodeHost   = "https://qrcodeapi.115.com"
	passportHost = "https://passportapi.115.com"

	referer115 = "https://115.com/"
	webUA      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	operationDelayMS = 200
	listPageSize     = 1150

	downloadChunkSize  = 10 * 1024 * 1024
	downloadConcurrent = 2
	downloadLinkTTL    = 5 * time.Minute
)

// apiEnvelope 是 115 Web API 的统一响应外壳；state 可能是 bool 或 0/1。
type apiEnvelope struct {
	State   json.RawMessage `json:"state"`
	Errno   flexNumber      `json:"errno"`
	ErrNo   flexNumber      `json:"errNo"`
	Code    flexNumber      `json:"code"`
	Error   string          `json:"error"`
	Message string          `json:"message"`
	Msg     string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
}

func (d *Driver) apiRequest(ctx context.Context, method, rawURL string, query, form url.Values, out any) error {
	return d.apiRequestMode(ctx, method, rawURL, query, form, nil, out, false)
}

func (d *Driver) apiRequestFull(ctx context.Context, method, rawURL string, query, form url.Values, out any) error {
	return d.apiRequestMode(ctx, method, rawURL, query, form, nil, out, true)
}

func (d *Driver) apiRequestMode(ctx context.Context, method, rawURL string, query, form url.Values, extraHeaders map[string]string, out any, full bool) error {
	if err := d.waitInterval(ctx); err != nil {
		return err
	}
	if len(query) > 0 {
		sep := "?"
		if strings.Contains(rawURL, "?") {
			sep = "&"
		}
		rawURL += sep + query.Encode()
	}

	var body io.Reader
	if len(form) > 0 {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return domain.Wrap(domain.CodeInternal, err)
	}
	headers := map[string]string{
		"User-Agent":      webUA,
		"Referer":         referer115,
		"Accept":          "application/json, text/plain, */*",
		"Accept-Language": "zh-CN,zh;q=0.9",
	}
	if len(form) > 0 {
		headers["Content-Type"] = "application/x-www-form-urlencoded"
	}
	for k, v := range extraHeaders {
		headers[k] = v
	}
	httpx.SetHeaders(req, headers)
	if ck := d.currentCookie(); ck != "" {
		req.Header.Set("Cookie", ck)
	}

	resp, data, err := httpx.Execute(d.client, req, 16<<20)
	if err != nil {
		return domain.Wrap(domain.CodeDriverError, err)
	}
	d.absorbSetCookie(ctx, resp.Header)

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return domain.Errorf(domain.CodeAuthExpired, "115 Cookie 认证失败，请重新登录")
	case resp.StatusCode == http.StatusForbidden:
		return domain.Errorf(domain.CodePermissionDenied, "115 访问被拒绝，Cookie 权限不足")
	case resp.StatusCode == http.StatusTooManyRequests:
		return domain.Errorf(domain.CodeRateLimited, "115 请求过于频繁，请稍后再试")
	case resp.StatusCode >= http.StatusBadRequest:
		return domain.Errorf(domain.CodeDriverError, "115 HTTP %d：%s", resp.StatusCode, httpx.Truncate(data, 300))
	}

	var env apiEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return domain.Errorf(domain.CodeDriverError, "115 返回非 JSON 内容：%s", httpx.Truncate(data, 300))
	}
	if !isSuccessState(env.State) {
		return mapAPIError(env)
	}
	if out == nil {
		return nil
	}
	if full {
		if err := json.Unmarshal(data, out); err != nil {
			return domain.Errorf(domain.CodeDriverError, "115 响应解析失败：%v", err)
		}
		return nil
	}
	if raw, ok := out.(*json.RawMessage); ok {
		*raw = append(json.RawMessage(nil), env.Data...)
		return nil
	}
	if len(env.Data) > 0 && string(bytes.TrimSpace(env.Data)) != "null" {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return domain.Errorf(domain.CodeDriverError, "115 响应 data 解析失败：%v", err)
		}
	}
	return nil
}

func isSuccessState(raw json.RawMessage) bool {
	if len(bytes.TrimSpace(raw)) == 0 {
		return true
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b
	}
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return n == 1
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.EqualFold(strings.TrimSpace(s), "true")
	}
	return false
}

func mapAPIError(env apiEnvelope) error {
	code := env.Errno.int64()
	if code == 0 {
		code = env.ErrNo.int64()
	}
	if code == 0 {
		code = env.Code.int64()
	}
	msg := firstNonEmpty(env.Error, env.Message, env.Msg, "未知错误")
	if isAuthErrno(code) || looksLikeAuthMessage(msg) {
		return domain.Errorf(domain.CodeAuthExpired, "115 Cookie 认证失效：%s", msg)
	}
	if code == 0 {
		return domain.Errorf(domain.CodeDriverError, "115 接口返回失败：%s", msg)
	}
	return domain.Errorf(domain.CodeDriverError, "115 接口错误(%d)：%s", code, msg)
}

func isAuthErrno(code int64) bool {
	if code == 401 {
		return true
	}
	return strings.HasPrefix(strconv.FormatInt(code, 10), "401")
}

func looksLikeAuthMessage(msg string) bool {
	l := strings.ToLower(msg)
	for _, k := range []string{"登录", "登陆", "cookie", "重新授权"} {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func (d *Driver) waitInterval(ctx context.Context) error {
	return driver.WaitRequestInterval(ctx, d.intervalGate, operationDelayMS)
}

func (d *Driver) rootID() string {
	if id := strings.TrimSpace(d.add.RootFolderID); id != "" {
		return id
	}
	return "0"
}

func (d *Driver) normalizeParent(parentID string) string {
	p := strings.TrimSpace(parentID)
	if p == "" || p == "/" || p == "root" || p == "0" {
		return d.rootID()
	}
	return p
}

func nowMillis() string {
	return strconv.FormatInt(time.Now().UnixMilli(), 10)
}

func nowSeconds() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}

// listQuery 组装 /files 列表请求参数。
func listQuery(cid string, offset int, showDir bool, recursive bool) url.Values {
	query := url.Values{
		"aid":              {"1"},
		"cid":              {cid},
		"o":                {"user_ptime"},
		"asc":              {"1"},
		"offset":           {strconv.Itoa(offset)},
		"limit":            {strconv.Itoa(listPageSize)},
		"snap":             {"0"},
		"natsort":          {"0"},
		"record_open_time": {"1"},
		"format":           {"json"},
		"fc_mix":           {"0"},
	}
	if showDir {
		query.Set("show_dir", "1")
	} else {
		query.Set("show_dir", "0")
	}
	if recursive {
		query.Set("cur", "0")
	}
	return query
}
