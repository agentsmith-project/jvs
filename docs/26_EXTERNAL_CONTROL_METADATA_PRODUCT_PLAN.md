# Separated Control Root Repo Product Plan

**Subtitle:** separated control root repo handoff for platform integrations.

**Status:** active clean redesign, non-release-facing, not part of the v0 public contract. Clean future milestone handoff.

**中文名:** 控制面分离式 repo。

本文面向产品、工程、安全和 QA，定义 JVS 如何支持“control root 与 workspace payload root 分离”的 repo 形态。它只讨论产品合同、语义边界和验收要求；除本文明确冻结的 Phase 1 conformance CLI selector 外，不冻结最终 CLI 名称、内部目录结构或 metadata schema，也不更新 release-facing user docs。

## 文档验收标准

这份 handoff 应满足：

- 产品能直接解释：控制面分离式 repo 是 JVS 通用能力，不是 AgentSmith 专用功能。平台可以暴露 workspace payload，但不暴露 JVS control metadata。
- 工程能直接拆阶段：先固定对象模型、显式定位、JSON/error 合同和 fail-closed root invariant，再接 repo clone from source or template repos 和后续 lifecycle。
- QA 能直接抽矩阵：覆盖 root overlap、root occupancy、payload root-level `.jvs` marker、clean CWD、symlink escape、save/restore/cleanup boundary、clone split target、doctor JSON checks 和普通本地 repo 兼容。
- 安全评审能直接验收：control writes 只能进入 control root，payload writes 只能进入 payload root，control/payload 任何重叠、互为祖先/后代、symlink escape 或 path drift 都 fail closed。
- 文档明确 Phase 1 最小合同，不把 CLI/API 入口留成“后续决定”而没有语义冻结。

## 一句话目标

JVS 支持一种新的 repo mode：`separated_control`。在这个模式下，repo identity 和 JVS control metadata 位于可信的 `control root`，workspace 用户文件位于单独的 `payload root`；JVS 操作必须通过显式 control root + workspace/payload root 语义定位，不能依赖 workload 可写 payload 树里的 `.jvs` 作为权威来源。

## 产品判断

需求合理，且应作为 JVS 的通用平台集成能力，而不是 AgentSmith 或 AFSCP 的特化能力。

AgentSmith/AFSCP 是首个集成场景：sandbox 需要把某个 workspace payload 挂载为 agent 的 `$HOME` 或 `/workspace`，WebDAV/export 也只应暴露用户文件。JVS `.jvs` 如果仍在 payload root 里，不可信 workload 可以 rename/unlink 顶层控制目录项，filtered mount、chmod 或 sidecar 只能把正确性压力推到运行时层。

更好的产品边界是：JVS 原生承认 control plane 和 payload plane 可以分离。任何需要暴露 workspace payload、但不暴露 JVS control metadata 的平台，都可以使用同一能力。

## 背景

JVS 当前的普通本地模型仍然成立：

```text
project folder/
  .jvs/          # JVS control metadata
  user files     # workspace payload
```

这个模型对本地个人开发简单直观，必须保留。普通用户的默认 `jvs init` 仍可创建 `.jvs` inside folder 的 repo。

平台集成场景需要另一种形态：

```text
trusted platform path/
  control/       # JVS control root

workload-visible path/
  payload/       # workspace payload root
```

平台只把 `payload/` 挂载或 export 给 workload。JVS runner、AFSCP controller 或其他可信自动化用显式 target 操作 `control/`。

推荐的 AFSCP 类布局可以是：

```text
/afscp/namespaces/<namespace_id>/repos/<repo_id>/
  control/
    <JVS control metadata and JVS-owned runtime state>
  payload/
    <workspace files exposed to sandbox/WebDAV>
```

这只是一个集成布局示例，不是 JVS 的唯一目录结构承诺。

## 非目标

本里程碑不做：

- 不迁移已有 `.jvs`-inside-folder repo。
- 不改变普通本地用户默认 `jvs init` 心智。
- 不实现 AFSCP、Kubernetes、JuiceFS CSI、WebDAV、sandbox manager 或平台 ACL。
- 不实现 filtered mount，也不把 filtered mount 当成 Phase 1 主安全边界。
- 不把 payload root 下的 locator/shortcut 当作权威来源。
- 不引入 Git remote、branch、origin、push/pull 或网络同步心智。
- 不在 Phase 1 做 public human polish 或完整 library API，除非实现显式定位语义必须有最小内部 API。
- 不在 Phase 1 做旧 repo migration、完整 move/rename/delete/detach lifecycle，或跨模式 attach/adopt。
- 不把 `--save-points all` clone 发布为默认，除非 docs/24 要求的 durable imported-save-point cleanup protection 已经满足。

## 对象模型和词汇

### Repo Mode

JVS 至少承认两种 repo mode：

| Mode | 含义 | 普通发现入口 |
| --- | --- | --- |
| `embedded_control` | 现有普通本地模式，control metadata 位于 workspace folder 下的 `.jvs` | 从 workspace folder 向上找 `.jvs` |
| `separated_control` | control root 与 workspace payload root 分离 | Phase 1 必须显式指定 control root + workspace |

`embedded_control` 保持现有用户体验。本文主要定义 `separated_control`。

### Control Root

`control root` 是 JVS 可信控制面根目录。它拥有或指向：

- repo identity，例如 `repo_id`。
- workspace registry。
- save point descriptors 和 JVS-owned save point storage。
- audit/provenance。
- runtime state、locks、restore/recovery plans、cleanup plans、tmp/staging，除非实现拆出独立 runtime root；即便拆出，也必须在同一可信 control boundary 内。

control root 不等于用户 workspace folder。control root 不应被 sandbox、WebDAV 或不可信 workload 暴露为可写 payload。

### Payload Root

`payload root` 是某个 workspace 的用户文件根目录。它可以被 workload 挂载、被 WebDAV/export 暴露，或被用户工具读写。

在 `separated_control` mode 下，payload root 不应包含 JVS control metadata。default strict separated profile 不创建 payload `.jvs`。Phase 1 默认 marker set 固定为 payload root 下 root-level `.jvs` path；file、directory 或 symlink 都算 present。出现该 path 时，Phase 1 默认 CI 必须以 `E_PAYLOAD_LOCATOR_PRESENT` fail closed；该 path 不能作为 JVS authority。

