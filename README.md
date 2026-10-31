# lazymusic

**把音乐留在终端。** 支持 macOS、Linux 和 Windows x64 的音乐 TUI，连接 Apple Music 或本地音乐库，默认使用 Vim 键位。

浏览推荐、搜索音乐、整理队列，再切到黑胶播放台看同步歌词。关闭界面后音乐继续播放，重新打开恢复原来的页面和播放状态。

[界面演示](#界面演示) · [安装](#安装) · [Windows](#windows-原生构建与运行) · [快速上手](#快速上手) · [键位](#键位) · [使用指南](#使用指南) · [Neovim](#neovim) · [配置与命令行](#配置与命令行) · [常见问题](#常见问题)

## 功能

- **Apple Music / LOCAL**：按音源能力显示页面；Apple Music 提供 Home 个性化推荐、New、Radio、Library、Playlists，以及 catalog / library 双范围搜索。
- **原生播放**：macOS、Linux 和 Windows 播放时均不依赖浏览器；支持歌曲、动态电台、电台节目，以及 Content 内的 Apple Music 视频。
- **队列整理**：加入队列、下一首播放、拾取与放置、整组插入、撤销与重做，以及定位当前播放歌曲。
- **后台与 session**：界面退出后继续播放；保存页面、分类、光标、队列、播放位置、布局与音效设置。
- **黑胶播放台与歌词**：高清机身、圆形旋转封面、随播放状态变化的唱针与指示灯，歌词自动滚动并高亮当前句。
- **音效与布局**：十段均衡器、Pulse / Wave 律动、Zen 模式、可调分栏和精简布局；音量键直接控制系统默认输出。
- **终端与 Neovim**：独立 TUI 和浮动终端共用交互；`?` 查看当前面板与 Global 命令，`@` 查看运行日志。

## 界面演示

以下 GIF 来自真实终端会话。Apple Music 使用实际目录数据；LOCAL 播放演示使用临时静音 WAV。录制后的键位调整以[当前键位](#键位)及界面内 `?` 为准。

**律动与 Zen**：Pulse 渐变柱形、峰值余辉和 Wave 曲线，播放 / 暂停平滑过渡。

![真实操作：Pulse、Wave、Zen 和暂停恢复](docs/media/rhythm.gif)

**布局与音效**：切换完整 / 精简布局、调整面板大小、设置 EQ 和查看日志。

![真实操作：布局、窗口调节、均衡器与日志](docs/media/layouts-equalizer.gif)

<details>
<summary>更多演示：Apple Music 搜索、队列播放和后台恢复</summary>

**搜索与作品信息**：搜索目录，跳转专辑，查看可用 credits，再切换搜索类型。

![真实操作：Apple Music 搜索、专辑跳转与 credits](docs/media/apple-music-catalog.gif)

**队列与播放**：加入歌曲，调整播放模式，快进、暂停和切歌。

![真实操作：队列、播放模式、快进、暂停与切歌](docs/media/queue-playback.gif)

**后台恢复**：退出界面保持播放，再次进入恢复 session。

![真实操作：后台播放与 session 恢复](docs/media/background-session.gif)

</details>

## 安装

可从 [GitHub Releases](https://github.com/lixuan998/lazymusic/releases/latest) 下载预编译包，运行时无需安装 Go 或编译器。

| 平台 | Release 包 | 安装方式 |
| --- | --- | --- |
| macOS Apple Silicon（M 系列，macOS 13+） | `lazymusic_<版本>_darwin_arm64.tar.gz` | 解压后按包内 README 安装 |
| Windows 10/11 x64 | `lazymusic_<版本>_windows_amd64.zip` | 解压后运行 `lazymusic.exe` |
| Ubuntu 22.04+ amd64 | `lazymusic_<版本>_amd64.deb` | `sudo apt install ./lazymusic_<版本>_amd64.deb` |

每个包包含安装说明、Neovim 插件、许可证及构建信息；Release 的 `SHA256SUMS` 可校验下载完整性。
macOS 包使用 ad-hoc 签名，未经 Developer ID 签名/公证；Windows 程序未做 Authenticode 签名。
授权 helper、Widevine CDM、用户凭据和会话均不包含在包内。

从源码安装需要 **Go 1.25+、CGO 和 C/C++ 工具链**。Apple Music 授权组件和 CDM 支持 macOS Apple Silicon / Intel、Linux / Windows x86_64；首次使用按需下载。

### Windows 原生构建与运行

支持 **Windows 10/11 x64**，可以直接编译、运行 `lazymusic.exe`，不依赖 WSL。
音频使用 Windows Media Foundation 解码和 waveOut 输出，十段 EQ 在播放过程中生效，
音量键控制系统默认输出设备；后台通过当前用户私有的命名管道通信。
Windows 无需 GStreamer；N/KN 版本需要安装系统的 Media Feature Pack。

**编译依赖**：安装 [Go 1.25+](https://go.dev/dl/) 和 64 位 MinGW-w64 GCC。
可选用 [MSYS2 UCRT64](https://www.msys2.org/) 或
[w64devkit](https://github.com/skeeto/w64devkit/releases)，将 Go 和 GCC 的 `bin` 目录加入 PATH。
使用 MSYS2 时，在 **UCRT64 shell** 中安装工具链：

```sh
pacman -S --needed mingw-w64-ucrt-x86_64-gcc
```

随后在 **PowerShell** 中进入仓库，编译并启动：

```powershell
git clone https://github.com/lixuan998/lazymusic.git
cd lazymusic
# 使用 w64devkit 时改成它的 bin 目录；Go 的 bin 目录也需要在 PATH 中
$env:PATH = "C:\msys64\ucrt64\bin;$env:PATH"
.\build.ps1
.\bin\lazymusic.exe

# 直接配置并打开本地音乐库
.\bin\lazymusic.exe --source local --music-dir 'D:\Music'
```

`build.ps1` 会为本次构建开启 CGO，使用 GCC/G++，输出 `bin\lazymusic.exe`。
已有编译好的程序时，运行它无需安装 Go 或 GCC。
测试和静态检查同样可以在 PowerShell 中执行：

```powershell
.\build.ps1 -Task test
.\build.ps1 -Task vet
```

**从 WSL 仓库构建**：也可用 PowerShell 进入
`\\wsl.localhost\<发行版>\home\<用户名>\Workspace\lazymusic` 执行脚本。
脚本会将源码暂存到 Windows 临时目录构建，再把程序放回仓库 `bin`，避免 Go 在 UNC 上的文件锁错误。
这里生成的仍是 Windows 程序；在 WSL shell 中运行 `make build` 生成的是 Linux 程序。
如果 PowerShell 因 UNC 路径或未签名脚本阻止执行，可仅为本次构建运行
`powershell -NoProfile -ExecutionPolicy Bypass -File .\build.ps1`；不会修改系统执行策略。

**安装与卸载**：

```powershell
.\build.ps1 -Task install
# 需要 Neovim 插件时，改用以下安装命令
.\build.ps1 -Task install -InstallNvim
.\build.ps1 -Task uninstall
```

默认程序位置是 `%LOCALAPPDATA%\Programs\lazymusic\bin\lazymusic.exe`，自行将其 `bin` 目录加入 PATH。
`-InstallNvim` 同时部署 Lua 插件和帮助，并生成 `%LOCALAPPDATA%\nvim\lua\plugins\lazymusic.lua`；
已有不同内容会先备份。`-Prefix`、`-NvimConfigDir` 可覆盖路径。
卸载只移除程序，保留插件、配置、授权和会话。更新前先退出 TUI 并停止对应后台服务，再拉取代码重新安装。

**图片与终端**：推荐使用支持 Sixel 的 Windows Terminal 1.22+。
默认自动探测 Kitty/Sixel，封面和黑胶播放台支持 Sixel；Sixel 使用最多 256 色整帧输出，
旋转与视频上限 10 FPS，实际速度取决于终端和连接。
需要指定 Sixel 时，在启动程序前设置：

```powershell
$env:LAZYMUSIC_IMAGE_PROTOCOL = "sixel"
.\bin\lazymusic.exe
```

没有确认图片协议或无法取得字符格像素尺寸时，Turntable 使用字符图像回退，视频保留音轨。
Sixel 目前仅支持直接终端；Neovim 浮窗仍要求外层终端支持 Kitty 图像协议。
更多选项见[图片与终端兼容](#图片与终端兼容)。

**配置与后台**：默认配置位于 `%APPDATA%\lazymusic`，LOCAL 默认使用其下的 `profiles\local`，
支持原有 `LAZYMUSIC_STATE_DIR` 和 `--state-dir`。`q` 打开提示后，再按 `q` 保留后台播放、`Q` 停止当前来源服务。
`lazymusic status`、`lazymusic ctl` 和 `lazymusic quit` 在 Windows 上同样可用。

**Apple Music 支持边界**：登录使用固定版本、SHA-256 校验的官方 `vibez_windows_amd64.zip`；
`LAZYMUSIC_VIBEZ_HELPER` 可指向自备 `vibez.exe`。Widevine 文件为 `widevinecdm.dll`，
支持自动准备及 `LAZYMUSIC_CDM_PATH`。当前 Apple 后端只声明 AAC 音质，本地格式取决于系统解码器。
Windows 已验证构建、测试、TUI 启动、本地静音播放与控制、后台通信及真实 CDM 加载/挑战生成；
真实账号登录、订阅歌曲、电台和受保护视频播放尚未实测。

### macOS

安装 Xcode Command Line Tools 和 [Go](https://go.dev/doc/install)：

```sh
xcode-select --install
go version
```

音频使用系统 Core Audio / AVFoundation，无需安装 GStreamer。

### Ubuntu / WSL

安装编译、解码和音频输出依赖：

```sh
sudo apt update
sudo apt install -y \
  build-essential pkg-config git ca-certificates \
  libgstreamer1.0-dev gstreamer1.0-tools \
  gstreamer1.0-plugins-base gstreamer1.0-plugins-good \
  gstreamer1.0-libav gstreamer1.0-pulseaudio \
  pulseaudio-utils
```

另行安装 [Go 1.25 或更新版本](https://go.dev/doc/install)，用 `go version` 确认；系统仓库中的 `golang-go` 可能低于要求。

| 依赖 | 用途 |
| --- | --- |
| `build-essential`、`pkg-config`、`libgstreamer1.0-dev` | CGO / C++ 编译和 GStreamer 链接 |
| `gstreamer1.0-plugins-base`、`gstreamer1.0-plugins-good` | 播放管线、常用格式及均衡器 |
| `gstreamer1.0-libav` | AAC 等音频解码 |
| `gstreamer1.0-pulseaudio` | 连接 PulseAudio 或 PipeWire 的兼容服务 |
| `pulseaudio-utils` | 提供系统音量工具 `pactl`；已有 `wpctl` 时优先使用它 |

运行时需要当前用户可访问的音频设备和音频服务。WSL 需要可用的音频转发；只有 SSH、没有音频输出的服务器需先配置输出环境。使用普通用户运行 lazymusic。

### macOS / Linux 编译与安装

```sh
git clone https://github.com/lixuan998/lazymusic.git
cd lazymusic
make install
```

默认安装以下内容，无需 `sudo`：

| 路径 | 内容 |
| --- | --- |
| `~/.local/bin/lazymusic` | 可执行文件 |
| `~/.local/share/lazymusic/nvim` | Neovim 插件和帮助 |
| `~/.config/nvim/lua/plugins/lazymusic.lua` | 自动创建 / 更新的 lazy.nvim 配置；遵循 `XDG_CONFIG_HOME` |

已有 Neovim 配置内容不同时，先保存同目录下的 `lazymusic.lua.bak-*`；相同内容重复安装不会生成新备份。使用 lazy.nvim / LazyVim 时，重启 Neovim 后可用 `<leader>mm` 打开。

若安装目录尚未加入 `PATH`，将下面的 `export` 加入 `~/.zshrc` 或 `~/.bashrc`：

```sh
export PATH="$HOME/.local/bin:$PATH"
lazymusic
```

其他安装方式：

```sh
make build                                     # 只构建 bin/lazymusic
./bin/lazymusic
make install INSTALL_NVIM=0                     # 仅安装 binary
make install PREFIX="$HOME/Applications/lazymusic"
make install BINDIR="$HOME/bin"
make install NVIM_CONFIG_DIR="$HOME/.config/nvim-work"
```

`make install` 自动先编译，支持 `GO`、`INSTALL`、`PREFIX`、`BINDIR`、`NVIM_CONFIG_DIR`、`NVIM_PLUGIN_DIR` 和 `DESTDIR`。`DESTDIR` 用于打包暂存，生成的 Neovim 配置引用最终安装路径。

### macOS / Linux 更新与卸载

退出 TUI、停止后台服务后更新：

```sh
lazymusic quit
git pull --ff-only
make install
lazymusic
```

如两个来源都曾启动服务，可分别运行 `lazymusic quit --source apple` 和 `lazymusic quit --source local`。使用自定义状态目录时传入相同的 `--state-dir`。更新插件后重启 Neovim，加载新的 Lua 模块。

```sh
make uninstall
# 自定义安装位置时使用相同参数，例如：
make uninstall BINDIR="$HOME/bin"
```

卸载仅移除 binary，保留插件、配置、登录和 session。需要清除当前来源的授权与 session 时，可在卸载前运行 `lazymusic logout`；LOCAL 的音乐文件不会被删除。

## 快速上手

1. 运行 `lazymusic`，首次进入 Content 中的 **Login** 页面，用 `j/k` 选择 **APPLE MUSIC** 或 **LOCAL**，`Enter` / `l` 确认。
2. Apple Music 按提示完成网页授权；LOCAL 默认使用 `~/Music`，也可在 Login 中修改目录。
3. `1` 聚焦 Navigation，用 `j/k` 选择页面，`Enter` / `l` 进入 Content。移动到 Login 不会自动发起授权。
4. Content 用 `,/.` 切分类、`h/l` 进退层级；歌曲 `Enter` 播放，集合 `Enter` 打开、`P` 播放全部，`y` 加入队列。
5. `Space` 暂停 / 继续，`<` / `>` 上下曲，`m` 轮转播放模式。`3` 进入 Queue，`f` 找到当前歌曲。
6. `q` 打开离开提示，再按 `q` 后台播放、`Q` 完全退出当前来源服务；`Esc` 取消离开。

已有 session 时，启动会直接恢复。后台服务仍在运行则保持播放；服务已退出或系统重启后的冷恢复保留队列及位置，初始为暂停。

也可直接选择本地目录：

```sh
lazymusic --source local --music-dir "$HOME/Music"
```

更换 LOCAL 根目录会清空旧队列。切换来源会暂停之前的来源，各自保存队列和界面；之后从 Navigation → Login 或 `:login` 切换。

## 键位

按 `?` 查看 **当前面板 + Global** 的实际绑定；`j/k`、`Ctrl+u/d` 滚动帮助，`?` / `Esc` 关闭。以下是默认值，可通过 [`keys.json`](#配置与命令行) 修改。

普通导航键立即响应，长按只在当前面板连续移动；没有连按切窗。输入搜索词、目录和歌单名称时，普通字符保持文本含义，`Esc` / `Ctrl+c` 取消输入。最低 TUI 尺寸为 **60 × 16**。

### Global

| 按键 | 操作 |
| --- | --- |
| `Ctrl+h/j/k/l` | 焦点移向左 / 下 / 上 / 右面板 |
| `1` / `2` / `3` / `4` / `5` / `6` | Navigation / Content / Queue / Player / Log / Sound |
| `Tab` / `Shift+Tab` | 下一个 / 上一个面板 |
| `H/J/K/L` | 向左 / 下 / 上 / 右调整当前面板尺寸 |
| `+` / `_` | 下一个 / 上一个布局：完整、Content + Player、Queue + Player |
| `Ctrl++` / `Ctrl+-` | 开关 Zen：律动或播放台 + Equalizer + Player |
| `Ctrl+=` | 恢复默认布局和面板尺寸 |
| `Space` / `x` | 播放或暂停 / 停止 |
| `<` / `>` | 上一曲 / 下一曲 |
| `Ctrl+<` / `Ctrl+>` | 后退 / 前进 10 秒，通常按 `Ctrl+Shift+,` / `Ctrl+Shift+.` |
| `m` | 轮转播放模式 |
| `=` / `-` | 系统默认输出音量增加 / 减少 5% |
| `@` | 打开 / 关闭运行日志 |
| `?` | 查看当前面板和 Global 帮助 |
| `:` | 输入命令，如 `:login`、`:seek 30`、`:vol 50` |
| `q` → `q` / `Q` | 确认后台运行 / 完全退出当前来源服务 |

尺寸调节使用普通大写字母，即 `Shift+h/j/k/l`。修饰键操作需要终端正确上报按键；若被终端自己的快捷键拦截，应调整终端绑定。

### 面板内命令

列表导航：`j/k` 上下，`gg/G` 首尾，`Ctrl+u/d` 半页。Content 的 `h/l` 返回上级 / 打开下级；`Enter` 打开集合或播放歌曲。

| 面板 | 按键 | 操作 |
| --- | --- | --- |
| 1 Navigation | `j/k`、`Enter` / `l` | 选择页面、进入 Content |
| 1 Navigation | `,/.` | 切换下方 Sound 视图 |
| 2 Content | `,/.` | 切换推荐分类、Library 分类或搜索类型 |
| 2 Content | `r`、`/` | 刷新、过滤已加载的内容 |
| 2 Content · Search | `i`、`s` | 编辑搜索词、切换 catalog / library 范围 |
| 2 Content | `v` | 在正在播放的视频与浏览页面之间切换 |
| 2 Content · Video | `Enter`、`h` / `Esc` | 播放 / 暂停、返回浏览；全局播放键继续生效 |
| 2 Content | `P`、`y` | 播放当前歌曲 / 整个集合、加入队列 |
| 2 Content | `p` | 拾取单曲或集合，之后在 Queue 放置 |
| 2 Content | `a`、`d` | 添加菜单、移除入口；Playlists 列表中 `a` 新建歌单，删除受来源能力限制 |
| 2 Content / 3 Queue | `u` | 撤销最近一次放置，再按重做 |
| 2 Content / 3 Queue / 4 Player | `o`、`c` | 跳转专辑 / 艺人、查看 credits |
| 3 Queue | `Enter`、`f` | 播放选中歌曲、定位当前播放项并尽量居中 |
| 3 Queue | `p`、`Esc` | 拾取；再次 `p` 放到光标歌曲之后；`Esc` 取消拾取 |
| 3 Queue | `[` / `]` | 将选中歌曲上移 / 下移 |
| 3 Queue | `a`、`d`、`D` | 添加菜单、移除队列项、清空队列 |
| 3 Queue / 4 Player | `r`、`s` | 轮转循环模式、开关随机 |
| 4 Player | `Enter`、`,/.` | 播放 / 暂停、前后轮转播放模式 |
| 4 Player | `b`、`a` | 选择音质偏好、为当前歌曲打开添加菜单 |
| 5 Log | `j/k`、`gg/G`、`Ctrl+u/d` | 滚动、首条 / 最新日志、半页滚动 |
| 6 Sound | `,/.` | Pulse / Wave / Equalizer / Turntable |
| 6 Sound · EQ | `h/l`、`j/k` | 选择频段、降低 / 提高 1 dB |
| 6 Sound · EQ | `e`、`r`、`p` | 旁路、归零、轮换预设 |
| 2 Login | `j/k`、`Enter` / `l`、`h` | 选择来源、确认、返回 |
| 2 Login | `r`、`y` | 账户选项中重新授权、授权期间复制手动登录链接 |

同一字符按当前面板解释：Content 的 `r` 刷新，Queue / Player 的 `r` 调循环，Sound EQ 的 `r` 归零。`P` 只在 Content 用于播放整组；Queue 的默认放置方向为光标之后。

## 使用指南

### 音乐来源与音质

| 来源 | 页面与浏览 | 播放 |
| --- | --- | --- |
| APPLE MUSIC | Home、New、Radio、Library、Playlists、Search、Login；歌曲、专辑、艺人、歌单和视频搜索 | 需要有效订阅；AAC 64 / 256 kbps 偏好，支持歌曲、电台及音乐视频 |
| LOCAL | Library、Folders、Search、Login；按文件和目录组织、文件名搜索 | 直接读取原文件，解码能力取决于系统音频栈，无云端授权 |

Apple Music **仅登录授权时临时使用浏览器，播放不依赖浏览器**。当前使用固定版本、校验摘要的 Vibez 官方授权组件，Apple 授权页中的应用身份仍为 **Vibez**，依赖其授权服务。首次使用下载授权组件和 CDM，凭据保存在本机私有目录。

Player 中按 `b` 选择后端支持的音质。Apple 的 AAC 64 / 256 kbps 是请求偏好，实际流以服务返回为准；当前 macOS、Linux 和 Windows 都没有已验证的 ALAC 无损 / Hi-Res 播放路径，因此不提供这些选项。曲目的音质标签不等于正在播放的格式。

LOCAL 识别 MP3、M4A、AAC、WAV、AIFF、FLAC、OGG、Opus 扩展名，能否解码取决于系统与已安装插件。不转码，暂不读取嵌入式音乐标签，也不播放 Apple 下载的 DRM 文件。

### 推荐与电台

Home 按 Apple 返回的分类标题组织顶部标签，用 `,/.` 切换，分类内容支持分页。保留接口返回的全部分类和资源，包含专属电台及暂不支持播放的类型。分类随账号、地区和时间变化，接口未返回的模块可能与官方 App 不同，例如某些账号看不到特定的推荐分组。

New / Radio 只对 APPLE MUSIC 显示，读取账号地区对应的 Apple 官方公开页面结构化内容；有入口的分类可用 **View all** 展开。不需要运行浏览器，也不向公开页面发送账号授权。页面结构变化或地区重定向会报告错误；视频等未支持播放的内容仍然可见。

| 类型 | 操作与行为 |
| --- | --- |
| 个人、发现、艺人、风格等动态电台 | `Enter` 或 `P` 调台，后台自动补充下一批歌曲；`y/p` 不将无限电台展开为固定队列，可在 Queue / Player 操作当前歌曲 |
| 有限时长的电台节目 | `Enter` / `P` 播放，`y` 加入队列，`p` 拾取；支持定位进度和 session 恢复 |
| 直播台 | 已实现媒体窗口刷新，但当前账号 / 地区尚未完成真实直播播放验证，不保证可用 |

动态电台和 CBCS 电台节目已在 macOS、Linux 验证原生播放。Player 显示电台与当前歌曲，沿用全局播放控制、系统音量和 EQ。`q q` 后继续播；冷恢复暂停，继续播放后恢复电台续播。用普通作品替换队列或清空队列会结束电台模式。

### 拾取、放置与撤销

1. 在 Content 选中歌曲或集合，按 `p` 拾取；或在 Queue 按 `p` 拾取已有歌曲。界面以 `◆` 和状态提示标记已拾取内容。
2. 切到 Queue，将光标移到目标歌曲，按 `p` 放到它后面；空队列直接插入。`Esc` 取消尚未提交的拾取。
3. 在 Content / Queue 按 `u` 撤销最近一次成功放置，再按 `u` 重做；下一次成功放置更新记录。

Queue 内拾取是移动；Content 拾取是复制加入。专辑 / 歌单展开全部歌曲，艺人展开其专辑及歌曲，包含分页，按来源顺序整体插入。集合准备期间可继续导航，失败不会部分插入；提交后需等待结果。

放置保留当前播放歌曲与进度，`f` 可随时定位当前项。靠近队列首尾时居中位置按边界调整，普通浏览不会随自动切歌抢走光标。

撤销记录随 session 保存，整组插入一起撤销。若撤销移除了当前正在播放的插入项，会停止它并选择相邻项；重做不自动播放。队尾追加保留撤销记录，其他删除、重排或替换队列会使其失效。尚未放置的拾取内容只在当前前端保留。

### 收藏、歌单与作品信息

- 歌曲 / 专辑按 `a` → **Music library** 添加收藏；Library 项目不再显示重复收藏入口。
- Playlists 列表按 `a`，输入名称并 `Enter` 新建歌单；空列表也可创建。
- Content / Queue / Player 的歌曲按 `a` → **Choose playlist**，选择可编辑歌单添加。
- `y` 直接加入播放队列；添加菜单中的 **Play next** 安排为下一首。
- `o` 列出专辑与每位艺人的跳转入口，`j/k` 选择、`Enter` / `l` 打开，Content 中 `h` 返回。
- `c` 查看来源实际提供的贡献者、作曲及发行信息；`j/k`、`Ctrl+u/d` 滚动，`Esc` 关闭。缺少完整 credits 时会注明。

Apple 返回 **202 Accepted** 表示已受理，列表同步可能稍有延迟，可在 Content 按 `r` 刷新。写入不会自动重试；连接失败时应先检查实际结果，再决定是否重试。

**当前 Apple 后端不支持删除云端收藏歌曲、专辑或歌单。** `d` 会解释限制，请使用 Apple Music 官方客户端完成删除。Queue 的 `d` 只移除播放队列项；LOCAL 不提供文件删除。LOCAL 按文件 / 目录组织，尚不读取嵌入式 credits。

### 音乐视频

Apple Music 的 Search → **Videos** 分类可搜索音乐视频；Home、New 或歌单返回的视频也可直接播放和加入队列。Videos 只提供 catalog 搜索，切到 library 搜索时回到 Songs。

选中视频按 `Enter` / `P` 播放，Content 自动显示画面。`v` 在视频与原来的浏览页面间切换，视频页 `h` / `Esc` 返回浏览。`Space`、`x`、`<` / `>`、`Ctrl+<` / `Ctrl+>` 仍分别控制暂停、停止、切歌和跳转。暂停保留画面，停止清除画面；后台运行继续播放音轨，重新进入后按当前进度恢复视频。

视频通过现有授权 CDM 的原生 H.264 解码器播放，不依赖浏览器或 FFmpeg；Windows 的实际账号验证边界见[Windows 支持说明](#windows-原生构建与运行)。仅使用服务明确允许软件播放的流；优先保留可用软件流的原始分辨率，输出上限为 **1280 × 720、30 FPS**。实际清晰度与帧率受 Apple 提供的授权版本、终端和网络吞吐限制；不会将低分辨率作品上采样成假 HD。慢连接只处理当前画面，不堆积旧帧。视频画面不写磁盘或 session，音轨沿用原生音频输出。后台预取一个加密视频片段；图片通过本地 IPC（Unix socket / Windows 命名管道）直接传输 PNG，并在支持的终端中更新同一张图片。Neovim 中继会合并一张图片的分块输出，减少逐帧开销。

画面需要 Kitty 或 Sixel 图片协议：例如直接运行于 Ghostty、Kitty、Windows Terminal 1.22+；Neovim 0.12+ 仍需外层终端支持 Kitty 并配合更新后的内置图片中继。Sixel 使用最多 256 色整帧输出，视频上限 10 FPS，实际速度取决于终端和连接。其他终端显示说明并继续音轨，不把视频转成字符画。原有 Turntable 在不支持图片协议时仍使用半格回退图。

实际可播放作品取决于订阅、地区和 Apple 返回的授权。视频 CDN `mvod.itunes.apple.com` 返回 **403** 时，程序会自动通过 [Google Public DNS](https://developers.google.com/speed/public-dns/docs/doh/json) 查询备用节点；解析不可用或返回的节点仍失败时，再尝试 [Cloudflare](https://developers.cloudflare.com/1.1.1.1/encryption/dns-over-https/make-api-requests/dns-json/)，最多尝试四个不同节点。清单、画面片段和配套音轨共用这一回退。

备用解析只发送 CDN 域名，不发送账号凭据或媒体 URL。备用请求在进程内直连选出的 CDN IP，保留原签名 URL、Range 和 HTTPS 主机名校验；不会改系统 DNS、代理设置或其他网络请求。成功节点只在当前来源服务的内存中短暂保存，最长 5 分钟且不超过 DNS TTL，失败时重新选择。备用节点连接或读取超时会暂时避开；未完整读取的片段最多重读一次，完整拿到数据后才交给解码器。`@` 日志记录自动切换和恢复结果，不记录签名地址或节点 IP。

如果原路径和备用节点都拒绝请求，仍显示真实错误；不会重试登录、许可证或内容编辑请求，也不会无限重试。可用同一作品在官方客户端核对订阅与地区限制。

### Turntable 与同步歌词

聚焦 Navigation 或 Sound，用 `,/.` 切到 **Turntable**。留声机使用内嵌高清机身与当前歌曲封面，圆形封面铺满唱片，仅留窄黑边。

| 播放状态 | 封面 | 唱针 | 指示灯 |
| --- | --- | --- | --- |
| 播放 | 顺时针旋转 | 唱片上 | 亮 |
| 暂停 | 保留当前角度 | 唱片上 | 灭 |
| 停止 | 隐藏封面，显示默认唱片 | 移回停靠位 | 灭 |

下方显示同步歌词，当前句居中高亮，前后句随进度滚动；快进 / 回退重新定位。无可用时间轴时显示 `No Lyrics Available`。

| 来源 | 封面 | 歌词 |
| --- | --- | --- |
| APPLE MUSIC | 自动读取当前歌曲封面 | LRCLIB 精确元数据查询，超时或无同步结果时回退网易云音乐公开歌词；严格比对曲名、艺人和时长 |
| LOCAL | 同目录的 `cover.jpg/jpeg/png`、`folder.jpg/png`、`front.jpg/png`，文件名不区分大小写 | 同目录、同名的 UTF-8 `.lrc`，如 `song.flac` 对应 `song.lrc`，文件名不区分大小写 |

公共歌词查询仅发送曲名、艺人、专辑和时长，不发送 Apple 凭据或音频。无歌词、版本不匹配或服务不可用时显示缺省提示。缺少封面时使用默认唱片；歌词与封面只缓存在内存中，切歌、切换来源及隐藏播放台时取消旧请求。

原图显示条件见 [Neovim 图片与终端兼容](#图片与终端兼容)。机身与封面统一合成为图片后缩放；支持时只更新唱片区域，慢连接不会堆积历史动画帧。无法使用图片协议时，整台播放台回退为彩色像素图。

### 布局、律动与均衡器

完整布局将左侧分为 Navigation 和 Sound，中间 Content、右侧 Queue，底部 Player；Log 按需打开。`+` / `_` 轮换完整、Content + Player、Queue + Player；`H/J/K/L` 调整当前面板，`Ctrl+=` 恢复默认布局。

`Ctrl++` / `Ctrl+-` 开关 **Zen**：左侧 Pulse / Wave / Turntable、右侧 Equalizer、底部 Player。聚焦 Sound 后 `,/.` 切左侧视图，`h/l`、`j/k` 调 EQ。普通 Sound 则在 Pulse / Wave / Equalizer / Turntable 四种视图间切换。

- **Pulse / Wave**：渐变柱形、缓降峰值与多层细曲线，前端独立 30 FPS 更新；暂停平滑收束，隐藏时停止动画。它们是装饰律动，没有实时分析音频频谱。
- **Equalizer**：32 Hz–16 kHz 十段，±12 dB，调整实际音频输出；提供 Flat / Bass / Vocal / Treble 预设和旁路，设置随 session 保存。
- **Player**：分行显示曲目、艺人 / 专辑、加粗进度条、状态图标、系统音量及模式 / 音质；小终端自动收紧。

## Neovim

随附插件支持 **Neovim 0.10+**，用 `<leader>mm` 打开浮动终端。macOS/Linux 使用 `make install`，Windows 使用 `.\build.ps1 -Task install -InstallNvim`，自动部署插件并生成 lazy.nvim 配置，从任何工作目录均可使用，无需在工程根目录启动 Neovim。

| 操作 | 行为 |
| --- | --- |
| `<leader>mm` / `:LazyMusic` | 打开或聚焦现有终端 |
| `:LazyMusicHide` | 只隐藏浮窗，保留同一 TUI 进程 |
| TUI 内 `q q` | 退出前端并关闭浮窗，后台继续播放；再次打开恢复 session |
| TUI 内 `q Q` | 完全退出当前来源服务 |
| `Ctrl+反斜杠`，再 `Ctrl+n` | 返回 Neovim 普通模式 |

插件透传焦点、布局、跳转进度等按键，只作用于音乐终端 buffer。音乐命令与独立 TUI 相同，`?` 反映同一套实际绑定。帮助入口为 `:help lazymusic`。

### 手动接入 lazy.nvim

如使用 `make install INSTALL_NVIM=0` 跳过自动配置，可将以下内容加入插件列表；LazyVim 可保存为 `lua/plugins/lazymusic.lua`：

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
      cmd = "lazymusic", -- 先查 PATH，再查 ~/.local/bin/lazymusic
      args = {},        -- 例如 { "--source", "local" }
      width = 0.84,     -- 可用空间比例，也可使用字符格数
      height = 0.82,
      border = "rounded",
    },
  },
}
```

插件仍需要已安装的 binary；`opts.cmd` 可指定完整路径，`opts.args` 逐项传参，不经过 shell。使用本地 checkout 时，将仓库声明替换为 `dir = "/absolute/path/to/lazymusic"`。`opts.keymap = false` 可关闭插件自身设置的普通模式快捷键；完整选项见[插件帮助](docs/lazymusic.txt)。

### 图片与终端兼容

| 运行环境 | Turntable 显示 |
| --- | --- |
| 直接运行于 Ghostty / Kitty | 探测成功后通过 Kitty 图片协议显示原图 |
| 直接运行于 Windows Terminal 1.22+ 或其他 Sixel 终端 | 确认 Sixel 能力与像素尺寸后显示最多 256 色图片；整帧重绘 |
| Neovim 0.12+ TUI，开启 `termguicolors`，外层终端支持 Kitty | 随附插件中继原图，需要同时更新 binary 与插件 |
| Neovim 0.10 / 0.11、无 TTY 的 GUI、tmux / zellij 或不支持协议的终端 | 彩色像素图回退 |

图片可通过 SSH / WSL 传输，不要求图片文件位于外层终端所在机器。同一 Neovim 宿主终端已确认的支持能力会在后台退出、重新打开时复用；缓存命中的封面、动画错误或视图切换不会撤销已确认的支持。探测丢包会有限重试，迟到的成功响应也能恢复原图。

默认 `LAZYMUSIC_IMAGE_PROTOCOL=auto` 同时探测 Kitty 与 Sixel，收到 Kitty 成功响应后优先使用 Kitty。
Sixel 通过 DA1 的 `4` 能力标记确认，并查询字符格像素尺寸（或通过窗口像素尺寸计算），
不根据 `TERM` 名称猜测支持。缺少可靠尺寸或探测失败时回退字符图像。
设置 `LAZYMUSIC_IMAGE_PROTOCOL=kitty` / `sixel` 可限制探测的协议，`pixel` 强制字符回退；
原有 `LAZYMUSIC_IMAGE_DISABLE=1` 仍优先禁用图片。

Windows PowerShell 中可显式选择 Sixel：

```powershell
$env:LAZYMUSIC_IMAGE_PROTOCOL = "sixel"
.\bin\lazymusic.exe
```

Sixel 封面旋转和视频最多 10 FPS，按实际字符格像素大小缩放并保留图像比例。
同一时间只准备、输出一帧；文本刷新后重绘缓存图片，弹窗、隐藏、换来源和退出只清理本程序占用的区域。
Sixel 目前仅支持直接终端运行；Neovim 浮窗继续使用现有 Kitty 中继，未扩展其允许的控制序列。

更新后运行 `make install` 并**重启 Neovim**，避免旧 Lua 模块继续运行；确认 `:set termguicolors?` 已开启。需强制像素图时使用 `LAZYMUSIC_IMAGE_DISABLE=1`。

## 配置与命令行

| 位置或选项 | 含义 |
| --- | --- |
| macOS：`~/Library/Application Support/lazymusic` | 默认私有状态目录 |
| Linux：`$XDG_CONFIG_HOME/lazymusic`，未设置时为 `~/.config/lazymusic` | 默认私有状态目录 |
| Windows：`%APPDATA%\lazymusic` | 默认状态目录；LOCAL 默认使用 `profiles\local` 子目录 |
| `--state-dir PATH` / `LAZYMUSIC_STATE_DIR` | 指定精确状态目录 |
| `--source apple` / `--source local` | 选择来源；未指定时沿用已选来源 |
| `--music-dir PATH` | 配置 LOCAL 根目录，与 `--source local` 一起使用 |
| `keys.json` | 当前来源状态目录内的自定义键位 |

默认 Apple 使用主状态目录，LOCAL 使用其下的 `profiles/local`，授权、队列及 session 按来源隔离。显式状态目录按指定路径使用，不自动追加 profile；不同来源应使用不同目录。

`keys.json` 以动作名覆盖默认绑定，例如为帮助增加 `F1`：

```json
{
  "help": ["?", "f1"]
}
```

动作名与默认值以 [`keys.go`](internal/tui/keys.go) 为准。不同面板可复用按键，同一面板内以及与 Global 之间不能冲突，启动时会校验。不要沿用已经删除的动作名。

常用 CLI：

```sh
lazymusic sources
lazymusic status
lazymusic ctl --op pause
lazymusic ctl --op seek --delta 10s
lazymusic ctl --op quality --quality high
lazymusic quit
```

以上控制当前来源；指定其他来源时添加 `--source apple|local`，指定自定义 session 时添加相同的 `--state-dir`。`status` 输出 JSON，`ctl` 连接或启动对应后台服务。`lazymusic help` 查看入口说明；TUI 内可用 `:login`、`:play`、`:pause`、`:stop`、`:next`、`:previous`、`:seek 30`、`:vol 50`、`:detach` 等命令。

## 常见问题

### WSL 登录时无法自动打开浏览器怎么办？

在 Login 中选择 APPLE MUSIC 并确认。首次运行可能先下载组件，界面显示准备阶段；授权服务就绪后显示完整手动链接，并尝试自动打开浏览器。用 `y` 请求复制，或手动选择屏幕上的完整地址，保持 lazymusic 运行并在浏览器中完成授权。

WSL 可在 Windows 浏览器中打开链接，需要 localhost 转发可用。通过 SSH 登录远程主机时，需要把链接中的同一 localhost 端口转发到浏览器所在机器。链接只对本次授权有效，取消、完成或超时后清除，不保存到日志或 session。

终端复制需要 OSC 52；Neovim 使用其配置的 clipboard provider。不支持自动复制时仍可手动复制显示的地址。

### Ubuntu 22.04 还没显示链接就登录失败？

官方授权组件可能因旧 C++ 运行库缺少 `GLIBCXX_3.4.32` 而启动失败。lazymusic 检测到此问题时，会自动下载并校验固定版本的 conda-forge GCC 兼容库，保存在当前来源的 `helpers/runtime/`，只供授权子进程使用；不升级系统库、不需要安装 Conda。首次下载需要访问 `conda.anaconda.org`。

若仍失败，Login 会保留具体原因；`@` 查看日志。缺少 GStreamer 库时检查 Ubuntu 依赖，网络或证书错误按提示处理。

### Ubuntu 无声音或无法调整音量？

```sh
go version
pkg-config --modversion gstreamer-1.0
gst-inspect-1.0 playbin3
gst-inspect-1.0 avdec_aac
pactl info
```

缺少 `playbin3` 检查 base 插件，缺少 `avdec_aac` 检查 libav。确认当前用户的 PipeWire / PulseAudio 服务及默认输出设备可用；系统音量使用 `wpctl`，找不到时使用 `pactl`。音量操作作用于运行后台服务的机器。

### Windows 编译成功，但没有图片或声音？

图片需要终端支持对应协议。Windows Terminal 1.22+ 可使用 Sixel；可先设置
`$env:LAZYMUSIC_IMAGE_PROTOCOL = "sixel"` 再启动程序。
确保未设置 `LAZYMUSIC_IMAGE_DISABLE=1`，先在直接终端运行，避免经过 tmux/zellij 或 Neovim 浮窗。
缺少能力或像素尺寸回复时会回退字符图像；这不会阻止音频播放。

无声音时检查默认输出设备与系统音量；Windows N/KN 需要 Media Feature Pack。
LOCAL 解码格式取决于 Media Foundation；失败时用 `@` 查看运行日志。
如更换了状态目录，控制后台时需要使用相同的 `--source` 和 `--state-dir`。

### 有歌词的歌曲为什么仍显示 No Lyrics Available？

Apple Music 歌词来自独立公共服务，覆盖范围和版本可能与官方客户端不同；官方有歌词不代表公共服务有匹配的同步版本。确认歌曲版本一致且歌词服务可访问。LOCAL 则检查 `.lrc` 与音频同目录、同名、UTF-8 编码并包含时间戳。当前不显示没有时间轴的纯文本歌词。

### 在哪里查看运行错误？

`@` 打开日志。warning / error 提示 3 秒自动消失，不转移焦点；日志继续保留。状态目录下的 `runtime.log` 为轮转运行日志，`service.log` 用于启动诊断。正常取消页面请求不会报错。

Apple 多设备播放冲突只有在服务返回明确原因时显示，普通暂停不会被推断为冲突。提交 issue 前检查日志，避免包含个人信息或凭据。

### 为什么没有无损 / Hi-Res 选项？

当前 Apple 播放路径提供 AAC，macOS 原生 MusicKit 探索尚未得到实际 ALAC 解码证据。各平台都只展示已支持的音质偏好，不能用目录标签代替播放格式验证。LOCAL 直接播放原文件，解码支持取决于系统。

### 更新后连接失败，或提示已有界面连接？

旧后台进程可能使用旧协议：退出 TUI 后执行 `lazymusic quit`，再重新进入。来源和 `--state-dir` 应与原服务一致。同一来源的状态目录同时只允许一个前端连接；`:LazyMusicHide` 仅隐藏界面，不释放该连接，需用 `q q` 退出前端。

## 参与开发

发布包使用 GitHub Actions 在三个原生平台分别测试、构建和打包，流程见[发布说明](docs/RELEASING.md)。

Windows 在 PowerShell 中使用 `.\build.ps1`、`.\build.ps1 -Task test`、`.\build.ps1 -Task vet`；
仓库的 Windows CI 会执行相同检查。macOS/Linux 使用：

```sh
make build
make test
make test-nvim  # 需要 Neovim 0.10+；包括终端生命周期和图片中继回归
make vet
make help
```

| 目录 | 职责 |
| --- | --- |
| `cmd/lazymusic`、`internal/app` | CLI、应用生命周期、内嵌 Login 与来源切换 |
| `internal/music`、`internal/catalog`、`internal/backend` | 平台无关的数据、浏览 / 编辑合约与后端注册 |
| `internal/sources`、`internal/engine` | 来源浏览与原生播放实现 |
| `internal/daemon`、`internal/session`、`internal/eventlog` | 后台 IPC、持久化与日志 |
| `internal/tui` | 面板、交互、布局与动画 |
| `internal/artwork`、`internal/turntable`、`internal/termimage`、`internal/lyrics` | 封面、播放台合成、终端图片与同步歌词 |
| `lua/lazymusic`、`tests` | Neovim 插件与回归测试 |

新增来源从 backend registry 接入，UI 按能力显示功能。目录边界、接入步骤、session 和凭据处理约定见 [AGENTS.md](AGENTS.md)。

欢迎通过 [Issues](https://github.com/lixuan998/lazymusic/issues) 反馈问题或讨论功能，请附系统、架构、终端、运行方式（直接 / Neovim / tmux）、版本及复现步骤。提交前运行适用检查；不要提交授权信息、CDM、辅助 binary、一次性探针或构建产物。

## 致谢与第三方组件

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) 与 [Lip Gloss](https://github.com/charmbracelet/lipgloss)：终端交互与绘制。
- [Vibez](https://github.com/simonepelosi/vibez)：当前 Apple 授权组件与固定版本的原生播放代码子集；[来源清单](third_party/vibez-source.json)与 [MIT 许可证](third_party/VIBEZ-LICENSE)。
- [LRCLIB](https://lrclib.net/docs)、[网易云音乐](https://music.163.com)：公开歌词查询。
- [conda-forge GCC runtimes](https://github.com/conda-forge/ctng-compilers-feedstock)：旧版 Linux 授权兼容库，按需下载、不随 binary 分发；[GPLv3](https://www.gnu.org/licenses/gpl-3.0.html) 及 [GCC Runtime Library Exception 3.1](https://www.gnu.org/licenses/gcc-exception-3.1.html)。
- Chromium CDM 接口头文件采用 [BSD 3-Clause](third_party/CHROMIUM-CDM-BSD-3-Clause.txt)；下载的 CDM 不随源码分发。
