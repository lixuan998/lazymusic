#!/usr/bin/env python3
"""Package native, tested binaries without accounts, downloaded helpers or CDMs."""
import argparse
import gzip
from email.utils import formatdate
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import struct
import subprocess
import tarfile
import tempfile
import time
import zipfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = ("darwin-arm64", "windows-amd64", "ubuntu-amd64")


def run(*args, cwd=ROOT):
    return subprocess.check_output([str(x) for x in args], cwd=cwd, text=True).strip()


def normalize_version(value):
    value = value.removeprefix("v")
    if not re.fullmatch(r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?", value):
        raise ValueError("release version must be a semantic version, such as 0.5.0")
    return value


def binary_target(path):
    with path.open("rb") as stream:
        data = stream.read(4096)
        if len(data) >= 20 and data[:4] == b"\x7fELF" and data[4:6] == b"\x02\x01" and struct.unpack_from("<H", data, 18)[0] == 62:
            return "ubuntu-amd64"
        if data[:8] == struct.pack("<II", 0xFEEDFACF, 0x0100000C):
            return "darwin-arm64"
        if data[:2] == b"MZ" and len(data) >= 64:
            stream.seek(struct.unpack_from("<I", data, 60)[0])
            header = stream.read(6)
            if header == b"PE\0\0\x64\x86":
                return "windows-amd64"
    raise ValueError("binary is not a supported native 64-bit executable")


def copy_file(source, destination, mode=0o644):
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, destination)
    destination.chmod(mode)


def dependency_licenses(destination):
    destination.mkdir(parents=True, exist_ok=True)
    raw = run("go", "list", "-m", "-json", "all")
    decoder = json.JSONDecoder()
    while raw.strip():
        raw = raw.lstrip()
        module, end = decoder.raw_decode(raw)
        raw = raw[end:]
        if module.get("Main"):
            continue
        if "Dir" not in module:
            raise ValueError("missing module files; run go mod download first: " + module["Path"])
        for entry in Path(module["Dir"]).iterdir():
            if entry.is_file() and entry.name.upper().startswith(("LICENSE", "COPYING", "NOTICE")):
                name = module["Path"].replace("/", "_") + "_" + entry.name
                copy_file(entry, destination / name)
    copy_file(Path(run("go", "env", "GOROOT")) / "LICENSE", destination / "Go-LICENSE")
    for name in ("VIBEZ-LICENSE", "CHROMIUM-CDM-BSD-3-Clause.txt", "vibez-source.json"):
        copy_file(ROOT / "third_party" / name, destination / name)


def install_notes(target, version):
    common = """
## Usage

Run `lazymusic` to select APPLE MUSIC or LOCAL. Choose a local music folder with
`lazymusic --source local --music-dir PATH`. Press `?` for keys. Press `q`, then
`q` to detach; press `q`, then `Q` to stop the source service.

Images use verified Kitty or Sixel support. Sixel uses up to 256 colors and
10 FPS; Neovim's image relay remains Kitty-only. Apple playback requires a
subscription and authorization; the official Vibez helper and Widevine CDM
are prepared on demand and are NOT bundled here. No account state is included.
Real Apple account login, subscription playback, stations and protected video
have not been verified on Windows. LOCAL formats depend on the OS decoders.

Source and complete documentation: https://github.com/lixuan998/lazymusic
"""
    if target == "darwin-arm64":
        instructions = """For Apple Silicon (M-series), macOS 13 or newer. No Go compiler or
GStreamer is required. The executable is ad-hoc signed, not Developer ID signed
or notarized; macOS may require you to allow it in Privacy & Security.

```sh
mkdir -p "$HOME/.local/bin"
install -m 0755 lazymusic "$HOME/.local/bin/lazymusic"
export PATH="$HOME/.local/bin:$PATH"
lazymusic
```

Optional Neovim setup from this extracted directory:

```sh
mkdir -p "$HOME/.local/share/lazymusic/nvim"
cp -R lua doc "$HOME/.local/share/lazymusic/nvim/"
lazymusic setup-nvim --config-dir "${XDG_CONFIG_HOME:-$HOME/.config}/nvim" --plugin-dir "$HOME/.local/share/lazymusic/nvim" --binary "$HOME/.local/bin/lazymusic" --help-dir "$HOME/.local/share/lazymusic/nvim/doc"
```
"""
    elif target == "windows-amd64":
        instructions = """For Windows 10/11 x64. Extract the ZIP and open PowerShell in the extracted
directory. No Go compiler, GCC or GStreamer is required. Windows N/KN needs the
Media Feature Pack. The executable is not Authenticode-signed.

```powershell
.\\lazymusic.exe
.\\lazymusic.exe --source local --music-dir 'D:\\Music'
```

Windows Terminal 1.22+ supports Sixel. If needed, select it before starting:

```powershell
$env:LAZYMUSIC_IMAGE_PROTOCOL = 'sixel'
.\\lazymusic.exe
```

To install, copy lazymusic.exe to a directory on your user PATH. To install the
Neovim plugin, copy this archive's lua and doc directories to your chosen plugin
directory, then run `lazymusic.exe setup-nvim --config-dir PATH --plugin-dir PATH
--binary PATH --help-dir PATH`, using full paths. Neovim's default config is
`%LOCALAPPDATA%\\nvim`. Configuration defaults to `%APPDATA%\\lazymusic`.
"""
    else:
        instructions = f"""For Ubuntu 22.04 or newer on amd64. Install the downloaded package with apt
so its GStreamer, decoder, and system-volume dependencies are installed:

```sh
sudo apt install ./lazymusic_{version}_amd64.deb
lazymusic
```

An accessible audio output is required (PulseAudio/PipeWire; WSL needs audio
forwarding). Run the player as your normal user. No Go compiler is needed.

The Neovim plugin and prebuilt help tags are installed at
`/usr/share/lazymusic/nvim`. Opt in per-user, without modifying system files:

For Neovim help, use the packaged `/usr/share/lazymusic/nvim/doc` directory
(which already includes tags); omit `--help-dir` when registering the config:

```sh
lazymusic setup-nvim --config-dir "${{XDG_CONFIG_HOME:-$HOME/.config}}/nvim" --plugin-dir /usr/share/lazymusic/nvim --binary /usr/bin/lazymusic
```

Remove with `sudo apt remove lazymusic`; user configuration and sessions stay.
"""
    return f"# lazymusic {version} — {target}\n\n" + instructions + common


