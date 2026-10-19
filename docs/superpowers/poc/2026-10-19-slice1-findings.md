# 切片 1 PoC 实测（2026-10-19）

## PoC-1 mygo 原生骨架 + onevra 侧栏窗口

- 结论：**PASS**（工具链、pin、原生窗口、onevra 侧栏外壳、关窗退出全链路跑通）。
  说明：本会话拿不到像素级视觉确认（`screencapture` 无录屏权限、AppleScript 无自动化
  授权、AX 未受信任、无 orca/computer-use CLI），因此布局内容由 mygo 官方 headless
  `ui.NewTester` 断言 + `CGWindowListCopyWindowInfo` 实测在屏窗口双重证据替代，
  进程存活与退出用 pgrep 实测。不是纯编译通过的"PASS"。
- 数字：
  - `go tool mygo dev`：冷构建 1.6s / 增量构建 2.0s；从启动到窗口在屏 **4.0s**
    （winprobe 0.5s 粒度轮询实测）。
  - 窗口：`CGWindowListCopyWindowInfo` 实测 `owner=OpenArchitect Dev
    name=OpenArchitect layer=0 bounds=x=2205 y=264 w=1226 h=783`（px，位于外接
    显示屏；DIP↔px 换算未验证，见"影响"）。
  - 关窗退出：临时探针（env `OAC_PROBE_CLOSE=15s` 门控，测完已还原）在 WhenReady
    后 15s 调 `shell.Window().Close()`，实测窗口出现 14.8s 后应用进程消失，dev 日志
    打出 `• OpenArchitect Dev quit` 后 dev 工具自行退出 → `OnClosed → App.Quit()`
    链路成立。
  - headless 渲染断言：220x400 侧栏视图 `tt.Texts()` =
    `"架构图 / 仓库 / 设置"`；600x500 内容视图 = `"尚未扫描任何仓库"`，均 PASS。
  - 用户数据目录：运行期实测创建 `~/Library/Application Support/OpenArchitect`
    （证实 `mygo.App.SetName("OpenArchitect")` 决定 PathUserData，Task 14 CLI
    的 `os.UserConfigDir()/OpenArchitect` 对齐假设成立）。
  - 日志中唯一异常：`error messaging the mach port for
    IMKCFRunLoopWakeUpReliable`——macOS 输入法框架的已知无害告警，非帧错误、无
    panic；进程全程存活。
- 影响 / 与 brief 的偏差记录：
  1. `mygo.json` 模板里**没有** `productName` 键，等价字段是 `name`；已按 brief
     只改这一处为 `OpenArchitect`。`identifier` 仍是模板的
     `com.example.tmpscaffold`（brief 明令"只改这一处"），打包/签名任务
     （Task 22/24 前后）需另行定夺。
  2. API 逐条对照模块缓存源码核实（`mygo@v0.5.5`、`onevra/source@v0.2026.12`），
     brief 代码全部真实可用：`sidebar.ShellOptions{Title, StateKey, LeftWidth,
     Theme, Left, Content}` 字段存在；`Theme` 字段类型是 `func(*ui.Context)
     *ui.Theme`，而 `theme.Onevra` 恰是该签名的**函数**（不是变量）；
     `Shell.Window()` 返回 `*mygo.Window`；`Window.OnClosed(func()) (off func())`
     存在；`lucide.Box` 是 `func() *ui.SVG`，`_ = lucide.Box` 取函数值可编译。
     唯一语义差别：brief 注释把 `theme.Onevra` 写得像主题值，实际是每帧回调，
     不影响使用。
  3. 模板脚手架（native）自带 `main_test.go`（计数器 demo 测试）、
     `resources/icon.png`、`.agents/skills/mygo-maintenance/`、`.gitignore`。
     前三者不属于 brief 文件清单，随 Step 3 的 `rm -rf .tmp-scaffold` 一并删除；
     `.gitignore` 的 `/build/`、`/.mygo/`、`mygo_windows_*.syso` 三条决定**保留**，
     已并入仓库根 `.gitignore` 并提交（否则 `go tool mygo dev` 的 13MB 缓存目录
     `.mygo/` 会永久污染 git status）。`go build ./...` 落在仓库根的产物
     `OpenArchitect` 二进制已删除、不入库。
  4. `go mod tidy` 把 `golang.org/x/image` 间接版本从模板的 v0.46.0 抬到 v0.47.0
     （onevra 的 MVS 选择），业务 pin（mygo v0.5.5、onevra source v0.2026.12）
     不受影响。
  5. 侧栏 220 DIP、居中占位文案的**像素级**目测留给控制器/人工验收；本任务内的
     等效证据链为上述 headless 断言 + 在屏窗口 + 关窗退出三项实测。

