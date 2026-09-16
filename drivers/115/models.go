package pan115

import (
	"encoding/json"
	"strings"
	"time"

	"litepan/internal/domain"
)

// fileEntry 覆盖 115 Web 与 Open API 两种字段风格，优先使用短字段（Web）。
type fileEntry struct {
	Fid          string     `json:"fid"`
	FileID       string     `json:"file_id"`
	Cid          flexNumber `json:"cid"`
	Pid          string     `json:"pid"`
	Fn           string     `json:"fn"`
	N            string     `json:"n"`
	FileName     string     `json:"file_name"`
	Fc           flexNumber `json:"fc"`
	FileCategory flexNumber `json:"file_category"`
	Aid          flexNumber `json:"aid"`
	Sha          string     `json:"sha"`
	Sha1         string     `json:"sha1"`
	Pc           string     `json:"pc"`
	PickCode     string     `json:"pick_code"`
	S            flexNumber `json:"s"`
	Fs           flexNumber `json:"fs"`
	Size         flexNumber `json:"size"`
	Upt          flexNumber `json:"upt"`
	Uet          flexNumber `json:"uet"`
	Uppt         flexNumber `json:"uppt"`
	T            flexNumber `json:"t"`
	U            string     `json:"u"`
	Thumb        string     `json:"thumb"`
	Thumbnail    string     `json:"thumbnail"`
}

func (e fileEntry) entryID() string {
	for _, s := range []string{e.Fid, e.FileID} {
		if v := strings.TrimSpace(s); v != "" {
			return v
		}
	}
	if v := e.Cid.String(); v != "" && v != "0" {
		return v
	}
	return ""
}

func (e fileEntry) entryName() string {
	for _, s := range []string{e.N, e.Fn, e.FileName} {
		if v := strings.TrimSpace(s); v != "" {
			return v
		}
	}
	return ""
}

func (e fileEntry) isDir() bool {
	if strings.TrimSpace(e.Fid) != "" || strings.TrimSpace(e.FileID) != "" {
		return false
	}
	if v := e.FileCategory.String(); v != "" {
		return v == "0"
	}
	if v := e.Fc.String(); v != "" {
		return v == "0"
	}
	return true
}

func (e fileEntry) entrySize() int64 {
	for _, n := range []flexNumber{e.S, e.Fs, e.Size} {
		if v := n.int64(); v > 0 {
			return v
		}
	}
	return 0
}

func (e fileEntry) pickCode() string {
	for _, s := range []string{e.Pc, e.PickCode} {
		if v := strings.TrimSpace(s); v != "" {
			return v
		}
	}
	return ""
}

func (e fileEntry) sha1() string {
	for _, s := range []string{e.Sha, e.Sha1} {
		if v := strings.TrimSpace(s); v != "" {
			return v
		}
	}
	return ""
}

// parentID 返回条目的父目录 ID：文件用 cid（Web）或 pid（Open），目录用 pid。
func (e fileEntry) parentID() string {
	if !e.isDir() {
		if v := e.Cid.String(); v != "" {
			return v
		}
	}
	return strings.TrimSpace(e.Pid)
}

func (e fileEntry) thumb() string {
	for _, s := range []string{e.U, e.Thumb, e.Thumbnail} {
		if v := strings.TrimSpace(s); v != "" {
			return v
		}
	}
	return ""
}

func (e fileEntry) modUnix() int64 {
	for _, n := range []flexNumber{e.Uppt, e.Upt, e.Uet, e.T} {
		if v := n.int64(); v > 0 {
			return v
		}
	}
	if ts := parseTimeText(e.T.String()); !ts.IsZero() {
		return ts.Unix()
	}
	return 0
}

func (e fileEntry) modTime() time.Time {
	if v := e.modUnix(); v > 0 {
		return time.Unix(v, 0)
	}
	return time.Time{}
}

func (e fileEntry) toFileItem() domain.FileItem {
	item := domain.FileItem{
		ID:      e.entryID(),
		Name:    e.entryName(),
		Size:    e.entrySize(),
		IsDir:   e.isDir(),
		ModTime: e.modTime(),
		Thumb:   e.thumb(),
		IDKind:  domain.IDStable,
	}
	if sha := e.sha1(); sha != "" {
		item.Hash = map[domain.HashType]string{domain.HashSHA1: sha}
	}
	return item
}

func parseTimeText(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if ts, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return ts
		}
	}
	return time.Time{}
}

// listPage 是 /files 列表接口的顶层响应（count/offset 等与 data 同级）。
type listPage struct {
	Count  flexNumber  `json:"count"`
	Offset flexNumber  `json:"offset"`
	Limit  flexNumber  `json:"limit"`
	Cid    flexNumber  `json:"cid"`
	Data   []fileEntry `json:"data"`
}

type mkdirResp struct {
	Cid      flexNumber `json:"cid"`
	FileID   string     `json:"file_id"`
	FileName string     `json:"file_name"`
}

// downloadEntry 是解码后的下载数据单项。
type downloadEntry struct {
	FileName string          `json:"file_name"`
	FileSize flexNumber      `json:"file_size"`
	PickCode string          `json:"pick_code"`
	URLRaw   json.RawMessage `json:"url"`
}

func (e downloadEntry) url() string {
	if len(e.URLRaw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(e.URLRaw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var obj struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(e.URLRaw, &obj) == nil {
		return strings.TrimSpace(obj.URL)
	}
	return ""
}

// dirPathEntry 是目录父链的一段。
type dirPathEntry struct {
	FileID   flexNumber `json:"file_id"`
	FileName string     `json:"file_name"`
}

// dirInfoResp 兼容 category/get 的顶层字段与 data 包裹两种响应。
type dirInfoResp struct {
	FileName string         `json:"file_name"`
	Paths    []dirPathEntry `json:"paths"`
	Data     *struct {
		FileName string         `json:"file_name"`
		Paths    []dirPathEntry `json:"paths"`
	} `json:"data"`
}

func (r dirInfoResp) name() string {
	if r.Data != nil && strings.TrimSpace(r.Data.FileName) != "" {
		return strings.TrimSpace(r.Data.FileName)
	}
	return strings.TrimSpace(r.FileName)
}

func (r dirInfoResp) paths() []dirPathEntry {
	if r.Data != nil && len(r.Data.Paths) > 0 {
		return r.Data.Paths
	}
	return r.Paths
}
