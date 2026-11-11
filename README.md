# OpenArchitect

面向一人公司（OPC）的开源桌面工具：自动发现你机器上的多个代码仓库，维护跨项目的
统一架构图，并把它当作一片**持续生长的森林**来观测——仓库自动检入、架构随 commit
自动成长，智能体改过的地方在图上结果子。

- 回答两个问题：**改这里会坏哪里**（影响分析）、**哪里在动、谁在动**（生长与归因）。
- 本地优先：数据只在本地 SQLite，无服务端，离线可用；MIT。
- 观测为主，不执行：自动化的只有 `git fetch` 与扫描两个纯观测动作。

## 快速上手

```sh
go build ./cmd/openarchitect   # 桌面 app
go build ./cmd/oac             # headless CLI

oac sync ~/code                # 发现仓库、检入漂移、记录 commit 与归因
oac graph <repo-id> --json     # 架构图数据
oac impact <element-id> -d 2   # 改这里会坏哪里
oac update --check             # 查询新版本（不带 --check 直接安装）
```

## 安装与保持更新

从 [GitHub Releases](../../releases) 下载对应芯片的 tar.gz（arm64 / amd64），
解压出 `oac` 放进 PATH、`OpenArchitect.app` 拖进「应用程序」即可。之后的更新闭环：

- 发版：推 `v*` 标签 → CI 自动构建双架构、发 Release（附 sha256 checksums，
  说明取自 CHANGELOG；标签与版本常量不一致会直接失败）。
- 消费：`oac update` 或桌面 app 设置页「检查更新 / 安装更新」。下载先验
  checksum 再落盘，换身用 rename 原子替换、失败回滚；**重启后生效**。
- 更新只在用户明确点下时联网，自动观测路径永远离线（I2′）。
- 仓库若为私有，检查前设 `OPENARCHITECT_GITHUB_TOKEN` 即可。

## 文档

设计基线见 `docs/superpowers/2026-11-10-project-setup.md`（PM × 架构师双视角），
各子系统按 `docs/superpowers/specs/` → `plans/` 演进。

## 开发

```sh
go vet ./... && go test ./...
```
