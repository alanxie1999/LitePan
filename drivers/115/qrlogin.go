package pan115

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/httpx"
)

const (
	qrTokenPath  = "/api/1.0/web/1.0/token"
	qrStatusPath = "/get/status/"
	qrLoginPath  = "/app/1.0/web/1.0/login/qrcode"

	qrCodeTimeoutSec = 240

	qrStatusWaiting  = 0
	qrStatusScanned  = 1
	qrStatusAllowed  = 2
	qrStatusExpired  = -1
	qrStatusCanceled = -2
)

// qrSession 是扫码会话的不透明续询令牌内容（base64(JSON)，由客户端持有并回传）。
type qrSession struct {
	UID     string `json:"u"`
	Sign    string `json:"s"`
	Time    int64  `json:"t"`
	Created int64  `json:"ts"`
}

type qrEnvelope struct {
	State int             `json:"state"`
	Code  int             `json:"code"`
	Error string          `json:"error"`
	Msg   string          `json:"message"`
	Data  json.RawMessage `json:"data"`
}

type qrTokenData struct {
	UID    string `json:"uid"`
	Sign   string `json:"sign"`
	Time   int64  `json:"time"`
	QRCode string `json:"qrcode"`
}

type qrStatusData struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
}

type qrCredential struct {
	UID  string `json:"UID"`
	CID  string `json:"CID"`
	SEID string `json:"SEID"`
	KID  string `json:"KID"`
}

// StartQRLogin 取二维码 token，渲染二维码图，返回不透明续询令牌。
func (d *Driver) StartQRLogin(ctx context.Context) (*driver.QRStartResult, error) {
	resp, body, err := d.qrFetch(ctx, http.MethodGet, qrcodeHost+qrTokenPath, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, domain.Errorf(domain.CodeDriverError, "获取 115 二维码失败，HTTP %d", resp.StatusCode)
	}
	var env qrEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, domain.Errorf(domain.CodeDriverError, "115 二维码接口返回异常")
	}
	if env.State != 1 {
		return nil, domain.Errorf(domain.CodeDriverError, "获取 115 二维码失败：%s", firstNonEmpty(env.Error, env.Msg, "未知错误"))
	}
	var data qrTokenData
	if err := json.Unmarshal(env.Data, &data); err != nil || strings.TrimSpace(data.UID) == "" {
		return nil, domain.Errorf(domain.CodeDriverError, "115 二维码数据解析失败")
	}

	content := strings.TrimSpace(data.QRCode)
	if content == "" {
		content = "https://115.com/scan/dg-" + data.UID
	}
	png, err := qrcode.Encode(content, qrcode.Medium, 256)
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, err)
	}

	now := time.Now().Unix()
	if data.Time <= 0 {
		data.Time = now
	}
	opaque := encodeQRSession(qrSession{UID: data.UID, Sign: data.Sign, Time: data.Time, Created: now})
	return &driver.QRStartResult{
		Token:         opaque,
		QRImageBase64: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		QRURL:         content,
		ExpiresIn:     qrCodeTimeoutSec,
		Title:         "扫码获取 Cookie",
		Hint:          "请使用 115 App 扫码，确认后授权信息将自动填入表单",
	}, nil
}

