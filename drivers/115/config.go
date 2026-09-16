package pan115

import (
	"encoding/json"
	"math/big"
	"strings"

	"litepan/pkg/jsonvalue"
)

type flexString = jsonvalue.FlexibleString

// Addition 是 115 网盘 Cookie 账号的配置项；Cookie 落 account_auth_states，运行期注入。
type Addition struct {
	Cookie       string     `json:"cookie" label:"Cookie" form:"required,full"`
	DownloadMode string     `json:"download_mode" label:"下载模式" type:"select" options:"proxy:本机代理,redirect:302重定向" default:"proxy" form:"pair=opts1"`
	RootFolderID string     `json:"root_folder_id" label:"根目录ID（默认 0）" default:"0" form:"pair=opts1"`
	CacheTTL     flexString `json:"cache_ttl" label:"缓存时间(分钟)" type:"number" default:"30" form:"pair=opts2"`
}

// flexNumber 兼容 JSON 数字与字符串，并容忍 `t` 这类日期文本（非数字时按 0 处理）。
type flexNumber string

func (f *flexNumber) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*f = ""
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexNumber(strings.TrimSpace(s))
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		*f = ""
		return nil
	}
	*f = flexNumber(n.String())
	return nil
}

func (f flexNumber) String() string { return strings.TrimSpace(string(f)) }

func (f flexNumber) int64() int64 {
	s := f.String()
	if s == "" {
		return 0
	}
	if v, err := json.Number(s).Int64(); err == nil {
		return v
	}
	return 0
}

// bytes 解析容量字段；115 的 size 可能是浮点字符串（如 "1234.56"）。
func (f flexNumber) bytes() int64 {
	s := f.String()
	if s == "" {
		return 0
	}
	if v, err := json.Number(s).Int64(); err == nil {
		return v
	}
	fl, _, err := big.ParseFloat(s, 10, 256, big.ToZero)
	if err != nil {
		return 0
	}
	i, _ := fl.Int(nil)
	if i == nil || !i.IsInt64() {
		return 0
	}
	return i.Int64()
}
