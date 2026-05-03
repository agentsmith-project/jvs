# Repo And Workspace Lifecycle Product Plan

**Subtitle:** planned repo and workspace lifecycle operations handoff.

**Status:** active clean redesign, non-release-facing, not part of the v0 public contract. Clean future milestone handoff.

**中文名:** Repo 与 workspace 生命周期操作。

本文面向产品、工程、安全 UX 和 QA，定义 JVS repo/workspace 生命周期操作的产品口径和验收边界。它只讨论未来 handoff 的文档和行为合同，不包含实现代码，也不更新 release-facing CLI spec 或 user docs。

Scope note: 本 handoff 描述 embedded/current lifecycle model：active `.jvs` 位于 project folder，main workspace folder 等于 repo root，main workspace registry `RealPath` 等于 repo root。不要把本文的 folder-based repo model 套到 `separated_control` mode；separated control root repo 由 `docs/26_EXTERNAL_CONTROL_METADATA_PRODUCT_PLAN.md` 覆盖，其 Phase 1 lifecycle commands 必须 unsupported fail closed，Phase 2 需要重写 separated lifecycle matrix。

## 文档验收标准

这份 handoff 应满足：

- 产品能直接解释：JVS 管理两类真实对象，repo/project 和 workspace。repo_id 是稳定身份，不随 move/rename 改变；workspace 有稳定 name 和真实 folder path。
- 工程能直接拆阶段：先收敛命令语义、状态更新矩阵和 fail-closed 安全门，再实现 preview/run、durable lifecycle operation journal、atomic rename、repo detach archive 和 doctor 分层。
- QA 能直接抽矩阵：覆盖 workspace rename name-only、workspace move path-only、workspace delete destructive preview/run、repo move/rename discovery、cwd safety guard、repo detach archive、folder basename != workspace name、manual bypass 后 doctor drift 表、journal/fault injection。
- 安全 UX 能直接拒绝危险自动化：不猜路径、不扫全盘、不抢 live repo、不重写历史、不把 detach 伪装成 delete、不删除用户 project working files。
- 文档明确：正常计划变更必须通过 lifecycle commands；doctor 只诊断并保守修复用户绕开 JVS 文件系统操作导致的 drift，不是正常 move/rename/delete/detach 入口。
- 文档避免把 `workspace reconnect` 或旧 `remove` 心智保留为日常主命令、主产品 handoff 或 release-facing 文档口径。

## 一句话目标

JVS 提供低心智的 repo/workspace lifecycle commands，让用户显式 move、rename、delete 或 detach 真实 JVS 对象，并在操作后保持 `status`、`workspace list`、`workspace path`、`save`、`history` 和 `doctor --strict` 健康。

## 背景

JVS 的核心对象不是抽象 branch，而是用户能在文件系统里看到的 folder：

- repo/project：一个带 `.jvs` control plane 的 JVS project folder，拥有稳定 `repo_id`、adopted main workspace、save point history 和 registry。
- workspace：一个真实 working folder，拥有稳定 workspace `name`，并在 registry 中记录真实 `RealPath`。external workspace 还在 workspace root 里有轻量 locator，帮助 discovery 找回 repo。

用户会用 Finder、`mv`、`rm`、备份恢复、复制目录或同步工具绕开 JVS。绕开后，JVS 需要能诊断 drift，并在证据足够时做保守 metadata repair。但正常计划变更不应该依赖 doctor。用户想移动 repo、改 workspace 名字、移动 workspace folder、删除 workspace folder 或停止 JVS 管理 repo 时，应该有清晰的 lifecycle command。

本设计把上一轮的 external workspace reconnect 心智收回到底层：stale external locator 是 drift 的一种症状，不是产品主路径。主路径是 lifecycle commands 主动更新 registry、locator 和控制面。

## 非目标

本里程碑不做：

- 不把 `workspace reconnect` 发布为日常用户主命令或主 handoff。
- 不发布新的 `workspace remove` 或 `repo remove` 口径。
- 不兼容未发布的旧 `workspace remove --force` 行为；若现有实现或文档有该行为，未来按 clean contract 重构，不做兼容兜底。
- 不把 doctor 设计成正常移动、重命名、删除或停止管理入口。
- 不自动搜索 repo，不扫描全盘，不根据 basename、最近使用记录、父目录名或环境变量猜目标。
- 不抢走另一个 live repo 仍承认的 workspace。
- 不在 move/rename/delete/detach 中修改用户文件内容、save point descriptors、payload storage、audit/provenance 或历史语义。
- 不在 GA 首版 repo/workspace move 中支持跨设备 copy+delete。same-filesystem no-overwrite atomic rename 是首版发布线；跨设备明确失败或留到后续阶段。
- 不实现 destructive `repo delete`。删除用户 project files 属于 future 单独高风险命令，需要更强确认和独立验收。
- 不实现 `workspace detach` 首版。停止管理但保留 external workspace folder 是 future 需求；首版用非目标约束收敛范围。
- 不发布新的 public `lifecycle resume` 命令；pending lifecycle recovery 的首版入口固定为重跑原 lifecycle command。
- 不为 `repo move`/`repo rename` 设计 offline-pending external workspace 首版；registered external workspace connection 证据不完整时先 fail closed。
- 不更新 release-facing CLI spec、user docs 或 public contract，因为本文是 future milestone handoff。

## 命令语义决策

旧稿最大问题是同一个 `remove` 同时表达了 "删除工作目录" 和 "停止 JVS 管理"。本里程碑必须拆开：

- `delete` 只表示删除用户可见的真实 working folder，且必须 destructive preview/run。
- `detach` 只表示停止 JVS 管理，但保留用户 project working files。
- `remove` 不作为新公开口径。

首版公开命令只包含：

- `jvs workspace move <name> <new-folder>`：移动 workspace folder，workspace name 不变。
- `jvs workspace rename <old-name> <new-name> [--dry-run]`：只改 workspace name，folder path 不变。
- `jvs workspace delete <name>`：删除 workspace folder 和 registry entry。
- `jvs repo move <new-folder>`：移动整个 JVS project folder。
- `jvs repo rename <new-folder-name>`：同父目录 repo folder 改名，是 repo move 的 basename-only 糖衣。
- `jvs repo detach`：停止 JVS 管理当前 repo，保留 project working files。

Future 明确保留但不进入首版：

- `jvs workspace detach <name>`：停止管理 external workspace 但保留 workspace folder。
- `jvs repo delete`：删除用户 project files 的高风险命令。

## 用户心智

推荐用户心智：

- JVS 管理 repo/project 和 workspace 两类真实对象。
- `repo_id` 是 project identity；移动或改 folder 名不会产生新 repo_id。
- main workspace folder 就是 repo root。
- main workspace name 是保留名且 immutable；`jvs workspace rename main ...` 必须 fail closed。
- workspace `name` 是用户给这个 workspace 的稳定名字；移动 folder 不改变 name。
- workspace folder path 是真实位置；重命名 workspace 不应该偷偷移动 folder。
- workspace folder basename 不必等于 workspace name；两者不同是健康状态，不是 drift。
- workspace 有独立 JVS name，所以 `workspace rename` 只改 JVS name，不动 folder。
- repo 没有独立 display name；用户看到的 repo 名就是 folder basename，所以 `repo rename` 是同父目录 folder basename-only 改名。
- 正常计划变更用 lifecycle command。
- doctor 处理的是异常漂移：我绕开 JVS 操作后，doctor 告诉我发生了什么，能修的保守 metadata 才修。
- 删除 workspace folder 或停止管理 repo 前，我先看 preview，再用 plan id run。
- JVS will not move/delete the folder you are currently standing in；用户不需要理解 cwd、inode 或 shell 细节，JVS 自动检测并给出可复制的安全下一步命令。

最重要的普通话术：

```text
Renamed workspace "experiment" to "review".
Folder path unchanged: /work/experiment
```

```text
Moved workspace "experiment".
Old folder: /work/experiment
New folder: /ssd/experiment
Workspace name unchanged.
```

```text
Moved JVS project.
Repo ID unchanged: repo-...
Updated workspace connections: 2
```

```text
Detached JVS project.
Project files kept in place: /work/project
JVS project archive: /work/project/.jvs-detached/repo-...-<operation-id>-20260503T120000Z/
```

## 对象模型和权威来源

### Repo/Project

Repo/project 是一个 JVS 管理的 project folder。它的关键身份和状态：

- `repo_id` 是稳定身份。`repo move`、`repo rename` 和 `repo detach` 必须保留它。
- registry 是 workspace 列表、workspace `name` 和 workspace `RealPath` 的权威来源。
- save point history、payload storage、audit/provenance 不因 lifecycle move/rename/delete/detach 被重写。
- active repo 的 ordinary discovery 以 repo root 下 active `.jvs` 为入口。
- detached repo 的 ordinary discovery 不得把 main folder 当 active repo。

### Main Workspace Clean Contract

本产品尚未发布，因此不保留旧实现心智：

Scope note: 本节的 `main workspace folder == repo root`、main workspace registry `RealPath == repo root` 合同只适用于 embedded/current lifecycle model。separated control root mode 由 `docs/26_EXTERNAL_CONTROL_METADATA_PRODUCT_PLAN.md` 覆盖；其 Phase 1 lifecycle commands 必须 unsupported fail closed，Phase 2 再重写 separated lifecycle matrix。

- clean contract 只支持 adopted main：main workspace folder 就是 repo root。
- main workspace name 固定为 `main` 且 immutable。`jvs workspace rename main <new-name>` 必须 fail closed，并提示 `main workspace is the repo root; use jvs repo rename to rename the folder.`
- main workspace 的 registry `RealPath` 必须等于 repo root canonical path。
- legacy `repoRoot/main` fallback 是旧实现细节，必须在本里程碑移除或迁移。
- lifecycle command 不得用 fallback 路径解释 main workspace，也不得在 output 中暗示 main 是 repo root 之外的 child folder。
- 如果现有 repo 仍处于 legacy shape，进入本里程碑前必须做一次性迁移或 fail closed，并给出内部 migration issue；不能把 legacy shape 当作长期兼容合同。

### Workspace

