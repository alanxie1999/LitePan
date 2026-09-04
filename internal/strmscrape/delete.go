package strmscrape

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/proxybase"
	"litepan/internal/strm"
)

const (
	cloudTargetFolder = "folder"
	cloudTargetFiles  = "files"
)

func collectCloudFileIDs(taskAccountID int64, g workGroup) []string {
	seen := make(map[string]struct{}, len(g.entries))
	out := make([]string, 0, len(g.entries))
	for _, e := range g.entries {
		accountID, fileID, ok := parseStrmCloudFile(e.absPath)
		if !ok || accountID != taskAccountID || fileID == "" {
			continue
		}
		if _, dup := seen[fileID]; dup {
			continue
		}
		seen[fileID] = struct{}{}
		out = append(out, fileID)
	}
	return out
}

func parseStrmCloudFile(path string) (int64, string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		accountID, fileID, ok := proxybase.ParseLitePanSTRMURL(line)
		if ok {
			return accountID, fileID, true
		}
	}
	return 0, "", false
}

func deleteLocalWork(root string, g workGroup) error {
	if g.flatFile != "" {
		return deleteFlatWork(root, g)
	}
	if strings.TrimSpace(g.absDir) == "" || sameFilePath(g.absDir, root) {
		return domain.Errorf(domain.CodeValidation, "拒绝删除 STRM 库根目录")
	}
	if !isInside(root, g.absDir) {
		return domain.Errorf(domain.CodeValidation, "作品路径越出当前 STRM 输出目录")
	}
	if err := os.RemoveAll(g.absDir); err != nil && !os.IsNotExist(err) {
		return domain.Errorf(domain.CodeDriverError, "删除本地 STRM 目录：%v", err)
	}
	return nil
}

func deleteFlatWork(root string, g workGroup) error {
	if !isInside(root, g.flatFile) && !sameFilePath(filepath.Dir(g.flatFile), root) {
		return domain.Errorf(domain.CodeValidation, "作品路径越出当前 STRM 输出目录")
	}
	if err := clearFlatScrapedMetadata(g); err != nil && !os.IsNotExist(err) {
		return domain.Errorf(domain.CodeDriverError, "删除本地刮削元数据：%v", err)
	}
	for _, name := range []string{pendingMarkerPath(g), manualCompleteMarkerPath(g)} {
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			return domain.Errorf(domain.CodeDriverError, "删除本地标记：%v", err)
		}
	}
	if err := os.Remove(g.flatFile); err != nil && !os.IsNotExist(err) {
		return domain.Errorf(domain.CodeDriverError, "删除本地 STRM 文件：%v", err)
	}
	return nil
}

func (s *Service) resolveCloudFolderID(ctx context.Context, task *domain.StrmTask, relDir string) (string, bool) {
	if s.files == nil || task == nil {
		return "", false
	}
	parentID := strings.TrimSpace(task.ParentID)
	if parentID == "" {
		return "", false
	}
	segs := strm.SafeDirSegments(relDir)
	if len(segs) == 0 {
		return "", false
	}
	current := parentID
	for _, want := range segs {
		items, err := s.files.List(ctx, task.AccountID, current, false)
		if err != nil {
			return "", false
		}
		next := ""
		for _, item := range items {
			if !item.IsDir {
				continue
			}
			if strm.SafeName(item.Name) == want {
				next = item.ID
				break
			}
		}
		if next == "" {
			return "", false
		}
		current = next
	}
	if current == "" || current == parentID {
		return "", false
	}
	return current, true
}

func (s *Service) deleteCloudWork(ctx context.Context, task *domain.StrmTask, g workGroup, fileIDs []string) (target string, err error) {
	if s.files == nil {
		return "", domain.Errorf(domain.CodeInternal, "文件服务未装配")
	}
	if len(fileIDs) == 0 {
		return cloudTargetFiles, domain.Errorf(domain.CodeValidation, "没有可删除的网盘文件")
	}
	if g.flatFile == "" {
		if folderID, ok := s.resolveCloudFolderID(ctx, task, g.relKey); ok {
			if err := s.files.DeleteFiles(ctx, task.AccountID, []string{folderID}, ""); err == nil {
				return cloudTargetFolder, nil
			}
		}
	}
	if err := s.files.DeleteFiles(ctx, task.AccountID, fileIDs, ""); err != nil {
		return cloudTargetFiles, err
	}
	return cloudTargetFiles, nil
}

func (s *Service) DeleteItem(ctx context.Context, req DeleteItemRequest) (*DeleteItemResult, error) {
	req.ItemID = strings.TrimSpace(req.ItemID)
	if req.StrmTaskID <= 0 || req.ItemID == "" {
		return nil, domain.Errorf(domain.CodeValidation, "参数不完整")
	}
	if !s.operationMu.TryLock() {
		return nil, domain.Errorf(domain.CodeValidation, "刮削任务进行中")
	}
	defer s.operationMu.Unlock()

	releaseFiles := func() {}
	if s.strm != nil {
		var ok bool
		releaseFiles, ok = s.strm.TryBeginTaskFileOperation(req.StrmTaskID)
		if !ok {
			return nil, domain.Errorf(domain.CodeValidation, "该 STRM 任务正在处理本地文件，请稍后再试")
		}
	}
	defer releaseFiles()

	task, root, err := s.resolveTask(ctx, req.StrmTaskID)
	if err != nil {
		return nil, err
	}
	g, findErr := findWorkByID(root, req.ItemID)
	if findErr != nil {
		if !s.indexHasItem(req.StrmTaskID, req.ItemID) {
			return nil, findErr
		}
		if err := s.deleteIndexItem(req.StrmTaskID, req.ItemID); err != nil {
			return nil, err
		}
		result := &DeleteItemResult{
			ItemID:         req.ItemID,
			LocalDeleted:   true,
			CloudRequested: req.DeleteCloud,
		}
		if req.DeleteCloud {
			result.CloudTarget = cloudTargetFiles
			result.CloudError = "本地 STRM 已不存在，无法解析网盘文件"
		}
		return result, nil
	}
	if g.flatFile == "" {
		if sameFilePath(g.absDir, root) || !isInside(root, g.absDir) {
			return nil, domain.Errorf(domain.CodeValidation, "作品路径越出当前 STRM 输出目录")
		}
	} else if !isInside(root, g.flatFile) && !sameFilePath(filepath.Dir(g.flatFile), root) {
		return nil, domain.Errorf(domain.CodeValidation, "作品路径越出当前 STRM 输出目录")
	}

	fileIDs := collectCloudFileIDs(task.AccountID, g)
	if err := deleteLocalWork(root, g); err != nil {
		return nil, err
	}
	if err := s.deleteIndexItem(req.StrmTaskID, req.ItemID); err != nil {
		s.log.Warn("刮削索引删除失败", "task_id", req.StrmTaskID, "item_id", req.ItemID, "err", err)
	}

	result := &DeleteItemResult{
		ItemID:         req.ItemID,
		LocalDeleted:   true,
		CloudRequested: req.DeleteCloud,
	}
	if !req.DeleteCloud {
		return result, nil
	}
	target, cloudErr := s.deleteCloudWork(ctx, task, g, fileIDs)
	result.CloudTarget = target
	if cloudErr != nil {
		result.CloudError = cloudErr.Error()
		return result, nil
	}
	result.CloudDeleted = true
	return result, nil
}
