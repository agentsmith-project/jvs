# External Control Root Product Handoff

**Subtitle:** control data location handoff for advanced operators and platform
integrations.

**Status:** implemented design record for the current external control root operator/platform profile; active clean redesign, non-release-facing, not part of the v0 public contract. The release-facing source of truth is `docs/02_CLI_SPEC.md` and `docs/user/`; later lifecycle parity and API work stay here until promoted.

**中文名:** 外部控制数据根。

本文面向产品、工程、安全和 QA，定义 JVS 如何在同一个 workspace
心智下支持两种 control data location：

- 默认：JVS control data 位于 workspace folder 内的 `.jvs/`。
- 高级/平台：调用方显式指定 external control root，JVS control data
  位于 workspace folder 外部的可信 control root。

这不是两种产品，也不是让用户在 default 和 external 两套 repo 产品之间切换。
JVS 的用户模型仍然是 folder、workspace、save point；差异只在 control data
存放位置、workspace binding 的权威来源、安全边界，以及备份/生命周期必须成对
维护的对象上。

底层实现可以继续使用 durable internal mode metadata。这类历史内部值是
implementation detail；只有在工程测试、兼容迁移或内部诊断必须表达底层状态时
才出现，不作为用户心智、标题、主入口或 public JSON 字段。

## 文档验收标准

这份 handoff 应满足：

- 产品能直接解释：JVS 默认把 control data 放在 workspace 的 `.jvs/`；
  平台/operator 可以显式把 control data 放在 external control root。
- 工程能直接拆阶段：先固定对象模型、显式定位、JSON/error 合同和
  fail-closed boundary invariant，再接 clone 和后续 lifecycle parity。
- QA 能直接抽矩阵：覆盖 root overlap、root occupancy、workspace folder
  root-level `.jvs` marker、clean CWD、symlink escape、save/restore/cleanup
  boundary、external clone target、doctor JSON checks 和默认 `.jvs/` 工作流兼容。
- 安全评审能直接验收：control writes 只能进入 control data boundary；
  workspace writes 只能进入 workspace folder；两者任何重叠、互为祖先/后代、
  symlink escape 或 path drift 都 fail closed。
- 文档明确当前最小合同，不把 CLI/API 入口留成“后续决定”而没有语义冻结。

## 一句话目标

JVS 支持 external control root：同一个 workspace 仍然是用户工作的真实 folder，
但高级/operator 调用方可以用 `--control-root <path> --workspace <name>` 把
JVS control data 放在该 workspace folder 之外，并让所有操作从 control root
验证 workspace binding 和安全边界。

## 产品判断

需求合理，且应作为 JVS 的通用平台集成能力，而不是 AgentSmith 或 AFSCP 的
特化能力。

AgentSmith/AFSCP 是首个集成场景：sandbox 需要把某个 workspace folder 挂载为
agent 的 `$HOME` 或 `/workspace`，WebDAV/export 也只应暴露用户文件。JVS
`.jvs/` 如果仍在 workspace folder 中，不可信 workload 可以 rename/unlink 顶层
控制目录项，filtered mount、chmod 或 sidecar 只能把正确性压力推到运行时层。

更好的产品边界是：JVS 原生承认 control data location 可以外置。任何需要暴露
workspace folder、但不暴露 JVS control data 的平台，都可以使用同一能力。

## 背景

默认本地模型仍然成立：

```text
workspace folder/
  .jvs/          # JVS control data
  user files     # workspace files
```

这个模型对本地个人开发简单直观，必须保留。普通用户的默认 `jvs init` 继续采用
workspace folder 内 `.jvs/`。

平台集成场景可以显式外置 control data：

```text
trusted platform path/
  control/       # external control root

workload-visible path/
  workspace/     # workspace folder exposed to user/workload
```

平台只把 `workspace/` 挂载或 export 给 workload。JVS runner、AFSCP controller
或其他可信自动化用显式 target 操作 `control/`。

推荐的 AFSCP 类布局可以是：

```text
/afscp/namespaces/<namespace_id>/repos/<repo_id>/
  control/
    <JVS control data and JVS-owned runtime state>
  workspace/
    <workspace files exposed to sandbox/WebDAV>
```

这只是一个集成布局示例，不是 JVS 的唯一目录结构承诺。

## 非目标

本里程碑不做：

- 不迁移已有默认 `.jvs/` workspace。
- 不改变普通本地用户默认 `jvs init` 心智。
- 不把 external control root 讲成第二套产品或要求用户理解底层 repo mode。
- 不实现 AFSCP、Kubernetes、JuiceFS CSI、WebDAV、sandbox manager 或平台 ACL。
- 不实现 filtered mount，也不把 filtered mount 当成当前主安全边界。
- 不把 workspace folder 下的 locator/shortcut 当作权威来源。
- 不引入 Git remote、branch、origin、push/pull 或网络同步心智。
- 不在当前里程碑做 moving control data into/out of workspace。
- 不在当前里程碑做 external lifecycle parity：repo move、repo rename、repo detach、
  workspace move、workspace rename、workspace delete 或 workspace new 对 external
  control root 继续 fail closed。
- 不在当前里程碑发布 safe locator/discovery。
- 不在当前里程碑发布稳定 library API，除非实现显式定位语义必须有最小内部 API。
- 不在当前里程碑发布 real platform gates，例如 AFSCP/JuiceFS/WebDAV 组合 gate。
- 不把 `--save-points all` clone 发布为 external control root 默认能力，除非
  docs/24 要求的 durable imported-save-point cleanup protection 已经满足。

## 对象模型和词汇

### Control Data Location

Control data location 是产品层对象，不是用户要选择的产品型号。

| Location | 含义 | 发现/定位入口 |
| --- | --- | --- |
| Default workspace control data | JVS control data 位于 workspace folder 下的 `.jvs/` | 从 workspace folder 向上找 `.jvs/` |
| External control root | JVS control data 位于 workspace folder 外部的可信 control root | 当前合同必须显式指定 control root + workspace |

默认位置保持现有用户体验。本文主要定义 external control root。

### Workspace Folder

`workspace folder` 是用户工作的真实 filesystem directory。它可以被 workload
挂载、被 WebDAV/export 暴露，或被用户工具读写。

在 default control data location 下，workspace folder 内的 `.jvs/` 是 JVS
control data。这个 `.jvs/` 是权威 control root，不是 locator。

在 external control root 下，workspace folder 不应包含 JVS control data。
当前 strict external profile 的 marker set 固定为 workspace folder 下 root-level
`.jvs` path；file、directory 或 symlink 都算 present。出现该 path 时，当前
conformance 必须以 `E_PAYLOAD_LOCATOR_PRESENT` fail closed；该 path 不能作为
JVS authority。

### External Control Root

`external control root` 是 JVS 可信控制数据根目录。它拥有或指向：

