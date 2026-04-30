# Workspace Path And History Pointer Semantics

**状态:** 产品行为改进交接稿 / development handoff candidate。

**Release classification:** active clean redesign, non-release-facing, not part of the v0 public contract.

本文定义产品行为和 UX 规则，供开发团队实现前对齐；它不是代码实现方案，也暂不作为 v0 公共契约。

## 背景

当前用户创建第二个 workspace 的典型命令是：

```bash
jvs workspace new experiment --from <save>
```

这里的 `experiment` 对用户来说很像一个 workspace 名字，但它同时会隐式决定真实文件夹路径。这个隐含行为让用户需要额外猜测：

- `experiment` 是名字、路径，还是两者都是？
- 新文件夹会创建在哪里？
- 当前文件夹会不会被污染？
- 以后如何从输出里的 workspace 名字反推真实路径？

即使新 workspace 不会污染 `main` workspace 的 JVS 状态，如果它默认出现在用户父目录或某个推导位置，也可能污染用户的视觉工作环境：文件浏览器、终端 `ls`、编辑器项目列表里会突然多出一个真实目录。

## 当前问题

`jvs workspace new experiment --from <save>` 的问题不是功能能力不足，而是产品语义不够显式：

- 命令参数看起来像“名字”，但行为上又创建真实路径。
- 用户没有在命令里明确说“把新文件夹放到这里”。
- 输出可以告诉用户最终路径，但这是事后解释，不是事前选择。
- 当用户只想临时开一个实验目录时，隐式路径会增加心智负担。

创建真实文件夹时，路径最好由用户明确给出，而不是由 JVS 从一个看似名字的参数推导出来。

## 产品判断

创建真实文件夹时，路径应该显式。

建议把用户心智从“给 workspace 起名，JVS 推导路径”调整为“明确给出目标文件夹路径，JVS 从路径得到默认 workspace 名字”。

建议命令形态：

```bash
jvs workspace new <folder> --from <save>
jvs workspace new <folder> --from <save> --name <name>
```

其中：

- `<folder>` 是用户明确指定的目标路径。
- `<folder>` 不是 workspace name。
- workspace name 默认来自目标文件夹的 basename。
- 可选 `--name <name>` 用于显式覆盖默认 workspace name。
- 如果 basename 不合法，或者推导出的 name 已存在，应报错。
- 如果 `--name <name>` 不合法或已存在，也应报错。
- 不自动改名，不自动追加后缀。
- 当前产品未 GA，倾向干净重构，不保留旧 `workspace new <name>` 的隐式路径行为。

## 术语

- 当前指针：一个 workspace 当前整体内容来源于哪个 save point。
- newest/history head：该 workspace 自己最近一次 `jvs save` 产生的 save point。restore 不会直接改写它。
- source/origin：workspace 被创建时使用的 source save point。
- locator：外部 workspace 根目录里的轻量隐藏标记，用来帮助 JVS 找回所属 repo 和 workspace。
- registry：repo 控制面里记录 workspace 名字、路径和当前指针的权威清单。

## 建议规则

- 相对路径按当前工作目录 `cwd` 解析。
- 创建成功后必须打印绝对 `Folder:`，让用户确认真实位置。
- workspace name 默认来自目标文件夹 basename。
- 目标目录必须不存在；GA 阶段不支持创建到已存在的空目录。
- 目标目录不能位于任何已有 workspace 内。
- 目标目录不能位于 `.jvs/` 内。
- 目标目录不能与任何已有 workspace 路径重叠。
- `--repo` 用于断言 `<save>` 所属的 JVS repo；它不是目标路径的基准。

## Workspace 自描述与跳转心智

显式路径语义会带来一个自然问题：用户进入新 workspace 文件夹后，在里面运行 `jvs status`、`jvs save`、`jvs restore` 是否应该像在 main workspace 一样正常工作？

产品判断是：应该正常工作。用户已经站在一个真实 workspace 文件夹里时，JVS 应该能识别“这是同一个 repo 下的某个 workspace”，而不要求用户每次都记住原始 repo 路径。

建议的用户心智：

