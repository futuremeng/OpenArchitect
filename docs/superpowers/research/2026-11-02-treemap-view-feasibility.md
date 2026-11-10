# 调研（2026-11-02）：矩形树图视图的可行性与面积语义

- 触发：`2026-11-02-repowise-gap-and-refactor-verdict.md` §7 之后追加的问题——
  「可视化是否该增加或改成矩形树图」
- 上游事实来源：fork `github.com/futuremeng/onevra/source@v0.2026.13-0.20261028081121-a3bbb6f230ab`
  （`go.mod:22` 的 `replace` 就指向它）的 `components/treemap/`，全部读源码
- 我方现状：`internal/core/graph/aggregate.go`、`internal/ui/{flow_data,node_card,state}.go`、
  `2026-10-27-graph-interaction-gap-review.md` 的 G1–G9

---

## 1. 结论

**增加，不替换。** 而且这件事的成本被严重高估了：**fork 里已经有一个完整的
squarified treemap 组件**，我们缺的只是「把图喂给它」那一个映射文件。零上游改动、
零 schema 改动、零新依赖。

更要紧的是它和我们已经认定的缺口**互为动机**：

- repowise 的 health map 就是矩形树图（面积=NLOC，颜色=lens 切 churn/health）。
  §4-1（git 层）与 §4-2（findings 层）落地的指标**在 flow 关系图上无处可挂**，
  矩形树图是唯一的自然挂载点。
- 反过来，treemap 顺手解掉交互复盘里四个我们短期内不打算单独修的缺口（§7）。

所以正确的排期不是「哪天有空加个图」，而是**切片 3（git 层）的展示面就是它**——
两层一起做，才有东西可看。

## 2. 上游事实（读源码，非 README 转述）

`components/treemap/`：`treemap.go` 524 行、`layout.go` 583 行、`draw.go` 201 行、
`treemap_test.go` 249 行（含契约测试）。公开面只有五个符号：

| 符号 | 位置 | 内容 |
|---|---|---|
| `Node` | `treemap.go:28` | `{ID, Label, Caption string; Value, Measure float64; Color ui.Color; Children []Node}` |
| `State` | `treemap.go:40` | `{Root, Active string}` + 私有 view；注释明写「State retains navigation between frames」，零值即就绪 |
| `Options` | `treemap.go:75` | `Label/Description/Height/HideHeader/FormatValue/MeasureLabel/MeasureDomain/FormatMeasure/Open(id)/Tip(c, Detail)/Empty/AnimationDuration` |
| `View` | `treemap.go:106` | `View(c *ui.Context, state *State, nodes []Node, opts Options) ui.Element` |
| `Path` | `treemap.go:91` | `Path(nodes, id) []Node`——从根到某节点的链，抽屉的 breadcrumb 直接可用 |

组件已经替我们做完的事（每条都在源码里，不是推测）：

1. **Bruls–Huizing–van Wijk squarify**（`layout.go:484`）：贪心最坏宽高比，
   纯 O(n log n) 排序 + 一次线性铺放，**没有迭代布局**。
2. **下钻与面包屑**：点击当前层 tile 下钻，`Breadcrumbs` 回退（`treemap.go:229`）；
   `State.Root` 是公开可读写的一个字符串。
3. **键盘是空间序**：方向键移动到「该方向上最近的 tile」（`nearest()`，
   `treemap.go:416`），Home/End 跳最大/最小，Enter/Space 下钻，Esc/Backspace 上层。
4. **按 tile 尺寸自适应文字**（`fitTexts`/`fit`，`layout.go:431-478`）：大叶子出
   大号数字 + caption，中等的出「标签 … 数值」一行，装不下就只出标签，两字符以下
   返回空串。**这就是 LOD，而且是自动的**。
5. **第二量着色**：`Measure` + `MeasureLabel` + `MeasureDomain`，分支取其叶子的
   **按面积加权均值**（`layout.go:180` `measure()`）。
6. 悬停 tooltip（默认给 path/size/share/measure）、`Open(id)` 回调、`Empty` 插槽、
   400 ms 下钻与数据过渡动画（负值关闭）。

## 3. 它能回答什么 flow 答不了的问题

flow 回答「谁连到谁」。treemap 回答**「哪里大、哪里重、哪里热」**——这三个问题
我们现在一个都答不了，而它们恰是 repowise 五层里第 2/5 层的图面：

| 问题 | 现在的回答方式 | treemap 上的回答 |
|---|---|---|
| 我的 8 个仓里，规模集中在哪？ | 无。图只数节点个数，不看分布 | 面积，一眼 |
| 哪个包是「看着小但一动全崩」的？ | `oac rank` 的文本表 | 面积小 + Measure 深红 |
| 改动会落在哪片区域？（churn） | 无（git 层 0%） | 面积=规模，颜色=churn，热点区自己会发光 |
| 该先修哪个包？（findings） | 无 | 面积=规模，颜色=findings 数/严重度 |


## 4. 面积该挂什么（这是唯一需要想清楚的设计决策）