Workspace 是一个真实 working folder。它的关键身份和状态：

- workspace `name` 是稳定名字，用户用它选择 workspace。
- `RealPath` 是真实 folder path。
- folder basename 不是 workspace identity；basename 与 workspace name 不一致时，`status`、`workspace list` 和 `doctor --strict` 应保持健康。
- main workspace 的 `RealPath` 跟 repo/project folder 一起移动。
- external workspace 的 locator 是 discovery 入口线索，至少包含 `repo_root`、`repo_id` 和 `workspace_name`。
- locator 不是第二份 durable truth，但它必须与 registry 一致，否则 discovery 和 doctor 应 fail closed，除非 pending lifecycle journal 明确要求 recovery 并提供 `recommended_next_command`。

## 推荐 CLI

### Workspace Commands

```bash
jvs workspace move <name> <new-folder>
jvs workspace move --run <workspace-move-plan-id>

jvs workspace rename <old-name> <new-name> [--dry-run]

jvs workspace delete <name>
jvs workspace delete --run <workspace-delete-plan-id>
```

语义：

- `workspace move <name> <new-folder>` 移动真实 workspace folder。workspace name 不变。
- `workspace rename <old-name> <new-name>` 只改 workspace name。folder path 不变。当前实现若会移动 external folder，必须在本里程碑重构掉。
- `workspace rename main <new-name>` 必须 fail closed；main workspace 是 repo root，folder 改名应使用 `jvs repo rename <new-folder-name>`。
- `workspace delete <name>` 删除 workspace folder 和 registry entry。它是 destructive command，必须 preview/run。
- `workspace delete` 不能删除 main workspace。
- `workspace delete` 不能删除 save point storage、save point descriptors、repo history 或 payload storage。
- `workspace detach` 是 future 非首版；首版不支持 "停止管理但保留 external workspace folder"。

`workspace rename` 是 metadata operation，可以直接执行并支持 `--dry-run`。因为它要同步 external workspace connection metadata，所以正常执行前仍必须写 durable lifecycle operation journal。

### Repo Commands

```bash
jvs repo move <new-folder>
jvs repo move --run <repo-move-plan-id>
jvs --repo <old-repo-root> repo move --run <repo-move-plan-id>

jvs repo rename <new-folder-name>
jvs repo rename --run <repo-rename-plan-id>
jvs --repo <old-repo-root> repo rename --run <repo-rename-plan-id>

jvs repo detach
jvs repo detach --run <repo-detach-plan-id>
```

语义：

- `repo move <new-folder>` 移动整个 JVS project folder，保留 `repo_id` 和 history，更新 main path 和所有 external workspace connections。
- `repo rename <new-folder-name>` 是同父目录 repo move 的糖衣。参数必须是 folder basename，不接受路径、`.`、`..`、path separator 或绝对路径。
- `repo detach` 停止 JVS 管理当前 repo，采用 preview/run。首版语义是 archive JVS project metadata，让 working files 留在原处。
- `repo detach` 不是 `repo delete`。`detach` 表示保留 working files；`delete` 才表示删除用户 project files。
- destructive `repo delete` 不进入首版，未来必须作为单独高风险命令设计。

`repo move` 和 `repo rename` 的 run 后不产生新 repo，不复制 repo，不改变 save point history。它们只是改变 project folder 真实位置。首版 run 前必须证明所有 registered external workspace connections reachable、writable、metadata well-formed 且 freshness evidence matches；任一失败都必须在移动 repo root 前 fail closed，并要求先用 doctor diagnose drift 或恢复 workspace connection。

`repo move`/`repo rename` 的 unsafe-cwd run retry 必须支持显式 repo path。用户离开 old repo root 后，ordinary discovery 可能找不到要 run 的 repo，因此 handoff 要求支持 `jvs --repo <old-repo-root> repo move --run <repo-move-plan-id>` 和 `jvs --repo <old-repo-root> repo rename --run <repo-rename-plan-id>`。这里的 `<old-repo-root>` 是 preview/run plan 记录的 source repo root，不是让用户重新发现 repo。

## Direct vs Preview/Run

| Command | 执行模式 | 原因 |
| --- | --- | --- |
| `workspace rename <old-name> <new-name>` | 直接执行 + `--dry-run` | metadata operation，folder path 不变；正常执行仍写 journal |
| `workspace new` | 继续直接创建 + `--dry-run` | 创建新目标，不移动或删除已有真实对象 |
| `repo clone` | 继续直接创建 + `--dry-run` | 创建新目标 repo，不移动或删除源对象 |
| `workspace move <name> <new-folder>` | preview/run | 移动已有真实 workspace folder |
| `workspace delete <name>` | preview/run | 删除已有 workspace folder 和 registry entry |
| `repo move <new-folder>` | preview/run | 移动已有 repo/project folder 和控制面 |
| `repo rename <new-folder-name>` | preview/run | repo move 糖衣，仍移动已有 repo/project folder |
| `repo detach` | preview/run | 停止 JVS 管理但保留 project working files |

Preview/run 规则：

- preview 只做 discovery、身份校验、安全检查、transfer/move/delete/detach plan 和冲突检查，不移动或删除对象，不写 lifecycle operation journal。
- preview 可以从将被 move/delete 的 repo/workspace 内运行；因为 preview 不 mutation，它只需要在输出里标记 run 是否需要换到安全目录。
- preview 输出 plan id、对象身份、旧路径、新路径或删除/归档范围、将更新的 metadata、不会修改的历史范围。
- run 开始前必须重新校验所有身份、路径、locks、mtime 或等价 freshness 证据，并执行 cwd safety guard，不能盲信 preview。
- unsafe cwd guard 是 run preflight 的一部分；如果当前 shell folder 位于即将被移动或删除的真实 folder 内，run 必须在写 journal 或任何 mutation 前 fail closed，并输出 copyable safe next command。
- run 写入 durable lifecycle operation journal 后才允许做第一个 mutation。
- plan id 不能跨 repo 使用，不能在 repo_id mismatch 时运行。
- run 成功后 plan 标记 consumed；run 失败时保留可诊断状态，并通过 journal 支持 resume forward 或 fail closed。
- pending lifecycle recovery 不通过新的 public `lifecycle resume` 命令。preview/run operation 的 status/doctor recommended next command 固定为重跑原 `--run <plan-id>`，例如 `jvs repo move --run <repo-move-plan-id>`、`jvs workspace delete --run <workspace-delete-plan-id>`、`jvs repo detach --run <repo-detach-plan-id>`。

GA 首版 move 只承诺 same-filesystem no-overwrite atomic rename。跨设备 move 必须 fail closed，并解释后续版本可能支持 reviewed copy+delete。

## CWD Safety / Safe Invocation Context

JVS 子进程不能改变用户父 shell 的 cwd。用户如果站在即将被移动或删除的 folder 里，JVS 即使能完成 filesystem mutation，也会把用户 shell 留在一个已经移动、删除或语义混乱的位置。产品规则固定为：

```text
JVS will not move/delete the folder you are currently standing in.
```

用户不需要理解 cwd、inode、canonical path 或 discovery 细节。JVS 必须自动检测当前进程继承到的 shell folder，并给出一条可以复制执行的安全下一步命令。

Preview 规则：

- preview 可以在任何 repo/workspace 内运行，包括即将被 move/delete 的 source folder 内。
- preview 不 mutation，所以不得要求用户先 `cd` 到别处才能生成 plan。
- 如果 preview 发现当前 folder 会让后续 run 不安全，preview 仍然生成 plan，并在 human/JSON output 中提供 safe run command。

Run 规则：

- run 必须在写 durable lifecycle operation journal 或任何 mutation 前检查当前 cwd 是否位于 affected tree。
- 如果 cwd 在 affected tree 内，run fail closed，错误码为 `E_LIFECYCLE_UNSAFE_CWD`，并说明 `No files were changed.`
- safe next command 必须包含 `cd <safe-parent-or-repo-root>` 后重跑原 plan id；repo move/rename 必须额外包含显式 `--repo <old-repo-root>`。
- 如果 cwd 不在 affected tree 内，run 按普通 preview/run freshness、identity、lock 和 path safety 规则继续。
- 如果 JVS 无法可靠判断 cwd 是否在 affected tree 内，按 unsafe 处理并 fail closed。

具体操作规则：

- `workspace rename` 不需要 cwd guard，因为 folder path 不变。
- `workspace move`：如果 cwd 在 source workspace subtree 内，run fail closed before mutation；输出建议 `cd <safe-parent-or-repo-root>` 后重跑 `jvs workspace move --run <workspace-move-plan-id>`。如果 cwd 不在 source workspace 内则正常继续。
- `workspace delete`：如果 cwd 在 target workspace subtree 内，run fail closed before mutation；输出建议 `cd <safe-parent-or-repo-root>` 后重跑 `jvs workspace delete --run <workspace-delete-plan-id>`。
- `repo move`：如果 cwd 在 source repo subtree 内，run fail closed before mutation；输出建议 `cd <safe-parent>` 后重跑 `jvs --repo <old-repo-root> repo move --run <repo-move-plan-id>`，并在成功后建议进入 new repo path 或原来的 same relative subfolder。
- `repo rename`：如果 cwd 在 source repo subtree 内，run fail closed before mutation；输出建议 `cd <safe-parent>` 后重跑 `jvs --repo <old-repo-root> repo rename --run <repo-rename-plan-id>`，并在成功后建议进入 new repo path 或原来的 same relative subfolder。
- `repo detach` 通常不需要 cwd guard，因为 project files 和 repo folder stay in place；但 success output 必须明确说明当前 folder 成功后不再是 active JVS repo。
- future `repo delete` 必须要求 cwd outside target repo，且不能复用 `repo detach` 的低风险话术。

## Workspace Lifecycle Semantics

### `workspace move`

`workspace move <name> <new-folder>` 移动真实 workspace folder，workspace name 不变。

必须做：

