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