### Workspace

workspace 是一个命名的工作视图，至少由以下语义组成：

- `workspace_name`，Phase 1 先只要求 `main`。
- `payload_root`，该 workspace 的真实用户文件 root。
- 所属 `repo_id`。
- 所属 `control_root`。

分离式模式下，workspace payload root 是 workspace 的用户文件位置，但 repo 不再天然等于某个 folder。

### Repo Identity

`repo_id` 是 repo 的稳定身份。路径不是身份。复制、repo clone 或 template-source workflow 创建新 repo 时必须生成新 `repo_id`。source `repo_id` 只能作为 provenance 记录，不能成为 target repo identity。

### Locator/Shortcut

`locator` 或 `shortcut` 是方便发现的线索，不是权威状态。

普通本地 `embedded_control` mode 下，folder 里的 `.jvs` 是 control root 本身，不是本文说的非权威 locator。

external workspace 或未来高级模式可以有轻量 locator，帮助 human discovery 找回 control root、repo_id 和 workspace_name。但在 `separated_control` Phase 1：

- default strict separated profile 和 platform-managed runner 不能依赖 payload 可写 locator。
- locator 不能包含唯一控制状态。
- default strict separated profile 的 Phase 1 默认 marker set 只包含 payload root 下 root-level `.jvs` path；file、directory 或 symlink 都算 present。
- 该 `.jvs` path 必须以 `E_PAYLOAD_LOCATOR_PRESENT` fail closed，不能读取、follow 或解释为 authority。
- 未来若要支持更多 marker 名称或形态，只能放进后续 gated compatibility profile，不属于 Phase 1 默认 CI；即使 ignored，也必须输出 `locator_authoritative=false`，并证明没有读取其中状态。
- 未来兼容 profile 如果显式允许 locator evidence，locator mismatch、repo_id mismatch 或 workspace mismatch 必须 fail closed。
- JSON 必须能表达 `locator_authoritative=false`。

## Phase 1 最小定位合同

Phase 0 和 Phase 1 conformance CLI selector 固定为 `--control-root <path> --workspace <name>`，并必须写进失败的 CLI/JSON golden tests。调用方必须能在干净 CWD 下显式指定 control root 和 workspace，稳定操作目标 separated repo，不受 payload locator、另一个 repo 的 CWD 或 ambient discovery 影响。

固定初始化入口：

```bash
jvs init --control-root <path> --payload-root <path> --workspace main --json
```

固定运行时显式定位入口：

```bash
jvs --control-root <path> --workspace <name> status --json
jvs --control-root <path> --workspace <name> doctor --strict --json
jvs --control-root <path> --workspace <name> save -m "message" --json
```

未来可以新增 config、environment 或 library API 等等价入口，例如：

```bash
JVS_CONTROL_ROOT=<path> JVS_WORKSPACE=<name> jvs status --json
```

这些 future/API 入口不是 Phase 1 conformance selector。conformance suite 和 handoff examples 必须使用固定 selector，不能保留含糊语义：

- `--control-root <path> --workspace <name>` 是 Phase 1 separated conformance selector。
- 如果未来扩展 `--repo`，必须明确 `--repo <path>` 在 separated mode 只能指向 `control root`，并要求同时提供 `--workspace <name>` 或等价 workspace selector；它不是 Phase 1 conformance selector。
- `--repo <payload-root>` 在 separated mode 不能被猜成 control root。
- 没有显式 target 且 CWD 不足以安全证明唯一 repo/workspace 时，必须失败，错误码为 `E_EXPLICIT_TARGET_REQUIRED`。

### Clean CWD 合同

Phase 1 命令必须能从任意干净 CWD 运行：

```bash
cd /home/runner
jvs --control-root /trusted/repo/control --workspace main status --json
```

期望：

- discovery 不读取当前 CWD 中另一个 `.jvs`。
- discovery 不读取 payload root 下的 `.jvs` 作为 authority。
- 输出中的 `control_root`、`payload_root`、`repo_id`、`workspace_name` 来自显式 target 和 control metadata 校验。
- mutation 前重新校验 control/payload boundary。

## JSON 输出合同

JSON 使用 docs/02 现有 envelope。成功响应的 authoritative fields 是 `data.control_root`、`data.payload_root`、`data.repo_mode` 和 `data.workspace_name`，Phase 0 必须固定 JSON golden 断言。普通错误响应必须 `ok:false`、`data:null`，`error` 只包含稳定 `code`、`message` 和 optional `hint`；普通失败的 boundary/context 诊断不能放进 non-null `data`。需要详细诊断时，调用 `jvs --control-root <path> --workspace <name> doctor --strict --json`，由 doctor diagnostic result 的 `data.checks[]` 输出 failed checks 和稳定 `error_code` 映射。集成方不能解析 human output 或目录布局来判断边界。

legacy/top-level `repo_root` 若保留，在 separated mode 下必须等于 `data.control_root`，或明确标记为 display-only/source display path；它绝不能表示 payload root，也不能成为 separated mode 的 authority。

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
    "workspace_name": "main",
    "control_root": "/trusted/repo/control",
    "payload_root": "/workload/repo/payload",
    "repo_mode": "separated_control",
    "separated_control": true,
    "boundary_validated": true,
    "locator_authoritative": false,
    "doctor_strict": "passed"
  },
  "error": null
}
```

`repo_root` 如果保留在 envelope 中，在 separated mode 下必须显示 control root，或按现有 envelope 兼容规则明确为 display-only/source display path。`data.control_root`、`data.payload_root`、`data.repo_mode` 和 `data.workspace_name` 是 JSON golden 的 authoritative 字段。

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
    "workspace_name": "main",
    "control_root": "/trusted/repo/control",
    "payload_root": "/workload/repo/payload",
    "repo_mode": "separated_control",
    "separated_control": true,
    "boundary_validated": true,
    "locator_authoritative": false,
    "doctor_strict": "passed",
    "checks": [
      {
        "name": "root_overlap",
        "status": "passed",
        "error_code": null,
        "message": "Control and payload roots are separate."
      },
      {
        "name": "payload_locator",
        "status": "passed",
        "error_code": null,
        "message": "No payload-root control marker is present."
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
    "message": "Cannot use separated control repo: payload root is inside control root.",
    "hint": "Run `jvs --control-root /trusted/repo/control --workspace main doctor --strict --json` to inspect failed checks, or create/clone with a payload root outside the control root."
  }
}
```

