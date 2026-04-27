# Save Point / Workspace Semantics

## Status / Scope / Supersedes

**状态**: 本文是 JVS clean product redesign / 干净重构后的产品语义总纲。

**范围**: 本文约束公开 UX、CLI 命名、帮助文本、文档、验收用例、行为设计和后续实现方向。

**覆盖关系**: 当前产品尚未发布，本文 supersedes/replaces old draft semantics。凡旧草案、历史实现、旧文档中的公开心智、命令含义或用户文案与本文冲突，均以本文为准。实现应朝本文的新语义收敛，而不是把新旧两套心智并存。

**Breaking change policy**: 命令/API 设计可以 breaking change。未发布前优先选择低心智、干净、一致的产品模型；不为兼容旧设计增加用户心智负担。

JVS 的核心产品定义是：

```text
JVS saves real folders as save points.
```

用户应把 JVS 理解为“真实文件夹的保存点系统”，而不是另一套源码管理心智。用户从一个真实文件夹开始工作，保存它，查看历史，只读查看旧保存点，并在需要时把某个保存点的内容复制回真实文件夹。

用户主路径固定为：

```text
init -> save -> history -> view -> restore
```

其中 `init` 表示把当前普通文件夹接入 JVS。接入后，低心智输出必须优先显示真实文件夹：

```text
Folder: /real/path
Workspace: main
```

`workspace/main` 是次要技术标识，不是用户理解产品的入口。

## Product Principles

1. JVS 是真实文件夹的 `save point` 系统。
2. `workspace` 是 JVS 管理的真实文件夹，不是虚拟概念。
3. 用户优先看到 `Folder: /real/path`；workspace 名称是次要技术标识。
4. `main` 是默认 workspace；它指向一个真实文件夹。
5. `repo root` / project container 是控制平面，不是默认被保存的用户文件夹。
6. 公开主词统一为 `save point` / 保存点。
7. 保存点内容和创建事实不可改写。
8. “不可改写”不等于“永久保存”；`cleanup run` 可以删除不受保护的保存点。
9. 默认保存点捕获 workspace 中所有 managed files。
10. JVS 控制数据永不进入保存点。
11. 显式 ignored/unmanaged files 默认不进入保存点，也不应被 restore 删除。
12. `view` 是只读临时 view session/path，不注册 workspace，不影响 history。
13. `restore` 只把保存点内容复制到真实 workspace，不回退、不重排、不删除历史。
14. `restore <save> --path <path>` 是误删单个文件/目录时的低风险主路径。
15. `workspace new <name> --from <save>` 只创建一个真实文件夹，文件内容从保存点准备出来；它不继承外部保存点作为自己的 history。
16. `restore <save> --to <workspace> --save` 是“复制到目标 workspace 并保存为目标的新保存点”，不是合并、替换历史或跨 workspace 继承历史。
17. 会覆盖或删除真实文件的操作必须在操作前重新扫描真实文件系统，不能只信 watcher 或 metadata。
18. destructive operations 默认拒绝未保存改动；无法确认安全时按 unsaved changes 处理。
19. `label` 只用于分类、过滤和展示，永远不是 `<save>` 解析目标。
20. `keep` 才表示防止 cleanup 删除；它和 `label` 完全分离。
21. 默认文案必须使用用户语言，强调真实路径、只读、安全、history 未改变、`label` 不保护、`keep` 保护。
22. 用户不需要先知道 save ID；可以按 path、时间、message、label、metadata 找到候选保存点，再选择具体 ID。
23. whole-workspace replacement 默认必须 preview 或高摩擦确认，不能静默覆盖/删除 managed files。
24. 所有会物化保存点内容或写入临时/真实文件的操作，都必须在写入前通过 capacity/safety margin gate。
25. replacement/destructive run 必须绑定 preview plan 或 expected target state，防止多个 agent 同时覆盖彼此的新保存点。
26. 所有读取/物化 source save point 的操作，在 resolve source 到完成或 recovery 接管期间，都必须注册 active source pin。
27. workspace real path 必须 canonicalized；V1 禁止 nested/overlapping workspace。
28. V1 默认不清 active workspace history；只有用户显式选择高摩擦 “clean old save points in this folder” 模式时，才可 prune 符合条件的旧保存点。

## Vocabulary

### Primary Public Terms

| 术语 | 语义 |
|------|------|
| `folder` / 文件夹 | 用户真实工作的文件夹。默认输出必须优先展示。 |
| `workspace` | JVS 管理的真实文件夹；`main` 是默认 workspace 名称。 |
| `JVS project` | 保存点、metadata、workspace registry 和控制数据所属的项目。 |
| `control data` | `.jvs` 或等价控制平面数据；永不进入保存点。 |
| `managed file` | 默认会被 save point 捕获、会被 restore 管理的 workspace 文件。 |
| `ignored/unmanaged file` | 显式排除或 JVS 控制数据；默认不保存、不恢复、不删除。 |
| `save point` / 保存点 | 一个 workspace managed files 的已保存副本，加上不可改写的创建事实。 |
| `save` | 从当前 workspace 的真实 managed files 创建一个新保存点。 |
| `history` | 查看保存点历史；默认当前 workspace，`history --all` 查看全局保存点列表。 |
| `view` | 只读查看某个保存点或保存点内某个路径。 |
| `restore` | 把某个保存点或保存点内某个路径复制到真实 workspace。 |
| `label` | 用户维护的分类、过滤、展示 metadata。 |
| `keep` | 用户显式声明的 cleanup 删除保护。 |
| `cleanup` | 清理不受保护的保存点；默认先 preview，run 才会删除。 |
| `recovery plan` | restore/remove 等操作失败或无法事务化时的闭环恢复计划。 |
| `operation plan` | preview 生成的可执行计划；run 必须绑定该计划并重校验 expected target state。 |
| `tombstone` | 已删除保存点的最小审计记录；用于显示 ID、删除原因和 `deleted-save` 错误。 |

### Public Status Words

默认人类输出可以展示 workspace 状态，但必须使用用户语言，而不是内部字段名：

| 默认文案 | 含义 |
|----------|------|
| `Folder` | 真实工作文件夹路径；默认先于 workspace 名称展示。 |
| `Workspace` | workspace 名称，例如 `main`、`exp`。 |
| `Newest save point` / 最新保存点 | 这个 workspace 最近保存出的保存点；没有保存过则显示 `none`。 |
| `Files match save point` | 当前 managed files 与某个保存点一致。 |
| `Files changed since save point` | 当前 managed files 从某个保存点继续编辑而来，已有未保存改动。 |
| `Files were last restored from` | 当前 managed files 曾从某个保存点恢复过，且之后可能已经编辑。 |
| `Started from` / `Based on` | workspace 初始内容来自某个保存点，但该 workspace 尚未把它作为自己的 history。 |
| `Restored paths` | 当前 workspace 中有指定路径从某个保存点恢复过。 |
| `Unsaved changes` | 当前真实文件相对 JVS 已知来源是否有尚未保存的改动。 |

避免在文件已经编辑后继续说 `Files copied from`，因为它会暗示当前文件仍与来源完全一致。干净状态可以说 `Files match save point A`；普通 `save B -> edit` 后应说 `Files changed since save point B`；只有确实先从另一个保存点恢复后再编辑，才说 `Files were last restored from A` 或 `Started from A`，并同时展示 `Unsaved changes: yes`。

### Advanced / Internal State Fields

以下字段只用于实现、JSON、高级 map、测试断言和本文的精确语义段落。它们不应作为 Primary Public Terms，也不应作为默认人类输出主词。

| 字段 | 精确定义 | 默认公开文案 |
|------|----------|--------------|
| `history_head` | 某个 workspace 最近一次追加的保存点；没有保存过则为 `null`。 | `Newest save point` |
| `content_source` | 当前 workspace 最近一次 whole-workspace materialized/copied from 的保存点；可能为空。 | `Files match save point` / `Files were last restored from` |
| `started_from` | workspace 初始内容来自哪个外部保存点；用于 `workspace new --from`，不是 history edge。 | `Started from` / `Based on` |
| `path_sources` | 当前 workspace 中指定路径最近从哪个保存点恢复过。 | `Restored paths` |
| `has_unsaved_changes` | workspace managed files 相对 JVS 已知来源是否发生未保存改动。 | `Unsaved changes` |
| `parent` | 新保存点指向保存前 workspace `history_head` 的线性历史边。 | 默认不展示；可在高级/JSON/map 中展示。 |
| `restored_from` | 保存点的 whole-workspace provenance，表示保存前内容来自某个被恢复的保存点。 | 默认输出说“created from restored save point A”。 |
| `restored_paths` | 保存点的 path-level provenance，表示保存前哪些路径来自哪些保存点。 | 默认输出说“includes restored path p from A”。 |
| `operation_lock` | workspace 操作锁，用于阻止并发 save/restore/remove 互相踩踏。 | 默认只在等待/失败时说明。 |
| `active_source_pin` | 正在读取/物化某个 source 保存点的操作保护；cleanup 不能删除该 source。 | 默认只在 cleanup/recovery 输出中说明。 |
| `expected_target_state` | destructive/replacement run 绑定的目标状态证据，例如 expected newest save point 和扫描证据。 | 默认说“folder changed since preview”。 |

### Old Draft / Historical Implementation Names

`checkpoint` 是旧草案/历史实现名，需要被新产品语义中的 `save point` 替换。它不是用户兼容层，也不是新公开词。它只能出现在旧草案说明、历史实现识别、代码重构 TODO、或本文的 clean redesign notes 中；新的公开产品文案必须使用 `save point` / 保存点。

### Forbidden Public Mental Models

公开用户心智不得使用 `branch`、`promote`、`checkout`、`commit`、`fork` 来解释 JVS。这些词只能出现在本段这种“禁止心智”说明，或出现在本文末尾的 clean redesign notes 中。

具体禁止项：

- `main` 不是 `branch`，也不是 `link`；它是默认真实 `workspace`。
- `workspace new <name> --from <save>` 不是 `fork`；它是从保存点创建另一个真实文件夹。
- `restore <save> --to <workspace> --save` 不是 `promote`；它是在目标 workspace 复制并保存。
- `view <save>` 不是 `checkout`；它是只读查看，不能移动 workspace 状态。
- `save` 不是 `commit`；它是把真实文件夹保存为一个保存点。
- Save Point Map / Workspace Map 不是 Git `branch` graph。
- `parent`、`restored_from`、`started_from`、`restored_paths` 是不同 provenance/历史事实，不得混成 Git-like graph 心智。

## First Use / Adopt Existing Folder

### Product Goal

第一次使用必须支持“从这个真实文件夹开始”。用户不需要先理解 repo root、project container、workspace container 或 main workspace 的内部布局。

主路径：

```text
cd /real/path
jvs init
jvs save -m "baseline"
```

等价显式形式：

```text
jvs init /real/path
```

本文选择 `jvs init [folder]` 作为 V1 public command。产品文案可以把这个动作称为“adopt this folder”，但不额外引入第二个命令名作为用户必须理解的概念。

### `jvs init [folder]`

`jvs init [folder]` 把一个已有普通文件夹接入 JVS，并把它注册为默认 `main` workspace。

必须语义：

1. 如果未传 `folder`，使用当前工作目录。
2. 解析 canonical real path。
3. 不移动用户文件。
4. 不复制用户文件到另一个工作目录。
5. 用户下一步仍在同一个真实文件夹工作。
6. 可以创建 `.jvs` 或等价控制数据，但 JVS 控制数据永不进入保存点。
7. 如果实现需要 project container 或 workspace registry，必须把它作为控制平面隐藏在低心智输出之后。
8. 默认输出先展示 `Folder: /real/path`，再展示 `Workspace: main`。
9. 初始状态没有保存点：`Newest save point: none`。
10. 现有文件尚未保存：`Unsaved changes: yes` 或 `Not saved yet`。
11. 第一次 `save A` 创建该 workspace 的第一个保存点，`A.parent=null`。

成功文案示例：

```text
Folder: /Users/alice/project
Workspace: main
JVS is ready for this folder.
Files were not moved or copied.
Newest save point: none
Not saved yet.
Unsaved changes: yes
Next: jvs save -m "baseline"
```

如果 `.jvs` 位于被接入文件夹内，必须满足：

- `.jvs` 是 JVS control data。
- `.jvs` 默认 ignored/unmanaged。
- `.jvs` 不会被 save point 捕获。
- restore 不会用保存点内容覆盖 `.jvs`。
- cleanup 不会把 `.jvs` 当作 workspace payload。

如果实现把 control data 放在外部 project container，默认输出仍必须以真实工作文件夹为主：

```text
Folder: /Users/alice/project
Workspace: main
Project data: /Users/alice/.jvs/projects/...
```

`Project data` 是辅助信息；不得让用户以为自己需要改到另一个 `main` 文件夹中继续工作。

## Core Model

### JVS Project And Workspace

JVS project 包含控制平面 metadata、保存点存储、workspace registry、cleanup plan、recovery plan 和一个或多个真实 `workspace` 文件夹。

`repo root` 或 project container 的职责是承载控制平面。它本身不是用户正在保存的 workspace，除非用户明确把该真实路径作为 workspace 接入。

每个 `workspace` 至少有：

- workspace name
- canonical real folder path
- 最新保存点，对应高级字段 `history_head`
- whole-workspace 内容来源，对应高级字段 `content_source`
- 初始来源，对应高级字段 `started_from`
- path-level 恢复来源，对应高级字段 `path_sources`
- 是否有未保存更改，对应高级字段 `has_unsaved_changes`

`main` 是默认 workspace 名称。产品文案不要只说“在 main 中”，还应在关键输出里先显示真实路径。

### Workspace Path Topology

V1 选择低风险规则：禁止 nested/overlapping workspace。

硬不变量：

1. 注册 workspace 前必须把用户输入 path 解析为 canonical real path，解析 symlink、`..`、大小写折叠文件系统 alias 和等价路径表示。
2. workspace registry 中保存 canonical real folder path；所有 containment 检查使用 canonical path。
3. 新 workspace 的 canonical path 不得等于、位于、包含或通过 symlink/alias 覆盖任何已注册 workspace path。
4. `jvs init [folder]`、`workspace new <name>`、workspace path import 或未来 rename/move，如果发现 nested/overlap，必须失败且不移动/复制/删除文件。
5. 保存、恢复、查看、删除 workspace 时，扫描边界必须以注册的 canonical workspace root 为准；symlink 不能绕过 containment 检查。
6. 如果 workspace 内出现指向另一个 workspace 的 symlink，默认不得把目标 workspace 的内容当作当前 workspace managed files；必须按 ignored/unmanaged risk 拒绝或要求用户显式 ignore/unmanage。
7. `workspace remove` 只能删除目标 workspace canonical root 内的文件；如果解析结果与注册 root 不一致，必须拒绝并进入人工处理/恢复路径。

