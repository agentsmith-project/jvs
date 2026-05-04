# Smart Copy Boundaries

**Subtitle:** filesystem-aware transfer planning handoff.

**Status:** implemented design record for current transfer reporting plus later
filesystem-aware copy refinements; active clean redesign, non-release-facing,
not part of the v0 public contract. Remaining gated filesystem work is later
work, 非当前 GA blocker。

**中文名:** 智能复制边界。

本文面向产品、工程和 QA，说明 JVS transfer reporting 的设计原因，以及后续
如何在每次真正需要 materialize/copy 文件时，按本次 source path +
materialization destination 的真实边界选择复制方式。它不是公开用户手册。
已进入 release-facing docs 的字段和文案以 `docs/02_CLI_SPEC.md` 和
`docs/user/` 为准；本文用于解释当前模型和 later copy-planning refinements。
长期心智应从“按 repo root 判断 engine”转为“按这一次文件从哪里写到哪里判断
能不能快”。旧的 repo-root-only 判断最多作为过渡期 default/requested hint，
不应保留为长期产品口径。

Current public CLI contract covers `data.transfers[]` for save, restore
preview/run, workspace new, view, and repo clone. Other commands are either
non-copy surfaces today or remain later copy-planning refinements; do not read
this design record as a blanket promise that every JSON command has transfers.

## 文档验收标准

这份 handoff 应满足：

- 产品能直接拿它解释用户心智：JVS 会检查本次两个位置，再选择 fast copy 或 normal copy；速度可能因位置变化，安全性不变。
- 工程能直接拿它拆阶段：先稳定 intent/result contract，再接 save + workspace new，再接 restore preview/run + view，最后对齐 recovery copy points 和 capacity gate。
- QA 能直接拿它抽测试矩阵：覆盖优化不可用、pair 不支持、fake fast-path 成功、fallback 成功/失败、跨设备容量、preview/run 变化、命令级集成、真实 gated profile。
- 文档明确哪些 transfer reporting 已进入当前合同，哪些 gated filesystem
  optimization 仍是 later work；不把远程同步、公开手动 engine 选择、特定文件系统认证矩阵扩进范围。
- 文档不把 btrfs、zfs、APFS、JuiceFS 等品牌名变成普通用户承诺；承诺落在能力探测、运行时 fallback 和清楚输出。
- 文档不把 recovery backup 的 rename/ledger 安全模型改写成普通 copy transfer。

## 一句话目标

当 `save`、`view`、`restore`、`workspace new` 等流程需要把文件从 A materialize/copy 到 B 时，JVS 先检查这一次真实 source path 和 materialization destination 能否一起使用优化复制；能用就用，不能用或运行中失败就安全降级到普通复制，并把原因告诉用户和 JSON 消费方。Recovery 只在 resume/rollback 或 backup restore 中真正需要 materialize/copy 的点接入 transfer planning；纯 backup safety boundary 只记录安全边界，不输出 fast/normal copy 结论。

## 用户心智

普通用户不需要知道文件系统品牌或内部 engine 名字。他们需要知道：

- JVS 会针对本次操作检查两个位置。
- 如果二者能一起使用快路径，输出 `Copy method: fast copy`。
- 如果不能，输出 `Copy method: normal copy`。
- 当速度变慢时，安全性不降低；只是本次位置组合无法使用快路径。
- 同一个 folder 在不同目标位置、不同挂载点、外部 workspace 路径下，复制速度可能不同。
- `Checked for this operation` 表示 JVS 检查的是这一次操作，而不是泛泛检查了整个 repo 或机器。

建议普通话术：

```text
Copy method: fast copy
Checked for this operation
```

```text
Copy method: normal copy
Why: these two locations cannot use fast copy together
Checked for this operation
```

高级说明可以提到“某些文件系统或挂载存储支持 copy-on-write 或 metadata clone”，但不要把具体品牌写成承诺。即使高级输出展示 `effective_engine`，普通输出仍以 fast copy / normal copy 为主。

## 非目标

本里程碑不做：

- 不新增公开 engine override flag，也不要求用户手动选择 `juicefs-clone`、`reflink-copy` 或 `copy`。
- 不设计远程备份、远程 sync、push/pull、跨机器复制协议。
- 不建立特定文件系统认证矩阵；真实环境测试可以存在，但不是用户承诺。
- 不改变 save point、restore、cleanup、recovery 的产品语义。
- 不重写 restore recovery backup 的 sibling rename ledger 安全模型。
- 不承诺 xattr、ACL、hardlink 的完整保真；这些仍应按现有 metadata preservation 口径演进。
- 不把快路径失败当成操作失败，除非普通复制 fallback 本身也失败或安全检查不通过。
- 不为了性能绕过 preview/run revalidation、capacity gate、recovery backup、checksum 或审计。

## 用户故事

### Save

用户在一个 workspace 中运行 `jvs save -m "baseline"`。JVS 应按 workspace managed files 到 save point staging content path 的真实路径规划复制方式。若 workspace 和 JVS storage 位于不同挂载点，不能因为 repo root 看起来支持某种 engine 就误判本次 save 可以快。