### JSON 字段要求

| 字段 | 要求 |
| --- | --- |
| `repo_id` | 必须来自 control root 中的 repo identity |
| `workspace_name` | 必须是本次显式选择并经 control registry 校验的 workspace |
| `control_root` | canonical absolute display path |
| `payload_root` | canonical absolute display path |
| `repo_mode` | separated repo 固定为 `separated_control` |
| `separated_control` | 布尔摘要，便于集成方快速断言 |
| `boundary_validated` | 本次操作已经完成 control/payload root invariant 校验 |
| `locator_authoritative` | separated Phase 1 固定为 `false` |
| `doctor_strict` | `passed`、`failed`、`not_run` 或等价枚举 |
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

Phase 1 doctor strict 至少固定这些 check names：

| Check name | 覆盖 |
| --- | --- |
| `root_overlap` | control/payload same path、ancestor/descendant 或无法分类的 overlap |
| `payload_locator` | payload root 的 root-level `.jvs` path；file、directory 或 symlink 都算 present |
| `repo_identity` | repo_id、clone manifest、stored metadata 一致性 |
| `workspace_binding` | workspace selector、registry entry、payload evidence 一致性 |
| `path_boundary` | canonical path、symlink、hardlink、case-folding、path drift 和 boundary escape |
| `permissions` | control/payload required read/write/fsync 权限 |
| `active_operation` | active lock、intent、cleanup plan、lifecycle operation |
| `recovery_state` | pending restore/recovery state、restore plan 和 recovery identity mismatch |

Doctor strict failure goldens must assert failed `checks[].name` and stable `error_code` mappings at minimum:

| Failed check | Golden fixture | Stable `error_code` |
| --- | --- | --- |
| `root_overlap` | same root fixture | `E_CONTROL_PAYLOAD_OVERLAP` |
| `root_overlap` | payload-inside-control fixture | `E_PAYLOAD_INSIDE_CONTROL` |
| `root_overlap` | control-inside-payload fixture | `E_CONTROL_INSIDE_PAYLOAD` |
| `payload_locator` | payload root-level `.jvs` file, directory, or symlink | `E_PAYLOAD_LOCATOR_PRESENT` |
| `permissions` | required control or payload read/write/fsync permission denied | `E_PERMISSION_DENIED` |
| `active_operation` | active lock、intent、cleanup plan 或 lifecycle operation blocks a new mutation | `E_ACTIVE_OPERATION_BLOCKING` |
| `recovery_state` | pending restore/recovery state blocks a new mutation | `E_RECOVERY_BLOCKING` |

## 错误码合同

下表 code 是 Phase 1 JSON golden 的稳定错误码。若实现前做最后一次命名统一，必须在 Phase 0 同步更新本文和测试；开发开始后不得让同一场景存在多个可接受 code。

| 错误码 | 场景 | 行为 |
| --- | --- | --- |
| `E_CONTROL_PAYLOAD_OVERLAP` | control root 与 payload root 是同一路径、同一物理 identity，或重叠关系无法进一步分类 | fail closed before mutation |
| `E_PAYLOAD_INSIDE_CONTROL` | payload root 位于 control root 内部 | fail closed before mutation |
| `E_CONTROL_INSIDE_PAYLOAD` | control root 位于 payload root 内部 | fail closed before mutation |
| `E_PATH_BOUNDARY_ESCAPE` | canonical path、symlink、hardlink 或平台 file identity 显示操作会逃出声明边界，或出现 path drift | fail closed before mutation |
| `E_CONTROL_MISSING` | 需要已有/source/control registry 的命令显式选择的 control root 不存在或不可达；例如 status、save、doctor 或 clone source | fail closed |
| `E_CONTROL_MALFORMED` | control root 存在但缺少必须 metadata、schema 损坏或无法证明 repo identity | fail closed |
| `E_PAYLOAD_MISSING` | 已登记/被选择 workspace 的 payload root 不存在或不可达 | fail closed |
| `E_REPO_ID_MISMATCH` | 显式 target、locator、registry、clone manifest 或 stored metadata 的 repo_id 不一致 | fail closed |
| `E_WORKSPACE_MISMATCH` | workspace selector、registry entry、payload evidence 或 locator 的 workspace_name 不一致 | fail closed |
| `E_PERMISSION_DENIED` | control root 或 payload root 的 required read/write/fsync 权限不足 | fail closed，不做部分写入 |
| `E_EXPLICIT_TARGET_REQUIRED` | CWD/ambient discovery 不能安全证明 separated repo target，且调用方未提供 control root + workspace | fail closed；通过 `error.hint` 或 human output 给出显式 selector 示例 |
| `E_PAYLOAD_LOCATOR_PRESENT` | default strict separated profile 下 payload root 出现 root-level `.jvs` path；file、directory 或 symlink 都算 present | fail closed；不能读取为 authority |
| `E_TARGET_ROOT_OCCUPIED` | init/clone target control root 或 payload root 已存在且非空，或需要 adopt/merge/overwrite existing control/payload data 才能继续 | fail closed before mutation；missing target roots 由 JVS 创建，不使用 missing-root 错误码 |
| `E_SOURCE_DIRTY` | clone source workspace 有 unsaved changes、writer race，或无法证明 source clean | fail closed before target publish |
| `E_ATOMIC_PUBLISH_BLOCKED` | clone staging 无法 no-overwrite/atomic publish 到 final target，或 final target 会呈现半成品 repo | fail closed；source unchanged |
| `E_IMPORTED_HISTORY_PROTECTION_MISSING` | 请求 `--save-points all`，但 durable imported-save-point cleanup protection 未实现、未写入或 doctor strict 无法验证 | fail closed，并提示 `--save-points main` 或升级 |
| `E_SEPARATED_LIFECYCLE_UNSUPPORTED` | separated repo 上调用 Phase 1 未实现的 move/rename/delete/detach lifecycle 命令 | stable unsupported fail closed |
| `E_ACTIVE_OPERATION_BLOCKING` | control root 中已有 active lock、intent、cleanup plan 或 lifecycle operation 阻塞本次操作 | fail closed；如果存在安全下一步，用 `error.hint` 或 human output 展示 |
| `E_RECOVERY_BLOCKING` | restore/recovery state 或 restore plan 要求先 resume/rollback/diagnose，不能开始新的 mutation | fail closed；如果存在安全 recovery 下一步，用 `error.hint` 或 human output 展示 |

