#!/usr/bin/env bash
# scripts/plus/install.sh の確認。fixture のリリースを file:// で配り、checksum が合うときだけ入ること、
# 合わないときに止まること、公式の gemini-api に触らないことを見る。ネットワークは使わない。
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
installer="$root/scripts/plus/install.sh"
work="$(mktemp -d "${TMPDIR:-/tmp}/gemini-api-plus-install-test.XXXXXX")"
trap 'rm -rf -- "$work"' EXIT

version="9.9.9"
platform="Linux_x86_64"
archive="gemini-api-plus_${platform}.tar.gz"
release_dir="$work/release/plus-v${version}"
mkdir -p "$release_dir" "$work/src"

# 偽のバイナリを固めて、本物と同じ名前のアーカイブと checksums.txt を作る。
printf '#!/bin/sh\necho fork-binary\n' >"$work/src/gemini-api-plus"
chmod +x "$work/src/gemini-api-plus"
tar -czf "$release_dir/$archive" -C "$work/src" gemini-api-plus
shasum -a 256 "$release_dir/$archive" | sed "s| $release_dir/| |" >"$release_dir/checksums.txt"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# 公式の gemini-api が既に入っている状態を作る。上書きされていないことを後で見る。
official_bin_setup() {
  local dir="$1"
  mkdir -p "$dir"
  printf 'official gemini-api\n' >"$dir/gemini-api"
}

run_installer() {
  local install_dir="$1"
  GEMINI_VERSION="$version" \
    GEMINI_INSTALL_DIR="$install_dir" \
    GEMINI_PLUS_PLATFORM="$platform" \
    GEMINI_PLUS_DOWNLOAD_BASE="file://$work/release" \
    bash "$installer" >"$install_dir.log" 2>&1
}

# 1. checksum が合えば gemini-api-plus が入り、公式の gemini-api は変わらない。
ok_dir="$work/bin-ok"
official_bin_setup "$ok_dir"
run_installer "$ok_dir" || {
  cat "$ok_dir.log" >&2
  fail "checksum が合うのに install が失敗した"
}
[ -x "$ok_dir/gemini-api-plus" ] || fail "gemini-api-plus が実行可能な形で入っていない"
[ "$("$ok_dir/gemini-api-plus")" = "fork-binary" ] || fail "入ったバイナリが fixture と違う"
[ "$(cat "$ok_dir/gemini-api")" = "official gemini-api" ] || fail "公式の gemini-api が上書きされた"
echo "ok: checksum 一致で入る・gemini-api を上書きしない"

# 2. checksum が 1 文字でも違えば止まり、何も入らない。
bad_dir="$work/bin-bad"
official_bin_setup "$bad_dir"
cp "$release_dir/checksums.txt" "$work/checksums.good"
# 先頭の 1 文字を必ず別の文字に変える。tar に時刻が入りハッシュは毎回変わるので、
# 固定の文字に置き換えると、先頭が同じ文字のときに不一致にならない。
awk '{ c = substr($1, 1, 1); n = (c == "0") ? "1" : "0"; print n substr($0, 2) }' "$work/checksums.good" >"$release_dir/checksums.txt"
if run_installer "$bad_dir"; then
  cp "$work/checksums.good" "$release_dir/checksums.txt"
  fail "checksum 不一致なのに成功した"
fi
cp "$work/checksums.good" "$release_dir/checksums.txt"
grep -q -i "checksum" "$bad_dir.log" || fail "checksum 不一致の理由が出ていない"
[ ! -e "$bad_dir/gemini-api-plus" ] || fail "checksum 不一致なのにバイナリが入った"
[ "$(cat "$bad_dir/gemini-api")" = "official gemini-api" ] || fail "不一致時に公式の gemini-api が変わった"
echo "ok: checksum 不一致で止まり、何も入れない"

# 3. checksums.txt にアーカイブの行が無ければ止まる（照合できないものは入れない）。
missing_dir="$work/bin-missing"
official_bin_setup "$missing_dir"
: >"$release_dir/checksums.txt"
if run_installer "$missing_dir"; then
  cp "$work/checksums.good" "$release_dir/checksums.txt"
  fail "checksum 行が無いのに成功した"
fi
cp "$work/checksums.good" "$release_dir/checksums.txt"
[ ! -e "$missing_dir/gemini-api-plus" ] || fail "checksum 行が無いのにバイナリが入った"
echo "ok: checksum 行が無ければ止まる"

echo "install_test: all passed"