边界：

- `materialization_destination`: 实际写入或 clone 的 save point staging content path。
- `capability_probe_path`: 已存在的 staging parent 或 storage parent，用于安全探测 destination 能力。
- `published_destination`: 用户在 history 中看到的 save point 语义位置，而不是 probe 的唯一依据。

期望：

- 成功保存的语义不变。
- 输出说明本次 copy method。
- save descriptor 中记录最终实际使用的 engine、是否优化、降级原因和警告，或记录可派生的 transfer result。

### Workspace New

用户运行 `jvs workspace new ../experiment --from <save>`，目标 folder 可能在 repo 外部、另一个磁盘、另一个挂载点或同一挂载点。JVS 应按 save point content 到新 workspace materialization path 的路径组合规划复制方式。

边界：

- `materialization_destination`: 实际创建并填充 workspace 内容的 staging/tmp path；如果实现直接写最终 folder，则它可以等于 final folder。
- `capability_probe_path`: 创建前已经存在的 explicit target parent，或实现用于 workspace staging 的已存在 parent。
- `published_destination`: 用户请求的新 workspace folder。

期望：

- 显式目标路径语义不变。
- 外部 workspace 不继承 repo root 的能力判断。
- 如果目标位置不能 fast copy，普通复制成功即可。
- 不为 workspace new 新增 save descriptor；需要记录时使用 CLI JSON/human、audit 或 workspace metadata。

### Restore Preview And Run

用户先 preview，再 run。Preview 应显示当时预计的 copy method，但 run 必须重新探测并重新校验，不能信任 preview 时的能力结果。

Restore preview 不是“完全不写入”。正确边界是：

- Preview 不写用户 workspace。
- Preview 不提交 restore/recovery 状态变更。
- Preview 可能为了 source validation 或 impact evidence 做内部临时 materialization。
- 这类内部 preview materialization 可以接 planner，但必须标记为内部 transfer，例如 `phase=preview_validation`、`result_kind=expected`、`permission_scope=preview_only`。
- 普通输出可以显示 expected copy method，但不能把 preview 结果描述成 run 许可。

Restore run 边界：

- `materialization_destination`: restore run 实际写入的 staging/tmp content path，或实现中真正 copy 到的 workspace target path。
- `capability_probe_path`: run 时存在的 staging parent、target parent 或受控临时探测目录。
- `published_destination`: 用户看到的 workspace restore 后语义位置。

期望：

- Preview 只承诺“根据当前检查预计如此”，不承诺 run 时一定相同。
- Run 输出最终实际 copy method。
- 如果 preview 显示 fast copy，但 run 时 pair 不支持或快路径失败，run 可以安全降级到 normal copy，并清楚记录原因。
- 如果 fallback 需要更多容量，run 必须在写入前做容量安全检查。

### View

用户运行 `jvs view <save>` 或 `jvs view <save> <path>`。JVS 创建只读 view 时，应按 save point content/path 到 view materialization path 的真实组合规划。不要因为 repo root 支持快路径，就假设临时 view 目录也支持。

边界：

- `materialization_destination`: 实际 view materialization path，包括临时目录、cache 目录或只读 view staging。
- `capability_probe_path`: view materialization parent 或受控 cache parent。
- `published_destination`: 用户拿到的 view path 或 view handle。

期望：

- View 的只读语义不变。
- 输出和 JSON 可说明本次 copy method。
- 临时目录、外部 cache 或不同挂载点都按本次 path pair 判断。

### Recovery Backup And Recovery Copy Points

Restore run 创建 recovery backup 的主安全模型仍是 sibling rename ledger。这个 backup creation 不是本里程碑要优化的普通 copy transfer，也不能为了快路径改写为 copy-first 或 clone-first。

正确边界：

- Restore run 的 backup creation 保留 rename/ledger 安全模型。
- 如果 backup creation 没有 copy/materialization 点，则只记录 backup safety boundary，不输出 `Copy method: fast copy` 或 `Copy method: normal copy`。
- Transfer planning 只作用于真正需要 materialize/copy 的 recovery resume、recovery rollback 或 backup restore copy 点。
- 如果 recovery resume/rollback 需要把 backup 内容 copy 回 workspace，或需要从 backup materialize 到 staging，再按对应真实 source path + materialization destination 规划。
- 不允许为快路径重写 recovery safety model。

期望：

- recovery 安全语义优先，性能优化只能附着在已有安全 copy 点上。
- 有 copy 点时，copy fallback 也必须纳入容量检查。
- 无 copy 点时，audit/recovery plan 记录 safety boundary、ledger path、protected targets，不生成 transfer result。

## 当前 Gap

当前代码已经有可复用雏形：

- `TransferPlanner`
- `CapabilityReport`
- `TransferPairReport`
- `PlanTransfer`
- descriptor 字段 `ActualEngine`、`EffectiveEngine`、`DegradedReasons`

