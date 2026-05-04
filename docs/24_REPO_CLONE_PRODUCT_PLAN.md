# Repo Clone Product Plan

**Subtitle:** local JVS project clone handoff.

**Status:** implemented design record for current repo clone behavior; active clean redesign, non-release-facing, not part of the v0 public contract. The release-facing source of truth is `docs/02_CLI_SPEC.md` and `docs/user/`; later clone refinements stay in this record until promoted.

**中文名:** 本地 JVS project clone。

本文面向产品、工程和 QA，记录 `jvs repo clone` 的产品口径和验收边界。它只讨论文档和行为合同，不包含实现代码；若与 release-facing docs 冲突，以 release-facing docs 为准。

## 文档验收标准

这份 handoff 应满足：

- 产品能直接解释：`jvs repo clone` 复制本地或已挂载的 JVS project，不是 Git clone，不做 remote、push、pull 或 origin。
- 工程能直接拆阶段：先交付 `--save-points main` 的安全闭包，再交付 `--save-points all` 和 durable imported-save-point cleanup protection，或在同一里程碑一次交付完整默认体验。
- QA 能直接抽矩阵：覆盖只创建目标 `main` workspace、dirty source 拒绝、runtime state 排除、transfer planning、capacity fallback、atomic publish、cleanup imported protection、remote 误用拒绝。
- 文档明确：默认复制所有 save points 不等于复制所有 workspaces。无论复制多少历史，目标 repo 永远只创建 `main` workspace。
- 文档避免 branch/Git 心智，使用 JVS 的 folder、workspace、save point、history、restore、cleanup 和 doctor 语言。

## 一句话目标

`jvs repo clone <target-folder>` 从当前 workspace/repo discovery 或全局 `--repo` 找到源 JVS project，在显式目标路径创建一个新的本地 JVS project，生成新的 `repo_id`，只创建目标 `main` workspace，并按用户选择复制源 repo 的 save point 历史。

## 背景

用户看到 “clone repo” 时，直觉上会期待新 project 能保留历史，打开后可以继续 `status`、`history`、`view`、`restore` 和 `save`。但 JVS 的 workspace 是真实文件夹，源 repo 里可能有多个 workspace，甚至有外部 workspace path 和 locator。直接把这些 workspace registry entries、locators 或真实 folders 都复制到目标，会把源机器上的路径和运行时状态带进一个新 repo，既难理解，也容易不安全。

本功能的产品判断是：repo clone 复制的是本地 JVS project 的 durable history 和目标 main folder 的当前内容，不复制源 repo 的 workspace 拓扑。

## 非目标

本里程碑不做：

- 不做 Git clone，不接受 URL、`git@host`、`ssh://` 或 scp-like remote source。
- 不做 remote、push、pull、origin、tracking 或跨机器协议。
- 不设计 `jvs repo clone <source> <target>` 双位置主命令；source 来自当前 discovery 或全局 `--repo`。
- 不把 save point 当作 branch，也不把 `--save-points main` 解释成 branch。
- 不复制源 repo 的非 main workspaces、workspace registry entries、locators 或真实 folders。
- 不复制 locks、intents、restore/recovery plans、cleanup plans、active pins、views、tmp/staging、open view state 等 runtime state。
- 不创建到已存在目标路径；默认目标 folder 必须不存在。
- 不依赖 `jvs doctor --strict --repair-runtime` 来把 clone 后的目标修成可用状态。
- 不新增自动 cleanup 或 retention policy；cleanup 仍是 preview/run 的 reviewed deletion flow。

## 用户心智

推荐用户心智：

- 我站在一个 JVS project 里，或者用 `--repo` 指定一个本地 JVS project。
- 我明确给出新 project 的目标 folder。
- JVS 创建一个新的 repo identity，并把目标 folder 作为唯一 workspace：`main`。
- JVS 会复制 save points，让新 project 能看历史、开 view、restore 和继续 save。
- 即使复制了所有 save points，也不会复制源 repo 里的其他 workspaces。
- 如果某个源 workspace 有未保存修改，这些修改还不是 save point；clone 默认失败，提示先保存。
- 如果目标位置和源存储不能使用快路径，JVS 用普通复制，安全性不变，只是可能更慢。