- repo identity，例如 `repo_id`。
- workspace registry 和 workspace binding。
- save point descriptors 和 JVS-owned save point storage。
- audit/provenance。
- runtime state、locks、restore/recovery plans、cleanup plans、tmp/staging，
  除非实现拆出独立 runtime root；即便拆出，也必须在同一可信 control boundary 内。

External control root 不等于 workspace folder。它不应被 sandbox、WebDAV 或
不可信 workload 暴露为可写 workspace files。

### Workspace Binding

workspace binding 是 external control root 中的权威记录，至少包含：

- workspace name，当前里程碑只要求 `main`。
- workspace folder 的 canonical path 和可用 physical identity evidence。
- 所属 `repo_id`。
- 所属 external control root。

JVS 操作必须从 explicit selector 找到 control root，再从 control root 校验
workspace binding，然后才读取或写入 workspace folder。裸 workspace folder
不能安全自动发现 external control root。

### Repo Identity

`repo_id` 是 repo 的稳定身份。路径不是身份。复制、repo clone 或 template-source
workflow 创建新 repo 时必须生成新 `repo_id`。source `repo_id` 只能作为
provenance 记录，不能成为 target repo identity。

### Locator/Shortcut

`locator` 或 `shortcut` 是方便 human discovery 的线索，不是权威状态。

Default workspace control data 下，folder 里的 `.jvs/` 是 control data 本身，
不是本文说的非权威 locator。

External control root 或未来高级模式可以有轻量 locator，帮助 human discovery
找回 control root、repo_id 和 workspace name。但在当前 external control root
合同中：

- platform-managed runner 不能依赖 workspace 可写 locator。
- locator 不能包含唯一控制状态。
- 当前默认 marker set 只包含 workspace folder 下 root-level `.jvs` path；
  file、directory 或 symlink 都算 present。
- 该 `.jvs` path 必须以 `E_PAYLOAD_LOCATOR_PRESENT` fail closed，不能读取、
  follow 或解释为 authority。
- 未来若要支持更多 marker 名称或形态，只能放进后续 gated compatibility
  profile，不属于当前默认 CI；即使 ignored，也必须由 doctor diagnostic checks
  证明没有读取其中状态。
- 未来兼容 profile 如果显式允许 locator evidence，locator mismatch、
  repo_id mismatch 或 workspace mismatch 必须 fail closed。
- public JSON 不发布 locator authority 判据；workspace marker 状态只通过
  doctor `checks[]` 中的 `workspace_control_marker` 表达。

### Internal Mode Metadata

实现可以用 durable internal mode metadata 区分 storage layout。内部实现和兼容
迁移可以继续识别历史 internal mode 值。

这些字段不是用户模型。产品文档、operator 入口和 user-facing examples 应说
external control root 或 control data location；只有 JSON schema、错误诊断、
内部兼容测试和工程设计需要时才提 internal mode metadata。

## 当前最小定位合同

当前 conformance CLI selector 固定为 `--control-root <path> --workspace <name>`。
调用方必须能在干净 CWD 下显式指定 external control root 和 workspace，稳定操作
目标 workspace，不受 workspace folder locator、另一个 repo 的 CWD 或 ambient
discovery 影响。

固定初始化入口：

```bash
jvs init <workspace-folder> --control-root <control-root> --workspace main --json
```

文档和 help 中可写成：

```bash
jvs init [folder] --control-root C --workspace main
```

固定运行时显式定位入口：

```bash
jvs --control-root <control-root> --workspace <name> status --json
jvs --control-root <control-root> --workspace <name> doctor --strict --json
jvs --control-root <control-root> --workspace <name> save -m "message" --json
```

未来可以新增 config、environment 或 library API 等等价入口，例如：

```bash
JVS_CONTROL_ROOT=<path> JVS_WORKSPACE=<name> jvs status --json
```

这些 future/API 入口不是当前 conformance selector。conformance suite 和 handoff
examples 必须使用固定 selector，不能保留含糊语义：

- `--control-root <path> --workspace <name>` 是当前 external control root
  conformance selector。
- 如果未来扩展 `--repo`，必须明确 `--repo <path>` 在 external control root
  工作流中只能指向 control root，并要求同时提供 `--workspace <name>` 或等价
  workspace selector；它不是当前 conformance selector。
- `--repo <workspace-folder>` 在 external control root 工作流中不能被猜成
  control root。
- 没有显式 target 且 CWD 不足以安全证明唯一 repo/workspace 时，必须失败，
  错误码为 `E_EXPLICIT_TARGET_REQUIRED`。

### Clean CWD 合同

当前命令必须能从任意干净 CWD 运行：

```bash
cd /home/runner
jvs --control-root /trusted/repo/control --workspace main status --json
```

期望：

- discovery 不读取当前 CWD 中另一个 `.jvs/`。
- discovery 不读取 external workspace folder 下的 `.jvs` 作为 authority。
- 输出中的 `control_root`、`folder`、`workspace`、`repo_id`
  来自显式 target 和 control metadata 校验。
- mutation 前重新校验 control/workspace boundary。

## JSON 输出合同

JSON 使用 docs/02 现有 envelope。普通命令成功响应的 public target fields 是
`data.folder`、`data.workspace` 和 `data.control_root`；其中 `folder` 是
workspace folder 的 canonical absolute display path，`workspace` 是显式选择并
经 control registry 校验的 workspace。命令可以继续输出自己的 public result
字段，例如 `repo_id`、`newest_save_point` 或 `unsaved_changes`。
`status --json` must not emit `data.repo` for external control roots；
external status 使用 `data.control_root` 表达 control data location。status
human output prints `Control data: <control-root>`，不能把 control root 标成
`Repo: <control-root>`。默认 `.jvs/` status 继续保留普通 repo/folder 心智。

external `repo clone --json` 是结果型 JSON 例外：source selector 不作为 target
`data.folder` 输出，也不把 source `data.control_root` 重用为 target control root。
clone target 只通过 `data.target_folder` 和 `data.target_control_root` 表达；
source control root/source workspace 只作为 source context 或 provenance 字段。

普通命令 public JSON 不发布 internal mode、compatibility summary 或 boundary
diagnostic fields。需要详细诊断时，调用
`jvs --control-root <path> --workspace <name> doctor --strict --json`，由 doctor
diagnostic result 的 `data.checks[]` 输出 failed checks 和稳定 `error_code`
映射。集成方不能解析 human output 或目录布局来判断边界。

普通错误响应必须 `ok:false`、`data:null`，`error` 只包含稳定 `code`、
`message` 和 optional `hint`；普通失败的 boundary/context 诊断不能放进
non-null `data`。

legacy/top-level `repo_root` 若保留，在 external control root 下必须等于
`data.control_root`，或明确标记为 display-only/source display path；它绝不能
表示 workspace folder，也不能成为 external control root 工作流的 authority。

### 成功示例

