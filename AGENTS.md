# 工程维护约定

本文件适用于整个 lazymusic 工程。binary 必须叫 `lazymusic`；支持 macOS 和 Linux。用户的当前任务指令优先。

## 结构和边界

- `cmd/lazymusic/`：CLI 解析、信号与启动入口；不包含平台业务或绘制代码。
- `internal/app/`：单个 Bubble Tea application、内嵌 Login、来源配置与授权组件安装。异步消息使用来源 epoch，防止切换来源后旧响应污染新页面。
- `internal/music/`：平台无关的 Source、Capabilities、Quality、Track、Query、State；曲目标识是 `(Source, ID)`。
- `internal/catalog/`：Browser、Page、Item、Tab 合约及有界集合分页。Item.Open/Play 为后端提供的 opaque Query。
- `internal/sources/apple/`、`internal/sources/local/`：浏览、搜索、推荐与目录配置。任何服务 URL/资源 ID 解释均放在对应后端。
- `internal/backend/registry.go`：来源注册与组合点，定义描述、能力、可用音质、Browser 与 Engine 工厂。
- `internal/engine/`：播放合约、Native Apple 适配器、LOCAL 播放器和供 LOCAL 使用的 Queue 元数据状态机。Queue 本身不输出音频，不对用户暴露模拟后端。
- `internal/audio/`：LOCAL 系统原生音频输出；`internal/volume/`：系统默认输出设备音量，两者职责独立。
- `internal/daemon/`：Unix socket HTTP、进程锁、单前端 lease、UI revision、串行播放命令、日志和快照。协议破坏性变更递增 ProtocolVersion。
- `internal/session/`：版本化 JSON 快照、迁移、原子保存和可持久化字段清理。
- `internal/eventlog/`：有界历史、脱敏、持久化与轮转。
- `internal/tui/`：`model.go` 接收异步状态，`actions.go` 分发按键，`browse.go` 管理查询/分类，`navigation.go` 管理层级与焦点，`panels.go` 管理布局/日志/动画，`sound.go` 管理分栏尺寸、Zen 和均衡器，`view.go` 渲染，`keys.go` 定义动作绑定。
- `internal/vibez/`：固定版本的第三方原生播放子集。保留来源与许可证；变更需同步 `third_party/vibez-session.patch`、摘要或 local_additions。
- `scripts/`：仅保留依赖准备和第三方代码导入等维护工具。不要加入一次性实测脚本、账号探针、构建产物或测试报告。

## 增加音乐平台

1. 在 `music` 中定义稳定 Source ID，在 `internal/sources/<id>/` 实现 `catalog.Browser`；声明实际支持的能力，不要假装支持 Home/搜索/歌单。
2. 返回中立的 Item/Tab；保留平台推荐分类标题。生成明确的 Open 和 Play Query。禁止 UI 根据资源字符串拼平台 URL。
3. 实现 `engine.Engine`；支持 EQ 时声明 Equalizer 能力并实现 `SetEqualizer(music.Equalizer)`，在播放输出中实际应用，不只保存状态。Restore 必须只恢复元数据、队列及位置，不自动发声；Close 释放原生输出、流和 goroutine。异步运行错误必须进入 State.Error，或实现 `Events() <-chan eventlog.Entry`。
4. 在 backend registry 增加来源描述、Browser 和 Engine 工厂；在 app 的来源设置流程增加该平台的授权/目录配置步骤。浏览和播放能力不要写进 app/TUI 的平台判断。
5. 只有真实播放路径能选择的音质才能放入 Source.Qualities，使用稳定 Quality.ID 和 DefaultQuality，并实现 `SetQuality(string) error`。提供新后端的实测支持边界。目录中的 lossless 标签不等于播放器支持无损；偏好与实际流质量必须区分。
6. 凭据只保存在来源私有运行目录，目录 0700、文件 0600；不要让多个来源共享队列、用户 token 或签名 URL。默认目录使用 `profiles/<id>`，显式 --state-dir 保持精确含义。
7. 补充合约、分页、来源隔离、授权失败和恢复测试；有原生差异时在 macOS 和 Linux 构建验证。更新 Sources、Login 描述及 README。

## 必须遵守的行为

- 整个应用只有一个 terminal program。Login 是 Content 子页面；移动到 Login 不得自动授权、启动外部程序或停止音乐。Enter/l 才执行选择。
- h/l 在面板内进退，Ctrl-h/j/k/l 切焦点。`,.` 必须按当前焦点分发：Navigation 动画、Content 分类、Player 播放模式；Sound 提供 Pulse/Wave/真实 EQ。Queue/Log 不得隐式操作旧 Content。
- 布局切换后焦点必须落在可见面板，Player 始终可见；日志可在所有布局用 @ 打开。页脚显示实际页面或当前焦点，不写死 NORMAL。
- q 首次提示，第二次 q 后台，Q 退出当前来源服务；退出/切换前保存 UI，冷恢复暂停。不可让迟到请求覆盖新 revision/epoch。
- Ctrl+方向键调当前面板尺寸，Ctrl+= 恢复默认布局；Ctrl++/Ctrl+- 开关 Zen，Zen 必须同时显示律动、真实 EQ 和播放条。尺寸、音效与 Zen 状态持久化。EQ 操作按当前焦点路由，不能冒用播放音量或显示假频谱。
- UI 音量只调整系统默认输出。原生播放器增益保持 1，禁止重复缩放。外部系统音量变化应在轮询后反映。
- warning/error 需可查看，通知 3 秒自动消失、不抢焦点且日志保留；主动取消旧请求不能作为 error 入库或通知。没有明确服务错误时，不把普通暂停推断为多设备冲突。
- 日志、快照、错误信息不能含 token、cookie、私钥、CDM 密钥、授权头或签名流地址。清理整个签名 URL 的 path/query，不只 query。会话只存恢复所需的稳定元数据。
- 原子写入快照；保留无法读取的旧文件用于恢复。IPC 目录校验所有者和 symlink，socket 私有，命令有界；集合分页检测循环、取消和数量上限。
- 不在源码中放凭据、CDM、辅助授权 binary、缓存、录音或测试音频。当前 Apple 授权沿用 Vibez 官方组件；不要复制其 broker 凭据，也不要声称是独立 Apple 应用身份。

## 验证与改动规范

- 使用 `gofmt`。`make build`、`make test`、`make vet` 是标准检查；原生链接要求 CGO。测试不得操作真实系统音量、默认账号或正在播放的服务；注入 mixer/browser/engine。
- 保留有价值的单元测试。回归测试围绕用户行为、边界和并发，避免照抄实现。实测脚本放临时目录，验证后清理。
- 更改第三方代码前核对固定版本与许可证，保持导入可复现。`scripts/vendor_vibez.py /path/to/pinned/source` 重新导入后应用本地 patch。
- 在提交前 review 暂存区，检查秘密/产物、macOS/Linux 构建、session 兼容、异步取消与资源释放。不要为了完成提交删除用户已有修改或重置账号。
