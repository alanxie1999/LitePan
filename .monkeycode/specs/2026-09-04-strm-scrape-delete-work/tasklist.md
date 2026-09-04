# 需求实施计划

- [x] 1. 后端删除类型、索引删除与 `.strm` 解析
  - 在 `internal/strmscrape/types.go` 增加 `DeleteItemRequest` / `DeleteItemResult`
  - 在 `index.go` 增加按 `id` 删除条目
  - 从作品 `.strm` 解析与任务账号一致的网盘 `file_id`
  - 覆盖需求 3.1 / 3.6 / 3.7、设计 Data Models
  - [x]* 1.1 编写 `.strm` 解析与账号过滤单元测试

- [x] 2. 实现本地作品删除与网盘目录定位
  - 目录作品删除作品根；扁平作品只删对应 `.strm` 与刮削元数据
  - 沿 `task.ParentID` + `rel_dir` 定位网盘作品目录，源根降级为删文件
  - 覆盖需求 2.1 / 2.2 / 3.4 / 3.5、正确性约束
  - [x]* 2.1 编写扁平/目录本地删除与源根保护测试

- [x] 3. 实现 `Service.DeleteItem` 并装配依赖
  - 注入 `file.Service`，占用刮削锁与 STRM 文件操作锁
  - 先解析 `.strm`，再删本地、索引，最后按勾选删网盘
  - 覆盖需求 2.3 / 2.4 / 3.2 / 4.1 / 4.3 / 4.4 / 5
  - [ ]* 3.1 编写索引缺失拒绝删除、网盘失败仍返回本地成功的测试

- [x] 4. 检查点 - 确保后端核心路径可编译
  - `go test ./internal/strmscrape/` 已通过

- [x] 5. 增加删除 API 与海报墙入口
  - 路由 `POST /admin/strm-scrape/delete-item`
  - `strmScrape.ts` 增加 `deleteStrmScrapeItem`
  - 海报墙卡片增加「删除」，确认框默认勾选同时删除网盘
  - 成功后移除卡片并更新统计；网盘失败展示警告
  - 覆盖需求 1、2.5、4.2 / 4.3
