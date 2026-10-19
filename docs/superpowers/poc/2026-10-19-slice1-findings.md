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
  内存代价约 +350~550 MB HeapAlloc（mygo ~275→672 MB、onevra ~84→632~672 MB），
  均在门槛内；(b) §6.4 的 `depends_on`/`calls` 边要求跨包类型对象同一性
  （`types.Info.Uses` 解析到别的包的 `*types.TypeName`），brief 自己的判断
  （"NeedTypes 在类型检查跨包时基本必须依赖它"）与实测一致——去掉 NeedDeps
  后本包导出类型计数不变，但那只证明"数数"不受影响，边生成需要依赖类型完整
  加载，不值得为省内存赌在 Task 10 的正确性上；(c) 模式切换本身会让 `go list`
  按新驱动参数重跑，去掉 NeedDeps 的首跑反而最慢（25.4 s / 24.8 s），
  "收窄即加速"的预期在真实缓存行为下不成立。
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
    named struct/interface 类型**，未扣 main 包类型与 `_test.go`（§6.3 的
    Component 排除集会略低于此数），即 519/138 是 Component 数的上界。
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