## 安全不变量

这些不变量是 Phase 1 的发布线，不是实现建议。

- 所有 root path 在使用前必须解析为 canonical absolute path，并记录平台可用的 physical identity，例如 device/inode、file ID 或等价证据。
- `control_root == payload_root` 必须 fail closed。
- `payload_root` 位于 `control_root` 内部必须 fail closed。
- `control_root` 位于 `payload_root` 内部必须 fail closed。
- 如果 symlink、bind mount、hardlink、case-folding 或 path drift 让 JVS 无法可靠判断边界，必须 fail closed。
- 每次 mutation 前必须重新校验 root identity、workspace identity、repo_id、权限、active locks/recovery 和 boundary。
- JVS control writes 只能发生在 control root 或受控 control runtime boundary 内。
- JVS payload writes 只能发生在本次 workspace payload root 内。
- save/status/history/view/restore/cleanup/recovery 不能把 control root、runtime、locks、plans、audit 或 locator 捕获为 payload 文件。
- restore/recovery 不能覆盖 control root，不能通过 payload symlink 写入 control root。
- cleanup 不能删除 workspace root、payload root、control root、runtime root、locks、plans 或 audit root。cleanup 只能删除经过 reviewed flow 判定为 unprotected 的 JVS-owned save point objects。
- default strict separated profile 不在 payload root 创建 `.jvs`。
- payload root 中如存在 root-level `.jvs` path，无论它是 file、directory 还是 symlink，它都不是 locator，不是 authority，不是 control root。default strict separated profile 必须以 `E_PAYLOAD_LOCATOR_PRESENT` fail closed；未来更多 marker 名称/形态或 ignore 行为只能作为非 Phase 1 默认的 gated compatibility profile，也必须输出 `locator_authoritative=false` 并证明没有读取其中状态。

## 操作边界合同

### Init

`init` 创建 separated repo 时必须：

- target control root 和 payload root 只接受 missing 或 empty directory。
- JVS 可以创建 missing target root；已存在 empty directory 可被初始化。
- non-empty target root、adopt existing payload/control data、merge existing contents、overwrite existing repo/workspace 都不是 Phase 1 交付面，必须以 `E_TARGET_ROOT_OCCUPIED` fail closed。
- 生成新 `repo_id`。
- 创建 `main` workspace registry entry，指向 payload root canonical path。
- 写入 `repo_mode=separated_control` 或等价 durable metadata。
- 不在 payload root 创建 `.jvs`。
- 初始化后 `doctor --strict` 通过，JSON 中 `boundary_validated=true`、`locator_authoritative=false`。

### Explicit Targeting, Status, History

`status`、`history` 和 workspace metadata read 操作必须：

- 只从显式 control root 读取 authority。
- 用 workspace name 找到 payload root。
- 在读取 payload 前校验 boundary。
- 即使 CWD 位于另一个 JVS repo 内，也不能改变 target。
- 如果 payload root 下存在恶意 `.jvs`，不能被它改写 repo_id、workspace_name 或 control root。

### Save

`save` 只保存 workspace payload 中的 managed files。

必须排除：

- control root。
- control runtime、locks、plans、audit。
- payload root 的 root-level `.jvs` path，file、directory 或 symlink 都算 present；default strict separated profile 下应先因 `E_PAYLOAD_LOCATOR_PRESENT` 失败。
- symlink escape 指向 payload root 外的文件。

mutation 前必须重新校验 control/payload boundary。save descriptor、audit 和 JSON 不能把 control metadata 描述成用户 payload。

### View

`view` 只 materialize save point payload 或指定 path。view materialization 不能把 control root 作为 payload 内容展示，也不能通过 symlink escape 读取 control root。

如果 view 需要临时目录或 cache，它必须属于 JVS-controlled boundary，并且不被当作 workspace payload 保存。

### Restore

`restore preview` 和 `restore run` 都必须按 payload boundary 计算影响。

要求：

- preview 不写用户 payload，不提交 recovery 状态变更。
- run 前重新校验 boundary，不能信任 preview 的旧 path/identity。
- restore 只能写 payload root 内的 managed target。
- payload symlink 指向 control root 或 payload root 外部时必须 fail closed。
- restore/recovery backup 不能覆盖 control root。
- 如果 pending recovery 存在，新 mutation 必须被 `E_RECOVERY_BLOCKING` 阻塞。

### Recovery

recovery 只处理已记录的 workspace payload mutation 和 JVS-controlled recovery state。

要求：

- recovery plan、restore plan、locks 和 audit 位于 control boundary，不进入 save point payload。
- recovery resume/rollback 写 payload 前重新校验 payload root identity。
- recovery 不能删除、覆盖或 materialize control root。
- 如果 recovery state 与 repo_id、workspace_name 或 payload root identity mismatch，fail closed。

### Cleanup

cleanup 是 reviewed deletion flow，不是 root deletion flow。

要求：

- cleanup preview/run 只考虑 JVS-owned save point objects 和已定义的 runtime-safe residue。
- cleanup 不删除 workspace root、payload root、control root、runtime root、locks、plans 或 audit root。
- cleanup 不因为 payload root 不可达就删除 workspace registry。
- cleanup run 重新校验 imported protection、active operation、repo_id 和 boundary。
- docs/24 的 imported save point protection 未满足时，`--save-points all` clone 不得发布。

### Transfer Planning Inheritance

