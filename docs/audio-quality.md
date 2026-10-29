# 音质支持与验证记录

## 当前支持范围

| 来源 | 可选音质 | 实际含义 |
| --- | --- | --- |
| APPLE MUSIC（macOS / Linux） | AAC 64 / 256 kbps | 请求的音质偏好，实际可用流取决于服务响应；未验证无损 / Hi-Res |
| LOCAL | 原文件 | 不转码，解码能力取决于系统；原文件的高采样率不保证输出设备保持相同格式 |

按 `b` 打开同一音质选择入口，选项由后端能力提供。Apple 切换音质时在当前位置重新准备歌曲。不能把偏好、曲目标签、输出设备采样率或升采样当作实际音源格式。

## 现有 Apple 播放链路

当前 Apple 后端使用网页授权，通过 MZPlay / Widevine 取得音频，再由本机音频栈播放。播放期间无需浏览器；授权方案沿用 Vibez 官方组件和应用身份。这条链路并不因此获得 Apple 原生应用的全部音频能力。

2026-10-29 的只读核验结果：

- Apple 当前 [MusicKit JS v3 SDK](https://js-cdn.music.apple.com/musickit/v3/musickit.js) 的 PlaybackBitrate 为 STANDARD=64、HIGH=256。SDK 中的 lossless / hi-res 枚举属于质量统计字段，不构成可调用的 Hi-Res 选择接口。
- 对已授权账号的一首收藏曲目，catalog 声明有 lossless，但当时 webPlayback 返回的 6 个资源均为 64 / 256 kbps 档位，未返回 ALAC。单个样本不能代表全部曲目、账号或未来服务行为。
- 工程的订阅流管线目前只实现 AAC 分片处理。增加 ALAC 解封装与解码仍不足以让上游交付并授权 ALAC 资源。

当前限制同时来自播放链路与尚未实现的其他后端，不能据此推断 Apple Music 本身不支持无损，或所有原生方案都不可能。Apple 另有[原生 MusicKit 框架](https://developer.apple.com/musickit/)，需要单独验证授权、实际音频变体和输出格式。

## macOS 原生 MusicKit 实测

2026-10-29，在本机 macOS 27.0.1、Xcode / MusicKit SDK 27、Swift 6.4 环境完成四轮验证：

| 检查 | 实测结果 |
| --- | --- |
| 原生授权、订阅 | MusicAuthorization 为 authorized，canPlayCatalogContent 为 true |
| 系统 Apple Music 设置 | Lossless 已开启，Streaming / Download 均为 Hi-Res Lossless |
| 验证样本 | 一首 catalog 标为 lossless 的收藏曲目，以及 Norah Jones《Come Away with Me》的两个 Hi-Res catalog 版本 |
| 播放实现 | ApplicationMusicPlayer；另用完整 AppKit 生命周期和 MusicCatalogResourceRequest 原生请求复测，沿用现有 Developer Token 与本机原生用户授权 |
| 完整播放 | 从 60 秒开始播放，进度继续到 70 秒以上，排除仅播 30 秒试听 |
| 当前音质属性 | MusicPlayer.State.audioVariant 始终为 nil / unknown；未将 Song.audioVariants 当作实际播放格式 |
| 实际解码 | 四轮均创建 format `paac`、sampleRate `44100` 的 AudioQueue；解码器日志为 AAC，未观察到 ALAC |
| 其他运行证据 | RemotePlayerService 日志显示 `Using pre-defined lossy formats`；网络正常，非低数据模式 / 计费网络 |

还尝试了验证 App 自身的 Hi-Res / 立体声偏好，实际编码没有变化。这些偏好键不是公开的 MusicKit 音质控制 API，不能作为实现成功的依据。验证未改动系统 Apple Music 的音质设置。当前 SDK 的 SystemMusicPlayer 不适用于 macOS，未作为生产替代路径。

结论限定于**这台机器、当前系统 / SDK 和现有授权方案**：原生 MusicKit 可播放完整订阅内容，但尚未验证真正的无损或 Hi-Res。因此未接入为生产后端，也未向 macOS 音质菜单增加未兑现的选项。Linux 继续使用现有 AAC / Widevine 方案。

Apple 的[当前音质属性](https://developer.apple.com/documentation/musickit/musicplayer/state-swift.class/audiovariant)为只读；[官方说明](https://developer.apple.com/videos/play/wwdc2022/110347/)描述了根据用户设置和网络条件自动选择音质的行为。当前 SDK 未提供公开的逐播放器音质 setter。

## 后续接入标准

只有在真实播放路径取得 **ALAC 解码证据、有效采样率 / 位深、实际音质切换结果** 后，后端才能向现有 `b` 菜单声明无损 / Hi-Res。不得仅凭 macOS 系统判断、曲目 audioVariants 标签或 192 kHz 输出设备设置开放选项。

UI 继续共用同一入口、布局和键位；平台差异体现在实际后端能力和实现中。验证 App、探针源码、原始日志和临时偏好已在验证结束后清理，不随源码提交。