```json
{
  "schema_version": 1,
  "command": "init",
  "ok": true,
  "repo_root": "/trusted/repo/control",
  "workspace": "main",
  "data": {
    "repo_id": "repo-...",
    "folder": "/workload/repo/workspace",
    "workspace": "main",
    "control_root": "/trusted/repo/control",
    "format_version": 1,
    "newest_save_point": null,
    "unsaved_changes": true
  },
  "error": null
}
```

### Doctor 成功示例

```json
{
  "schema_version": 1,
  "command": "doctor",
  "ok": true,
  "repo_root": "/trusted/repo/control",
  "workspace": "main",
  "data": {
    "repo_id": "repo-...",
    "folder": "/workload/repo/workspace",
    "workspace": "main",
    "control_root": "/trusted/repo/control",
    "healthy": true,
    "findings": [],
    "checks": [
      {
        "name": "root_overlap",
        "status": "passed",
        "error_code": null,
        "message": "Control root and workspace folder are separate."
      },
      {
        "name": "workspace_control_marker",
        "status": "passed",
        "error_code": null,
        "message": "No control data marker is present in the workspace folder."
      },
      {
        "name": "repo_identity",
        "status": "passed",
        "error_code": null,
        "message": "Repo identity matches the control root."
      },
      {
        "name": "workspace_binding",
        "status": "passed",
        "error_code": null,
        "message": "Workspace selector matches the control registry."
      },
      {
        "name": "path_boundary",
        "status": "passed",
        "error_code": null,
        "message": "Canonical paths stay within declared boundaries."
      },
      {
        "name": "permissions",
        "status": "passed",
        "error_code": null,
        "message": "Required read/write/fsync permissions are available."
      },
      {
        "name": "active_operation",
        "status": "passed",
        "error_code": null,
        "message": "No active operation blocks this command."
      },
      {
        "name": "recovery_state",
        "status": "passed",
        "error_code": null,
        "message": "No recovery state blocks this command."
      }
    ]
  },
  "error": null
}
```

### 失败示例

```json
{
  "schema_version": 1,
  "command": "status",
  "ok": false,
  "repo_root": "/trusted/repo/control",
  "workspace": "main",
  "data": null,
  "error": {
    "code": "E_PAYLOAD_INSIDE_CONTROL",
    "message": "Cannot use external control root: workspace folder is inside control root.",
    "hint": "Run `jvs --control-root /trusted/repo/control --workspace main doctor --strict --json` to inspect failed checks, or create/clone with a workspace folder outside the control root."
  }
}
```

### JSON 字段要求

| 字段 | 要求 |
| --- | --- |
| `repo_id` | 必须来自 external control root 中的 repo identity |
| `folder` | workspace folder 的 canonical absolute display path |
| `workspace` | 必须是本次显式选择并经 control registry 校验的 workspace |
| `control_root` | canonical absolute display path |
| `healthy` | 只属于 `doctor --strict --json` diagnostic result；普通命令不需要该字段 |
| `checks[]` | 只属于 `doctor --strict --json` diagnostic result；普通错误响应必须 `data:null` |
| `error.code` | 使用稳定错误码，不要求集成方解析 message |
| `error.message` | 面向 human/operator 的短消息；自动化不能解析 message 判断类别 |
| `error.hint` | optional；仅在存在安全下一步或诊断入口时给出，没有安全自动化下一步时可省略 |

### Doctor JSON Check 合同

`doctor --strict --json` 的 `data.checks[]` 是合同 surface。每个 check 至少包含：

| 字段 | 要求 |
| --- | --- |
| `name` | 稳定 check name |
| `status` | `passed`、`failed`、`skipped` 或等价稳定枚举 |
| `error_code` | 失败时为稳定错误码；通过或 skipped 时为 `null` 或空字符串 |
| `message` | 面向 human/operator 的短消息；自动化不能解析 message 判断类别 |

当前 doctor strict 至少固定这些 check names：

| Check name | 覆盖 |
| --- | --- |
| `root_overlap` | control root/workspace folder same path、ancestor/descendant 或无法分类的 overlap |
| `workspace_control_marker` | workspace folder 的 root-level `.jvs` path；file、directory 或 symlink 都算 present |
| `repo_identity` | repo_id、clone manifest、stored metadata 一致性 |
| `workspace_binding` | workspace selector、registry entry、workspace evidence 一致性 |
| `path_boundary` | canonical path、symlink、hardlink、case-folding、path drift 和 boundary escape |
| `permissions` | control/workspace required read/write/fsync 权限 |
| `active_operation` | active lock、intent、cleanup plan、lifecycle operation |
| `recovery_state` | pending restore/recovery state、restore plan 和 recovery identity mismatch |

Doctor strict failure goldens must assert failed `checks[].name` and stable
`error_code` mappings at minimum:

| Failed check | Golden fixture | Stable `error_code` |
| --- | --- | --- |
| `root_overlap` | same root fixture | `E_CONTROL_PAYLOAD_OVERLAP` |
| `root_overlap` | workspace-inside-control fixture | `E_PAYLOAD_INSIDE_CONTROL` |
| `root_overlap` | control-inside-workspace fixture | `E_CONTROL_INSIDE_PAYLOAD` |
| `workspace_control_marker` | workspace root-level `.jvs` file, directory, or symlink | `E_PAYLOAD_LOCATOR_PRESENT` |
| `permissions` | required control or workspace read/write/fsync permission denied | `E_PERMISSION_DENIED` |
| `active_operation` | active lock、intent、cleanup plan 或 lifecycle operation blocks a new mutation | `E_ACTIVE_OPERATION_BLOCKING` |
| `recovery_state` | pending restore/recovery state blocks a new mutation | `E_RECOVERY_BLOCKING` |

## 错误码合同

下表 code 是当前 JSON golden 的稳定错误码。若实现前做最后一次命名统一，必须在
contract scaffolding 同步更新本文和测试；开发开始后不得让同一场景存在多个可接受
code。