## PoC-2 store 打开与驱动可用性（无 cgo）

- 结论：**PASS**（`CGO_ENABLED=0` 下 `sqlite.Open` 在 Go-native mygo 应用里
  打开真实库文件成功，且 `sqlite.LibraryPath()` 在 `go run` 与
  `go tool mygo dev` 两种跑法下都找到了库。不是"能编译"级别的 PASS：
  CREATE/INSERT/SELECT 含中文单引号分号样本往返、`PRAGMA foreign_keys` 实测
  回读、`probe.db` 真实落盘全做过）。
- 数字：
  - `go tool mygo dev`：构建 1.4s；app 日志给出
    `sqlite library: <worktree>/.mygo/dev/darwin-arm64/OpenArchitect Dev.app/Contents/Resources/libmygo-sqlite3.dylib`
    （dev 工具把库放进 app Resources，`Load()` 优先走
    `mygo.App.Path(mygo.PathResources)` 的分支被实证）+ `store.Open ok`。
  - `go run .`：`sqlite library: ~/Library/Caches/mygo/natives/f78fe493…4d0/libmygo-sqlite3.dylib`
    + `store.Open ok`。首次跑法下库不存在，插件从 GitHub release 下载
    `mygo-sqlite3-darwin-arm64.dylib`（1,551,032 字节，URL
    `github.com/egoist/mygo/releases/download/sqlite-3.53.4-1/…`，
    SHA-256 `f78fe493…` 校验通过后按摘要目录名入缓存；curl 实测该 URL
    HTTP 200 / 2.4s 可达，本机 GitHub 可达而 proxy.golang.org 不可达）。
  - 单测（`CGO_ENABLED=0 go test ./internal/core/store/ -v`）：
    TestOpenRoundTrip PASS 2.32s（含首下库时间）、TestForeignKeysEnabled
    PASS 0.00s；包级缓存复跑 0.487s。`go vet ./... && go test ./...` 全绿，
    输出无告警。`go.mod`/`go.sum` 零变更（sqlite 是 mygo 的子包，purego 已在
    既有纯 Go 闭包里，未引入任何 cgo 依赖）。
- 影响 / 与 brief 的偏差记录：
  1. 全部 API 逐条对照模块缓存源码核实（`mygo@v0.5.5/plugins/sqlite/`），
     brief 代码真实可用：`Open(ctx, path, OpenOptions) (*DB, error)`、
     `Execute/Query(ctx, sql, args...) (Result|Rows, error)`、
     `Transaction(ctx, []Statement) ([]Result, error)`、`Close() error`、
     `LibraryPath() (string, error)` 签名一致；`Rows{Columns []string;
     Values [][]any}` 是结构体快照而非游标（`database.go:50`），故测试直接
     下标 `Values[0][0]`；查询值类型只有 nil/int64/float64/string/[]byte，
     临时版 `num` 只需 int64/float64 两个 case（实测 PRAGMA 返回 int64）。
  2. `OpenOptions` 的 doc 注释 "Foreign keys are enabled by default" **属实**
     （临时探针直接 `sqlite.Open` 后立即查询 `PRAGMA foreign_keys` = 1，插件
     源码无任何 foreign_keys 处理，默认 1 来自库的编译期宏
     `SQLITE_DEFAULT_FOREIGN_KEYS=1`，见 `native/build.zig:23`）。brief 在
     `Open` 里显式跑 `PRAGMA foreign_keys = ON` 是幂等双保险，按 brief 原样保留。
  3. `Execute` 明确拒绝事务控制语句（BEGIN/COMMIT/ROLLBACK、ATTACH、VACUUM
     INTO，见 `database.go:141-144`），语句分组只走 `Transaction`（内部
     `BEGIN IMMEDIATE`，任一失败整体回滚）。Task 7 的"单事务写快照"必须经
     `DB.Transact`，不能自己 Execute BEGIN。
  4. `Path()` 方法未列在 brief 的 Interfaces 清单里，但 brief 的 db.go 代码
     块包含它——照代码块落地；Task 6/14 定位库文件时会用到。
  5. 打包形态注意：`LibraryPath` 对 `mygo.App.IsPackaged()` 为真的 app 不再
     回退下载，缺库直接报错——Task 22/24 的打包链路必须保证 dylib 进
     Resources。R1 例外维持原样：`plugins/sqlite` 目前只被
     `internal/core/store` 引用（临时探针曾在 main.go 引用，测完已用 Edit
     还原，`git diff main.go` 为空）。
  6. PoC-2 门槛的两半（Go test 无 cgo 打开真库 + app 上下文里
     `LibraryPath()` 找库）都已实测通过；Task 24 提到的"PoC-2 剩下的一半"
     若指打包形态（IsPackaged）下的库路径，仍留给 Task 24 验。