- 通过 registry 找到 workspace name。
- 拒绝 main workspace；main folder 的移动必须走 `repo move`。
- run 前检查当前 cwd；如果 cwd 在 source workspace subtree 内，必须在写 journal 或移动 folder 前 fail closed，并输出 `cd <safe-parent-or-repo-root>` 后重跑 `jvs workspace move --run <workspace-move-plan-id>`。
- 校验 `<new-folder>` 不存在，且 parent 可写。
- 校验 source workspace folder 和 destination parent 位于同一 filesystem，首版使用 atomic no-overwrite rename。
- 更新 registry `RealPath` 为 new folder canonical path。
- external workspace connection 的 repo identity 和 workspace name 保持不变；如果 connection metadata 随 folder 一起移动，移动后必须重新验证。
- 移动后 `workspace path <name>` 输出新 path，`workspace list` 健康。

不能做：

- 不能改变 workspace name。
- 不能要求 folder basename 等于 workspace name。
- 不能重写 save point history 或 workspace provenance。
- 不能 fallback 到 copy+delete。
- 不能覆盖已有目标。

### `workspace rename`

`workspace rename <old-name> <new-name>` 只改 workspace name，folder path 不变。

必须做：

- 在普通 old-name selector 校验前，先检查是否存在匹配 old/new identity 的 pending rename journal；如果 registry 已经不再承认 old selector，也必须能根据 journal 继续同一次 rename recovery，而不是直接报 workspace not found。
- 校验 old name 存在，new name 未被占用且符合 workspace name 规则；但 pending rename journal recovery 使用 journal 中已记录的 old/new identity 做 selector。
- 拒绝 main workspace rename；`jvs workspace rename main <new-name>` fail closed，并提示 `main workspace is the repo root; use jvs repo rename to rename the folder.`
- 不需要 cwd guard；rename 是 name-only metadata operation，folder path 不变。
- preflight external workspace connection reachable、writable、metadata well-formed 且 freshness evidence matches；如果不可达、不可写、malformed 或 mismatch，rename fail closed。
- 写 durable lifecycle operation journal。
- 更新 registry entry name。
- 更新该 workspace locator 的 `workspace_name`，如果该 workspace 是 external workspace。
- 保持 registry `RealPath` 不变。
- 保持真实 folder path 不变。
- 运行后 `workspace path <new-name>` 指向原 folder。
- 运行后旧 name 不再作为 workspace selector 成功。
- 如果中途失败，status/doctor 必须报告 pending lifecycle op 和 `recommended_next_command`，不能把半更新状态当 healthy。
- pending rename recovery 的用户入口固定为重跑同一条 direct command，例如 `jvs workspace rename <old-name> <new-name>`；不引入 public `lifecycle resume`。

当前实现已知冲突：如果 `workspace rename` 现在会移动 external folder，或只更新 registry 不更新 locator `workspace_name`，必须在本里程碑按 name-only 语义重构。

### `workspace delete`

`workspace delete <name>` 删除 workspace folder 和 registry entry，使用 preview/run。

必须做：

- preview 展示将删除的 workspace folder、workspace entry 和 workspace connection metadata。
- preview/run 都明确这是 destructive command。
- run 前检查当前 cwd；如果 cwd 在 target workspace subtree 内，必须在写 journal 或删除 folder 前 fail closed，并输出 `cd <safe-parent-or-repo-root>` 后重跑 `jvs workspace delete --run <workspace-delete-plan-id>`。
- run 删除 workspace folder 和 workspace registry entry。
- run 后 `workspace list` 不再显示该 workspace。
- save point storage、save point descriptors、payload storage 和 repo history 不由 delete 清理，留给 cleanup 的 reviewed flow。
- 如果 workspace 有 unsaved changes，默认失败，提示先 save 或显式未来强制选项。首版不需要设计 force。

不能做：

- 不能删除 main workspace；main workspace 是 repo root。
- 不能删除 save point storage。
- 不能重写其他 workspace 的 current pointer 或 history。
- 不能把 "停止管理但保留 external workspace folder" 塞进 delete。该需求属于 future `workspace detach`。
- 不能兼容旧 `workspace remove --force` 的未发布行为。

## Repo Lifecycle Semantics

### `repo move`

`repo move <new-folder>` 移动整个 JVS project folder，保留 repo identity。

必须做：

- preview 展示 old repo root、new repo root、repo_id、main workspace path 更新和 external workspace connection 更新列表。
- preview 如果发现当前 cwd 在 old repo root subtree 内，仍生成 plan，但必须提示 safe run command：`cd <safe-parent>` 后执行 `jvs --repo <old-repo-root> repo move --run <repo-move-plan-id>`。
- preview 和 run 都必须校验所有 registered external workspace connections reachable、writable、metadata well-formed 且 freshness evidence matches。
- 任一 registered external workspace connection 不可达、不可写、malformed 或 freshness mismatch，必须在移动 repo root 前 fail closed，并提示先用 doctor diagnose drift 或恢复 workspace connection。
- run 前检查当前 cwd；如果 cwd 在 old repo root subtree 内，必须在写 journal 或移动 repo root 前 fail closed，并输出显式 `--repo <old-repo-root>` 的 safe next command。
- run 使用 same-filesystem no-overwrite atomic rename 移动 repo/project folder。
- main workspace `RealPath` 更新为 new repo root canonical path。
- 所有 registered external workspace locators 的 `repo_root` 更新为 new repo root。
- registry 中 external workspace `RealPath` 保持不变。
- `repo_id`、save point history、workspace names、external workspace paths 全部不变。
- run 后从 main folder 和 external workspaces 运行 `status`、`save`、`history` 和 `doctor --strict` 都应健康。
- run 成功 output 应建议进入 new repo root，或如果用户原来位于 old repo root 的子目录，则建议进入 new repo root 下同一个 relative subfolder。
- 如果 repo 已移动但 external workspace connection 尚未全部更新，从新 repo root 或可证明的 external workspace 运行 status/doctor 应报告 pending lifecycle op，并给出 `recommended_next_command`，例如 `jvs repo move --run <repo-move-plan-id>`。

不能做：

- 不能生成新 repo_id。
- 不能把 repo move 当作 clone。
- 不能在 external locator 缺失或 malformed 时猜测修复；该场景应 fail closed 或由 doctor 给出保守 repair。
- 不能设计 offline-pending 首版：未能验证任一 registered external workspace connection 时，不能先移动 repo root 再期待后续补写。

### `repo rename`

`repo rename <new-folder-name>` 是 `repo move` 的同父目录改名糖衣。

规则：

- `<new-folder-name>` 必须是 basename。
- 不接受绝对路径、相对路径、path separator、`.` 或 `..`。
- old parent 保持不变，目标 path 是 `parent(old_repo_root) / <new-folder-name>`。
- preview/run、状态更新、安全边界与 `repo move` 相同。
- 如果 cwd 在 old repo root subtree 内，run 必须在 mutation 前 fail closed；safe next command 使用 `jvs --repo <old-repo-root> repo rename --run <repo-rename-plan-id>`，成功后建议进入 renamed repo path 或 same relative subfolder。
- implementation 可以复用 repo move planner，但 human/JSON output 必须保留 `repo rename` command identity。

这让用户表达低心智操作：

```bash
jvs repo rename better-name
```

而不是要求用户手写完整 sibling path。

### `repo detach`

`repo detach` 停止 JVS 管理当前 repo，使用 preview/run。首版策略必须固定为 archive control plane，不 delete。

必须做：

- working files 留在原处。
- preview 展示 repo root、repo_id、archive path、将保留的 project files、将 disabled/orphaned 的 external workspace connections。
- run 创建 no-overwrite archive directory，路径固定包含 journal `operation_id`：`<repo-root>/.jvs-detached/<repo-id>-<operation-id>-<utc-timestamp>/`。
- archive path 的 `<operation-id>` 必须来自 run journal 的 `operation_id`；`plan_id` 只是 preview/run handle，二者可以相同也可以不同，所有 `archive_path` 字段都不得从 `plan_id` 派生。
- 移动 active `.jvs` 前，必须先在 archive directory 写 repo-root 可发现的 `DETACHING` marker，例如 `<archive-dir>/DETACHING`。
- `DETACHING` marker 至少包含 `operation_id`、repo_id、old repo root、archive path、expected active `.jvs` identity、registered workspace summary 和 `recommended_next_command`，例如 `jvs repo detach --run <repo-detach-plan-id>`。
- `DETACHING` 写入并 fsync marker 和 archive directory 后，才允许将 active `<repo-root>/.jvs` no-overwrite rename/archive 到 `<archive-dir>/.jvs`。
- archive 完成后写入并发布 `DETACHED` metadata，至少包含 `operation_id`、repo_id、old repo root、archive path、detached_at、tool version、registered workspace summary 和 `recommended_next_command`。
- `DETACHED` 发布后可以保留或替换 `DETACHING`，但恢复入口必须能从 repo root 做 bounded scan：`.jvs-detached/*/{DETACHING,DETACHED}`。这是 repo-local bounded scan，不是全盘扫描。
- 所有 registered external workspace locators 转为 disabled/orphaned connection metadata，用户文件不删。
- 如果任何 registered external workspace locator 不可达、不可写、malformed 或 freshness evidence 不匹配，首版 fail closed，要求先用 doctor diagnose drift 或恢复 workspace connection。
- run 后普通 discovery 不再把 main folder 当 active repo。
- run 后如果用户当前 shell 仍在原 project folder 内，success output 必须明确说明这个 folder 已不再是 active JVS repo；后续 JVS 命令需要从其他 active repo/workspace 运行或走 future explicit adopt/attach flow。
- run 后 external workspace discovery 必须看到 detached/orphaned 状态，而不是 silently stale。
- registry、save point storage 和 audit/provenance 不被重写成虚假的历史；历史随 archived `.jvs` 保留。

不能做：

- 不能删除用户 project working files。
- 不能删除 external workspace working files。
- 不能把 save point storage 当作 cleanup 直接清掉。
- 不能叫 `repo remove`。
- 不能隐式执行 future `repo delete`。

如果未来实现 destructive `repo delete`，必须使用独立命令、独立 plan id、强确认、单独安全验收和 release-facing 文档更新。`detach` 不能变成 delete 的别名。

## Durable Lifecycle Operation Journal And Recovery

这是硬合同。任何多位置 lifecycle operation 都必须有 durable journal/recovery 语义，不能只靠 best-effort cleanup。

