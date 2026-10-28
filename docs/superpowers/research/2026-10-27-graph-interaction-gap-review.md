# 交互复盘（2026-10-27）：图页交互的现状缺口与 GitHub 参考方案

目的：切片 2 已把「地图式深度缩放 + GIS 导航」初步落地，但真实用起来效果缺口很大。
本文按代码实测盘点现状（全部 file:line 证据），列出按严重度排序的缺口，并给出
GitHub 上成熟图编辑器的参考机制与落点建议，供切片 2/3 的交互任务排期用。

调研方法：读 `internal/ui` 全部交互代码 + fork 的
`onevra/source@v0.2026.13-…-96938d5c50cf/components/flow`（fork 与上游
v0.2026.12 的差异 = NavigationMap/OnZoom/ScrollZoom 三件套）+ 切片 1/2 spec 与
findings + GitHub 参考项目（xyflow、G6、cytoscape.js、elkjs、d3-zoom、MapLibre、yFiles 文档）。

---

## 1. 现状盘点（实测，非 README 转述）

手势 → 行为（fork flow + App 装配后的真实语义）：

| 输入 | 当前行为 | 证据 |
|---|---|---|
| 拖拽（任意处） | 平移相机（NavigationMap：按下即平移，节点上也平移） | fork `render.go:210-270` input()，`flow.go:100-104` |
| 滚轮/触控板滚动/捏合 | **改深度层（仓库↔包↔元素），不改缩放** | `flow.go:85-90` OnZoom → `state.go:244-253` ZoomDepth |
| 双击空白 | 同上，改深度（±1 层） | fork `render.go:243-256` |
| ±按钮 | 改深度；「适应窗口」按钮 = Fit | `page_graph.go:136-157` |
| 深度变化后 | 重聚合 + **无条件 Fit()**（相机推平） | `state.go:249-252` |
| 单击节点 | 选中 → 右侧详情抽屉 | `flow.go:243-246`，`detail_panel.go` |
| 双击节点 | 展开方法列表（drillInto） | `flow.go:247-249`，`detail_panel.go:127-138` |
| 单击空白 | 清选中 | fork `render.go:259-263` |
| hover 节点 | 相关边加粗（仅此） | fork `render.go:305-312` emphasized() |
| Tab/方向键 | 按 **数据序（id 排序）** 走焦点，相机跟随 | `flow.go:145-147`，`render.go:277-280` |
| ⌘K | 命令面板搜索元素 → Select + Reveal | `command.go:35,157-162` |
| 详情抽屉 chip | 上下游邻居 → Select + Reveal | `detail_panel.go:170-183` |
| 仓库 chips | 过滤合并图，持久化 `graph.repos` | `page_graph.go:95-127` |

关键事实（决定后面所有判断）：

1. **相机缩放没有任何用户入口**。`OnZoom` 劫持了滚轮/捏合/双击空白全部缩放手势
   （fork `flow.go:85-90`）；±按钮也被改绑深度。唯一改 scale 的路径是 `Fit()`，
   而 Fit 的 scale 由图尺寸决定、钳在 `[minZoom=0.1, 1]`（fork `render.go:122-134`）。
   PoC-4 实测 300 节点 Fit 后 scale≈0.1348（切片 1 findings），卡片缩到 ~23×6px——
   **用户看清楚了位置关系，但没有任何办法把字读出来**。
2. **卡片随相机缩放**（mygo 元素级 Translate/尺寸，fork `flow.go:221-234`），而
   「缩放改深度不改字号」的地图契约成立的前提是标签按屏幕空间恒尺寸绘制
   （MapLibre/Mapbox 的 semantic zoom 模型）。前提没实现，契约就是半句话。
3. **切片 1 spec §9.5 设计的 LOD 分级绘制（`Content` 按 `flowState.Zoom()` 分级）
   没有实现**：`node_card.go:43-55` 的 nodeCard 是单一形态。`flowState` 就在 App
   手里（`state.go:90`），`State.Zoom()` 是公开 API（fork `flow.go:118`）——
   这条是 spec 欠账，不是上游缺口。
4. **深度切换无空间锚点**：`ZoomDepth` 每次 `Fit()`（`state.go:251`），从包层
   某处深入元素层时视野整体推平重排，用户刚建立的「我在看哪」的空间记忆清零。
   spec §9.5 要求的「相机对准切换前的焦点节点」也没做。
