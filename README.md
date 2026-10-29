# lazymusic

macOS / Linux 原生音乐 TUI。布局参考 lazygit，默认 Vim 键位。支持 APPLE MUSIC 与 LOCAL，运行 `lazymusic` 即可在 Content 中选择来源、登录或恢复 session。

## 构建

需要 Go 1.25+、CGO 和 C/C++ 编译工具。

```sh
make build
./bin/lazymusic
```

macOS 需要 Xcode Command Line Tools，使用系统 Core Audio / AVFoundation（LOCAL 经 AVAudioEngine 与 AVAudioUnitEQ 输出）。Linux 需要 `pkg-config`、GStreamer 开发头文件及解码插件，例如 Debian/Ubuntu 的 `libgstreamer1.0-dev gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-libav`。系统音量使用 PipeWire 的 `wpctl` 或 PulseAudio 的 `pactl`。已有运行库但缺少头文件时，`scripts/linux-local-deps.sh` 可以准备隔离的构建依赖。

## 来源与播放

- **APPLE MUSIC**：Home 顶部按平台返回的分类显示推荐，支持歌曲、专辑、艺人、歌单、个人收藏，以及 catalog/library 双范围搜索。`Enter` 播放歌曲或打开集合，`p` 播放整个集合，`a` 加入队列，`A` 下一首播放。
- **LOCAL**：默认 `~/Music`；可在 Login 中输入目录，或用 `--source local --music-dir PATH` 配置。支持目录浏览、按文件名搜索、按目录组织专辑/艺人；暂未解析嵌入标签。实际格式支持取决于系统解码器，不读取 Apple 下载的 DRM 文件。
- Apple 登录暂时使用校验摘要固定的 Vibez v2.0.6 官方授权组件，授权身份是 Vibez。仅登录时临时打开浏览器；播放使用本机 CDM 和原生音频输出，无需保留浏览器。此方案依赖其授权服务以及 Apple 订阅权限。
- Apple 原生路径提供 AAC 64 / 256 kbps 的**偏好选择**，按 `b` 设置；切换会在当前位置重新准备歌曲。实际可用流取决于服务响应，暂不提供无损/Hi-Res。LOCAL 直接播放原文件，不转码。

首次进入时列出全部来源，不会自动发起 Apple 登录。导航到 Login 只显示选项，`Enter` / `l` 进入操作。来源切换后，之前的来源暂停，保留各自队列和界面。

## 键位

| 按键 | 操作 |
| --- | --- |
| `j/k`、方向键 | 列表上下移动 |
| `h/l`、`Enter` | 上级 / 下级、确认或播放 |
| `Ctrl-h/l`、`Ctrl-j/k` | 左右、上下切换面板 |
| `1/2/3/4/5/6`、`Tab` | Navigation / Content / Queue / Player / Log / Sound，或轮转焦点 |
| `,` / `.` | Content 切分类；Navigation/Sound 切 Pulse/Wave/EQ；Player 切顺序/单曲循环/列表循环/随机 |
| `Ctrl+方向键`、`Ctrl+=` | 调整当前面板尺寸、恢复默认布局 |
| `Ctrl++` / `Ctrl+-` | 开关 Zen：律动 + Equalizer + Player |
| `+` / `_`（Shift `=` / `-`） | 轮换完整、Content+Player、Queue+Player 布局 |
| `Space`、`x` | 播放/暂停、停止 |
| `[` / `]`、`<` / `>` | 上一首/下一首、后退/前进 10 秒 |
| `=` / `-` | 系统默认输出音量 ±5% |
| `b`、`r`、`z` | 支持的音质偏好、循环模式、随机播放 |
| `/`、`i` / `Ctrl-f`、`S` | 过滤已加载项、编辑搜索、切换 catalog/library |
| `d`、`c`、`J/K`（Queue） | 移除、清空、调整顺序 |
| `@` | 开关运行日志窗口 |
| `Esc` | 退出输入/对话框或返回上级 |
| Sound 的 EQ：`h/l`、`j/k`、`Space`、`r`、`p` | 频段、±1 dB、旁路、归零、切预设 |
| `L`、`R`、`?` | Login、刷新、帮助 |
| `q`，再按 `q` / `Q` | 后台运行 / 完全退出当前来源的服务 |

左侧 Navigation 与 Sound 上下分栏，Sound 可用 `,.` 切换 Pulse/Wave/Equalizer。EQ 为 32 Hz–16 kHz 十段真实音效调节，范围 ±12 dB，支持 Flat/Bass/Vocal/Treble 预设、旁路及持久化；Apple 与 LOCAL 的 macOS/Linux 输出均接入音效处理。Pulse/Wave 是随播放状态和进度变化的动画，不是音频频谱分析。播放器显示播放状态图标、模式及音质偏好。默认输出设备不提供系统音量时，界面显示不可用并在操作时提示错误。

## Session、日志和配置

`q q` 关闭 TUI，服务继续播放；再次运行恢复焦点、布局、分类、列表光标、队列及播放状态。`q Q` 停止服务；再次启动保留队列和位置，以暂停状态恢复。切换音源时各来源的配置和 session 隔离。

Ctrl 组合键依赖终端正确报告相应按键；应用请求增强键盘协议。窗口尺寸、分栏与 Zen 状态也会保存。

配置默认保存在 macOS 的 `~/Library/Application Support/lazymusic`、Linux 的 `~/.config/lazymusic`，可用 `--state-dir PATH` 或 `LAZYMUSIC_STATE_DIR` 指定精确目录。LOCAL 默认使用 `profiles/local` 子目录，Apple 保留主配置位置。`keys.json` 可覆盖 `internal/tui/keys.go` 中定义的动作绑定。

运行日志保存在私有目录的 `runtime.log`（500 条内存历史，文件约 1 MiB 轮转一次）。后端报告的 warning/error 显示 3 秒后自动消失，不抢焦点，并保留在 `@` 日志窗口。切换页面引起的正常请求取消不会记录为错误。Apple 多设备播放限制若通过播放器/服务返回错误，会展示原始原因的脱敏版本；未收到明确原因时不会猜测为多端冲突。服务启动诊断另见 `service.log`。

```sh
lazymusic status
lazymusic ctl --op pause
lazymusic ctl --op quality --quality high
lazymusic quit
lazymusic logout
```

同一来源只允许一个 TUI 持有界面写入权。升级协议后，旧服务需要先执行 `lazymusic quit`，再重新进入。

## 开发

```sh
make test
make vet
```

有价值的单元测试覆盖 session、IPC 竞争、来源隔离、路径边界、分页、UI 焦点/布局/通知和音质偏好。探针、一次性实测脚本、历史报告、凭据和构建缓存不属于源码。工程结构与新平台接入规范见 [AGENTS.md](AGENTS.md)。

原生 Apple 实现导入了 MIT 授权的 Vibez 后端子集，来源、摘要和本地补丁保存在 `third_party/`。CDM 及授权组件按需下载到私有运行目录，不随源码提交。