separated `save`、`view`、`restore` 和 `repo clone` 继承 docs/23 的 filesystem-aware transfer planning 合同：

- JSON 使用 `data.transfers[]` 作为 canonical transfer surface。
- capacity gate 按最终可能写入路径和 fallback copy 最坏情况计算。
- preview/dry-run 结果不能作为 run 许可；run 或 mutation 前必须重新 probe source/destination pair、重新做 capacity gate，并重新校验 boundary。
- fast copy 不可用、运行时失败或 fallback 成功/失败都必须进入 human output、JSON、audit/descriptor/plan 的对应落点。
- recovery backup 的 rename/ledger 安全模型仍按 docs/23，不被误建模成普通 copy transfer。

separated `repo clone` 还继承 docs/24 的 repo clone transfer/capacity/imported-history protection 规则，包括 save point storage copy、main workspace materialization、fallback capacity gate、atomic publish 和 `--save-points all` imported history protection。

## Repo Clone 和 Template Source Workflow 合同

Phase 1 separated `repo clone` 必须显式提供 source target 和 target roots。template workflows 使用同一个 `repo clone`，其 source 是 template repo 或 source repo；Phase 1C 不交付独立 `template` command 或未定义的 template surface。推荐入口可类似：

```bash
jvs --control-root <source-control-root> --workspace main repo clone \
  --target-control-root <target-control-root> \
  --target-payload-root <target-payload-root> \
  --save-points main \
  --json
```

具体命令名可以调整，但语义必须固定：

- source 通过显式 source control root + workspace 定位。
- target 必须显式提供 `target_control_root` 和 `target_payload_root`。
- target repo 生成新 `repo_id`。
- target Phase 1 只创建 `main` workspace；clone options 不再重复使用第二个 `--workspace`，也不新增 target workspace selector。
- target control root 不在 target payload root 内，target payload root 不在 target control root 内。
- target control root 和 target payload root 只接受 missing 或 empty directory；JVS 可以创建 missing roots。
- target roots 不抢占另一个 repo、workspace 或非空用户目录；non-empty/adopt/merge/overwrite 必须以 `E_TARGET_ROOT_OCCUPIED` fail closed。
- source runtime state、locks、restore/recovery plans、cleanup plans、lifecycle plans、tmp/staging、open view state 不复制。
- source active operation 以 `E_ACTIVE_OPERATION_BLOCKING` fail closed；active writer race、dirty source workspace 或无法确认 source workspace clean 时，以 `E_SOURCE_DIRTY` fail closed before target publish。
- target publish 必须是 no-overwrite/atomic publish 语义；发布被阻塞时以 `E_ATOMIC_PUBLISH_BLOCKED` fail closed，source unchanged，target 不呈现为半成品 active repo。
- target 成功后 `doctor --strict` 通过，payload root 不含 control metadata。

`--save-points all` 受 docs/24 imported-save-point protection 约束。Phase 1C 可以只支持 main-only clone；缺少 durable imported protection、cleanup preview/run revalidation 或 doctor strict 检查时，`--save-points all` 必须以 `E_IMPORTED_HISTORY_PROTECTION_MISSING` fail closed。

## Lifecycle 合同

完整 move/rename/delete/detach 可以延后到 Phase 2。但 Phase 1 不能让旧 lifecycle 命令在 separated repo 上半工作。

docs/25 的 `main workspace folder == repo root`、main `RealPath == repo root` 合同只适用于 embedded/current lifecycle model。`separated_control` mode 由本文覆盖：repo identity 位于 control root，workspace payload 位于 payload root，二者不得被 docs/25 的 embedded lifecycle matrix 合并解释。

Phase 1 要求：

- separated repo 上所有尚未按本文重新设计的 repo/workspace lifecycle command 必须 stable unsupported fail closed。
- 错误码使用 `E_SEPARATED_LIFECYCLE_UNSUPPORTED`。
- 不移动 control root。
- 不移动 payload root。
- 不只改 registry 而不改 payload/control。
- 不 delete payload、control、runtime、locks、plans 或 audit。
- 输出和 JSON 必须说明 `No files were changed.` 或等价语义。

后续 Phase 2 若实现 separated move/rename/delete/detach，必须重新定义 control root、payload root、workspace registry、locator/shortcut、runtime state 和 recovery journal 的 lifecycle matrix，不能复用 docs/25 的 embedded main-workspace matrix。

## 分阶段实施计划

### Phase 0: Contract And TDD Scaffolding

目标是先让测试能准确表达 separated repo 语义。

- 固定 object vocabulary：control root、payload root、workspace、repo identity、locator/shortcut。
- 固定 conformance CLI selector 为 `--control-root <path> --workspace <name>`；`--repo` 只能作为未来/兼容扩展且只能指 control root，不是 Phase 1 conformance selector。
- 固定 JSON 必有字段和错误码，JSON golden 断言 `data.control_root`、`data.payload_root`、`data.repo_mode`、`data.workspace_name` 为 authoritative fields。
- 固定 doctor `checks[]` 最小字段和 stable check names。
- 增加 path canonicalization、physical identity、ancestor/descendant、symlink escape、path drift 的 fake fixtures。
- 增加 explicit targeting test harness，能从 clean CWD 和另一个 repo CWD 运行命令。
- 增加 malicious payload `.jvs` fixture。
- 增加 separated repo doctor strict fixture。
- 增加 lifecycle unsupported tests。
- 增加 init/clone target missing、empty、occupied fixtures。
- 增加 clone dirty source、target occupied、atomic publish blocked、imported history protection missing fixtures。
- 不改普通用户语义，不发布 public docs。

### Phase 1A: Init, Explicit Targeting, Status, Doctor

- 实现 separated init。
- target control/payload roots 只接受 missing 或 empty directory；non-empty/adopt existing data/control 以 `E_TARGET_ROOT_OCCUPIED` fail closed。
- 实现显式 control root + workspace 定位。
- 实现固定 conformance selector：`--control-root <path> --workspace <name>`。
- 实现 clean CWD contract。
- 实现 `status --json` 和 `doctor --strict --json` 的最小字段。
- `doctor --strict` 覆盖 root invariant、repo_id/workspace mismatch、missing/malformed control、missing registered payload、permission denied、payload `.jvs` marker presence、active operation/recovery。
- payload root 不创建 `.jvs`。