这条规则的目的不是向用户解释复杂拓扑，而是保证 `save`、`restore`、`restore --path`、`workspace remove` 不会误伤另一个真实 workspace。

### Managed / Ignored / Unmanaged Files

默认规则：

1. workspace folder 下所有非控制数据文件都是 managed files。
2. JVS control data 永远不是 managed files。
3. 用户显式 ignore/unmanage 的文件不是 managed files。
4. `save` 默认捕获所有 managed files。
5. whole-workspace `restore` 默认只创建、替换、删除 managed files。
6. single-path `restore` 默认只创建、替换、删除指定 path 下的 managed files。
7. ignored/unmanaged files 默认不被 save 捕获，也不应被 restore 删除。

如果 restore 会影响 ignored/unmanaged files，默认必须拒绝或明确保护。`workspace remove` 更严格：只要目标 workspace 真实文件夹内仍有 ignored/unmanaged files，默认必须拒绝删除真实文件夹和 metadata，不能做“保留 ignored/unmanaged 文件但注销 workspace”的半删除。

需要显式 flag 才能删除或覆盖 ignored/unmanaged files，例如：

```text
--delete-unmanaged
--overwrite-unmanaged
```

这类 flag 必须高摩擦：preview 先展示真实路径、数量、大小和示例文件，用户显式确认后才能执行。

MVP 必须提供基础 ignore/unmanage 用户入口，不能等到 cleanup 才解决大生成物。例如支持 `jvs ignore add <path>` / `jvs unmanage <path>`，或等价的 documented ignore file/config。`save` 的大文件 warning 必须提示可用入口和将被排除的 workspace-relative path 规则。

路径分类必须以操作前重新扫描的真实文件系统为准。不能只信 watcher、缓存或 metadata。无法确认文件状态时，按 unsaved/unmanaged risk 处理并拒绝破坏性操作。

### Materialization Capacity And Source Pins

任何会读取保存点并物化内容的操作，都必须同时满足 source pin 和 capacity gate。

适用操作包括：

- `view <save> [path]`
- `restore <save>`
- `restore <save> --path <path>`
- `workspace new <name> --from <save>`
- `restore <save> --to <workspace> --save`
- recovery resume/rollback 中需要读取 source、safety save、snapshot 或临时 material 的步骤
- 任何未来会写临时 view copy、staging、workspace payload 或 rollback material 的命令

source pin 规则：

1. `<save>` 解析为具体 source save point 后，操作必须注册 `active_source_pin`。
2. pin 从 source resolve 成功开始，到操作成功完成、失败且无 recovery 需求、或 recovery plan 接管 source 引用后结束。
3. pin 期间 cleanup preview/run 必须把 source 列为 protected，原因是 `active_operation` 或更具体的 `active_view` / recovery plan protection。
4. 如果操作失败并创建 recovery plan，source pin 转移为 recovery plan cleanup protection；不得在恢复闭环前删除 source。
5. 如果 source 已是 tombstone/deleted，操作必须在读取前失败，错误类型为 `deleted-save`，不能注册半个 materialization。

capacity gate 规则：

1. 写入任何临时数据、view copy、workspace 文件、safety save、snapshot、rollback material 或新保存点前，必须估算容量。
2. 估算必须覆盖 source materialization、临时目录、目标 workspace delta、metadata/index 开销、safety save/recovery material，以及无法去重时的保守上限。
3. 估算必须扣除 configured safety margin；超过预算时必须 pre-write fail。
4. pre-write fail 不能写 partial data，不能改变 workspace files，不能改变 `history_head`、`content_source`、`path_sources`、`unsaved changes`，不能创建公开保存点。
5. 用户确认 large-file warning 不能绕过 capacity safety margin 硬拒绝。
6. 如果写入过程中发现实际消耗超过预算，操作必须停止在事务/recovery 边界，输出 recovery plan 或明确 rollback 结果。

### Save Point

一个 `save point` 捕获某个 workspace 在保存时的 managed files 完整内容。它不捕获 JVS 控制平面，也不捕获其他 workspace 文件夹。

保存点的不可改写创建事实至少包括：

- save point ID
- created time
- 保存时的 workspace name
- 保存时的 workspace folder path
- `parent`
- `started_from`，如适用
- `restored_from`，如适用
- `restored_paths`，如适用
- materialization engine 事实
- managed file set hash 或等价完整性证据
- save 创建时捕获的 message、note、key-value metrics/metadata，如实现支持

保存点发布后，内容和创建事实不可重写。

metadata trust model 必须拆成两类：

- creation-time captured metadata/metrics：创建保存点时由命令参数、扫描器或 materialization engine 捕获的 message、note、`--meta`、metric、managed set summary、engine fact。它们是保存点创建事实的一部分，默认不可静默改写。
- later editable annotations：保存后用户维护的 `label`、`keep`、展示用追加 note、correction annotation、review annotation 或索引字段。它们必须独立存储，并带 audit trail 或版本记录。

如果用户后续需要修正 creation-time metric，例如把 `acc=0.973` 修正为 `acc=0.937`，实现不得直接覆盖原始创建事实。允许的做法是：

- 写入带时间、操作者和原因的 correction annotation，并在展示中清楚区分原始值和修正值。
- 或创建新的保存点/新 annotation 表达修正后的解释。

editable note/label/annotation 仍然不是 ref、不是 alias、不是 cleanup protection。用户不能用 later annotation 作为 `<save>` 目标；重要保存点仍必须显式 `keep`。

重要边界：保存点不可改写，不代表保存点一定永久存在。`cleanup run` 可以删除不受 `keep`、live workspace 需求、active view、active operation、recovery plan 或 provenance policy 保护的保存点。

### Workspace State

每个 workspace 对用户展示这些事实：

- `Folder`
- `Workspace`
- `Newest save point`
- `Files match save point` / `Files were last restored from` / `Started from`
- `Restored paths`，如适用
- `Unsaved changes`

实现、JSON 和高级 map 必须进一步区分这些状态字段：

| 字段 | 含义 | 例子：`history_head=B` 时执行 `restore A` |
|------|------|-------------------------------------------|
| `history_head` | 此 workspace 最近追加的保存点。 | 仍是 `B` |
| `content_source` | 当前 whole-workspace 内容最近 copied/materialized from 的保存点。 | 变为 `A` |
| `started_from` | workspace 初始内容来自的外部保存点，只在没有继承 history 时使用。 | 不变或为空 |
| `path_sources` | 当前指定路径来自的保存点。 | full restore 时清空，single-path restore 时记录路径 |
| `has_unsaved_changes` | 当前真实文件是否相对 JVS 已知来源发生未保存改动。 | restore 完成后为 `false`，编辑后为 `true` |

这些高级字段不能合并。一个 workspace 的文件可以来自旧保存点 `A`，同时最新保存点仍然是更新的 `B`。下一次 `save` 会创建一个新的保存点，追加在 `B` 之后；高级 metadata 记录该保存点来自恢复过的 `A`。

### Workspace State Machine

状态机是 workspace-local 的。

| 状态 | `history_head` | `content_source` | `started_from` | `path_sources` | `unsaved changes` | 常见后续动作 |
|------|----------------|------------------|----------------|----------------|-------------------|--------------|
| 普通文件夹刚 init | `null` | `null` | `null` | empty | yes/not saved yet | `save`, `status` |
| `workspace new --from R` 后尚未保存 | `null` | `R` | `R` | empty | no | `save`, `view`, `restore` |
| 干净且位于 newest 内容 | `H` | `H` | maybe | empty | no | `save`, `history`, `view`, `restore` |
| whole restore 后但未编辑 | `H` | `R` | maybe | empty | no | `save`, `view`, `restore` |
| single-path restore 后但未编辑 | `H` | previous | maybe | `{path: R}` | no | `save`, `view`, `restore --path` |
| 有未保存改动 | `H` | any | maybe | maybe | yes | `save`，或显式 discard 后做破坏性操作 |

状态转换：

- `save` 创建新保存点 `S`，设置 `S.parent=<previous history_head>`。
- 如果是 workspace 的第一次保存，且 workspace 有 `started_from=R`，设置 `S.started_from=R`，并保持 `S.parent=null`。
- 如果保存前 `content_source` 与 previous `history_head` 不同，且这不是 first-save `started_from` 场景，则设置 `S.restored_from=<content_source>`。
- 如果保存前存在 `path_sources`，设置 `S.restored_paths=<path_sources>`。
- `save` 成功后，将 workspace `history_head` 和 `content_source` 都移动到 `S`，清空 `path_sources`，清除 `unsaved changes`。
- whole-workspace `restore R` 把 `R` 的 managed files 复制到 workspace，设置 `content_source=R`，清空 `path_sources`，清除 `unsaved changes`，但不移动 `history_head`。
- single-path `restore R --path p` 只替换 `p`，记录 `path_sources[p]=R`，清除该 path 的 dirty 状态，但不移动 `history_head`。
- 编辑真实文件会设置 `unsaved changes=true`；如果只编辑某些路径，实现可以保留 path-level dirty 信息用于 single-path guard。
- `view R` 不改变任何 workspace 状态。
- `workspace new <name> --from R` 创建新的真实 workspace，设置 `history_head=null`、`content_source=R`、`started_from=R`、`path_sources=empty`、`unsaved changes=false`。
- `workspace remove` 删除真实 workspace 文件夹和 workspace metadata；它不直接删除保存点。

### `path_sources` Normalization And Invalidation

`path_sources` 是 workspace 当前文件状态的 path-level provenance。它必须足够精确，避免 cleanup 错删 direct source，也避免 UI 暗示某个 path 仍精确匹配 source。

路径规范化规则：

1. 所有 key 使用 workspace-relative path。
2. 输入 path 必须经 lexical normalization 和 canonical containment check；禁止空 path、绝对路径、`..` 越界和通过 symlink 跳出 workspace root。
3. 目录 path 与文件 path 使用同一规范形式，不用尾随 slash 表达语义。
4. 重复 restore 同一路径时，新的 source 覆盖旧 entry。
5. restore 父目录时，删除该目录下已有 descendant entries，因为父目录 restore 已重新定义整个子树来源。
6. restore 子路径时，可以在父目录 entry 下新增更具体 child entry；展示和 dirty guard 按 most-specific entry 解释。
7. 如果父子 entry 来自同一 source 且 evidence 等价，实现可以 coalesce 为较短父目录 entry。

失效/降级规则：

- 如果 restored path 的内容仍与 source save point 中对应 path 的 evidence 匹配，entry 状态是 `exact`。
- 如果该 path 后续被编辑但仍存在，entry 降级为 `modified_after_restore`；默认人类输出应说“last restored from A, edited since”，不能说仍然 match。
- 如果该 path 后续被删除，entry 必须清除；下一次 save 不记录该 path 的 `restored_paths` provenance。
- 如果该 path 被外部 move/rename，默认不得猜测新路径 provenance；旧 entry 清除或降级为 unresolved，并要求重新扫描/重新 restore。只有 JVS 自己提供的未来 rename 操作可以显式迁移 entry。
- 如果 restore 的目录中只有部分文件被编辑，entry 可以保留目录级 `modified_after_restore`，也可以拆成更细 entries；但 JSON/测试必须能区分 exact 与 edited-since。

保存时：

- `save` 只把仍存在且未被清除的 `path_sources` 写入新保存点的 `restored_paths`。
- `restored_paths` 必须记录 source save point、source path、target path、状态（例如 `exact` 或 `modified_after_restore`）和保存时 evidence。
- 保存成功后 workspace `path_sources` 清空，因为新保存点成为当前 whole-workspace `content_source`。

cleanup 保护：

- live workspace 中 active `path_sources` 引用的 source save points 受 `live_workspace_path_source` 保护。
- live history 中保存点的 `restored_paths` direct sources 受 `provenance_protected` 保护。
- 被清除的 path source 不再产生 cleanup protection；如果已写入某个 retained save point 的 `restored_paths`，则由该保存点继续提供 direct source protection。

## Save ID / Label Resolver

接受 `<save>` 的命令只接受具体保存点 ID 或明确设计的 save ID prefix。`<save>` 永远不解析 label。即使某个 label 当前只匹配一个保存点，也不得自动用于 `view`、`restore`、`keep` 或其他需要具体保存点的命令。

适用命令包括：

- `view <save> [path]`
- `restore <save>`
- `restore <save> --path <path>`
- `restore <save> --to <workspace> --save`
- `keep <save>`
- `jvs label add <save> <label>`

如果用户把 label 输入到这些位置，必须失败，并显示匹配保存点列表，要求用户选择具体 save point ID。

错误文案示例：

```text
"best" is a label, not a save point ID.
Matching save points:
  A17c92  label=best  message="run 42 acc=0.93"
  B81de0  label=best  message="run 57 acc=0.94"
Choose a save point ID, then run the command again.
```

这个规则也适用于唯一匹配：

```text
"baseline" is a label, not a save point ID.
Matching save points:
  A17c92  label=baseline
Choose save point A17c92 explicitly.
```

### Finding Save Points Without Knowing The ID

用户不应先记住 save ID 才能恢复文件。V1 主发现路径选择：

```text
jvs history --path <path>
```

`history --path <path>` 用于列出“包含这个 path 或曾变更这个 path”的候选保存点。它是低心智发现路径，不改变任何 workspace 文件或 history。

必须支持的候选线索：

- workspace-relative path，例如 `src/config.json` 或 `data/run42/`。
- 文件名片段或 exact path filter，具体匹配模式必须在 help 中说明。
- 时间范围，例如 `--since` / `--until` 或等价过滤。
- message/note 文本过滤。
- label 过滤。
- key-value metadata/metric 过滤和排序。

输出必须返回候选列表和下一步命令，而不是把候选自动当成 restore target：

```text
Folder: /repo/main
Workspace: main
Candidates for path: src/config.json
  A17c92  2026-04-27  message="baseline"  labels=baseline
  B81de0  2026-04-27  message="tuned config"  labels=run
Choose a save point ID, then run:
  jvs view <save> src/config.json
  jvs restore <save> --path src/config.json
```

`jvs find <path>` 可以作为未来/别名候选，但 V1 文档和 help 必须以 `history --path <path>` 为主路径，避免同时教学两套入口。

`restore --path <path>` 如果未提供 `<save>`，不得猜测“最可能”的保存点，也不得静默恢复。它必须退化为候选发现模式：列出包含/变更该 path 的候选保存点，显示真实 `Folder`，要求用户选择具体 save ID 后再执行 `restore <save> --path <path>`。

label 查询也必须遵守同一规则。`history --label best`、`history --meta acc>=0.93` 或等价过滤可以返回候选列表；但 `label`、metadata、message、时间都不是 ref。输出必须包含下一步命令示例，例如 `jvs view A17c92`、`jvs restore A17c92 --path src/config.json`、`jvs keep A17c92`。

## Command Semantics

### `status`

`status` 展示目标 workspace 的当前状态。它必须让用户先看见真实文件夹路径，再看见 workspace name 和保存点状态。

必须语义：

