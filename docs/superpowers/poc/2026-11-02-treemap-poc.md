# PoC 记录（2026-11-02）：矩形树图两道门槛

- 探针：`tools/treemapprobe`（headless `ui.NewTester` 1400×900，`AnimationDuration`
  取负关闭过渡，每帧 `c.Invalidate()` 强制重建——量的是布局不是空闲帧）
- 命令：`go run ./tools/treemapprobe -repos <三个真仓> -tiles 50,600,6000 -frames 120`
- 上游：fork `onevra/source@…-a3bbb6f230ab` 的 `components/treemap`，**零改动**
- 机器：本机 darwin/arm64，go1.27.2

## PoC-12：规模门槛 —— 通过

| tiles | 首帧 | 中位 | p95 | max |
|---|---|---|---|---|
| 50 | 35.4 ms（含字体/主题冷启动） | 30 µs | 68 µs | 35.4 ms |
| 600 | 40 µs | 148 µs | 384 µs | 510 µs |
| 6000 | 242 µs | **1.187 ms** | 2.138 ms | 8.045 ms |

**判定**：6000 tile 的中位每帧 1.19 ms，是 60 fps 预算（16.7 ms）的 7%，也是
flow 在 600 节点实测 3.4 ms（切片 1 findings）的三分之一。**treemap 不是规模
路线上的瓶颈**，PoC-11 那个「数千节点」的门槛在分布视图下天然不成立。
失败分支（只渲染两层 / 树只到包层）都不需要启用。

注意 max=8 ms 出现在 6000 tile 档的个别帧——量级仍安全，但它出现在数据变化后的
第一帧，说明**重扫之后要先关动画**（`AnimationDuration` 传负值），这条已进
treemap 调研 §9-4。

## PoC-13：面积口径 —— 元素数口径不可接受，落 `File` 元素

| 仓 | 包 | 非测试 .go 文件 | 导出 struct/interface | **零元素的包** | 零元素的文件 | 占比 |
|---|---|---|---|---|---|---|
| OpenArchitect | 51 | 110 | 46 | **21** | 70 | **0.64** |
| OpenCourt | 21 | 52 | 84 | 3 | 17 | 0.33 |
| OpenLaw | 30 | 89 | 114 | **17** | 42 | 0.47 |

**判定**：以「元素数」当面积，**自家仓库 41% 的包（21/51）在图上面积为 0**，
组件会把这些节点**静默省略**（`Node` 注释：无有限正尺寸的节点 omitted）——
用户不会收到任何提示。OpenLaw 是 57%（17/30）。文件级更甚：64%/47% 的文件
不贡献任何元素。

这直接裁决了 `2026-11-02-repowise-gap-and-refactor-verdict.md` §6：**采纳 (a)**，
加一类 `File` 元素（ID `fil:<module>/<path>`）。它同时是 treemap 的面积单位、
git 层 `file_metrics` 的外键落点、和快照 diff 不再漏文件的根据。

## 两道门槛的合起来的意思

treemap 的规模成本不是问题，问题只有一个：面积挂什么。数据说**必须挂文件**，
而文件节点现在不存在。所以 treemap 的正确排期确认不变——**与切片 3 的 git 层
同期**，前置任务是 `File` 元素（一个 scanner 改动 + 一类 ID，schema 零改）。
