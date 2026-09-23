# User Instruction Memory

This file records user instructions, preferences, and teachings for reference in future interactions.

## Format

### User Instruction Entry
User instruction entries should follow this format:

[User Instruction Summary]
- Date: [YYYY-MM-DD]
- Context: [Mentioned scenario or time]
- Instructions:
  - [Content of user teaching or instruction, described line by line]

### Project Knowledge Entry
Entries discovered by the Agent during task execution should follow this format:

[Project Knowledge Summary]
- Date: [YYYY-MM-DD]
- Context: Discovered by Agent while performing [specific task description]
- Category: [Operations & Deployment|Build Methods|Testing Methods|Troubleshooting & Debugging|Workflow & Collaboration|Environment Configuration]
- Instructions:
  - [Specific knowledge points, described line by line]

## Deduplication Strategy
- Before adding a new entry, check for similar or identical instructions.
- If a duplicate is found, skip the new entry or merge it with the existing one.
- When merging, update the context or date information.
- This helps avoid redundant entries and keeps the memory file tidy.

## Entries

[Project Knowledge Summary]
- Date: 2026-09-23
- Context: Discovered by Agent while performing "sync upstream Ponphil/LitePan while preserving local STRM rating/delete features"
- Category: Workflow & Collaboration
- Instructions:
  - This repo (alanxie1999/LitePan fork) shares full commit ancestry with upstream github.com/Ponphil/LitePan; the fork's main was a snapshot on top of upstream commit 46a0a89 ("播放诊断及直读文案"), and branch history beyond that was hidden by a shallow boundary until `git fetch --unshallow upstream`.
  - To sync upstream: `git remote add upstream https://github.com/Ponphil/LitePan`, `git fetch --unshallow upstream`, then `git merge upstream/main` from main; the previous sync used a real merge commit (279ddcb) with base 46a0a89.
  - The fork must always keep 4 local features across merges: (1) scrape rating written into NFO, (2) poster wall shows rating, (3) poster-wall delete cascades to cloud source files, (4) rating sort. These live in internal/strmscrape (delete.go, item.go, nfo.go, match.go, index.go, types.go, service.go), web/src/components/admin/StrmScrapePanel.vue, web/src/api/strmScrape.ts, internal/api/router.go + strm_scrape.go, internal/app/wire_services.go.
  - Upstream renamed proxybase.ParseLitePanSTRMURL to strm.ParsePlayReference (returns strm.PlayReference). Keep delete.go's parseStrmCloudFile on the new API.
  - Local infra kept over upstream: drivers/115 tracked (do not restore the `drivers/115/` line in .gitignore), README banner for the 4 features, docker-compose image tags point to ajun59420/litepan, internal/api/web embedded assets are rebuilt from web/src before syncing.
  - STRM index schema is version "3" (rating column) in internal/strmscrape/index.go; older indexes are dropped and rebuilt automatically by ensureIndexLocked.