- 从 CWD 或显式 workspace 参数解析目标 workspace。
- 输出 `Folder: /real/path`。
- 输出 `Workspace: <name>`。
- 默认人类输出使用 `Newest save point` 展示最新保存点。
- 默认人类输出使用精确来源文案：`Files match save point`、`Files changed since save point`、`Files were last restored from`、`Started from`、`Restored paths`。
- 默认人类输出使用 `Unsaved changes` 展示是否有未保存更改。
- 如果文件来源与最新保存点不同，必须说明 history 没有改变，下一次 `save` 会创建新的保存点。
- 如果展示 `label`，不得暗示 label 有保护作用。
- 如果展示 `keep`，必须把它描述为 cleanup protection。

示例：

```text
Folder: /repo/main
Workspace: main
Newest save point: B
Files changed since save point: B
Unsaved changes: yes
Next save creates a new save point after B.
```

如果先从另一个保存点恢复后再编辑：

```text
Folder: /repo/main
Workspace: main
Newest save point: B
Files were last restored from: A
Unsaved changes: yes
History was not changed.
Next save creates a new save point after B.
```

### `save`

`save` 从目标真实 workspace 的当前 managed files 创建新保存点。

必须语义：

1. 解析目标 workspace 和真实文件夹路径。
2. 获取 workspace operation lock。
3. 重新扫描真实文件系统，计算 managed/ignored/unmanaged set。
4. 操作前校验并记录 workspace 内容证据：managed set、size summary、hash/mtime/inode 或等价变化检测证据。
5. 输出或预先计算 save size summary：新增/变更大文件 top N、估算 logical size、估算 storage impact、large-path warning 和 generated-artifact ignore guidance。
6. 如果估算 storage impact 超过 configured threshold，默认要求用户确认，或建议先 ignore/unmanage 大生成物后重试。
7. 写入 unpublished staging 之前必须执行硬容量门：估算本次 staging/storage impact、目标存储可用空间和 configured safety margin。估算必须覆盖 staging 临时空间、最终保存点存储、索引/metadata 开销和无法去重时的保守上限。
8. 如果估算 impact 会超过可用空间扣除 safety margin 后的安全预算，必须 pre-staging fail：不进入 capture，不写 partial staging，不创建 save point，不移动 workspace 状态。用户确认 large-file threshold 不能绕过这个硬拒绝。
9. 只把该 workspace managed files 捕获到 unpublished staging；此时不能产生公开 save point，不能移动 workspace 状态。
10. JVS control data 永不捕获。
11. 捕获操作完成后、发布前，重新扫描并校验 workspace 内容证据与操作前一致。
12. 如果校验发现外部进程并发写入或 managed set/内容证据变化，操作必须失败；丢弃或隔离 unpublished staging；不产生公开 save point；不移动 `history_head`、`content_source`、`path_sources` 或 `unsaved changes`；提示用户停止写入后重试。
13. 只有校验一致时，构造并原子发布新保存点 `S`。
14. 新保存点 `S.parent=<save 前的 workspace history_head>`。
15. 如果这是 `workspace new --from R` 后的第一次保存，设置 `S.parent=null`、`S.started_from=R`。
16. 如果保存前 `content_source` 与 `history_head` 不同，且不是 first-save `started_from` 场景，则设置 `S.restored_from=<content_source>`。
17. 如果保存前有 `path_sources`，设置 `S.restored_paths=<path_sources>`。
18. 发布成功后，才将 workspace `history_head` 移到 `S`。
19. 将 workspace `content_source` 移到 `S`。
20. 清空 `path_sources`。
21. 清除 `unsaved changes`。
22. 释放 workspace operation lock。

`save` 的并发顺序必须是 pre-staging capacity gate -> staging-before-publish：先确认硬容量预算，再捕获到 unpublished staging，再做发布前校验，最后才公开保存点并移动 workspace 状态。容量门或一致性检测失败都不能留下用户可见的半个 save point。

`save` 支持展示用 message/note 和可选 key-value metadata：

```text
jvs save -m "train run 42" --note "larger batch" --meta acc=0.937 --meta seed=1234 --meta run_id=42
```

这些 metadata 是保存点展示和过滤信息，不是可恢复 ref。用户不能用 `acc=0.937`、`run_id=42` 或 label 作为 `<save>` 直接 restore；必须选择具体 save point ID。

默认人类输出不直接展示 `parent`、`restored_from`、`started_from` 或 `restored_paths` 字段名。它应说“saved after B”“started from A”或“includes restored path src/app.py from A”。JSON、高级 map 和测试断言可以展示字段名。

### `history`

`history` 默认查看当前 workspace。

必须语义：

- `history` 列出目标 workspace 自己追加过的线性保存历史。
- `history --all` 列出当前 JVS project 内所有保存点。
- `history --path <path>` 列出当前 workspace 中包含或变更该 path 的候选保存点；它是找回误删文件的主发现路径。
- `history --label <label>`、message/time/metadata filters 只筛选候选；不得把结果自动变成 `<save>`。
- `workspace new --from R` 后尚未保存时，`history` 必须说该 workspace 还没有自己的保存点，并展示 `Workspace started from R`。
- `workspace new --from R` 第一次 `save E` 后，`history` 从 `E` 开始；不得把 `R` 的历史线作为该 workspace 自己的历史。
- 当文件来源与最新保存点不同，默认人类输出必须同时展示“最新保存点”和“文件来源”。
- `history` 不得暗示 `restore` 回退、重排或删除了历史。
- `history --all` 是全局保存点列表，不是 workspace 切换 UI。
- 对已被 cleanup 删除的保存点，默认 `history` 不得假装它仍可恢复；如果保留 tombstone，应显示 deleted 状态，`view/restore` 下一步必须指向仍存在的 save point。

示例：

```text
Folder: /repo/workspaces/exp
Workspace: exp
History: no save points yet
Workspace started from: A
```

第一次保存后：

```text
Folder: /repo/workspaces/exp
Workspace: exp
History:
  E  "agent success"
Workspace started from: A
```

高级/JSON/测试断言：

```text
E.parent = null
E.started_from = A
```

### `view <save> [path]`

`view <save> [path]` 用于只读查看某个保存点，或只读查看保存点内的某个文件/目录。

必须语义：

- `<save>` 必须解析为唯一保存点 ID；不得解析 label。
- 解析 source 后必须注册 active source pin；view session active 期间 cleanup 不得删除 source save point。
- 可选 `path` 必须是 workspace-relative path，不能越过保存点 root。
- 如果传入 `path`，只暴露该文件或目录的 read-only view。
- 如果 view backend 需要写临时 copy/cache，必须在写入前通过 capacity gate；失败时不得创建 partial view path。
- 不改变任何 workspace 文件。
- 不改变 `history_head`。
- 不改变 `content_source`。
- 不改变 `started_from`。
- 不改变 `path_sources`。
- 不创建或清除 `unsaved changes`。
- 不改变 history metadata。
- 不改变 `label` 或 `keep` metadata。
- 不改变 cleanup plan。
- 不注册 workspace。
- 不把 view path 保护成普通 workspace。
- 如果无法保证只读，必须在暴露 view 前失败，并明确说明没有做任何修改。

`view` 的生命周期：

- 成功后创建一个 read-only view session/path，带 view ID。
- view path 可以是 read-only mount、read-only temporary copy、UI preview 或等价机制。
- 用户可以从 view path 手动复制文件内容到真实 workspace。
- view session 默认临时存在；可以通过 `jvs view close <view-id>` 显式释放。
- 非交互 CLI view 可以在命令退出时释放，除非实现显式输出持久 view path。
- 持久 view path 必须有 TTL 或 cleanup 策略，例如 24 小时后过期。
- active view 的 source save point 必须受 cleanup 保护，保护原因是 `active_view`。
- view 释放后，对应 cleanup protection 消失。

从 view path 运行 JVS 写命令必须失败，或者要求用户明确选择真实 workspace。

失败文案示例：

```text
This path is a read-only view of save point A, not a workspace.
No files or history were changed.
Run the command from a real folder, or pass --workspace /real/path.
```

成功文案示例：

```text
Opened read-only view.
Save point: A
Path inside save point: src/config.json
View: V42
View path: /tmp/jvs/views/V42/src/config.json
No workspace or history changed.
```

### `restore <save>`

`restore <save>` 把保存点 managed files 复制到当前 workspace，是 whole-workspace replacement。

必须语义：

1. `<save>` 必须解析为唯一保存点 ID；不得解析 label。
2. 解析 source 后注册 active source pin；操作完成或 recovery plan 接管前 cleanup 不得删除 source。
3. 解析目标 workspace 和真实文件夹路径。
4. 获取 workspace operation lock。
5. 重新扫描真实文件系统和 managed/ignored/unmanaged set。
6. 如果目标 workspace 有 `unsaved changes`，默认拒绝。
7. 如果用户显式选择 `--save-first`，先保存目标 workspace 当前未保存内容，再继续；后续 restore 时的最新保存点就是这次刚创建的新保存点。
8. 如果用户显式选择 `--discard-unsaved`，丢弃未保存内容，再继续。
9. 计算 replacement preview：将覆盖的 managed files 数量/样例、将删除的 managed files 数量/样例、将创建的 managed files 数量/样例、ignored/unmanaged 保留摘要、当前 newest save point 和可回到的 save point。
10. 如果 replacement 会覆盖或删除任何 managed files，默认必须 preview 且不修改文件，或要求高摩擦确认；不得静默执行整工作区替换。
11. destructive run 必须绑定 preview plan 或显式 expected target state；目标 newest save point 或扫描证据变化时必须拒绝并要求重新 preview。
12. 写入前必须通过 capacity gate，覆盖 source materialization、workspace delta、临时文件和 recovery/safety material。
13. 用 source 保存点 managed files 完整替换目标 workspace managed files。
14. 替换后，目标 managed files 必须完整匹配 source save point。
15. 目标中 source 不存在的 managed files 会被删除。
16. 目标 ignored/unmanaged files 默认保留；如果操作会覆盖/删除它们，必须拒绝，除非用户显式传入高摩擦 flag。
17. 设置目标 `content_source=<save>`。
18. 清空目标 `path_sources`。
19. 设置目标 `unsaved changes=false`。
20. 保持目标 `history_head` 不变。
21. 操作后重新检查目标文件没有被并发改写；发现变化则失败并进入恢复计划。
22. 操作 resolved 后释放 source pin 和 workspace operation lock。

命令成功文案必须说明：

- 影响的真实 `Folder`。
- managed files 现在匹配 source save point。
- 覆盖/删除的 managed files 数量或摘要。
- 只替换/删除 managed files；ignored/unmanaged files 保留。
- 最新保存点仍是哪个。
- history 没有改变。
- 下一次 `save` 会创建新的保存点，并记录来源。
- 用户可通过哪个 save point 回到 restore 前内容；通常是 restore 前的 newest save point，若使用了 `--save-first` 则是刚创建的 safety/newest save point。

成功文案示例：

```text
Folder: /repo/main
Workspace: main
Restored save point: A
Managed files now match save point A.
Only managed files were replaced or deleted.
Ignored/unmanaged files were kept.
Newest save point is still B.
History was not changed.
Next save creates a new save point after B.
```

preview 文案如果实现提供，必须展示 managed-only deletion / unmanaged kept：

```text
Preview only. No files were changed.
Folder: /repo/main
Workspace: main
Plan: R-restore-A-001
Source save point: A
This will replace managed files so they match A.
Managed files to overwrite: 18
Managed files to delete: 3
Examples:
  overwrite src/config.json
  delete tmp/old-result.txt
Ignored/unmanaged files will be kept.
History will not change.
Newest save point is still B.
You can return to save point B after this restore.
Expected newest save point: B
Expected folder evidence: ws-8f31c2
Run: jvs restore --run R-restore-A-001
```

preview/dirty 文案必须说清楚真实路径和可回退保存点：

```text
Refusing to overwrite unsaved changes.
Folder: /repo/main
Workspace: main
Newest save point: B
No files were changed.
Choose --save-first, --discard-unsaved, or cancel.
```

### `restore <save> --path <path>`

`restore <save> --path <path>` 是误删或误改单个文件/目录时的低风险主路径。用户不必恢复整个 workspace。

如果用户只知道 path，不知道 save ID，可以运行：

```text
jvs restore --path <path>
```

未提供 `<save>` 时，这个命令必须只列出候选保存点并要求用户选择具体 save ID；它不得猜测、不得恢复、不得修改 workspace。

候选输出示例：

```text
Folder: /repo/main
Workspace: main
No save point ID was provided.
Candidates for path: src/config.json
  A17c92  message="baseline"
  B81de0  message="tuned config"
Choose a save point ID, then run:
  jvs restore A17c92 --path src/config.json
No files were changed.
```

必须语义：

1. `<save>` 必须解析为唯一保存点 ID；不得解析 label。
2. 解析 source 后注册 active source pin；操作完成或 recovery plan 接管前 cleanup 不得删除 source。
3. 解析目标 workspace 和真实文件夹路径，并获取 workspace operation lock。
4. `<path>` 必须是 workspace-relative path，并按 `path_sources` normalization 规则规范化。
5. `<path>` 可以是文件或目录。
6. 默认只恢复当前 workspace。
7. 只替换指定 path 下的 managed files。
8. 不触碰 path 外的 managed files。
9. 不触碰 ignored/unmanaged files，除非用户显式传入高摩擦 flag。
10. 如果 source save point 中不存在 `<path>`，默认失败并说明没有修改任何文件。
11. 如果目标 `<path>` 下存在会被覆盖或删除的 unsaved managed content，默认拒绝。
12. 如果目标 `<path>` 已被误删且 source save point 中存在该 path，可以直接恢复；这是创建缺失内容，不需要恢复整个 workspace。
13. dirty guard 只检查和保护相关 path；path 外的 unsaved changes 不阻止 single-path restore。
14. 如果用户传入 `--save-first`，先保存整个 workspace 当前内容，再恢复该 path。
15. 如果用户传入 `--discard-unsaved`，只丢弃该 path 的未保存改动，再恢复该 path；path 外未保存改动保留。
16. 如果 `<path>` 是目录，source 中不存在的目标 managed files 会在该目录内删除，以便该目录 managed files 匹配 source。
17. 写入前必须通过 capacity gate，覆盖 source path materialization、目标 path delta、临时文件和 recovery/safety material。
18. destructive run 必须绑定 preview plan 或显式 expected target state；目标 newest save point 或相关 path 扫描证据变化时必须拒绝并要求重新 preview。
19. 设置或更新 `path_sources[<normalized path>]=<save>`，并按父子路径覆盖/合并规则更新 entries。
20. 不移动 `history_head`。
21. 不改变 whole-workspace `content_source`，除非实现选择在 JSON 中表达 mixed source；默认人类输出用 `Restored paths` 表达。
22. 操作成功后，相关 path 的 dirty 状态清除；如果 path 外还有未保存改动，workspace 仍显示 `Unsaved changes: yes`。
23. 后续 `save E` 线性追加到当前 `history_head` 后面，并记录 `E.restored_paths`。
24. 操作 resolved 后释放 source pin 和 workspace operation lock。

