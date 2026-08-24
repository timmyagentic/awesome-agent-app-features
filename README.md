# Awesome Agent App Features

[English](README.en.md) · [Agent 接入指南](docs/agent-integration.md) · [兼容策略](COMPATIBILITY.md) · [安全模型](docs/security.md)

给 Agent 应用复用的无界面 Feature 基座。使用者只需要停留在自己的目标项目中，告诉 coding agent 要接入哪个 feature；Agent 从官方远程入口解析契约、固定一个 commit SHA、添加所需依赖或模板，再按宿主架构编写适配。卡片、命令、权限、本地化、重启与业务流程始终留在宿主项目。

这不是 awesome 链接清单、Codex Skills 集合、UI 框架或托管 SaaS。当前未发布的 `v1` 契约收稳三个接入层面的结果：

| 能力 | 本仓库提供 | 宿主产品提供 |
| --- | --- | --- |
| Agent 友好接入 | machine-readable manifests、边界、示例、API/消费者契约和验证命令 | coding agent 按现有架构编写胶水代码并跑宿主测试 |
| Feedback | 脱敏 `Draft`、不可序列化预览、显式 `Approved`、Feedback v1 HTTPS client、单租户 Cloudflare relay | 触发时机、飞书/Slack/CLI/Web 呈现、确认动作、失败体验 |
| Updater | immutable exact plan、stable-only、同 release checksum、双版本验证、锁、no-clobber backup、回滚 | 更新提示、授权、安装类型判断、重启与重启后确认 |

## 无 Clone 接入

官方 machine-readable 入口是：

[features/index.json](features/index.json)

它的公开发现地址是：

```text
https://raw.githubusercontent.com/timmyagentic/awesome-agent-app-features/main/features/index.json
```

`main` 只用于发现。Agent 必须先把它解析为一个完整 commit SHA，确认该提交的 `CI` 已成功，再从同一个 SHA 重新获取入口、feature manifest、文档和 source subtree。使用者不需要 clone 本仓库；目标项目也不得使用 Git submodule、本地 `replace` 或浮动 `main`。

在目标项目中直接给 Agent 这段指令即可：

```text
请为当前项目接入 awesome-agent-app-features 的 feedback（或 updater）。
官方入口：
https://raw.githubusercontent.com/timmyagentic/awesome-agent-app-features/main/features/index.json

不要要求我 clone 仓库。先解析 main 为完整 commit SHA，并确认该 SHA 的 CI
成功；之后所有入口、manifest、文档、依赖和模板都固定到同一 SHA。
先按 integration-plan schema 盘点并映射宿主，再实现薄适配层和测试。
保留所有 invariants；无法执行的真实客户端、凭证、部署或重启验证标为
UNVERIFIED。
```

项目尚未发布版本。评估当前 v1 契约时请使用 Go 1.25 或更高版本，并固定到你已经审查的完整 commit SHA，不要依赖浮动的 `main`：

```bash
go get github.com/timmyagentic/awesome-agent-app-features@<agent-resolved-commit-sha>
```

这条命令由 Agent 在目标项目中执行；Go 会把依赖下载到 module cache，不会创建本仓库的工作副本。Agent 可在写入宿主前远程运行两个零配置示例：

```bash
go run github.com/timmyagentic/awesome-agent-app-features/examples/feedback@<agent-resolved-commit-sha>
go run github.com/timmyagentic/awesome-agent-app-features/examples/updater-demo@<agent-resolved-commit-sha>
```

Updater demo 只更新临时目录中的假二进制，不访问 GitHub Release，也不触碰已安装产品。完整协议和宿主 mapping 见 [Agent 接入指南](docs/agent-integration.md)；计划结构见 [integration-plan.schema.json](features/integration-plan.schema.json)。

`v1` 公开包只有：

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

// 宿主用自己的卡片、文本、CLI 或 Web UI 渲染每一个字段。
renderEveryField(draft.Report())
if !userExplicitlyConfirmed() {
    return nil
}