### Phase 1B: Save, History, View, Restore, Recovery, Cleanup Boundary

- `save` 只 walk payload managed files。
- `history` 和 `view` 不把 control root 当 payload。
- `restore preview/run` 按 payload boundary 计算和写入。
- separated save/view/restore 继承 docs/23 的 `data.transfers[]`、capacity gate、preview/run re-probe 和 fallback reporting。
- restore/recovery 不覆盖 control root，不跟随 symlink escape。
- recovery pending state 阻塞新 mutation，并通过 `error.hint`、human output 或 doctor `checks[]` 给出安全诊断/恢复入口；普通错误响应仍保持 `data:null`。
- cleanup 不删除 workspace/payload/control/runtime/locks/plans/audit roots。
- 对每个 mutation 增加 pre-mutation revalidation。

### Phase 1C: Separated Repo Clone Main-Only

- repo clone 支持显式 source control root + workspace；template workflows 通过从 template repo/source repo 执行 repo clone 覆盖。
- target 必须显式提供 target control root + target payload root。
- target control/payload roots 只接受 missing 或 empty directory；occupied 以 `E_TARGET_ROOT_OCCUPIED` fail closed。
- target 生成新 repo identity，只创建 main workspace。
- runtime/locks/plans/open views/tmp 不复制。
- dirty source、active writer race、source active operation fail closed with stable codes。
- target roots no-overwrite/atomic publish，publish blocked 以 `E_ATOMIC_PUBLISH_BLOCKED` fail closed。
- separated repo clone 继承 docs/24 transfer/capacity/imported-history protection 规则。
- `--save-points main` 先交付；`--save-points all` 只有在 docs/24 imported protection 满足后交付，否则 `E_IMPORTED_HISTORY_PROTECTION_MISSING`。

### Phase 2: Migration And Lifecycle

- 设计从 embedded repo 迁移到 separated repo 的 reviewed flow。
- 设计 separated repo move/rename/delete/detach。
- 定义 lifecycle journal、recovery、safe CWD 和状态更新矩阵。
- 设计 locator/shortcut 的 human discovery 和 repair 边界。
- 明确普通本地 `.jvs` repo 与 separated repo 的跨模式 doctor 语义。

### Phase 3: Polish, Library API, Docs

- 更新 public CLI spec、user docs、operator docs 和 conformance plan。
- 提供稳定 library API，如果平台集成需要绕开 CLI。
- 增加更好的 human output。
- 增加真实平台集成 gated profile，例如 AFSCP/JuiceFS/WebDAV 组合 evidence。默认 CI 不依赖这些平台。

## QA 验收矩阵

