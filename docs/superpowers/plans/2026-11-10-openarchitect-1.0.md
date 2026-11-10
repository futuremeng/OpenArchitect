# OpenArchitect 1.0 发布计划：多仓观测 + 自动检入 + 果实角标

- 状态：执行中
- 日期：2026-11-10
- 范围（用户已拍板"推荐档"）：存量（切片1全量+切片2核心）+ 生长A（forest-local）
  + 果实C-lite（归因+角标）+ 发布工程。生长B/D/E 与切片7不进1.0。
- 上游设定：`2026-11-10-project-setup.md`；设计：`specs/2026-11-10-openarchitect-growth-model-design.md`

## 设计决策（定死，不再讨论）

1. **schema v2 用旁表**：`Migrate` 的契约是每条语句可重放（migrate.go:9-10），
   SQLite `ALTER TABLE ADD COLUMN` 重放必炸。故 v2 = 新表 `repo_sync`
   （repo_id PK, remote_url, sync_state, last_head）+ 新表 `commits`
   （repo_id+hash PK, author, author_email, date, message, agent_tag, files_json）
   + 索引。`repos` 表与 `UpsertRepo` 一字不改。
2. **scanner 不碰 git**：`scanner.Run` 签名不变；forest 在 `Run` 之后调
   `store.SetSnapshotCommit(snapshotID, hash, branch)` 回填预留两列。
   I2′ 的"自动只有 fetch+scan"由此在结构上成立。
3. **head 游标在 repo_sync.last_head**：首次无快照时回退看最新快照的 commit_hash。
   不触及源文件时只推进 last_head、只记 commits，不重扫（风险登记③内建）。
4. **归因纯函数**：`internal/growth.Attribution(author, message) string`，
   规则表是包内数据（`AgentTrailers`），1.0 不做 TOML。
5. **git 一律 os/exec**（I3 同风格）：`internal/forest/gitcmd` 是唯一出口。

## 任务清单

- [x] T1 schema v2 + migrate 测试（重放安全：跑两遍 Migrate 幂等）
- [x] T2 model：`Repo` 加 RemoteURL/SyncState（join 而来）；`Commit` 新类型
- [x] T3 store：`repo_sync` 读写、`commits` 批量写/读、`SetSnapshotCommit`；各配测试
- [x] T4 growth：`Attribution` 纯函数 + 表驱动测试（Claude/Codex/OpenCode trailer）
- [x] T5 forest/gitcmd：IsRepo/Head/RemoteURL/Fetch/DiffFiles/Log + 真 git 仓库测试
- [x] T6 forest/discover：ScanRoot 遍历登记 + stale 判定 + 测试
- [x] T7 forest/sync：游标比较→commits 记账→闸门→按需 scan→回填 commit + 测试
- [x] T8 cmd/oac：`sync <root>` 子命令、`version`、usage 更新
- [x] T9 UI：仓库页同步状态徽标（new 灰/tracked 绿/stale 红 + remote 标记）+「同步全部」按钮（BeginSync→forest.Sync，与扫描共用后台槽）；图页顶栏果实图例（红/橙计数）；nodeCard 果实圆点（full/icon 档行尾、dot 档直接染圆点色）；详情面板果实行（agent、短 hash、日期、subject）。数据在 Reload 预聚合（buildFruits 按目录落位），每帧零查询。600 卡 A/B 基准：1.89 ms vs 1.86 ms，角标成本在噪声内，10% 闸门通过，无需降级为纯点色
- [x] T10 发布工程：版本号常量✅ CHANGELOG✅ README✅；CI R3 守卫✅（forest 不得直接 import net/http、net/url、go-git、internal/agent、internal/ui；将来 source 适配器例外只开 internal/forest/source/**）
- [x] T11 追加批次（折入 1.0）：图例逐 agent 身份色、卡内果实条（分级渲染）、时间轴滑杆、worktree 语义（发现层跳过 linked worktree/submodule）、工具链定位文档（paperclip/Orca 配合四原则）

## 验收（1.0 DoD）

1. 指定 ScanRoot，`oac sync <root>`：新仓自动登记、有 commit 漂移的仓自动重扫，
   新快照 `commit_hash` 非空；ledger 有行。
2. 带 `Co-authored-by: Claude` 的 commit 进 `commits` 表且 `agent_tag='agent:claude'`。
3. 只改 README 的 commit：commits 记账、不触发重扫（闸门生效，测试钉死）。
4. `go vet ./... && go test ./...` 全绿；R1/R2/R3 守卫绿。
5. 老库（v1）打开自动升 v2，重放两遍无错。

## 验收执行记录（2026-11-10）

- DoD1 真机冒烟：`oac sync` 发现 42 仓；OpenArchitect 快照绑定 f865f47c/main；第二轮全 clean。
- DoD2 真仓归因：agent:opencode 40 / mixed 420 / human 5817。
- DoD3 测试钉死（TestSyncScanGateAndCursor 第 2 轮）。DoD4 vet+test 全绿。DoD5 迁移重放绿。
- 冒烟发现并修复：无 adapter 语言不失败、推进游标、note 说明。