但主流程仍有明显缺口：

- 多数生命周期路径仍以 repo root 级 `detectEngine(repoRoot)` 或类似判断作为 engine 来源。
- destination capability 和 source/destination pair capability 还没有稳定接到所有真实搬运点。
- `detectEngine(repoRoot)` 容易在跨挂载点、外部 workspace、临时 view path、restore staging path 上给出错误性能预期。
- 开发若只拿最终 folder 做 pair probe，容易忽略真正写入的是 staging/tmp content path，也容易在 final folder 不存在时做错误判断。
- descriptor 已能承载实际/有效 engine 和降级原因，但后续应来自 transfer plan + runtime fallback 的合并结果，而不是只来自初始 engine 参数。
- Preview 能力结果不能被 run 信任；run 阶段必须重新探测。
- recovery backup creation 的 rename/ledger 不能被误建模成普通 copy transfer。
- 用户常用输出需要说清“这次为什么快/为什么普通复制”，不能只藏在 descriptor 或 JSON。

这个 gap 不要求推翻现有 engine；它要求把 engine 决策从“repo 级默认”下沉到“每一次真实 materialization/copy 的 source/destination pair”。

## 设计原则

- 每次 materialization/copy 都有明确 source role、source path、destination role、materialization destination、capability probe path、published destination。
- 优化能力必须按本次 pair 判断；destination-only 检查只能作为候选，不是最终许可。
- `published_destination` 不能默认作为 pair probe 目标；只有它同时就是实际 materialization destination 且探测安全时，才能作为 pair probe 输入。
- 所有优化类要么有 pair probe，要么有清晰 runtime fallback 到 copy。
- `detectEngine(repoRoot)` 最多作为 requested/default，不直接决定跨路径 transfer。
- Run 阶段重新探测，preview 结果只作为用户预期和计划记录。
- Capacity gate 按最终可能写入路径检查，尤其要覆盖 fallback 到普通复制的最坏情况。
- 普通输出给心智，JSON 给自动化，descriptor/audit/plan 给审计和后续诊断；各字段含义一致，但不是每个流程都有 descriptor。
- 快路径是性能优化，不是安全前提。
- Recovery safety model 不能为性能优化让路。

## 搬运意图模型

建议所有 materialization/copy 入口先构造一个 transfer intent。字段名不必完全照抄，但信息必须存在：

| 字段 | 含义 |
| --- | --- |
| `transfer_id` | 命令内稳定 ID，例如 `save-primary`、`restore-preview-validation-1` |
| `operation` | `save`、`view`、`restore`、`workspace_new`、`recovery_resume`、`recovery_rollback` 等 |
| `phase` | `preview_validation`、`materialization`、`backup_restore`、`view_materialization`、`recovery_resume`、`recovery_rollback` 等 |
| `primary` | 是否为普通 human output 默认展示的主 transfer |
| `source_role` | `workspace_content`、`save_point_content`、`restore_target`、`recovery_backup_content` 等 |
| `source_path` | 本次真实 source path；公开 JSON 使用可公开 ref/display path，例如 `save_point:<id>`、`content_view:<id>/path` 或 `control_data` |
| `destination_role` | `save_point_content`、`workspace_folder`、`temporary_folder`、`content_view`、`backup_restore_target` 等 |
| `materialization_destination` | 实际写入、clone 或普通复制的 staging/tmp content path；没有 copy 点时应为空或不存在 |
| `capability_probe_path` | 用于探测能力的已存在目录或 parent；probe 应在其下使用受控临时 path |
| `published_destination` | 用户看到的最终目标、handle 或 publish 后语义位置 |
| `requested_engine` | 来自 config/default 的偏好；`auto` 表示由 planner 选择 |
| `safety_context` | 是否需要 capacity gate、recovery plan、read-only view、preview evidence、audit 等 |
| `result_kind` | `expected` 用于 preview 或 dry planning；`final` 用于实际 run result |
| `permission_scope` | `preview_only`、`execution` 或内部等价值，防止 preview 结果被当作 run 许可 |

三种 destination path 的职责必须分清：

- `materialization_destination`: 真正写入或 clone 的路径，是 copy engine 和 runtime fallback 最关心的路径。
- `capability_probe_path`: 探测能力的安全锚点，通常是已存在目录或 parent。destination 不存在时，不要把 final folder 硬传给 pair prober；应在这个 parent 下使用受控临时 probe path，或使用 planner 支持的 parent-aware probe。
- `published_destination`: 用户理解的最终目标。它可以用于输出、JSON display、audit 语义定位，但不能单独授权 fast path。

路径应在规划前做 canonical/absolute 解析，并使用现有 workspace/repo 安全边界校验。对用户不应展示内部控制路径时，可以在 JSON 中保留 role 和公开路径，同时在 audit/plan 中记录工程可诊断的内部 path。

## Transfer Planning 流程

建议每次 materialization/copy 使用同一套流程：