| 错误码 | 场景 | 行为 |
| --- | --- | --- |
| `E_CONTROL_PAYLOAD_OVERLAP` | control root 与 workspace folder 是同一路径、同一物理 identity，或重叠关系无法进一步分类 | fail closed before mutation |
| `E_PAYLOAD_INSIDE_CONTROL` | workspace folder 位于 control root 内部 | fail closed before mutation |
| `E_CONTROL_INSIDE_PAYLOAD` | control root 位于 workspace folder 内部 | fail closed before mutation |
| `E_PATH_BOUNDARY_ESCAPE` | canonical path、symlink、hardlink 或平台 file identity 显示操作会逃出声明边界，或出现 path drift | fail closed before mutation |
| `E_CONTROL_MISSING` | 需要已有/source/control registry 的命令显式选择的 control root 不存在或不可达；例如 status、save、doctor 或 clone source | fail closed |
| `E_CONTROL_MALFORMED` | control root 存在但缺少必须 metadata、schema 损坏或无法证明 repo identity | fail closed |
| `E_PAYLOAD_MISSING` | 已登记/被选择 workspace 的 folder 不存在或不可达 | fail closed |
| `E_REPO_ID_MISMATCH` | 显式 target、locator、registry、clone manifest 或 stored metadata 的 repo_id 不一致 | fail closed |
| `E_WORKSPACE_MISMATCH` | workspace selector、registry entry、workspace evidence 或 locator 的 workspace name 不一致 | fail closed |
| `E_PERMISSION_DENIED` | control root 或 workspace folder 的 required read/write/fsync 权限不足 | fail closed，不做部分写入 |
| `E_EXPLICIT_TARGET_REQUIRED` | CWD/ambient discovery 不能安全证明 external control root target，且调用方未提供 control root + workspace | fail closed；通过 `error.hint` 或 human output 给出显式 selector 示例 |
| `E_PAYLOAD_LOCATOR_PRESENT` | 当前 strict external profile 下 workspace folder 出现 root-level `.jvs` path；file、directory 或 symlink 都算 present | fail closed；不能读取为 authority |
| `E_TARGET_ROOT_OCCUPIED` | init target control root 或 clone target roots 已存在且非空，或需要 merge/overwrite/adopt existing control data 才能继续；external init workspace folder 中的普通用户文件不属于该错误 | fail closed before mutation；missing target roots 由 JVS 创建，不使用 missing-root 错误码 |
| `E_SOURCE_DIRTY` | clone source workspace 有 unsaved changes、writer race，或无法证明 source clean | fail closed before target publish |
| `E_ATOMIC_PUBLISH_BLOCKED` | clone staging 无法 no-overwrite/atomic publish 到 final target，或 final target 会呈现半成品 repo | fail closed；source unchanged |
| `E_IMPORTED_HISTORY_PROTECTION_MISSING` | 请求 `--save-points all`，但 durable imported-save-point cleanup protection 未实现、未写入或 doctor strict 无法验证 | fail closed，并提示 `--save-points main` 或升级 |
| `E_SEPARATED_LIFECYCLE_UNSUPPORTED` | external control root 上调用当前未实现的 move/rename/delete/detach lifecycle 命令 | stable unsupported fail closed |
| `E_ACTIVE_OPERATION_BLOCKING` | control root 中已有 active lock、intent、cleanup plan 或 lifecycle operation 阻塞本次操作 | fail closed；如果存在安全下一步，用 `error.hint` 或 human output 展示 |
| `E_RECOVERY_BLOCKING` | restore/recovery state 或 restore plan 要求先 resume/rollback/diagnose，不能开始新的 mutation | fail closed；如果存在安全 recovery 下一步，用 `error.hint` 或 human output 展示 |

## 安全不变量

这些不变量是当前发布线，不是实现建议。

- 所有 root path 在使用前必须解析为 canonical absolute path，并记录平台可用的
  physical identity，例如 device/inode、file ID 或等价证据。
- `control_root == workspace_folder` 必须 fail closed。
- `workspace_folder` 位于 `control_root` 内部必须 fail closed。
- `control_root` 位于 `workspace_folder` 内部必须 fail closed。
- 如果 symlink、bind mount、hardlink、case-folding 或 path drift 让 JVS 无法可靠
  判断边界，必须 fail closed。
- 每次 mutation 前必须重新校验 root identity、workspace identity、repo_id、
  权限、active locks/recovery 和 boundary。
- JVS control writes 只能发生在 external control root 或受控 control runtime
  boundary 内。
- JVS workspace writes 只能发生在本次 workspace folder 内。
- save/status/history/view/restore/cleanup/recovery 不能把 control root、runtime、
  locks、plans、audit 或 locator 捕获为 workspace files。
- restore/recovery 不能覆盖 control root，不能通过 workspace symlink 写入
  control root。
- cleanup 不能删除 workspace folder、control root、runtime root、locks、plans 或
  audit root。cleanup 只能删除经过 reviewed flow 判定为 unprotected 的 JVS-owned
  save point objects。
- strict external profile 不在 workspace folder 创建 `.jvs`。
- workspace folder 中如存在 root-level `.jvs` path，无论它是 file、directory
  还是 symlink，它都不是 locator，不是 authority，不是 control root。strict
  external profile 必须以 `E_PAYLOAD_LOCATOR_PRESENT` fail closed；未来更多
  marker 名称/形态或 ignore 行为只能作为 gated compatibility profile，也必须由
  doctor diagnostic checks 证明没有读取其中状态。

## 操作边界合同

### Init

`init` 创建 external control root 工作流时必须：

- target control root 只接受 missing 或 empty directory；JVS 可以创建 missing
  control root，已存在 empty directory 可被初始化。
- workspace folder 可以 missing、empty，或已有 non-empty 用户文件；external init
  必须 adopt existing user files，不清空、不重建、不移动，并保留文件 metadata。
- workspace folder 下 root-level `.jvs` file、directory 或 symlink 仍必须以
  `E_PAYLOAD_LOCATOR_PRESENT` fail closed，因为它会混淆 control authority。
- non-empty control root、existing JVS control data、merge/overwrite existing repo
  control data 都不是当前交付面，必须以 `E_TARGET_ROOT_OCCUPIED` fail closed。
- 生成新 `repo_id`。
- 创建 `main` workspace registry entry，指向 workspace folder canonical path。
- 写入 durable internal mode metadata。
- 不在 workspace folder 创建 `.jvs`。
- 初始化后 public JSON 报告 `folder`、`workspace`、`control_root`，并且
  `doctor --strict --json` 通过。

### Explicit Targeting, Status, History

`status`、`history` 和 workspace metadata read 操作必须：

- 只从显式 control root 读取 authority。
- 用 workspace name 找到 workspace folder。
- 在读取 workspace folder 前校验 boundary。
- 即使 CWD 位于另一个 JVS repo 内，也不能改变 target。
- 如果 workspace folder 下存在恶意 `.jvs`，不能被它改写 repo_id、
  workspace name 或 control root。

### Save

`save` 只保存 workspace folder 中的 managed files。

必须排除：

- control root。
- control runtime、locks、plans、audit。
- workspace folder 的 root-level `.jvs` path，file、directory 或 symlink 都算
  present；strict external profile 下应先因 `E_PAYLOAD_LOCATOR_PRESENT` 失败。
- symlink escape 指向 workspace folder 外的文件。

mutation 前必须重新校验 control/workspace boundary。save descriptor、audit 和
JSON 不能把 control metadata 描述成用户 workspace files。

### View

`view` 只 materialize save point workspace files 或指定 path。view materialization
不能把 control root 作为 workspace 内容展示，也不能通过 symlink escape 读取
control root。

如果 view 需要临时目录或 cache，它必须属于 JVS-controlled boundary，并且不被当作
workspace files 保存。

### Restore

`restore preview` 和 `restore run` 都必须按 workspace boundary 计算影响。