| 场景 | 命令/fixture | 期望 | 默认 CI/后续 gate |
| --- | --- | --- | --- |
| Basic separated init | `jvs init --control-root C --payload-root P --workspace main --json` | C/P missing 时由 JVS 创建，empty dir 可初始化；新 `repo_id`；`repo_mode=separated_control`；payload 无 `.jvs`；doctor strict passed | 默认 CI |
| Init target occupied | C 或 P non-empty、含既有 JVS control/data，或需要 adopt existing data/control | `E_TARGET_ROOT_OCCUPIED`；无 mutation | 默认 CI |
| Root same path | `C == P` | `E_CONTROL_PAYLOAD_OVERLAP`；无 mutation | 默认 CI |
| Payload inside control | `P=C/payload` | `E_PAYLOAD_INSIDE_CONTROL`；无 mutation | 默认 CI |
| Control inside payload | `C=P/.control` | `E_CONTROL_INSIDE_PAYLOAD`；无 mutation | 默认 CI |
| Symlink escape | payload 内 symlink 指向 control 或 payload 外 | save/restore/view fail closed；`E_PATH_BOUNDARY_ESCAPE` | 默认 CI |
| Path drift before mutation | preview/check 后替换 control 或 payload identity | run/mutation 前重新校验并以 `E_PATH_BOUNDARY_ESCAPE` 失败 | 默认 CI |
| Payload marker present | payload root 下 root-level `.jvs` file、directory 或 symlink | default strict separated profile `E_PAYLOAD_LOCATOR_PRESENT`；JSON `locator_authoritative=false`；不得读取为 authority | 默认 CI |
| Clean CWD | 从 `/home/runner` 或另一个 JVS repo CWD 运行 explicit status | target 来自 `--control-root` + `--workspace`；不受 CWD 影响 | 默认 CI |
| Explicit targeting required | separated repo 无 explicit target，CWD 不能证明唯一 repo | `E_EXPLICIT_TARGET_REQUIRED`；通过 `error.hint` 或 human output 给出显式 selector 示例 | 默认 CI |
| Missing control source | status/save/doctor/clone source 显式 control root 不存在 | `E_CONTROL_MISSING` | 默认 CI |
| Malformed control | control root 缺 repo_id 或 schema 损坏 | `E_CONTROL_MALFORMED` | 默认 CI |
| Missing registered payload | registry 中被选择 workspace 指向的 payload root missing | `E_PAYLOAD_MISSING` | 默认 CI |
| Repo ID mismatch | control、locator、manifest 或 registry repo_id 不一致 | `E_REPO_ID_MISMATCH` | 默认 CI |
| Workspace mismatch | selector 为 `main`，registry/locator/evidence 为另一个 workspace | `E_WORKSPACE_MISMATCH` | 默认 CI |
| Permission denied | control 或 payload 缺 required read/write/fsync 权限 | `E_PERMISSION_DENIED`；无部分写入 | 默认 CI fake permission |
| Save excludes control | payload 有普通文件，control 有 audit/locks/plans | save point 只含 payload managed files；control/runtime 不进入 save point | 默认 CI |
| Save transfer inheritance | fake fast/fallback save transfer | JSON `data.transfers[]`；capacity gate 覆盖 fallback；fallback reporting 清楚 | 默认 CI |
| View excludes control | `jvs view` separated save point | view 不展示 control/runtime/locks/plans/audit | 默认 CI |
| View transfer inheritance | fake view materialization transfer | JSON `data.transfers[]`；internal transfer 正确标注；fallback reporting 清楚 | 默认 CI |
| Restore control symlink fail | payload target symlink 到 control file | restore fail closed；control unchanged | 默认 CI |
| Restore preview/run re-probe | preview fast，run pair unsupported 或 fallback 容量不足 | run 重新 probe、重新 capacity gate；不足时写入前 fail closed | 默认 CI |
| Recovery boundary | pending recovery 后 payload/control identity drift | recovery fail closed；不覆盖 control | 默认 CI |
| Cleanup boundary | cleanup preview/run | 不删除 workspace root、payload root、control root、runtime、locks、plans、audit | 默认 CI |
| Active operation blocking | control root 有 active lock、intent、cleanup plan 或 lifecycle operation | 新 mutation `E_ACTIVE_OPERATION_BLOCKING` | 默认 CI |
| Recovery blocking | control root 有 pending restore/recovery state 或 restore plan | 新 mutation `E_RECOVERY_BLOCKING` | 默认 CI |
| Clone split target | separated clone with target control and payload roots | target roots missing 时由 JVS 创建，empty dir 可用；新 repo_id；main only；target payload 无 control；doctor strict passed | 默认 CI |
| Clone target same path | target `C == P` | clone fail closed before publish；`E_CONTROL_PAYLOAD_OVERLAP` | 默认 CI |
| Clone target payload inside control | target `P=C/payload` | clone fail closed before publish；`E_PAYLOAD_INSIDE_CONTROL` | 默认 CI |
| Clone target control inside payload | target `C=P/.control` | clone fail closed before publish；`E_CONTROL_INSIDE_PAYLOAD` | 默认 CI |
| Clone target occupied | target control/payload root non-empty 或需 merge/overwrite/adopt | `E_TARGET_ROOT_OCCUPIED`；target not published | 默认 CI |
| Clone source dirty | source workspace dirty 或 writer race fake | `E_SOURCE_DIRTY`；target not published | 默认 CI |
| Clone runtime excluded | source 有 locks/plans/views/tmp | target 不复制 runtime state；doctor strict passed | 默认 CI |
| Clone transfer inheritance | separated clone fake fast/fallback/capacity cases | JSON `data.transfers[]` 继承 docs/24；capacity gate 覆盖 fallback；run re-probe | 默认 CI |
| Clone atomic publish blocked | staging 后 final target 被创建或 no-replace rename 失败 | `E_ATOMIC_PUBLISH_BLOCKED`；source unchanged；final target 不是半成品 active repo | 默认 CI |
| Clone all protection gate | `--save-points all` without docs/24 imported protection | `E_IMPORTED_HISTORY_PROTECTION_MISSING`；提示 main-only 或升级 | 默认 CI |
| Doctor JSON | `doctor --strict --json` | 包含 authoritative `data.workspace_name`、`data.control_root`、`data.payload_root`、`data.repo_mode`；`checks[]` 每项含 `name`、`status`、`error_code`、`message`；必要 check names 全部出现 | 默认 CI JSON golden |
| Doctor strict failure goldens | root_overlap、payload_locator、permissions、active_operation、recovery_state failed fixtures | failed `checks[].name` 与稳定 `error_code` 映射匹配本文 Doctor JSON Check 合同 | 默认 CI JSON golden |
| Lifecycle unsupported | separated repo 调用 move/rename/delete/detach | `E_SEPARATED_LIFECYCLE_UNSUPPORTED`；No files changed | 默认 CI |
| Compatibility embedded repo | 普通 `.jvs` inside folder repo 的 init/status/save | 现有行为不回归；不要求 explicit control root | 默认 CI |
| AFSCP layout smoke | `/afscp/.../control` + `/afscp/.../payload` fixture | 同通用 separated contract；不使用 AFSCP 专有语义 | 后续 gated profile |
| Real WebDAV/export | payload 通过平台 export，control 不暴露 | JVS runner 操作 control；workload 看不到 control | 后续平台 gate |

## Human Output 草案

普通 human output 不需要暴露太多内部结构，但必须让平台 operator 看清边界。

Init 成功：

```text
Initialized JVS repo
Mode: separated control
Control root: /trusted/repo/control
Workspace: main
Payload root: /workload/repo/payload
Repo ID: repo-...
Boundary validated: yes
Doctor strict: passed
```

Explicit target 缺失：

```text
Cannot choose a separated JVS repo from the current folder.
Pass the control root and workspace explicitly.

Recommended:
jvs --control-root /trusted/repo/control --workspace main status --json

No files were changed.
```

Lifecycle unsupported：

```text
This lifecycle command is not supported for separated-control repos yet.
Repo ID: repo-...
Control root: /trusted/repo/control
Workspace: main
Payload root: /workload/repo/payload
No files were changed.
```

## 工程设计原则

- 先让 target resolution 返回一个完整 `RepoContext`，包含 repo_id、repo_mode、control_root、workspace_name、payload_root、boundary validation evidence。
- 所有命令都消费同一个 `RepoContext`，不要每个命令自己猜 control/payload。
- authority 只来自 control root 和显式 target，不来自 payload locator。
- 路径校验必须是 physical identity 校验，不只是字符串前缀。
- 输出 JSON 和 doctor check 使用同一份 boundary validation 结果。
- mutation 前统一 revalidate，不把 preview/status 结果当 run 许可。
- separated mode 的 fail closed 分支要先于旧 embedded discovery/lifecycle fallback。
- 普通 embedded repo 的代码路径要有 compatibility tests，避免把新约束误施加给普通用户。