### Journal Scope

必须写 journal 的 operation：

- `workspace move`
- `workspace delete`
- `workspace rename`，因为它要同步 external workspace locator
- `repo move`
- `repo rename`
- `repo detach`

Preview plan 只做检查，不写 journal，不做 mutation。Run 或 direct metadata operation 开始 mutation 前，必须先写 durable lifecycle operation journal。推荐路径是 active repo control plane 内的 `.jvs/lifecycle/operations/<operation-id>.json`；repo detach 成功后，journal 随 `.jvs` 进入 project archive。

每个 journal record 至少包含：

- `operation_id`
- `operation_type`
- `repo_id`
- object identity：workspace name、old/new workspace name、repo identity、main/external marker
- old path 和 new path；delete/detach 也记录 deleted/archive target path
- current phase
- per-target locator status：not_applicable、pending_marker_written、updated、disabled_orphaned、verified、failed
- freshness evidence：canonical path、platform file identity、mtime/ctime 或项目已有等价 evidence、expected metadata hash/version
- plan id 和 preview evidence hash，如果 operation 来自 preview/run
- original command identity 和 exact `recommended_next_command`
- last error、recovery message 和 fail-closed reason

`recommended_next_command` 是恢复合同的一部分：

- preview/run operation 固定推荐重跑原 `--run <plan-id>`，例如 `jvs repo move --run <repo-move-plan-id>`、`jvs workspace delete --run <workspace-delete-plan-id>`、`jvs repo detach --run <repo-detach-plan-id>`。
- 当 repo move/rename 因 unsafe cwd fail closed 时，human/JSON output 的 `safe_next_command` 必须使用显式 repo path，例如 `cd <safe-parent> && jvs --repo <old-repo-root> repo move --run <repo-move-plan-id>`；journal 中仍保留原 command identity 和 old/new repo root。
- direct `workspace rename` 固定推荐重跑同一条 `jvs workspace rename <old-name> <new-name>`。
- 首版不提供 public `lifecycle resume` 命令，也不要求用户学习内部 phase 名称才能恢复。

### Idempotent Phases

所有 phase 必须 idempotent：

- resume forward 优先。只要 evidence 能证明 commit 已发生或部分发生，下一次用户重跑原 command 时应继续到 verified/consumed；status/doctor 只负责报告 pending op 和 recommended next command。
- 只有 move 未 commit 且证据确定时，才能 rollback/no-op journal。
- commit uncertain 必须 fail closed，不能再次执行 destructive action。
- journal consumed 前，ordinary health checks 不能把半更新状态当 healthy。
- status/doctor 发现 pending lifecycle op 时，必须先报告 pending op 和 `recommended_next_command`，再讨论 drift diagnose。
- `doctor --strict` 只报告 pending lifecycle operation 和 recommended next command；它不得自动 resume lifecycle mutation。
- 任何 lifecycle resume forward 的 mutation 都必须由用户重跑原 lifecycle command 触发，而不是由 doctor 自动执行。

### Repo Move/Rename Phases

`repo move` 和 `repo rename` phases 大致固定为：

1. `prepared`
2. `repo_root_moved`
3. `main_registry_updated`
4. `external_locators_updated`
5. `verified`
6. `consumed`

`prepared` 必须完成：

- 写 repo journal。
- 验证所有 registered external workspace connections reachable、writable、metadata well-formed 且 freshness evidence matches；任一失败必须在 repo root atomic rename 前 fail closed。
- 给所有 registered external workspace connections 写 pending lifecycle marker，包含 operation id、repo_id、old repo root、new repo root、expected workspace name 和 `recommended_next_command`。
- 记录每个 target 的 freshness evidence。
- 不设计 offline-pending 首版；prepared 不能跳过任何 registered external workspace connection。

如果 repo 已移动但 locators 未全部更新：

- 从新 repo root 运行 `jvs status` 或 `jvs doctor --strict`，必须发现 journal 并提示重跑原 `--run <plan-id>`。
- 从已经写入 pending marker 且 identity 可证明的 external workspace 运行 `jvs status` 或 `jvs doctor --strict`，必须报告 pending lifecycle op 和 `recommended_next_command`。
- 如果 external workspace 只有 stale locator、没有 pending marker、也无法证明新 repo root，doctor 不能猜路径，必须 fail closed 或 diagnose only。

### Repo Move/Rename Identity Cases

`repo move` 和 `repo rename` 在 repo root atomic rename commit uncertain 时，必须用 source/destination identity 做硬判定。`repo rename` 复用同一判定，但 human/JSON output 和 `recommended_next_command` 必须保留 `repo rename` command identity。

| Source repo root identity | Destination repo root identity | 判定 |
| --- | --- | --- |
| missing | matches expected repo identity | move committed；重跑原 `--run <plan-id>` resume forward，更新 main registry、external locators、verify、consume |
| matches expected repo identity | missing | move not committed；source 仍是 authoritative repo root，重跑原 `--run <plan-id>` 可在重新校验全部 preflight 后再执行 atomic rename |
| matches expected repo identity | matches expected repo identity | fail closed；可能 copy/duplicate，不能猜哪边是 canonical |
| missing | missing | fail closed；无法证明是否被外部删除、移动或权限隐藏 |
| different identity | any | fail closed；source 已被外部改写或被其他 live repo 占用 |
| any | different identity | fail closed；destination 已被外部改写或被其他 live repo 占用 |

只有 "source missing 且 destination matches" 能视为 move 已 commit 并 resume forward。只有 "source matches 且 destination missing" 能视为 move 未 commit。source 或 destination 任何一侧出现 `different identity` 永远 fail closed，包括 source different + destination expected；其他情况也不能继续写 registry 或 locator。

### Workspace Move Identity Cases

`workspace move` commit uncertain 时必须用 source/dest identity 做硬判定：

| Source identity | Destination identity | 判定 |
| --- | --- | --- |
| missing | matches expected workspace identity | move committed；resume forward，更新 registry/verify/consume |
| matches expected workspace identity | missing | move not committed；rollback/no-op journal，保留 source |
| matches expected workspace identity | matches expected workspace identity | fail closed；可能 copy/duplicate，不能猜 |
| missing | missing | fail closed；无法证明是否被外部删除或移动 |
| different identity | any | fail closed；目标或源已被外部改写 |
| any | different identity | fail closed；目标或源已被外部改写 |

只有 "source missing 且 destination matches" 能视为 commit 已完成并 resume forward。只有 "source matches 且 destination missing" 能视为 move 未 commit。source 或 destination 任何一侧出现 `different identity` 永远 fail closed，包括 source different + destination expected；其他情况也不能继续写 registry。

### Workspace Rename Recovery

`workspace rename` 是 direct metadata operation，但仍必须：

- dry run 只做检查，不写 journal。
- normal run 在普通 old-name selector 校验前先查 pending rename journal；若 old selector 已失效但 journal old/new identity 匹配，必须继续 recovery。
- normal run preflight external locator reachable、writable、metadata well-formed 且 freshness evidence matches。
- normal run 写 journal 后再更新 registry 或 locator。
- 如果 registry 已更新但 locator 未更新，status/doctor 报告 pending lifecycle op 和 `recommended_next_command`；用户重跑同一条 `jvs workspace rename <old-name> <new-name>` 后 resume locator update。
- 如果 locator 已更新但 registry 未更新，status/doctor 报告 pending lifecycle op 和 `recommended_next_command`；用户重跑同一条 rename 后 resume registry update 或在证据证明未 commit 时 no-op。
- 如果 old/new name 同时被不同 metadata 面承认且 journal 缺失，doctor 不能猜，必须 fail closed。

### Workspace Delete Recovery

`workspace delete` phases 应至少包括：

1. `prepared`
2. `workspace_folder_deleted`
3. `registry_entry_deleted`
4. `verified`
5. `consumed`

`prepared` 必须记录要删除的 exact folder identity 和 workspace entry identity。删除 folder commit uncertain 时，不能重试 destructive delete，除非 evidence 证明目标仍是同一个 expected workspace folder。若 folder 已消失但 registry entry 仍存在，resume forward 只允许删除 matching registry entry，不允许删除 save point storage。

### Repo Detach Recovery

`repo detach` phases 应至少包括：

1. `prepared`
2. `external_connections_marked_pending`
3. `archive_dir_created`
4. `detaching_marker_written`
5. `control_plane_archived`
6. `detached_metadata_written`
7. `external_connections_disabled`
8. `verified`
9. `consumed`

`detaching_marker_written` 是移动 active `.jvs` 前的硬门槛。`DETACHING` marker 必须 durable fsync，且必须包含 `operation_id`、repo_id、old repo root、expected active `.jvs` identity、archive path、registered workspace summary 和 `recommended_next_command`。没有 durable `DETACHING`，不得 archive active `.jvs`。

如果 active `.jvs` 已被归档但 `DETACHED` 未写入，从 repo root 运行 status/doctor 必须通过 repo-local bounded scan `.jvs-detached/*/{DETACHING,DETACHED}` 找到 pending detach，并推荐重跑 `jvs repo detach --run <repo-detach-plan-id>`。如果 active `.jvs` 已被归档但 external connections 未全部 disabled，也必须通过 `DETACHING`/`DETACHED` 或 reachable pending external workspace connection 报告 pending detach 和 `recommended_next_command`。若 archive path、repo_id、expected active `.jvs` identity 或 freshness evidence 不匹配，fail closed；不得创建新的 active `.jvs`。

## Doctor 分层

正常操作路径：

1. 用户运行 lifecycle command。
2. command 自己更新 registry、locator、main path、journal 和计划状态。
3. command 完成后 `status`、`workspace list`、`workspace path`、`save`、`history` 和 `doctor --strict` 应健康。

异常 drift 路径：

1. 用户绕开 JVS，用 `mv`、`rm`、`cp`、Finder、备份恢复或同步工具改变 repo/workspace。
2. doctor 发现 registry、locator、control plane 或真实 folder 的不一致。
3. doctor 提示正确 lifecycle command，或在证据完全充足时做 metadata-only 保守 repair。

`doctor --strict` 对 pending lifecycle operation 的合同固定为 diagnose-only：