5. **相机与层级不持久化**：`config.go` 只有 `scan.root/impact.depth/graph.repo(s)`，
   spec §9.5 设计的 `view.graph.<repoID>`（zoom+锚点+层级）没有落点。重启即丢。
6. **无 minimap**（切片 2 spec §3.1-9 已列任务未做）；视口矩形受上游 API 缺口
   阻塞（`State` 不暴露 camera x/y，fork `flow.go:107-138` 公开面只有
   Ready/Zoom/Fit/ZoomIn/ZoomOut/Reveal/Position）。
7. **选中不默认高亮邻域**：淡出（Muted）只在影响模式开启时算（`flow_data.go:83-92`
   quietSet）；普通浏览下点一个节点，几百条边里追它的上下游全靠眼睛。
8. **边不可交互**：无 hover/点击边，看不到关系类型与 weight；边 label 11px
   （fork `render.go:37`）低倍下糊成一团。
9. **键盘只有 ⌘K**：无 Esc 取消选中、F 适应、+/− 深度、Enter 下钻；焦点序是
   id 字典序不是空间序，方向键在图上没有意义。
10. **每帧全量重建卡片、无屏外剔除**（`flow_data.go:19-78` 注释自认）：600 节点
    实测 3.4–3.5 ms 尚可，切片 2 合并图数千节点的 PoC-11 是硬门槛。

## 2. 缺口清单（按实际场景严重度排序）

场景基准：一人公司 3–10 仓、合并图数百到数千节点，日常动作 = 「找入口 →
看邻域 → 追影响 → 跨层定位」。

| # | 缺口 | 场景叙事 | 严重度 |
|---|---|---|---|
| G1 | **读不出字**：无 scale 通道 + 无 LOD | 元素层 300+ 节点 Fit 后 0.13x，卡片不可读，且无从补救。用户第一反应「缩放坏了」——其实被改成了深度。这是当前最大的体验断裂 | P0 |
| G2 | **深度切换=瞬移**：无锚点、无过渡语义 | 在包层盯上 `store` 包，滚轮深入，落点却是全图 Fit；「从哪来的」空间线索断了 | P0 |
| G3 | **无方位感**：无 minimap/视口矩形 | 大图平移后完全不知道自己看的是哪个区域；Reveal 是唯一救生圈 | P1（切片 2 已排） |
| G4 | **邻域不点亮**：追上下游靠肉眼 | 「改这里会坏哪里」的第一眼答案应该是 1-hop 高亮，而不是先找到影响模式开关 | P1 |
| G5 | **相机/层级记忆缺失** | 重启回到 Fit；重扫后深度层也重置（zoomLevel 是会话态） | P1 |
| G6 | **边不可查**：类型/weight 无入口 | depends_on 和 calls 只有线型区别，weight 粗细无人可读；点边看详情是图编辑器标配 | P2 |
| G7 | **键盘覆盖薄** | 无 Esc/F/+−/Enter；对「键盘流」用户是硬伤 | P2 |
| G8 | **无分组框**：仓库/包边界只靠 label 文字 | 合并图里两仓同名包靠 `GroupLabel` 防重名（`flow_data.go:47-51`），但视觉上没有边界；同包边不画（`flow_data.go:103-105`）的根因是布局画不出同排边，有分组框就能画 | P2 |
| G9 | **规模上限**：无剔除/虚拟化 | PoC-11（≥600 节点合并图）未跑；数千节点帧时间未知 | P2（门槛已有，失败分支已写） |


## 3. GitHub 参考方案（已核实存在的机制）

