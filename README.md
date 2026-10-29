# lazymusic

**把音乐留在终端。** 一个支持 Apple Music 和本地音乐库的音乐 TUI，默认使用 Vim 键位。

在同一界面浏览推荐、搜索收藏、管理队列和调整音效。关闭 TUI 后继续播放，下次打开接着听。

[安装](#安装) · [快速上手](#快速上手) · [键位](#键位) · [Neovim](#neovim) · [配置](#配置) · [常见问题](#常见问题) · [参与开发](#参与开发)

## 功能

- **两种音乐来源**：Apple Music 个性化推荐、收藏、歌单和搜索；LOCAL 浏览并播放自己的音乐目录。
- **键盘优先**：Vim 导航、面板焦点切换、可调分栏，以及 Content / Queue 精简布局。
- **持续播放**：独立后台服务保存队列、位置和界面，重新运行即可恢复 session。
- **播放控制**：播放、暂停、停止、切歌、快进退、随机和循环；直接调整系统默认输出音量。
- **音效与 Zen**：十段均衡器、预设及旁路；Zen 同屏显示律动、均衡器和播放条。
- **运行日志**：`@` 查看日志；运行错误显示 3 秒提示，不抢走当前焦点。

### 音乐来源

| 来源 | 浏览与搜索 | 播放与音质 |
| --- | --- | --- |
| APPLE MUSIC | 按平台分类的 Home 推荐、个人收藏、歌单、catalog / library 双范围搜索 | 原生播放，提供 AAC 64 / 256 kbps 偏好；需要有效订阅 |
| LOCAL | 默认 `~/Music`，支持自定义目录、目录浏览和文件名搜索 | 直接播放原文件，格式支持取决于系统解码器 |

Apple Music **仅授权时临时使用浏览器，播放不依赖浏览器**。当前授权使用固定版本、校验摘要的 Vibez 官方组件，Apple 授权页中的应用身份为 **Vibez**，依赖其授权服务。首次使用会下载授权组件和播放所需的 CDM；凭据保存在本机私有目录。

当前 Apple 后端尚未验证出真正的无损 / Hi-Res，因此音质菜单只展示可兑现的 AAC 选项。LOCAL 不转码，也不读取 Apple 下载的 DRM 文件；暂不解析音乐文件的嵌入标签。详细边界与原生 MusicKit 实测见 [音质说明](docs/audio-quality.md)。

## 安装

目前从源码安装。需要 **Go 1.25+**、CGO 和 C/C++ 工具链；先按系统准备依赖，再执行下方安装命令。

Apple Music 的授权和 CDM 组件目前支持 **macOS Apple Silicon / Intel、Linux x86_64**。LOCAL 的格式支持由对应平台的音频栈决定。

### macOS

安装 Xcode Command Line Tools，并按 [Go 官方说明](https://go.dev/doc/install) 安装 Go：

```sh
xcode-select --install
go version
```

使用系统 Core Audio / AVFoundation，无需额外安装 GStreamer。

### Ubuntu

安装编译工具、GStreamer 开发包、解码与音频输出插件：

```sh
sudo apt update
sudo apt install -y \
  build-essential pkg-config git ca-certificates \
  libgstreamer1.0-dev gstreamer1.0-tools \
  gstreamer1.0-plugins-base gstreamer1.0-plugins-good \
  gstreamer1.0-libav gstreamer1.0-pulseaudio \
  pulseaudio-utils
```

随后按 [Go 官方说明](https://go.dev/doc/install) 安装 **Go 1.25 或更新版本**。Ubuntu 仓库里的 `golang-go` 可能低于此要求，请先检查 `go version`。

| 依赖 | 用途 |
| --- | --- |
| `build-essential`、`pkg-config`、`libgstreamer1.0-dev` | CGO / C++ 编译与 GStreamer 链接 |
| `gstreamer1.0-plugins-base`、`gstreamer1.0-plugins-good` | 播放管线、常用格式和音频输出 |
| `gstreamer1.0-libav` | AAC 等音频解码 |
| `gstreamer1.0-pulseaudio` | 通过 PulseAudio 或 PipeWire 的 PulseAudio 兼容服务输出音频 |
| `pulseaudio-utils` | 提供 `pactl`，用于系统音量；已有 `wpctl` 时优先使用它 |

在普通用户的桌面音频会话中运行。仅有 SSH 或无音频服务的服务器，需要先配置可用的输出设备和用户音频服务。相关包说明见 [GStreamer 官方安装文档](https://gstreamer.freedesktop.org/documentation/installing/on-linux.html)、[Ubuntu 解码插件](https://packages.ubuntu.com/noble/gstreamer1.0-libav)和[音量工具](https://packages.ubuntu.com/noble/pulseaudio-utils)。

### 编译与安装

```sh
git clone https://github.com/lixuan998/lazymusic.git
cd lazymusic
make install
```

默认安装到 `~/.local/bin/lazymusic`，不需要 `sudo`。如果该目录尚未加入 `PATH`：

```sh
export PATH="$HOME/.local/bin:$PATH"
lazymusic
```

将这行 `export` 加入 `~/.zshrc` 或 `~/.bashrc`，即可在新终端中直接运行。

也可以只编译，或指定安装位置：

```sh
make build                              # 产物：bin/lazymusic
./bin/lazymusic
make install PREFIX="$HOME/Applications/lazymusic"  # 安装到 PREFIX/bin
make install BINDIR="$HOME/bin"          # 直接指定 binary 目录
```

`make install` 会先编译；支持 `GO`、`INSTALL`、`PREFIX`、`BINDIR` 和用于打包的 `DESTDIR` 覆盖。`DESTDIR` 仅作为安装路径的前缀，不影响运行时配置位置。

### 更新与卸载

退出正在运行的 TUI，并停止相应来源的后台服务后更新：

```sh
lazymusic quit
git pull --ff-only
make install
lazymusic
```

如曾使用多个来源，可通过 `--source apple` / `--source local` 分别停止其服务；使用自定义状态目录时需同时传入相同的 `--state-dir`。

```sh
make uninstall
# 自定义安装位置时，传入与安装时相同的 PREFIX 或 BINDIR
make uninstall BINDIR="$HOME/bin"
```

卸载只删除 binary；配置、登录信息和 session 会保留。卸载前可用 `lazymusic logout` 清除当前来源的登录与 session。

## 快速上手

1. 运行 `lazymusic`，在 Content 的 Login 页面选择 **APPLE MUSIC** 或 **LOCAL**，按 `Enter` / `l` 确认。
2. Apple Music 按官方页面提示完成授权；LOCAL 默认使用 `~/Music`，也可在 Login 中修改目录。
3. 按 `1` 进入 Navigation，使用 `j/k` 选择 Home、Library、Playlists 或 Search，按 `l` / `Enter` 进入内容。
4. 歌曲按 `Enter` 播放；专辑或歌单按 `p` 播放全部，按 `a` 加入队列。
5. 按 `q` 打开离开提示，再按 `q` 后台播放；下次运行 `lazymusic` 恢复。在离开提示中按 `Q` 则完全退出当前来源的服务。

直接指定本地音乐目录：

```sh
lazymusic --source local --music-dir "$HOME/Music"
```

切换来源会暂停之前的来源，各自保留队列和界面。后台恢复会连接仍在播放的服务；服务已退出时，恢复队列及位置并保持暂停。

## 键位

按 `?` 查看内置帮助。最低终端尺寸为 **60 × 16**；较宽的终端可同时显示 Content 和 Queue。

### 导航与布局

| 按键 | 操作 |
| --- | --- |
| `j` / `k`、`↑` / `↓` | 列表上下移动 |
| `h` / `l`、`Enter` | 返回上级 / 打开下级、确认或播放 |
| `Ctrl+h/j/k/l` | 切换到左 / 下 / 上 / 右面板 |
| `1`–`6`、`Tab` / `Shift+Tab` | Navigation / Content / Queue / Player / Log / Sound；轮转焦点 |
| `gg` / `G`、`Ctrl+u/d` | 顶部 / 底部、半页滚动 |
| `,` / `.` | Content 切分类；Navigation / Sound 切律动与 EQ；Player 切播放模式 |
| `+` / `_`（`Shift+=` / `Shift+-`） | 轮换完整、Content + Player、Queue + Player 布局 |
| `Ctrl++` / `Ctrl+-` | 开关 Zen：律动 + Equalizer + Player |
| `Ctrl+Shift+h/j/k/l` | 向左 / 下 / 上 / 右调整当前面板尺寸 |
| `Ctrl+=` | 恢复默认布局 |
| `@`、`L`、`R`、`Esc` | 日志窗口、Login、刷新、取消或返回 |

### 播放与搜索

| 按键 | 操作 |
| --- | --- |
| `Space` / `x` | 播放暂停 / 停止 |
| `[` / `]`、`<` / `>` | 上一首 / 下一首、后退 / 前进 10 秒 |
| `=` / `-` | 系统默认输出音量 ±5% |
| `b`、`r`、`z` | 音质偏好、循环模式、随机播放 |
| `a` / `A`、`p` | 加入队列 / 下一首播放、播放整个集合 |
| `/`、`i` / `Ctrl+f`、`S` | 过滤已加载内容、编辑搜索、切换 catalog / library 搜索 |
| 窗口 2 / 3 / 4：`o` / `c` | 打开专辑与艺人跳转菜单 / 查看 credits |
| Queue：`d`、`C`、`J` / `K` | 移除、清空、调整顺序 |
| `q` → `q` / `Q` | 后台运行 / 完全退出当前来源服务 |

### 均衡器

按 `6` 聚焦 Sound，使用 `,.` 切到 Equalizer。支持 **32 Hz–16 kHz 十段、±12 dB** 调节，音效应用于实际输出，并随 session 保存。

| 按键 | 操作 |
| --- | --- |
| `h` / `l` | 选择频段 |
| `j` / `k` | 降低 / 提高 1 dB |
| `Space`、`r`、`p` | 切换旁路、归零、轮换 Flat / Bass / Vocal / Treble 预设 |

Pulse / Wave 是随播放状态和进度变化的律动动画，未进行实时音频频谱分析。Ctrl 组合键需要终端正确上报，应用会请求增强键盘协议。

### 作品信息

在 Content 选中作品、Queue 选中歌曲或聚焦 Player 时，按 `o` 打开跳转菜单。专辑和每位艺人分别列为一项，使用 `j/k` 选择，`Enter` / `l` 在 Content 中打开，`h` 返回原页面。不会改变播放队列。

按 `c` 查看 credits，`j/k` 或 `Ctrl+u/d` 滚动，`Esc` 关闭。显示音源实际提供的贡献者、角色、作曲及发行信息；没有完整制作人员名单时会明确注明。LOCAL 当前按文件与目录组织，支持专辑 / 艺人跳转，但尚不读取嵌入式 credits。为避免冲突，Queue 清空改为大写 `C`。

## Neovim

仓库内包含 Neovim 0.10+ 插件，用 `<leader>mm` 在浮动终端打开 lazymusic。先运行 `make install`，再将以下配置加入 `lazy.nvim` 插件列表；LazyVim 可保存为 `lua/plugins/lazymusic.lua`：

```lua
return {
  {
    "lixuan998/lazymusic",
    main = "lazymusic",
    cmd = { "LazyMusic", "LazyMusicHide" },
    keys = {
      { "<leader>mm", function() require("lazymusic").open() end, desc = "Open lazymusic" },
    },
    opts = {
      cmd = "lazymusic", -- 默认查找 PATH，再查找 ~/.local/bin/lazymusic
      args = {},        -- 例如 { "--source", "local" }
      width = 0.94,
      height = 0.92,
      border = "rounded",
    },
  },
}
```

本地开发可将仓库地址替换为 `dir = "/absolute/path/to/code/lazy-music"`。插件不代替 binary 安装；可用 `opts.cmd` 指定可执行文件的完整路径，参数通过 `opts.args` 逐项传入，不经过 shell。

重复打开会复用已有终端；`:LazyMusicHide` 只隐藏浮窗。TUI 内 `q q` 会关闭浮窗并保持后台播放，`q Q` 停止当前来源服务；再次 `<leader>mm` 恢复 session。`Esc`、面板焦点和窗口调节键使用当前 buffer 的透传映射，不影响其他终端。

需要临时回到 Neovim 普通模式时，先按 `Ctrl+反斜杠`，再按 `Ctrl+n`。完整配置见 `:help lazymusic` 或 [插件帮助](doc/lazymusic.txt)。

## 配置

| 位置或选项 | 含义 |
| --- | --- |
| macOS：`~/Library/Application Support/lazymusic` | 默认配置和私有运行目录 |
| Linux：`$XDG_CONFIG_HOME/lazymusic`，未设置时为 `~/.config/lazymusic` | 默认配置和私有运行目录 |
| `--state-dir PATH` / `LAZYMUSIC_STATE_DIR` | 使用指定的精确状态目录 |
| `--music-dir PATH` | 设置 LOCAL 音乐根目录，需同时选择 `--source local` |
| `keys.json` | 当前来源状态目录中的自定义按键 |

默认情况下，Apple Music 使用主状态目录，LOCAL 使用其下的 `profiles/local`；队列、授权和 session 按来源隔离。

`keys.json` 以动作名覆盖对应的默认绑定，例如为帮助添加 `F1`：

```json
{
  "help": ["?", "f1"]
}
```

动作名与默认值见 [keys.go](internal/tui/keys.go)。同一按键不能分配给多个动作。

也可通过 CLI 查看或控制当前来源：

```sh
lazymusic sources
lazymusic status
lazymusic ctl --op pause
lazymusic ctl --op quality --quality high
lazymusic quit
```

## 常见问题

### Ubuntu 编译或播放缺少插件

先确认 Go 版本与上述依赖已安装，检查 GStreamer：

```sh
go version
pkg-config --modversion gstreamer-1.0
gst-inspect-1.0 playbin3
gst-inspect-1.0 avdec_aac
pactl info
```

缺少 `playbin3` 时检查 `gstreamer1.0-plugins-base`；缺少 `avdec_aac` 时检查 `gstreamer1.0-libav`。若 `apt` 找不到这些包，检查是否启用了 Ubuntu 的 Universe 仓库。无声音或无法调整音量时，检查当前用户的 PipeWire / PulseAudio 会话和默认输出设备，使用普通用户运行 `lazymusic`。

### 在哪里看错误？

按 `@` 查看运行日志。warning / error 提示会在 3 秒后自动消失，不改变焦点，日志仍保留；切换页面时的正常请求取消不会作为错误上报。私有状态目录中的 `runtime.log` 保留轮转日志，`service.log` 用于服务启动诊断。提交 issue 前请检查日志中没有个人信息或凭据。

Apple 多设备播放冲突如果由服务返回明确错误，会显示脱敏后的原因；没有明确原因时，不把普通暂停猜测为多端冲突。

### 为什么没有 Apple Music Hi-Res 选项？

当前音源链路提供 AAC，曲目的无损标签不等于实际播放格式。macOS 原生 MusicKit 的实测也尚未得到 ALAC 解码证据。现阶段不显示未验证的音质选项；入口仍为 `b`，未来由后端能力扩展。见 [音质与验证记录](docs/audio-quality.md)。

### 更新后无法连接服务？

旧后台进程可能仍在运行旧协议。先退出 TUI，运行 `lazymusic quit`，再重新进入；自定义来源或状态目录应传入相同参数。同一来源同时只允许一个 TUI 持有界面写入权。

## 参与开发

```sh
make build
make test
make test-nvim  # 修改 Neovim 插件时运行，需要 nvim
make vet
make help
```

代码分为平台无关的浏览与播放合约、来源实现、后台服务和 TUI；新增平台从 backend registry 接入。工程结构、session / 安全约定及新来源接入步骤见 [AGENTS.md](AGENTS.md)。

欢迎通过 [Issues](https://github.com/lixuan998/lazymusic/issues) 报告问题或讨论新来源。请附上系统、架构、终端、Go 版本和复现步骤。提交改动前运行检查；不要提交登录凭据、CDM、辅助 binary、一次性探针或构建产物。

## 致谢

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) 与 [Lip Gloss](https://github.com/charmbracelet/lipgloss)：终端交互与绘制。
- [Vibez](https://github.com/simonepelosi/vibez)：当前 Apple 授权组件与固定版本的原生播放代码子集。来源清单见 [vibez-source.json](third_party/vibez-source.json)，保留其 [MIT 许可证](third_party/VIBEZ-LICENSE)。
- Chromium CDM 接口头文件遵循 [BSD 3-Clause](third_party/CHROMIUM-CDM-BSD-3-Clause.txt)。下载的 CDM 组件不随源码分发。
