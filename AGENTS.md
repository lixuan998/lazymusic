# 工程维护约定

本文件适用于整个 lazymusic 工程。binary 必须叫 `lazymusic`；支持 macOS 和 Linux。用户的当前任务指令优先。

## 结构和边界

- `cmd/lazymusic/`：CLI 解析、信号与启动入口；不包含平台业务或绘制代码。
- `internal/app/`：单个 Bubble Tea application、内嵌 Login、来源配置与授权组件安装。异步消息使用来源 epoch，防止切换来源后旧响应污染新页面。
- `internal/app/auth_runtime.go`：仅为存在 C++ ABI 缺口的 Linux amd64 官方授权子进程准备固定、双重摘要校验的兼容运行库。来源为 conda-forge GCC，保存在来源私有 helpers/runtime 中；只解包指定的普通库文件，不执行包脚本、不安装系统包。通过系统 ELF loader 的 --library-path 启动，不导出 LD_LIBRARY_PATH，避免影响浏览器、父进程或播放后端。不得以兼容 C++ 的方式替换 glibc。
- `internal/nvimconfig/`：为已安装的 Lua 插件生成 lazy.nvim 配置，幂等更新、原子替换并备份不同的旧内容；运行路径不得包含 DESTDIR。
- `internal/music/`：平台无关的 Source、Capabilities、Quality、Track、Query、State；曲目标识是 `(Source, ID)`。Source.Pages 声明来源专有的导航根节点与 opaque Query，UI 不判断平台名称添加页面。
- `internal/catalog/`：Browser、Page、Item、Tab、Inspector 和可选 Editor 合约及有界集合分页。Item.Open/Play、Actions.Playlists 为后端提供的 opaque Query；Item.Writable 仅表示可编辑的目标歌单。
- `internal/sources/apple/`、`internal/sources/local/`：浏览、搜索、推荐与目录配置。任何服务 URL/资源 ID 解释均放在对应后端。
- `internal/backend/registry.go`：来源注册与组合点，定义描述、能力、可用音质、Browser 与 Engine 工厂。
- `internal/engine/`：播放合约、Native Apple 适配器、LOCAL 播放器和供 LOCAL 使用的 Queue 元数据状态机。Queue 本身不输出音频，不对用户暴露模拟后端。
- `internal/engine/continuous.go`：动态电台控制层，后台有界取歌、低水位续播、保留约 200 首历史；网络请求不持有控制锁，generation 拦截旧电台回复，Pause/Stop 禁止迟到请求自动发声。session.Playback.Continuous 保存稳定的 opaque Query，冷恢复暂停。通过可选 StartContinuous / Select 保留来源中立的 IPC 和 Queue 选择行为。
- `internal/audio/`：LOCAL 系统原生音频输出；`internal/volume/`：系统默认输出设备音量，两者职责独立。
- `internal/daemon/`：Unix socket HTTP、进程锁、单前端 lease、UI revision、串行播放命令、日志和快照。协议破坏性变更递增 ProtocolVersion。
- `internal/session/`：版本化 JSON 快照、迁移、原子保存和可持久化字段清理。
- `internal/eventlog/`：有界历史、脱敏、持久化与轮转。
- `internal/tui/`：`model.go` 接收异步状态，`actions.go` 分发按键，`browse.go` 管理查询/分类，`navigation.go` 管理层级与焦点，`panels.go` 管理布局/日志/动画，`sound.go` 管理尺寸、Zen 和均衡器，`edit.go` 管理添加/创建与歌单选择流程，`view.go` 渲染，`keys.go` 定义 Global/面板绑定，`help.go` 从同一注册表生成上下文帮助，`keyboard.go` 区分修饰键与文本。
- `internal/tui/animation.go`：独立前端 30 FPS 时钟、播放/暂停与样式过渡、旧帧取消；`rhythm.go`：Pulse/Wave 的纯字符网格渲染。动画状态不持久化，不调用 IPC 或 dirty；隐藏、弹窗遮盖及暂停收束后停帧。不能以轮询进度驱动逐帧时间；没有 PCM 数据时不得称为实时频谱。
- `internal/artwork/`：有界 JPEG/PNG 原图和 96×96 RGB 回退图；`catalog.ArtworkProvider` 为可选来源合约，通过 daemon 的 artwork IPC 返回已校验的原始编码与回退像素，不向 TUI 暴露 URL 或任意文件路径。Apple 图片请求仅允许 HTTPS mzstatic.com CDN，不发送 API 授权头 / cookie；LOCAL 封面必须在音乐根目录内且为普通文件。封面不写 session / 磁盘缓存。`tui/artwork.go` 管理异步加载、取消和 generation。`internal/turntable/` 内嵌高清机身 PNG，以图片像素坐标确定唱盘中心，保留封面比例并合成单张完整场景，禁止分别按字符格放置封面与机身。素材替换必须同步中心坐标、尺寸、生成提示词与居中回归测试。`termimage/` 实现 Kitty 直接图片传输及 Unicode placement；`tui/graphics.go` 与 `graphics_probe.go` 分开管理图片和终端能力：探测与图片使用两个私有 ID、独立 generation；刷新 / 隐藏只清图片，最终退出 / 切来源清理两者。探测超时从 Raw 输出完成后开始，丢包有限重试，迟到成功可恢复原图；确认过的能力不被绘图错误 / 旧超时撤销。异步准备场景按图片 generation 串行输出、缩放和清理。无协议时将同一场景转为半格回退图；播放台使用独立前端旋转时钟，不能轮询服务或写 session 驱动帧；暂停立即停帧并保留角度，停止隐藏封面并复位。唱针播放 / 暂停时在唱片上、停止时停靠到右侧；指示灯只有实际播放时亮，暂停 / 缓冲 / 错误 / 停止均熄灭。播放态素材与停止态素材同尺寸内嵌，暂停只更换指示灯区域，不重解码封面，不把状态改动持久化。优先探测并修改私有图像的 root frame，仅上传唱片区域，不累积终端动画帧；不支持时刷新完整图像，同一时间只准备 / 发送一帧，防止慢连接积压。像素回退延迟采样同一图像，不逐帧编码 PNG。不得删除其他程序的图片。application 必须校验 source epoch 后再发 RawMsg；graphicsWrite 的输出 / 完成事件有序处理，不能被包裹器吞掉。`player_view.go` 负责顶部边框和自适应播放条；改变高度时必须校验 Queue 的实际可见行数。
- `internal/lyrics/`：平台无关的有界 LRC 时间轴、当前位置查找与 LRCLIB 精确元数据查询。`catalog.LyricsProvider` 为可选来源接口；LOCAL 只读音乐根目录内同名普通 `.lrc`，Apple 只把曲名、艺人、专辑和时长发到固定 HTTPS 公共服务（LRCLIB，超时 / 无结果时回退网易云音乐公开歌词并严格比对标题、全部艺人和时长），不转发账户头 / cookie / token。尊重 Retry-After，不做模糊匹配或猜测时间轴。`daemon/lyrics.go` 提供来源隔离的只读 IPC 与有界内存缓存；`tui/lyrics.go` 管理取消、generation、播放位置插值及当前句居中高亮。歌词不写 session / 磁盘 / 日志，缺失或失败只显示 No Lyrics Available；隐藏、切歌、切来源取消请求和时钟。
- `internal/video/`：来源中立的有界 PNG 播放帧与可选 Provider；`daemon/video.go` 校验 frontend lease、当前作品和来源，图片不进入 State/session。`tui/video.go` 管理 Content 视频、取消、generation 和有序单帧传输，复用能力探测，使用第三个私有图片 ID。隐藏/切来源/退出后不得输出迟到帧；暂停保留画面，停止清理，不支持图片协议时停止解码并保留音轨。Neovim 中继只增加本程序拥有的第三个 ID，不扩大允许的协议。
- `browserless/videos.go` 解析已授权视频及配套 AAC 音轨，`video_frames.go` 用 CDM 内部 H.264 解码，不通过 raw Decrypt 导出视频、不引入外部解码进程。只选 manifest 明确声明 WIDEVINE_SOFTWARE、HDCP NONE 的流，不修改保护策略、不假报输出保护。`mp4/fragments.go` 按 track ID、全部 moof/trun、CBCS subsamples 和 HLS 时间轴解析，`mp4/video.go` 只变换公开且明确处于 clear range 的 AVC 长度前缀/参数。保留当前及最多一个预取的加密片段（各最多 32 MiB）、当前图片与一帧前瞻，空闲释放解码器；预取使用独立可取消上下文，重置 / 隐藏 / 关闭后不得污染新流。视频保留软件授权的原始细节，最多输出 1280×720 / 30 FPS，禁止上采样冒充高清；过时帧仍走 CDM 解码依赖但跳过 RGB 转换，PNG 工作区可复用。IPC 直接传输有界 PNG，身份元数据单独编码，不再对整张图片进行 JSON base64；变更需更新协议版本。刷新期限包含解码及上传时间，重复画面不能触发长等待。原生终端按验证结果编辑同一 root frame，失败时完整重传而不撤销图片能力。Neovim 对完整的已验证图片分块合并输出，有界缓存且取消时丢弃未完成图片，不能放宽命令或 ID 校验。Source.SearchSections 提供 opaque 搜索类型与标题，UI 不拼 Apple 资源类型。新平台支持视频时声明 Capabilities.Video、返回 MediaType=video 的 Track 并实现 video.Provider。
- `browserless/cdn.go`、`cdn_dns.go`：每个 StreamServer 拥有独立媒体 HTTP 客户端；只对固定 Apple 视频 CDN 的 HTTPS GET 403 回退。通过固定 Google / Cloudflare HTTPS DNS 查询公开域名，拒绝私网、fake-IP、文档网段与无关 DNS 记录，最多四个不同备用节点；解析成功但节点仍失败时也需尝试第二个解析服务。保留签名 URL / Range / Host / TLS 校验，备用请求仅在进程内直连，禁止修改系统解析器、代理、全局 http.DefaultTransport 或复用账号头。成功节点仅在完整读取正文后按 DNS TTL（最多 5 分钟）内存缓存，并发查询合并，可取消；`media_http.go` 对备用正文读取失败仅重读一次，禁止拼接半个片段或重复输出音轨；单节点尝试有界，取消不标记健康节点失败；失败不冒充成功、不无限重试。关闭服务释放私有连接池。日志仅记录切换类别，不记录媒体地址、账户材料或 IP；授权、许可证、写入请求不得进入回退。
- `internal/vibez/`：固定版本的第三方原生播放子集。保留来源与许可证；变更需同步 `third_party/vibez-session.patch`、摘要或 local_additions。
- `scripts/`：仅保留依赖准备和第三方代码导入等维护工具。不要加入一次性实测脚本、账号探针、构建产物或测试报告。
- `lua/lazymusic/`：Neovim 浮动终端插件，可通过 lazy.nvim 加载；`docs/lazymusic.txt` 为帮助，安装时部署到插件的 doc/；`tests/nvim_spec.lua` 为独立 headless 回归测试。插件只启动/复用 TUI，不另建播放或授权逻辑。
- `lua/lazymusic/graphics.lua`：Neovim 0.12+ / termguicolors 下的 buffer-local OSC 图片中继。只允许本程序私有 ID 范围内的 Kitty query / 直接 PNG 虚拟 placement / root-frame 局部编辑和停止控制 / 清理命令，禁止文件或共享内存传输和任意控制序列。TermResponse 按拥有的 ID 返回对应子进程；只清理自己持有的图片。成功探测仅按 stdout_tty UI 身份在 Neovim 内存中缓存，后台退出重新打开复用；不同 UI 重新探测，显式禁用优先。不得将逐帧响应误作能力证明。安装时同步部署此模块；升级后重启 Neovim 加载新 Lua。旧版 Neovim、GUI 无 stdout TTY、tmux/zellij 保留像素回退，不假装原图已经显示。

