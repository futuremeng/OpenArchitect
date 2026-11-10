# 工具链定位（2026-11-10）：paperclip · Orca · OpenArchitect

## 1. 三个工具，一句话各管一段

| 工具 | 管什么 | 视角 |
|---|---|---|
| **paperclip** | 给 AI agent 派活、管活（work 侧） | 任务 |
| **Orca**（Orca.app） | worktree / 终端 / agent 编排（dev 侧），spawn codex/claude 干活 | 过程 |
| **OpenArchitect** | 多仓架构 + 果实（谁改了哪块架构）的**事后观察** | 结果 |

用户与前两者仍在磨合期；OpenArchitect 是它们的**补充工具**——不抢它们的
状态所有权，只把它们的产出变成可读的架构叙事。

## 2. 配合四原则（架构约束）

1. **观察单向（I2′ 的推论）**：OpenArchitect 永远不驱动 agent、不写
   paperclip/Orca 的任何状态。自动动作依旧只有 `git fetch` 与 `scanner.Run`。
2. **归因是唯一已落地的耦合面**：两个工具 spawn 的 claude/codex/opencode
   已经通过 commit trailer 被 `growth.Attribution` 认下。`AgentTrailers`
   是数据不是代码——工具将来若自带署名（如 `Co-authored-by: orca`），
   加一行即可，不开新代码路径。
3. **worktree 不是新树**（本批已落地）：Orca 的标准布局是主检出旁的兄弟
   worktree，与主检出共享对象库。发现层认得「`.git` 是文件」这一族
   （linked worktree / submodule），整树跳过——否则同一批 commit 记两遍、
   果实数翻倍。观察以主检出为准。
4. **source 适配器的第一候选是 `orca` CLI**：本地 os/exec 调用（与 gitcmd
   同型，R3 合规），只读 worktree/automation 状态来丰富「生长」叙事。
   网络例外仍只留给 Gitea/GitHub source，且只在 `internal/forest/source/**`。

## 3. 磨合期约定

- 不假设 paperclip/Orca 的数据格式与版本；**git 历史之外的一切都是
  可选的只读增强**——接口变了就降级（少一块信息），不许 fail 一次 sync。
- 果实归因继续只吃 git 历史：这是两个工具都稳定产出的公共地面。
- 任何「OpenArchitect → 工具」方向的写操作（比如替 Orca 建 worktree、
  替 paperclip 建任务）都不进路线图，除非用户显式改规则。

## 4. 落点清单

- ✅ 发现层跳过 linked worktree / submodule（`gitcmd.IsLinkedWorktree` +
  discover，`TestDiscoverSkipsLinkedWorktrees` 钉住）
- ⏸ `orca` source 适配器（只读 worktree→repo 映射，把「哪个 worktree
  在改哪个仓」画进仓库页）——1.2 候选
- ⏸ paperclip 任务 ↔ 仓的映射展示（若 paperclip 导出任务清单文件，
  按只读增强接入）——观察其格式稳定后再排
- ⏸ `AgentTrailers` 增加 orca/paperclip 自身署名条目——等真实 trailer 出现
- ⏸ 只读 Web 观察视图（若立项，组件层候选 affectUI，见
  research/2026-11-10-affectui-survey.md——词汇表与 IA 参照，非依赖）