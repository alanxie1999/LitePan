package pan115

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/httpx"
)

// Driver 是 115 网盘 Cookie 驱动实例；Cookie 失效需重新扫码或抓取。
type Driver struct {
	add    Addition
	client *http.Client

	intervalGate driver.RequestIntervalGate
	persist      driver.AuthPersistFunc

	mu                sync.Mutex
	cookie            string
	lastCookieChanged bool

	pickMu sync.RWMutex
	pickBy map[string]string
}

var config = driver.Config{
	Name:                   "115",
	DisplayName:            "115网盘",
	Description:            "115网盘 Cookie 接入，支持扫码登录、文件管理与下载",
	CardTags:               []string{"扫码登录", "Cookie", "本机代理", "SHA1"},
	SortOrder:              3,
	AuthLabel:              "Cookie",
	CardColor:              "#22A7F0",
	CardLogo:               "/logos/115.png",
	DefaultRoot:            "0",
	AuthType:               driver.AuthCookie,
	HealthCheckInterval:    70 * time.Minute,
	SupportsAccountProfile: true,
	ProvideHashes:          []string{"sha1"},
}

func New() driver.Driver { return &Driver{} }

func init() { driver.Register(New) }

func (d *Driver) Config() driver.Config { return config }

func (d *Driver) GetAddition() any { return &d.add }

func (d *Driver) Init(ctx context.Context) error {
	if d.client == nil {
		d.client = httpx.NewClient(httpx.ClientOptions{Timeout: 30 * time.Second})
	}
	d.mu.Lock()
	if d.cookie == "" {
		d.cookie = strings.TrimSpace(d.add.Cookie)
	}
	empty := d.cookie == ""
	d.mu.Unlock()
	if empty {
		return domain.Errorf(domain.CodeValidation, "Cookie 不能为空")
	}
	return nil
}

func (d *Driver) Drop(context.Context) error {
	httpx.CloseClient(d.client)
	return nil
}

// Ping 通过 status 接口验证 Cookie 是否仍有效（不会踢掉其他登录设备）。
func (d *Driver) Ping(ctx context.Context) error {
	query := url.Values{"_": {nowMillis()}}
	return d.apiRequest(ctx, http.MethodGet, myHost+"/?ct=guide&ac=status", query, nil, nil)
}

func (d *Driver) ExplainConnectionError(technical string, saving bool) string {
	prefix := "添加失败"
	if saving {
		prefix = "保存失败"
	}
	lower := strings.ToLower(technical)
	switch {
	case strings.Contains(technical, "Cookie 不能为空"):
		return prefix + "：请填写 115 网盘 Cookie，或使用扫码登录"
	case strings.Contains(technical, "认证") ||
		strings.Contains(lower, "auth_expired") ||
		strings.Contains(lower, "permission_denied"):
		return prefix + "：115 Cookie 无效或已过期，请重新扫码登录或抓取完整 Cookie"
	default:
		return ""
	}
}

// ListFiles 列举目录；115 列表接口按 count 分页，show_dir=1 同时返回文件夹。
func (d *Driver) ListFiles(ctx context.Context, parentID string) ([]domain.FileItem, error) {
	parent := d.normalizeParent(parentID)
	var items []domain.FileItem
	offset := 0
	for {
		var page listPage
		if err := d.apiRequestFull(ctx, http.MethodGet, webAPIHost+"/files", listQuery(parent, offset, true, false), nil, &page); err != nil {
			return nil, err
		}
		if len(page.Data) == 0 {
			break
		}
		for _, e := range page.Data {
			d.rememberPickCode(e)
			items = append(items, e.toFileItem())
		}
		offset += len(page.Data)
		if total := page.Count.int64(); total > 0 && int64(offset) >= total {
			break
		}
		if len(page.Data) < listPageSize {
			break
		}
	}
	return items, nil
}

func (d *Driver) GetFileInfo(ctx context.Context, fileID string) (*domain.FileItem, error) {
	id := strings.TrimSpace(fileID)
	root := d.rootID()
	if id == "" || id == "0" || id == "/" || id == "root" || id == root {
		return &domain.FileItem{
			ID:     root,
			Name:   "根目录",
			IsDir:  true,
			IDKind: domain.IDStable,
		}, nil
	}
	entry, err := d.lookupEntry(ctx, id)
	if err != nil {
		return nil, err
	}
	if entry.entryID() == "" {
		return nil, domain.Errf(domain.CodeNotFound)
	}
	item := entry.toFileItem()
	return &item, nil
}

func (d *Driver) rememberPickCode(e fileEntry) {
	id := e.entryID()
	pc := e.pickCode()
	if id == "" || pc == "" {
		return
	}
	d.pickMu.Lock()
	if d.pickBy == nil {
		d.pickBy = make(map[string]string)
	}
	d.pickBy[id] = pc
	d.pickMu.Unlock()
}

func (d *Driver) cachedPickCode(fileID string) string {
	id := strings.TrimSpace(fileID)
	if id == "" {
		return ""
	}
	d.pickMu.RLock()
	pc := d.pickBy[id]
	d.pickMu.RUnlock()
	return pc
}

var (
	_ driver.Driver                   = (*Driver)(nil)
	_ driver.InfoGetter               = (*Driver)(nil)
	_ driver.Downloader               = (*Driver)(nil)
	_ driver.Deleter                  = (*Driver)(nil)
	_ driver.Mover                    = (*Driver)(nil)
	_ driver.Copier                   = (*Driver)(nil)
	_ driver.Renamer                  = (*Driver)(nil)
	_ driver.FolderCreator            = (*Driver)(nil)
	_ driver.AuthRefresher            = (*Driver)(nil)
	_ driver.AuthCredentialConsumer   = (*Driver)(nil)
	_ driver.AuthPersistConsumer      = (*Driver)(nil)
	_ driver.AccountProfileProvider   = (*Driver)(nil)
	_ driver.ConnectionErrorExplainer = (*Driver)(nil)
	_ driver.RequestIntervalConsumer  = (*Driver)(nil)
	_ driver.QRLoginProvider          = (*Driver)(nil)
	_ driver.FullListLister           = (*Driver)(nil)
)