## 增加音乐平台

1. 在 `music` 中定义稳定 Source ID，在 `internal/sources/<id>/` 实现 `catalog.Browser`；声明实际支持的能力，不要假装支持 Home/搜索/歌单。
2. 返回中立的 Item/Tab；保留平台推荐分类标题和所有返回资源，不以暂不支持播放为由隐藏项目或整组分类。生成明确的 Open 和 Play Query；不可播放时提供 Item.PlaybackNote。来源特有页面通过 Source.Pages 声明，分类通过 Page.Tabs / ActiveTab 声明，session 按 query 独立保存分类。禁止 UI 根据资源字符串拼平台 URL。
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
- 登录组件 stdout/stderr 共用有界过滤器，兼容 ANSI、CR 和无末尾换行；原始输出不能写日志。只向 UI 返回白名单错误类别、严格格式的库名/ABI/HTTP 状态。不得用统一的 try again 吞掉具体原因。手动 URL 必须在最小 60×16 窗口也完整可见，Login 的 y 按当前绑定复制完整 URL；不以剪贴板请求等同于已确认复制成功。
- 普通字符按终端生成的文本处理，保留基本键位消歧和备用字符码；不要在缺少关联文本时强制所有字符用 CSI-u 上报，否则 Shift+/ 等符号会退化成原键。所有页面与弹窗共用同一键盘协议配置。
- h/l 在面板内进退，Ctrl-h/j/k/l 切焦点。`,.` 必须按当前焦点分发：Navigation 动画、Content 分类、Player 播放模式；Sound 提供 Pulse/Wave/真实 EQ。Queue/Log 不得隐式操作旧 Content。
- Sound 的第四种视图是 Turntable，和 Pulse/Wave/EQ 一样用 ,/. 切换并保存选择。只加载当前播放曲目的封面，切歌、切来源、隐藏视图、退出取消请求，旧响应不能污染新曲目；装饰资源失败不抢焦点或阻断播放。新增来源可实现 catalog.LyricsProvider 提供有时间戳的歌词；禁止将音频 / 原始歌词发送到未声明服务。新增来源可实现 catalog.ArtworkProvider，缺图时返回空图片使用占位唱片。封面必须等比取中并用抗锯齿圆形遮罩裁剪，铺满唱盘仅留窄边，保留唱臂前景，和机身合成后再显示；不能保留突出的方形边角。播放台下方只显示同步歌词，不重复 Player 元数据。
- Zen 左侧在 Pulse / Wave / Turntable 之间循环，右侧始终为真实 EQ；跳过普通 Sound 的独立 EQ 槽，不能重复显示波形。图片加载尺寸 / 原生 placement / 回退渲染共用 turntableSize 与图像矩形，不能沿用普通侧边栏尺寸。Zen 的 h/l、j/k 仍调 EQ，,/. 仅切左侧视图。新布局必须覆盖普通与 Zen 的缩放、取消及 EQ 控制测试。
- 布局切换后焦点必须落在可见面板，Player 始终可见；日志可在所有布局用 @ 打开。页脚显示实际页面或当前焦点，不写死 NORMAL。
- q 首次提示，第二次 q 后台，Q 退出当前来源服务；退出/切换前保存 UI，冷恢复暂停。不可让迟到请求覆盖新 revision/epoch。
- H/J/K/L 调当前面板尺寸，Ctrl+= 恢复默认布局；Ctrl++/Ctrl+- 开关 Zen，Zen 必须同时显示律动、真实 EQ 和播放条。尺寸、音效与 Zen 状态持久化。EQ 操作按当前焦点路由，不能冒用播放音量或显示假频谱。
- o/c 作品跳转与 credits、a 添加菜单必须作用于窗口 2/3/4 的实际选中作品；Queue 清空使用 D，d 移除当前队列项。Content 的 a 在歌单列表新建歌单，y 加入播放队列，添加菜单提供 Play next。通过 catalog.Inspector/Editor 返回中立数据，UI 不猜测艺人 ID、不拆分 artistName 拼搜索，不把目录推断当作真实 credits。弹框异步查询可取消，关闭后迟到结果不能重开弹框或抢焦点；已提交的写入必须等回复，重复 Enter 不得重发。
- Queue 的 f 按 State.Index 定位当前曲目（含暂停），使用实际可见曲目行数居中并在首尾夹紧；重复曲目不能按 ID 猜位置。QueueOffset 与 QueueCursor 一同保存，普通导航仅在移出视口时滚动，不随播放轮询抢选中项。
- `internal/tui/pick.go`：Content p 拾取单曲或集合，P 播放整组；Queue p 初次拾取，之后 p 放光标后，Esc 取消。Queue 拾取是移动，Content 是复制加入。通过 Item.Play 和 catalog.All 异步展开全部分页及嵌套集合，不能在 UI 猜平台 kind/URL；最多 10000 首/1000 页/16 层，失败不得部分放置。拾取状态不持久化，来源切换取消读请求，generation 拦截迟到消息；写入必须等结果，重复按键不得重复提交。使用 QueueKey 守卫索引，集合一次 Insert，保留真实当前曲目/流/进度，不能以 Replace 重新播放。Content P 播放当前歌曲或整组。
- Content / Queue 的 u 撤销、重做最近一次成功放置。daemon 保持一条 session.Placement，下一次成功放置覆盖，失败/取消/无位移不覆盖；ID + revision 防止请求重试误变成重做。engine.QueueEditor 必须在播放器锁内校验 QueueKey 并原子移动/插入/整段移除，保留存活曲目的原生元数据、流与进度；移除当前播放项时停止并选相邻项，重做不自动播放。只允许队尾追加保留历史，用队列前缀摘要和长度校验；其他结构编辑失效，不可按 ID 猜重复项。新后端需要实现此可选合约才能支持放置及撤销。TUI 输入框中的 u 仍是文本。
- 快捷键注册在 keys.go：Global 与每个面板冲突检查独立，面板可复用字符，不能覆盖 Global。? 只显示当前面板和 Global，必须可滚动且反映自定义绑定；新增动作同时走 dispatch/help/validation。保留 Vim 导航、文本输入时的字符含义与 Ctrl-c/Esc 取消。修饰键优先按 Code/Mod 路由，普通 Shift 符号仍按 Text；覆盖 CSI-u 大写码、alternate/associated text、XTerm modifyOtherKeys 和 Neovim 的终端透传。默认 resize 使用普通大写 H/J/K/L，避免终端对 Ctrl+Shift+j 等键的拦截。不识别连按；h/j/k/l 必须立即响应，长按只在当前窗口移动。播放/暂停、停止、m 轮转模式是 Global；r/s 循环/随机只在 Queue/Player。< / > 上下曲，Ctrl+< / Ctrl+> 快退进（兼容 Ctrl+Shift+逗号/句号编码）。Content 刷新 r、Search 范围 s、EQ 归零 r。Queue 的 [/] 调整顺序；Login 从 Navigation 或 :login 进入。
- UI 音量只调整系统默认输出。原生播放器增益保持 1，禁止重复缩放。外部系统音量变化应在轮询后反映。
- warning/error 需可查看，通知 3 秒自动消失、不抢焦点且日志保留；主动取消旧请求不能作为 error 入库或通知。没有明确服务错误时，不把普通暂停推断为多设备冲突。
- 日志、快照、错误信息不能含 token、cookie、私钥、CDM 密钥、授权头或签名流地址。清理整个签名 URL 的 path/query，不只 query。会话只存恢复所需的稳定元数据。
- 原子写入快照；保留无法读取的旧文件用于恢复。IPC 目录校验所有者和 symlink，socket 私有，命令有界；集合分页检测循环、取消和数量上限。
- 不在源码中放凭据、CDM、辅助授权 binary、缓存、录音或测试音频。当前 Apple 授权沿用 Vibez 官方组件；不要复制其 broker 凭据，也不要声称是独立 Apple 应用身份。
- Apple Home 完整保留 recommendations 接口返回的分类、分页及资源；接口未返回的首页模块不得伪造。New/Radio 的 editorial.go 只解析官方公开区域页面的 serialized-server-data 内容字段，不读取网页 token/配置、执行脚本或发送账号授权头。限制正文大小、缓存数量、同源重定向和账号地区路径；不把 Radio 被重定向到 New 当作正常响应。公开内容 schema 的测试使用合成元数据，不保存账号响应或网页认证材料。
- Apple stations.go 使用真实电台 ID 的 next-tracks 接口；禁止把个人/风格电台改造成 ra.<songID>。Query.Continuous 使 Enter 经 Engine.StartContinuous 调台；Browse 只解析元数据，动态歌曲电台返回 Page.Continuous 拒绝无限展开，有限节目返回带 Kind/MediaType 的单个 Track，使 y/p 可使用原有队列路径。原生播放器中保留媒体类型，禁止把电台节目当作普通歌曲查询或无限续播。
- browserless/stations.go 与 license/stations.go 解析 Apple 已授权 station assets，按现有偏好选择 AAC 变体；CENC/CBCS 参数和公开 KID 从媒体保护元数据读取，实际密钥只由授权 CDM 使用，不导出。CBCS 的常量 IV 与 HLS PSSH KID 不能按普通歌曲的 CENC 假设处理。直播按 media-sequence 刷新窗口、去重和切换保护信息；没有真实直播验证时必须说明边界，不能仅用合成流测试声明完整兼容性。新本地 native 文件需加入 vendor_vibez.py 的 local_additions，并同步固定源码 patch 和摘要。

