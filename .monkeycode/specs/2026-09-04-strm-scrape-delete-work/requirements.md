# Requirements Document

## Introduction

在 STRM 刮削海报墙中，管理员需要按作品（电影或剧集）删除本地 STRM 目录，并可选联动删除对应的网盘源内容。当前海报墙仅支持匹配、重刮、标记完成/完结，缺少删除入口；STRM 任务删除只清理整库输出目录，不会按单部作品删除网盘内容。

本功能以海报墙卡片为入口，按作品粒度删除本地 STRM（含 nfo/海报等刮削元数据）。确认框默认勾选「同时删除网盘内容」；勾选时从 `.strm` 播放链接解析网盘 `file_id`，删除对应网盘作品目录或扁平文件。

## Glossary

- **海报墙**：STRM 刮削管理页中按作品展示的卡片列表，对应 `StrmScrapePanel`
- **作品**：一部电影或一部剧集，对应刮削索引中的一条 `Item`（电影文件夹、剧集根目录，或库根下扁平的单个 `.strm`）
- **STRM 文件**：本地 `.strm` 文本文件，内容为 LitePan 播放 URL，可解析出网盘账号 ID 与 `file_id`
- **刮削元数据**：作品目录下的 `.nfo`、海报/背景图等刮削产物
- **网盘作品目录**：该作品全部源视频所在的网盘目录树根；剧集包含季/集子目录及同目录字幕、nfo 等附属文件
- **扁平作品**：直接散落在 STRM 库根、各自独立成卡的单个 `.strm`
- **目录作品**：作品根目录下包含一个或多个 `.strm`（电影文件夹或剧集根目录）
- **STRM 任务源根**：当前 STRM 任务绑定的网盘 `ParentID`
- **系统**：LitePan 管理端及其 STRM 刮削服务

## Requirements

### Requirement 1: 海报墙删除入口

**User Story:** AS 管理员, I want 在海报墙卡片上删除一部电影或一部剧集, so that 我可以清掉不想保留的作品，而不必进入网盘文件列表逐个删除。

#### Acceptance Criteria

1. WHEN 管理员将鼠标悬停在海报墙卡片上，THE 系统 SHALL 在现有卡片操作区展示「删除」按钮。
2. WHEN 管理员点击「删除」，THE 系统 SHALL 弹出确认对话框，标题为「删除作品」，正文包含作品标题，并说明将删除本地 STRM 目录。
3. WHEN 确认对话框打开，THE 系统 SHALL 展示勾选项「同时删除网盘内容」，该勾选项默认勾选。
4. WHEN 管理员在确认对话框中取消操作，THE 系统 SHALL 保持该作品在海报墙中不变。
5. WHEN 该作品正在刮削，或当前 STRM 任务正在批量刮削，THE 系统 SHALL 禁用该卡片的「删除」按钮。
6. WHILE 删除请求进行中，THE 系统 SHALL 在该卡片上展示进行中状态，并禁止对该卡片再次发起删除。

### Requirement 2: 删除本地 STRM 与刮削索引

**User Story:** AS 管理员, I want 删除作品后本地 STRM 目录和海报墙条目一并消失, so that 媒体库和刮削结果保持一致。

#### Acceptance Criteria

1. WHEN 管理员确认删除一部目录作品，THE 系统 SHALL 删除该作品根目录及其全部子目录、`.strm`、刮削元数据和其它同目录文件。
2. WHEN 管理员确认删除一部扁平作品，THE 系统 SHALL 删除该 `.strm` 及其对应刮削元数据（如同 stem 的 `.nfo` 与海报图），并保留库根下其它作品的文件。
3. WHEN 本地 STRM 文件或目录已不存在，THE 系统 SHALL 继续删除刮削索引中的该条目。
4. WHEN 本地文件删除完成，THE 系统 SHALL 从该 STRM 任务的刮削索引中移除对应条目。
5. WHEN 删除成功，THE 系统 SHALL 从当前海报墙列表移除该卡片，并更新筛选统计中的条目计数。

### Requirement 3: 联动删除网盘内容

**User Story:** AS 管理员, I want 删除 STRM 作品时默认同时删掉网盘里的源内容, so that 本地库和网盘不会留下孤儿文件。

#### Acceptance Criteria

1. WHEN 管理员确认删除且勾选「同时删除网盘内容」，THE 系统 SHALL 读取该作品下全部 `.strm` 文件，解析其中的 LitePan 播放 URL，得到网盘账号 ID 与 `file_id`。
2. WHEN 管理员确认删除且取消勾选「同时删除网盘内容」，THE 系统 SHALL 只删除本地 STRM 与刮削索引，并保持网盘内容不变。
3. WHEN 解析得到的网盘账号 ID 与当前 STRM 任务账号 ID 一致，THE 系统 SHALL 将这些 `file_id` 作为该作品的网盘源文件。
4. WHEN 作品为目录作品，且全部同源网盘文件位于同一网盘作品目录内，且该目录不是 STRM 任务源根，THE 系统 SHALL 删除该网盘作品目录（含字幕、nfo、季目录等附属内容）。
5. WHEN 作品为扁平作品，或网盘作品目录判定为 STRM 任务源根，THE 系统 SHALL 仅删除该作品对应的网盘源文件，并保留同目录其它网盘文件。
6. WHEN 某条 `.strm` 无法解析出有效的网盘 `file_id`，THE 系统 SHALL 跳过该文件的网盘删除，并继续处理该作品的其余 `.strm`。
7. WHEN 解析到的网盘账号 ID 与当前 STRM 任务账号 ID 不一致，THE 系统 SHALL 跳过该文件的网盘删除。

### Requirement 4: 失败反馈与部分成功

**User Story:** AS 管理员, I want 删除失败时看到明确原因, so that 我知道本地和网盘分别删到了哪一步。

#### Acceptance Criteria

1. WHEN 本地 STRM 删除失败，THE 系统 SHALL 向管理员展示失败原因，并保持该作品仍在海报墙中。
2. WHEN 本地删除成功且（未勾选网盘删除，或网盘删除全部成功），THE 系统 SHALL 向管理员展示删除成功提示。
3. WHEN 本地删除成功且至少一条网盘删除失败，THE 系统 SHALL 仍移除海报墙卡片，并向管理员展示警告，说明本地已删除、网盘删除未全部完成。
4. IF 当前 STRM 任务正在执行文件操作或刮削，THE 系统 SHALL 拒绝删除并提示管理员稍后再试。

### Requirement 5: 权限与作用范围

**User Story:** AS 管理员, I want 删除能力仅作用于当前选中的 STRM 任务作品, so that 其它任务的库不会被误删。

#### Acceptance Criteria

1. WHEN 管理员发起删除，THE 系统 SHALL 仅处理当前选中 STRM 任务输出目录内、且条目 ID 匹配的那一部作品。
2. WHEN 请求中的 `item_id` 在当前任务刮削索引中不存在，THE 系统 SHALL 返回校验错误，并保持本地文件与网盘内容不变。
3. WHEN 解析出的作品路径位于当前任务输出目录之外，THE 系统 SHALL 拒绝删除。