- 外部 workspace 根目录可以有一个轻量 `.jvs` locator。
- 真正控制面仍在 repo 的 `.jvs/` 中，locator 只帮助 JVS 找回所属 repo 和 workspace。
- 用户不需要理解 locator 的格式。
- 用户应该知道不要删除这个隐藏标记；删除后该文件夹可能不再能自描述为 JVS workspace。

从产品语义看，locator 应绑定两件事：

- repo identity：这个 workspace 属于哪个 JVS repo。
- workspace name：这个文件夹在该 repo 的 workspace registry 里叫什么。

repo workspace registry 仍然是权威来源。locator 是入口线索，不是第二份权威状态。两者不一致时，JVS 应清楚报错，例如说明当前文件夹的 locator 指向某个 repo/workspace，但 repo registry 不再承认它；需要时引导用户运行 doctor 或 repair，而不是默默猜测。

`cwd` 和 locator 提供默认 repo + workspace。显式参数只应覆盖用户明确指定的部分：

- `--repo` 是目标 repo 断言。如果当前目录的 locator 已经属于另一个 repo，应报错，不静默切换到另一个 repo。
- `--workspace <name>` 可以在已解析 repo 内显式选择 workspace。
- 如果 `--workspace <name>` 与当前目录 locator 指向的 workspace 不同，可以执行，但输出必须清楚显示目标 `Folder:` 和 workspace 名字，让用户知道操作的是谁。
- 如果 locator 指向的 workspace 不存在，或者 registry 不承认该 workspace，应报清楚错误，并引导用户运行 doctor 或 repair。

## 不做 Workspace Switch

不建议增加：

```bash
jvs workspace switch exp1
```

原因：

- JVS 不能改变父 shell 的 `cwd`；命令执行完后，用户所在终端目录不会被切换。
- `switch` 容易让用户误以为 JVS 内部存在一个隐藏的“当前 workspace”状态。
- 这会削弱“当前 workspace 由当前目录或显式 `--workspace` 决定”的简单心智。

更好的产品模型是：用户自己进入目标文件夹，JVS 从当前目录识别 workspace；或者用户在命令里显式指定 `--workspace`。

## 跟踪多个 Workspace

如果同一个 repo 下可以有多个真实 workspace，`jvs workspace list` 应该成为用户理解全局状态的主要入口，而不依赖隐式 switch。

默认输出建议更强，至少帮助用户看到：

- 当前所在 workspace 的标记。
- workspace 名字。
- `Folder:` 绝对路径。
- 当前指针。
- newest save point。
- started from save point。

这不是为了暴露内部结构，而是为了回答用户最常见的问题：“我现在在哪个 workspace？还有哪些 workspace？它们分别在什么真实文件夹里？哪个是从哪个 save point 开出来的？”

默认 `workspace list` 不强制检查所有 workspace 是否有未保存修改。跨多个真实文件夹做状态检查可能变慢，也可能让一个普通列表命令显得不可预测。需要看到未保存修改时，可以提供类似：

```bash
jvs workspace list --status
```

这个模式再显示 dirty 状态。产品心智是：默认列表回答“有哪些 workspace、在哪里、当前指向哪里”；带 status 时回答“这些 workspace 是否还有未保存修改”。

`jvs status` 也应清楚显示 repo、folder、workspace。用户在任意 workspace 里运行 status 时，应该能快速确认：

- 当前命令关联的是哪个 repo。
- 当前真实文件夹是哪一个。
- 当前 workspace 名字是什么。
- 当前指针。
- newest/history head。
- started from save point。
- 当前文件夹是否有未保存修改。
- path restore 来源说明，如果当前整体指针之外还有路径级来源需要解释。

## Repo History Tree 与 Workspace 指针

同一个 repo 下有多个 workspace 后，用户会自然想看“这些 workspace 在历史树里的位置”。这里的产品心智应保持简单：

- repo history tree 的主体是 save point graph。
- 不引入“每个 save point 属于哪个 workspace”的心智负担。
- workspace 标签只是渲染时叠加的当前指针标签。
- workspace 标签不是 save point 自身属性。

也就是说，save point 仍然是 repo 历史里的节点；workspace 只是指向某个节点的当前位置。

workspace 当前指针表示当前 workspace 的内容来源 save point。