要求：

- preview 不写 workspace files，不提交 recovery 状态变更。
- run 前重新校验 boundary，不能信任 preview 的旧 path/identity。
- restore 只能写 workspace folder 内的 managed target。
- workspace symlink 指向 control root 或 workspace folder 外部时必须 fail closed。
- restore/recovery backup 不能覆盖 control root。
- 如果 pending recovery 存在，新 mutation 必须被 `E_RECOVERY_BLOCKING` 阻塞。

### Recovery

recovery 只处理已记录的 workspace mutation 和 JVS-controlled recovery state。

要求：

- recovery plan、restore plan、locks 和 audit 位于 control boundary，不进入 save
  point workspace files。
- recovery resume/rollback 写 workspace folder 前重新校验 workspace folder
  identity。
- recovery 不能删除、覆盖或 materialize control root。
- 如果 recovery state 与 repo_id、workspace name 或 workspace folder identity
  mismatch，fail closed。

### Cleanup

cleanup 是 reviewed deletion flow，不是 root deletion flow。

要求：

- cleanup preview/run 只考虑 JVS-owned save point objects 和已定义的
  runtime-safe residue。
- cleanup 不删除 workspace folder、control root、runtime root、locks、plans 或
  audit root。
- cleanup 不因为 workspace folder 不可达就删除 workspace registry。
- cleanup run 重新校验 imported protection、active operation、repo_id 和 boundary。
- docs/24 的 imported save point protection 未满足时，`--save-points all` clone
  不得发布。

### Transfer Planning Inheritance

External control root 的 `save`、`view`、`restore` 和 `repo clone` 继承 docs/23 的
filesystem-aware transfer planning 合同：

- JSON 使用 `data.transfers[]` 作为 canonical transfer surface。
- capacity gate 按最终可能写入路径和 fallback copy 最坏情况计算。
- preview/dry-run 结果不能作为 run 许可；run 或 mutation 前必须重新 probe
  source/destination pair、重新做 capacity gate，并重新校验 boundary。
- fast copy 不可用、运行时失败或 fallback 成功/失败都必须进入 human output、
  JSON、audit/descriptor/plan 的对应落点。
- recovery backup 的 rename/ledger 安全模型仍按 docs/23，不被误建模成普通 copy
  transfer。

External `repo clone` 还继承 docs/24 的 repo clone transfer/capacity/imported-history
protection 规则，包括 save point storage copy、main workspace materialization、
fallback capacity gate、atomic publish 和 `--save-points all` imported history
protection。

## Repo Clone 和 Template Source Workflow 合同

当前 external `repo clone` 必须显式提供 source target 和 target folder/control root。
template workflows 使用同一个 `repo clone`，其 source 是 template repo 或 source
repo；当前不交付独立 `template` command 或未定义的 template surface。推荐入口：

```bash
jvs --control-root <source-control-root> --workspace main repo clone \
  <target-workspace-folder> \
  --target-control-root <target-control-root> \
  --save-points main \
  --json
```

语义必须固定：

- source 通过显式 source control root + workspace 定位。
- target workspace folder 来自 `repo clone <target-folder>`。
- target control root 必须显式提供。
- result JSON 使用 clone result surface：target workspace folder 输出为
  `data.target_folder`，target control root 输出为 `data.target_control_root`。
- target repo 生成新 `repo_id`。
- target 当前只创建 `main` workspace；clone options 不再重复使用第二个
  `--workspace`，也不新增 target workspace selector。
- target control root 不在 target workspace folder 内，target workspace folder
  不在 target control root 内。
- target control root 和 target workspace folder 只接受 missing 或 empty
  directory；JVS 可以创建 missing roots。
- target roots 不抢占另一个 repo、workspace 或非空用户目录；
  non-empty/adopt/merge/overwrite 必须以 `E_TARGET_ROOT_OCCUPIED` fail closed。
- source runtime state、locks、restore/recovery plans、cleanup plans、lifecycle
  plans、tmp/staging、open view state 不复制。
- source active operation 以 `E_ACTIVE_OPERATION_BLOCKING` fail closed；active
  writer race、dirty source workspace 或无法确认 source workspace clean 时，以
  `E_SOURCE_DIRTY` fail closed before target publish。
- target publish 必须是 no-overwrite/atomic publish 语义；发布被阻塞时以
  `E_ATOMIC_PUBLISH_BLOCKED` fail closed，source unchanged，target 不呈现为
  半成品 active repo。
- target 成功后 `doctor --strict` 通过，workspace folder 不含 control metadata。

`--save-points all` 受 docs/24 imported-save-point protection 约束。当前可以只支持
main-only clone；缺少 durable imported protection、cleanup preview/run
revalidation 或 doctor strict 检查时，`--save-points all` 必须以
`E_IMPORTED_HISTORY_PROTECTION_MISSING` fail closed。
ordinary clone omitted `--save-points` means `all`；external control root
omitted `--save-points` means `main`。面向 operator/script 的推荐心智是 external
clone 显式传 `--save-points main`，避免把 ordinary clone 的 all-history 默认误读为
external 默认能力。

## Lifecycle 合同

完整 move/rename/delete/detach 的 external lifecycle parity 延后。当前不能让旧
lifecycle 命令在 external control root 上半工作。

docs/25 的 `main workspace folder == repo root`、main `RealPath == repo root`
合同只适用于 default control data location。External control root 由本文覆盖：
repo identity 位于 control root，workspace files 位于 workspace folder，二者
不得被 docs/25 的 default lifecycle matrix 合并解释。

当前要求：

- external control root 上所有尚未按本文重新设计的 repo/workspace lifecycle
  command 必须 stable unsupported fail closed。
- 错误码使用当前外部合同 `E_SEPARATED_LIFECYCLE_UNSUPPORTED`，直到错误码统一迁移。
- 不移动 control root。
- 不移动 workspace folder。
- 不只改 registry 而不改 workspace/control。
- 不 delete workspace folder、control、runtime、locks、plans 或 audit。
- 输出和 JSON 必须说明 `No files were changed.` 或等价语义。

后续 external lifecycle parity 若实现 move/rename/delete/detach，必须重新定义
control root、workspace folder、workspace registry、locator/shortcut、runtime state
和 recovery journal 的 lifecycle matrix，不能复用 docs/25 的 default
main-workspace matrix。

## 分阶段实施计划

### Phase 0: Contract And TDD Scaffolding

目标是先让测试能准确表达 external control root 语义。

- 固定 object vocabulary：control data location、external control root、
  workspace folder、workspace binding、repo identity、locator/shortcut。
- 固定 conformance CLI selector 为 `--control-root <path> --workspace <name>`；
  `--repo` 只能作为未来/兼容扩展且只能指 control root，不是当前 conformance
  selector。
- 固定 public entry examples：
  `jvs init [folder] --control-root C --workspace main` 和
  `repo clone <target-folder> --target-control-root TC --save-points main`。
