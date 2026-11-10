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
```

## 文档

设计基线见 `docs/superpowers/2026-11-10-project-setup.md`（PM × 架构师双视角），
各子系统按 `docs/superpowers/specs/` → `plans/` 演进。

## 开发

```sh
go vet ./... && go test ./...
```
