# 工程维护约定

本文件适用于整个 lazymusic 工程。binary 必须叫 `lazymusic`；支持 macOS 和 Linux。用户的当前任务指令优先。

## 结构和边界

- `cmd/lazymusic/`：CLI 解析、信号与启动入口；不包含平台业务或绘制代码。
- `internal/app/`：单个 Bubble Tea application、内嵌 Login、来源配置与授权组件安装。异步消息使用来源 epoch，防止切换来源后旧响应污染新页面。
- `internal/nvimconfig/`：为已安装的 Lua 插件生成 lazy.nvim 配置，幂等更新、原子替换并备份不同的旧内容；运行路径不得包含 DESTDIR。
- `internal/music/`：平台无关的 Source、Capabilities、Quality、Track、Query、State；曲目标识是 `(Source, ID)`。
- `internal/catalog/`：Browser、Page、Item、Tab、Inspector 和可选 Editor 合约及有界集合分页。Item.Open/Play、Actions.Playlists 为后端提供的 opaque Query；Item.Writable 仅表示可编辑的目标歌单。
- `internal/sources/apple/`、`internal/sources/local/`：浏览、搜索、推荐与目录配置。任何服务 URL/资源 ID 解释均放在对应后端。
- `internal/backend/registry.go`：来源注册与组合点，定义描述、能力、可用音质、Browser 与 Engine 工厂。
- `internal/engine/`：播放合约、Native Apple 适配器、LOCAL 播放器和供 LOCAL 使用的 Queue 元数据状态机。Queue 本身不输出音频，不对用户暴露模拟后端。
- `internal/audio/`：LOCAL 系统原生音频输出；`internal/volume/`：系统默认输出设备音量，两者职责独立。
- `internal/daemon/`：Unix socket HTTP、进程锁、单前端 lease、UI revision、串行播放命令、日志和快照。协议破坏性变更递增 ProtocolVersion。
- `internal/session/`：版本化 JSON 快照、迁移、原子保存和可持久化字段清理。
- `internal/eventlog/`：有界历史、脱敏、持久化与轮转。
- `internal/tui/`：`model.go` 接收异步状态，`actions.go` 分发按键，`browse.go` 管理查询/分类，`navigation.go` 管理层级与焦点，`panels.go` 管理布局/日志/动画，`sound.go` 管理尺寸、Zen 和均衡器，`edit.go` 管理添加/创建与歌单选择流程，`view.go` 渲染，`keys.go` 定义 Global/面板绑定，`help.go` 从同一注册表生成上下文帮助，`keyboard.go` 区分修饰键与文本。
- `internal/vibez/`：固定版本的第三方原生播放子集。保留来源与许可证；变更需同步 `third_party/vibez-session.patch`、摘要或 local_additions。
- `scripts/`：仅保留依赖准备和第三方代码导入等维护工具。不要加入一次性实测脚本、账号探针、构建产物或测试报告。
- `lua/lazymusic/`：Neovim 浮动终端插件，可通过 lazy.nvim 加载；`docs/lazymusic.txt` 为帮助，安装时部署到插件的 doc/；`tests/nvim_spec.lua` 为独立 headless 回归测试。插件只启动/复用 TUI，不另建播放或授权逻辑。

## 增加音乐平台