如果 single-path restore 只触碰一个路径，且不会删除大量 managed files，可以使用更轻量的确认文案；但它仍然必须绑定 preview plan 或显式 expected target state。也就是说，run 前仍要重校验 target newest save point 和相关 path 的扫描证据，发现变化就拒绝并要求重新 preview。

成功文案示例：

```text
Folder: /repo/main
Workspace: main
Restored path: src/config.json
From save point: A
Newest save point is still B.
History was not changed.
Next save creates a new save point after B and records this restored path.
```

只读查看加手动复制也是支持路径：

```text
jvs view A src/config.json
```

用户可以从 read-only view 中手动复制文件内容。区别是：

- `view A src/config.json` 不改变真实 workspace，也不记录 provenance。
- `restore A --path src/config.json` 会替换真实 workspace 中该 path，并在后续 save 中记录 path-level provenance。

### `workspace new <name> --from <save>`

`workspace new <name> --from <save>` 从保存点创建新的真实 workspace 文件夹，但不继承外部保存点作为自己的 `history_head`。

必须语义：

1. `<save>` 必须解析为唯一保存点 ID；不得解析 label。
2. 解析 source 后注册 active source pin；操作完成或 recovery plan 接管前 cleanup 不得删除 source。
3. `<name>` 必须是合法且未使用的 workspace name。
4. 新 workspace folder 必须 canonicalized，且不得与任何已注册 workspace nested/overlap。
5. 写入前必须通过 capacity gate，覆盖 source materialization、新 folder payload、metadata/index 和 recovery/safety material。
6. 在配置的 workspace 位置创建新的真实文件夹。
7. 把 source 保存点 managed files 复制/准备到该真实文件夹。
8. 注册 workspace metadata。
9. 设置新 workspace `history_head=null`。
10. 设置新 workspace `content_source=<save>`。
11. 设置新 workspace `started_from=<save>`。
12. 设置新 workspace `path_sources=empty`。
13. 设置 `unsaved changes=false`。
14. 不影响原 workspace。
15. 不改写 source 保存点。
16. 不把 source 保存点或 source workspace 的 parent chain 作为新 workspace 的历史线。
17. 输出中必须先显示新 workspace 的真实 `Folder`，再显示 workspace name。
18. 操作 resolved 后释放 source pin。

默认落点必须可解释：如果用户没有显式指定路径，`workspace new <name> --from <save>` 在 JVS 配置的 workspace 根目录下创建 `<name>` 文件夹；本文示例使用 `/repo/workspaces/<name>`。实现可以配置不同根目录，但输出必须显示最终真实 `Folder`。

如果 workspace name 或默认落点已经存在，命令必须失败，不创建、不移动、不复制任何文件，并要求用户选择新名字：

```text
Workspace name already exists: experiment.
Folder already exists: /repo/workspaces/experiment
Choose a new workspace name.
No files were changed.
```

成功文案示例：

```text
Folder: /repo/workspaces/experiment
Workspace: experiment
Started from save point: A
Newest save point: none
Original workspace unchanged.
```

第一次在该 workspace 执行 `save E` 时，默认输出应说：

```text
Folder: /repo/workspaces/experiment
Workspace: experiment
Saved first save point E for this workspace.
Workspace started from save point A.
```

默认 `history` 输出应说：

```text
Folder: /repo/workspaces/experiment
Workspace: experiment
History:
  E  "agent success"
Workspace started from: A
```

不得把 `A` 的历史线展示成 `experiment` 的线性 history。

cleanup 保护规则：

- 活跃 workspace 尚未保存且 `started_from=R` 时，`R` 作为 direct source/provenance 受保护。
- 活跃 workspace 的首个保存点 `E.started_from=R` 仍在 live history 中时，`R` 作为 direct provenance/source 受保护。
- 默认只保护 direct `started_from` 来源。
- 如果未来要保护传递闭包，必须在 cleanup policy 中显式升级，并更新 preview/run 输出。

### `restore <save> --to <workspace>`

不带 `--save` 的 `restore <save> --to <workspace>` 先不进入公开支持范围。当前规范只定义：

- `restore <save>`：whole-workspace replacement 到当前 workspace，不移动 history。
- `restore <save> --path <path>`：single-path restore 到当前 workspace，不移动 history。
- `restore <save> --to <workspace> --save`：whole-workspace replacement 到目标 workspace，并立即保存为目标的新保存点。

实现若收到 `restore <save> --to <workspace>` 且没有 `--save`，必须失败并提示使用上述受支持形式之一。这样可以避免“只复制到另一个 workspace 的 working files”绕过 dirty guard、事务边界和用户心智。

### `restore <save> --to <workspace> --save`

`restore <save> --to <workspace> --save` 表示“把 source 保存点的内容复制到目标 workspace，并保存为目标的新保存点”。它不是 merge，不是跨 workspace 继承历史，也不是修改 source workspace。

这是 whole-workspace replacement：

- target managed files 会完整匹配 source save point。
- target 中与 source 不一致的 managed files 会被覆盖。
- target 中 source 不存在的 managed files 会被删除。
- target ignored/unmanaged files 默认保留；如果会被影响，默认拒绝，除非用户显式传入高摩擦 flag。

必须语义：

1. 解析 source `<save>`；不得解析 label。
2. 注册引用 source 保存点的 active operation；从读取 source 到操作 resolved 期间，cleanup 必须以 `active_operation` 保护 source 保存点。
3. 解析 target `<workspace>` 和真实文件夹路径。
4. 获取 target workspace operation lock。
5. 对 target workspace 重新扫描真实文件系统。
6. 对 target workspace 执行 dirty guard。
7. 如果使用 `--save-first`，先把 target workspace 当前未保存内容保存为一个 target 保存点；后续复制并保存的结果追加在这个新 target 保存点后面。
8. 如果使用 `--discard-unsaved`，先丢弃 target workspace 未保存内容。
9. preview 必须展示 target folder、source save point、将覆盖/删除的 target managed files 数量/样例、ignored/unmanaged 保留、history 不改变到复制完成前、以及 rollback point。
10. destructive run 必须绑定 preview plan 或显式 expected target state；target newest save point 或扫描证据变化时必须拒绝并要求重新 preview。
11. 写入前必须通过 capacity gate，覆盖 source materialization、target workspace delta、临时文件、新保存点 storage、metadata/index 和 recovery/safety material。
12. 将 source 保存点 managed files 复制到 target workspace，使 target managed files 完整匹配 source。
13. 在 target workspace 创建新保存点 `T`。
14. 设置 `T.parent=<T 保存前的 target history_head>`。
15. 设置 `T.restored_from=<source save>`。
16. 将 target workspace `history_head` 和 `content_source` 移到 `T`。
17. 清空 target `path_sources`。
18. 设置 target `unsaved changes=false`。
19. source workspace 不变。
20. source 保存点不变。
21. 操作后重新检查 target 没有被并发改写；发现变化则失败并进入恢复计划。
22. 操作 resolved 后释放 source active-operation protection。
23. 释放 target workspace operation lock。

成功文案示例：

```text
Folder: /repo/workspaces/release
Workspace: release
Copied save point A into this folder.
Managed files now match save point A.
Only managed files were replaced or deleted.
Ignored/unmanaged files were kept.
Saved as C in release.
Created after release's previous newest save point B.
Created from copied save point A.
Source workspace unchanged.
```

preview 文案必须说明影响范围：

```text
Preview only. No files were changed.
Target folder: /repo/workspaces/release
Target workspace: release
Plan: R-copy-A-to-release-001
Source save point: A
This will replace target managed files so they match A.
Managed files only in target will be deleted.
Ignored/unmanaged files will be kept.
Rollback point: B
Expected newest save point: B
Expected folder evidence: ws-3c82aa
Run: jvs restore --run R-copy-A-to-release-001
```

dirty guard 文案必须指出 target workspace 和 path：

```text
Refusing to overwrite unsaved changes.
Target folder: /repo/workspaces/release
Target workspace: release
Newest save point: B
No files were changed.
Choose --save-first, --discard-unsaved, or cancel.
```

失败边界必须采用事务化优先策略：

- 复制和保存要么全部成功，要么 target workspace 回到操作前状态。
- 如果保存失败，target workspace 的最新保存点、文件来源和未保存更改状态必须恢复到操作前。
- 对应高级字段也必须恢复到操作前：`history_head`、`content_source`、`started_from`、`path_sources`、`has_unsaved_changes`。
- 如果存储 backend 无法提供完整事务化，实现必须在复制前创建 safety save、snapshot 或等价 recovery plan。
- 非事务化 fallback 必须在失败时进入显式可恢复状态，并输出 recovery plan ID。
- 非事务化 fallback 不得静默留下“文件已复制但未保存，history 状态不清楚”的状态。

### `workspace remove <name>`

`workspace remove <name>` 删除真实 workspace 文件夹，是高危操作。

必须语义：

1. 不能删除 `main` workspace。
2. 默认先 preview，或要求明确 confirm。
3. preview 必须显示真实 `Folder`、workspace name、managed/ignored/unmanaged 摘要、unsaved 状态。
4. 执行前必须重新扫描真实文件系统，不能只信 watcher/metadata。
5. 执行前必须获取 workspace operation lock。
6. destructive run 必须绑定 preview plan 或显式 expected target state；workspace newest save point、unsaved 状态、managed/ignored/unmanaged 扫描证据或 canonical folder path 变化时必须拒绝并要求重新 preview。
7. 必须重算 managed files 的 unsaved 状态；如果存在 unsaved managed changes，默认拒绝，并要求用户选择：`--save-first`、`--discard-unsaved` 或取消。
8. 如果真实 workspace folder 内存在 ignored/unmanaged files，默认拒绝删除真实文件夹和 workspace metadata；它不做“删除 metadata 但留下 folder”的半删除。用户必须先移走这些文件，或显式传入 `--delete-unmanaged` / 等价高摩擦 flag 才能删除。
9. 如果 remove 需要 safety save、snapshot、trash staging 或 rollback material，写入前必须通过 capacity gate。
10. 成功删除时，删除整个真实 workspace folder 和 workspace metadata。
11. 不删除任何保存点。
12. 输出必须提示：该 workspace 的未 keep 保存点未来可能成为 cleanup candidates。
13. 如果删除失败，不得留下 unregistered folder 或 orphan metadata；无法事务化时必须进入 recovery plan，并输出 recovery plan ID 或明确的 retry/rollback 语义。
14. 删除完成后释放 operation lock。

preview 文案示例：

```text
Preview only. No files were deleted.
Folder: /repo/workspaces/experiment
Workspace: experiment
Managed files: 12, size 84 MB
Ignored files: 1, size 1.1 GB
Unmanaged files: 1, size 100 MB
Unsaved managed changes: no
Save points will not be deleted by workspace remove.
Unkept save points from this workspace may be candidates in a future cleanup preview.
```

如果默认拒绝：

```text
Refusing to remove workspace with ignored/unmanaged files.
Folder: /repo/workspaces/experiment
Workspace: experiment
Managed files: 12, size 84 MB
Ignored files: 1, size 1.1 GB
Unmanaged files: 1, size 100 MB
Unsaved managed changes: no
No files or metadata were deleted.
Move these files elsewhere, or rerun with --delete-unmanaged after preview.
```

失败恢复语义：

- 如果文件删除中途失败，workspace 进入 `remove_pending` 或等价可恢复状态。
- 输出 `Recovery plan: RP...`。
- `jvs recovery status RP...` 显示已删除/未删除路径、workspace metadata 状态和可用动作。
- `jvs recovery resume RP...` 重试删除剩余路径。
- `jvs recovery rollback RP...` 尽力恢复操作前状态；无法恢复的 unmanaged deletion 必须在 plan 中明确列出。

### `label`

`label` 是分类 metadata，只用于过滤、分组和展示。

必须语义：

- `label` 不是保存点身份。
- `label` 不是可恢复目标。
- `label` 不是 alias。
- `label` 不是 protection。
- `label` 不是 retention。
- 多个保存点可以共享同一个 label，除非未来显式设计不同的受限 metadata 类型。

接受 `<save>` 的命令不得把 label 静默解析成保存点。如果 UI 允许按 label 过滤，用户仍必须选择一个具体保存点来执行 `view`、`restore` 或 `keep`。

推荐 Data/ML 工作流中，`label best` 只用于筛选候选；真正重要的结果必须选择具体 save ID 后执行 `keep`。

### `keep`

`keep` 用来防止 cleanup 删除某个具体保存点。

必须语义：

- `keep <save>` 保护一个具体保存点。
- `keep remove <save>` 或等价显式操作移除保护。
- `keep` 与 `label` 完全分离。
- 被 keep 的保存点必须在 cleanup 输出中显示为 protected。
- 被 keep 的保存点会保护直接解释它所需的保存点；默认人类输出应说 `needed to explain kept save point C`，不要说内部字段名。
- `keep` 不改写保存点内容或创建事实。

### `cleanup preview`

`cleanup preview` 计算清理计划并解释原因，但不删除任何保存点。

必须语义：

- 默认 cleanup 行为是 preview，不是删除。
- preview 生成或返回 plan ID。
- preview 列出 protected save points 和原因。
- preview 列出 deletion candidates 和原因。
- preview 必须说明 `label` 不保护保存点。
- preview 必须说明 `keep` 会保护保存点。
- preview 必须说明被 keep 保存点的 direct explanatory sources 会被级联保护。
- preview 必须把被 live history 中保存点通过 direct `restored_from`、`restored_paths` 或 `started_from` 引用的来源保存点列为 provenance/source protected。
- preview 必须说明 retained save point 的 direct sources 若不受保护且被删除，将保留 tombstone/ID/display reason。
- preview 必须保护 active view 的 source save points。
- preview 必须保护 active source pin / active operation 引用的 save points。
- preview 必须保护 active recovery plan 引用的 save points。
- preview 必须包含足够的 repository identity 和 candidate 信息，以便 `cleanup run` 后续绑定计划。
- preview 必须包含 size impact。

size impact 至少包括：

- deletion candidates 的 logical size 总量。
- estimated unique reclaimable size。
- shared/protected bytes，说明因为去重、共享块、keep、live workspace、active source pin、active operation 或 recovery plan 而不能释放的字节。
- 重要大文件摘要，例如 top N candidate files 或 save points。
- protected-but-large 列表。
- 带 label 但未 keep 的提醒。
- removed workspace / orphaned / provenance-protected 等原因分类大小。

V1 cleanup 默认不清理活跃 workspace 的旧 history chain。也就是说，不带显式 opt-in 时，live workspace 的 `history_head` 及 parent chain 默认都受保护。

V1 同时必须提供高摩擦 active history cleanup/prune 模式，用低心智文案称为“clean old save points in this folder”：

