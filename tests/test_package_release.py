import hashlib
import importlib.util
from pathlib import Path
import struct
import tarfile
import tempfile
import unittest
import zipfile

spec = importlib.util.spec_from_file_location("package_release", Path(__file__).resolve().parents[1]/"scripts/package_release.py")
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class ReleasePackagingTests(unittest.TestCase):
    def test_versions_cannot_be_paths_or_shell_commands(self):
        self.assertEqual(release.normalize_version("v0.5.0"), "0.5.0")
        self.assertEqual(release.normalize_version("1.2.3-rc.1"), "1.2.3-rc.1")
        for version in ("../0.5.0", "0.5", "01.2.3", "0.5.0; echo bad", "0.5.0\n", "main"):
            with self.subTest(version=version), self.assertRaises(ValueError):
                release.normalize_version(version)

    def test_package_target_uses_executable_headers(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder)/"binary"
            elf = bytearray(64)
            elf[:6] = b"\x7fELF\x02\x01"
            struct.pack_into("<H", elf, 18, 62)
            path.write_bytes(elf)
            self.assertEqual(release.binary_target(path), "ubuntu-amd64")
            path.write_bytes(struct.pack("<II", 0xFEEDFACF, 0x0100000C))
            self.assertEqual(release.binary_target(path), "darwin-arm64")
            pe = bytearray(96)
            pe[:2] = b"MZ"
            struct.pack_into("<I", pe, 60, 80)
            pe[80:86] = b"PE\0\0\x64\x86"
            path.write_bytes(pe)
            self.assertEqual(release.binary_target(path), "windows-amd64")
            pe[84:86] = b"\x4c\x01"
            path.write_bytes(pe)
            with self.assertRaises(ValueError):
                release.binary_target(path)

    def test_archives_preserve_modes_paths_and_reproducibility(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            source = root/"lazymusic_0.5.0_test"
            source.mkdir()
            binary = source/"lazymusic"
            binary.write_bytes(b"native executable fixture")
            binary.chmod(0o755)
            for suffix in (".zip", ".tar.gz"):
                first, second = root/("a"+suffix), root/("b"+suffix)
                release.archive_tree(source, first, 1780000000)
                release.archive_tree(source, second, 1780000000)
                self.assertEqual(first.read_bytes(), second.read_bytes())
                expected = source.name+"/lazymusic"
                if suffix == ".zip":
                    with zipfile.ZipFile(first) as archive:
                        self.assertEqual(archive.namelist(), [expected])
                        self.assertEqual(archive.getinfo(expected).external_attr>>16 & 0o777, 0o755)
                else:
                    with tarfile.open(first) as archive:
                        entry = archive.getmember(expected)
                        self.assertEqual((entry.uid, entry.gid, entry.mode), (0, 0, 0o755))

    def test_checksums_cover_exactly_three_packages(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder)
            with self.assertRaises(ValueError):
                release.write_checksums(path)
            for name in ("lazymusic_mac.tar.gz", "lazymusic_windows.zip", "lazymusic_amd64.deb"):
                (path/name).write_bytes(name.encode())
            release.write_checksums(path)
            lines = (path/"SHA256SUMS").read_text().splitlines()
            self.assertEqual(len(lines), 3)
            for line in lines:
                digest, name = line.split("  ")
                self.assertEqual(digest, hashlib.sha256((path/name).read_bytes()).hexdigest())


if __name__ == "__main__":
    unittest.main()