## 验证与改动规范

- 使用 `gofmt`。`make build`、`make test`、`make vet` 是标准检查；原生链接要求 CGO。测试不得操作真实系统音量、默认账号或正在播放的服务；注入 mixer/browser/engine。
- `make install` 默认安装至 `~/.local/bin`，同时部署 Lua 插件并更新 Neovim 的 lua/plugins/lazymusic.lua；支持 PREFIX、BINDIR、NVIM_CONFIG_DIR、NVIM_PLUGIN_DIR、DESTDIR 和 INSTALL_NVIM=0。保持 macOS/Linux 可移植，不使用 GNU 专有 install 参数。安装/卸载验证使用临时 staging 目录，不操作真实账号、服务或系统目录；卸载不得删除配置和 session。用户明确要求真实安装时，验证后再执行。
- 保留有价值的单元测试。回归测试围绕用户行为、边界和并发，避免照抄实现。实测脚本放临时目录，验证后清理。
- 修改 Neovim 插件时运行 `make test-nvim`；使用真实的无声测试进程验证浮窗与终端生命周期，不使用真实音乐账号。按键映射限当前音乐 buffer，不覆盖编辑器全局终端行为；命令及参数以 argv 传递，不拼 shell。
- 更改第三方代码前核对固定版本与许可证，保持导入可复现。`scripts/vendor_vibez.py /path/to/pinned/source` 重新导入后应用本地 patch。
- 在提交前 review 暂存区，检查秘密/产物、macOS/Linux 构建、session 兼容、异步取消与资源释放。不要为了完成提交删除用户已有修改或重置账号。