1. 构造 transfer intent，明确三种 destination path。
2. 如果没有 `materialization_destination`，说明本流程只有 safety boundary 或 ledger 操作；记录 boundary，不生成 fast/normal copy result。
3. 对 `capability_probe_path` 做写能力和基础 copy 能力检查。
4. 选择候选 optimized engine。
5. 对需要 pair 支持的优化做 source path + materialization boundary 的 pair probe；如果 destination 尚不存在，使用 `capability_probe_path` 下的受控临时 probe，不直接拿 `published_destination` 冒充真实写入路径。
6. 生成 transfer plan：`effective_engine`、`optimized_transfer`、`transfer.performance_class`、`degraded_reasons`、`warnings`。
7. 在真正写入前执行 capacity gate，按 fallback 最坏情况预留空间。
8. 运行 materialization。
9. 如果优化运行时失败且 fallback 安全，降级普通复制；若 fallback 不安全或容量不足，则失败并保留恢复/诊断信息。
10. 合并 plan 结果和 runtime fallback 结果，按流程落点写入 CLI human、CLI JSON、descriptor、audit、restore plan、recovery plan 或 metadata。

`TransferPlanner` 可以继续作为中心组件，但它需要被主流程调用，而不是只作为孤立能力报告。工程上应避免每个命令各自发明一套探测和降级文案。

## 各流程接入顺序

### Phase 0: Contract Scaffolding

目标是先稳定测试替身和输出合同。

- 定义 transfer intent/plan/result 的最小结构。
- 统一 `data.transfers[]` JSON 输出模型。
- 让 fake prober、fake pair prober、fake capacity meter、fake materializer 可以表达能力变化、fake fast-path success 和 fallback。
- 明确 `transfer.performance_class` 值，例如 `fast_copy`、`normal_copy`。
- 明确 human UI、JSON 字段和每个命令的落点。
- 不改变用户语义，不新增公开 flag。

### Phase 1: Save + Workspace New

优先接入 save 和 workspace new，因为它们最容易遇到外部目标路径和跨挂载点误判。

- Save: workspace content -> save point staging content path。
- Workspace new: save point content -> workspace materialization path，published destination 是 explicit target folder。
- `detectEngine(repoRoot)` 降为 requested/default。
- Save descriptor 的 `ActualEngine`、`EffectiveEngine`、`DegradedReasons` 来自 transfer plan + runtime fallback，或从独立 transfer result 派生。
- Workspace new 不新增 save descriptor；使用 CLI JSON/human、audit/workspace metadata if appropriate。
- 输出显示本次 copy method 和 why。

### Phase 2: Restore Preview/Run + View

接入 restore 和 view，并明确 preview/run 分离。

- Restore preview: 计算预计 transfer plan；不写用户 workspace，不提交状态变更；若内部临时 materialization 用于 validation/evidence，则作为 `preview_validation` 内部 transfer 规划。
- Restore preview 的 ordinary output 可显示 `Expected copy method`，但 JSON 必须表达 `result_kind=expected` 或等价语义，不给 run 许可。
- Restore run: 重新探测 source/destination pair，重新 capacity gate，输出最终实际结果。
- View: save point content/path -> view materialization path，按实际 view destination 规划。
- 如果 preview 和 run 能力不同，run 输出和 JSON 必须体现最终结果和 warning。

### Phase 3: Recovery Copy Points + Capacity Gate

把 recovery 安全边界、真实 copy 点和容量检查对齐。

- Restore run 的 backup creation 保留 sibling rename ledger；不是普通 copy transfer，不作为本里程碑优化对象。
- Recovery resume/rollback 或 backup restore 中真正需要 materialize/copy 的点，才接 transfer planning。
- 没有 copy 点时，只记录 backup safety boundary，不输出 fast/normal copy。
- Capacity gate 按本次 destination、同设备/跨设备、fallback copy 的最坏情况计算。
- 如果 fast copy 预计省空间，但 fallback copy 可能需要完整逻辑字节，run 前必须能识别容量不足并失败在写入前。
- copy fallback 成功和失败都要有清楚 audit/warning。

### Phase 4: Docs, Evidence, And Release Polishing

这是发布前整理，不是先行承诺。

- 更新 release-facing performance wording，只说“按本次操作检查，可能快也可能普通复制”。
- 确保普通用户 docs 不出现品牌承诺。
- 更新 conformance 和 release evidence。
- 真实 JuiceFS、真实跨设备 profile 作为显式 gated evidence，不进入默认 CI 依赖。

## UX 输出要求

普通输出必须在常用命令路径中可见，不能只在 verbose 或 JSON 里出现。

实际运行成功时建议：

```text
Copy method: fast copy
Checked for this operation
```

```text
Copy method: normal copy
Why: these two locations cannot use fast copy together
Checked for this operation
```

当降级来自运行时 fallback，可用：

```text
Copy method: normal copy
Why: fast copy failed during this operation; JVS safely used normal copy
Checked for this operation
```

Restore preview 可用：

```text
Expected copy method: fast copy
Checked for this preview
```