`Node.Value` 的语义是**可加**的：分支的尺寸取其子树之和（`treemap.go:20-24`）。
所以能当面积的量必须满足「父 = 子之和」，这一条直接筛掉一半候选：

| 候选 | 可加？ | 现在可得？ | 判断 |
|---|---|---|---|
| 元素数 | ✅ | ✅ 现成 | **过渡期默认**。缺点是元素 ≈ 导出 named type 数，一个 5000 行只有 `func main` 的包面积是 0——**误导** |
| 文件数 | ✅ | ❌ 需要 §6(a) 的 `File` 元素 | **目标默认**。稳定、可加、每个包都有非零面积，且直接给 §6 一个答案 |
| 代码行（NLOC） | ✅ | ❌ scanner 已遍历文件，加一次行数统计是十几行 | 最贴 repowise 的 health map 口径；切片 3 顺手 |
| churn / commit 数 | ✅ | ❌ 切片 3 的 `file_metrics` | 语义是「活跃度」不是「规模」→ **当 Measure 不当面积** |
| dependents 入度 | ❌ **不可加**（跨包边会被父节点重复求和） | ✅ 切片 2 的 `rank` | 只能当 Measure。它的分支聚合是「按面积加权均值」（`treemap.go:20-24` + `layout.go:180`），语义正好对：一个包有多 load-bearing，看的是成员加权 |
| findings 条数 / 严重度 | ✅ | ❌ §5-A 的 findings 表 | 当 Measure |

**推荐**：`Value` = 文件数（切片 3 前用元素数顶着，并在 UI 上**写明口径**）；
`Measure` = 可切换的 lens（churn / dependents / findings 数），`MeasureLabel` 与
`FormatMeasure` 把口径显示出来。repowise 的 health map 就是这个「面积一个口径、
颜色另一个口径、两个都印在角上」的做法。

**PoC-13 已把这条从推荐升级为裁决**：元素数口径下 OpenArchitect 41% 的包面积为 0
被组件静默省略（见 §10 与 `../poc/2026-11-02-treemap-poc.md`）。所以「过渡期用
元素数」也不是安全选项——过渡期必须显式画出留白并标注口径，或者直接把
`File` 元素一起做掉。

`Node` 注释还给了一个我们必须用上的机制：**分支的 Value 大于子节点之和时，差额
留成 unlabeled space**（`free()`，`layout.go:174`）。这正是「这个包有规模但图上
看不到东西」的诚实表达——过渡期（面积=文件数、children=元素数）那些只有函数没有
导出类型的包会显示为留白而不是消失。切片 3 落 `File` 元素后 children 填满，留白归零。

## 5. 层级映射：复用现成的分组函数，不新写

树 = **仓 → 包 → 元素**，与 flow 的深度层完全同构。关键是**不要新写分组逻辑**：
`internal/core/graph/aggregate.go` 已经有 `groupByRepo`（`:142`）和 `groupByPackage`
（`:131`），`Aggregate(g, Level)`（`:38`）就是靠这两个函数折叠图的。treemap 的树
构建器调同一对函数，于是：

- flow 的深度层和 treemap 的层级**天然一致**，两个视图间切换不会「换了个世界」；
- 元素 ID 规则（`sys:`/`ctn:`/`cmp:`/`pkg:`/未来的 `fil:`）就是 `Node.ID`，
  满足组件「nonempty, unique and stable across frames」的要求，`Path()` 反查
  面包屑直接可用；
- 合并图的顶层是每仓一个 Node，`Node.Color` 向下继承（`treemap.go:32`）——
  「这一格属于哪个仓」由颜色自动解决，不需要额外图例。

## 6. 交互契约（与 flow 是同一份状态，不是第二个应用）

1. **图页顶部一个视图切换**（关系图 / 分布图），不是新页面。两视图读同一个
   `app.graph`，仓库 chips 过滤器对两者同时生效。
2. **treemap 的 `Open(id)` → 选中该元素 + 切回关系图 + `Reveal`**。这是 G2
   「深度切换要有空间锚点」的另一种解：在树里定位，在图里看关系。
3. **`State.Root`/`Active` 持久化**到 `app_config` 的 `view.graph.<repoID>`
   （切片 1 §9.5 已设计这个键，一直没落）。两个字符串而已。
4. 选中态双向：flow 里选中的元素，在树里描边；树里 hover 的分支，在 flow 里
   就是同一批元素进 `quietSet`（`flow_data.go` 的邻域聚焦通道现成）。

## 7. 顺手解掉的四个已知缺口