| 项目 | 借鉴什么 | 对应缺口 |
|---|---|---|
| **xyflow/xyflow**（React Flow，MIT，spec PoC-11 已钦定逃生通道） | viewport API：`fitView(nodes,padding)`/`zoomTo`/`setViewport`/`useViewport`；semantic zoom 示例（按 zoom 阈值切节点渲染密度，即 LOD）；Expand and Collapse 示例（下钻=就地展开，相机不动）；`<MiniMap>` 带 viewport mask 与节点着色 | G1 G2 G3 |
| **d3/d3-zoom** | 指针锚定缩放的 transform 数学（k,x,y + scaleExtent）：zoom 保持指针下的图元不动——深度切换的锚点数学与它同构 | G2 |
| **antvis/G6**（MIT） | `zoom-canvas` behavior（光标锚定、可配灵敏度、范围钳制）；minimap 插件带**视口矩形**语义；**combo 元素**（把一组节点圈成可视分组框）——正是 G8 的形态参考 | G3 G8 |
| **cytoscape/cytoscape.js**（MIT） | 样式表按 zoom 区间声明式换样式（semantic zoom 的声明式版本）；`collapse()/expand()` compound nodes（子节点在父节点内展开）；neighborhood-highlight 扩展（点选点亮 1-hop）——G4 的标准答案 | G1 G4 G8 |
| **kieler/elkjs / Eclipse ELK**（EPL） | layered 算法原生支持 **compound/层级图**：contains 是层级不是普通边，依赖边跨层路由——若做 G8 分组框，ELK 的分层+compound 处理是算法参照（fork 的 layout.go 是简化 Sugiyama） | G8 |
| **maplibre/maplibre-gl-native**（BSD） | semantic zoom + **恒尺寸标签** + label collision：「滚轮改层级不改字号」这个隐喻的真正成立条件。若坚持 OnZoom=深度，卡片就必须按屏幕空间绘制（LOD 极端形态） | G1 |
| **yWorks yFiles**（商业，只读文档） | folding 交互语义：折叠节点上双击 → 子图在**父节点原位**展开；folding + semantic zoom 联动 | G2 |
| **Structurizr / C4 工具族** | 下钻面包屑（System › Container › Component）与层级切换的呈现方式——spec §9.5 已提面包屑但未实现 | G2 G5 |

**不跟**（避免滑回别人默认值）：React Flow 的 panOnScroll 默认（触控板平移+⌘滚轮
缩放是 web 习惯，macOS 原生地图是双指=平移/捏合=缩放，fork 的 ScrollZoom 已经改过
一轮，别再翻回去）· cytoscape 的力导向默认布局（架构图的层级语义不能换）· G6 的
WebGL 渲染（mygo 自绘栈下不是同一层决策）· yFiles 的商业依赖。

## 4. 优化建议（映射到代码，按可执行性分档）

### 4.1 零上游改动，app 层即可（本切片可做）

1. **LOD 分级绘制**（修 G1 的读字问题，spec §9.5 欠账）：`nodeCard` 闭包读
   `a.flowState.Zoom()`——scale < 0.5 只画圆点+边框+类型色；0.5–0.9 图标+名字；
   \> 0.9 加包路径。卡片尺寸仍钉死 168×46（布局稳定的约束不动），只是低倍下
   画得更少。每帧重建的帧时间预算反而变好。
2. **深度切换锚点**（修 G2 的大半）：`ZoomDepth` 里把 `Fit()` 换成：有
   `selected` → 重聚合后 `Reveal(selected)`；无选中 → Fit。再进一步：深度变化
   前后深度图有包含映射（`aggregate.go` 的 fold 关系），把旧层的选中节点映射到
   新层的对应组/成员节点，Reveal 映射结果——「盯着 store 深入，落点还是 store」。
3. **键盘**（G7）：graphPage 注册 Esc（Select("")）、F（Fit）、⌘+/−（ZoomDepth）、
   Enter 走 flow 已有的焦点激活。mygo `c.Shortcut` 够用。
4. **邻域高亮**（G4）：`quietSet()` 增加非影响模式分支——`selected != ""` 时
   1-hop（`view.Upstream/Downstream(id,1)`）保持亮，其余 Muted。机制现成，
   语义是 cytoscape neighborhood-highlight 的复刻。给一个开关防止「我就想全亮着」
   的用户（默认开，按观察定）。

### 4.2 需要 fork 上游（onevra flow）小改（提 PR，fork 在手）

5. **scale 通道**（G1 的另一半，二选一）：
   - 方案 A（推荐，地图正统）：`OnZoom` 只留给「层级」，**⌘+滚轮 / 捏合走
     scale**（fork `gesture.go` classify 已区分 Mods，`zoomAt` 已实现指针锚定）；
     配合 4.1-1 的 LOD，低倍看结构、⌘滚轮读文字。
   - 方案 B：滚轮恢复 scale，深度只走双击/±按钮/键盘。改动更小但丢掉了
     切片 2 刚建立的「滚轮=层级」心智。