如果有 secondary transfer，human output 默认展示 primary transfer；必要时追加短 summary，例如：

```text
Additional transfers: 1 internal validation; see JSON for details
```

输出原则：

- 普通输出用 `fast copy` / `normal copy`，避免要求用户理解 engine 名字。
- `Why:` 只讲本次操作，不讲笼统机器能力。
- 如果只做了 preview，应避免“will use”这种强承诺，使用 `expected` 或 `previewed` 语气。
- Run 输出必须表达最终实际结果。
- 如果有警告，优先解释对用户行动是否有影响。
- 无 copy 点的 recovery safety boundary 不显示 `Copy method`。

## JSON 输出要求

产品选择：所有相关 command 统一使用 `data.transfers[]` 作为 canonical 输出，即使只有一个 primary transfer。不要为新 surface 同时引入 `data.transfer` 和 `data.transfers[]` 两套同级合同；若已有兼容层需要 `data.transfer`，只能临时镜像 primary transfer，并把 `data.transfers[]` 作为新合同和测试目标。

每条 transfer 建议字段：

- `transfer_id`
- `operation`
- `phase`
- `primary`
- `result_kind`
- `permission_scope`
- `source_role`
- `source_path` 或可公开的 source display path
- `destination_role`
- `materialization_destination`
- `capability_probe_path`
- `published_destination`
- `checked_for_this_operation`
- `requested_engine`
- `effective_engine`
- `optimized_transfer`
- `performance_class`
- `degraded_reasons`
- `warnings`

公开 JSON 中的 JVS-owned source/materialization paths 使用 `save_point:<id>`、
`content_view:<id>[/path]`、`control_data` 或 `temporary_folder` 等稳定 ref，
不暴露内部控制目录布局。示例：

```json
{
  "data": {
    "transfers": [
      {
        "transfer_id": "workspace-new-primary",
        "operation": "workspace_new",
        "phase": "materialization",
        "primary": true,
        "result_kind": "final",
        "permission_scope": "execution",
        "source_role": "save_point_content",
        "source_path": "save_point:1708300800000-deadbeef",
        "destination_role": "workspace_folder",
        "materialization_destination": "temporary_folder",
        "capability_probe_path": "/abs",
        "published_destination": "/abs/experiment",
        "checked_for_this_operation": true,
        "requested_engine": "auto",
        "effective_engine": "copy",
        "optimized_transfer": false,
        "performance_class": "normal_copy",
        "degraded_reasons": [
          "these two locations cannot use fast copy together"
        ],
        "warnings": []
      }
    ]
  }
}
```

`effective_engine` 仍可使用内部 engine enum，方便自动化和审计；普通输出不必暴露。`optimized_transfer` 是布尔值，表示这次最终是否用了优化路径。`transfer.performance_class` 面向稳定用户心智，建议只表达 `fast_copy` / `normal_copy` 等少量类别。

## Descriptor, Audit, Plan, And Metadata

“同一份 transfer result”指同一处 planner/runtime 合并结果作为源数据，而不是每个流程都写同一种文件。各流程落点必须按命令语义区分：

| 流程 | Transfer result 落点 |
| --- | --- |
| `save` | save descriptor + CLI JSON/human + audit |
| `workspace new` | CLI JSON/human + audit/workspace metadata if appropriate；不要新增 save descriptor |
| `restore preview` | restore plan + CLI JSON/human；不写用户 workspace，不提交状态变更 |
| `restore run` | CLI JSON/human + recovery plan/audit as appropriate；run result 记录最终 transfer |
| `view` | CLI JSON/human + view metadata/audit if existing surface |
| `recovery resume` | recovery plan/audit/CLI result；仅 copy 点生成 transfer result |
| `recovery rollback` | recovery plan/audit/CLI result；仅 copy 点生成 transfer result |

Save point descriptor 已有字段可承载结果：

- `ActualEngine`
- `EffectiveEngine`
- `DegradedReasons`

后续语义建议：

- `ActualEngine`: 运行开始时尝试的实际 materialization engine；若没有优化尝试，可等于 `copy`。
- `EffectiveEngine`: 最终完成写入的 engine；runtime fallback 后应为 `copy`。
- `DegradedReasons`: plan 阶段和 runtime fallback 阶段的去重合并结果。
- `optimized_transfer`: 如 descriptor 需要新增或派生，应表示最终结果，不表示初始候选。
- `warnings`: 保留能力探测、fallback、容量估算相关警告。

### Performance Class 命名边界

`transfer.performance_class` 是 user-facing class，建议值为 `fast_copy` / `normal_copy`。它不等于底层 engine，也不应表达 `juicefs`、`reflink` 等实现细节。

Existing descriptor `PerformanceClass` may retain engine-level values during migration. Do not silently redefine it as `transfer.performance_class` unless a separate descriptor schema migration/update is planned and covered by tests.

工程 handoff 应二选一：

- 新增或派生独立的 transfer class 字段，用于 CLI/JSON/audit 和未来 descriptor。
- 或显式更新 descriptor schema/tests，把现有 `PerformanceClass` 从 engine-level 语义迁到 user-facing transfer class。

