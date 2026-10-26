# 观察案例库（2026-10-26）：VS Code 工作区全量盘点 + 三仓扫描实录

后续切片的真实观测对象。来源：VS Code `storage.json` 的 `openedWindows`
（12 个独立文件夹 + `one.code-workspace` 的 8 个文件夹，共 20 个项目）。

## 盘点结果

| 项目 | 语言 | 扫描状态 |
|---|---|---|
| github/futuremeng/OpenArchitect | Go | ✅ 18 元素 / 46 关系（0.8 s） |
| github/futuremeng/OpenCourt | Go | ✅ 68 元素 / 146 关系（2.7 s） |
| github/futuremeng/OpenLaw | Go | ✅ 52 元素 / 118 关系（2.4 s） |
| github/narrativeos/LangFixer | Rust | ⛔ 切片 5 |
| github/narrativeos/narrative-operator-MinerU | Python | ⛔ 切片 5 |
| github/narrativeos/narrative-operator-MinerU-Popo | Python | ⛔ 切片 5 |
| github/narrativeos/narrative-operator-nlp | Python | ⛔ 切片 5 |
| github/narrativeos/Nominatim | C++/C | ⛔ 切片 5 |
| github/narrativeos/TraceView | .NET (C#) | ⛔ 切片 5 |
| github/narrativeos/GeoLibre | Python | ⛔ 切片 5 |
| github/narrativeos/TraceLink | Rust | ⛔ 切片 5 |
| github/futuremeng/LocalCases | Rust | ⛔ 切片 5 |
| dnmp/www/one/one-api-plus | PHP (Laravel) | ⛔ 切片 5 |
| dnmp/www/one/one-portal-plus | TS/前端 | ⛔ 切片 5 |
| dnmp/www/one/one-monorepo | TS/前端 | ⛔ 切片 5 |
| dnmp/www/one/one-services | 混合（docker/compose） | ⛔ 切片 5 |
| dnmp/www/one/one-metadata | 文档仓 | ⛔ 切片 5 |
| dnmp/www/one/one-portal | JS/前端 | ⛔ 切片 5 |
| dnmp/www/one/one-template-calligraphy | TS/前端 | ⛔ 切片 5 |
| dnmp/www/one/one-vision | Python | ⛔ 切片 5 |

语言分布：Go 3 · Rust 3 · Python 5 · TS/JS 4 · PHP 1 · C++ 1 · C# 1 · 混合/文档 2。
**切片 5 的 tree-sitter sidecar 不是假想需求，是 17/20 的现实需求**——这份
分布就是切片 5 spec 的动机数据。

## 真实跨仓关系（PoC-8 的活体版本）

`OpenCourt/go.mod`：`require github.com/futuremeng/OpenLaw` +
`replace github.com/futuremeng/OpenLaw => ../OpenLaw`。
切片 2 spec 设计的双仓 fixture 场景在观察库里天然存在，PoC-8 可直接用真仓。

## 关键发现：扫描器把跨仓边在落库前就丢了（→ 切片 2 spec §7 已修正）

实测：OpenCourt 快照中 `to_id` 指向 OpenLaw 元素 ID 的边 = **0 条**
（`arch_relationships` 悬空边计数查询）。根因：`language/go.go` 的
`ownPackage(modulePath, ...)` 过滤把一切非本模块端点丢弃。
切片 2 spec 原稿 §7"单仓视图里被过滤的外部边在合并视图里复活"**不成立**——
库里根本没有这些边可复活。schema 侧无阻碍（`from_id/to_id` 无 FK，悬空边
可存），要改的是扫描器。spec 已按实测修正。

## 各仓 load-bearing（直接入度，calls+depends_on）

- OpenCourt：`Citation` 8 · `DocumentText` 8 · `Fact` 8 · `BrowserPlugin` 4
- OpenLaw：`store.DB` **20**（全库最高）· `RobotsCache` 4 · `Article` 3
- OpenArchitect：`model.Element` 6 · `store.DB` 4 · `graph.Graph` 4

影响分析示例（真数据）：`oac impact cmp:…OpenLaw/store.DB --depth 2` =
**上游 15（10 个 depth-1 calls + 5 个 depth-2 depends_on）/ 下游 0**——
OpenLaw 的重构震源，切片 2 diff/历史的第一观察对象。

## GUI 现状限制（观察时注意）

切片 1 GUI 只显示**全局最新快照**（`app.go:33-42` 跨仓取 max snapshot id），
此刻打开应用看到的是 OpenArchitect 自己的 18 节点图。看 OpenCourt/OpenLaw
用 CLI（`oac graph <repo-id>` / `impact`）；仓库过滤器是切片 2 第一优先任务。