- 固定 JSON 必有字段和错误码，普通命令 JSON golden 断言 `data.folder`、
  `data.workspace` 和 `data.control_root`；doctor JSON golden 断言同一 target
  fields 以及 diagnostic `data.checks[]`。
- 固定 doctor `checks[]` 最小字段和 stable check names。
- 增加 path canonicalization、physical identity、ancestor/descendant、symlink
  escape、path drift 的 fake fixtures。
- 增加 explicit targeting test harness，能从 clean CWD 和另一个 repo CWD 运行命令。
- 增加 malicious workspace `.jvs` fixture。
- 增加 external control root doctor strict fixture。
- 增加 lifecycle unsupported tests。
- 增加 init/clone target missing、empty、occupied fixtures。
- 增加 clone dirty source、target occupied、atomic publish blocked、imported
  history protection missing fixtures。
- 不改普通用户语义，不把 internal repo mode 发布为 public mental model。

### Phase 1A: Init, Explicit Targeting, Status, Doctor

- 实现 external control root init。
- target control root 只接受 missing 或 empty directory；workspace folder 可
  adopt existing non-empty 用户文件，但 root-level `.jvs` 仍 fail closed。
- 实现显式 control root + workspace 定位。
- 实现固定 conformance selector：`--control-root <path> --workspace <name>`。
- 实现 clean CWD contract。
- 实现 `status --json` 和 `doctor --strict --json` 的最小字段。
- `doctor --strict` 覆盖 root invariant、repo_id/workspace mismatch、
  missing/malformed control、missing registered workspace folder、permission denied、
  workspace `.jvs` marker presence、active operation/recovery。
- workspace folder 不创建 `.jvs`。

### Phase 1B: Save, History, View, Restore, Recovery, Cleanup Boundary

- `save` 只 walk workspace managed files。
- `history` 和 `view` 不把 control root 当 workspace files。
- `restore preview/run` 按 workspace boundary 计算和写入。
- external save/view/restore 继承 docs/23 的 `data.transfers[]`、capacity gate、
  preview/run re-probe 和 fallback reporting。
- restore/recovery 不覆盖 control root，不跟随 symlink escape。
- recovery pending state 阻塞新 mutation，并通过 `error.hint`、human output 或
  doctor `checks[]` 给出安全诊断/恢复入口；普通错误响应仍保持 `data:null`。
- cleanup 不删除 workspace/control/runtime/locks/plans/audit roots。
- 对每个 mutation 增加 pre-mutation revalidation。

### Phase 1C: External Repo Clone Main-Only

- repo clone 支持显式 source control root + workspace；template workflows 通过从
  template repo/source repo 执行 repo clone 覆盖。
- target workspace folder 来自 `repo clone <target-folder>`；target control root
  必须显式提供。
- target control root/workspace folder 只接受 missing 或 empty directory；
  occupied 以 `E_TARGET_ROOT_OCCUPIED` fail closed。
- target 生成新 repo identity，只创建 main workspace。
- runtime/locks/plans/open views/tmp 不复制。
- dirty source、active writer race、source active operation fail closed with stable
  codes。
- target roots no-overwrite/atomic publish，publish blocked 以
  `E_ATOMIC_PUBLISH_BLOCKED` fail closed。
- external repo clone 继承 docs/24 transfer/capacity/imported-history protection
  规则。
- `--save-points main` 先交付；`--save-points all` 只有在 docs/24 imported
  protection 满足后交付，否则 `E_IMPORTED_HISTORY_PROTECTION_MISSING`。

### Deferred: Control Data Location Migration And Parity

以下未完成项明确 deferred，不属于当前交付：

- moving control data into/out of workspace。
- external lifecycle parity。
- safe locator/discovery。
- library API。
- real platform gates。
- all save-points protection。

这些 deferred 项必须有独立设计、TDD scaffolding、operator docs 和 recovery
矩阵后才能发布。尤其是 moving control data into/out of workspace 不能靠手工拆
`.jvs/` 或复制 control root 来宣称支持。

## QA 验收矩阵