Audit record 应能回答：“这次从哪里到哪里，先计划了什么，最后用了什么，为什么降级，是否 fallback 成功。” Recovery audit 还应能回答：“backup safety boundary 是 rename/ledger，还是存在真实 copy point；如果没有 copy point，为什么没有 transfer result。”

## Consistency Invariants

这些 invariant 是 QA 和 review 的验收线：

| Surface | 必须一致的内容 | 不适用或例外 |
| --- | --- | --- |
| Human output | primary transfer 的 copy method、why、checked scope；preview 使用 expected 语气 | secondary transfer 只需 summary，详细看 JSON |
| CLI JSON `data.transfers[]` | 所有 transfer 的 ID、phase、roles、三种 destination path、optimized flag、performance class、degraded reasons、warnings | 内部敏感 path 可用 display path 加 audit 诊断 path |
| Save descriptor | save primary final transfer 的 engine/degraded/warning 派生值 | 只有 `save` 写 save descriptor |
| Audit | planner input、runtime result、fallback、capacity gate、safety boundary | 如果命令没有 audit surface，可先落到对应 plan/metadata |
| Restore plan | preview expected transfer、run 前 revalidation 需求、impact evidence source | preview 不写用户 workspace，不提交状态 |
| Recovery plan | ledger/safety boundary、copy points、resume/rollback final transfer | rename-only backup boundary 不生成 fast/normal copy |
| Workspace/view metadata | workspace/view materialization result if appropriate | 不创建 save descriptor |

按命令的 descriptor 边界：

| 操作 | 是否写 save descriptor | Transfer result 一致性要求 |
| --- | --- | --- |
| `save` | 是 | descriptor、human、JSON、audit 来自同一 final primary transfer result |
| `workspace new` | 否 | human、JSON、audit/workspace metadata 一致 |
| `restore preview` | 否 | restore plan 与 JSON/human 的 expected transfer 一致；不得给 run 许可 |
| `restore run` | 否 | final run transfer 与 recovery plan/audit/JSON/human 一致 |
| `view` | 否 | view metadata/audit if existing surface 与 JSON/human 一致 |
| `recovery resume` | 否 | recovery plan/audit/CLI result 一致；无 copy 点则只有 safety boundary |
| `recovery rollback` | 否 | recovery plan/audit/CLI result 一致；无 copy 点则只有 safety boundary |

## Capacity Gate 要求

Capacity 不能只看候选 fast path。对于每次 materialization/copy：

- 如果 plan 最终是 normal copy，按普通复制需要的字节和文件数检查。
- 如果 plan 是 fast copy，但存在 runtime fallback 可能，run 前必须确认 fallback copy 的容量安全，或者明确禁止 fallback 并在快路径失败时失败在写入前。
- 跨设备、跨挂载点、外部 workspace path 要按 destination 所在设备/挂载点计算。
- Restore materialization、recovery resume/rollback copy 点、backup restore copy 点都要纳入容量检查。
- Rename-only recovery backup safety boundary 不按普通 copy transfer 计算，但它的 ledger/rename 前置安全条件仍必须由 recovery 模块校验。
- Preview 的容量提示只是当时估计；run 必须重新检查。

默认 CI 可通过 fake capacity meter 覆盖这些分支。真实跨设备测试放显式 gated profile。

## 默认 CI 测试计划

默认 CI 应使用 fake prober、fake pair prober、fake capacity meter 和 fake materializer。真实 JuiceFS、真实 reflink、真实跨设备测试应放到显式 gated profile，避免把开发机环境变成默认测试前提。

| 场景 | 期望 | 默认 CI |
| --- | --- | --- |
| Fake fast-path success | human 显示 `Copy method: fast copy`；JSON `optimized_transfer=true`；`transfer.performance_class=fast_copy` | fake prober + fake materializer |
| Optimized clone unavailable | planner 不承诺快路径，降级 normal copy，输出 why | fake prober |
| Optimized clone runtime fail | runtime fallback 到 copy，结果记录 warning/degraded reason | fake materializer |
| Metadata clone unavailable | `effective_engine=copy`，`optimized_transfer=false` | fake prober |
| Destination 支持但 pair 不支持 | pair probe 降级 copy，输出 `these two locations cannot use fast copy together` | fake pair prober |
| Copy fallback 成功 | 操作成功，descriptor/audit 合并 plan + runtime fallback | fake materializer |
| Copy fallback 失败 | 操作失败，保留 recovery/diagnostic 信息，不伪装成功 | fake materializer |
| 跨设备 capacity | 按 destination 所在设备检查；容量不足时写入前失败 | fake meter |
| Preview/run 能力变化 | preview 可显示 expected fast，run 重新探测后可 normal copy | fake prober sequence |
| Fallback copy 容量不足 | fast candidate 不足以放行；fallback 容量不足时写入前失败 | fake meter |
| 外部 workspace path | 不继承 repo root 能力，按目标 materialization boundary 规划 | fake paths |
| View 临时目录不同挂载点 | view 按实际 view materialization path 规划 | fake paths |
| Recovery backup safety boundary | rename-only backup 不生成 fast/normal copy result，只记录 safety boundary | fake recovery ledger |
| Recovery resume/rollback copy point | 真正 copy 点有 transfer result 和容量检查 | fake recovery materializer |

