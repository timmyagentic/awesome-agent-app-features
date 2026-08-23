# Coding-agent integration guide

这是一份给接入方 coding agent 的施工契约，不是 Skill，也不是把一段代码盲目复制进项目。

## 标准流程

1. 当前尚未发布版本：固定依赖到已审查的完整 commit SHA，不要引用 `main`、本地 worktree 或 floating commit；正式 v1 tag 发布后再切换。
2. 完整读取 `features/<id>/feature.json`、对应 README、架构和安全文档。
3. 修改前盘点宿主现状，并为每条 `foundation.core`、adapter、host responsibility、exclusion、integration step 和 invariant 写出映射。
4. 复用公开 core/adapter；在宿主仓库写薄适配层处理 UI、授权、配置和生命周期。
5. 先补宿主侧聚焦失败测试，再完成接线。
6. 运行 manifest verification、本仓库 consumer contract，以及宿主项目正常验证。
7. 把无法执行的真实客户端、生产凭证或重启验证明确标记为 `UNVERIFIED`。

## Feedback 盘点

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

- 未批准时没有网络请求；取消后也没有请求。
- preview 覆盖所有 outbound fields，随后对 preview 的修改不能改变 approved payload。
- 默认和产品特有脱敏都生效；stale/future error 不会附到无关报告。
- endpoint 固定为 v1；失败提示不打印 payload/token，并提供安全 fallback。

## Updater 盘点

确认并记录：

- 唯一可信的当前版本值和 released binary 的严格、无副作用 version probe。
- 每个平台的 exact archive/checksum 名和 archive 内 executable name。
- executable path、权限与安装类型；symlink 通常意味着 package manager，不能走 standalone。
- 所有入口：聊天、CLI、UI、手动管理操作和后台 discovery。
- 更新授权规则，以及 shutdown、restart、post-restart acknowledgement。
- beta/nightly 现有通道；它们必须与 stable updater 分离。

所有入口应持有同一份 updater 配置。交互入口执行 `Prepare -> render exact Plan -> authorize -> Apply(plan)`；已经完成授权的非交互入口可以执行 `UpdateLatest`。聊天入口不能 shell 到另一个拥有独立策略的 CLI installer。

最低宿主测试：

- prompt 展示的 tag/asset 与 `Apply` 实际安装完全相同，source 在确认后变更 latest 也不能漂移。
- prerelease、draft、非法版本、missing/duplicate assets 和 checksum mismatch 均在 mutation 前失败。
- staged mismatch 不替换；installed mismatch 恢复旧 binary。
- concurrent entry points 得到 `ErrUpdateInProgress`/`ErrPlanSuperseded` 等明确结果。
- restart 与 post-restart acknowledgement 作为宿主逻辑单独验证。

## 允许与禁止的适配

允许：release/archive/binary 命名、严格 version output、UI、权限、配置、翻译、重启、错误 fallback。

禁止放入 core：平台 SDK、宿主仓库名、托管 endpoint、卡片模型、命令 parser、本地化文案、自然语言 intent、restart path 或任意产品分支。

package-manager 安装必须有单独 adapter，明确描述 stable selection、post-install version truth 和真实 rollback/recovery；不能把 standalone 的保证写成 npm/Homebrew 的保证。
