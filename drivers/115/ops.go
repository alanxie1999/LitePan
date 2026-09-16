package pan115

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"litepan/drivers/115/crypto/m115"
	"litepan/internal/domain"
	"litepan/internal/driver"
)

func normalizeIDs(fileIDs []string) []string {
	out := make([]string, 0, len(fileIDs))
	for _, id := range fileIDs {
		if id = strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// indexedForm 生成 115 批量接口使用的 fid[0]=.. 形式表单。
func indexedForm(key string, ids []string) url.Values {
	form := url.Values{}
	for i, id := range ids {
		form.Set(fmt.Sprintf("%s[%d]", key, i), id)
	}
	return form
}

// lookupEntry 通过 file_id 获取文件条目（含 pick_code、名称、大小）。
func (d *Driver) lookupEntry(ctx context.Context, fileID string) (fileEntry, error) {
	query := url.Values{"file_id": {fileID}}
	var entries []fileEntry
	if err := d.apiRequest(ctx, http.MethodGet, webAPIHost+"/files/get_info", query, nil, &entries); err != nil {
		return fileEntry{}, err
	}
	if len(entries) == 0 {
		return fileEntry{}, domain.Errf(domain.CodeNotFound)
	}
	d.rememberPickCode(entries[0])
	return entries[0], nil
}

// ResolveDownload 解析下载直链。115 直链与请求 UA 绑定，默认走本机代理并回带 Cookie/UA。
func (d *Driver) ResolveDownload(ctx context.Context, req driver.DownloadRequest) (*domain.DownloadInfo, error) {
	fileID := strings.TrimSpace(req.FileID)
	if fileID == "" {
		return nil, domain.Errorf(domain.CodeValidation, "file_id 不能为空")
	}

	entry, err := d.lookupEntry(ctx, fileID)
	if err != nil {
		return nil, err
	}
	pickCode := entry.pickCode()
	if pickCode == "" {
		pickCode = d.cachedPickCode(fileID)
	}
	if pickCode == "" {
		name := entry.entryName()
		if name == "" {
			name = fileID
		}
		return nil, domain.Errorf(domain.CodeDriverError, "文件 %s 缺少 pick_code，无法获取下载链接", name)
	}

	ua := strings.TrimSpace(req.UA)
	if ua == "" {
		ua = webUA
	}

	downloadURL, size, name, err := d.requestDownloadURL(ctx, pickCode, fileID, ua)
	if err != nil {
		return nil, err
	}
	if size <= 0 {
		size = entry.entrySize()
	}
	if name == "" {
		name = entry.entryName()
	}

	info := &domain.DownloadInfo{
		URL:         downloadURL,
		Headers:     buildDownloadHeaders(ua, d.currentCookie()),
		Size:        size,
		FileName:    name,
		ChunkSize:   downloadChunkSize,
		Concurrency: downloadConcurrent,
	}
	if strings.EqualFold(strings.TrimSpace(d.add.DownloadMode), "redirect") {
		info.Mode = domain.DownloadRedirect
		return info, nil
	}
	info.Mode = domain.DownloadProxy
	info.ForceProxy = true
	info.Expiration = downloadLinkTTL
	return info, nil
}

// requestDownloadURL 调用 chrome/downurl 接口，用 m115 加密请求、解密响应。
func (d *Driver) requestDownloadURL(ctx context.Context, pickCode, fileID, ua string) (string, int64, string, error) {
	key := m115.GenerateKey()
	payload, err := json.Marshal(map[string]string{"pickcode": pickCode})
	if err != nil {
		return "", 0, "", domain.Wrap(domain.CodeInternal, err)
	}
	form := url.Values{"data": {m115.Encode(payload, key)}}
	query := url.Values{"t": {nowSeconds()}}

	var encoded string
	if err := d.apiRequestMode(ctx, http.MethodPost, proAPIHost+"/app/chrome/downurl", query, form, map[string]string{
		"User-Agent": ua,
		"Referer":    referer115,
	}, &encoded, false); err != nil {
		return "", 0, "", err
	}
	if strings.TrimSpace(encoded) == "" {
		return "", 0, "", domain.Errorf(domain.CodeDriverError, "115 未返回下载数据")
	}
	decoded, err := m115.Decode(encoded, key)
	if err != nil {
		return "", 0, "", domain.Wrap(domain.CodeDriverError, err)
	}
	var byID map[string]downloadEntry
	if err := json.Unmarshal(decoded, &byID); err != nil {
		return "", 0, "", domain.Errorf(domain.CodeDriverError, "115 下载数据解析失败：%v", err)
	}

	var chosen downloadEntry
	found := false
	if e, ok := byID[fileID]; ok {
		chosen, found = e, true
	} else {
		for _, e := range byID {
			chosen, found = e, true
			break
		}
	}
	if !found || chosen.url() == "" {
		return "", 0, "", domain.Errorf(domain.CodeDriverError, "115 未返回有效下载链接")
	}
	return chosen.url(), chosen.FileSize.int64(), strings.TrimSpace(chosen.FileName), nil
}

func buildDownloadHeaders(ua, cookie string) http.Header {
	h := http.Header{}
	h.Set("User-Agent", ua)
	h.Set("Accept", "*/*")
	h.Set("Accept-Language", "zh-CN,zh;q=0.9")
	h.Set("Referer", referer115)
	if strings.TrimSpace(cookie) != "" {
		h.Set("Cookie", cookie)
	}
	return h
}

// DeleteFiles 删除文件：115 Web 端删除即移入回收站（可恢复），不做不可逆的永久删除。
func (d *Driver) DeleteFiles(ctx context.Context, fileIDs []string) error {
	ids := normalizeIDs(fileIDs)
	if len(ids) == 0 {
		return nil
	}
	return d.apiRequest(ctx, http.MethodPost, webAPIHost+"/rb/delete", nil, indexedForm("fid", ids), nil)
}

func (d *Driver) MoveFiles(ctx context.Context, fileIDs []string, targetParentID, _ string) error {
	ids := normalizeIDs(fileIDs)
	if len(ids) == 0 {
		return nil
	}
	form := indexedForm("fid", ids)
	form.Set("pid", d.normalizeParent(targetParentID))
	return d.apiRequest(ctx, http.MethodPost, webAPIHost+"/files/move", nil, form, nil)
}

func (d *Driver) CopyFiles(ctx context.Context, fileIDs []string, targetParentID string) error {
	ids := normalizeIDs(fileIDs)
	if len(ids) == 0 {
		return nil
	}
	form := indexedForm("fid", ids)
	form.Set("pid", d.normalizeParent(targetParentID))
	return d.apiRequest(ctx, http.MethodPost, webAPIHost+"/files/copy", nil, form, nil)
}

func (d *Driver) RenameFile(ctx context.Context, fileID, newName string) error {
	id := strings.TrimSpace(fileID)
	name := strings.TrimSpace(newName)
	if id == "" {
		return domain.Errorf(domain.CodeValidation, "file_id 不能为空")
	}
	if name == "" {
		return domain.Errorf(domain.CodeValidation, "新名称不能为空")
	}
	form := url.Values{
		"fid":                                 {id},
		"file_name":                           {name},
		fmt.Sprintf("files_new_name[%s]", id): {name},
	}
	return d.apiRequest(ctx, http.MethodPost, webAPIHost+"/files/batch_rename", nil, form, nil)
}

func (d *Driver) CreateFolder(ctx context.Context, parentID, name string) (*domain.FileItem, error) {
	folderName := strings.TrimSpace(name)
	if folderName == "" {
		return nil, domain.Errorf(domain.CodeValidation, "文件夹名称不能为空")
	}
	form := url.Values{
		"pid":   {d.normalizeParent(parentID)},
		"cname": {folderName},
	}
	var out mkdirResp
	if err := d.apiRequestFull(ctx, http.MethodPost, webAPIHost+"/files/add", nil, form, &out); err != nil {
		return nil, err
	}
	folderID := out.Cid.String()
	if folderID == "" {
		folderID = strings.TrimSpace(out.FileID)
	}
	return &domain.FileItem{
		ID:     folderID,
		Name:   folderName,
		IsDir:  true,
		IDKind: domain.IDStable,
	}, nil
}
