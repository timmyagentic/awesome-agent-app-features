# Awesome Agent App Features

[English](README.en.md) · [Agent 接入指南](docs/agent-integration.md) · [安全模型](docs/security.md)

给 Agent 应用复用的产品功能代码。把仓库链接交给 coding agent，它可以按你的代码结构，把成熟的 Feedback 和安全更新机制接进去。

这不是 awesome 链接清单，也不是 Codex Skills 集合。这里交付的是可运行的包、明确的安全约束、可机读的 feature manifest、接入步骤和回归测试。

## MVP 里有什么

| Feature | 解决的问题 | 当前实现 |
| --- | --- | --- |
| Feedback | 让用户在产品内提交脱敏反馈，同时不把 GitHub Token 放进客户端 | Go 草稿/脱敏/确认/提交包 + 单租户 Cloudflare GitHub relay |
| Updater | 让聊天、CLI 或 UI 共用同一条可信稳定版更新事务 | Go standalone 更新器；stable-only、同 Release checksum、双版本验证、跨进程锁和失败回滚 |

MVP 面向 Go 构建的 Agent 命令行/后台应用；更新器的原地替换承诺目前限定在 macOS 和 Linux。宿主产品继续负责卡片、按钮、自然语言意图、管理员权限、本地化、重启与最终 UI。

## 最快的使用方式：交给 Agent

把这段话连同你的项目发给 coding agent：

```text
请阅读 https://github.com/timmyagentic/awesome-agent-app-features ，
先检查我的项目现有架构，再按照 features/feedback/feature.json 和
features/updater/feature.json 接入这两个功能。保留 manifest 中的 invariants，
适配我现有的 UI、权限、版本输出和 Release 命名，并运行各自 verification。
```

Agent 应先盘点现有反馈入口、版本来源、发布资产、权限和重启生命周期，再修改代码；完整流程见 [docs/agent-integration.md](docs/agent-integration.md)。

## Feedback：核心接法

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

// 必须向用户展示完整脱敏内容或等价的完整渲染。
showToUser(draft.Preview())
if !userClickedSubmit() {
    return nil
}

approved, err := draft.Approve(true)
if err != nil {
    return err
}
receipt, err := (feedback.Client{
    Endpoint: "https://your-relay.example/v1/feedback",
}).Submit(ctx, approved)
```

库只允许 `Approved` 进入提交 API；relay 还会再次要求 `user_approved: true`。自动附加的环境信息是固定白名单，不接受任意环境变量 map。Relay 部署说明在 [relay/cloudflare](relay/cloudflare/README.md)。

## Updater：核心接法

```go
service, err := updater.New(updater.Config{
    Product:        "my-agent-app",
    CurrentVersion: currentVersion,
    ExecutablePath: executablePath,
    BinaryName:     "my-agent-app",
    AssetName:      updater.ReleaseArchiveName("my-agent-app"),
    Source: updater.GitHubSource{
        Repository: "owner/my-agent-app",
    },
    Verifier: updater.ExactVersionLine("my-agent-app"),
    Progress: renderProgress,
})
if err != nil {
    return err
}

result, err := service.Update(ctx)
```

默认资产约定：

```text
my-agent-app-v1.2.3-darwin-arm64.tar.gz
my-agent-app-v1.2.3-linux-amd64.tar.gz
my-agent-app-v1.2.3-windows-amd64.zip
checksums.txt
```

可以替换 `AssetName` 和 `VersionVerifier` 适配现有仓库，但不能绕过以下顺序：严格稳定版 → 同一 Release 精确资产 → SHA-256 → staged 版本 → 备份/替换 → installed 版本 → 失败回滚。

## 仓库结构

```text
feedback/               Go Feedback 核心
updater/                Go stable-only standalone 更新器
relay/cloudflare/       自托管单租户 GitHub issue relay
features/*/feature.json Agent 可读的接入契约
examples/               最小可编译接入示例
docs/                   架构、安全与 Agent 接入说明
```

## 安全边界

- 客户端永远不持有 GitHub Token，也不能指定 relay 的目标仓库。
- Feedback 只有一种形态；错误和能力缺口只是经用户确认的上下文。
- 普通更新永远不选择 beta/rc；Release 标签必须精确匹配 `v?X.Y.Z`。
- checksum manifest 和 archive 必须来自同一个已选 Release。
- checksum 解决下载完整性，不等于 Release 发布身份签名。高风险项目应通过 `Source`/发布流程增加签名或 provenance 校验。
- 已存在 `.update-backup` 时更新器拒绝覆盖，给人工恢复留下证据。

完整威胁模型见 [docs/security.md](docs/security.md)。

## 当前边界

这是 MVP，不包含 npm 自动回滚、多租户反馈 SaaS、Dashboard、账号系统、Windows 原地替换保证、包发布或托管 relay。仓库本身也没有发布 Go module tag；接入时请固定到经过你验证的 commit。

standalone 更新器会拒绝符号链接形式的可执行文件；npm、Homebrew 等安装必须由宿主先检测安装类型，再接入对应的包管理器适配器。

## 验证

```bash
gofmt -l .
go test -race ./...
go vet ./...
npm test --prefix relay/cloudflare
npm run check --prefix relay/cloudflare
npm run validate:worker --prefix relay/cloudflare
```

这个项目从 [CC Connect Next](https://github.com/timmyagentic/cc-connect-next) 已验证的 Feedback 和统一更新执行器中提炼，但去掉了飞书、聊天命令、CLI 文案和具体发布命名等产品绑定层。

MIT License