## PoC-3 go/packages 规模探针（定 U2 的 LoadMode）

- 结论：**PASS**，且 **Task 9 保留 `NeedDeps`**（`packagesNeedAll` =
  `NeedName|NeedFiles|NeedSyntax|NeedTypes|NeedTypesInfo|NeedDeps|NeedModule`
  整串照抄进 `internal/scanner/language/go.go`）。门槛"真实仓库解析 ≤ 60 s
  且峰值 ≤ 2 GB"在两个仓库、两种模式下全部通过：最差单次 elapsed 25.4 s
  （onevra、无 NeedDeps、该模式冷启动首跑），全程最大 sys 887.8 MB、最大
  HeapAlloc 672.3 MB，离门槛一个数量级。保留 NeedDeps 的实测依据：
  (a) 它对稳态耗时的代价只有约 3~7 s（mygo 1.7→7.3 s、onevra 0.9~7.0→5~8 s），
  内存代价约 +350~550 MB HeapAlloc（mygo ~275→672 MB、onevra ~84→593~645 MB），
  均在门槛内；(b) §6.4 的 `depends_on`/`calls` 边要求跨包类型对象同一性
  （`types.Info.Uses` 解析到别的包的 `*types.TypeName`），brief 自己的判断
  （"NeedTypes 在类型检查跨包时基本必须依赖它"）与实测一致——去掉 NeedDeps
  后本包导出类型计数不变，但那只证明"数数"不受影响，边生成需要依赖类型完整
  加载，不值得为省内存赌在 Task 10 的正确性上；(c) 模式切换本身会让 `go list`
  按新驱动参数重跑，出现全场最慢的一次读数——onevra 去 NeedDeps 的冷启动
  首跑 25.4 s（见下方"数字"该模式唯一一条冷跑记录），"收窄即加速"的预期在
  真实缓存行为下不成立。