1. 在 `music` 中定义稳定 Source ID，在 `internal/sources/<id>/` 实现 `catalog.Browser`；声明实际支持的能力，不要假装支持 Home/搜索/歌单。
2. 返回中立的 Item/Tab；保留平台推荐分类标题。生成明确的 Open 和 Play Query。禁止 UI 根据资源字符串拼平台 URL。
3. 实现 `engine.Engine`；支持 EQ 时声明 Equalizer 能力并实现 `SetEqualizer(music.Equalizer)`，在播放输出中实际应用，不只保存状态。Restore 必须只恢复元数据、队列及位置，不自动发声；Close 释放原生输出、流和 goroutine。异步运行错误必须进入 State.Error，或实现 `Events() <-chan eventlog.Entry`。
4. 在 backend registry 增加来源描述、Browser 和 Engine 工厂；在 app 的来源设置流程增加该平台的授权/目录配置步骤。浏览和播放能力不要写进 app/TUI 的平台判断。
5. 只有真实播放路径能选择的音质才能放入 Source.Qualities，使用稳定 Quality.ID 和 DefaultQuality，并实现 `SetQuality(string) error`。提供新后端的实测支持边界。目录中的 lossless 标签不等于播放器支持无损；偏好与实际流质量必须区分。
6. 凭据只保存在来源私有运行目录，目录 0700、文件 0600；不要让多个来源共享队列、用户 token 或签名 URL。默认目录使用 `profiles/<id>`，显式 --state-dir 保持精确含义。
7. 补充合约、分页、来源隔离、授权失败和恢复测试；有原生差异时在 macOS 和 Linux 构建验证。更新 Sources、Login 描述及 README。
8. 支持收藏/歌单编辑时实现可选 catalog.Editor。Actions 返回实际支持的操作和歌单查询；Mutate 再校验身份、归属和可编辑状态。只有支持新建时声明 PlaylistCreate。写入不得自动重试，202 只能显示已受理；失败也需清理浏览缓存。没有删除接口时返回 RemovalNote，不得模拟删除、隐藏真实项目或复用其他应用的网页凭据。后台写入要求有效 frontend lease，并使用独立锁，不能阻塞播放命令。

## 必须遵守的行为

- macOS 的无损 / Hi-Res 只能由经实际 ALAC 播放验证的后端声明到现有音质选择列表中，不能仅按 GOOS 或曲目 audioVariants 开放；Linux 未验证的格式不得出现。入口、布局和键位保持共用，平台差异只在后端能力与实现。
- 整个应用只有一个 terminal program。Login 是 Content 子页面；移动到 Login 不得自动授权、启动外部程序或停止音乐。Enter/l 才执行选择。
- Apple 授权保留自动浏览器打开，并从官方 helper 输出中只提取预期端口的 loopback /login URL 显示手动入口。URL 的 state 参数仅在当前 Login 过程中使用，不得写日志或 session；取消/完成后清除，迟到事件按 generation 与请求流隔离。
- 普通字符按终端生成的文本处理，保留基本键位消歧和备用字符码；不要在缺少关联文本时强制所有字符用 CSI-u 上报，否则 Shift+/ 等符号会退化成原键。所有页面与弹窗共用同一键盘协议配置。
- h/l 在面板内进退，Ctrl-h/j/k/l 切焦点。`,.` 必须按当前焦点分发：Navigation 动画、Content 分类、Player 播放模式；Sound 提供 Pulse/Wave/真实 EQ。Queue/Log 不得隐式操作旧 Content。
- 布局切换后焦点必须落在可见面板，Player 始终可见；日志可在所有布局用 @ 打开。页脚显示实际页面或当前焦点，不写死 NORMAL。
- q 首次提示，第二次 q 后台，Q 退出当前来源服务；退出/切换前保存 UI，冷恢复暂停。不可让迟到请求覆盖新 revision/epoch。
- H/J/K/L 调当前面板尺寸，Ctrl+= 恢复默认布局；Ctrl++/Ctrl+- 开关 Zen，Zen 必须同时显示律动、真实 EQ 和播放条。尺寸、音效与 Zen 状态持久化。EQ 操作按当前焦点路由，不能冒用播放音量或显示假频谱。
- o/c 作品跳转与 credits、a 添加菜单必须作用于窗口 2/3/4 的实际选中作品；Queue 清空使用 D，d 移除当前队列项。Content 的 a 在歌单列表新建歌单，y 加入播放队列，P 下一首播放。通过 catalog.Inspector/Editor 返回中立数据，UI 不猜测艺人 ID、不拆分 artistName 拼搜索，不把目录推断当作真实 credits。弹框异步查询可取消，关闭后迟到结果不能重开弹框或抢焦点；已提交的写入必须等回复，重复 Enter 不得重发。
- 快捷键注册在 keys.go：Global 与每个面板冲突检查独立，面板可复用字符，不能覆盖 Global。? 只显示当前面板和 Global，必须可滚动且反映自定义绑定；新增动作同时走 dispatch/help/validation。保留 Vim 导航、文本输入时的字符含义与 Ctrl-c/Esc 取消。修饰键优先按 Code/Mod 路由，普通 Shift 符号仍按 Text；覆盖 CSI-u 大写码、alternate/associated text、XTerm modifyOtherKeys 和 Neovim 的终端透传。默认 resize 使用普通大写 H/J/K/L，避免终端对 Ctrl+Shift+j 等键的拦截。不识别连按；h/j/k/l 必须立即响应，长按只在当前窗口移动。播放/暂停、停止、m 轮转模式是 Global；r/s 循环/随机只在 Queue/Player。< / > 上下曲，Ctrl+< / Ctrl+> 快退进（兼容 Ctrl+Shift+逗号/句号编码）。Content 刷新 r、Search 范围 s、EQ 归零 r。Queue 的 [/] 调整顺序；Login 从 Navigation 或 :login 进入。
- UI 音量只调整系统默认输出。原生播放器增益保持 1，禁止重复缩放。外部系统音量变化应在轮询后反映。
- warning/error 需可查看，通知 3 秒自动消失、不抢焦点且日志保留；主动取消旧请求不能作为 error 入库或通知。没有明确服务错误时，不把普通暂停推断为多设备冲突。
- 日志、快照、错误信息不能含 token、cookie、私钥、CDM 密钥、授权头或签名流地址。清理整个签名 URL 的 path/query，不只 query。会话只存恢复所需的稳定元数据。
- 原子写入快照；保留无法读取的旧文件用于恢复。IPC 目录校验所有者和 symlink，socket 私有，命令有界；集合分页检测循环、取消和数量上限。
- 不在源码中放凭据、CDM、辅助授权 binary、缓存、录音或测试音频。当前 Apple 授权沿用 Vibez 官方组件；不要复制其 broker 凭据，也不要声称是独立 Apple 应用身份。