最重要的普通话术：

```text
Workspaces created: main only
Source workspaces not created: experiment, review
```

当源 repo 只有 `main` workspace 时：

```text
Workspaces created: main only
Source workspaces not created: none
```

## 推荐 CLI

推荐命令：

```bash
jvs repo clone <target-folder> [--save-points all|main] [--dry-run]
```

可与现有全局 flags 配合：

```bash
jvs --repo /abs/source repo clone /abs/target --save-points all --json
```

规则：

- `<target-folder>` 必须显式提供。
- `<target-folder>` 默认必须不存在；不自动采用当前目录，不自动生成后缀，不写入已存在空目录。
- 相对 `<target-folder>` 按调用者 `cwd` 解析。
- source 不作为位置参数出现；source 由当前 workspace/repo discovery 或全局 `--repo <path>` 得到。
- `--repo` 是源 repo 断言，不是目标路径基准。
- `--save-points all` 是完整产品默认目标，因为用户自然会以为 clone repo 会保留历史。
- `--save-points all` 只有在 durable imported-save-point cleanup protection 已实现并通过验收后才能作为可发布默认。
- 如果工程分期，Phase 1 可以先只交付 `--save-points main`；Phase 2 再交付 `all` 和 cleanup imported protection。
- 也可以同一里程碑一次交付 `all` 和 protection。不能发布一个会被 cleanup 静默删掉导入历史的 `all` clone。
- `--dry-run` 只做 discovery、校验、closure/import 计划、transfer 计划和容量估算，不创建目标 repo。

## Save Point Scope 语义

### `--save-points all`

`all` 表示复制源 repo 中所有 durable save point descriptors 和 payload storage，让目标 repo 能保留源 repo 的完整 save point 历史。

关键边界：

- `all` 复制 save points，不复制所有 workspaces。
- 目标 repo 仍然只创建 `main` workspace。
- 源 repo 的非 main workspace registry entries 不进入目标。
- 源 repo 的外部 workspace locators 和真实 folders 不进入目标。
- 目标必须记录 durable imported-save-point provenance/protection，否则 `all` 不能发布。

`all` 复制来的 save points 可能有一部分不在目标 `main` workspace 当前历史路径上。它们仍然是用户期望保留的 clone 历史，cleanup 必须能解释并保护它们。

### `--save-points main`

`main` 表示复制 source `main` workspace 当前状态和历史闭包。它不是 `worktree_name == main` 的简单过滤，不是 branch，也不是只复制某个字段等于 `main` 的 save points。

闭包至少从 source `main` 的这些指针和来源出发：

- `Head`
- `Latest`
- `Base`
- `StartedFrom`
- `PathSources`

然后递归补齐所有 provenance 引用，至少包括：

- `ParentID`
- `StartedFrom`
- `RestoredFrom`
- `RestoredPaths`
- 其他能影响 `doctor`、`status`、`history`、`view`、`restore`、`save` 的 durable save point 引用

验收线：目标 repo 的 `doctor --strict`、`status`、`history`、`view`、`restore` 和后续 `save` 不能出现 dangling references。若某个 provenance 引用缺失、损坏或无法复制，clone 必须失败，不发布目标。

## 目标 Workspace 规则

无论 `--save-points all` 还是 `--save-points main`，目标 repo 永远只创建 `main` workspace。

目标规则：

- 目标生成新的 `repo_id`。
- `source_repo_id` 只写入 clone manifest/audit provenance，不能成为目标 repo identity。
- 目标只重建 `.jvs/worktrees/main/config.json`。
- 目标 `main` 的 `RealPath` 指向 `<target-folder>` 的 canonical path。
- 目标 registry 只承认 `main`。
- 目标不复制源 workspace paths、locators、runtime state 或 registry entries。
- 目标 main folder materialization 表示 source `main` workspace 的当前可保存内容。
- 源 repo 不变；源 workspaces、源 registry、源 locators 和源 save point storage 都不被修改。

成功输出必须包含：

```text
Workspaces created: main only
Source workspaces not created: <names or none>
```