### Command-Level Integration Matrix

每个入口至少需要一条默认 CI command-level 集成验收。命令名称可按仓库现有测试 harness 调整，但验收语义应稳定。

| 入口 | 默认 CI 集成验收 | 关键断言 |
| --- | --- | --- |
| `save` | fake fast-path success save | `data.transfers[]` 有 primary final transfer；human `fast copy`；save descriptor 派生 `optimized_transfer=true` 或等价字段 |
| `workspace new` | explicit target parent pair unsupported | 不使用 repo root capability；human/JSON 为 normal copy；不写 save descriptor |
| `restore preview` | preview expected fast then no workspace mutation | restore plan + JSON 为 `result_kind=expected`、`permission_scope=preview_only`；不提交状态变更 |
| `restore run` | preview fast, run pair unsupported or runtime fallback | run 重新探测；final result normal copy；capacity gate 覆盖 fallback |
| `view` | view cache/materialization path on different fake mount | 按 view materialization path 规划；human/JSON 一致 |
| `recovery resume` | resume contains a backup restore copy point | copy 点有 transfer result；recovery plan/audit/CLI result 一致 |
| `recovery rollback` | rollback is rename-only or contains copy point | rename-only 只记录 safety boundary；有 copy 点时才输出 transfer result |

## Gated Profile Contract

以下是建议 contract，不要求当前仓库已经存在这些命令。实现时可映射到现有 test runner，但 profile 名称、skip 语义和 evidence 格式应保持稳定。

| Profile | 建议触发命令 | 环境变量/fixture 要求 | Skip 条件 | 失败是否 blocker |
| --- | --- | --- | --- | --- |
| `transfer-real-optimized-clone` | `JVS_GATED_PROFILE=transfer-real-optimized-clone <repo test command>` | `JVS_GATED_TRANSFER_SRC`、`JVS_GATED_TRANSFER_DST_SAME_FS` 指向允许创建/删除 fixture 的真实目录 | env 缺失、目录不可写、平台不支持所需 clone 能力 | 默认 CI 非 blocker；release evidence 选择该 profile 后 fail 是 release blocker |
| `transfer-real-optimized-runtime-fail` | `JVS_GATED_PROFILE=transfer-real-optimized-runtime-fail <repo test command>` | 可注入真实或 shimmed runtime fail 的 fixture，且 fallback copy 可验证 | 无法安全注入 fail、fixture 不可写 | 同上 |
| `transfer-real-cross-device` | `JVS_GATED_PROFILE=transfer-real-cross-device <repo test command>` | `JVS_GATED_TRANSFER_SRC`、`JVS_GATED_TRANSFER_DST_OTHER_DEVICE` 位于不同 device/mount | 无第二 device/mount、权限不足 | 同上 |
| `transfer-real-capacity-boundary` | `JVS_GATED_PROFILE=transfer-real-capacity-boundary <repo test command>` | 可控容量受限 destination，或测试 harness 能模拟真实容量边界 | 无法安全限制容量、风险过高 | 同上 |
| `transfer-real-brand-juicefs` | `JVS_GATED_PROFILE=transfer-real-brand-juicefs <repo test command>` | JuiceFS fixture、mount 信息、clone 能力开关、cleanup 权限 | JuiceFS 未安装/未挂载、fixture 不满足版本/权限 | 仅当 release 明确引用该 evidence 时是 blocker |

Release evidence 建议格式：

- profile name
- command line
- commit SHA
- date/time
- OS/kernel/filesystem or mount summary
- fixture paths redacted or scoped
- scenario list
- pass/fail/skip with reason
- relevant JSON excerpt for one fast-path success and one fallback/degrade case

## 里程碑验收标准

### 产品验收

- 主名统一为 `Smart Copy Boundaries`；描述使用 `filesystem-aware transfer planning handoff`。
- 用户常用输出能看到本次 `Copy method`、`Why`、`Checked for this operation`。
- 普通用户不需要知道具体文件系统品牌就能理解为什么快或普通复制。
- 文档和 help 不承诺某个品牌或 repo root 永远 fast copy。
- Save point、restore、workspace、cleanup、recovery 的语义不因复制方式变化而改变。
- Restore preview 的文案表达 expected，不暗示 run 已获许可。

### 工程验收

