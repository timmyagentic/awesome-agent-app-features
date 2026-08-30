# Awesome Agent App Features

[English](README.en.md) · [Agent 接入指南](docs/agent-integration.md) · [贡献 Feature](docs/adding-a-feature.md) · [兼容策略](COMPATIBILITY.md) · [安全模型](docs/security.md)

给 Agent 应用复用的无界面 Feature 基座。使用者留在自己的项目中，让 coding agent 从远程契约接入底层能力；卡片、命令、权限、本地化、安装类型和业务流程仍由宿主实现。

贡献新的 Feature 请从 [CONTRIBUTING.md](CONTRIBUTING.md) 和 [作者指南](docs/adding-a-feature.md) 开始；`feature-author` 支持 Go core 与纯 source-subtree 骨架，并动态验证整个 catalog。

这不是 awesome 链接清单、Codex Skills 集合、UI 框架或托管 SaaS。Agent 友好接入由 Foundation 统一提供；公开 Feature catalog 只呈现 manifest 中真实注册的能力。module 在 `v1.0.0` 前仍明确处于 pre-1.0 阶段。

<!-- generated-feature-catalog:start -->
## Feature Catalog（由 manifest 生成）

下表由 `features/index.json` 及同一 revision 的 Feature manifests 生成；请运行 `go run ./cmd/feature-author sync-docs --root .` 更新，`validate` 会拒绝漂移。

| Feature | 状态 | 首发版本 | Delivery | Quick Start |
| --- | --- | --- | --- | --- |
| [User-approved in-product feedback](features/feedback/README.md) | released / stable | v0.1.0 | go-module + source-subtree | `GOWORK=off go run github.com/timmyagentic/awesome-agent-app-features/examples/feedback@<resolved-commit-sha>` |
| [Stable-only standalone updater](features/updater/README.md) | released / stable | v0.1.0 | go-module | `GOWORK=off go run github.com/timmyagentic/awesome-agent-app-features/examples/updater-demo@<resolved-commit-sha>` |
<!-- generated-feature-catalog:end -->

## 无 Clone 接入

唯一 machine-readable 入口是 [features/index.json](features/index.json)：

```text
https://raw.githubusercontent.com/timmyagentic/awesome-agent-app-features/main/features/index.json
```

`main` 只用于发现。Agent 必须先解析一个完整 commit SHA，确认该提交的 `CI` 成功，再从同一 SHA 获取入口、manifest、文档、依赖和 source subtree。目标项目不得使用 Git submodule、本地 `replace` 或浮动 `main`。

在目标项目中直接告诉 Agent：

```text
请为当前项目接入 awesome-agent-app-features 的 feedback（或 updater）。
官方入口：
https://raw.githubusercontent.com/timmyagentic/awesome-agent-app-features/main/features/index.json

不要要求我 clone 仓库。先把 main 解析为完整 commit SHA，并确认该 SHA 的
CI 成功；所有资源固定到同一 SHA。先盘点宿主已有实现，映射 feature 的责任、
invariants 和验证，再实现最薄适配。无法执行的真实客户端、凭证、部署或重启验证
标为 UNVERIFIED。最后写入不含敏感信息的 agent-app-features.lock.json，并运行
同 SHA 的 feature-lock validator 核对实际依赖、delivery 与宿主文件。
```

Go consumer 应精确固定 `v0.1.1`；Agent 远程接入仍先把发现入口解析为 CI 成功的完整 commit SHA，使所有资源固定到同一 revision。最低 Go 版本为 1.25：

```bash
go get github.com/timmyagentic/awesome-agent-app-features@v0.1.1
go run github.com/timmyagentic/awesome-agent-app-features/examples/feedback@v0.1.1
go run github.com/timmyagentic/awesome-agent-app-features/examples/updater-demo@v0.1.1
```

Go 使用 module cache，不会创建本仓库的工作副本。Updater demo 只替换临时目录中的假二进制，不访问 Release，也不触碰已安装产品。

完成确定性门禁后，可按 [Updater Feature contract](features/updater/README.md) 使用 `examples/updater-live` 对真实公开 GitHub Release 执行只触碰临时假二进制的 opt-in E2E。

公开 Go 包只有：

```text
feedback
feedback/httpclient
updater
updater/github
```

## Feedback

```go
draft, err := (feedback.Builder{}).Build(feedback.Input{
    Description: "启动失败，希望 doctor 能说明原因",
    Environment: feedback.Environment{
        Product: "my-agent-app",
        Version: "v1.4.0",
        Agent:   "codex",
    },
})
if err != nil {
    return err
}

renderEveryField(draft.Report()) // 宿主自己的卡片、文本、CLI 或 Web UI
if !userExplicitlyConfirmed() {
    return nil
}
approved, err := draft.Approve(true)
if err != nil {
    return err
}
_, err = (httpclient.Client{Endpoint: feedbackEndpoint}).Submit(ctx, approved)
```