如果源 repo 有 `main`、`experiment`、`review` 三个 workspace，`--save-points all` 后仍应输出：

```text
Save points copied: all
Workspaces created: main only
Source workspaces not created: experiment, review
```

这条输出是产品安全带：它直接告诉用户“默认复制所有 save points 不等于复制所有 workspaces”。

## Repo Identity And Clone Manifest

目标 repo 必须是新的 JVS project：

- `.jvs/repo_id` 使用新生成值。
- Completed clone JSON 输出包含 `source_repo_id` 和 `target_repo_id`。Dry-run
  JSON 是计划结果，不创建真实 target repo，因此不要求实际 `target_repo_id`。
- clone manifest/audit 记录 source provenance，但目标运行时 discovery 只认 target repo。
- 目标 locator 或 workspace config 不得指向 source repo。

建议 clone manifest 至少记录：

| 字段 | 含义 |
| --- | --- |
| `operation` | `repo_clone` |
| `created_at` | clone 创建时间 |
| `source_repo_root` | 源 repo display path |
| `source_repo_id` | 源 repo identity，仅用于 provenance |
| `target_repo_root` | 目标 repo path |
| `target_repo_id` | 新 repo identity；只在 completed clone manifest 中存在 |
| `save_points_mode` | `all` 或 `main` |
| `save_points_copied` | 数量和 ID 列表或可审计摘要 |
| `imported_save_points_count` | canonical imported save point ID 列表数量 |
| `imported_save_points_checksum` | canonical imported save point ID 列表 checksum，用于发现 manifest 截断或列表损坏 |
| `workspaces_created` | 固定为 `["main"]` |
| `source_workspaces_not_created` | 除 source main 外的 workspace 名称 |
| `runtime_state_copied` | 固定为 `false` |
| `transfers` | 与 CLI JSON `data.transfers[]` 对齐的摘要 |

如果工程选择保留源 save point IDs，manifest 应说明这些 IDs 在目标 repo 内作为 imported save point IDs 使用。若工程选择 remap，所有 descriptor、workspace config、provenance、history 和 manifest 引用必须一次性一致 remap。无论哪种方案，用户可见行为必须满足无 dangling references。

## Runtime State 排除

clone 不复制 runtime state。目标完成后应当是一个干净、可直接使用的 repo，而不是一个需要 runtime repair 的半成品。

必须排除：

- locks
- intents
- restore plans
- recovery plans
- cleanup plans
- active pins
- open views 和 view state
- tmp/staging 目录
- 进行中的 materialization state
- 任何只代表源 repo 当前操作中的 ephemeral marker

验收线：

- 目标完成后 `jvs doctor --strict` 通过。
- `jvs doctor --strict --repair-runtime` 不应成为正常 clone 成功路径的一部分。
- cleanup、recovery、view、restore 的源 repo 运行时状态不会出现在目标。

## Unsaved Changes Gate

源 repo 不变，并且 clone 只复制 save point 与 source main 的可保存当前内容。为了避免用户误以为未保存修改已经被复制，默认必须拒绝以下情况：

- source `main` workspace 有 unsaved changes。
- 任意可访问 source workspace 有 unsaved changes。

失败必须发生在创建目标 repo 之前。错误文案要说明原因和行动：

```text
Cannot clone: source workspace "experiment" has unsaved changes.
Save those changes as a save point first if you want them included.
JVS repo clone only creates target workspace "main"; source workspaces are not created in the clone.
```

如果某个 source workspace 不可访问或健康检查无法确认其状态，默认也不应静默跳过。至少要失败并提示先修复源 repo 或移除无效 registry entry；否则 clone 的历史承诺无法被 QA 验证。

## Cleanup And Retention 要求

`--save-points all` 的发布前置条件是 durable imported-save-point protection。

原因很简单：`all` 模式复制来的 save points 中，可能有一批不被目标 `main` workspace 当前 history/provenance 直接引用。如果 cleanup 只按目标 `main` 的 workspace history 保护，它可能把这些 imported save points 当作可回收对象删掉。用户会看到 “clone 保留了历史”，但之后一次 cleanup 静默削掉历史。这种版本不能发布。

要求：