当用户从某个 save point 创建 workspace 时，当前指针应立刻贴在 source save point 上，即使这个 workspace 还没有自己的 save point：

```text
A [experiment]
```

当用户进入 `experiment` 并运行 `jvs save` 后，新 save point 出现在 A 之后，workspace 当前指针移动到新节点：

```text
A -> B [experiment]
```

多次保存后，链条继续变长，workspace 标签始终贴在该 workspace 当前最新指向的 save point 上：

```text
A -> B -> C [experiment]
```

如果 main workspace 后来也从 A 走出另一条线，tree 可以显示为：

```text
A -> B [experiment]
|
`-> D [main]
```

删除 workspace 时，标签自然消失；不修改 save point 本身。多个 workspace 指向同一个 save point 时，同一节点可以显示多个标签：

```text
A [main] [experiment]
```

whole restore 后，workspace 当前指针应移动到 restored save point，表示当前文件夹整体内容来自那个 save point。但 restore 不创建新的 save point，也不改写 save point 本身；newest/history head 不因为 restore 发生变化。

下一次 `jvs save` 会生成新的 save point。这个新 save point 的 `parent` 仍来自 restore 前的 newest/history head，同时记录 `restored_from` 说明内容来源；workspace 标签移动到这个新 save point。

有未保存修改时，列表、tree 或 status 应用简洁标记表达 dirty，例如：

```text
B [experiment*]
```

也可以在 summary 里说明 `experiment` 有未保存修改。dirty 只表示当前文件夹相对当前指针已有变化，不表示产生了新的 save point。

path restore 后，workspace 整体当前指针不移动，因为此时只是部分路径来自别的 save point。默认 tree/list 上 workspace 标签仍贴在整体当前指针。某次 path restore 的来源和路径属于来源说明，应由 `jvs status` 或详情输出解释，而不是把默认 tree 变成路径级图。

已记录为 path restore 来源的路径变化不算 dirty。path restore 完成后，只有用户又手工修改文件，才显示 unsaved/dirty 标记。

默认 tree 的主边只表达 save point graph 的主体关系：

- `parent`：正常 save 延续出的前后关系。
- `started_from`：workspace 从某个 source save point 开出的关系。

`restored_from` 和 restored paths 是来源说明，不作为默认主树边。它们可以在 `status`、详情输出或 save point detail 中展示。这样 tree 保持可读，不把恢复、局部恢复和路径来源全部混成一张复杂图。

默认 tree 不应全量展示所有 save point。save point 可能很多，产品默认暂定为 recent 30。用户需要更长历史时，用 `--limit` 或 `-n` 控制显示长度；`--limit 0` 表示不限制。

不要用 `--all` 表示“不限制数量”。`--all` 如果同时承担“跨 workspace”和“不限数量”，用户会分不清它是在扩大范围还是放开长度。新设计中历史长度只由 `--limit`/`-n` 管理。

如果某个 workspace 指针在默认显示窗口外，summary 应提示它在窗口外，而不是为了显示这个标签强行把整个旧树拉进来。这样可以让默认输出保持轻量，同时不隐藏重要事实。

## History 命令方向语义

这个设计不新增另一套命名对象。用户心智仍然是：

```text
workspace 当前指针 + save point graph
```

不推荐用：

```bash
jvs history --tree <save>
```

来表达“查看从某个 save point 往后长出的后续”。`tree` 只是显示形状，不说明查询方向。如果 `jvs history` 默认看前史，而 `jvs history --tree <save>` 又变成看后续，用户需要记住“有没有 `--tree` 会改变方向”，这会增加心智负担。

建议把方向写进命令形态：

```bash
jvs history
jvs history to <save>
jvs history from [<save>]
```

建议心智：

- `jvs history`：看当前 workspace 当前指针的来路/前史。
- `jvs history to <save>`：看指定 save point 的来路/前史，到这个 save point 为止。
- `jvs history from [<save>]`：看从某个 save point 往后长出的后续 save point tree。

`history from` 的输出可以是 tree，因为它的语义本来就是“从这里往后”。但 tree 只是呈现形式，不承担方向语义。

示例：

```bash
jvs history
```

用户心智：我站在当前 workspace 里，想看这个 workspace 当前指针是怎么走到这里的。

```bash
jvs history to <save>
```

用户心智：我想看某个 save point 的来路，到这个节点为止。

```bash
jvs history from <save>
```

用户心智：我想看从这个 save point 之后长出了哪些后续 save point，以及 workspace 当前指针标签在哪里。

```bash
jvs history from
```

用户心智：我想看当前 workspace 从它的创始/source save point 往后长出的后续树。

`history from` 省略 `<save>` 时，默认起点是当前 workspace 的 source/origin save point。如果当前 workspace 没有 `started_from`，则从当前指针沿 parent 往前追溯到最早 ancestor，再从那里展示后续。

当前 workspace 还没有自己的 save point 时，`jvs history` 不应只显示 `No save points yet`。它应显示当前指针所指向的 source save point 及其来路，让用户知道自己站在哪里。

默认输出仍不应全量展示所有 save point。recent 30 的限制应自然适用于 `history`、`history to`、`history from`：

- `history` 和 `history to` 默认展示离目标指针最近的 30 个前史节点。
- `history from` 默认展示从起点往后可达、按创建时间最近的 30 个 save points。为了让树可读，可以显示必要连接和省略提示。
- 用户可以用 `--limit` 或 `-n` 控制显示长度。
- `--limit 0` 表示不限制数量。

`history to current`、用 workspace name 当 save point 参数等别名先不做。`jvs history` 已覆盖当前 workspace 当前指针的常用场景，先保持命令心智清楚。

save point graph 是 repo 级的。`jvs history from <save>` 展示从该 save point 往后可达的后续节点，不需要再用 `--all` 表示“跨 workspace”。如果旧 public surface 有 `jvs history --all`，GA 前干净重构中应移除这个旧 surface；新 history 不使用 `--all`。历史长度只由 `--limit`/`-n` 控制，`--limit 0` 表示不限制数量。

## 跳转方式

产品本身不做 `workspace switch`，但可以提供可组合的路径查询：

```bash
jvs workspace path exp1
```

建议 `jvs workspace path <name>` 保持输出纯路径，方便 shell 使用：

```bash
cd "$(jvs workspace path exp1)"
```

文档可以推荐用户按自己的 shell 习惯写 helper，例如 `jcd exp1`，但 JVS 产品命令本身不伪装成能改变当前终端目录的 switch。

## 示例

从当前 workspace 的相邻目录创建实验 workspace：

```bash
jvs workspace new ../project-exp1 --from <save>
```

预期心智：

- 用户明确选择了 `../project-exp1`。
- JVS 输出绝对 `Folder:`。
- 默认 workspace name 为 `project-exp1`。

从任意位置指定 save point 所属 repo，并把新 workspace 创建到 scratch 目录：

```bash
jvs --repo /home/me/project workspace new /scratch/project-exp1 --from <save>
```

预期心智：

- `--repo /home/me/project` 说明从哪个 JVS repo 读取 save point。
- `/scratch/project-exp1` 是新 workspace 的真实目标路径。
- 两者职责分离，不互相推导。

在当前 workspace 根目录内创建子目录应被拒绝：

```bash
jvs workspace new ./experiment --from <save>
```

拒绝原因：

- `./experiment` 按当前 `cwd` 解析后位于已有 workspace 内。
- 新 workspace 不应嵌套在已有 workspace 里。
- 这样会把实验目录放进用户正在管理的主工作文件夹，增加视觉污染和误操作风险。

## 非阻塞细节

产品行为已经收敛，开发前只剩少量不改变语义的表达细节：

- 错误文案的具体措辞。
- locator 与 registry 不一致时，doctor/repair 引导语的具体措辞。
- history tree 的具体 ASCII 渲染样式和省略提示样式。
- path restore 来源说明在 `status` 与详情输出中的版式。
- release note 如何简短说明 GA 前的干净重构。

## 非目标

- 本文不定义具体实现方案。
- 本文不定义迁移脚本。
- 本文不改变当前发布契约。
- 本文可以定义 restore 对 workspace 指针显示的影响，但不重新设计 restore 的安全流程、cleanup 或 workspace remove。
- 本文不引入 `jvs workspace switch`。
- 本文不引入 branch 概念或 `branch` 命令。