- 数字（探针 `go run ./tools/parseprobe -root <路径>`，GOFLAGS 默认、
  GOPROXY=goproxy.cn；"冷"= 该模式首跑，"热"= 缓存就绪后的复跑，热跑取 2~3 次）：
  - 仓库 A：`/Users/futuremeng/go/pkg/mod/github.com/oneo-me/onevra/source@v0.2026.12`
    （6.8 MB 模块目录，29 个自身包，packageErrors=0，**exportedTypes=138**）。
    - 全串（含 NeedDeps）：冷 elapsed=7.54s heapAllocMB=644.5 sysMB=745.0；
      热 5.15s/631.9/753.5、8.24s/593.0/757.3。
    - 去 NeedDeps：冷 elapsed=25.39s heapAllocMB=93.8 sysMB=117.4；
      热 0.89s/88.7/113.4、7.04s/83.5/105.3。
    - NeedDeps 开↔关（热稳态）：耗时 +4~8 s 量级（存在 ±4 s 运行间波动），
      HeapAlloc +约 500~550 MB，sys +约 640 MB。
  - 仓库 B（更大的仓库）：`/Users/futuremeng/go/pkg/mod/github.com/egoist/mygo@v0.5.5`
    （19 MB 模块目录，108 个自身包，**exportedTypes=519**）。
    - 全串（含 NeedDeps）：首跑（3 个包 audio/mp3、audio/vorbis、ui/richtext 因
      GOPROXY=off 拉不到 go-mp3/oggvorbis/x/net 而记 packageErrors=3）
      elapsed=5.57s heapAllocMB=656.3 sysMB=887.8；依赖补齐后 packageErrors=0，
      该模式冷 18.27s/638.4/867.0，热 8.80s/647.4/851.2、7.26s/672.3/875.4。
    - 去 NeedDeps（依赖已补齐）：冷 elapsed=4.94s heapAllocMB=284.5 sysMB=317.4；
      热 1.71s/275.4/308.6。
    - NeedDeps 开↔关（热稳态）：耗时 +5~6 s（mygo 1.7→7.3 s），
      HeapAlloc +约 360~400 MB，sys +约 560 MB。
  - 计数口径注意：探针按 brief 的 `componentOf` 数的是**自身包全部导出
    named struct/interface 类型**，未扣 main 包类型（§6.3 的 Component
    排除集会略低于此数），即 519/138 是 Component 数的上界；`packages.Load`
    未传 `Tests: 1`，`_test.go` 本就不被加载、不在计数口径之内。
    两模式下 exportedTypes 计数完全一致（138/519），说明本包枚举不依赖
    NeedDeps——这是探针口径的实测，不是边质量的实测。
  - 内存口径注意：heapAllocMB 是 `packages.Load` 返回后的即时存活堆、sysMB 是
    运行时保留量，均为 brief 探针的固定输出；两者任意一次读数都远低于 2 GB，
    且 sys ≥ 堆峰值的一般性成立使"峰值 ≤ 2 GB"结论稳健。
- 影响：
  1. U2 关闭：Task 9 的 `internal/scanner/language/go.go` 直接复用
     `tools/parseprobe/main.go` 里的 `packagesNeedAll` 整串位（含 NeedDeps），
     不需要 §12 PoC-3 失败分支里的"包级预筛 / NeedDeps 收窄"。
  2. spec §6.4 规模预期获实证支持并回填：mygo 模块（108 包，中等偏大）
     519 个导出类型落在"200~600 Component"区间内；onevra source
     （29 包，中小）138 个、落在区间下沿之外——说明该区间大致对应
     "百包级"仓库，包数更少的仓库 Component 会更少，§9 性能约束按
     600 上界设计仍安全。
  3. 探针保留在 `tools/parseprobe/`，每次映射规则调整复测；新增依赖
     `golang.org/x/tools v0.52.0`（具体 pin，进 `go.mod`/`go.sum`；
     `go mod tidy` 顺带带入 `golang.org/x/mod v0.42.0`、`golang.org/x/sync v0.24.0`
     等间接项，业务 pin mygo/onevra 零漂移）。
  4. 对 Task 9 的性能预算参考：`packages.Load("./...")` 的 5~9 s 是
     go list 子进程 + 类型检查的合计墙钟，桌面端扫描必须异步化（§10 的
     取消/进度链路），不能放主线程；运行间波动 ±4 s 属环境噪声，
     门槛余量足够大不影响结论。
  5. 本机没有比 mygo 更大的现成 Go 仓库可用（`~/github` 下各 checkout 均无
     `go.mod`），按 brief 的 fallback 用 mygo 模块目录充当第二仓库；
     300~600 导出类型档由 mygo=519 覆盖。

## PoC-4 flow 在 600 节点下的帧时间与稳定性

- 结论：**PASS**（门槛两半都实测成立，非"能渲染"级别）。
  600 节点稳定态整帧 took 中位数 3.90~4.10 ms/帧（3 次复跑全列于"数字"，
  另加首批 7 次复跑 3.80~5.10 ms 佐证），远低于 16 ms；其中真正由渲染
  负担决定的"功"（build+layout+paint+other，不含等屏的 present 阻塞）
  中位 3.40~3.50 ms、全场最差单帧 6.60 ms。漂移：headless `tt.Advance`
  ×2（间隔 500 ms）后与实窗 30 个真实帧后，全部 600 个节点
  `Position` maxAbsDelta = 0.000000（50/300/600 各档同为 0）。
  **Task 18/19 可照 spec §9.3 直接用 `flow`，无需切逃生通道。**
  两条如实附注：(a) 各档都存在零星 18.0~19.2 ms 单帧，但全部由
  present 相位（13.9~18.1 ms，等显示同步）主导、同帧"功"≤5.3 ms 且
  "no GC"、且每帧紧随其后都有引擎自报的 `skipped to catch up with the
  display` 帧，50 节点档同样出现（2/117 帧）——判定为系统 vsync 调度
  毛刺而非 flow 渲染能力（数字逐帧列于"数字"）；(b) 可读性量化信号：
  600 节点 fit 后 zoom=0.1000，恰为 flow 的 minZoom 下限
  （`render.go:20`），即 600 节点"整图 fit"在 1400×900 里已超出可读
  缩放（像素级目测本会话不可得，与 PoC-1 同因；见"影响"第 3 条）。