def plugin_files(destination):
    for name in ("init.lua", "graphics.lua"):
        copy_file(ROOT / "lua/lazymusic" / name, destination / "lua/lazymusic" / name)
    copy_file(ROOT / "docs/lazymusic.txt", destination / "doc/lazymusic.txt")
    content = (ROOT / "docs/lazymusic.txt").read_text()
    tags = sorted(set(re.findall(r"\*([A-Za-z0-9_.:-]+)\*", content)))
    (destination / "doc/tags").write_text("".join(f"{tag}\tlazymusic.txt\t/*{tag}*\n" for tag in tags), encoding="utf-8")


def archive_mode(path):
    # Windows does not represent Unix executable bits. Store portable modes
    # explicitly instead of inheriting host-dependent stat() results.
    return 0o755 if path.is_dir() or path.name in ("lazymusic", "lazymusic.exe") else 0o644


def archive_tree(source, output, epoch):
    if output.suffix == ".zip":
        date = time.gmtime(max(315532800, epoch))[:6]
        with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
            for path in sorted(source.rglob("*")):
                if path.is_file():
                    info = zipfile.ZipInfo(str(path.relative_to(source.parent)).replace("\\", "/"), date)
                    info.create_system = 3
                    info.external_attr = (0o100000 | archive_mode(path)) << 16
                    info.compress_type = zipfile.ZIP_DEFLATED
                    archive.writestr(info, path.read_bytes())
    else:
        with output.open("wb") as raw, gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=epoch) as compressed:
            with tarfile.open(fileobj=compressed, mode="w") as archive:
                for path in sorted(source.rglob("*")):
                    info = archive.gettarinfo(str(path), str(path.relative_to(source.parent)))
                    info.mode = archive_mode(path)
                    info.uid = info.gid = 0
                    info.uname = info.gname = ""
                    info.mtime = epoch
                    if path.is_file():
                        with path.open("rb") as stream:
                            archive.addfile(info, stream)
                    else:
                        archive.addfile(info)