- Save 和 workspace new 不再由 repo root detection 直接决定 transfer engine。
- Restore run、view、recovery copy points 都按真实 source/materialization destination pair 规划。
- Intent/result 明确 `materialization_destination`、`capability_probe_path`、`published_destination`。
- Recovery backup creation 保留 sibling rename ledger 安全模型；无 copy 点时只记录 safety boundary。
- Preview 和 run 分别探测；run 不复用 preview 能力结论作为许可。
- 所有优化路径都有 pair probe 或 runtime fallback，并能清楚降级到 copy。
- Descriptor/audit/output/JSON/plan 使用同一份 transfer result 源数据，但按流程落到正确 surface。
- `transfer.performance_class` 不静默重定义现有 descriptor `PerformanceClass`。
- Capacity gate 覆盖 fallback copy 最坏情况。

### QA 验收

- 默认 CI 使用 fake prober/meter/materializer 覆盖测试矩阵，不依赖真实 JuiceFS 或特定文件系统。
- 默认 CI 场景名不使用品牌名作为普通场景，使用 `optimized clone unavailable`、`optimized clone runtime fail` 等泛化命名。
- 默认 CI 覆盖 fake fast-path success，断言 `fast copy`、`optimized_transfer=true`、`transfer.performance_class=fast_copy`。
- 每个命令入口有 command-level integration acceptance，或明确该入口在本里程碑只记录 safety boundary。
- 真实 JuiceFS 和真实跨设备测试有显式 gated profile contract。
- JSON canonical 结构为 `data.transfers[]`，并稳定包含 roles、三种 destination path、`effective_engine`、`optimized_transfer`、`performance_class`、`degraded_reasons`、`warnings`。
- 人类输出和 JSON 对同一次 primary transfer 的结果一致。
- Fallback 成功、fallback 失败、容量不足都能被测试区分。

## 风险与缓解

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| Probe 本身产生副作用 | 用户目录出现临时文件或失败残留 | probe 写入 `capability_probe_path` 下受控临时目录，失败清理并记录 warning |
| Preview 和 run 结果不同 | 用户以为 preview 是强承诺 | 输出使用 expected/checked wording，run 必须重新显示最终结果 |
| 快路径失败后 fallback 容量不足 | restore 或 recovery copy 中途失败 | run 前按 fallback 最坏情况 capacity gate；不足则写入前失败 |
| Recovery backup 被当作普通 copy | 安全模型被性能优化弱化 | rename/ledger backup creation 只记录 safety boundary；copy 点才接 planner |
| 品牌名泄露成承诺 | 用户形成错误性能预期 | 普通输出只用 fast/normal copy，高级说明强调能力探测和安全降级 |
| 每个流程各写一套文案 | UX/JSON 不一致 | 中央 transfer result 渲染 human/JSON/descriptor/plan/audit |
| 外部 workspace 路径误判 | 跨挂载点 workspace new 性能和容量错误 | 所有 workspace new 用 explicit target materialization boundary 规划 |
| 过度扩展到 sync/backup 产品 | 范围蔓延 | 本里程碑只处理本地 materialization/copy |

## Handoff Checklist

开发开始前：

- [ ] 确认 transfer intent 最小字段和 `data.transfers[]` 稳定字段。
- [ ] 确认三种 destination path 的构造方式和 probe 安全边界。
- [ ] 确认 human UI 文案和 preview/run 语气。
- [ ] 确认 fake prober、fake pair prober、fake capacity meter、fake materializer 的测试接口。
- [ ] 确认 `detectEngine(repoRoot)` 只作为 requested/default 的过渡边界。
- [ ] 确认 descriptor `PerformanceClass` 迁移策略：独立派生 transfer class，或显式 schema migration。

Phase 1 完成前：

- [ ] Save 使用真实 source/materialization destination pair 规划。
- [ ] Workspace new 使用 save point content -> workspace materialization boundary 规划。
- [ ] Save descriptor 合并 plan + runtime fallback 结果。
- [ ] Workspace new 不新增 save descriptor。
- [ ] 普通输出和 JSON 显示 copy method、why、checked scope。

Phase 2 完成前：

- [ ] Restore preview 显示预计 transfer plan，但不作为 run 许可。
- [ ] Preview 内部临时 materialization 如存在，标记为内部 transfer。
- [ ] Restore run 重新探测、重新 capacity gate、输出最终结果。
- [ ] View 按实际 view materialization path 规划。
- [ ] Preview/run 能力变化有测试。

Phase 3 完成前：

- [ ] Recovery backup creation 保留 rename/ledger 模型，不改写为 copy transfer。
- [ ] Recovery resume/rollback 或 backup restore copy 点有独立 transfer planning 和输出/审计记录。
- [ ] Rename-only safety boundary 不输出 fast/normal copy。
- [ ] Capacity gate 覆盖 fallback copy 最坏情况。
- [ ] 跨设备、外部 path、fallback 失败都有默认 CI fake 测试。

发布前：

- [ ] Release-facing docs 不再保留 repo-root-only 长期心智。
- [ ] Performance docs 用“本次操作检查”表达，不做品牌承诺。
- [ ] Conformance/release evidence 记录默认 fake 覆盖和 gated real profile 覆盖。
- [ ] QA 确认 human output、JSON、descriptor/audit/plan/recovery metadata 对同一次操作一致。