- 只报告 pending operation、current phase、风险说明和 `recommended_next_command`。
- 不自动 resume lifecycle mutation，不移动、不删除、不 archive、不改 workspace/repo lifecycle metadata。
- 对 preview/run operation，recommended next command 是原 `--run <plan-id>`；对 direct `workspace rename`，recommended next command 是同一条 rename command。
- 既有 `doctor repair-runtime` 仍可用于已有 runtime-safe repairs，但本 handoff 的 lifecycle recovery 不通过 doctor 自动执行。

doctor 可以自动修复或由显式 repair flow 修复的范围必须很窄，且不包含 lifecycle mutation resume：

- stale external locator，且 repo_id、workspace_name、registry `RealPath`、canonical folder identity 都能完全证明。
- metadata-only mismatch，且不需要猜路径、不需要扫描、不需要改用户文件。
- runtime-safe residue，且属于已有 repair-runtime 范围，不会推进 move/rename/delete/detach phase。

doctor 不能做：

- 不能猜 repo path 或 workspace path。
- 不能扫描全盘找候选。
- 不能抢 live repo。
- 不能移动、删除或覆盖用户 working files。
- 不能删除 save point history。
- 不能重写 audit/provenance。
- 不能把 repo_id mismatch 当作可修复。
- 不能要求用户理解 `workspace reconnect`。
- 不能把 pending lifecycle journal 当作自动 resume 授权；它只能显示 `recommended_next_command`。

`workspace reconnect` 在新产品心智里不是主命令。它最多可以存在为内部 repair primitive 或 troubleshooting 下由明确错误提示引导的窄动作，不应作为 future handoff 的标题、日常命令入口或用户必须先理解的概念。

## Doctor Drift Decision Table

| Drift 场景 | 证据入口 | doctor 行为 | 自动 metadata repair | Fail-closed 条件 |
| --- | --- | --- | --- | --- |
| 用户手动移动 repo folder，`.jvs` 随 folder 移动 | 用户从新 repo root 运行 doctor/status；`.jvs` 中 repo_id 与 registry 匹配；old root 不再是 live repo | 诊断 repo root drift，提示未来应使用 `jvs repo move`；检查所有 registered external workspace connections | 仅当所有 registered external workspace connections reachable、writable、well-formed 且 freshness matches 时，才可以 metadata-only 更新 main `RealPath` 和 external connection repo path | old root 仍 live、任一 external connection 不可达/不可写/malformed/freshness mismatch、repo_id mismatch、无法证明 current cwd 就是 moved repo |
| 用户手动重命名 repo folder | 同手动移动 repo；new basename 仅是 path change | 同手动移动 repo；不把 basename 当 repo identity | 可以在同等证据下 metadata-only repair | 同手动移动 repo |
| 用户手动移动 external workspace folder，locator 随 folder 移动 | 用户从 moved workspace cwd 运行 doctor/status；locator repo_id/name 匹配 registry entry；repo registry 可打开 | 诊断 workspace path drift，提示未来应使用 `jvs workspace move` | 可以更新该 workspace registry `RealPath` | 只能从 repo root 看见 old path missing、没有 new cwd evidence、locator mismatch、目标被 live repo 承认 |
| 用户手动重命名 external workspace folder | 同手动移动 external workspace；folder basename 变化不等于 workspace rename | 诊断 path drift；明确 workspace name 未变 | 可以更新 registry `RealPath`；不改 workspace name | locator name mismatch、new folder identity 无法证明、old path 仍有不同 identity |
| Workspace folder basename 与 workspace name 不同 | registry `RealPath` 存在，locator repo_id/name 匹配 | 报告 healthy；不提示 repair | 不需要 repair | 无 |
| 用户手动删除 external workspace folder | registry entry 指向 missing path；没有 matching current cwd | 诊断 missing workspace folder；提示 reviewed cleanup 或重新创建/恢复路径 | 不自动删除 registry entry，不删除 save point storage | folder deletion commit uncertain、path 可能是临时不可达、权限不足 |
| 用户复制 repo folder | 两个 folders 有相同 repo_id，或 external connections 指向其中一个 | 诊断 duplicate repo identity/live repo conflict | 不自动修复；提示不要用 copy 当 clone，future 应用 `repo clone`/adopt flow | 两边都可打开、任一 side 有 active operation、无法确认哪一个是 canonical |
| Repo move/rename preflight 发现 registered external connection drift | external connection 不可达、不可写、malformed 或 freshness evidence mismatch | lifecycle preview/run fail closed before moving repo root；doctor 诊断 drift 或要求恢复 workspace connection | 不做 offline-pending；只有身份完全可证明的 metadata-only drift 才可由显式 repair flow 修复 | 任一 registered external connection 证据不完整、需要猜路径或需要扫描未知位置 |
| Locator repo_id 正确但 `workspace_name` 错 | external workspace locator 与 registry name 不一致 | doctor strict 失败；报告 workspace connection mismatch；若有 pending rename journal，显示 `recommended_next_command` | 不自动 resume；pending journal 只允许用户重跑同一条 `jvs workspace rename <old-name> <new-name>` | registry 有多个候选、old/new name 都存在、journal 缺失或 stale |
| External workspace after JVS repo move 未完全更新 | pending lifecycle marker 或 journal 指向 old/new repo root，repo_id/name 匹配 | 报告 pending lifecycle op，提示 `recommended_next_command`：重跑原 `jvs repo move --run <plan-id>` 或 `jvs repo rename --run <plan-id>` | 不通过 doctor 自动 repair；用户重跑 recommended command 后 resume forward 更新 connection metadata | 没有 pending marker 且 new repo root 不可证明、old repo root 被另一个 live repo 占用 |
| 用户手动移动 repo 后从 external workspace 进入 | external locator repo_root missing；workspace identity 可证明，但 new repo root 未给出 | 诊断 stale connection；要求用户从 current repo root 运行 doctor 或提供明确 cwd evidence | 不自动搜索，不自动 repair | 任何需要扫描或猜测 new repo root 的情况 |
| 用户手动 detach/delete `.jvs` | repo root 没有 active `.jvs`；可能有 `.jvs-detached` archive 或完全缺失；只允许 repo-local bounded scan `.jvs-detached/*/{DETACHING,DETACHED}` | 若 archive 有 `DETACHED` metadata，报告 detached；若只有 durable `DETACHING` 且 active `.jvs` 已移动，报告 pending detach 并推荐 `jvs repo detach --run <repo-detach-plan-id>`；否则诊断 not an active JVS project | 不重建 active repo；不自动删除 user files；不自动 resume detach | archive/marker malformed、repo_id mismatch、expected active `.jvs` identity mismatch、working files 与 archive evidence 不一致 |

表格验收线：每一行都必须有至少一个 unit/integration/conformance-style 场景，证明 doctor 的自动修复边界、diagnose-only 输出或 fail-closed 条件。

## 状态更新矩阵

| Operation | Registry `RealPath` | Locator `repo_root` / `workspace_name` | Main `RealPath` | Runtime journal/plans/locks | Save point history |
| --- | --- | --- | --- | --- | --- |
| `repo move` | main `RealPath` 更新到 new repo root；external workspace `RealPath` 不变 | 所有 registered external locators 的 `repo_root` 更新；`workspace_name` 不变；run 前全部 reachable/writable/well-formed/freshness matches | 更新到 new repo root | active locks/plans/recovery fail closed；run 写 journal 和 `recommended_next_command`；move plan run 后 consumed | 不变 |
| `repo rename` | 同 `repo move` | 同 `repo move` | 同 `repo move` | 同 `repo move`，command identity 为 rename | 不变 |
| `repo detach` | active registry 随 `.jvs` archive；ordinary discovery 不再读取 active registry | 所有 registered external locators 转 disabled/orphaned；不得指向虚假 active repo | active main `RealPath` 不再可被 discovery 当作 JVS repo | active locks/plans/recovery fail closed；detach journal archived/consumed；archive path 含 journal `operation_id`，`.jvs` archive 前 durable `DETACHING` | 不重写历史；历史随 archive 保留 |
| `workspace move` | 该 workspace `RealPath` 更新到 new folder；其他 entries 不变 | 该 external locator 的 `repo_root` 和 `workspace_name` 不变；移动后重新验证 | 不适用于 main；main 必须转 `repo move` | active locks/plans/recovery fail closed；run 写 journal；move plan run 后 consumed | 不变 |
| `workspace rename` | entry key/name 更新；`RealPath` 不变 | 该 external locator 的 `workspace_name` 更新；`repo_root` 不变 | main rename immutable；`jvs workspace rename main ...` fail closed，folder rename 用 `repo rename` | direct metadata op 仍写 journal 和 `recommended_next_command`；active operation/recovery fail closed；pending recovery 重跑同一条 rename | 不变 |
| `workspace delete` | 删除该 workspace entry；其他 entries 不变 | 删除 workspace folder 时 locator 随 folder 删除；不重写其他 locators | 不允许删除 main | active locks/plans/recovery fail closed；run 写 journal；delete plan run 后 consumed | save point storage/descriptors 不变，后续 cleanup 处理 |

矩阵验收线：每个 operation 都必须有单测或 conformance-style 验证，证明它只更新自己声明的状态面。

## Safety Boundaries

所有 lifecycle 和 doctor repair 都必须 fail closed：

- repo_id/name mismatch。
- malformed locator/config。
- target repo registry 不承认 workspace。
- locator `workspace_name` 与 registry name 不一致，除非 pending rename journal 明确要求 resume。
- `repo move`/`repo rename` 的任一 registered external workspace connection 不可达、不可写、malformed 或 freshness evidence mismatch。
- live repo conflict。
- workspace root、locator、repo root、repo `.jvs` 或目标 parent 的 symlink/no-follow 检查失败。
- source path 与 destination path overlap。
- 目标位于 `.jvs` 内。
- workspace path 与 repo root、repo `.jvs`、其他 workspace path 不安全重叠。
- dirty workspace，除非具体命令设计了 reviewed force，首版不需要。
- unsafe cwd：run 时当前 cwd 位于即将被 move/delete 的 workspace/repo subtree 内；`workspace rename` 和 `repo detach` 除外，future `repo delete` 必须要求 cwd outside target repo。
- 无法可靠判断当前 cwd 是否在 affected tree 内。
- active operation、active lock、recovery state 或未完成 plan。
- cross-device move。
- commit uncertain，无法证明 move/rename/delete/detach commit 结果。
- repo move/rename source/destination identity 不符合 source-missing + dest-expected committed 或 source-expected + dest-missing not-committed 判定；source different + dest expected 必须 fail closed。
- 权限失败、atomic rename 失败、atomic metadata write 失败。
- preview plan stale、plan id 不属于当前 repo、plan id 目标 identity 不匹配。
- repo detach archive path 已存在、不可写、无法 no-overwrite 创建，或 `.jvs` archive 前无法 durable 写入 `DETACHING` marker。