def build_deb(work, binary, version, output, metadata, epoch):
    package = work / "deb-root"
    copy_file(binary, package / "usr/bin/lazymusic", 0o755)
    plugin_files(package / "usr/share/lazymusic/nvim")
    docs = package / "usr/share/doc/lazymusic"
    docs.mkdir(parents=True)
    (docs / "README.md").write_text(install_notes("ubuntu-amd64", version), encoding="utf-8")
    (docs / "build-info.json").write_text(json.dumps(metadata, indent=2)+"\n", encoding="utf-8")
    dependency_licenses(docs / "licenses")
    (docs / "copyright").write_text("Upstream: https://github.com/lixuan998/lazymusic\nThird-party copyright and license notices are in the licenses directory.\n", encoding="utf-8")
    debian = work / "debian"
    debian.mkdir()
    deb_version = version.replace("-", "~", 1)
    source_control = "Source: lazymusic\nSection: sound\nPriority: optional\nMaintainer: lixuan <xclimatexx@gmail.com>\n\nPackage: lazymusic\nArchitecture: amd64\nDescription: Native terminal music player\n"
    (debian / "control").write_text(source_control)
    (debian / "changelog").write_text(f"lazymusic ({deb_version}) stable; urgency=medium\n\n  * Native release.\n\n -- lixuan <xclimatexx@gmail.com>  {formatdate(epoch, usegmt=False)}\n")
    shared = run("dpkg-shlibdeps", "-O", "-e"+str(package / "usr/bin/lazymusic"), cwd=work)
    dependencies = next(line.removeprefix("shlibs:Depends=") for line in shared.splitlines() if line.startswith("shlibs:Depends="))
    dependencies += ", ca-certificates, gstreamer1.0-plugins-base, gstreamer1.0-plugins-good, gstreamer1.0-libav, gstreamer1.0-pulseaudio, pulseaudio-utils | wireplumber"
    control = package / "DEBIAN"
    control.mkdir()
    size = sum(p.stat().st_size for p in package.rglob("*") if p.is_file())
    (control / "control").write_text(f"Package: lazymusic\nVersion: {deb_version}\nArchitecture: amd64\nMaintainer: lixuan <xclimatexx@gmail.com>\nSection: sound\nPriority: optional\nInstalled-Size: {(size+1023)//1024}\nDepends: {dependencies}\nHomepage: https://github.com/lixuan998/lazymusic\nDescription: Native terminal player for Apple Music and local files\n Vim-style navigation, persistent background playback, equalizer, artwork\n and synchronized lyrics. Apple Music authorization is prepared on demand.\n")
    checksums = []
    for path in sorted(package.rglob("*")):
        if path.is_file() and not path.is_relative_to(control):
            checksums.append(hashlib.md5(path.read_bytes()).hexdigest()+"  "+str(path.relative_to(package))+"\n")
    (control / "md5sums").write_text("".join(checksums))
    for path in package.rglob("*"):
        os.utime(path, (epoch, epoch))
    subprocess.run(["dpkg-deb", "--root-owner-group", "--build", str(package), str(output)], check=True, env={**os.environ, "SOURCE_DATE_EPOCH":str(epoch)})


def build(target, version, binary, output_dir):
    version = normalize_version(version)
    binary = binary.resolve(strict=True)
    if binary_target(binary) != target:
        raise ValueError("binary architecture does not match " + target)
    if run(binary, "version") != "lazymusic " + version:
        raise ValueError("binary version does not match release version")
    output_dir = output_dir.resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    revision = run("git", "rev-parse", "HEAD")
    epoch = int(run("git", "show", "-s", "--format=%ct", "HEAD"))
    metadata = {"version":version, "target":target, "git_commit":revision,
                "git_dirty":bool(run("git", "status", "--porcelain", "--untracked-files=no")),
                "go_version":run("go", "env", "GOVERSION"), "binary_sha256":hashlib.sha256(binary.read_bytes()).hexdigest()}
    with tempfile.TemporaryDirectory(prefix="lazymusic-package-") as folder:
        work = Path(folder)
        if target == "ubuntu-amd64":
            output = output_dir / f"lazymusic_{version.replace('-', '~', 1)}_amd64.deb"
            build_deb(work, binary, version, output, metadata, epoch)
        else:
            name = f"lazymusic_{version}_{target.replace('-', '_')}"
            tree = work / name
            tree.mkdir()
            copy_file(binary, tree / ("lazymusic.exe" if target.startswith("windows") else "lazymusic"), 0o755)
            plugin_files(tree)
            dependency_licenses(tree / "licenses")
            (tree / "README.md").write_text(install_notes(target, version), encoding="utf-8")
            (tree / "build-info.json").write_text(json.dumps(metadata, indent=2)+"\n", encoding="utf-8")
            output = output_dir / (name+(".zip" if target.startswith("windows") else ".tar.gz"))
            archive_tree(tree, output, epoch)
    print(output)


def write_checksums(directory):
    assets = sorted(p for p in directory.iterdir() if p.is_file() and (p.suffix in (".deb", ".zip") or p.name.endswith(".tar.gz")))
    if len(assets) != 3:
        raise ValueError("expected exactly the macOS, Windows and Ubuntu assets")
    content = "".join(hashlib.sha256(p.read_bytes()).hexdigest()+"  "+p.name+"\n" for p in assets)
    (directory / "SHA256SUMS").write_text(content, encoding="ascii")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    package = commands.add_parser("build")
    package.add_argument("--target", required=True, choices=TARGETS)
    package.add_argument("--version", required=True)
    package.add_argument("--binary", required=True, type=Path)
    package.add_argument("--output", type=Path, default=ROOT/"dist")
    checksums = commands.add_parser("checksums")
    checksums.add_argument("directory", type=Path)
    version = commands.add_parser("version")
    version.add_argument("value")
    args = parser.parse_args()
    if args.command == "build":
        build(args.target, args.version, args.binary, args.output)
    elif args.command == "checksums":
        write_checksums(args.directory)
    else:
        print(normalize_version(args.value))


if __name__ == "__main__":
    main()