| 缺口 | 在 flow 里的状态 | treemap 为什么天然没有 |
|---|---|---|
| G1 读不出字 | 4.1/4.2 用 LOD 三档 + scale 通道勉强补住，仍是补丁 | 树永远铺满视口，字号由 tile 尺寸决定（`fitTexts` 二分试排），**没有相机就没有 scale 塌缩** |
| G5 相机/层级记忆 | 需要 `Camera()/SetCamera()` + 防抖写库 | `State{Root, Active}` 两个字符串，直接进 `app_config` |
| G7 键盘是 id 字典序不是空间序 | flow 的 Tab/方向键按数据序走焦点 | `nearest()`（`treemap.go:416`）按几何位置找方向上最近的 tile |
| G9 规模（PoC-11 数千节点） | flow 每帧全量重建 + 布局迭代，600 节点是实测上限 | squarify 是排序 + 一次线性铺放，**无迭代布局**；6000 个矩形对 mygo 的「1000 label 397 µs」基线是可测的量 |

## 8. 为什么不替换 flow

**边在树里没有位置。** 我们的第一问句是「改这里会坏哪里」，那是 `calls`/`depends_on`
的方向遍历；treemap 只能表达包含关系（父子），跨包的一条 `calls` 边在矩形树图上
只能画成「两个格子之间的飞线」，而飞线在 squarified 布局里是出了名的不可读。
repowise 也没这么干——它的 graph 页和 health map 页是两页。

所以 §1 的「增加不替换」不是保守，是这两个视图回答的问题本来不同。

## 9. 风险

1. **面积口径的误导**是最实的一条。元素数当面积时，「一个包很大」实际是「一个包
   导出类型多」。缓解：`Options.Label` 与角标必须写口径（`FormatValue` 就是为这个
   存在的），并且切片 3 换文件数。
2. **零面积节点被静默省略**（`Node` 注释：无有限正尺寸的节点 omitted）。纯函数包、
   只有常量的包在元素数口径下**整块消失**，用户不会收到任何提示。缓解见 §4 的留白机制。
3. **每帧重建的成本**：flow 那边我们已经踩过「每帧全量重建卡片」的坑
   （`flow_data.go:19-78` 注释自认）。treemap 的 `fit()` 对每个 tile 做二分文本测量，
   数千 tile 时可能是同一类瓶颈 → PoC-12 先测。
4. **400 ms 下钻动画**与我们的「扫描后重排」相遇会打架。缓解：数据变化时
   `AnimationDuration` 传负值关掉，用户手势时保留。

## 10. PoC 清单与结果（2026-11-02 已跑，见 `../poc/2026-11-02-treemap-poc.md`）

| # | 验证 | 结果 |
|---|---|---|
| PoC-12 | 6000 tile 的每帧耗时 | **通过**：中位 **1.187 ms**（60 fps 预算的 7%，flow 600 节点实测 3.4 ms 的三分之一）。两条失败分支都不需要 |
| PoC-13 | 两种面积口径在真仓上的误导程度 | **元素数口径不可接受**：OpenArchitect 41% 的包（21/51）面积为 0 被**静默省略**，OpenLaw 57%；文件级 64%/47% 的文件不贡献元素 → **裁决 §6 采纳 (a)**，面积挂文件 |

探针：`tools/treemapprobe`（headless `ui.NewTester` 1400×900，关过渡动画，每帧强制
重建以量布局）。跑法：
`go run ./tools/treemapprobe -repos <仓路径列表> -tiles 50,600,6000 -frames 120`。

两个附带结论：数据变化后的第一帧在 6000 tile 档出现 8 ms 尖峰 → §9-4 的
「重扫后关动画」从缓解建议升级为**必做**；面积口径的口径文字（`FormatValue`）
必须常驻，因为 41% 的包在错误口径下是**无声**的。

## 11. 落点与排期

- **落点**：`internal/ui/treemap_data.go`（新，位置与 `flow_data.go` 平级，调
  `graph` 的分组函数产出 `[]treemap.Node`）+ `page_graph.go` 的视图切换 +
  `state.go` 存 `treemap.State` 与两个 config 键。估计 150–250 行，零上游改动、
  零 schema 改动、零新依赖。
- **排期**：**不要单独排**。它和切片 3 的 git 层是同一件事的两半——先有 churn/
  findings 可看，树才有内容。建议在切片 3 spec 里作为「分布视图」一节，与
  `file_metrics` 同期落地；§10 的两个 PoC 提前到切片 3 spec 之前跑，因为它们
  同时回答粒度裁决（`2026-11-02-repowise-gap-and-refactor-verdict.md` §6）。
- **切片 2 不动**：切片 2 的未落地清单（schema v2、`arch_files`、`element_history`、
  增量+平价、五个新命令）与本条无关，照原 plan 做完。

## 12. 一句话结论

**该增加，不该替换。** 组件已经在我们的 fork 里（`components/treemap/`，1308 行实现
+ 249 行测试：squarify、下钻、面包屑、空间序键盘、按 tile 自适应文字、第二量着色），
工作量只有一个数据映射文件。真正的决策不在「用不用」，而在**面积挂哪个量**：
必须可加，所以是文件数（过渡期元素数），churn 与 dependents 走 `Measure` 着色。
它与切片 3 的 git 层互为动机——没有 churn 的树只是一张目录饼图，有了 churn 才是
repowise 那张 health map。

