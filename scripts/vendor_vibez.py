#!/usr/bin/env python3
"""Import the small, pinned native backend subset; never run upstream installers."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
SOURCE = Path(sys.argv[1]).resolve()
COMMIT = "31def02bd334f76ecc8a69caeb6c3c642df0fe55"
actual = subprocess.check_output(["git", "-C", str(SOURCE), "rev-parse", "HEAD"], text=True).strip()
if actual != COMMIT:
    raise SystemExit(f"Expected pinned commit {COMMIT}; refusing to import {actual}")
if subprocess.check_output(["git", "-C", str(SOURCE), "status", "--porcelain", "--untracked-files=no"], text=True).strip():
    raise SystemExit("Refusing to import a modified upstream checkout")
PACKAGES = ["audioquality", "config", "provider", "provider/apple",
            "player", "player/browserless", "player/browserless/cdm",
            "player/browserless/license", "player/browserless/mp4", "player/gst"]
manifest = {"project": "https://github.com/simonepelosi/vibez", "tag": "v2.0.6",
            "commit": COMMIT, "license": "MIT", "files": {}}
for package in PACKAGES:
    for src in sorted((SOURCE / "internal" / package).iterdir()):
        if not src.is_file() or src.suffix not in {".go", ".cpp", ".h"}:
            continue
        # The selected native backend supports macOS/Linux only.
        if "windows" in src.name or src.name == "sink_e2e_test.go":
            continue
        rel = src.relative_to(SOURCE)
        original = src.read_bytes()
        content = original.replace(b"github.com/simone-vibes/vibez/internal/", b"lazymusic/internal/vibez/")
        dest = ROOT / "internal/vibez" / package / src.name
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_bytes(content)
        manifest["files"][str(rel)] = {"upstream_sha256": hashlib.sha256(original).hexdigest(),
                                       "imported_sha256": hashlib.sha256(content).hexdigest()}
patch = ROOT / "third_party/vibez-session.patch"
if patch.exists():
    subprocess.run(["git", "apply", str(patch)], cwd=ROOT, check=True)
    manifest["patch"] = {"path": str(patch.relative_to(ROOT)), "sha256": hashlib.sha256(patch.read_bytes()).hexdigest()}
    for rel, hashes in manifest["files"].items():
        dest = ROOT / "internal/vibez" / Path(rel).relative_to("internal")
        hashes["imported_sha256"] = hashlib.sha256(dest.read_bytes()).hexdigest()
manifest["local_additions"] = {}
for rel in [
    'internal/vibez/player/browserless/session.go',
    'internal/vibez/player/browserless/session_test.go',
    'internal/vibez/player/browserless/stations.go',
    'internal/vibez/player/browserless/stations_test.go',
    'internal/vibez/player/browserless/license/stations.go',
    'internal/vibez/player/browserless/license/stations_test.go',
    'internal/vibez/player/browserless/mp4/encryption.go',
    'internal/vibez/player/browserless/mp4/encryption_test.go',
    'internal/vibez/player/browserless/cdm/cdm_nocgo.go',
]:
    dest = ROOT / rel
    if dest.exists():
        manifest["local_additions"][rel] = hashlib.sha256(dest.read_bytes()).hexdigest()
(ROOT / "third_party").mkdir(exist_ok=True)
(ROOT / "third_party/VIBEZ-LICENSE").write_bytes((SOURCE / "LICENSE").read_bytes())
(ROOT / "third_party/vibez-source.json").write_text(json.dumps(manifest, indent=2) + "\n")
print(f"Imported {len(manifest['files'])} backend files from {COMMIT}")
