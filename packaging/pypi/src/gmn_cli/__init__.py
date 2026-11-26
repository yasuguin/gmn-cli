"""fork（gmn）の Go バイナリを呼ぶ薄いラッパー。

wheel の中の _bin/ に OS・arch ごとのバイナリが入っている。このモジュールは引数をそのまま渡すだけで、
--version や終了コードの扱いは Go 側に任せる。
"""

import os
import subprocess
import sys
from pathlib import Path

_BINARY_NAME = "gmn.exe" if os.name == "nt" else "gmn"


def _binary_path() -> str:
    path = Path(__file__).resolve().parent / "_bin" / _BINARY_NAME
    if not path.is_file():
        sys.exit(f"gmn: バイナリが見つからない: {path}")
    return str(path)


def main() -> None:
    binary = _binary_path()
    args = [binary, *sys.argv[1:]]
    if os.name == "nt":
        # Windows には exec が無いので子プロセスの終了コードを返す。
        sys.exit(subprocess.run(args, check=False).returncode)
    # POSIX は exec で置き換え、シグナルと終了コードを Go のプロセスにそのまま渡す。
    os.execv(binary, args)