- imported save point protection 必须是 durable metadata 或 manifest 驱动，不是 runtime active pin。
- cleanup preview 必须把 imported save points 归入稳定保护 reason，例如 `imported_clone_history` 或最终确认的同义 token。
- human cleanup output 应能自然解释为 “imported clone history” 或中文等价说法。
- cleanup run 必须 revalidate imported protection，不得只信 preview 时的旧结果。
- doctor strict 必须能发现 manifest/protection 缺失、损坏或引用不存在。
- all-clone 发布前必须先原子写入 manifest；普通 repo 没有 manifest 是合法状态，因此“manifest 缺失”只能通过 clone 编排在发布前失败来保证，已存在的 manifest 则由 doctor strict/cleanup fail-closed 校验。
- cleanup 仍只删除 unprotected save point storage；不引入自动 retention policy。
- 若用户未来需要丢弃 imported history，应另行设计 reviewed command；不在本里程碑隐式提供。

`--save-points main` 不需要保护源 repo 所有 save points；它只复制目标 main 当前状态需要的闭包。这个闭包应被目标 main workspace history/provenance 自然保护。
cleanup、doctor 和 verify 必须把 `PathSources`、`RestoredFrom`、`RestoredPaths` 等 provenance 引用视为 durable save point references，而不只看 Parent/StartedFrom。

## Filesystem-Aware Transfer Planning

`repo clone` 必须接入 filesystem-aware transfer planning，使用与 Smart Copy Boundaries 一致的输出和 JSON 模型。

至少有两类 transfer：

| Transfer | Source role | Destination role | 说明 |
| --- | --- | --- | --- |
| Save point storage copy | `save_point_storage` | `target_save_point_storage` | 复制 `all` 或 `main` 闭包需要的 durable descriptors/payloads |
| Main workspace materialization | `source_main_current_state` 或 `save_point_payload` | `target_main_workspace` | 把目标 `main` folder materialize 成 source main 当前内容 |

规则：

- JSON 使用 `data.transfers[]`。
- 每条 transfer 的 `operation` 建议为 `repo_clone`。
- Human output 使用 `Copy method: fast copy` 或 `Copy method: normal copy`。
- 如果两个 transfer 的 copy method 不同，human output 应分开展示，不要用一个总结果掩盖差异。
- `--dry-run` 可显示 expected copy method，但必须使用 expected/dry-run 语气。
- run 阶段必须重新探测，不能信任 dry-run 结果。
- capacity 按普通复制 fallback 的最坏情况估算，覆盖 save point storage copy、main workspace materialization 和目标 control metadata。
- 如果 fast copy 候选需要 fallback，但 fallback 容量不足，必须在写入前失败。

Human output 示例：

```text
Save point storage: Copy method: fast copy
Main workspace: Copy method: normal copy
Why: these two locations cannot use fast copy together
Checked for this operation
```

JSON transfer 摘要示例：

```json
{
  "data": {
    "transfers": [
      {
        "transfer_id": "repo-clone-save-points",
        "operation": "repo_clone",
        "phase": "save_point_storage_copy",
        "primary": true,
        "result_kind": "final",
        "permission_scope": "execution",
        "source_role": "save_point_storage",
        "destination_role": "target_save_point_storage",
        "checked_for_this_operation": true,
        "performance_class": "fast_copy",
        "optimized_transfer": true,
        "degraded_reasons": [],
        "warnings": []
      },
      {
        "transfer_id": "repo-clone-main-workspace",
        "operation": "repo_clone",
        "phase": "main_workspace_materialization",
        "primary": true,
        "result_kind": "final",
        "permission_scope": "execution",
        "source_role": "source_main_current_state",
        "destination_role": "target_main_workspace",
        "checked_for_this_operation": true,
        "performance_class": "normal_copy",
        "optimized_transfer": false,
        "degraded_reasons": [
          "these two locations cannot use fast copy together"
        ],
        "warnings": []
      }
    ]
  }
}
```

## Atomicity And Retry

clone 必须按 atomic publish 设计。

要求：

