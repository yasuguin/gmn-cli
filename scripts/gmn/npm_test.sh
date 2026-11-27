#!/usr/bin/env bash
# npm 配布の手元確認。snapshot の dist から本体と自分の OS 用パッケージを組み立て、pnpm pack した tgz を
# 一時ディレクトリのプロジェクトに入れて、pnpm v10 の既定（lifecycle script 停止）のままコマンドが動くかを見る。
# 公開しない・ネットワークは pnpm が依存を解決するときだけ使う（対応外の OS パッケージは registry から取らない）。
#
# 使い方: scripts/gmn/npm_test.sh [fork の版（既定 0.1.0）]
#   DIST=<既存の goreleaser dist> を渡すと snapshot を作らずにその dist を使う。
set -euo pipefail

version="${1:-0.1.0}"
root="$(cd "$(dirname "$0")/../.." && pwd)"
work="$(mktemp -d "${TMPDIR:-/tmp}/gmn-npm-test.XXXXXX")"
trap 'rm -rf -- "$work"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# 1. dist を用意する（既存の DIST を使うか、snapshot を作る）。
if [ -n "${DIST:-}" ]; then
  dist="$DIST"
else
  snapshot_log="$work/snapshot.log"
  "$root/scripts/gmn/release-snapshot.sh" "$version" >"$snapshot_log" 2>&1 || {
    cat "$snapshot_log" >&2
    fail "release-snapshot.sh が失敗した"
  }
  dist="$(sed -n 's/^== dist: //p' "$snapshot_log")"
  [ -n "$dist" ] || fail "snapshot の出力から dist を読めなかった"
fi
echo "ok: dist = $dist"

# 2. 本体と 7 パッケージを組み立てる。
pkgs="$work/packages"
node "$root/packaging/npm/build.mjs" --dist "$dist" --out "$pkgs" --version "$version" >/dev/null
echo "ok: build.mjs で 7 パッケージを組み立てた"

# 3. 自分の OS・arch に当たるパッケージを選ぶ。
case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "この OS では確かめられない（手元確認は darwin・linux）" ;;
esac
case "$(uname -m)" in
  arm64 | aarch64) arch=arm64 ;;
  x86_64) arch=x64 ;;
  *) fail "この arch では確かめられない: $(uname -m)" ;;
esac
native_name="@yasuguin/gmn-cli-${os}-${arch}"
native_dir="$pkgs/gmn-cli-${os}-${arch}"
[ -x "$native_dir/gmn" ] || fail "$native_dir/gmn が実行可能でない"

# 4. 本体と自分の OS 用パッケージを pnpm pack する。
tarballs="$work/tarballs"
mkdir -p "$tarballs"
(cd "$pkgs/gmn-cli" && pnpm pack --pack-destination "$tarballs" >/dev/null)
(cd "$native_dir" && pnpm pack --pack-destination "$tarballs" >/dev/null)
# 本体は scope 付き（@yasuguin/gmn-cli）。scope なしの gmn-cli は npm が既存の opn-cli に似すぎるとして拒否する。
main_name="$(node -p "require('$pkgs/gmn-cli/package.json').name")"
[ "$main_name" = "@yasuguin/gmn-cli" ] || fail "本体の name が @yasuguin/gmn-cli ではない: $main_name"
main_tgz="$(ls "$tarballs"/yasuguin-gmn-cli-[0-9]*.tgz)"
native_tgz="$(ls "$tarballs"/yasuguin-gmn-cli-"${os}-${arch}"-*.tgz)"
[ -f "$main_tgz" ] || fail "本体の tgz が無い"
[ -f "$native_tgz" ] || fail "OS 用の tgz が無い"
echo "ok: pnpm pack の tgz を作った"

# 5. tgz を一時プロジェクトに入れる。対応外の OS 用パッケージは registry に無い版なので、
#    本体の optionalDependencies のうち自分の OS 用だけを tgz に差し替える（pnpm の overrides）。
proj="$work/consumer"
mkdir -p "$proj"
cat >"$proj/package.json" <<JSON
{
  "name": "consumer",
  "private": true,
  "dependencies": {
    "@yasuguin/gmn-cli": "file:$main_tgz"
  }
}
JSON
# pnpm v10.5 以降の overrides は pnpm-workspace.yaml に書く（package.json の pnpm 欄は読まれない）。
cat >"$proj/pnpm-workspace.yaml" <<YAML
overrides:
  "$native_name": "file:$native_tgz"
YAML
# 以降のコマンドはすべて pnpm v10 で動かす（この環境の既定の pnpm は v11）。
pnpm10() { (cd "$proj" && pnpm dlx pnpm@10 "$@"); }
pnpm10 install >"$work/install.log" 2>&1 || {
  cat "$work/install.log" >&2
  fail "pnpm@10 install が失敗した"
}
pnpm_version="$(pnpm10 --version)"
case "$pnpm_version" in
  10.*) ;;
  *) fail "pnpm v10 で確かめていない（$pnpm_version）" ;;
esac
echo "ok: pnpm@${pnpm_version} install（既定の lifecycle script 停止のまま）"

# pnpm は OS 用パッケージを .pnpm の仮想ストアに置き、本体からだけ解決できる場所に張る。トップレベルには出ない。
native_store="$(find "$proj/node_modules/.pnpm" -maxdepth 1 -name "@yasuguin+gmn-cli-${os}-${arch}@*" | head -n 1)"
[ -n "$native_store" ] || fail "OS 用パッケージが入っていない: $native_name"

# 6. 2 つのコマンドの --version が、直接実行した OS 用バイナリと同じであることを見る。
expected="$("$native_dir/gmn" --version | head -n 1)"
case "$expected" in
  "gmn ${version} "*) ;;
  *) fail "直接実行の --version が期待と違う: $expected" ;;
esac
for cmd in gmn gmn-cli; do
  got="$(pnpm10 exec "$cmd" --version | head -n 1)"
  [ "$got" = "$expected" ] || fail "$cmd --version が違う: got='$got' want='$expected'"
  echo "ok: $cmd --version -> $got"
done

# 7. 同梱の skill が正本と同じ内容であること。
diff -r "$root/skills/gmn-cli" "$proj/node_modules/@yasuguin/gmn-cli/skills/gmn-cli" >/dev/null \
  || fail "同梱の skill が正本と違う"
echo "ok: 同梱の skill は正本と同じ"

echo "PASS: npm 配布の手元確認"
