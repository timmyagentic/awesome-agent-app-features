# Coding-agent remote integration protocol

[English](agent-integration.en.md)

这是一份给接入方 Agent 的远程施工协议。使用者只在自己的目标项目中工作，不需要 clone 或维护 `awesome-agent-app-features` 的工作副本。

## 唯一入口

公开发现地址：

```text
https://raw.githubusercontent.com/timmyagentic/awesome-agent-app-features/main/features/index.json
```

`features/index.json` 是唯一 machine-readable 入口。`main` 只帮助发现仓库；它不是依赖版本。Agent 必须先解析并固定一个完整 commit SHA，之后所有资源使用同一个 SHA。

## 解析协议

Agent 在目标项目中执行以下逻辑，不能要求用户切换目录或管理第二个仓库：

1. 从 GitHub Commit API 将 `main` 解析为 40 位 commit SHA。
2. 查询该 SHA 的 `CI` workflow；只接受 `completed/success`。
3. 从该 SHA 重新获取 `features/index.json`，不继续信任 discovery URL 的浮动内容。
4. 从入口选择 feature，并从同一 SHA 获取 manifest、README、schema 和 delivery 项。
5. 任一资源漂到另一 SHA、CI 未完成/失败或路径不符合入口时，停止接入。

GitHub CLI 参考命令如下；这些是 Agent 的内部步骤，不是用户安装流程：

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

如果无法使用 `gh`，Agent 可以调用同等 GitHub HTTPS API。不得通过浮动 `main` 混合读取多个时刻的文件。

## 先形成宿主映射

修改目标代码前，Agent 按同一 SHA 获取 `features/integration-plan.schema.json`，并形成一份可审查的计划。计划可以在对话中呈现；只有宿主希望长期审计时才写入目标仓库。

计划至少记录：

- resolved commit、成功 CI run URL、feature、contract 和 delivery mode。
- 宿主 runtime、已有 UI/命令/配置、安装类型和生命周期。
- 每条 foundation/adapter/host responsibility 的落点。
- 每条 invariant 的 `preserved`、`not-applicable` 或 `blocked` 证据。
- 将修改的宿主文件、聚焦测试、完整验证与 `UNVERIFIED` 项。

[integration-plan.example.json](../features/integration-plan.example.json) 只是结构示例，不是可复用的宿主决策。

## Delivery modes

### `go-module`

Agent 在目标项目中把解析出的 SHA 直接交给 Go：

```bash
go get "github.com/timmyagentic/awesome-agent-app-features@${resolved_commit}"
GOWORK=off go test github.com/timmyagentic/awesome-agent-app-features/compat/v1
```

Go 会把模块放进 module cache，不会在用户项目旁创建本仓库工作副本。目标 `go.mod` 最终记录由该 SHA 解析出的 immutable pseudo-version。

### `source-subtree`

Agent 从入口声明的同 SHA GitHub archive 或 Contents API 临时下载内容，只提取 manifest 声明的目录，例如 `relay/cloudflare`，再放到宿主基础设施目录。Agent 必须：

- 先检查 archive entry，拒绝 traversal、symlink 和声明目录外文件。
- 不复制整个 foundation，不创建 submodule，不保留临时下载目录。
- 把提取后的配置、部署、凭证和生命周期视为宿主所有。
- 未获生产授权时只运行测试和 dry-run，不部署。

## 远程零配置证明

Agent 可以在写宿主代码前直接运行同 SHA 示例：

```bash
GOWORK=off go run "github.com/timmyagentic/awesome-agent-app-features/examples/feedback@${resolved_commit}"
GOWORK=off go run "github.com/timmyagentic/awesome-agent-app-features/examples/updater-demo@${resolved_commit}"
```

Feedback 示例只显示 preview。Updater 示例只在临时目录执行完整 prepare/apply/checksum/version/replace 流程，不访问真实 Release 或已安装产品。

## Feedback 宿主盘点

确认并记录：

- 哪些用户可见错误或能力缺口可以触发“是否反馈”。
- 哪个宿主 surface 会渲染 `Draft.Report()` 的每一个字段。
- 哪个明确动作代表批准；自然语言总结本身不等于批准。
- 哪些产品特有 secret、ID、路径形态需要 `AdditionalRedact` 和回归测试。
- `product/version/os/arch/agent` 的可信来源。
- 是否接受匿名 per-install linkability；不接受就省略 `InstallID`。
- `/v1/feedback` endpoint 的配置/禁用方式和 relay 失败后的公开 fallback。

禁止采集任意环境变量 map、对话全文、reasoning、tool payload、原始日志、用户/群 ID 或凭证。宿主可以用飞书卡片、文本、CLI 或 Web 呈现，但必须完整显示 `Report`，且只有确认回调才能调用 `Approve(true)`。

最低宿主测试：

- 未批准或取消时没有网络请求。
- preview 覆盖所有 outbound fields，修改 preview 不能改变 approved payload。
- 默认和产品特有脱敏都生效；stale/future error 不会附到无关报告。
- endpoint 固定为 v1；失败提示不打印 payload/token，并提供安全 fallback。

## Updater 宿主盘点

确认并记录：

- 唯一可信的当前版本值和 released binary 的严格、无副作用 version probe。
- 每个平台的 exact archive/checksum 名和 archive 内 executable name。
- executable path、权限与安装类型；symlink 通常意味着 package manager，不能走 standalone。
- 所有入口：聊天、CLI、UI、手动管理操作和后台 discovery。
- 更新授权规则，以及 shutdown、restart、post-restart acknowledgement。
- beta/nightly 现有通道；它们必须与 stable updater 分离。

所有入口应持有同一份 updater 配置。交互入口执行 `Prepare -> render exact Plan -> authorize -> Apply(plan)`；已经完成授权的非交互入口可以执行 `UpdateLatest`。聊天入口不能 shell 到另一个拥有独立策略的 CLI installer。

最低宿主测试：

- prompt 展示的 tag/asset 与 `Apply` 实际安装完全相同，确认后 latest 变化也不能漂移。
- prerelease、draft、非法版本、missing/duplicate assets 和 checksum mismatch 均在 mutation 前失败。
- staged mismatch 不替换；installed mismatch 恢复旧 binary。
- concurrent entry points 得到明确 sentinel errors。
- restart 与 post-restart acknowledgement 作为宿主逻辑单独验证。

package-manager 安装必须有单独 adapter，明确 stable selection、post-install version truth 和真实 recovery；不能把 standalone 保证写成 npm/Homebrew/Windows 保证。

## 完成条件

Agent 只有在以下条件满足时才能报告接入完成：

- 依赖和所有读取资源固定到同一 commit SHA。
- integration plan 的每条 mapping 和 invariant 都有实现或明确 blocker。
- feature manifest 的 `verification.remote` 与适用的 `verification.host` 已执行。
- 目标项目的正常完整验证已执行。
- 真实客户端、生产凭证、部署、重启或付费检查未执行时明确标为 `UNVERIFIED`。

不允许用用户 clone、本地 `replace`、浮动 `main`、临时成功输出或 foundation 自身测试替代宿主验证。