- 在目标 parent 下创建 sibling staging，例如 hidden staging folder。
- 所有 control metadata、save point storage、main workspace materialization、manifest/audit 都先写入 staging。
- staging 内完成 health validation 和 `doctor --strict` 等价校验。
- 发布时使用 no-replace rename 到最终 `<target-folder>`。
- 如果最终目标在发布前被别人创建，clone 失败，不覆盖。
- `<target-folder>` 必须位于 source project/workspaces 外部，否则无法保证 source unchanged。
- 失败时 source unchanged。
- 失败时 target 不得成为 active JVS repo。
- 如果 rollback 不能安全移除已发布的 target folder 或 target control data root，
  target folder or target control data root may remain at the target path or be
  moved to a hidden quarantine；in either case, inspect/remove manually，避免误删
  晚到外部写入。
- 只有 move to quarantine 成功后，才提示
  `target folder was quarantined at ...; inspect and remove it manually` 或
  `target control root was quarantined at ...; inspect and remove it manually`。
- preexisting empty target dir 应恢复为空目录。
- staging 或 quarantine cleanup 失败也不能让 final target 看起来像成功 repo；
  下次重试应能清楚处理遗留 staging/quarantine。

错误恢复心智：

```text
Clone failed. Source was not changed. Target is not an active JVS repo.
If rollback could not safely remove the target folder or target control data
root, the target folder or target control data root may remain at the target
path or be moved to a hidden quarantine; in either case, inspect/remove
manually.
If a target path was moved to quarantine:
target folder was quarantined at ...; inspect and remove it manually.
target control root was quarantined at ...; inspect and remove it manually.
```

## Safety And Error 文案

### Remote-like 输入拒绝

`jvs repo clone` 只复制本地或已挂载的 JVS project。以下输入必须拒绝：

- `https://example.com/repo`
- `ssh://host/path`
- `git@host:org/repo`
- `user@host:path`
- scp-like remote path

错误文案：

```text
JVS repo clone only copies a local or mounted JVS project.
Remote URLs and git-style sources are not supported.
Use a local path with --repo, then provide the target folder.
```

注意不要误伤 Windows 盘符。`C:\work\project` 或 `D:/work/project` 在 Windows 上是本地路径，不应因为包含冒号而被当作 scp-like remote。remote-like 检测应识别 `://`、`git@...:`、`user@host:path` 等形态，而不是简单拒绝所有带 `:` 的字符串。

### Target 已存在

```text
Cannot clone: target folder already exists.
Choose a new folder path. JVS will not merge into an existing folder.
```

### Source 不是 JVS repo

```text
Cannot clone: source is not a JVS project.
Run from inside a JVS workspace or pass --repo <local-jvs-project>.
```

### Cleanup protection 缺失

如果用户请求 `--save-points all`，但当前 build 没有 durable imported protection：

```text
Cannot clone with --save-points all yet.
This build cannot protect imported save point history from cleanup.
Use --save-points main, or upgrade to a build that supports imported history protection.
```

## Human Output 草案

成功：

```text
Cloned JVS project
Source: /abs/source
Target: /abs/target
Save points copied: all (42)
Workspaces created: main only
Source workspaces not created: experiment, review
Save point storage: Copy method: fast copy
Main workspace: Copy method: normal copy
Why: these two locations cannot use fast copy together
Checked for this operation
Doctor strict: passed
```

`--save-points main` 成功：

```text
Cloned JVS project
Source: /abs/source
Target: /abs/target
Save points copied: main history closure (8)
Workspaces created: main only
Source workspaces not created: experiment, review
Copy method: normal copy
Checked for this operation
Doctor strict: passed
```

Dry run：

```text
Repo clone dry run
Source: /abs/source
Target: /abs/target
Save points to copy: all (42)
Workspaces that would be created: main only
Source workspaces that would not be created: experiment, review
Expected save point storage copy: fast copy
Expected main workspace copy: normal copy
Estimated capacity needed if normal copy fallback is used: 12.4 GB
No files were created.
```

## JSON Output 草案

JSON 使用现有 envelope。Completed clone 成功 `data` 建议包含：

