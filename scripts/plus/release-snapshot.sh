#!/usr/bin/env bash
# .goreleaser.plus.yaml の snapshot ビルドを一時ディレクトリに出し、リリース前の設定を手元で確かめる。
# 確かめること: 6 種類のバイナリと checksum ができる・現在の OS の --version に指定した fork 版が出る・
# Homebrew cask が生成されるならその中身を出す。publish はしない（--skip=publish）。
#
# 使い方: scripts/plus/release-snapshot.sh [fork の版（既定 0.1.0）]
set -euo pipefail

version="${1:-0.1.0}"
# release workflow の goreleaser-action（~> v2）と揃えた版。goreleaser が無ければ go run で動かす（install はしない）。
goreleaser_module="github.com/goreleaser/goreleaser/v2@v2.20.3"

root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"

dist="$(mktemp -d "${TMPDIR:-/tmp}/gemini-api-plus-dist.XXXXXX")"
config_dir="$(mktemp -d "${TMPDIR:-/tmp}/gemini-api-plus-config.XXXXXX")"
config="$config_dir/.goreleaser.plus.yaml"
# dist は repo 直下に作らない（この repo に .gitignore が無いため）。設定の先頭に絶対パスの dist を足す。
{
  printf 'dist: %s\n' "$dist"
  cat "$root/.goreleaser.plus.yaml"
} >"$config"

base_version="$(sed -n 's/^var Version = "\(.*\)"$/\1/p' "$root/internal/cli/version.go")"
if [ -z "$base_version" ]; then
  echo "internal/cli/version.go から本家の版を読めなかった" >&2
  exit 1
fi

# CurrentTag を plus-v<版> に固定する。実タグが無くても goreleaser は GORELEASER_CURRENT_TAG を使う。
export GORELEASER_CURRENT_TAG="plus-v${version}"
export PLUS_BASE_VERSION="$base_version"

if command -v goreleaser >/dev/null 2>&1; then
  goreleaser_cmd=(goreleaser)
else
  goreleaser_cmd=(go run "$goreleaser_module")
fi

"${goreleaser_cmd[@]}" release --snapshot --clean --skip=publish -f "$config"

echo "== dist: $dist"

bins=()
while IFS= read -r line; do bins+=("$line"); done < <(find "$dist" -type f \( -name gemini-api-plus -o -name gemini-api-plus.exe \) | sort)
echo "== binaries (${#bins[@]})"
printf '%s\n' "${bins[@]}"
if [ "${#bins[@]}" -ne 6 ]; then
  echo "バイナリが 6 種類ではない: ${#bins[@]}" >&2
  exit 1
fi

archives=$(find "$dist" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \) | wc -l | tr -d ' ')
echo "== archives: $archives"
if [ "$archives" -ne 6 ]; then
  echo "アーカイブが 6 つではない: $archives" >&2
  exit 1
fi

checksums="$dist/checksums.txt"
if [ ! -s "$checksums" ]; then
  echo "checksums.txt が無い" >&2
  exit 1
fi
echo "== checksums.txt ($(wc -l <"$checksums" | tr -d ' ') lines)"
cat "$checksums"

# 現在の OS・arch のバイナリで --version を確かめる。
case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "この OS の --version は確かめない（snapshot は作れている）" >&2; exit 0 ;;
esac
case "$(uname -m)" in
  arm64 | aarch64) arch=arm64 ;;
  x86_64) arch=amd64 ;;
  *) echo "この arch の --version は確かめない" >&2; exit 0 ;;
esac
native="$(printf '%s\n' "${bins[@]}" | grep "${os}_${arch}" | head -n 1 || true)"
if [ -z "$native" ]; then
  echo "${os}/${arch} のバイナリが見つからない" >&2
  exit 1
fi
version_line="$("$native" --version | head -n 1)"
echo "== $native --version"
echo "$version_line"
case "$version_line" in
  "gemini-api-plus ${version} (base gemini-api-cli ${base_version})") ;;
  *)
    echo "--version が期待と違う（期待: gemini-api-plus ${version} (base gemini-api-cli ${base_version})）" >&2
    exit 1
    ;;
esac

cask="$(find "$dist" -name 'gemini-api-plus.rb' | head -n 1 || true)"
if [ -n "$cask" ]; then
  echo "== cask: $cask"
  cat "$cask"
else
  echo "== cask: 生成されず（publish を skip した snapshot では出ない場合がある）"
fi