```text
jvs cleanup preview --workspace <name> --include-active-history
```

该模式仍然默认只 preview，不删除。它只能把目标 active workspace 中满足全部条件的旧保存点列为 candidates：

- 未被 `keep` 保护。
- 不是该 workspace 当前 `history_head` / `Newest save point`。
- 不是当前 `content_source`。
- 不是当前 `path_sources` 或 `started_from` 引用的 source。
- 不是 active view、active operation、active recovery plan 引用的 source。
- 不是 retained kept save point 的 direct explanatory source。
- 不是 retained live save point 的 direct `restored_from`、`restored_paths` 或 `started_from` source。

`--include-active-history` preview 必须额外展示：

- 真实 `Folder` 和 workspace name。
- 这是 active workspace old save point cleanup，不会删除当前文件夹内容。
- history 会保留 tombstone/audit，已删除 save point 不能再 view/restore。
- `Newest save point` 会保留。
- `keep` 保护哪些保存点。
- protected-but-large 保存点及保护原因。
- labelled-but-unkept candidates，提醒 label 不保护。
- estimated unique reclaimable size 和 shared/protected bytes。

不得使用 `branch` / `GC` 等心智解释这个模式；默认文案应说“clean old save points in this folder”。

### `cleanup run`

`cleanup run` 执行之前由 `cleanup preview` 生成的清理计划。

必须语义：

1. 必须显式指定 plan ID 或 plan file。
2. 校验 plan 属于当前 JVS project。
3. 重新计算 protection set、candidate set 和 size impact。
4. 如果重算结果与绑定 plan 不一致，必须失败且不删除任何保存点。
5. 保护所有 kept save points，并保护它们的 direct explanatory sources：`parent`、`restored_from`、`started_from` 和 `restored_paths` sources，如存在。
6. 保护 live workspace 所需保存点：
   - 每个 live workspace 的 `history_head`
   - live workspace history 所需 parent chain
   - 每个 live workspace 的 `content_source`
   - 每个 live workspace 的 `path_sources`
   - 每个 live workspace 的 `started_from`
   - live history 中保存点通过 direct `restored_from`、`restored_paths`、`started_from` 引用的来源保存点
   - active view 引用的保存点
   - active source pins、active operations 和 recovery plans 引用的保存点
7. 只删除重校验后仍然 unprotected 的 candidates。
8. 记录 audit information。
9. 输出每个保留/删除决定的原因。
10. 输出实际删除大小和预计/实际释放空间对比。

如果绑定 plan 来自 `--include-active-history`，第 6 条中的 live workspace parent chain 保护只对未选择 active-history cleanup 的 workspace 保持默认。对被显式选择的目标 workspace，run 可以删除重校验后仍符合 preview 条件的旧 history save points，但必须保留 tombstone/ID/display reason，并且不得删除当前 newest save point、active content/source/path sources、keep、active operation/recovery/view 或 direct protected provenance。

`cleanup run` 绝不能把 `label` 当作 retention rule。

## Dirty Guard / Operation Lock / Concurrency

会覆盖或删除真实文件的操作，在目标 workspace 有相关 `unsaved changes` 时默认拒绝。

必须受 dirty guard 保护的操作包括：

- `restore <save>`
- `restore <save> --path <path>`
- `restore <save> --to <workspace> --save`
- `workspace remove <name>`
- 任何未来会 replace、clear 或 delete workspace files 的操作

`save` 虽然不是破坏性操作，也必须参与 workspace operation lock 和并发检测，以免 agent/外部进程边写边保存导致保存点不一致。

`save` 的并发规则比普通写 metadata 更严格：

- 写入 unpublished staging 之前必须通过硬容量门，确认估算 staging/storage impact 不会超过可用空间扣除 safety margin 后的安全预算。
- 硬容量门失败必须在 capture 前拒绝；用户确认 large-file threshold 只能继续软阈值 warning，不能绕过安全余量硬拒绝。
- capture 必须先写入 unpublished staging，不能先发布公开保存点。
- capture 前记录 workspace 内容证据，capture 后、发布前再次校验。
- 只有前后证据一致，才能发布 save point 并移动 workspace `history_head`、`content_source`、`path_sources` 和 `unsaved changes`。
- 校验失败时，不产生公开 save point，不移动 workspace 状态；输出必须提示用户停止 agent/外部写入后重试。

允许显式选择：

- `--save-first`: 先保存目标 workspace 的未保存内容，再继续。
- `--discard-unsaved`: 丢弃目标 workspace 的未保存内容，再继续。

两个选项互斥。

强制安全规则：

1. 破坏性操作前必须重新扫描真实文件系统。
2. 不能只信 watcher、缓存或 metadata。
3. 无法确认真实文件状态时，按 unsaved changes 处理。
4. 操作必须持有 workspace operation lock。
5. 操作开始前记录目标 managed set、ignored/unmanaged set、hash/mtime/inode 或等价变化检测证据。
6. 操作后再次检测相关内容是否被外部进程并发写入。
7. 发现并发变化时，操作必须失败，并提示用户停止写入后重试；必要时创建 recovery plan。
8. 对 single-path restore，dirty guard 和并发检测可以收敛到相关 path，但必须保护该 path 下所有 managed/ignored/unmanaged risk。
9. 对 whole-workspace restore/remove，dirty guard 和并发检测覆盖整个 workspace。

错误文案必须显示真实 workspace path，并提供安全选择。错误文案不得暗示 history 已经改变。

### Expected Target State / Plan Binding

destructive/replacement 操作必须防止 stale preview 和多 agent 互相覆盖。V1 支持两种等价入口：

- preview -> run：preview 生成 operation plan，run 绑定 plan ID。
- automation direct run：命令显式传入 expected target state，例如 `--expect-head <save|none>`，以及实现要求的 content evidence token。

推荐命令形态：

```text
jvs restore <save> --preview
jvs restore --run <plan-id>

jvs restore <save> --to <workspace> --save --preview
jvs restore --run <plan-id>

jvs restore <save> --expect-head <save-id|none> --expect-state <evidence-token>
```

single-path restore 如果只影响一个路径且不会删除大量 managed files，可以使用较轻的确认体验；但直接执行时仍必须带 `--expect-head` 和实现要求的 path/content evidence，或先 preview 再 run。

适用操作包括：

- `restore <save>`
- `restore <save> --path <path>`
- `restore <save> --to <workspace> --save`
- `workspace remove <name>`
- 任何未来会 replace/delete workspace files 或 workspace metadata 的操作

operation plan 至少绑定：

- JVS project identity。
- target workspace name。
- target canonical folder path。
- target expected newest save point / `history_head`，没有保存点时为 `none`。
- preview 时扫描的 managed/ignored/unmanaged summary。
- preview 时相关 content evidence：hash/mtime/inode/size summary 或等价 token。
- source save point ID，如适用。
- path scope，如适用。
- planned overwrite/delete/create counts 和样例。
- generated at time、plan expiry 和 CLI command shape。

run 前必须重新解析 target workspace、canonical folder path、newest save point 和扫描证据。如果任一 expected value 变化，run 必须失败，不得写入 partial data，不得改变 workspace state，并提示重新 preview/plan。

失败示例：

```text
Folder changed since preview.
Folder: /repo/main
Expected newest save point: B
Current newest save point: C
No files were changed.
Run preview again before restoring.
```

这条规则的目标是防止两个 agent 同时操作同一 workspace：一个 agent preview 后，另一个 agent 新保存了 `C`，第一个 agent 的 restore/remove run 必须拒绝，而不是覆盖新状态。

### Materialization Capacity Gates

`save` 的 staging capacity gate 只是通用规则的一部分。所有 materialization operations 都必须在写入前估算容量和 safety margin，包括：

- read-only view 需要落盘 temporary copy/cache。
- whole-workspace restore。
- single-path restore。
- `workspace new --from`。
- `restore --to --save`。
- `--save-first`、safety save、snapshot、rollback material。
- recovery resume/rollback。

超过预算时必须 pre-write fail：

- 不写 partial data。
- 不创建 partial view path/new workspace。
- 不改变 target workspace files。
- 不移动 `history_head`、`content_source`、`path_sources` 或 `unsaved changes`。
- 不创建公开 save point。

## Recovery Plan

restore、`restore --to --save`、`workspace remove` 等操作必须优先事务化。无法保证完整事务化或操作中途失败时，必须进入显式 recovery plan，而不是留下模糊状态。

MVP 边界选择：MVP 必须包含最小 recovery status/resume/rollback，而不是只依赖“full restore/path restore 完全事务化”的实现假设。实现可以在底层做到事务化；但公开验收必须覆盖失败后 `jvs recovery status`、`resume`、`rollback` 的闭环路径。这样即使不同 filesystem/backend 无法提供强事务，用户也不会遇到“文件改了一半、history 状态不清楚”的状态。

MVP 中 `restore <save>`、`restore <save> --path <path>`、`--save-first` 产生的 safety save/snapshot，以及 view/restore materialization 的临时数据，都必须能被 recovery plan 或明确 rollback 结果解释。

### Recovery Plan Contents

每个 recovery plan 至少包含：

- recovery plan ID
- JVS project identity
- operation type
- target workspace name
- target real folder path
- source save point，如适用
- 操作前 `history_head`
- 操作前 `content_source`
- 操作前 `started_from`
- 操作前 `path_sources`
- 操作前 `has_unsaved_changes`
- 操作前 managed/ignored/unmanaged 摘要
- 操作前 expected target state / content evidence
- capacity estimate 和 safety margin decision
- safety save、snapshot、file backup 或等价 rollback material，如适用
- 已完成步骤
- 未完成步骤
- active locks 或 lock handoff 状态
- cleanup protection requirements
- recommended next command

### Recovery Commands

V1 必须提供文档层等价命令：

```text
jvs recovery status
jvs recovery status <plan>
jvs recovery resume <plan>
jvs recovery rollback <plan>
```

语义：

- `status` 列出 active recovery plans。
- `status <plan>` 展示 plan 详情、受影响真实路径、可用动作和风险。
- `resume <plan>` 从安全点继续完成原操作。
- `rollback <plan>` 尽力回到操作前状态，并恢复 workspace metadata。

如果 rollback 不能完整恢复，例如 unmanaged files 已被用户或外部进程删除，必须明确列出不可恢复项。

### Lifecycle

- 创建后，recovery plan 处于 active 状态。
- active plan 引用的保存点、safety save、view/source material 必须受 cleanup 保护。
- 同一 workspace 上新的 destructive operation 必须拒绝，直到 active plan 被 resume/rollback 解决。
- plan 成功 resume 或 rollback 后进入 resolved 状态。
- resolved plan 可保留 audit 记录，但不再保护 cleanup。
- 超过保留期的 resolved plan 可以自动清理。
- active plan 不得自动清理。

失败输出示例：

```text
Restore did not finish safely.
Folder: /repo/main
Workspace: main
Recovery plan: RP-20260427-001
No history was changed after the last confirmed step.
Run: jvs recovery status RP-20260427-001
```

## Data / ML Save Point Metadata

Data/ML 工作流需要把一次运行的上下文显示在保存点上，但这些信息不得变成 ref。

`save` 应支持：

- message，例如 `-m "train baseline"`
- note，例如 `--note "lr=3e-4, changed augmentation"`
- key-value metadata，例如 `--meta acc=0.937 --meta run_id=42 --meta seed=1234`
- label，例如 `--label run --label candidate`

展示示例：

```text
A17c92  "train run 42"  acc=0.937  seed=1234  labels=run,candidate
```

这些字段用于展示、排序、过滤、搜索和 cleanup 提醒；它们不是 save ID。`restore acc=0.937`、`view run_id=42`、`restore best` 都必须失败并要求选择具体 save point ID。

推荐 best result 工作流：

```text
jvs save -m "baseline" --meta run_id=base --label baseline
jvs save -m "train run 42" --meta acc=0.937 --meta seed=1234 --label run
jvs save -m "train run 57" --meta acc=0.940 --meta seed=5678 --label run
jvs history --label run --meta acc>=0.93 --sort acc:desc
jvs label add B81de0 best
jvs keep B81de0
```

numeric metadata 排序/过滤输出必须按数值语义排列候选，然后让用户选择具体 save ID：

```text
Folder: /repo/main
Workspace: main
Candidates:
  B81de0  "train run 57"  acc=0.940  seed=5678  labels=run
  A17c92  "train run 42"  acc=0.937  seed=1234  labels=run
Choose a save point ID, then run:
  jvs view B81de0
  jvs label add B81de0 best
  jvs keep B81de0
```

规则：

- `label best` 只用于筛选候选。
- `best` label 可以同时出现在多个候选上；它不是唯一 alias，也不会自动替换旧 best。
- 替换或移动 best 必须由用户显式执行，例如先 `jvs label remove <save> best` 再 `jvs label add <save> best`。
- 用户必须选择具体 save ID。
- 重要结果必须 `keep`。
- metadata 中可识别为数值的值必须按数值语义排序和过滤，而不是按字符串排序/比较，例如 `acc=0.94` 应排在 `acc=0.937` 之后，`acc>=0.9` 不能按字典序解释。
- `save` 创建时写入的 metrics 属于保存点创建事实；后续可编辑 metadata/note/label 必须走独立 annotation/audit，不得改写创建事实。
- 后续 metric correction 必须展示 audit trail 或展示为新的 correction annotation；不能静默把原始 `--meta` 值覆盖掉。
- editable annotation 可以参与展示、搜索、过滤和 cleanup 提醒，但仍不是 ref，也不提供 cleanup protection。
- cleanup preview 必须提醒“带 label 但未 keep”的大保存点可能被删除。

## Cleanup / Retention

cleanup 是两阶段：

```text
cleanup preview -> cleanup run
```

默认阶段是 preview。删除必须通过显式 `cleanup run`，并且绑定到某个 preview 计划。

保护类别：

| Protection | 含义 |
|------------|------|
| `keep` | 用户显式保护该保存点不被 cleanup 删除。 |
| `keep_explanatory_source` | 被 keep 保存点的 direct explanatory source：`parent`、`restored_from`、`started_from` 或 `restored_paths` source。 |
| `live_workspace_history` | live workspace 的 `history_head` 或 parent chain 需要该保存点。 |
| `live_workspace_content_source` | live workspace 当前文件是从该保存点 materialized/copied from。 |
| `live_workspace_path_source` | live workspace 当前某些路径是从该保存点恢复的。 |
| `live_workspace_started_from` | live workspace 尚未保存或首个 live save point 记录了该来源。 |
| `provenance_protected` | live history 中仍有保存点通过 direct `restored_from`、`restored_paths` 或 `started_from` 引用该来源保存点。 |
| `active_view` | active view session 正在引用该保存点。 |
| `active_operation` | 未安全完成的 operation、active source pin 或 recovery plan 正在引用该保存点。 |

删除候选类别：