Live repo conflict 只承诺 bounded evidence：

- old locator 指向的 repo path 如果仍 live。
- workspace folder 物理祖先中可打开的 control repo。
- target repo registry 里记录的 workspace paths。
- 当前 repo 的 registry 和 locators。
- pending lifecycle markers 和 operation journal 中记录的 old/new paths。
- repo root 下 `.jvs-detached/*/{DETACHING,DETACHED}` 的 repo-local bounded scan；不得升级为全盘扫描。

bounded evidence 之外不能扫描未知位置，也不能因为没找到冲突就抢占身份不明的 folder。

## Metadata Rewrite Primitive

lifecycle 和 doctor 都需要安全 metadata rewrite，但不能裸用通用 writer 当作安全 primitive。

要求：

- rewrite API 必须显式传入 expected `repo_id`、expected old/new `workspace_name`、expected old/new `repo_root` 或 operation-specific invariant。
- 写入前 no-follow 校验 workspace root、locator path、repo root 和相关 parent。
- 写入前重新读取 locator/config，并确认它仍等于 preview/journal 时的 expected identity。
- 使用 atomic write + fsync 或项目既有 durable metadata 写入约定。
- 写入后重新读取并验证 registry/locator 一致。
- 写失败必须保留可诊断状态，不能部分覆盖成不可信 metadata。

当前实现已知冲突：

- `workspace rename` 若会移动 external folder，或不会更新 locator `workspace_name`，必须重构成 name-only。
- `workspace rename` 若在检查 pending rename journal 前就做普通 old-name selector 校验，必须重构；pending recovery 不能因 old selector 已失效而报 workspace not found。
- `workspace rename main ...` 若能成功，必须改为 fail closed，并输出 main workspace is the repo root; use `jvs repo rename` to rename the folder.
- doctor 的 `WorkspaceLocatorMatchesRepo` 若只比较 `repo_id` 不比较 `workspace_name`，必须强化。same repo_id but wrong workspace_name 不是健康状态。
- `WriteWorkspaceLocator` 不能裸用作安全 rewrite primitive；需要 operation-specific expected identity 和 no-follow/atomic 校验。
- repo move/rename 若允许 registered external workspace connection offline/malformed/mismatch 后先移动 repo root，必须改为移动前 fail closed。
- repo detach 不能删除 active `.jvs`；必须先写 durable `DETACHING` marker，再 archive active `.jvs`，并写 `DETACHED` metadata。

## Human Output 草案

Human output 面向普通用户，不能暴露 locator/control plane 等内部词。可以使用 "workspace connection"、"JVS project archive"、"project metadata" 等自然说法。

`workspace rename` 成功：

```text
Renamed workspace
Old name: experiment
New name: review
Folder path unchanged: /work/experiment
Workspace connection updated: yes
Doctor strict: passed
```

`workspace move` dry run：

```text
Workspace move preview
Workspace: experiment
Old folder: /work/experiment
New folder: /ssd/experiment
Workspace name: unchanged
Move method: atomic rename required
Plan ID: wm-...
No files were moved.
```

`workspace delete` preview：

```text
Workspace delete preview
Workspace: experiment
Folder to delete: /work/experiment
Save point storage: kept
Plan ID: wd-...
No files were changed.
```

`workspace delete` run while inside target workspace：

```text
Cannot delete workspace "experiment" while your shell is inside it.
Current folder: /work/experiment/src
Folder to delete: /work/experiment
No files were changed.

Run it from a safe folder:
cd /work/project
jvs workspace delete --run wd-...
```

`repo move` success:

```text
Moved JVS project
Old folder: /work/project
New folder: /ssd/project
Repo ID unchanged: repo-...
Main workspace path updated: yes
Workspace connections updated: 2
Doctor strict: passed
```

`repo move` run while inside source repo：

```text
Cannot move this JVS project while your shell is inside it.
Current folder: /work/project/src
Project folder to move: /work/project
No files were changed.

Run it from outside the project:
cd /work
jvs --repo /work/project repo move --run rm-...

After it succeeds:
cd /ssd/project/src
```

`repo detach` preview:

```text
JVS project detach preview
Project: /work/project
Repo ID: repo-...
Project files: kept in place
JVS project archive: /work/project/.jvs-detached/repo-...-<operation-id>-20260503T120000Z/
Workspace connections: will be marked detached
Plan ID: rd-...
No files were changed.
```

`repo detach` success:

```text
Detached JVS project
Project files kept in place: /work/project
JVS project archive: /work/project/.jvs-detached/repo-...-op-...-20260503T120000Z/
This folder is no longer an active JVS repo.
```

Cross-device move failure:

```text
Cannot move workspace: source and target are on different filesystems.
This version only supports same-filesystem atomic moves.
No files were changed.
```

Pending lifecycle recovery:

```text
JVS found an unfinished project move.
Operation ID: op-...
Current phase: external workspaces still need updating
Next step: jvs repo move --run rm-...
Doctor strict did not attempt lifecycle repair.
```

## JSON Output 草案

JSON 使用现有 envelope。成功 `workspace rename` 的 `data` 建议包含：

```json
{
  "schema_version": 1,
  "command": "workspace rename",
  "ok": true,
  "repo_root": "/work/project",
  "workspace": "review",
  "data": {
    "operation": "workspace_rename",
    "operation_id": "op-...",
    "old_workspace_name": "experiment",
    "new_workspace_name": "review",
    "folder_path": "/work/experiment",
    "folder_moved": false,
    "workspace_connection_updated": true,
    "recommended_next_command": null,
    "save_point_history_changed": false,
    "doctor_strict": "passed"
  },
  "error": null
}
```

`repo move` preview 的 `data` 建议包含：

```json
{
  "schema_version": 1,
  "command": "repo move",
  "ok": true,
  "repo_root": "/work/project",
  "workspace": "main",
  "data": {
    "operation": "repo_move_preview",
    "plan_id": "rm-...",
    "repo_id": "repo-...",
    "old_repo_root": "/work/project",
    "new_repo_root": "/ssd/project",
    "cwd_inside_affected_tree": true,
    "safe_next_command": "cd /work && jvs --repo /work/project repo move --run rm-...",
    "return_to_after_success": "/ssd/project/src",
    "move_method": "same_filesystem_atomic_rename",
    "main_realpath_will_update": true,
    "external_connections_preflight": "all_registered_reachable_writable_well_formed_freshness_matched",
    "external_connections_to_update": [
      {
        "workspace_name": "experiment",
        "workspace_path": "/work/experiment",
        "old_repo_root": "/work/project",
        "new_repo_root": "/ssd/project"
      }
    ],
    "save_point_history_changed": false
  },
  "error": null
}
```

所有 move/delete preview/run 的 JSON `data` 可以使用同一组轻量字段表达 cwd safety：`cwd_inside_affected_tree`、`safe_next_command` 和 `return_to_after_success`。`safe_next_command` 在无需换目录时为 `null`；repo move/rename 需要换目录时必须包含 `--repo <old-repo-root>`，避免用户离开 repo 后无法 run plan。

`repo detach` preview 的 `data` 建议包含：

```json
{
  "schema_version": 1,
  "command": "repo detach",
  "ok": true,
  "repo_root": "/work/project",
  "workspace": "main",
  "data": {
    "operation": "repo_detach_preview",
    "plan_id": "rd-...",
    "operation_id": "op-...",
    "repo_id": "repo-...",
    "repo_root": "/work/project",
    "archive_path": "/work/project/.jvs-detached/repo-...-op-...-20260503T120000Z/",
    "detaching_marker": "required_before_archiving_active_jvs",
    "working_files_kept": true,
    "external_connections_to_disable": [
      {
        "workspace_name": "experiment",
        "workspace_path": "/work/experiment",
        "reachable": true,
        "writable": true
      }
    ],
    "save_point_history_changed": false
  },
  "error": null
}
```

失败 envelope 示例：

```json
{
  "schema_version": 1,
  "command": "repo move",
  "ok": false,
  "repo_root": "/work/project",
  "workspace": "main",
  "data": null,
  "error": {
    "code": "E_LIFECYCLE_CROSS_DEVICE_MOVE",
    "message": "Cannot move repo: source and target are on different filesystems.",
    "hint": "This version only supports same-filesystem atomic moves. No files were changed."
  }
}
```

## Error Code 草案

| Code | 含义 |
| --- | --- |
| `E_LIFECYCLE_REPO_ID_MISMATCH` | repo_id 与 plan、locator 或 registry 不一致 |
| `E_LIFECYCLE_WORKSPACE_NAME_MISMATCH` | workspace name 与 locator/registry/plan 不一致 |
| `E_LIFECYCLE_MALFORMED_METADATA` | locator、config 或 registry malformed |
| `E_LIFECYCLE_LIVE_REPO_CONFLICT` | folder 仍被另一个 live repo 承认 |
| `E_LIFECYCLE_UNSAFE_PATH` | symlink、path overlap、`.jvs` 内目标或 no-follow 检查失败 |
| `E_LIFECYCLE_UNSAFE_CWD` | run 时当前 shell folder 位于即将被 move/delete 的 folder 内；命令必须先 fail closed 并给出 safe next command |
| `E_LIFECYCLE_ACTIVE_OPERATION` | 存在 active lock、operation、recovery 或未完成 plan |
| `E_LIFECYCLE_DIRTY_WORKSPACE` | workspace 有 unsaved changes，命令不允许继续 |
| `E_LIFECYCLE_TARGET_EXISTS` | move 目标已存在 |
| `E_LIFECYCLE_CROSS_DEVICE_MOVE` | 首版 atomic rename 不能跨 filesystem |
| `E_LIFECYCLE_COMMIT_UNCERTAIN` | 无法证明 move/rename/delete/detach commit 结果 |
| `E_LIFECYCLE_PERMISSION_DENIED` | 权限不足或 atomic metadata write 失败 |
| `E_LIFECYCLE_EXTERNAL_CONNECTION_UNAVAILABLE` | registered external workspace connection 不可达、不可写、malformed 或 freshness mismatch |
| `E_LIFECYCLE_PLAN_STALE` | preview plan 已过期或 freshness 证据不匹配 |
| `E_LIFECYCLE_ARCHIVE_EXISTS` | repo detach archive path 已存在或无法 no-overwrite 创建 |
| `E_LIFECYCLE_PENDING_OPERATION` | 发现未完成 lifecycle operation，需要重跑 `recommended_next_command` 或 fail closed |