// PollQRLogin 轮询扫码状态；确认后换取 UID/CID/SEID/KID 并组装 Cookie。
func (d *Driver) PollQRLogin(ctx context.Context, opaque string) (*driver.QRPollResult, error) {
	sess, err := decodeQRSession(opaque)
	if err != nil || strings.TrimSpace(sess.UID) == "" {
		return nil, domain.Errorf(domain.CodeValidation, "扫码会话无效，请重新获取二维码")
	}
	if time.Now().Unix()-sess.Created > qrCodeTimeoutSec {
		return &driver.QRPollResult{Status: driver.QRExpired, Message: "二维码已过期，请重新获取"}, nil
	}

	query := url.Values{
		"uid":  {sess.UID},
		"time": {strconv.FormatInt(sess.Time, 10)},
		"sign": {sess.Sign},
		"_":    {nowMillis()},
	}
	resp, body, err := d.qrFetch(ctx, http.MethodGet, qrcodeHost+qrStatusPath, query, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		// 网络波动按等待处理，让前端继续轮询。
		return &driver.QRPollResult{Status: driver.QRWaiting}, nil
	}
	var env qrEnvelope
	if err := json.Unmarshal(body, &env); err != nil || env.State != 1 {
		return &driver.QRPollResult{Status: driver.QRWaiting}, nil
	}
	var status qrStatusData
	if err := json.Unmarshal(env.Data, &status); err != nil {
		return &driver.QRPollResult{Status: driver.QRWaiting}, nil
	}

	switch status.Status {
	case qrStatusAllowed:
		return d.finishQRLogin(ctx, sess)
	case qrStatusExpired:
		return &driver.QRPollResult{Status: driver.QRExpired, Message: "二维码已过期，请重新获取"}, nil
	case qrStatusCanceled:
		return &driver.QRPollResult{Status: driver.QRFailed, Message: firstNonEmpty(status.Msg, "已取消扫码登录")}, nil
	case qrStatusWaiting, qrStatusScanned:
		return &driver.QRPollResult{Status: driver.QRWaiting}, nil
	default:
		return &driver.QRPollResult{Status: driver.QRWaiting}, nil
	}
}

// finishQRLogin 用扫码确认后的 uid 换取 Cookie 凭证。
func (d *Driver) finishQRLogin(ctx context.Context, sess qrSession) (*driver.QRPollResult, error) {
	form := url.Values{"account": {sess.UID}, "app": {"web"}}
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	resp, body, err := d.qrFetch(ctx, http.MethodPost, passportHost+qrLoginPath, nil, form, headers)
	if err != nil {
		return &driver.QRPollResult{Status: driver.QRFailed, Message: "获取登录 Cookie 失败"}, nil
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return &driver.QRPollResult{Status: driver.QRFailed, Message: "获取登录 Cookie 失败，请重试"}, nil
	}
	var env struct {
		State int             `json:"state"`
		Error string          `json:"error"`
		Msg   string          `json:"message"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return &driver.QRPollResult{Status: driver.QRFailed, Message: "登录响应解析失败，请重试"}, nil
	}
	if env.State != 1 {
		return &driver.QRPollResult{Status: driver.QRFailed, Message: firstNonEmpty(env.Error, env.Msg, "扫码登录失败")}, nil
	}
	var data struct {
		Cookie qrCredential `json:"cookie"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return &driver.QRPollResult{Status: driver.QRFailed, Message: "登录凭证解析失败，请重试"}, nil
	}

	c := data.Cookie
	if strings.TrimSpace(c.UID) == "" || strings.TrimSpace(c.SEID) == "" {
		return &driver.QRPollResult{Status: driver.QRFailed, Message: "登录完成但未获取到有效 Cookie，请重试"}, nil
	}
	pairs := map[string]string{"UID": c.UID, "CID": c.CID, "SEID": c.SEID, "KID": c.KID}
	cookie := buildCookie([]string{"UID", "CID", "SEID", "KID"}, pairs)

	return &driver.QRPollResult{
		Status:      driver.QRSuccess,
		Credentials: domain.AuthCredentials{Cookie: cookie},
		Fields:      map[string]string{"cookie": cookie},
	}, nil
}

// qrFetch 发送扫码相关请求；扫码阶段无需 Cookie，如需则由 caller 注入。
func (d *Driver) qrFetch(ctx context.Context, method, rawURL string, query, form url.Values, extraHeaders map[string]string) (*http.Response, []byte, error) {
	if len(query) > 0 {
		rawURL += "?" + query.Encode()
	}
	var body io.Reader
	if len(form) > 0 {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, nil, domain.Wrap(domain.CodeInternal, err)
	}
	headers := map[string]string{
		"User-Agent":      webUA,
		"Accept":          "application/json, text/plain, */*",
		"Accept-Language": "zh-CN,zh;q=0.9",
		"Referer":         referer115,
	}
	for k, v := range extraHeaders {
		headers[k] = v
	}
	httpx.SetHeaders(req, headers)
	return httpx.Execute(d.client, req, 4<<20)
}

func encodeQRSession(s qrSession) string {
	b, _ := json.Marshal(s)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeQRSession(s string) (qrSession, error) {
	var out qrSession
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	return out, nil
}