| Candidate Reason | 含义 |
|------------------|------|
| `removed_workspace_history` | 保存点只属于已移除 workspace 的历史，且没有保护。 |
| `orphaned_provenance` | 保存点只被不属于 live history 的 provenance 引用，且没有 `keep` 或其他保护。 |
| `unkept_labeled_save` | 有 label 但未 keep，且不受 live/provenance 保护；preview 必须高亮提醒。 |
| `active_workspace_old_save` | 用户显式 `--include-active-history` 后，active workspace 中未 keep、非 current latest、非 active source、非 direct protected provenance 的旧保存点。 |

direct provenance 的 cleanup 策略固定如下：

- 如果某个保存点 `S` 被 `keep`，cleanup 必须默认保护 `S` 的 direct explanatory sources：`S.parent`、`S.restored_from`、`S.started_from`、`S.restored_paths` 中的 source save points，如存在。这样被 keep 保存点的详情页和 map 始终可解释。
- live workspace 的 parent chain 已由 `live_workspace_history` 保护；这里的 keep 级联规则覆盖 removed workspace 或其他 non-live history 中被 keep 的保存点。
- 显式 `--include-active-history` 是 live workspace parent chain 保护的唯一 V1 例外；它只允许删除未 keep 且不受其他 source/provenance 规则保护的旧 parent/history 保存点，并必须保留 tombstone。
- 如果某个 live workspace history 中仍保留保存点 `S`，且 `S.restored_from=R`，则 direct source `R` 默认受 `provenance_protected` 保护。
- 如果 `S.restored_paths` 包含 `path -> R`，则 direct source `R` 默认受 `provenance_protected` 保护。
- 如果 `S.started_from=R`，则 direct source `R` 默认受 `provenance_protected` 保护。
- `cleanup preview` 可以把 `R` 显示为 provenance-protected，并解释“kept because a live save point was created from it”或“kept because a live workspace started from it”。
- `cleanup run` 必须按同样规则保护 `R`，除非未来有新的显式 retention policy 改变该规则。
- 对 non-live 且未 keep 的保存点，cleanup 可以删除不受保护的 direct sources；但如果该 non-live 保存点因其他 policy 仍被保留，所有被删除的 direct source 必须保留 tombstone/ID/display reason，供详情页和 map 展示。
- 默认策略只保护 direct 来源；如果未来要保护传递闭包，必须在 cleanup policy 中显式升级并更新 preview/run 输出。
- 如果未来允许删除这类来源保存点，必须保留 tombstone/ID，并让 `history`、view、restore 和 map 能显示“source save point deleted: R”。不能让 provenance 静默悬空。

cleanup run 后的行为：

- 对仍然存在且受保护的 save point，`history`、`view`、`restore` 行为不变。
- 对已删除的 save point，`view <id>` 和 `restore <id>` 必须失败，错误类型为 `deleted-save`，并显示 tombstone/audit 信息：cleanup plan ID、删除时间、删除原因和 display reason。
- active workspace 的 history chain 在 V1 默认不被 cleanup 删除，因此未显式 `--include-active-history` 的普通 `history` 不应出现断链。
- 显式 active-history cleanup 删除旧保存点后，`history` 必须保留 tombstone 或 compacted deleted marker，显示 save ID、删除时间、cleanup plan 和 reason；不得把断开的历史伪装成从未存在。
- removed workspace 的已删除保存点可以在 `history --all --deleted` 或 audit 视图中显示 tombstone。

active-history cleanup 的 restore/view 边界：

- 删除的旧保存点不能再 view 或 restore；必须返回 `deleted-save`。
- 如果用户想回到 cleanup 前的旧内容，只有仍存在的 save point 可以作为 restore source。
- cleanup 不改变当前 workspace 文件、不移动 newest save point、不改变 `content_source` 或 `path_sources`。
- 实际释放空间可能小于 logical size；run 输出必须展示 estimated vs actual reclaimed bytes，以及仍 protected/shared 的大对象。
- protected-but-large 必须显示原因和下一步建议：例如 remove `keep` 后重新 preview，或该保存点仍是 current/latest/source，不能删除。
- labelled-but-unkept 必须高亮提醒：label 不保护；如果用户想保留，先 `keep <save>`。

human output 必须解释保留和删除原因。用户应能回答：

- 这个保存点为什么保留？
- 这个保存点为什么会被删除？
- 这次是否真的删除了东西？
- 预计释放多少空间？
- 哪些大文件仍受保护？
- 运行的是哪个 plan？
- `label` 是否影响 cleanup？答案必须是：否。
- `keep` 是否影响 cleanup？答案必须是：是。

## UX Copy Requirements

公开输出和文档必须不断强化：真实路径、只读查看、dirty safety、history not changed、`label` 不保护、`keep` 保护。

### `init`

```text
Folder: /repo/main
Workspace: main
JVS is ready for this folder.
Files were not moved or copied.
Newest save point: none
Not saved yet.
Unsaved changes: yes
Next: jvs save -m "baseline"
```

### `status`

```text
Folder: /repo/main
Workspace: main
Newest save point: B
Files match save point: B
Unsaved changes: no
```

如果普通保存后继续编辑：

```text
Folder: /repo/main
Workspace: main
Newest save point: B
Files changed since save point: B
Unsaved changes: yes
Next save creates a new save point after B.
```

如果先从另一个保存点恢复后再编辑：

```text
Folder: /repo/main
Workspace: main
Newest save point: B
Files were last restored from: A
Unsaved changes: yes
History was not changed.
Next save creates a new save point after B.
```

如果是新 workspace：

```text
Folder: /repo/workspaces/exp
Workspace: exp
Newest save point: none
Started from save point: A
Unsaved changes: no
```

### `save`

普通保存：

```text
Folder: /repo/main
Workspace: main
Saved save point C.
Created after save point B.
```

从恢复内容保存：

```text
Folder: /repo/main
Workspace: main
Saved save point C.
Created after save point B.
Created from restored save point A.
```

从 single-path restore 保存：

```text
Folder: /repo/main
Workspace: main
Saved save point C.
Created after save point B.
Includes restored path src/config.json from save point A.
```

从 `workspace new --from A` 后第一次保存：

```text
Folder: /repo/workspaces/exp
Workspace: exp
Saved first save point E for this workspace.
Workspace started from save point A.
```

不要把新保存点描述成重写历史、替换历史或把历史移回去。

### `history`

默认 workspace 输出：

```text
Folder: /repo/main
Workspace: main
Newest save point: C
History:
  A  "baseline"
  B  "before experiment"
  C  "restored config"
```

如果当前文件来自另一个保存点：

```text
Folder: /repo/main
Workspace: main
Workspace files were last restored from A.
Newest save point is B.
History was not changed.
```

全局输出：

```text
All save points in this JVS project
```

`history --all` 不应表现为 workspace 切换界面。

按 path 找候选：

```text
Folder: /repo/main
Workspace: main
Candidates for path: src/config.json
  A17c92  "baseline"  labels=baseline
  B81de0  "tuned config"  labels=run
Choose a save point ID, then run:
  jvs view A17c92 src/config.json
  jvs restore A17c92 --path src/config.json
```

### `view <save> [path]`

成功：

```text
Opened read-only view of save point A.
View: V42
View path: /tmp/jvs/views/V42
No workspace or history changed.
```

失败：

```text
Cannot open a guaranteed read-only view of save point A.
No changes were made.
```

### `restore <save>`

preview / 高摩擦确认：

```text
Preview only. No files were changed.
Folder: /repo/main
Workspace: main
Plan: R-restore-A-001
Source save point: A
Managed files to overwrite: 18
Managed files to delete: 3
Ignored/unmanaged files will be kept.
History will not change.
Newest save point is still B.
You can return to save point B after this restore.
Run with this plan only if this still looks right.
Run: jvs restore --run R-restore-A-001
```

成功：

```text
Folder: /repo/main
Workspace: main
Restored save point A.
Managed files now match save point A.
Only managed files were replaced or deleted.
Ignored/unmanaged files were kept.
Newest save point is still B.
History was not changed.
Next save creates a new save point after B.
```

dirty guard：

```text
Refusing to overwrite unsaved changes.
Folder: /repo/main
Workspace: main
Save first with --save-first, discard them with --discard-unsaved, or cancel.
```

### `restore <save> --path <path>`

未提供 save ID：

```text
Folder: /repo/main
Workspace: main
No save point ID was provided.
Candidates for path: src/config.json
  A17c92  "baseline"
  B81de0  "tuned config"
Choose a save point ID, then run:
  jvs restore A17c92 --path src/config.json
No files were changed.
```

```text
Folder: /repo/main
Workspace: main
Restored path src/config.json from save point A.
Only this path was replaced.
Newest save point is still B.
History was not changed.
```

### `workspace new <name> --from <save>`

```text
Folder: /repo/workspaces/experiment
Workspace: experiment
Started from save point A.
Newest save point: none
Original workspace unchanged.
```

必须展示真实路径，并且不得说新 workspace 继承了 `A` 的 history。

### `restore <save> --to <workspace> --save`

preview：

```text
Preview only. No files were changed.
Target folder: /repo/workspaces/release
Target workspace: release
Plan: R-copy-A-to-release-001
Source save point: A
This will replace target managed files so they match A.
Ignored/unmanaged files will be kept.
Rollback point: B
Expected newest save point: B
Expected folder evidence: ws-2b7c44
Run: jvs restore --run R-copy-A-to-release-001
```

成功：

```text
Folder: /repo/workspaces/release
Workspace: release
Copied save point A into this folder.
Managed files now match save point A.
Only managed files were replaced or deleted.
Ignored/unmanaged files were kept.
Saved as C in release.
Created after release's previous newest save point B.
Created from copied save point A.
Source workspace unchanged.
```

dirty guard 必须指出 target workspace 和 folder。

### `workspace remove`

preview：

```text
Preview only. No files were deleted.
Folder: /repo/workspaces/experiment
Workspace: experiment
Managed files: 12, size 84 MB
Ignored files: 1, size 1.1 GB
Unmanaged files: 1, size 100 MB
Unsaved managed changes: no
This will delete the real workspace folder, not its save points.
Unkept save points from this workspace may become cleanup candidates later.
```

成功：

```text
Deleted workspace folder: /repo/workspaces/experiment
Deleted workspace metadata: experiment
Save points were not deleted.
Unkept save points from this workspace may become cleanup candidates later.
```

### `cleanup preview`

preview 输出必须说：

```text
Preview only. No save points were deleted.
Plan: P
Candidates: 4 save points, logical size 18.2 GB
Estimated unique space to free: 17.9 GB
Shared/protected bytes: 0.3 GB
```

保留原因：

```text
Kept A: protected by keep.
Kept P0: needed to explain kept save point A.
Kept B: required by workspace main (/repo/main).
Kept R: needed because a live save point was created from it.
Kept V: active read-only view V42 uses it.
```

删除候选：

```text
Would delete D: removed workspace history and no keep protection. Size: 7.1 GB.
Would keep tombstone for R: retained save point E still displays source R.
```

label 警告：

```text
Labels do not protect save points from cleanup. Use keep to protect a save point.
Labelled but unkept: E label=best size=4.3 GB.
```

protected-but-large：

```text
Protected but large:
  A  12.4 GB  protected by keep
  B   9.8 GB  required by workspace main (/repo/main)
```

active workspace old save points cleanup：

```text
Preview only. No save points were deleted.
Folder: /repo/main
Workspace: main
Cleaning old save points in this folder.
Newest save point C will be kept.
Current files/source save points will be kept.
Would delete A: old save point in this folder, no keep protection. Size: 6.4 GB.
Would keep tombstone for A after deletion.
Labelled but unkept: B label=best size=4.3 GB.
Labels do not protect save points from cleanup. Use keep to protect a save point.
```

如果同一场景中用户执行了 `keep C`，`B` 必须保留。默认输出应说：`Kept B: needed to explain kept save point C.` 此时只有 `A` 或其他未受直接保护的旧保存点可以成为 active cleanup candidate。

### `cleanup run`

run 输出必须说明使用了哪个 plan，以及是否重校验通过：

```text
Running cleanup plan P.
Revalidated plan P.
Deleted D: removed workspace history and no keep protection. Freed: 7.1 GB.
Kept A: protected by keep.
Kept R: needed because a live save point was created from it.
Total freed: 17.9 GB.
```

如果重校验失败：

```text
Cleanup plan P no longer matches the JVS project state.
No save points were deleted. Run cleanup preview again.
```

之后 `view` 或 `restore` 已删除的保存点：

```text
Error: deleted-save
Save point D was deleted by cleanup.
Cleanup plan: P
Deleted at: 2026-04-27T10:15:00-07:00
Delete reason: removed workspace history and no keep protection.
Display reason: removed workspace history and no keep protection.
No workspace or history changed.
```

## MVP / V1 / Future

### MVP

MVP 聚焦普通文件夹上的清晰主路径，必须闭环：

- `jvs init [folder]` 接入已有普通文件夹。
- 默认输出优先 `Folder: /real/path`，再显示 workspace 名称。
- `main` 作为默认真实 workspace。
- `status`
- `save`
- `save -m <message>` 基础 message。
- `save` staging-before-publish conformance：capture 到 unpublished staging，capture 前和 capture 后/发布前校验 workspace 内容证据，校验失败不产生公开 save point、不移动 workspace 状态。
- `save` 防爆盘策略：save 前或 save 输出中显示新增/变更大文件 top N、估算 logical size 和 storage impact；超过 configured threshold 时默认要求确认，或建议 ignore/unmanage 后重试；写 unpublished staging 前必须通过可用空间和 safety margin 硬容量门，失败时 pre-staging refuse，用户确认不能绕过。
- 基础 ignore/unmanage 用户入口或 pre-save guidance，用户必须能在 MVP 中排除大生成物，不能等 cleanup 才处理。
- `history`
- `view <save>`
- `view <save> [path]`
- `restore <save>` whole-workspace restore。
- `restore <save> --path <path>` single-path restore。
- `restore` 和 `restore --path` 的 dirty guard。
- `restore <save>` 和 `restore <save> --path <path>` 必须有最小 recovery 闭环：`recovery status` / `resume` / `rollback`；即使底层实现事务化，失败验收也必须覆盖 recovery/status 路径。
- `restore` / `restore --path` / view materialization 写入前必须通过 capacity gate，失败时 pre-write refuse。
- `restore` / `restore --path` destructive run 必须绑定 preview plan 或 expected target state，防止 stale preview 覆盖新保存点。
- managed vs ignored/unmanaged files 基础规则。
- JVS control data 永不进入保存点。
- `<save>` 不解析 label 的 resolver 规则，即使 MVP 尚未完整支持 label。
- `history --path <path>` 或等价主发现路径必须可用；本文选择 `history --path`。
- `view` no-mutation conformance test。
- `save` staging-before-publish conformance test。
- `restore` dirty guard conformance test。
- `restore` history-not-changed conformance test。
- single-path restore conformance test。
- save-after-restore 线性追加 conformance test。
- 普通文件夹 golden scenario：
  `init -> save -> history -> view -> restore --path -> save`