- 数字（探针 `go run ./tools/flowprobe -nodes N -frames 120`，
  环境变量 `MYGO_FRAME_STATS=all` 逐帧记录引擎整帧；窗口 1400×900；
  "整帧 took"= 引擎 frame 全程 build+layout+paint+present；
  "功"= build+layout+paint+other，即扣除等待显示的 present；
  稳定态 = 剔除 readyAt 之前的帧，每档 readyAt=3、稳定态 117 帧；
  机器：Apple M2 Max / 96 GB / macOS 27.0.1 (26A434)、AC 电源、
  外接显示屏在屏渲染，GOPROXY=goproxy.cn）：
  - 600 节点 · 最终批 3 次（代码与入库一致）：
    - run1：整帧 median=4.10 p95=5.40；功 median=3.50 p95=4.70 max=5.40；
      探针自测 flow.View median=940µs、首帧 2.868ms；>16ms 帧 1 个
      （f40=18.7，present=15.6，no GC，skip@f41）；冷启动首帧（含字体图集/首建树）
      f1=49.3ms；zoom=0.1000；liveDrift f8→f38 maxAbsDelta=0.000000；
      headless 双 Advance 前后 maxAbsDelta=0.000000；labelsFound=600/600。
    - run2：4.10→中位 3.90 p95=4.80；功 median=3.40 p95=4.20 max=5.50；
      flow.View median=900µs、首帧 2.144ms；>16ms：f80=18.0（present=14.4，
      no GC，skip@f81）；冷 f1=44.7ms；zoom=0.1000；漂移同上全 0。
    - run3：整帧 median=4.10 p95=5.70；功 median=3.50 p95=4.90 max=6.60；
      flow.View median=914µs、首帧 2.436ms；>16ms：f77=18.4（present=13.9，
      no GC，skip@f78）；冷 f1=38.8ms；zoom=0.1000；漂移同上全 0。
  - 300 节点 · 1 次（最终批）：整帧 median=2.30 p95=5.30；功 median=1.90
    p95=4.70 max=5.20；flow.View median=490µs、首帧 1.854ms；>16ms：
    f86=19.2（present=15.0，no GC，skip@f87）；冷 f1=32.7ms；zoom=0.1348；
    漂移全 0（300/300 比较）；labelsFound=300/300。
  - 50 节点 · 1 次（最终批）：整帧 median=0.60 p95=2.00；功 median=0.40
    p95=1.50 max=3.00；flow.View median=117µs、首帧 214µs；>16ms：
    2 个（f7=18.4 present=18.1 no GC skip@f8；f96=18.2 present=16.6 no GC
    skip@f97）；
    冷 f1=15.6ms；zoom=0.8564；漂移全 0；labelsFound=50/50。
  - 首批复跑（输出行差异、驱动路径相同的早期版本探针）：600 节点 7 次
    整帧 median=3.80/3.90/4.10/3.80/4.00/4.20/5.10ms，其中 4 次各有
    1 个 18.1~18.6ms 的 present 主导帧（14.0~15.2ms、no GC）、3 次为 0
    （max 6.10~7.30）；>16ms 合计 4/819 稳定帧 ≈0.5%。50/300 各 1 次：
    median=0.90/2.90ms、max=3.20/6.30ms、无 >16ms 帧。全部 9 次
    liveDrift 与 headless 双 Advance 漂移均为 0。
  - 就绪口径：`state.Ready()` 全部跑次在第 3 帧变 true（headless
    Tester 同样 builds=3）；探针侧零值 `state.Ready()` 预检通过。
  - 日志异常仅 `IMKCFRunLoopWakeUpReliable`（PoC-1 已记录的无害告警）。
  - 未测项如实声明：像素级目视可读性（本会话无截图通路，见"影响 3"）；
    未跑 `go tool mygo dev` 打包形态（引擎同为 mygo 原生 surface 路径，
    测量在 `go run` 下完成）。
