#!/usr/bin/env python3
"""goreleaser の成果物（アーカイブ 6 つ）から、OS・arch 別の wheel を作る。

使い方:
  uv run --no-project packaging/pypi/build_wheels.py --dist <dist> --out <out> --version 0.1.0

標準ライブラリだけで書く（依存を宣言しない）。wheel は zip を自分で組み、RECORD のハッシュを付ける。
pyproject と setuptools を使わないのは、ファイルごとに platform tag を変えるだけの単純な組み立てで足りるため。
"""

import argparse
import base64
import hashlib
import sys
import tarfile
import zipfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent

DIST_NAME = "gemini_headless_cli"  # wheel のファイル名に使う正規化された名前
PROJECT_NAME = "gemini-headless-cli"
SUMMARY = "Unofficial fork of google-gemini/gemini-api-cli. Not affiliated with Google."

# goreleaser のアーカイブ名（{Os}_{arch}）→ wheel の platform tag。
# macOS は Go の既定の最低版（バイナリの LC_BUILD_VERSION minos が 13.0）に合わせる。
# Linux は CGO_ENABLED=0 の静的リンクなので manylinux_2_17 を名乗っても glibc に依存しない。
PLATFORMS = {
    "Darwin_arm64": ("gemini-headless", "macosx_13_0_arm64"),
    "Darwin_x86_64": ("gemini-headless", "macosx_13_0_x86_64"),
    "Linux_x86_64": ("gemini-headless", "manylinux_2_17_x86_64.manylinux2014_x86_64"),
    "Linux_arm64": ("gemini-headless", "manylinux_2_17_aarch64.manylinux2014_aarch64"),
    "Windows_x86_64": ("gemini-headless.exe", "win_amd64"),
    "Windows_arm64": ("gemini-headless.exe", "win_arm64"),
}


def archive_path(dist: Path, key: str) -> Path:
    ext = "zip" if key.startswith("Windows_") else "tar.gz"
    return dist / f"gemini-headless-cli_{key}.{ext}"


def read_binary(archive: Path, binary_name: str) -> bytes:
    if archive.name.endswith(".zip"):
        with zipfile.ZipFile(archive) as zf:
            for info in zf.infolist():
                if Path(info.filename).name == binary_name:
                    return zf.read(info)
    else:
        with tarfile.open(archive, "r:gz") as tf:
            for member in tf.getmembers():
                if member.isfile() and Path(member.name).name == binary_name:
                    f = tf.extractfile(member)
                    assert f is not None
                    return f.read()
    raise SystemExit(f"{archive.name} の中に {binary_name} が無い")


def record_hash(data: bytes) -> str:
    digest = base64.urlsafe_b64encode(hashlib.sha256(data).digest()).rstrip(b"=")
    return "sha256=" + digest.decode("ascii")


def metadata(version: str, readme: str) -> str:
    # PEP 639 の License-Expression を使う（Metadata-Version 2.4）。
    return "\n".join(
        [
            "Metadata-Version: 2.4",
            f"Name: {PROJECT_NAME}",
            f"Version: {version}",
            f"Summary: {SUMMARY}",
            "License-Expression: Apache-2.0",
            "License-File: LICENSE",
            "Requires-Python: >=3.9",
            "Project-URL: Homepage, https://github.com/yasuguin/gemini-headless-cli",
            "Project-URL: Upstream, https://github.com/google-gemini/gemini-api-cli",
            "Description-Content-Type: text/markdown",
            "",
            readme,
        ]
    )


def build_one(
    key: str, dist: Path, out: Path, version: str, license_text: bytes, readme: str
) -> Path:
    binary_name, tag = PLATFORMS[key]
    archive = archive_path(dist, key)
    if not archive.is_file():
        raise SystemExit(f"アーカイブが無い: {archive}")
    binary = read_binary(archive, binary_name)

    init_py = (HERE / "src" / DIST_NAME / "__init__.py").read_bytes()
    dist_info = f"{DIST_NAME}-{version}.dist-info"
    wheel_meta = "\n".join(
        [
            "Wheel-Version: 1.0",
            "Generator: packaging/pypi/build_wheels.py",
            "Root-Is-Purelib: false",
            f"Tag: py3-none-{tag}",
            "",
        ]
    ).encode()
    entry_points = (
        b"[console_scripts]\n"
        b"gemini-headless = gemini_headless_cli:main\n"
        b"gemini-headless-cli = gemini_headless_cli:main\n"
    )
    meta = metadata(version, readme).encode()

    # 実行ビットは zip の external_attr で持たせる（wheel はこれで権限を運ぶ）。
    files: list[tuple[str, bytes, int]] = [
        (f"{DIST_NAME}/__init__.py", init_py, 0o644),
        (f"{DIST_NAME}/_bin/{binary_name}", binary, 0o755),
        (f"{dist_info}/METADATA", meta, 0o644),
        (f"{dist_info}/WHEEL", wheel_meta, 0o644),
        (f"{dist_info}/entry_points.txt", entry_points, 0o644),
        (f"{dist_info}/licenses/LICENSE", license_text, 0o644),
    ]
    record_lines = [
        f"{name},{record_hash(data)},{len(data)}" for name, data, _ in files
    ]
    record_lines.append(f"{dist_info}/RECORD,,")
    record = ("\n".join(record_lines) + "\n").encode()
    files.append((f"{dist_info}/RECORD", record, 0o644))

    wheel_path = out / f"{DIST_NAME}-{version}-py3-none-{tag}.whl"
    with zipfile.ZipFile(wheel_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        for name, data, mode in files:
            info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
            info.external_attr = (0o100000 | mode) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            zf.writestr(info, data)
    return wheel_path


def main() -> None:
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument(
        "--dist", type=Path, required=True, help="goreleaser の dist ディレクトリ"
    )
    parser.add_argument("--out", type=Path, required=True, help="wheel の出力先")
    parser.add_argument(
        "--version", required=True, help="fork の版（plus-v を外したもの。例 0.1.0）"
    )
    parser.add_argument("--license", type=Path, default=ROOT / "LICENSE")
    parser.add_argument("--readme", type=Path, default=HERE / "README.md")
    args = parser.parse_args()

    args.out.mkdir(parents=True, exist_ok=True)
    license_text = args.license.read_bytes()
    readme = args.readme.read_text(encoding="utf-8")

    built = [
        build_one(key, args.dist, args.out, args.version, license_text, readme)
        for key in PLATFORMS
    ]
    for path in built:
        print(path)
    if len(built) != 6:
        sys.exit(f"wheel が 6 つではない: {len(built)}")


if __name__ == "__main__":
    main()