MVP 如果不包含 multi-workspace，必须明确：完整 agent 实验闭环在 V1。但普通文件夹的 save/history/view/single-path restore/full restore 必须可用且闭环。

### V1

V1 仍以本文为总纲，但实现队列拆成阶段，避免一次性塞入所有复杂度。

V1a：multi-workspace 和 materialization safety

- workspace path topology enforcement：canonical path、禁止 nested/overlap、symlink containment tests。
- `workspace new <name> --from <save>`，且新 workspace `history_head=null`。
- workspace list/path/remove，且 remove 有 preview/confirm、dirty guard、unmanaged protection、expected target state 和 recovery plan。
- `restore <save> --to <workspace> --save`。
- 所有 materialization operations 的 source pin 和 capacity gate。
- expected target state / plan binding conformance tests。
- AI agent golden scenario：
  `save A -> agent bad edits -> restore A --discard-unsaved --preview -> restore --run <plan> -> workspace new exp --from A -> agent success -> save E -> restore E --to main --save --preview -> restore --run <plan>`

V1b：Data/ML metadata 和发现路径

- 完整 `label` 管理。
- 完整 `keep` 管理。
- `save --note` 和 `save --meta key=value`。
- creation-time metrics 与 later editable annotations 分离，并有 audit/correction 语义。
- numeric metadata 按数字语义排序/过滤。
- `history --path <path>`、label/message/time/metadata candidate discovery。
- map 视图区分 `parent`、`restored_from`、`restored_paths` 和 `started_from`。

V1c：cleanup 和 active history cleanup

- `cleanup preview`。
- `cleanup run`。
- cleanup size impact 输出。
- 带 label 但未 keep 的 cleanup 提醒。
- protected-but-large 输出。
- active view / active source pin / active operation / recovery plan cleanup protection。
- 默认不清 active workspace history chain。
- 显式 `cleanup preview --workspace <name> --include-active-history` 可清理 active workspace 中符合条件的旧保存点。
- tombstone/history/view/restore `deleted-save` 语义。
- cleanup plan revalidation 的 conformance tests。
- `restore --to --save` 失败回滚/恢复计划 conformance test。
- `workspace remove` 失败恢复计划 conformance test。

### Future

Future 可以包括：

- richer active workspace retention policies beyond explicit V1 `--include-active-history`
- 更丰富的 tombstone browsing/audit UX
- 更丰富的 retention policy
- remote synchronization
- signed save point descriptors
- trust policy
- 更丰富的 Save Point Map / Workspace Map 交互
- 更丰富的 diff 视图
- target workspace single-path restore，例如 `restore A --path p --to exp --save`
- 更强的 read-only view backend

Future 仍不得合并 `parent`、`restored_from`、`restored_paths` 与 `started_from`，也不得让 `label` 承担 cleanup protection。

## Non-Goals

本文产品模型不追求：

- Git parity
- text conflict resolution
- 自动改写已保存历史
- 以隐藏虚拟工作区作为主用户模型
- 把 repository root 当作可保存 workspace
- label-based cleanup protection
- 对每个保存点承诺永久归档
- `restore` 时隐式删除保存点
- `restore` 时隐式移动 history
- 在 V1 默认清理 active workspace 的旧 history chain

## Golden Scenarios

### Golden Scenario 1: Ordinary Folder Start To Single-File Restore

初始：用户已有普通文件夹 `/work/report`，里面有真实文件。

```text
cd /work/report
jvs init
jvs save -m "baseline"
jvs history
jvs view A notes.md
rm notes.md
jvs restore --path notes.md
jvs restore A --path notes.md
jvs save -m "restore notes"
```

`jvs init` 后：

```text
Folder: /work/report
Workspace: main
JVS is ready for this folder.
Files were not moved or copied.
Newest save point: none
Not saved yet.
Unsaved changes: yes
```

`save A` 后：

```text
Folder: /work/report
Workspace: main
Newest save point: A
Files match save point: A
Unsaved changes: no
```

`view A notes.md` 后：

```text
Opened read-only view of save point A.
View path: /tmp/jvs/views/V42/notes.md
No workspace or history changed.
```

`restore --path notes.md` 后：

```text
Folder: /work/report
Workspace: main
No save point ID was provided.
Candidates for path: notes.md
  A  "baseline"
Choose a save point ID, then run:
  jvs restore A --path notes.md
No files were changed.
```

`restore A --path notes.md` 后：

```text
Folder: /work/report
Workspace: main
Restored path notes.md from save point A.
Only this path was replaced.
Newest save point is still A.
History was not changed.
```

后续 `save B` 的默认输出：

```text
Folder: /work/report
Workspace: main
Saved save point B.
Created after save point A.
Includes restored path notes.md from save point A.
```

### Golden Scenario 2: Restore Then Save

初始状态：`main` 是真实 workspace，路径为 `/repo/main`。

```text
save A
save B
jvs restore A --preview
jvs restore --run R-restore-A-001
edit
save C
```

`jvs restore A --preview` 后：

```text
Preview only. No files were changed.
Folder: /repo/main
Workspace: main
Plan: R-restore-A-001
Source save point: A
Newest save point is still B.
Expected newest save point: B
Expected folder evidence: ws-clean-B-19aa
Run: jvs restore --run R-restore-A-001
```

`jvs restore --run R-restore-A-001` 后，默认用户可见状态：

```text
Folder: /repo/main
Workspace: main
Newest save point: B
Files match save point: A
Unsaved changes: no
History was not changed.
```

编辑后：

```text
Folder: /repo/main
Workspace: main
Newest save point: B
Files were last restored from: A
Unsaved changes: yes
```

`save C` 后：

```text
Folder: /repo/main
Workspace: main
Newest save point: C
Files match save point: C
Unsaved changes: no
C was saved after B.
C was created from restored save point A.
```

高级/JSON/测试断言：

```text
C.parent = B
C.restored_from = A
```

`history` 对 `main` 默认展示的线性顺序是：

```text
A -> B -> C
```

高级 map 可以额外展示 provenance：

```text
C restored_from A
```

### Golden Scenario 3: Read-Only View Lifecycle

```text
status
view A
status
jvs cleanup preview
jvs view close V42
```

两次 `status` 在以下方面必须一致：

- 最新保存点
- 文件来源
- path-level 来源
- 未保存更改
- workspace registration
- labels
- keep state
- cleanup plans

active view 存在时，`cleanup preview` 必须保护 `A`：

```text
Kept A: active read-only view V42 uses it.
```

从 view path 运行 `jvs save` 必须失败：

```text
This path is a read-only view of save point A, not a workspace.
No files or history were changed.
```

### Golden Scenario 4: AI Agent Experiment

初始：`main` 在 `/repo/main`，已保存 `A`。

```text
save A
agent edits badly
agent process stopped / no active writer
jvs restore A --discard-unsaved --preview
jvs restore --run R-discard-bad-edits-001
workspace new exp --from A
agent edits successfully in exp
agent process stopped / no active writer
save E -m "agent success"
jvs restore E --to main --save --preview
jvs restore --run R-copy-E-to-main-001
```

bad edits 后恢复必须先 preview：

```text
Preview only. No files were changed.
Folder: /repo/main
Workspace: main
Plan: R-discard-bad-edits-001
Source save point: A
This will discard unsaved managed changes and restore A.
Expected newest save point: A
Expected folder evidence: ws-bad-edits-7a91
Run: jvs restore --run R-discard-bad-edits-001
```

run 后：

```text
Folder: /repo/main
Workspace: main
Restored save point A.
Managed files now match save point A.
Only managed files were replaced or deleted.
Ignored/unmanaged files were kept.
History was not changed.
```

`workspace new exp --from A`：

```text
Folder: /repo/workspaces/exp
Workspace: exp
Started from save point A
Newest save point: none
```

agent 成功后在 `exp` 保存 `E` 的默认输出：

```text
Folder: /repo/workspaces/exp
Workspace: exp
Saved first save point E for this workspace.
Workspace started from save point A.
```

把 `E` 复制保存到 `main`：

```text
Preview only. No files were changed.
Target folder: /repo/main
Target workspace: main
Plan: R-copy-E-to-main-001
Source save point: E
This will replace target managed files so they match E.
Rollback point: A
Expected newest save point: A
Expected folder evidence: ws-clean-A-421c
Run: jvs restore --run R-copy-E-to-main-001
```

```text
Folder: /repo/main
Workspace: main
Copied save point E into this folder.
Managed files now match save point E.
Only managed files were replaced or deleted.
Ignored/unmanaged files were kept.
Saved as M in main.
Created after main's previous newest save point A.
Created from copied save point E.
Source workspace unchanged.
```

高级断言：

```text
M.parent = A
M.restored_from = E
```

### Golden Scenario 5: Data / ML Best Result

```text
jvs save -m "baseline" --label baseline
run training
jvs save -m "train run 42" --meta acc=0.937 --meta loss=0.21 --meta seed=1234 --label run
run training
jvs save -m "train run 57" --meta acc=0.940 --meta loss=0.19 --meta seed=5678 --label run
jvs history --label run --meta acc>=0.93 --sort acc:desc
jvs label add B81de0 best
jvs keep B81de0
jvs cleanup preview
```

期望：

```text
B81de0  "train run 57"  acc=0.940  loss=0.19  seed=5678  labels=run,best  keep=yes
A17c92  "train run 42"  acc=0.937  loss=0.21  seed=1234  labels=run
```

cleanup preview 必须包含：

```text
Preview only. No save points were deleted.
Candidates: ...
Estimated space to free: ...
Protected but large: ...
Labelled but unkept: ...
Labels do not protect save points from cleanup. Use keep to protect a save point.
```

`restore best` 必须失败并列出匹配保存点，要求选择具体 ID。

### Golden Scenario 6: Cleanup Run After Workspace Removal

初始：

```text
main history: A -> B -> C
exp history: E -> F
keep B
workspace remove exp
cleanup preview
cleanup run P
```

期望：

- `workspace remove exp` 删除真实 `/repo/workspaces/exp`，不删除 `E` 或 `F`。
- `cleanup preview` 可以把 unkept `E`、`F` 列为 removed workspace candidates。
- `cleanup run P` 删除重校验后仍不受保护的 `E`、`F`。
- active `main` 的 `A -> B -> C` history 在 V1 默认不被 cleanup 删除。
- `view B` 和 `restore B` 仍可用，因为 `B` 被 keep。
- `view E` 和 `restore E` 都失败，错误类型为 `deleted-save`，并显示 tombstone/audit 信息。

失败示例：

```text
Error: deleted-save
Save point E was deleted by cleanup.
Cleanup plan: P
Deleted at: 2026-04-27T10:15:00-07:00
Delete reason: removed workspace history and no keep protection.
Display reason: removed workspace history and no keep protection.
No workspace or history changed.
```

### Golden Scenario 7: Label Versus Keep

初始：

```text
jvs label add A release
jvs label add B release
jvs keep A
jvs cleanup preview
```

期望：

```text
A is protected because of keep.
B is not protected merely because it has label release.
cleanup preview may list deletion candidates but deletes nothing.
cleanup run requires the preview plan and revalidation.
```

### Golden Scenario 8: Clean Old Save Points In Active Folder

初始：

```text
main history: A -> B -> C
current files match C
jvs label add B best
jvs keep C
jvs cleanup preview --workspace main --include-active-history
jvs cleanup run P
```

期望：

- preview 明确显示 `Folder: /repo/main`，并说这是 clean old save points in this folder。
- `C` 是 newest/current，必须保留。
- `C` 被 keep，所以 `B` 必须保留，默认输出说：`Kept B: needed to explain kept save point C.`
- `B` 有 label 但未 keep，preview 仍必须提醒 label 不保护；它这次不是 candidate，因为它被 `keep C` 级联保护。
- `A` 或另一条不被直接保护的旧保存点，只有在未 keep、非 active source、非 direct protected provenance、非 active operation/recovery/view source 时才可删除。
- run 必须绑定 plan `P` 并重校验；如果 preview 后 `main` 新保存了 `D`，run 必须失败且不删除。
- 删除后 `history` 显示 tombstone/deleted marker；`view A` / `restore A` 返回 `deleted-save`。
- cleanup 不改变 `/repo/main` 当前文件，不移动 newest save point，不改变 `content_source` 或 `path_sources`。

## Acceptance Checklist

### Product Semantics