- 影响：
  1. §9.3 的 168×46 定死尺寸规则获得机制级实证：`flow` 对给定
     Width/Height 的节点走 `measure()` 短路（flow.go:280-292），
     `prepare()` 只在尺寸/拓扑/间距哈希变化时重排（flow.go:367-406）。
     实测 ready 后实窗 30 个真实帧、headless 两次 500 ms Advance 后
     全部节点位置零漂移；反之若生产卡片（Task 17）让 Content 自然测量，
     文本长短差异会触发 resized→重排——定死尺寸正是防这个，探针常量
     `probeCardWidth=168 / probeCardHeight=46` 与 Task 17 生产卡片同值。
  2. 规模曲线近似线性：功 median 0.40→1.90→3.40~3.50ms（50/300/600），
     斜率 ~5-6 µs/节点。600 节点只占 16 ms 预算的 ~22%，Task 19 的
     架构图主视图全量渲染 600 节点无需 LOD/虚拟化即可保帧；§9.3 引用的
     "1000 静态 label 已花 397 µs" 口径与本次实测同数量级且仍是下界
     （卡片+边全套 600 节点整帧 3.5~4.1 ms）。
  3. 需要人工定夺的决策点（性能门槛不受影响）：600 节点 fit zoom 触到
     minZoom=0.1 下限，说明默认"整图 fit"不可读。若按 spec §12
     "性能过、可读性差"分支处理，方向是默认只画 Container 层、双击下钻
     Component（Task 17 加 `showComponents bool`）——按 brief"先报告
     再加"，本条目即报告，探针与生产均未加该开关。pan/zoom/Reveal 的
     局部可读性不受影响（zoom 上限 2.5）。
  4. >16ms 孤立帧定性为显示同步等待（present 相位占比 75%~98%、同帧功
     正常、no GC、紧随引擎自报 catch-up skip 帧、50 节点也出现），
     属 OS/WindowServer 环境毛刺。任何原生渲染层（含 WebView 替代方案）
     都无法豁免"每一帧都不许 >16ms"的口径；按计划的门槛口径（稳定后
     每帧耗时中位数）通过，且最差"功"帧 6.6 ms 亦有 2.4 倍余量。
  5. 与 brief 骨架的 API 偏差记录（均按模块缓存实测源码执行，brief
     Step 1 预先允许）：mygo@v0.5.5 不存在 `Window.SetContent` 与
     `ui.Context.Frame()`；改用 `mygo.WindowOptions{Content: ui.View(fn)}`
     注册内容回调（content.go:21/api_gen.go:2518、window.go:168），
     帧计数由探针自持、以 `c.AnimationFrame()`（ui/context.go:372，
     flow 未就绪时内部同款调用）逐帧驱动 vsync 真实帧；drift 对比帧号
     取 readyAt+5 → readyAt+35（≥30 帧 ≈500 ms，即 brief 的 Advance
     等价操作），另在 `ui.NewTester` 里做字面意义的两次
     `tt.Advance(500ms)` 双重证据；主题按 flow 官方 example 以
     `theme.Apply(c)` 应用。go.mod/go.sum 零变更，三个 pin 无漂移。

## PoC-5 单事务写快照（2121 条 Statement）

- 结论：**PASS**。门槛"单事务 2100 条 `Statement` 不失败且可接受地快"达成：
  实测口径是单事务 2121 条（1 快照 + 620 元素 + 1500 关系）全部 COMMIT 成功、
  零失败；空库单快照写 4 次采样中位 ≈16.7 ms（14.7~28.3 ms），同一库连写
  20 快照（库随写放大）三次跑次均值 23.2~41.8 ms/op。未触发失败/过慢分支，
  分块提交 + 自实现 Savepoint 的失败分支保持未启动。"可接受地快"无门槛
  数字，判断依据（全部对应下方"数字"行）：生产路径每次扫描只写一个快照，
  该写相对 PoC-3 记过的 `packages.Load` 5~9 s 扫描墙钟是 ~300 倍小的零头，
  且连写放大到 41.8 ms/op 仍被异步扫描链路（§10 取消/进度）完全掩盖。
