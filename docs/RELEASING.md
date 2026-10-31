# 发布安装包

`.github/workflows/release.yml` 提供手动触发的发布流程，使用 Go 1.27.3 和 CGO，
在 macOS arm64、Windows amd64、Ubuntu 22.04 amd64 原生 runner 上分别构建。
不要使用 `CGO_ENABLED=0` 替代原生构建；音频输出和 CDM 桥接需要 CGO。

## 构建与检查

```sh
gh workflow run release.yml --ref main -f version=0.5.0 -f publish=false
gh run list --workflow release.yml
gh run view <run-id>
gh run download <run-id> --dir ./release-download
```

版本可带 `v` 前缀；流程会校验并标准化。二进制使用 `-X main.version=<版本>`，
打包前验证实际的 `lazymusic version` 输出和可执行文件架构。
每个平台都运行完整 Go 测试和 vet，以及 Python 打包测试。
macOS 检查系统动态库链接和 ad-hoc 签名；Windows 检查未依赖 MinGW 运行时 DLL；
Ubuntu 的 `.deb` 从 ELF 计算共享库依赖，并在干净 Ubuntu 22.04 容器内通过 apt 试装。

`scripts/package_release.py` 只打包明确指定的可执行文件、插件、帮助及许可证。
`build-info.json` 记录版本、目标架构、源码 commit、源码是否有未提交改动和二进制摘要。
不包含授权组件、CDM、账号材料或会话。归档时间戳基于源码 commit；
`.deb` 使用 root 文件所有权，不通过安装脚本修改用户配置。

Ubuntu 包安装到 `/usr/bin/lazymusic`，插件位于 `/usr/share/lazymusic/nvim`。
Neovim 配置由用户显式执行 `lazymusic setup-nvim` 后生成，包内 README 包含命令。
Ubuntu 运行时依赖 GStreamer、AAC 解码和默认设备音量工具，均声明在 Depends 中。

## 发布

先检查三份原生产物及其构建信息，准备面向用户的更新说明，然后创建 Release 并上传资产。
同一个构建也可使用 `publish=true`，在所有原生构建和试装成功后自动生成 `SHA256SUMS` 并发布。
自动发布以该工作流的源码 commit 创建 `v<版本>` 标签，不覆盖已有 Release。

```sh
gh workflow run release.yml --ref main -f version=0.5.0 -f publish=true
```

公开产物为 macOS `.tar.gz`、Windows `.zip`、Ubuntu `.deb` 和 `SHA256SUMS`。
macOS 没有 Developer ID 签名/公证，Windows 没有 Authenticode 签名；不要将 ad-hoc 签名称为公证。
发布说明应保留 Apple Music 真实账号播放的已验证范围，不能将构建成功称为实际订阅播放验证。
