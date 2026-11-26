#!/usr/bin/env bash
# upstream-sync-pr.sh の振る舞いテスト。本家と GitHub を叩かず、一時ディレクトリの偽の本家 repo・
# fork の origin（bare repo）と、呼び出しを記録する偽の gh で確かめる。失敗が 1 つでもあれば非 0 で終わる。
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=local_test@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=local_test@example.com

pass=0
fail=0
ok() { pass=$((pass + 1)); echo "ok   - $1"; }
ng() { fail=$((fail + 1)); echo "FAIL - $1"; }

# 偽の gh。引数を記録し、pr list・issue list には環境変数の値を返す。
mkdir -p "$work/bin"
cat >"$work/bin/gh" <<'GH'
#!/usr/bin/env bash
echo "$*" >>"$FAKE_GH_LOG"
case "$1 $2" in
"pr list") printf '%s\n' "${FAKE_GH_OPEN_PRS:-0}" ;;
"issue list") printf '%s\n' "${FAKE_GH_ISSUE_TITLES:-}" ;;
esac
exit 0
GH
chmod +x "$work/bin/gh"
export PATH="$work/bin:$PATH"

# 偽の本家（v0.4.0 が fork の起点、v0.5.0 が取り込み対象）と、fork の origin と作業用の clone を作る。
# kind: readme（衝突しない）| conflict（衝突する）
make_fixture() {
  local dir=$1 kind=$2
  local up="$dir/upstream"
  mkdir -p "$up"
  git -C "$up" init -q -b main
  printf 'package main\n\nfunc main() {\n\trun()\n}\n' >"$up/cli.go"
  git -C "$up" add cli.go
  git -C "$up" commit -q -m "v0.4.0"
  git -C "$up" tag v0.4.0

  git clone -q --bare --no-tags "$up" "$dir/origin.git"
  git clone -q "$dir/origin.git" "$dir/fork"
  local fork="$dir/fork"
  printf 'package main\n\nfunc main() {\n\trun() // plus: fork の差し込み\n}\n' >"$fork/cli.go"
  mkdir -p "$fork/scripts/plus"
  cp "$here/sync-upstream.sh" "$here/upstream-sync-pr.sh" "$fork/scripts/plus/"
  git -C "$fork" add -A
  git -C "$fork" commit -q -m "fork の差し込み"
  git -C "$fork" push -q origin main

  case "$kind" in
  readme) echo "変更" >"$up/README.md"; git -C "$up" add README.md ;;
  conflict) printf 'package main\n\nfunc main() {\n\trunAll()\n}\n' >"$up/cli.go"; git -C "$up" add cli.go ;;
  esac
  git -C "$up" commit -q -m "v0.5.0"
  git -C "$up" tag v0.5.0
}

run_pr() {
  local dir=$1 out=$2
  local rc=0
  : >"$FAKE_GH_LOG"
  (cd "$dir/fork" && PLUS_SYNC_UPSTREAM_URL="$dir/upstream" bash scripts/plus/upstream-sync-pr.sh v0.5.0) >"$out" 2>&1 || rc=$?
  echo "$rc"
}

export FAKE_GH_LOG="$work/gh.log"

# (a) 衝突しなければ、取り込みのブランチを origin に push し、PR を作る。issue は立てない。
d="$work/a"; make_fixture "$d" readme
rc=$(run_pr "$d" "$d/out")
if [[ "$rc" == 0 ]] && git -C "$d/origin.git" rev-parse -q --verify refs/heads/upstream-sync/v0.5.0 >/dev/null \
  && git -C "$d/origin.git" merge-base --is-ancestor "$(git -C "$d/upstream" rev-parse v0.5.0)" refs/heads/upstream-sync/v0.5.0 \
  && grep -q '^pr create .*--head upstream-sync/v0.5.0' "$FAKE_GH_LOG" && ! grep -q '^issue create' "$FAKE_GH_LOG"; then
  ok "(a) 衝突しなければ取り込みのブランチを push して PR を作る"
else
  ng "(a) PR を作っていない（rc=$rc）: $(cat "$d/out"; cat "$FAKE_GH_LOG")"
fi

# (b) PR の題は conventional commit の形で、本家の版を含む。
if grep -q "^pr create .*--title chore(upstream): 本家 v0.5.0 を取り込む" "$FAKE_GH_LOG"; then
  ok "(b) PR の題は chore(upstream): 本家 v0.5.0 を取り込む"
else
  ng "(b) PR の題が違う: $(cat "$FAKE_GH_LOG")"
fi

# (c) 衝突したら PR は作らず、issue を立てる。ブランチは push しない。
d="$work/c"; make_fixture "$d" conflict
rc=$(run_pr "$d" "$d/out")
if [[ "$rc" == 0 ]] && grep -q '^issue create .*--title upstream v0.5.0 を取り込む' "$FAKE_GH_LOG" \
  && ! grep -q '^pr create' "$FAKE_GH_LOG" \
  && ! git -C "$d/origin.git" rev-parse -q --verify refs/heads/upstream-sync/v0.5.0 >/dev/null; then
  ok "(c) 衝突したら PR を作らず issue を立てる"
else
  ng "(c) 衝突の扱いが違う（rc=$rc）: $(cat "$d/out"; cat "$FAKE_GH_LOG")"
fi

# (d) 衝突しても、同じ題の issue があれば立てない。
d="$work/d"; make_fixture "$d" conflict
rc=$(FAKE_GH_ISSUE_TITLES="upstream v0.5.0 を取り込む" run_pr "$d" "$d/out")
if [[ "$rc" == 0 ]] && ! grep -q '^issue create' "$FAKE_GH_LOG"; then
  ok "(d) 同じ版の issue があれば立てない"
else
  ng "(d) issue を二重に立てた（rc=$rc）: $(cat "$FAKE_GH_LOG")"
fi

# (e) 同じ版の取り込みの PR が開いていれば、何もしない。
d="$work/e"; make_fixture "$d" readme
rc=$(FAKE_GH_OPEN_PRS=1 run_pr "$d" "$d/out")
if [[ "$rc" == 0 ]] && ! grep -q '^pr create' "$FAKE_GH_LOG" \
  && ! git -C "$d/origin.git" rev-parse -q --verify refs/heads/upstream-sync/v0.5.0 >/dev/null; then
  ok "(e) 同じ版の PR が開いていれば何もしない"
else
  ng "(e) PR を二重に作った（rc=$rc）: $(cat "$FAKE_GH_LOG")"
fi

# (f) main に取り込み済みなら何もしない。
d="$work/f"; make_fixture "$d" readme
git -C "$d/fork" fetch -q --no-tags "$d/upstream" "+refs/tags/v0.5.0:refs/tags/upstream/v0.5.0"
git -C "$d/fork" merge -q --no-ff -m "本家 v0.5.0 を取り込む" refs/tags/upstream/v0.5.0
git -C "$d/fork" push -q origin main
rc=$(run_pr "$d" "$d/out")
if [[ "$rc" == 0 ]] && ! grep -q 'create' "$FAKE_GH_LOG" && grep -q '取り込み済み' "$d/out"; then
  ok "(f) 取り込み済みなら何もしない"
else
  ng "(f) 取り込み済みなのに動いた（rc=$rc）: $(cat "$d/out"; cat "$FAKE_GH_LOG")"
fi

echo "結果: ok $pass / FAIL $fail"
[ "$fail" -eq 0 ]