`Report` 是 deep copy，且不能被 JSON 序列化；只有 opaque `Approved` 能生成 Feedback v1 payload。Core 不生成 Issue 标题、Markdown、卡片或文案。Reference Relay 在服务端固定 GitHub repository 和 token。完整 wire contract 在 [protocol/feedback/v1](protocol/feedback/v1)。

## Updater

```go
service, err := updater.New(updater.Config{
    Product:        "my-agent-app",
    CurrentVersion: currentVersion,
    ExecutablePath: executablePath,
    BinaryName:     "my-agent-app",
    AssetName:      updater.ReleaseArchiveName("my-agent-app"),
    Source: updatergithub.Source{Repository: "owner/my-agent-app"},
    Verifier: updater.ExactVersionLine("my-agent-app"),
    Progress: renderProgress,
})
if err != nil {
    return err
}

plan, err := service.Prepare(ctx)
if err != nil || !plan.Available() {
    return err
}
renderExactUpdate(plan.Release(), plan.ArchiveAsset())
if !userExplicitlyConfirmed() {
    return nil
}
_, err = service.Apply(ctx, plan)
```

`Prepare` 固定 release、只读 Release Notes、archive、archive 内 binary name 和 checksum；`Apply` 只执行这个 plan，不再查询 latest。宿主可以本地化、截断或忽略同一 Plan 中的 Notes，但不能为了展示说明重新查询浮动 latest。Standalone 原地替换只支持 macOS/Linux；npm、Homebrew、Windows 和其他安装类型由宿主 adapter 负责。事务细节见 [Updater contract](docs/updater-contract.md)。

## 接入记录

Agent 完成接入后，在目标项目根目录维护一个可见的 `agent-app-features.lock.json`，结构由 [integration-lock.schema.json](features/integration-lock.schema.json) 定义。它只记录：

- 本仓库和完整 commit SHA；
- Feature、contract 和实际 delivery；
- Agent 修改的相对路径；
- 执行过的检查、时间与 `UNVERIFIED`。

它不保存配置值、凭证、payload、日志、用户 ID、绝对路径、运行状态或删除历史。升级时更新它；检查或移除时结合当前代码和 Git 历史判断。Lock 是维护线索，不代替宿主测试，也不授权部署。

JSON Schema 负责无敏感字段和路径形状；同 SHA 的无状态 validator 进一步核对 Feature/contract、manifest 声明、Go module 版本与内容、source-subtree 的逐文件来源和实际宿主文件。只有 manifest 的 `host_owned_files` 可在复制后由宿主修改；其余交付文件必须与固定源码逐字节一致：

```bash
GOWORK=off go run \
  github.com/timmyagentic/awesome-agent-app-features/cmd/feature-lock@<resolved-commit-sha> \
  validate \
  --source <temporary-exact-sha-source-root> \
  --source-commit <resolved-commit-sha> \
  --host <target-project-root>
```

`--source` 是从同 SHA archive 解出的临时目录，不是用户维护的 clone；验证后删除。

## 边界与门禁

- 飞书卡片等产品 UI 只是 `Report`、`Plan` 或 `Event` 的宿主 renderer，不进入本仓库。
- Feedback 永不后台提交；Relay 再次验证 schema、approval 和 byte limits。
- Updater 只接受 stable tag，checksum 在确认前固定，staged/installed version 不匹配就拒绝或回滚。
- Core 只依赖 Go 标准库；基础设施 adapter 依赖 core，core 不反向依赖 adapter。
- Source subtree 只提取 manifest 声明的目录，并拒绝 traversal 和 symlink；声明的 Relay subtree 必须在离开 foundation 根目录后独立完成 install/test/typecheck/types-check/dry-run。

```text
api/v1.txt                         公开 API 快照
compat/v1                          外部消费者契约
features/index.json                远程 Agent 单一入口
features/integration-lock.schema.json  最小宿主 lock
cmd/feature-lock                    无状态宿主 lock 语义校验
features/*/feature.json            Feature manifests
feedback/                          Feedback core
updater/                           Updater core
relay/cloudflare/                  单租户 GitHub Issues Relay
```

```bash
make verify
```

完整所有权边界见 [architecture](docs/architecture.md)。本项目从 [CC Connect Next](https://github.com/timmyagentic/cc-connect-next) 的 Feedback 与统一更新执行器中提炼，只保留可复用基座。MIT License。