approved, err := draft.Approve(true)
if err != nil {
    return err
}
receipt, err := (httpclient.Client{
    Endpoint: "https://feedback.example/v1/feedback",
}).Submit(ctx, approved)
```

`Report` 是 deep copy，标准 JSON 编码会返回 `ErrApprovalRequired`；只有 opaque `Approved` 能生成 schema 1 wire payload。Core 不生成 Issue title、Markdown body、卡片或文案。Reference relay 使用服务端固定的 GitHub repository/token，客户端不能选择目的地。协议和共享 fixtures 在 [protocol/feedback/v1](protocol/feedback/v1)。

## Updater

```go
service, err := updater.New(updater.Config{
    Product:        "my-agent-app",
    CurrentVersion: currentVersion,
    ExecutablePath: executablePath,
    BinaryName:     "my-agent-app",
    AssetName:      updater.ReleaseArchiveName("my-agent-app"),
    Source: updatergithub.Source{
        Repository: "owner/my-agent-app",
    },
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
result, err := service.Apply(ctx, plan)
```

请分别导入 `updater` 与 `updater/github`。`Prepare` 锁定 release、archive、archive 内 binary name 和 checksum；`Apply` 只执行这个 plan，不会再次查询 latest。新的成功 `Prepare` 会让旧 plan 返回 `ErrPlanSuperseded`。已经完成授权的非交互 CLI/管理员入口可以直接调用 `UpdateLatest`。

默认归档命名为 `<product>-<tag>-<os>-<arch>.tar.gz`（Windows 为 zip），checksum 文件默认为 `checksums.txt`。若归档内 binary name 随 tag/平台变化，使用 `ArchiveBinaryName`。standalone 原地替换承诺限定在 macOS/Linux，并拒绝 symlink executable；npm、Homebrew 与 Windows 需要宿主自己的安装 adapter。

## Agent 接入协议

Agent 以 [features/index.json](features/index.json) 为唯一入口，从同一个 commit SHA 获取所选 `features/<id>/feature.json`、README 和 delivery 项。它先使用 [integration-plan.schema.json](features/integration-plan.schema.json) 映射宿主已有 UI、权限、版本真相、Release 资产、安装类型与生命周期，再修改目标项目。

`go-module` delivery 通过精确 SHA 增加依赖；`source-subtree` delivery 只从同一 SHA 的 GitHub archive 或 Contents API 提取声明目录到宿主基础设施。Agent 可以在临时目录或语言包缓存中下载内容，但不能让使用者管理本仓库的第二份检出。

宿主侧的飞书卡片只是 `Report`/`Plan`/`Event` 的 renderer，不能进入本仓库。完整所有权边界见 [docs/architecture.md](docs/architecture.md)。

## 安全边界

- 远程入口先解析为完整 commit SHA；入口、manifest、依赖、示例和模板必须来自同一 SHA，且该提交 CI 成功。
- Feedback 不在后台提交；`Approved` 必须来自明确用户动作。
- 默认脱敏、固定环境白名单和 UTF-8 byte limits 在 Go 与 relay 两侧重复执行。
- 普通更新只接受精确 `v?X.Y.Z` stable tag；draft、prerelease 和前导零版本会被拒绝。
- checksum 在 `Prepare` 阶段固定，archive 在任何提取/执行/替换前验证。
- staged 和 installed binary 都必须报告 exact target version，否则不替换或回滚。
- 锁文件 symlink、executable symlink 和已有 recovery backup 都 fail closed。
- checksum 证明下载内容一致，不证明发布者身份；高风险产品仍应加入独立签名或 provenance。

## 结构与门禁

```text
api/v1.txt                    v1 公开 API 快照
compat/v1                     只使用公开符号的外部消费者编译契约
features/index.json           无 Clone 远程 Agent 单一入口
features/*.schema.json        入口、Feature 与宿主计划契约
features/*/feature.json       Agent 可读的接入 manifest
feedback/                     Provider-neutral Feedback core
feedback/httpclient/          Feedback v1 HTTPS adapter
protocol/feedback/v1/         JSON Schema 与 Go/JS 共享 fixtures
updater/                      Stable standalone transaction core
updater/github/               GitHub Releases source adapter
relay/cloudflare/             可运行的单租户 GitHub Issues relay
examples/updater-demo/        仅修改临时文件的离线完整事务
```

```bash
make verify
```

正式发布后，`v1` 遵循 SemVer；公开 API、wire protocol 和 manifest invariants 的变化规则见 [COMPATIBILITY.md](COMPATIBILITY.md)。

本项目从 [CC Connect Next](https://github.com/timmyagentic/cc-connect-next) 的 Feedback 与统一更新执行器中提炼，只保留可复用基座。MIT License。