## 阶段实施计划

### Phase 0: Contract And Test Scaffolding

- 固定命令 parse contract 和 preview/run plan id envelope：`workspace move`、`workspace delete`、`repo move`、`repo rename`、`repo detach`。
- 固定不发布 `workspace remove`/`repo remove` 的新口径；现有未发布 old behavior 按 clean contract 重构。
- 增加 fake filesystem identity、cross-device、symlink、overlap、active lock、commit uncertain、archive path conflict 的测试入口。
- 增加 fake current working directory / affected subtree 判断的测试入口，覆盖 preview allowed、run fail closed 和 copyable safe next command。
- 固定 state update matrix 的 contract tests。
- 固定 durable lifecycle operation journal schema、phase idempotence 和 recovery/fail-closed tests。
- 固定 doctor/lifecycle 分层：正常 move/rename/delete/detach 不依赖 doctor 修尾。
- 记录 future publish gate：发布前必须验证 release-facing docs 不出现 `workspace reconnect` 或 `remove` 旧心智；这不是当前 non-release-facing handoff 的当前状态测试。

### Phase 1: Main Workspace And Metadata-Only Rename

- 移除或迁移 legacy `repoRoot/main` fallback；clean contract 只支持 adopted main = repo root。
- 先写失败用例：main workspace `RealPath` 必须等于 repo root canonical path。
- 先写失败用例：`jvs workspace rename main <new-name>` fail closed，并提示 main workspace is the repo root; use `jvs repo rename` to rename the folder.
- 先写失败用例：external workspace rename 后 folder path 不变，locator `workspace_name` 必须更新。
- 重构 `workspace rename` 为 name-only，并写 journal；pending recovery 必须先查 journal 再做普通 old-name selector 校验。
- 强化 `WorkspaceLocatorMatchesRepo` 或等价 doctor 检查：必须同时比较 `repo_id` 和 `workspace_name`。
- 引入 operation-specific safe locator rewrite primitive，替代裸 `WriteWorkspaceLocator` 作为 repair/run 写入口。
- 验收 `status`、`workspace list`、`workspace path`、`save`、`history` 和 `doctor --strict`。

### Phase 2: Workspace Move And Delete

- 交付 `workspace move` preview/run，首版 same-filesystem atomic rename。
- 交付 `workspace delete` preview/run，删除 workspace folder 和 registry entry，save point storage 留给 cleanup。
- 明确拒绝 main workspace delete。
- 覆盖 workspace move/delete 在 source/target workspace 内 run 时 `E_LIFECYCLE_UNSAFE_CWD` fail closed；preview 仍可在 workspace 内生成 plan 和 safe run command。
- 覆盖 dirty workspace、target exists、symlink、overlap、active operation、cross-device、source/dest identity cases 和 commit uncertain。
- 验收 move/delete 后 lifecycle 状态矩阵只更新声明状态面。

### Phase 3: Repo Move/Rename And Discovery

- 交付 `repo move` preview/run，保留 repo_id/history。
- 交付 `repo rename` basename-only sugar。
- 更新 main `RealPath` 和所有 registered external locators 的 `repo_root`。
- 首版要求所有 registered external workspace connections reachable、writable、metadata well-formed、freshness evidence matches；任一失败在移动 repo root 前 fail closed，不设计 offline-pending。
- 支持 unsafe-cwd retry 所需的 `jvs --repo <old-repo-root> repo move --run <plan-id>` 和 `jvs --repo <old-repo-root> repo rename --run <plan-id>`；覆盖 cwd 在 old repo root subtree 内时 run mutation 前 fail closed。
- 覆盖 stale/malformed locator fail closed、live repo conflict、external workspace offline/unwritable/freshness mismatch、plan stale、source/dest identity crash table、atomic publish failure。
- 覆盖 repo root moved but external locator update incomplete 时 status/doctor 报 `recommended_next_command`，并由重跑原 `--run` 继续。
- 验收从 main 和 external workspace discovery 都健康。

### Phase 4: Repo Detach Archive

- 交付 `repo detach` preview/run，不使用 `repo remove`。
- 固定 archive control plane 策略：active `.jvs` no-overwrite archive 到 `.jvs-detached/<repo-id>-<operation-id>-<timestamp>/`；archive 前 durable 写 `DETACHING` marker，archive 后写 `DETACHED` metadata。
- 保留 working files；不设计项目文件删除。
- registered external workspace connections 转 detached/orphaned；不可达、不可写、malformed 或 freshness mismatch fail closed。
- 覆盖从 repo 内执行 detach 成功后 human output 明确说明当前 folder 不再是 active JVS repo。
- 覆盖 active `.jvs` 已移动但 `DETACHED` 未写入时，从 repo root bounded-scan `.jvs-detached/*/{DETACHING,DETACHED}` 发现 pending detach 并推荐重跑原 `repo detach --run`。
- 验收 run 后 ordinary discovery 不再把 main folder 当 active repo，external workspace 显示 detached/orphaned 状态。

### Phase 5: Doctor Drift Layer

- doctor 增加 manual bypass drift 诊断和 metadata-only 保守 repair。
- `doctor --strict` 对 pending lifecycle operation 只报告 pending op 和 `recommended_next_command`，不自动 resume lifecycle mutation；已有 `repair-runtime` 仅限 runtime-safe repairs。
- 覆盖 doctor drift decision table 的每一行。
- 把旧 reconnect 场景吸收到 doctor primitive/troubleshooting，不作为主命令心智。
- 补齐 release evidence 前的 docs-contract/conformance 矩阵。

## QA 验收矩阵

