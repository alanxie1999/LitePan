package pan115

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/driver"
)

// ListAllFiles 使用 cur=0 让服务端递归展开 rootID 下全部文件，分页拉取。
// 该模式只返回文件，条目自带父目录 ID，由上层结合 pid→路径 缓存还原目录结构。
// 完整性策略：只以空页作为结束信号，不把 Count 或短页当作可靠终点。115 的 Count 可能因
// 厂商缓存或并发变更暂时偏小；若据此提前停止，会让上层把未扫到的目录误判为已删除。
func (d *Driver) ListAllFiles(ctx context.Context, rootID string) ([]driver.FullListEntry, error) {
	root := d.normalizeParent(rootID)
	var entries []driver.FullListEntry
	offset := 0
	seen := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var page listPage
		if err := d.apiRequestFull(ctx, http.MethodGet, webAPIHost+"/files", listQuery(root, offset, false, true), nil, &page); err != nil {
			return nil, err
		}
		if len(page.Data) == 0 {
			break
		}
		newIDs := 0
		for _, e := range page.Data {
			fileID := e.entryID()
			if fileID == "" {
				return nil, domain.Errorf(domain.CodeDriverError, "115 全量清单返回了缺少文件 ID 的条目，已停止扫描")
			}
			if _, dup := seen[fileID]; dup {
				continue
			}
			seen[fileID] = struct{}{}
			newIDs++
			d.rememberPickCode(e)
			entries = append(entries, driver.FullListEntry{
				FileID:   fileID,
				ParentID: e.parentID(),
				Name:     e.entryName(),
				Size:     e.entrySize(),
				Sha1:     e.sha1(),
				PickCode: e.pickCode(),
				MTime:    e.modUnix(),
			})
		}
		if newIDs == 0 {
			return nil, domain.Errorf(domain.CodeDriverError, "115 全量清单分页重复，已停止扫描以避免使用不完整结果")
		}
		offset += len(page.Data)
	}
	return entries, nil
}

// ResolveDirPath 通过 /category/get 的 paths 父链拼出目录完整路径。
// 该接口返回的 paths 不含目录自身，必须再追加目录名；账号根目录返回空串。
// 注意：这里只把账号根（0）视为根，配置的 RootFolderID 仍需返回它从账号根起的真实路径，
// 否则 STRM 扫描会因任务根前缀无法匹配而漏掉文件。
func (d *Driver) ResolveDirPath(ctx context.Context, dirID string) (string, error) {
	id := strings.TrimSpace(dirID)
	if id == "" || id == "0" || id == "/" || id == "root" {
		return "", nil
	}
	query := url.Values{"cid": {id}}
	var info dirInfoResp
	if err := d.apiRequestFull(ctx, http.MethodGet, webAPIHost+"/category/get", query, nil, &info); err != nil {
		return "", err
	}
	return buildDirPath(info.paths(), info.name()), nil
}

// buildDirPath 把 category/get 的父目录链（不含自身）与目录自身名称拼成完整路径。
// 父链中 file_id 为 0 的根段跳过；结果不含首尾斜杠。
func buildDirPath(paths []dirPathEntry, selfName string) string {
	segs := make([]string, 0, len(paths)+1)
	for _, p := range paths {
		if strings.TrimSpace(p.FileID.String()) == "0" {
			continue
		}
		if name := strings.TrimSpace(p.FileName); name != "" {
			segs = append(segs, name)
		}
	}
	if name := strings.TrimSpace(selfName); name != "" {
		segs = append(segs, name)
	}
	return strings.Join(segs, "/")
}

var _ driver.FullListLister = (*Driver)(nil)