## 风险与缓解

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| `--repo` 语义含糊 | 平台可能把 payload root 当 control root，或受 CWD 影响 | Phase 1 conformance selector 固定为 `--control-root <path> --workspace <name>`；`--repo` 只能作为未来/兼容扩展且只能指 control root |
| payload `.jvs` 被误信任 | 不可信 workload 可劫持 repo identity 或 workspace | default strict separated profile 以 `E_PAYLOAD_LOCATOR_PRESENT` fail closed；JSON 固定 `locator_authoritative=false` |
| target root 被误 adopt | init/clone 把既有数据或另一个 repo 合并进 separated repo | Phase 1 只接受 missing/empty target roots；occupied 以 `E_TARGET_ROOT_OCCUPIED` fail closed |
| 只做字符串路径判断 | symlink、bind mount、case drift 可逃逸 | canonical absolute path + physical identity，mutation 前重验 |
| restore 写穿到 control | control metadata 被 payload symlink 覆盖 | restore/recovery no-follow boundary，escape fail closed |
| cleanup 删除 root | 工作区或 control runtime 被误删 | cleanup 只处理 reviewed unprotected save point objects，不删除 roots |
| 旧 lifecycle 半支持 | 只移动 control 或只移动 payload，repo 进入坏状态 | Phase 1 stable unsupported fail closed |
| clone 复制 runtime | target 带 source locks/plans/recovery | 明确 runtime state excluded，并由 doctor strict 验收 |
| transfer/capacity 只按 embedded repo 估算 | separated save/view/restore/clone 在 fallback 或跨边界时写入前失败太晚 | 继承 docs/23 `data.transfers[]`、capacity gate、preview/run re-probe 和 fallback reporting |
| `--save-points all` 被 cleanup 删除 imported history | clone 承诺的历史丢失 | 遵守 docs/24 imported protection gate，Phase 1C 可先 main-only |

## Handoff Checklist

开发开始前必须先写红的测试类型：

- [ ] path invariant unit tests：same path、payload inside control、control inside payload、symlink escape、path drift、permission denied。
- [ ] explicit targeting tests：clean CWD、另一个 repo CWD、missing explicit target、固定 `--control-root <path> --workspace <name>` selector。
- [ ] JSON golden tests：success、doctor success、boundary failure；普通错误响应固定 `ok:false`、`data:null`、`error.code`、`error.message`、optional `error.hint`；authoritative fields 固定为 `data.control_root`、`data.payload_root`、`data.repo_mode`、`data.workspace_name`。
- [ ] doctor JSON tests：`checks[]` 每项含 `name`、`status`、`error_code`、`message`，且出现 root_overlap、payload_locator、repo_identity、workspace_binding、path_boundary、permissions、active_operation、recovery_state。
- [ ] doctor strict failure golden tests：failed `checks[].name` 与稳定 `error_code` 映射，至少覆盖 root_overlap、payload_locator、permissions、active_operation、recovery_state。
- [ ] payload marker tests：payload root-level `.jvs` absent，以及 `.jvs` file、directory、symlink present；present 时 `E_PAYLOAD_LOCATOR_PRESENT`，`locator_authoritative=false`。
- [ ] init/clone root occupancy tests：missing roots、empty dirs、non-empty occupied roots、existing control/data adopt blocked。
- [ ] command integration tests：init、status、doctor、save、history、view、restore preview/run、recovery blocking、cleanup boundary。
- [ ] transfer inheritance tests：separated save/view/restore/clone 的 `data.transfers[]`、capacity gate、preview/run re-probe、fallback success/failure reporting。
- [ ] repo clone tests：split target roots、新 repo_id、main-only、runtime excluded、dirty source、target overlap、target occupied、atomic publish blocked、all-protection gate。
- [ ] lifecycle unsupported tests：separated repo 上 move/rename/delete/detach 全部 stable fail closed。
- [ ] embedded compatibility tests：普通 `.jvs` inside folder repo 的现有 init/status/save/doctor 不回归。

Phase 1A 完成前：

- [ ] separated init 不在 payload root 创建 `.jvs`。
- [ ] separated init 只接受 missing 或 empty target roots；non-empty/adopt existing data/control 以 `E_TARGET_ROOT_OCCUPIED` fail closed。
- [ ] explicit target resolution 生成包含 repo_id、control_root、workspace_name、payload_root 的 validated context。
- [ ] doctor strict 覆盖 root invariant 和 identity mismatch。
- [ ] JSON 输出包含 `repo_mode=separated_control`、`separated_control=true`、`boundary_validated`、`locator_authoritative=false`，且 authoritative fields 在 `data.*`。

Phase 1B 完成前：

- [ ] save 只 walk payload managed files，control/runtime/locks/plans/audit 不进入 save point。
- [ ] view 不展示 control metadata。
- [ ] restore/recovery 不跟随 symlink escape，不覆盖 control。
- [ ] separated save/view/restore 继承 docs/23 transfer planning、capacity gate、preview/run re-probe 和 fallback reporting。
- [ ] cleanup 不删除 workspace/payload/control/runtime/locks/plans/audit roots。
- [ ] 所有 mutation 前重新校验 boundary 和 active operation/recovery。

Phase 1C 完成前：

- [ ] repo clone 需要显式 target control root + target payload root；template workflow 只作为从 template repo/source repo 执行 repo clone。
- [ ] target 生成新 repo identity，只创建 main workspace。
- [ ] source runtime/locks/plans 不复制。
- [ ] dirty source、writer race、target root overlap、target root occupied 和 atomic publish blocked 都 fail closed before publish，并使用稳定错误码。
- [ ] separated repo clone 继承 docs/24 transfer/capacity/imported-history protection 规则。
- [ ] `--save-points all` 只有在 docs/24 imported protection 满足时才可通过，否则 `E_IMPORTED_HISTORY_PROTECTION_MISSING`。

Phase 2 前置：

- [ ] migration/lifecycle 有独立状态更新矩阵和 recovery journal 设计。
- [ ] separated lifecycle 不再依赖 Phase 1 unsupported fallback。
- [ ] locator/shortcut 的 human discovery 和 repair 范围重新评审。

## 关键产品结论

控制面分离式 repo 是 JVS 应该拥有的通用能力。它让平台集成可以安全暴露 workspace payload，而不把 JVS control metadata 放进 workload 可写目录树。

普通本地 `.jvs` inside folder repo 继续存在；separated repo 则明确改变产品心智：repo identity 位于 control root，workspace payload 位于 payload root，repo 不再天然等于一个 folder。这个边界必须由 JVS 原生支持并由 TDD 验收，而不是让每个集成方在 mount、WebDAV 或 sandbox 层重复补丁。