| 场景 | 命令/条件 | 期望 |
| --- | --- | --- |
| Workspace rename name-only | `jvs workspace rename experiment review` | workspace name 改为 `review`；folder path 不变；external locator `workspace_name` 更新；doctor strict 通过 |
| Workspace rename old selector | rename 后运行旧 name 命令 | 旧 name 失败；提示 workspace 不存在；不移动 folder |
| Workspace rename pending old selector invalid | fault injection：registry 已更新为 new name，locator 未更新；用户重跑 `jvs workspace rename experiment review` | command 先检查 pending rename journal，再根据 journal old/new identity resume；不因 old selector 已失效而报 workspace not found |
| Workspace rename rejects main | `jvs workspace rename main trunk` | fail closed；提示 main workspace is the repo root; use `jvs repo rename` to rename the folder |
| Workspace rename preflight locator writable | external workspace connection 不可写 | rename fail closed；registry/locator/folder 不变 |
| Workspace rename journal recovery | registry 更新后 locator 更新失败 | status/doctor 报 pending lifecycle op 和 `recommended_next_command`；重跑同一条 rename 后健康；doctor strict 不自动 mutate；半更新不算 healthy |
| Workspace rename conflict | new name 已存在 | 失败；registry/locator/folder 不变 |
| Workspace basename differs from name | workspace name `experiment`，folder `/work/review-folder` | health checks 通过；不要求 basename 等于 name |
| Workspace move path-only | `jvs workspace move experiment /ssd/experiment` then run plan | folder 移动；workspace name 不变；registry `RealPath` 更新；save point history 不变 |
| Workspace move preview inside source | cwd 为 `/work/experiment/src`，运行 `jvs workspace move experiment /ssd/experiment` | preview 成功生成 plan；不要求先 cd；输出 safe run command |
| Workspace move unsafe cwd run | cwd 为 `/work/experiment/src`，运行 `jvs workspace move --run <plan-id>` | `E_LIFECYCLE_UNSAFE_CWD`；mutation 前 fail closed；输出 `cd <safe-parent-or-repo-root>` 后重跑 run command |
| Workspace move source/dest identity | fault injection 覆盖 source-only、source-missing + dest-expected、source-different + dest-expected、both、none、mismatch | 仅 source-missing + dest-expected resume forward；source-only no-op/rollback journal；source-different + dest-expected 和其他 mismatch fail closed |
| Workspace move cross-device | source/target 不同 filesystem | 失败；说明首版只支持 atomic rename；不 fallback copy+delete |
| Workspace move target exists | target folder 已存在 | 失败；不 overwrite、不 merge |
| Workspace delete preview/run | `jvs workspace delete experiment` then run plan | preview 显示删除范围；run 删除 folder 和 registry entry；save point storage 留给 cleanup |
| Workspace delete unsafe cwd run | cwd 为 target workspace subtree，运行 `jvs workspace delete --run <plan-id>` | `E_LIFECYCLE_UNSAFE_CWD`；不删除 folder；输出 copyable `cd <safe-parent-or-repo-root>` 和同一 plan id |
| Workspace delete rejects main | `jvs workspace delete main` | fail closed；提示 main workspace 是 repo root，应使用 repo-level flow |
| Workspace delete dirty | workspace 有 unsaved changes | 默认失败；提示先 save；不删除 folder |
| Workspace detach non-goal | 用户要求保留 external workspace folder 但停止管理 | 首版提示非目标/future；不挤进 delete |
| Repo move updates all external locators | repo 有 external workspaces | main `RealPath` 更新；所有 registered external locator `repo_root` 更新；external `RealPath` 不变；移动前全部 connection 通过 reachable/writable/well-formed/freshness preflight |
| Repo move discovery from new root | repo root moved before locator update fault | 从新 repo root status/doctor 报 pending op 和 `recommended_next_command`；重跑原 `--run` 后健康 |
| Repo move discovery from external | external workspace 有 pending marker | status/doctor 报 pending lifecycle op 和 `recommended_next_command`，不 silent stale |
| Repo rename basename-only | `jvs repo rename better-name` | 目标为同父目录 basename；路径参数被拒绝；行为同 repo move |
| Repo move repo_id stable | move/rename 后检查 repo_id/history | repo_id 和 save point history 不变 |
| Repo move registered external unavailable | 任一 registered external workspace connection 不可达、不可写、malformed 或 freshness mismatch | preview/run 在移动 repo root 前 fail closed；提示先用 doctor diagnose drift 或恢复 workspace；不设计 offline-pending |
| Repo move/rename source/dest identity | fault injection 覆盖 source-only、source-missing + dest-expected、source-different + dest-expected、both、none、mismatch | 仅 source-missing + dest-expected resume forward；source-only 判定 move not committed；source-different + dest-expected 和其他 mismatch fail closed |
| Repo move/rename unsafe cwd run | cwd 在 old repo root subtree，运行 repo move/rename `--run` | `E_LIFECYCLE_UNSAFE_CWD`；移动 repo root 前 fail closed；输出 `cd <safe-parent>` 和 `jvs --repo <old-repo-root> repo move|rename --run <plan-id>`；成功后建议进入 new repo path 或 same relative subfolder |
| Repo detach preview/run | `jvs repo detach` then run plan | working files 保留；archive path 来自 journal `operation_id`；`.jvs` archive 前写并 fsync `DETACHING`；`.jvs` archived；`DETACHED` metadata 写入；active discovery 不再把 folder 当 repo |
| Repo detach from inside repo | cwd 在 repo subtree，运行 detach run | 不需要 cwd guard；成功后 output 明确当前 folder 不再是 active JVS repo |
| Repo detach crash before DETACHED | `DETACHING` durable，active `.jvs` 已移动，`DETACHED` 未写入 | 从 repo root bounded-scan `.jvs-detached/*/{DETACHING,DETACHED}` 找到 pending detach；status/doctor 推荐 `jvs repo detach --run <repo-detach-plan-id>`；doctor strict 不自动 resume |
| Repo detach external unavailable | registered external workspace locator 不可达、不可写、malformed 或 freshness mismatch | fail closed；要求先 doctor diagnose drift 或恢复 workspace；不 archive `.jvs` |
| Repo detach not delete files | repo detach 成功后 | project working files 仍在；external workspace working files 仍在；没有 future `repo delete` 行为 |
| Repo delete not first release | `jvs repo delete` | 首版不支持；未来必须单独高风险设计，且 require cwd outside target repo |
| Manual bypass doctor table | 用户用 `mv`/Finder/`rm`/`cp` 改 repo 或 workspace | doctor drift decision table 每行有 diagnose/repair/fail-closed 验收 |
| Manual repo move requires cwd evidence | 只知道 old repo root missing | doctor 不扫描全盘；要求从 new repo root 运行或提供明确 cwd evidence |
| Stale reconnect absorbed | old external locator stale 但 identity 可证明 | doctor/lifecycle primitive 可 metadata-only 修复；不要求用户理解 daily `workspace reconnect` |
| Locator name mismatch | locator repo_id 正确但 `workspace_name` 错 | doctor strict 失败；无 pending rename journal 时不能视为 healthy |
| Live repo conflict | old/live repo 仍承认 workspace | fail closed；不抢占 |
| Unsafe paths | symlink、overlap、目标在 `.jvs` 内 | fail closed；不写 metadata |
| Active operation/recovery | locks、plans、recovery 存在 | lifecycle command fail closed；先完成或恢复 |
| Journal fault injection | 每个 phase 后 crash/retry | phases idempotent；重跑原 command resume forward 优先；status/doctor 只显示 `recommended_next_command`；commit uncertain fail closed |
| Commit uncertain | move/rename/delete/detach 结果无法确认 | fail closed with recovery instructions；不继续第二次 destructive action |
| Future publish gate: release-facing docs clean | 发布前搜索 CLI spec/user docs | 不出现 `workspace reconnect`、`workspace remove`、`repo remove` 作为日常命令或公共合同；不作为当前 non-release-facing handoff 当前状态测试 |

## Release Readiness Checklist

- [ ] 文档和 tests 使用 `25_REPO_WORKSPACE_LIFECYCLE_PRODUCT_PLAN.md`，旧 reconnect 计划不再作为 active handoff。
- [ ] release-facing CLI spec/user docs 不更新本 future handoff；release-facing docs clean 是 future publish gate，不作为当前 handoff 当前状态测试。
- [ ] 不发布新的 `workspace remove` 或 `repo remove` 口径；未发布旧行为按 clean contract 重构。
- [ ] 不发布新的 public `lifecycle resume` 命令；pending recovery 首版通过重跑原 command 完成。
- [ ] `workspace rename` 是 name-only，folder path 不变。
- [ ] `workspace rename main ...` fail closed，并提示 main workspace is the repo root; use `jvs repo rename` to rename the folder.
- [ ] `workspace rename` preflight external locator reachable、writable、metadata well-formed、freshness evidence matches，并更新 external locator `workspace_name`。
- [ ] `workspace rename` pending recovery 在普通 old-name selector 校验前先查 pending rename journal；old selector 已失效时仍能按 journal old/new identity resume。
- [ ] `workspace rename` 写 durable lifecycle operation journal；journal 包含 `recommended_next_command`；半更新状态由 status/doctor 报 pending op 和推荐重跑同一条 rename。
- [ ] `workspace move` 是 path-only，workspace name 不变。
- [ ] `workspace delete` 使用 preview/run，删除 workspace folder 和 registry entry。
- [ ] `workspace move`/`workspace delete` preview 可从 affected workspace 内运行；run 若 cwd 在 affected workspace subtree 内，必须 mutation 前 fail closed 并给出 copyable safe next command。
- [ ] `workspace delete` 不能删除 main workspace，不能删除 save point storage。
- [ ] `workspace detach` 明确为 future 非首版或非目标，避免范围蔓延。
- [ ] main workspace clean contract 只支持 adopted main = repo root；legacy `repoRoot/main` fallback 已移除或迁移。
- [ ] `repo move` 和 `repo rename` 保留 repo_id/history。
- [ ] `repo move` 和 `repo rename` 要求所有 registered external workspace connections reachable、writable、metadata well-formed、freshness evidence matches；任一失败在移动 repo root 前 fail closed，不设计 offline-pending。
- [ ] `repo move` 和 `repo rename` 更新 main `RealPath` 和所有 registered external locator `repo_root`。
- [ ] `repo move` 和 `repo rename` 支持 unsafe-cwd retry 的显式 `--repo <old-repo-root>` run form；cwd 在 old repo root subtree 内时 run 必须 mutation 前 fail closed，并输出 safe next command 和 success return path。
- [ ] `repo move`/`repo rename` source/dest identity crash table 覆盖 source-missing + dest-expected、source-different + dest-expected、source-only、both、none、mismatch；只有 source-missing + dest-expected resume forward，source-different + dest-expected 必须 fail closed。
- [ ] `repo move` discovery 覆盖从 new repo root 和 pending external workspace 报 `recommended_next_command`，并通过重跑原 `--run` recovery。
- [ ] `repo detach` 使用 preview/run，保留 working files，不叫 `repo remove`。
- [ ] `repo detach` archive path 固定为 `.jvs-detached/<repo-id>-<operation-id>-<utc-timestamp>/`，其中 `<operation-id>` 来自 journal `operation_id`，不得从 `plan_id` 派生。
- [ ] `repo detach` 在 archive active `.jvs` 前创建 archive dir，写并 fsync `DETACHING` marker；marker 包含 `operation_id`、repo_id、old repo root、archive path、expected active `.jvs` identity、registered workspace summary 和 `recommended_next_command`。
- [ ] `repo detach` archive 完成后写/发布 `DETACHED` metadata；status/doctor 能从 repo root bounded-scan `.jvs-detached/*/{DETACHING,DETACHED}` 发现 pending detach。
- [ ] `repo detach` 将所有 registered external workspace connections 标为 detached/orphaned；任一不可达、不可写、malformed 或 freshness mismatch fail closed。
- [ ] `repo detach` success output 说明当前 folder 不再是 active JVS repo。
- [ ] destructive `repo delete` 不进首版；未来必须独立高风险命令，并 require cwd outside target repo。
- [ ] move 首版只支持 same-filesystem no-overwrite atomic rename。
- [ ] durable lifecycle operation journal 覆盖 workspace move/delete、workspace rename、repo move/rename/detach，且每条 journal 都包含 `recommended_next_command`。
- [ ] journal phases idempotent；resume forward 优先；commit uncertain fail closed。
- [ ] `doctor --strict` 发现 pending lifecycle operation 时只报告 pending op 和 recommended next command，不自动 resume lifecycle mutation；`doctor repair-runtime` 只做已有 runtime-safe repairs。
- [ ] workspace move source/dest identity cases 有 fault injection 覆盖。
- [ ] workspace move source/dest identity crash table 覆盖 source-missing + dest-expected、source-different + dest-expected、source-only、both、none、mismatch；只有 source-missing + dest-expected resume forward，source-different + dest-expected 必须 fail closed。
- [ ] doctor drift decision table 每行有测试或 conformance-style evidence。
- [ ] doctor 只修 metadata-only 且身份完全可证明的 drift。
- [ ] `WorkspaceLocatorMatchesRepo` 或等价检查同时比较 repo_id 和 workspace_name。
- [ ] locator rewrite 使用 operation-specific expected identity，不能裸用通用 writer。
- [ ] human output 不向普通用户暴露 locator/control plane；使用 workspace connection、JVS project archive 等自然话术。
- [ ] safety matrix 覆盖 repo_id/name mismatch、malformed metadata、live repo conflict、symlink、overlap、`.jvs` 内目标、unsafe cwd、active operation、cross-device、commit uncertain、archive conflict 和权限失败。