```json
{
  "schema_version": 1,
  "command": "repo clone",
  "ok": true,
  "repo_root": "/abs/target",
  "workspace": "main",
  "data": {
    "operation": "repo_clone",
    "source_repo_root": "/abs/source",
    "target_repo_root": "/abs/target",
    "source_repo_id": "src-...",
    "target_repo_id": "dst-...",
    "save_points_mode": "all",
    "save_points_copied_count": 42,
    "workspaces_created": [
      "main"
    ],
    "source_workspaces_not_created": [
      "experiment",
      "review"
    ],
    "runtime_state_copied": false,
    "clone_manifest": ".jvs/audit/repo-clone-manifest.json",
    "doctor_strict": "passed",
    "transfers": [
      {
        "transfer_id": "repo-clone-save-points",
        "operation": "repo_clone",
        "phase": "save_point_storage_copy",
        "performance_class": "fast_copy"
      },
      {
        "transfer_id": "repo-clone-main-workspace",
        "operation": "repo_clone",
        "phase": "main_workspace_materialization",
        "performance_class": "normal_copy"
      }
    ]
  },
  "error": null
}
```

Dry-run 使用同一 envelope 和 clone planning fields，但 `dry_run` 为 `true`，不创建
target folder/control data，也不生成真实 target repo identity。Dry-run JSON 可以
省略 `target_repo_id` 或将其置为 `null`；completed clone JSON 必须包含实际
`target_repo_id`。

失败 envelope：

```json
{
  "schema_version": 1,
  "command": "repo clone",
  "ok": false,
  "repo_root": "/abs/source",
  "workspace": "main",
  "data": null,
  "error": {
    "code": "E_UNSAVED_CHANGES",
    "message": "Cannot clone: source workspace \"experiment\" has unsaved changes.",
    "hint": "Save those changes as a save point first if you want them included. JVS repo clone only creates target workspace \"main\"."
  }
}
```

## 阶段实施计划

### Phase 0: Contract And Test Scaffolding

- 固定 CLI parse contract：`jvs repo clone <target-folder> [--save-points all|main] [--dry-run]`。
- 固定 JSON envelope、`data.transfers[]`、human output 的必有字段。
- 增加 fake transfer planner、fake capacity meter、fake dirty workspace detector、fake atomic publish failure 的测试入口。
- 明确 source discovery 与 `--repo` 规则。
- 明确 remote-like 输入拒绝规则，并覆盖 Windows drive path。

### Phase 1: Safe Main Closure Clone

- 交付 `--save-points main`。
- 实现 source dirty gate：source main 和任意可访问 source workspace dirty 时失败。
- 计算 main current-state/history closure，补齐 parent/provenance 引用。
- 创建新 target repo_id，只创建 target main workspace。
- 排除 runtime state。
- 接入 repo_clone transfer planning 和 fallback capacity gate。
- sibling staging + doctor strict + no-replace rename 发布。
- 验收目标 `status`、`history`、`view`、`restore`、`save` 和 `doctor --strict`。

### Phase 2: All Save Points With Imported Protection

- 交付 `--save-points all`。
- 复制所有 durable save point descriptors/payloads。
- 增加 durable imported-save-point manifest/protection。
- cleanup preview/run 展示并 revalidate imported protection group。
- doctor strict 校验 imported manifest/protection 一致性。
- 证明 cleanup 不会静默删除 imported history。
- 将完整产品默认推进到 `--save-points all`。

### Phase 3: Polish And Release Evidence

- 更新 CLI spec、conformance plan、release evidence 和 user docs。
- 加入真实大目录和跨设备 gated profile evidence。
- 补齐错误文案、dry-run 文案和 JSON snapshots。
- 明确从 Phase 1 到 Phase 2 的兼容说明。

## QA 验收矩阵