| 场景 | 命令/fixture | 期望 | 默认 CI/后续 gate |
| --- | --- | --- | --- |
| Basic external init | `jvs init W --control-root C --workspace main --json` | C missing 时由 JVS 创建、empty dir 可初始化；W missing/empty 或 non-empty 用户文件可被 adopt；新 `repo_id`；public JSON 含 `folder`、`workspace`、`control_root`；workspace 无 `.jvs`；doctor strict passed | 默认 CI |
| Init workspace adopt | existing non-empty W with user files and no root-level `.jvs` | init succeeds；user files and metadata preserved；control registry points to W；status/save operate on existing files | 默认 CI |
| Init target occupied | C non-empty、含既有 JVS control/data，或需要 merge/overwrite existing control data | `E_TARGET_ROOT_OCCUPIED`；无 mutation | 默认 CI |
| Root same path | `C == W` | `E_CONTROL_PAYLOAD_OVERLAP`；无 mutation | 默认 CI |
| Workspace inside control | `W=C/workspace` | `E_PAYLOAD_INSIDE_CONTROL`；无 mutation | 默认 CI |
| Control inside workspace | `C=W/.control` | `E_CONTROL_INSIDE_PAYLOAD`；无 mutation | 默认 CI |
| Symlink escape | workspace 内 symlink 指向 control 或 workspace 外 | save/restore/view fail closed；`E_PATH_BOUNDARY_ESCAPE` | 默认 CI |
| Path drift before mutation | preview/check 后替换 control 或 workspace identity | run/mutation 前重新校验并以 `E_PATH_BOUNDARY_ESCAPE` 失败 | 默认 CI |
| Workspace marker present | workspace folder 下 root-level `.jvs` file、directory 或 symlink | strict external profile `E_PAYLOAD_LOCATOR_PRESENT`；doctor check `workspace_control_marker` failed；不得读取为 authority | 默认 CI |
| Clean CWD | 从 `/home/runner` 或另一个 JVS repo CWD 运行 explicit status | target 来自 `--control-root` + `--workspace`；不受 CWD 影响 | 默认 CI |
| Explicit targeting required | external control root workflow 无 explicit target，CWD 不能证明唯一 repo | `E_EXPLICIT_TARGET_REQUIRED`；通过 `error.hint` 或 human output 给出显式 selector 示例 | 默认 CI |
| Missing control source | status/save/doctor/clone source 显式 control root 不存在 | `E_CONTROL_MISSING` | 默认 CI |
| Malformed control | control root 缺 repo_id 或 schema 损坏 | `E_CONTROL_MALFORMED` | 默认 CI |
| Missing registered workspace | registry 中被选择 workspace 指向的 folder missing | `E_PAYLOAD_MISSING` | 默认 CI |
| Repo ID mismatch | control、locator、manifest 或 registry repo_id 不一致 | `E_REPO_ID_MISMATCH` | 默认 CI |
| Workspace mismatch | selector 为 `main`，registry/locator/evidence 为另一个 workspace | `E_WORKSPACE_MISMATCH` | 默认 CI |
| Permission denied | control 或 workspace 缺 required read/write/fsync 权限 | `E_PERMISSION_DENIED`；无部分写入 | 默认 CI fake permission |
| Save excludes control | workspace 有普通文件，control 有 audit/locks/plans | save point 只含 workspace managed files；control/runtime 不进入 save point | 默认 CI |
| Save transfer inheritance | fake fast/fallback save transfer | JSON `data.transfers[]`；capacity gate 覆盖 fallback；fallback reporting 清楚 | 默认 CI |
| View excludes control | `jvs view` external save point | view 不展示 control/runtime/locks/plans/audit | 默认 CI |
| View transfer inheritance | fake view materialization transfer | JSON `data.transfers[]`；internal transfer 正确标注；fallback reporting 清楚 | 默认 CI |
| Restore control symlink fail | workspace target symlink 到 control file | restore fail closed；control unchanged | 默认 CI |
| Restore preview/run re-probe | preview fast，run pair unsupported 或 fallback 容量不足 | run 重新 probe、重新 capacity gate；不足时写入前 fail closed | 默认 CI |
| Recovery boundary | pending recovery 后 workspace/control identity drift | recovery fail closed；不覆盖 control | 默认 CI |
| Cleanup boundary | cleanup preview/run | 不删除 workspace folder、control root、runtime、locks、plans、audit | 默认 CI |
| Active operation blocking | control root 有 active lock、intent、cleanup plan 或 lifecycle operation | 新 mutation `E_ACTIVE_OPERATION_BLOCKING` | 默认 CI |
| Recovery blocking | control root 有 pending restore/recovery state 或 restore plan | 新 mutation `E_RECOVERY_BLOCKING` | 默认 CI |
| Clone external target | `jvs --control-root C --workspace main repo clone T --target-control-root TC --save-points main --json` | target roots missing 时由 JVS 创建，empty dir 可用；新 repo_id；main only；target workspace 无 control；doctor strict passed | 默认 CI |
| Clone target same path | target `TC == T` | clone fail closed before publish；`E_CONTROL_PAYLOAD_OVERLAP` | 默认 CI |
| Clone target workspace inside control | target `T=TC/workspace` | clone fail closed before publish；`E_PAYLOAD_INSIDE_CONTROL` | 默认 CI |
| Clone target control inside workspace | target `TC=T/.control` | clone fail closed before publish；`E_CONTROL_INSIDE_PAYLOAD` | 默认 CI |
| Clone target occupied | target control/workspace root non-empty 或需 merge/overwrite/adopt | `E_TARGET_ROOT_OCCUPIED`；target not published | 默认 CI |
| Clone source dirty | source workspace dirty 或 writer race fake | `E_SOURCE_DIRTY`；target not published | 默认 CI |
| Clone runtime excluded | source 有 locks/plans/views/tmp | target 不复制 runtime state；doctor strict passed | 默认 CI |
| Clone transfer inheritance | external clone fake fast/fallback/capacity cases | JSON `data.transfers[]` 继承 docs/24；capacity gate 覆盖 fallback；run re-probe | 默认 CI |
| Clone atomic publish blocked | staging 后 final target 被创建或 no-replace rename 失败 | `E_ATOMIC_PUBLISH_BLOCKED`；source unchanged；final target 不是半成品 active repo | 默认 CI |
| Clone all protection gate | `--save-points all` without docs/24 imported protection | `E_IMPORTED_HISTORY_PROTECTION_MISSING`；提示 main-only 或升级 | 默认 CI |
| Doctor JSON | `doctor --strict --json` | 包含 `data.folder`、`data.workspace`、`data.control_root`；`checks[]` 每项含 `name`、`status`、`error_code`、`message`；必要 check names 全部出现 | 默认 CI JSON golden |
| Doctor strict failure goldens | root_overlap、workspace_control_marker、permissions、active_operation、recovery_state failed fixtures | failed `checks[].name` 与稳定 `error_code` 映射匹配本文 Doctor JSON Check 合同 | 默认 CI JSON golden |
| Lifecycle unsupported | external control root 上调用 move/rename/delete/detach | `E_SEPARATED_LIFECYCLE_UNSUPPORTED`；No files changed | 默认 CI |
| Default `.jvs/` compatibility | 普通 workspace `.jvs/` 的 init/status/save | 现有行为不回归；不要求 explicit control root | 默认 CI |
| AFSCP layout smoke | `/afscp/.../control` + `/afscp/.../workspace` fixture | 同通用 external control root contract；不使用 AFSCP 专有语义 | 后续 gated profile |
| Real WebDAV/export | workspace folder 通过平台 export，control 不暴露 | JVS runner 操作 control；workload 看不到 control | 后续平台 gate |

## Human Output 草案

普通 human output 不需要暴露太多内部结构，但必须让平台 operator 看清边界。

Init 成功：

```text
Initialized JVS workspace
Control data: external
Control root: /trusted/repo/control
Workspace: main
Folder: /workload/repo/workspace
Repo ID: repo-...
Boundary validated: yes
Doctor strict: passed
```

Explicit target 缺失：

```text
Cannot choose an external-control-root workspace from the current folder.
Pass the control root and workspace explicitly.

Recommended:
jvs --control-root /trusted/repo/control --workspace main status --json

No files were changed.
```

Lifecycle unsupported：

```text
This lifecycle command is not supported for external-control-root workspaces yet.
Repo ID: repo-...
Control root: /trusted/repo/control
Workspace: main
Folder: /workload/repo/workspace
No files were changed.
```

## 工程设计原则

- 先让 target resolution 返回一个完整 `RepoContext`，包含 repo_id、
  control_root、workspace name、workspace folder path、boundary validation
  evidence。
- 所有命令都消费同一个 `RepoContext`，不要每个命令自己猜 control/workspace。
- authority 只来自 control root 和显式 target，不来自 workspace locator。
- 路径校验必须是 physical identity 校验，不只是字符串前缀。
- 输出 JSON 和 doctor check 使用同一份 boundary validation 结果。
- mutation 前统一 revalidate，不把 preview/status 结果当 run 许可。
- external control root 的 fail closed 分支要先于旧 discovery/lifecycle fallback。
- 普通 default `.jvs/` 工作流的代码路径要有 compatibility tests，避免把新约束误
  施加给普通用户。

