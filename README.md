# Awesome Agent App Features

[English](README.en.md) · [Agent 接入指南](docs/agent-integration.md) · [兼容策略](COMPATIBILITY.md) · [安全模型](docs/security.md)

给 Agent 应用复用的无界面 Feature 基座。把仓库和目标项目交给 coding agent，它先理解宿主架构，再把可靠的底层能力接进去；卡片、命令、权限、本地化、重启与业务流程始终留在宿主项目。

这不是 awesome 链接清单、Codex Skills 集合、UI 框架或托管 SaaS。当前未发布的 `v1` 契约收稳三个接入层面的结果：

| 能力 | 本仓库提供 | 宿主产品提供 |
| --- | --- | --- |
| Agent 友好接入 | machine-readable manifests、边界、示例、API/消费者契约和验证命令 | coding agent 按现有架构编写胶水代码并跑宿主测试 |
| Feedback | 脱敏 `Draft`、不可序列化预览、显式 `Approved`、Feedback v1 HTTPS client、单租户 Cloudflare relay | 触发时机、飞书/Slack/CLI/Web 呈现、确认动作、失败体验 |
| Updater | immutable exact plan、stable-only、同 release checksum、双版本验证、锁、no-clobber backup、回滚 | 更新提示、授权、安装类型判断、重启与重启后确认 |

## 当前接入方式

项目尚未发布版本。评估当前 v1 契约时请使用 Go 1.25 或更高版本，并固定到你已经审查的完整 commit SHA，不要依赖浮动的 `main`：

```bash
go get github.com/timmyagentic/awesome-agent-app-features@FULL_REVIEWED_COMMIT_SHA
```

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

## 给 coding agent 的接入指令

```text
本项目尚未发布版本。请固定使用已审查的完整 commit SHA；不要使用 main、
本地 replace 或浮动引用。正式 v1 tag 发布后再切换到对应不可变版本。
先读取 features/<feature>/feature.json、对应 README 和
docs/agent-integration.md，再盘点我项目已有的 UI、权限、版本真相、
Release 资产、安装类型与重启生命周期。复用 foundation core/adapters，
在宿主侧实现展示和业务流程，保留每条 invariants，并运行双方验证。
```

宿主侧的飞书卡片只是 `Report`/`Plan`/`Event` 的 renderer，不能进入本仓库。完整所有权边界见 [docs/architecture.md](docs/architecture.md)。

## 安全边界

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
features/*                    Agent 可读的接入 manifest
feedback/                     Provider-neutral Feedback core
feedback/httpclient/          Feedback v1 HTTPS adapter
protocol/feedback/v1/         JSON Schema 与 Go/JS 共享 fixtures
updater/                      Stable standalone transaction core
updater/github/               GitHub Releases source adapter
relay/cloudflare/             可运行的单租户 GitHub Issues relay
```

```bash
make verify
```

正式发布后，`v1` 遵循 SemVer；公开 API、wire protocol 和 manifest invariants 的变化规则见 [COMPATIBILITY.md](COMPATIBILITY.md)。

本项目从 [CC Connect Next](https://github.com/timmyagentic/cc-connect-next) 的 Feedback 与统一更新执行器中提炼，只保留可复用基座。MIT License。