6. **`State.Camera() (x, y, zoom)` + `SetCamera`**（G3 视口矩形 + G5 相机持久化，
   切片 2 spec §2 已提 issue 建议，fork 里直接加即可）；有了它 minimap 视口矩形
   和 spec §9.5 的 `view.graph.*` 持久化（防抖 150 ms 写 app_config）都通了。
7. **`OnZoomAt(x, y, zoomIn)`**（G2 完全体）：把手势的指针位置交给 app，app 用
   `Hit(x,y)`（fork `render.go:180-194`，导出即可）找到指针下的组节点，深入时
   预选该组、浅出时 Reveal 原组——GIS 的「在点的地方展开」语义。
8. **`State.Hovered()`**（低倍 tooltip）：hover 节点时浮一个恒尺寸的名字+包路径
   tooltip，低 LOD 下读名的标准补偿（MapLibre label 思路的 app 层版）。

### 4.3 后续切片

9. **包/仓分组框（combo）**（G8）：视觉分组 + 把 `contains` 从「画出来的边」升级
   为「框」，同包 `calls` 边恢复绘制；布局参照 ELK layered compound。这是布局
   层的大改，单独 spec。
10. **规模**（G9）：PoC-11 先行；红了先靠 4.1 的 LOD 省帧时间 + 过滤器收窄，
    再考虑屏外剔除（需要 4.2-6 的 Camera()），最后才是 WebView+React Flow
    逃生通道（spec 已授权，只换 internal/ui）。

## 5. 一句话结论

当前交互的骨架（相机、深度层、Reveal、抽屉）是对的，断在两点：**缩放手势被
深度独占后，「把字读出来」这条路不存在**（G1），以及**深度切换丢空间锚点**
（G2）。两者都能在 fork flow + app 层用 LOD 绘制 + 锚点映射 + ⌘滚轮通道解决，
参考实现全部在 MIT 协议项目里（React Flow semantic-zoom/Expand-Collapse、
cytoscape neighborhood-highlight、G6 combo、d3-zoom 锚定数学）。建议切片 2
剩余任务把 4.1 全部收进，4.2 的四个上游改动打包成一个 fork PR。

---

## 6. 落地记录（2026-10-27，4.1 全部收进）

四件事全部落在 app 层，零上游改动；`go test ./...` 全绿（含 -race），
每条都有变异证据（改坏实现必红）：

| 项 | 落点 | 测试 |
|---|---|---|
| LOD 分级绘制（G1 之一） | `node_card.go`：`nodeCard(c, el, zoom)`，`lodDot=0.5`/`lodFull=0.9` 三档（点/图标+名/全卡），盒尺寸恒 168×46；`flow_data.go` 闭包读 `a.flowState.Zoom()` | `TestNodeCardLODStairs`（阈值两侧各取点）+ 原两行文本测试 |
| scale 通道（G1 之二） | `page_graph.go`：overlay 拆成 适应窗口/放大/缩小（`flow.State.ZoomIn/ZoomOut`，公开 API）+ 深入/上一层（Layers±）；`graphShortcuts`：Esc 清选中、F Fit、⌘=/⌘− 缩放（命令面板打开时不注册） | `TestZoomControlsSplitScaleFromDepth`、`TestGraphPageEscapeClearsSelection`（含面板开着时 Esc 归对话框） |
| 深度锚点（G2） | `state.go`：`Select` 写焦点记忆（focusElement/focusPackage），`ZoomDepth` 换层后 `anchor()` 映射到新深度对应节点 `Select+Reveal`，映射不到才 Fit；`Open` 的 Fit 改 Reveal；`Reload` 连锚点一起清；`graph.RepoGroupID` 转正 | `TestZoomDepthAnchorsSelectionAcrossLevels`（元素→包→仓→包→元素全穿越）、`TestZoomDepthWithoutSelectionFallsBackClean` |
| 邻域聚焦（G4） | `flow_data.go` `quietSet()`：影响集优先，否则选中 + 1-hop 保持亮；默认开，mapHeader 加「邻域聚焦」开关（`SetNeighborFocus`） | `TestQuietSetNeighborhoodFocus`（含 contains 墙挡 2-hop）、`TestQuietSetImpactWinsOverNeighborhood`（2-hop 区分两种淡出集） |

未落地的仍按原计划：4.2 的四个上游改动（⌘滚轮走 scale、`Camera()/SetCamera()`、
`OnZoomAt`、`Hovered()`）打包成 fork PR；4.3 的 combo 分组框与规模门槛照旧。
