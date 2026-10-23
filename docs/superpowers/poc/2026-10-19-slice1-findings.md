# 切片 1 PoC 实测（2026-10-23 起，Task 1 记录）

## PoC-1 mygo 原生骨架 + onevra 侧栏窗口
- **结论：通过。** `mygo init -template native` 脚手架 → 自建模块 `github.com/futuremeng/OpenArchitect` → 精确 pin `mygo v0.5.5` + `onevra source@v0.2026.12` → `go tool mygo dev` 起窗口成功。
- **数字：** `go tool mygo dev` 首次构建 896 ms；`go tool mygo doctor` 全绿（bun / makensis / osslsign / codesign 缺失均为可选项，原生 UI 路线不需要）；dev 进程持续存活无报错；`CGO_ENABLED=0 go build .` 通过。
- **肉眼验收：** 窗口标题 OpenArchitect，左侧 220 DIP 侧栏「架构图 / 仓库 / 设置」，右侧居中「尚未扫描任何仓库」；关窗进程退出。
- **影响与决策：**
  1. 脚手架模板的 go.mod 引用 **mygo v0.6.3**（比 spec §2.1 验证组合新）。本切片按 plan 维持 pin **v0.5.5**（与 onevra v0.2026.12 的已验证组合）；升级 v0.6.3 是后续独立决策，跑过 §11 ui 测试再动。
  2. 脚手架产物去留：**采纳** `mygo.json`（`name` 改为 `OpenArchitect`，`identifier` 改为 `com.futuremeng.openarchitect`——模板默认值是 `com.example.*`，不改会撞 bundle id）、`.gitignore`（与仓库原有规则合并）、`resources/icon.png`（打包默认图标，占位无害）；**不采纳** 模板 `go.mod`（模块名不符）、`main.go`/`main_test.go`（counter 演示，与 plan Step 5 的侧栏骨架不符）、`.agents/skills`（模板自带 agent 技能，与本项目无关）。
  3. `tool github.com/egoist/mygo/cmd/mygo` 指令已复刻进我们的 go.mod——`go tool mygo dev/build` 可用的前提。
  4. `mygo.json` 没有 `productName` 键；等价字段是 `name`（plan Task 1 Step 4 预留的分叉在此定稿）。