## 验证与改动规范

- 使用 `gofmt`。`make build`、`make test`、`make vet` 是标准检查；原生链接要求 CGO。测试不得操作真实系统音量、默认账号或正在播放的服务；注入 mixer/browser/engine。
- `make install` 默认安装至 `~/.local/bin`，同时部署 Lua 插件并更新 Neovim 的 lua/plugins/lazymusic.lua；支持 PREFIX、BINDIR、NVIM_CONFIG_DIR、NVIM_PLUGIN_DIR、DESTDIR 和 INSTALL_NVIM=0。保持 macOS/Linux 可移植，不使用 GNU 专有 install 参数。安装/卸载验证使用临时 staging 目录，不操作真实账号、服务或系统目录；卸载不得删除配置和 session。用户明确要求真实安装时，验证后再执行。
- 保留有价值的单元测试。回归测试围绕用户行为、边界和并发，避免照抄实现。实测脚本放临时目录，验证后清理。
- 修改 Neovim 插件时运行 `make test-nvim`；使用真实的无声测试进程验证浮窗与终端生命周期，不使用真实音乐账号。按键映射限当前音乐 buffer，不覆盖编辑器全局终端行为；命令及参数以 argv 传递，不拼 shell。
- 更改第三方代码前核对固定版本与许可证，保持导入可复现。`scripts/vendor_vibez.py /path/to/pinned/source` 重新导入后应用本地 patch。
- 在提交前 review 暂存区，检查秘密/产物、macOS/Linux 构建、session 兼容、异步取消与资源释放。不要为了完成提交删除用户已有修改或重置账号。