| 场景 | 命令/条件 | 期望 |
| --- | --- | --- |
| Basic main clone | `jvs repo clone target --save-points main` | 创建新 repo_id；目标只有 `main`；doctor strict 通过 |
| All save points clone | `jvs repo clone target --save-points all` | 所有 save points 复制；仍只创建 `main` workspace |
| All does not create workspaces | 源有 `experiment`、`review` | 输出 `Workspaces created: main only` 和 `Source workspaces not created: experiment, review`；目标 registry 无这些 entries |
| Main closure provenance | source main 有 parent、started-from、path restore 来源 | 目标无 dangling references；status/history/view/restore/save 正常 |
| Dirty main rejected | source main 有 unsaved changes | 命令失败；目标不存在；提示先 save |
| Dirty non-main rejected | source `experiment` 可访问且 dirty | 命令失败；提示该 workspace 未保存，且 clone 不会创建它 |
| Source unchanged | 成功和失败路径 | 源 repo_id、registry、save point storage、runtime state 不变 |
| New repo identity | 成功 clone | `source_repo_id != target_repo_id`；source id 只在 manifest/audit provenance |
| Target RealPath | 成功 clone | `.jvs/worktrees/main/config.json` 的 `RealPath` 指向 target folder |
| Source paths not copied | 源有外部 workspace path/locator | 目标不含源 workspace paths、locators、registry entries |
| Runtime state excluded | 源有 locks/plans/views/tmp | 目标不复制；doctor strict 通过；不需要 repair-runtime |
| Target exists | 目标 folder 已存在 | 失败，不 merge，不 overwrite |
| Remote URL rejected | target/source-like 输入为 `https://...` 或 `ssh://...` | 失败，说明只支持本地或已挂载 JVS project |
| scp-like rejected | 输入为 `git@host:org/repo` 或 `user@host:path` | 失败，说明不是 Git/remote clone |
| Windows drive safe | Windows 上目标为 `C:\work\clone` 或 `D:/work/clone` | 不因冒号误判为 remote |
| Dry run | `--dry-run` | 不创建目标；显示将复制的 save points、只创建 main、transfer 预期和 fallback capacity |
| Transfer JSON | `--json` 成功 | `data.transfers[]` 至少含 save point storage copy 和 main workspace materialization，`operation=repo_clone` |
| Human copy method | fast/normal/fallback fake 场景 | human 显示 `Copy method: fast copy` 或 `normal copy`，必要时显示 `Why` |
| Capacity fallback | fast candidate 但 normal fallback 空间不足 | 写入前失败；目标不发布 |
| Atomic publish failure | staging 后 final target 被创建，或 target folder publish 后 control publish 失败 | source unchanged；target 不是 active JVS repo；无法安全移除 target folder 或 target control data root 时，target folder or target control data root may remain at the target path or be moved to a hidden quarantine；in either case, inspect/remove manually；只有 moved to quarantine 后才提示 `target folder was quarantined at ...; inspect and remove it manually` 或 `target control root was quarantined at ...; inspect and remove it manually` |
| Imported protection | `--save-points all` 后 cleanup preview | imported save points 被 durable reason 保护 |
| Missing protection blocks all | build 未实现 imported protection | `--save-points all` 失败，提示用 `--save-points main` 或升级 |
| Cleanup revalidation | all clone 后 protection 变化 | cleanup run 失败并要求 fresh preview |
| Doctor imported manifest | manifest/protection 损坏 | doctor strict 失败并指出 clone imported history provenance 问题 |

## Release Readiness Checklist

- [ ] 默认产品目标是 `--save-points all`，且只有 durable imported-save-point protection 通过后才发布。
- [ ] `--save-points main` 明确定义为 source main current-state/history closure，不是 branch 或 `worktree_name == main` 过滤。
- [ ] 目标永远只创建 `main` workspace。
- [ ] 成功 human output 明确 `Workspaces created: main only`。
- [ ] 成功 human output 明确 `Source workspaces not created: ...`。
- [ ] 目标 repo_id 是新值；source_repo_id 只作为 manifest/audit provenance。
- [ ] 目标不复制源 workspace paths、locators、registry entries 或 runtime state。
- [ ] 源 main 或任意可访问源 workspace dirty 时默认失败且不创建目标。
- [ ] `repo_clone` 接入 `data.transfers[]`、human copy method 和 fallback capacity gate。
- [ ] sibling staging、doctor strict、no-replace rename 发布路径有失败注入测试。
- [ ] URL、ssh、scp-like remote 被拒绝；Windows drive path 不被误伤。
- [ ] QA 矩阵覆盖“默认复制所有 save points 不等于复制所有 workspaces”。