- [ ] JVS 被描述为真实文件夹的保存点系统。
- [ ] 用户主路径是 `init` / `save` / `history` / `view` / `restore`。
- [ ] 首次使用支持已有普通文件夹接入。
- [ ] `jvs init [folder]` 明确不移动、不复制用户文件，下一步仍在真实文件夹工作。
- [ ] 默认输出优先 `Folder: /real/path`，再显示 `Workspace: <name>`。
- [ ] `workspace` 始终是真实文件夹。
- [ ] `main` 是默认真实 workspace。
- [ ] `repo root` / project container 永远不被默认当作 workspace。
- [ ] workspace path 必须 canonicalized，禁止 nested/overlapping workspace，symlink/alias 不能绕过 containment。
- [ ] 公开主词是 `save point` / 保存点。
- [ ] 保存点内容和创建事实不可改写。
- [ ] 不把 immutable 描述成永久保留。
- [ ] cleanup 可以删除不受保护的保存点。
- [ ] 默认 save point 捕获所有 managed files。
- [ ] `save` 先捕获到 unpublished staging，发布前校验一致后才公开保存点并移动 workspace 状态。
- [ ] `save` 校验并发写入失败时，不产生公开 save point、不移动 workspace 状态，并提示停止写入后重试。
- [ ] `save` 输出或预检包含大文件 top N、logical size、storage impact 和 ignore/unmanage guidance。
- [ ] `save` 写 unpublished staging 前执行硬容量门；超过可用空间扣除 safety margin 后的安全预算时，pre-staging fail，不产生 partial staging 或 save point，用户确认 threshold 不能绕过。
- [ ] view/restore/restore --path/workspace new/restore --to --save/recovery 等所有 materialization 操作写入前都有 capacity gate，失败时不写 partial data、不改变 workspace state。
- [ ] JVS control data 永不捕获。
- [ ] ignored/unmanaged files 默认不被保存点捕获，也不应被 restore 删除。
- [ ] MVP 提供基础 ignore/unmanage 入口或 pre-save guidance，用户能排除大生成物。
- [ ] 默认公开状态展示真实 folder、最新保存点、文件来源、未保存更改。
- [ ] 文件已编辑时，不用 `Files copied from` 暗示仍等同 source。
- [ ] `history_head`、`content_source`、`started_from`、`path_sources`、`parent`、`restored_from`、`restored_paths` 只作为高级/内部/JSON/map/test 字段。
- [ ] `restore <save>` 是 whole-workspace replacement，只复制内容。
- [ ] `restore <save>` 默认 preview 或高摩擦确认；preview 展示真实 Folder、覆盖/删除 managed files 数量/样例、ignored/unmanaged 保留、history 不变、可回到哪个 save point。
- [ ] `restore <save>` 不删除保存点、不重排保存点、不移动 history。
- [ ] `restore <save>` 会让 target managed files 完整匹配 source save point。
- [ ] `restore <save> --path <path>` 只替换指定路径。
- [ ] `restore --path <path>` 未提供 `<save>` 时只列候选保存点并要求用户选择具体 ID，不猜测、不修改文件。
- [ ] `history --path <path>` 是不知道 save ID 时的主发现路径，支持按 path/时间/message/label/metadata 找候选。
- [ ] single-path restore 的 dirty guard 只检查/保护相关路径。
- [ ] `path_sources` 规范化 workspace-relative path，处理父子路径覆盖/合并，并在编辑/删除/移动后清除或降级。
- [ ] single-path restore 后 history 不变，后续 save 线性追加并记录 `restored_paths`。
- [ ] restore 后第一次 `save` 创建新的保存点，追加在 restore 前的最新保存点后面。
- [ ] restore 后第一次 `save` 在高级 metadata 中记录 `restored_from=<save>` 或 `restored_paths`。
- [ ] `restored_from` 是 provenance，不是线性历史边。
- [ ] `view <save> [path]` 只读，不改变 workspace、history、metadata 或 workspace registration。
- [ ] view 是临时 session/path，cleanup 保护 active view source save point。
- [ ] 从 view path 运行 JVS 写命令必须失败或要求明确选择真实 workspace。
- [ ] 所有读取/物化 source save point 的操作都注册 active source pin；cleanup 不得删除 active source。
- [ ] `workspace new <name> --from <save>` 创建新的真实 workspace，并保持原 workspace 不变。
- [ ] `workspace new <name> --from <save>` 的新 workspace `history_head=null`。
- [ ] 新 workspace 第一次 save 的 `parent=null`，并记录 `started_from=<save>`。
- [ ] `history` 输出说 workspace started from R，而不是继承 R 的历史线。
- [ ] cleanup 保护 active workspace 的 direct `started_from` source。
- [ ] 不支持无 `--save` 的 `restore <save> --to <workspace>`。
- [ ] `restore <save> --to <workspace> --save` 在目标 workspace 创建新保存点，不偷改 source history。
- [ ] `restore <save> --to <workspace> --save` 明确是 whole-workspace replacement，会覆盖/删除不一致的 managed files。
- [ ] `restore <save> --to <workspace> --save` 读取 source 时注册 active operation/protection，cleanup 不得删除 source。
- [ ] `restore <save> --to <workspace> --save` 要么事务化全部成功，要么恢复 target workspace 操作前状态；非事务化 fallback 必须创建 safety save 或 recovery plan。
- [ ] dirty guard 前必须重新扫描真实文件系统，不能只信 watcher/metadata。
- [ ] save/restore/workspace remove 等操作必须持有 workspace operation lock。
- [ ] 并发写入检测失败时，操作失败并提示重试/停止写入。
- [ ] restore/restore --path/restore --to --save/workspace remove 等 destructive run 绑定 preview plan 或 `--expect-head`/expected content evidence；target 在 preview 后变化时拒绝。
- [ ] `workspace remove` 是高危操作，默认 preview/confirm。
- [ ] `workspace remove` 不能删除 main。
- [ ] `workspace remove` 必须重算 unsaved/unmanaged。
- [ ] `workspace remove` 需要用户明确选择保存、丢弃或取消。
- [ ] `workspace remove` 遇到 ignored/unmanaged files 默认拒绝删除真实文件夹和 metadata，除非显式 `--delete-unmanaged`。
- [ ] `workspace remove` 成功时删除整个真实 workspace folder 和 metadata；失败不得留下 unregistered folder 或 orphan metadata。
- [ ] `workspace remove` 不删除保存点，但提示 unkept save points 未来可能成为 cleanup candidates。
- [ ] restore/remove 失败必须输出 recovery plan ID 或闭环恢复/重试语义。
- [ ] `history` 默认当前 workspace。
- [ ] `history --all` 是全局保存点列表。
- [ ] 高级 map 区分 `parent`、`restored_from`、`restored_paths` 和 `started_from`。
- [ ] `label` 只用于分类、过滤、展示。
- [ ] `<save>` 永远不解析 label。
- [ ] 用户输入 label 到 restore/view 时，必须报错并显示匹配保存点列表，要求选择具体 save point ID。
- [ ] label 查询返回候选列表和下一步命令；label 仍不是 ref。
- [ ] `label` 不提供 cleanup protection。
- [ ] `keep` 提供 cleanup protection，且与 `label` 分离。
- [ ] save message/note/key-value metadata 是展示信息，不是 ref。
- [ ] creation-time captured metadata/metrics 与 later editable annotations 分离；创建事实不可静默改写。
- [ ] 数值 metadata 按数值语义排序/过滤；后续可编辑 metadata 通过 audit/correction annotation 表达，不改写保存点创建事实。
- [ ] best result 工作流必须选择具体 save ID，并对重要结果使用 `keep`。
- [ ] `cleanup preview` 是默认行为，且不删除。
- [ ] `cleanup preview/run` 输出 size impact。
- [ ] `cleanup run` 必须绑定 plan。
- [ ] `cleanup run` 删除前必须重校验。
- [ ] cleanup 保护 kept save points、live workspace 所需保存点、active views、active operations 和 recovery plans。
- [ ] cleanup 保护 kept save points 的 direct explanatory sources：`parent`、`restored_from`、`started_from`、`restored_paths` sources。
- [ ] cleanup 把 live history 中 direct `restored_from`、`restored_paths`、`started_from` 引用的来源保存点列为 protected；未来若允许删除，必须保留 tombstone/ID。
- [ ] cleanup 对已删除但仍被 retained save 解释所需的 source 保留 tombstone/ID/display reason。
- [ ] V1 默认不清 active workspace history chain；显式 `--include-active-history` 才能清理 active folder 的旧保存点。
- [ ] active history cleanup 只能删除未 keep、非 newest、非 active content/path/started source、非 active operation/recovery/view source、非 direct protected provenance 的旧保存点。
- [ ] active history cleanup 输出 tombstone/history/view/restore `deleted-save` 语义、space reclaim、protected-but-large 和 labelled-but-unkept 提醒。
- [ ] cleanup run 后，对已删除保存点的 view/restore 必须给出 `deleted-save` tombstone/audit 错误，包含 plan ID、time 和 reason。
- [ ] MVP / V1 / Future 阶段边界清晰。
- [ ] MVP 普通文件夹 save/history/view/single-path restore/full restore 必须闭环。
- [ ] MVP 明确选择最小 recovery status/resume/rollback 闭环；full restore/path restore 不能只依赖事务化假设。
- [ ] MVP 若不包含 multi-workspace，明确 agent 完整实验闭环在 V1。
- [ ] V1 拆分为 V1a/V1b/V1c 或等价阶段，本文仍作为实现总纲。

### Forbidden Public Mental Models And Clean Redesign Gates

- [ ] 新公开文案以 `save point` 作为主词。
- [ ] Primary Public Terms 只围绕真实 folder、workspace、save point、save、history、view、restore、label、keep、cleanup。
- [ ] 默认人类输出不使用 `history_head`、`parent`、`restored_from`、`started_from`、`C.parent = B` 这类模型词。
- [ ] `checkpoint` 只作为旧草案/历史实现名出现，并被 `save point` 替换。
- [ ] 不保留旧命令/旧术语作为用户兼容层。
- [ ] 不为了兼容旧设计增加用户心智负担。
- [ ] 公开 docs、help text、UI 不使用禁止心智词，除非是在明确的 clean redesign notes 或禁止心智说明中。
- [ ] 公开 docs 不把 `main` 称为 `branch` 或 `link`。
- [ ] 公开 docs 不把 Save Point Map / Workspace Map 称为 Git `branch` graph。
- [ ] 公开 docs 不把 `restored_from` 描述成 `branch` 或 `merge` edge。
- [ ] 当前实现和旧文档必须向本文收敛，而不是维持新旧两套公开心智。
- [ ] 命令/API 可做 breaking change，以低心智、干净、一致的模型为优先。

### Implementation And Team Gates

- [ ] Worker 修改行为前先有 failing test 或明确 verification path。
- [ ] Review 必须检查是否是结构性根因修正，而不是 workaround。
- [ ] 自动化测试覆盖 ordinary folder init/adopt。
- [ ] 自动化测试覆盖 init 后不移动/复制用户文件。
- [ ] 自动化测试覆盖 workspace canonical path、symlink containment 和 nested/overlap 拒绝。
- [ ] 自动化测试覆盖 JVS control data 不进入保存点。
- [ ] 自动化测试覆盖 `history --path` / label / metadata 查询只返回候选，不直接 restore。
- [ ] 自动化测试覆盖 restore history-not-changed 行为。
- [ ] 自动化测试覆盖 whole-workspace restore 默认 preview/强确认输出真实 Folder、覆盖/删除数量、ignored/unmanaged 保留、rollback point。
- [ ] 自动化测试覆盖 golden restore/save provenance 场景。
- [ ] 自动化测试覆盖 single-path restore 只替换指定路径。
- [ ] 自动化测试覆盖 single-path restore dirty guard 只保护相关路径。
- [ ] 自动化测试覆盖 `path_sources` normalization、父子 path override、编辑/删除/移动后的清除或降级。
- [ ] 自动化测试覆盖 MVP 的 `view` no-mutation、restore dirty guard、restore history-not-changed、save-after-restore 线性追加。
- [ ] 自动化测试覆盖 `view <save> [path]` lifecycle 和 active view cleanup protection。
- [ ] 自动化测试覆盖所有 source materialization 操作注册 active source pin，cleanup 不删除 source。
- [ ] 自动化测试覆盖从 view path 运行写命令失败。
- [ ] 自动化测试覆盖 `workspace new --from` 不继承 source history，first save `parent=null` 且记录 `started_from`。
- [ ] 自动化测试覆盖 `restore`、`restore --path`、`restore --to --save`、`workspace remove` 的 dirty guard。
- [ ] 自动化测试覆盖 managed/ignored/unmanaged restore/remove 保护。
- [ ] 自动化测试覆盖并发写入检测和 workspace operation lock。
- [ ] 自动化测试覆盖 `save` pre-staging hard capacity gate：容量预算不足时不进入 capture、不写 partial staging、不创建 save point，用户确认 threshold 不能绕过。
- [ ] 自动化测试覆盖 view/restore/path restore/workspace new/restore --to --save/recovery capacity gate：预算不足时 pre-write fail 且 workspace state 不变。
- [ ] 自动化测试覆盖 expected target state / plan binding：preview 后 newest save point 或扫描证据变化时 run 拒绝。
- [ ] 自动化测试覆盖 `restore --to --save` 复制成功但保存失败时的事务化回滚或 recovery plan。
- [ ] 自动化测试覆盖 `workspace remove` preview/confirm、不能删除 main、失败 recovery plan。
- [ ] 自动化测试覆盖 `history` 默认 workspace 和 `history --all`。
- [ ] 自动化测试覆盖 label 不是 restore/view target，且不提供 cleanup protection。
- [ ] 自动化测试覆盖 `keep` cleanup protection。
- [ ] 自动化测试覆盖 `cleanup preview` 不删除。
- [ ] 自动化测试覆盖 `cleanup preview/run` size impact。
- [ ] 自动化测试覆盖 `cleanup run` plan binding 和 revalidation。
- [ ] 自动化测试覆盖 `restored_from`、`restored_paths`、`started_from` provenance protection。
- [ ] 自动化测试覆盖 V1 默认不 prune active workspace history chain；显式 `--include-active-history` 可以删除符合条件的旧 save point 并保留 tombstone。
- [ ] 自动化测试覆盖 cleanup run 后 deleted save point 的 view/restore tombstone/error。
- [ ] 自动化测试覆盖 creation-time metrics 不可静默改写、correction annotation/audit、numeric metadata 数值排序/过滤。
- [ ] human-readable output 测试覆盖 `Folder: /real/path` 优先、只读文案、history not changed、label not protecting、keep protecting。
- [ ] 如有 JSON output，必须暴露不同字段：`history_head`、`content_source`、`started_from`、`path_sources`、`unsaved_changes`、`parent`、`restored_from`、`restored_paths`。
- [ ] 涉及 UX/UI 变动时必须截图验收。
- [ ] 修改范围保持收敛，不修改或 revert 无关文件。
- [ ] Team subagents 使用 `xhigh` reasoning effort。
- [ ] Worker 默认 `gpt-5.5` + `xhigh`，除非任务确实非常简单。

## Clean Redesign Notes

本节不是兼容迁移计划，也不承诺保留旧命令或旧术语。当前产品尚未发布，因此这是 implementation reset / clean redesign 的落地说明：旧草案和历史实现名需要被新产品语义替换，公开 UX、docs、help text、CLI/API 应收敛到本文模型。

设计原则：

- 不保留旧命令/旧术语作为用户兼容层。
- 不为了兼容旧设计增加用户心智负担。
- 当前实现可以 breaking change。
- 旧草案和历史实现名只用于识别需要替换的对象。
- 新公开示例只教学 `init`、`save`、`history`、`view`、`restore`、`workspace new`、`label`、`keep`、`cleanup`。

| Old draft / historical name | Replacement in clean redesign |
|-----------------------------|-------------------------------|
| `checkpoint` | `save point`；旧草案/历史实现名，需要替换。 |
| `jvs checkpoint` | `jvs save`；不要作为用户兼容 alias 保留。 |
| `commit` | `save`。 |
| `checkout` | 只读查看用 `view`；复制内容到 workspace 用 `restore`。 |
| `branch` | 多个真实 `workspace` 文件夹；不要用该词作为公开解释。 |
| `fork` | `workspace new <name> --from <save>`。 |
| `promote` | `restore <save> --to <workspace> --save`。 |
| `current` | 拆成 `content_source`、`path_sources` 和 `unsaved changes`；不要与 `history_head` 混为一谈。 |
| `latest` | 替换为用户语言“最新保存点”，内部可对应 workspace `history_head`；避免暗示全局最新。 |
| `tag` as recoverable alias | 不是 `label`；label 不解析为 save target。如未来需要用户可恢复别名，应另行设计新概念。 |
| `gc plan` | `cleanup preview`。 |
| `gc run` | `cleanup run`。 |

实现重置建议：

- CLI/help/docs 直接替换到新命令和新术语，不提供旧命令作为用户承诺。
- 测试和示例以新产品语义为准；旧测试若表达旧心智，应改写或删除。
- 内部存储字段若仍使用历史名称，应视为待重构 implementation detail；公开 JSON、UX、docs 必须表达新模型。
- 若内部旧字段会迫使公开 UX 解释旧心智，应优先重构字段或增加清晰的新投影层。
- 所有实现偏差都应登记为“待收敛到本文语义”，而不是登记为兼容行为。
