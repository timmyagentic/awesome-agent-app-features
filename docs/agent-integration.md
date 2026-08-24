# Coding-agent 远程接入协议

[English](agent-integration.en.md)

这是一份给接入方 Agent 的施工协议。使用者只在目标项目中工作，不需要维护 `awesome-agent-app-features` 的本地副本。

## 1. 固定远程来源

唯一 machine-readable 入口是 `features/index.json`：

```text
https://raw.githubusercontent.com/timmyagentic/awesome-agent-app-features/main/features/index.json
```

`main` 只用于发现。Agent 必须：

1. 通过 GitHub API 把 `main` 解析为 40 位 commit SHA。
2. 确认该 SHA 的 `CI` workflow 已 `completed/success`。
3. 从该 SHA 重新读取入口和所选 Feature 的 manifest、README、schema 与 delivery。
4. 所有资源、依赖和模板使用同一 SHA；任何漂移都停止接入。

参考命令：

```bash
foundation_repository="timmyagentic/awesome-agent-app-features"
resolved_commit="$(gh api "repos/${foundation_repository}/commits/main" --jq .sha)"

gh run list \
  --repo "${foundation_repository}" \
  --commit "${resolved_commit}" \
  --workflow CI \
  --limit 20 \
  --json headSha,status,conclusion \
  | jq -e --arg commit "${resolved_commit}" \
      'any(.[]; .headSha == $commit and .status == "completed" and .conclusion == "success")'

curl --fail --silent --show-error --location --proto '=https' \
  "https://raw.githubusercontent.com/${foundation_repository}/${resolved_commit}/features/index.json"
```

无法使用 `gh` 时可以调用等价的 GitHub HTTPS API。不能从浮动 `main` 混合读取不同时间的文件。

## 2. 先映射宿主

修改代码前，Agent 在对话或工作计划中写清：

- Feature、contract、resolved commit SHA 和实际 delivery；
- foundation、generic adapter 与宿主各自负责什么；
- 宿主现有入口、配置、安装类型和可复用代码；
- 每条 invariant 在宿主中的落点；
- 预计修改的相对路径、聚焦测试、完整验证与 `UNVERIFIED`。

这是一份针对当前代码的短计划，不需要再创建通用 plan JSON。卡片、命令、权限、本地化、重启和业务流程都映射到宿主；本仓库只提供底层值、状态机、端口和基础设施 adapter。

## 3. 应用声明的 Delivery

### `go-module`

在目标项目中执行：

```bash
go get "github.com/timmyagentic/awesome-agent-app-features@${resolved_commit}"
GOWORK=off go test github.com/timmyagentic/awesome-agent-app-features/compat/v1
```

Go 把模块放入 module cache，并在 `go.mod` 中记录 immutable pseudo-version。不得使用本地 `replace`、submodule 或浮动 `main`。

### `source-subtree`

在临时目录下载同 SHA 的 GitHub archive，只提取 manifest 声明的目录，例如 `relay/cloudflare`。提取前检查 archive entries，拒绝 traversal、symlink 和声明目录外文件；完成后删除临时材料。复制后的配置、凭证、部署和维护由宿主负责。没有生产授权时只测试和 dry-run。

## 4. 运行证明

写宿主代码前可以运行同 SHA 的零配置示例：

```bash
GOWORK=off go run "github.com/timmyagentic/awesome-agent-app-features/examples/feedback@${resolved_commit}"
GOWORK=off go run "github.com/timmyagentic/awesome-agent-app-features/examples/updater-demo@${resolved_commit}"
```

Feedback 示例只显示 preview。Updater 示例只更新临时假二进制，不访问真实 Release 或已安装产品。之后仍需运行 manifest 中适用的 remote checks、聚焦宿主测试和目标项目完整验证。

## 5. 写入最小 Lock

接入完成后，在目标项目根目录写入可见的 `agent-app-features.lock.json`，并使用同 SHA 的 [integration-lock.schema.json](../features/integration-lock.schema.json) 验证。

Lock 只记录：

- source repository 和完整 commit SHA；
- Feature ID、contract、实际 delivery 与 resolved module version；
- Agent 修改的宿主相对路径；
- 成功执行的 checks、`verified_at` 和诚实的 `unverified`。

不要写配置值、Token、Cookie、用户或群 ID、payload、原始日志、绝对路径、源码副本或运行状态。Lock 是后续 Agent 的定位线索，不是运行配置、审计数据库或完成证明。

## Feedback 宿主盘点

- 哪些用户动作、错误或能力缺口触发“是否反馈”。
- 哪个宿主 surface 展示 `Draft.Report()` 的全部字段。
- 哪个明确动作代表批准；只有该回调可以调用 `Approve(true)`。
- 哪些产品特有 secret、ID 和路径需要 `AdditionalRedact` 测试。
- Relay endpoint 的配置来源、不可用体验和公开 fallback。

不得捕获任意环境 map、对话 transcript、reasoning、tool payload、原始日志、身份或凭证。宿主测试至少覆盖取消时零请求、preview/outbound 一致、脱敏、stale error、exact endpoint 和 fallback。

## Updater 宿主盘点

- current-version truth、严格且不改文件的版本输出；
- archive、checksum 和 archive entry 命名；
- executable path、安装类型和所有更新入口；
- 授权、progress renderer、restart 与 post-restart acknowledgement；
- 与 stable-only 流程分离的 beta/nightly 策略。

所有入口共享一个 Updater 配置。交互流程是 `Prepare -> 展示 exact Plan -> 授权 -> Apply(同一 Plan)`；只有已经授权的非交互入口可以调用 `UpdateLatest`。npm、Homebrew、Windows 等安装类型必须使用宿主 adapter，不能继承 standalone 替换承诺。

## 后续维护

- 检查：对照 lock、当前依赖、宿主接线和 manifest invariants，只读报告 drift。
- 验证：固定 lock 中的 source，重跑适用检查，不把历史成功当作当前证据。
- 优化：在同一 source 下补齐宿主 UX、fallback 或测试。
- 升级：解析新的 CI-successful commit，比较契约后把所有 delivery 一起迁移并更新 lock，禁止 mixed-source。
- 移除：先查当前引用和 Git 历史，只删除确定未共享的接入代码；从 lock 删除对应 Feature，空 lock 可以删除。

这些是 Agent 对普通代码库的操作，不是本仓库维护的 action 状态机。Git 负责历史，宿主测试负责当前真相。

## 完成标准

只有以下条件满足才能报告完成：

- 依赖和所有远程资源来自同一 commit SHA；
- 每条 responsibility 和 invariant 有宿主落点或明确 blocker；
- 适用 remote checks、聚焦测试和目标项目完整验证通过；
- 无法执行的客户端、凭证、部署、付费、重启或生产验证标为 `UNVERIFIED`；
- `agent-app-features.lock.json` 通过 exact-source schema，并与实际 delivery 和宿主文件一致。

Foundation 自身测试、临时成功输出或历史 lock 都不能替代当前宿主验证。