## 风险与缓解

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| `--repo` 语义含糊 | 平台可能把 workspace folder 当 control root，或受 CWD 影响 | 当前 conformance selector 固定为 `--control-root <path> --workspace <name>`；`--repo` 只能作为未来/兼容扩展且只能指 control root |
| workspace `.jvs` 被误信任 | 不可信 workload 可劫持 repo identity 或 workspace | strict external profile 以 `E_PAYLOAD_LOCATOR_PRESENT` fail closed；doctor check `workspace_control_marker` 固定映射该错误码 |
| target root 被误 adopt | init control root 或 clone target roots 把既有 control data、另一个 repo 或非空 target 合并进 external control root 工作流 | external init 只 adopt workspace user files；control root/clone target roots 只接受 missing/empty，occupied 以 `E_TARGET_ROOT_OCCUPIED` fail closed |
| 只做字符串路径判断 | symlink、bind mount、case drift 可逃逸 | canonical absolute path + physical identity，mutation 前重验 |
| restore 写穿到 control | control data 被 workspace symlink 覆盖 | restore/recovery no-follow boundary，escape fail closed |
| cleanup 删除 root | workspace 或 control runtime 被误删 | cleanup 只处理 reviewed unprotected save point objects，不删除 roots |
| 旧 lifecycle 半支持 | 只移动 control 或只移动 workspace，repo 进入坏状态 | 当前 stable unsupported fail closed；external lifecycle parity deferred |
| clone 复制 runtime | target 带 source locks/plans/recovery | 明确 runtime state excluded，并由 doctor strict 验收 |
| transfer/capacity 只按 default `.jvs/` 工作流估算 | external save/view/restore/clone 在 fallback 或跨边界时写入前失败太晚 | 继承 docs/23 `data.transfers[]`、capacity gate、preview/run re-probe 和 fallback reporting |
| `--save-points all` 被 cleanup 删除 imported history | clone 承诺的历史丢失 | 遵守 docs/24 imported protection gate，当前可先 main-only |

## Handoff Checklist

开发开始前必须先写红的测试类型：

- [ ] path invariant unit tests：same path、workspace inside control、control
  inside workspace、symlink escape、path drift、permission denied。
- [ ] explicit targeting tests：clean CWD、另一个 repo CWD、missing explicit
  target、固定 `--control-root <path> --workspace <name>` selector。
- [ ] JSON golden tests：success、doctor success、boundary failure；普通错误响应
  固定 `ok:false`、`data:null`、`error.code`、`error.message`、optional
  `error.hint`；ordinary command fields 固定为 `data.folder`、`data.workspace`、
  `data.control_root`。
- [ ] doctor JSON tests：`checks[]` 每项含 `name`、`status`、`error_code`、
  `message`，且出现 root_overlap、workspace_control_marker、repo_identity、
  workspace_binding、path_boundary、permissions、active_operation、
  recovery_state。
- [ ] doctor strict failure golden tests：failed `checks[].name` 与稳定
  `error_code` 映射，至少覆盖 root_overlap、workspace_control_marker、permissions、
  active_operation、recovery_state。
- [ ] workspace marker tests：workspace folder root-level `.jvs` absent，以及
  `.jvs` file、directory、symlink present；present 时
  `workspace_control_marker` failed 且 error code 为 `E_PAYLOAD_LOCATOR_PRESENT`。
- [ ] init/clone root occupancy tests：missing roots、empty dirs、control root
  occupied blocked、clone target roots occupied blocked、init workspace user files
  adopted without rewrite、workspace root-level `.jvs` blocked。
- [ ] command integration tests：init、status、doctor、save、history、view、
  restore preview/run、recovery blocking、cleanup boundary。
- [ ] transfer inheritance tests：external save/view/restore/clone 的
  `data.transfers[]`、capacity gate、preview/run re-probe、fallback success/failure
  reporting。
- [ ] repo clone tests：target folder + target control root、新 repo_id、main-only、
  runtime excluded、dirty source、target overlap、target occupied、atomic publish
  blocked、all-protection gate。
- [ ] lifecycle unsupported tests：external control root 上
  move/rename/delete/detach 全部 stable fail closed。
- [ ] default `.jvs/` compatibility tests：普通 workspace `.jvs/` 的现有
  init/status/save/doctor 不回归。

Phase 1A 完成前：

- [ ] external init 不在 workspace folder 创建 `.jvs`。
- [ ] external init control root 只接受 missing 或 empty directory；
  workspace folder 可 adopt existing non-empty 用户文件并保留 metadata；
  workspace root-level `.jvs` 以 `E_PAYLOAD_LOCATOR_PRESENT` fail closed。
- [ ] explicit target resolution 生成包含 repo_id、control_root、workspace name、
  workspace folder 的 validated context。
- [ ] doctor strict 覆盖 root invariant 和 identity mismatch。
- [ ] JSON 输出包含 public target fields：`data.folder`、`data.workspace`、
  `data.control_root`；doctor JSON 通过 `data.checks[]` 暴露 diagnostic fields。

Phase 1B 完成前：

- [ ] save 只 walk workspace managed files，control/runtime/locks/plans/audit
  不进入 save point。
- [ ] view 不展示 control data。
- [ ] restore/recovery 不跟随 symlink escape，不覆盖 control。
- [ ] external save/view/restore 继承 docs/23 transfer planning、capacity gate、
  preview/run re-probe 和 fallback reporting。
- [ ] cleanup 不删除 workspace/control/runtime/locks/plans/audit roots。
- [ ] 所有 mutation 前重新校验 boundary 和 active operation/recovery。

Phase 1C 完成前：

- [ ] repo clone 需要 target workspace folder + target control root；template
  workflow 只作为从 template repo/source repo 执行 repo clone。
- [ ] target 生成新 repo identity，只创建 main workspace。
- [ ] source runtime/locks/plans 不复制。
- [ ] dirty source、writer race、target root overlap、target root occupied 和
  atomic publish blocked 都 fail closed before publish，并使用稳定错误码。
- [ ] external repo clone 继承 docs/24 transfer/capacity/imported-history
  protection 规则。
- [ ] `--save-points all` 只有在 docs/24 imported protection 满足时才可通过，
  否则 `E_IMPORTED_HISTORY_PROTECTION_MISSING`。

Deferred 前置：

- [ ] moving control data into/out of workspace 有独立 reviewed flow。
- [ ] external lifecycle parity 不再依赖 current unsupported fallback。
- [ ] safe locator/discovery 的 human discovery 和 repair 范围重新评审。
- [ ] library API 有稳定 selector、error 和 JSON/typed result 合同。
- [ ] real platform gates 定义 AFSCP/JuiceFS/WebDAV 等 evidence。
- [ ] all save-points protection 满足 docs/24 imported history cleanup protection。

## 关键产品结论

External control root 是 JVS 应该拥有的通用能力。它让平台集成可以安全暴露
workspace folder，而不把 JVS control data 放进 workload 可写目录树。

普通本地 `.jvs/` inside workspace 继续是默认体验；external control root 只改变
control data location。JVS 产品心智仍然是同一个 workspace、同一套 save point
工作流。这个边界必须由 JVS 原生支持并由 TDD 验收，而不是让每个集成方在 mount、
WebDAV 或 sandbox 层重复补丁。