- 数字（`BenchmarkSaveSnapshot`，`internal/core/store/bench_test.go`；
  条件：Apple M2 Max / darwin/arm64、go1.27.2、`CGO_ENABLED=0`、
  mygo v0.5.5 `plugins/sqlite`（native libmygo-sqlite3，PoC-2 记录的
  release 3.53.4-1）、库文件在 `t.TempDir()`、连接仅 `store.Open` 的
  PRAGMA（foreign_keys=ON）、驱动 `Transaction` 内部 BEGIN IMMEDIATE；
  命令 `go test ./internal/core/store/ -bench BenchmarkSaveSnapshot
  -benchtime {1x,20x} -run '^$'`）：
  - 语句数构成：1（INSERT snapshot）+ 620（INSERT arch_elements）+
    1500（INSERT arch_relationships）= 2121 条，恰为 §6.4 规模
    （≈600 Component + System/Container，≈1500 边）的合成快照，行形状
    （methods JSON 列、weight、TEXT id）与真实扫描产出同构。
  - `benchtime 1x`（新库只写 1 个快照，共 4 次跑次）：
    16.5 ms / 16.9 ms / 14.7 ms / 28.3 ms，中位 ≈16.7 ms；其中首个带
    `-benchmem` 的 1x 跑次记得 2,155,544 B/op、61,626 allocs/op。
  - `benchtime 20x`（同一库连写 20 个快照、累计 ≈42,420 条子行，共 3 次
    跑次）：均值 23.2 ms/op / 26.5 ms/op / 41.8 ms/op（库增长带来的
    索引维护计入其中）。
  - 失败次数：0。全部 7 个 bench 跑次合计 26 次单事务 2121 语句写，
    `b.Fatal` 未触发、每次 COMMIT 成功、跑次全 PASS。
  - 原子性（单测，非基准）：`TestSaveSnapshotRollsBackCompletely` PASS——
    第 1 条语句违反 `architecture_snapshots.repo_id` 外键即整批回滚，
    architecture_snapshots / arch_elements / arch_relationships 三张表
    `count(*)` 均回读为 0。
  - 检索上限（单测）：`TestSearchElementsHonoursLimit` PASS——60 个同名
    候选经 `LIMIT ?`（`maxSearchResults = 50`）截回 50 条。
- 影响：
  1. W2 的"一次扫描 = 一个事务"以 `DB.Transact` 单事务落地并被实证：驱动
      对任一条失败整批回滚（`database.go:165-229`，见上方回滚用例），
      "半成品快照不可见"在单事务路径内天然成立，无需引入失败分支的
      Savepoint 规则。
  2. Task 8~11 的扫描产出可直接经 `SaveSnapshot` 落库，Task 12/13 的图
      重建走 `Elements`/`Relationships`：读回序由 `ORDER BY element_id`
      与 `ORDER BY from_id, to_id, relationship_type` 固定（round-trip
      用例断言首位元素/关系），golden 比对前无需另行排序即可得确定序。
  3. 对 brief 代码块的三处自身缺陷做了最小修正（语义不变，不改则无法
      编译/过 vet）：`relationship.go` 块内未使用的 `sqlite` import 删除；
      `rows_test.go` 的 `TestEscapeLike` 三个通配样例 want 缺 `%` 前后缀、
      与比较式 `got = "%"+escapeLike(in)+"%"` 矛盾，want 补全为整段模式；
      `snapshot_test.go` 回滚断言对 `num` 的 float64 返回用 `%d`（vet
      报错）改 `%v`。
  4. Task 2/6 审查挂账的三项清理完成：`db_test.go` 临时 `num` 副本（含
      `t := &testing.T{}` 死分支）删除、"表名故意带注入形状的字符"注释
      更正为绑定值；`newTestDB` 改收 `testing.TB` 供基准复用；
      `migrate_test.go` 测试专用 `str`（panic 版）删除、归并到
      `rows.go` 正式版，既有迁移/级联用例复跑 PASS。
