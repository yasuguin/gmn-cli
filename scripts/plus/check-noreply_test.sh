#!/usr/bin/env bash
# check-noreply.sh の振る舞いテスト。一時ディレクトリの git repo で、PR の範囲に入る commit を作って確かめる。
# 失敗が 1 つでもあれば非 0 で終わる。
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script="$here/check-noreply.sh"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# 実機の git 設定（署名・既定ブランチなど）に依存しないようにする。
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_NOSYSTEM=1

# noreply のアドレスは、テスト用のメールアドレスの決まりに当たらないよう組み立てる。
noreply_domain="users.noreply.github.com"
noreply="1+maintainer@${noreply_domain}"
other="local_test@example.com"

pass=0
fail=0
ok() { pass=$((pass + 1)); echo "ok   - $1"; }
ng() { fail=$((fail + 1)); echo "FAIL - $1"; }

# commit を 1 つ作る。引数: author のアドレス、committer のアドレス、題。
commit() {
  echo "$3" >>file.txt
  git add file.txt
  GIT_AUTHOR_NAME=a GIT_AUTHOR_EMAIL="$1" GIT_COMMITTER_NAME=c GIT_COMMITTER_EMAIL="$2" \
    git commit -q -m "$3"
}

# main に 1 commit ある repo を作り、PR のブランチを切る。
new_repo() {
  rm -rf "$work/repo"
  mkdir -p "$work/repo"
  cd "$work/repo"
  git init -q -b main
  commit "$other" "$other" "base（範囲の外なので noreply でなくてよい）"
  git switch -q -c pr
}

run() { "$script" main pr >"$work/out" 2>&1; }

# (a) noreply だけなら通る。GitHub の web で作った commit の committer（noreply@github.com）も通す。
new_repo
commit "$noreply" "$noreply" "fix: noreply の commit"
commit "$noreply" "noreply@github.com" "fix: GitHub の web で作った commit"
if run; then ok "(a) noreply だけの範囲は通る"; else ng "(a) noreply だけの範囲が落ちた: $(cat "$work/out")"; fi

# (b) author が noreply でない commit があれば落ち、その commit とアドレスを出す。
new_repo
commit "$noreply" "$noreply" "fix: noreply の commit"
commit "$other" "$noreply" "fix: author が noreply でない"
sha=$(git rev-parse --short HEAD)
if run; then
  ng "(b) author が noreply でないのに通った"
elif grep -q "$sha" "$work/out" && grep -q "$other" "$work/out"; then
  ok "(b) author が noreply でなければ落ち、commit とアドレスを出す"
else
  ng "(b) 出力に commit かアドレスが無い: $(cat "$work/out")"
fi

# (c) committer だけが noreply でない commit も落ちる。
new_repo
commit "$noreply" "$other" "fix: committer が noreply でない"
if run; then ng "(c) committer が noreply でないのに通った"; else ok "(c) committer だけが noreply でなくても落ちる"; fi

# (d) 本家の commit（refs/upstream/ か refs/tags/upstream/ から辿れるもの）は、作者が noreply でなくても通る。
new_repo
git switch -q -c upstream-work main
commit "$other" "$other" "feat(cli): 本家の commit"
git update-ref refs/tags/upstream/v9.9.9 HEAD
git switch -q pr
GIT_AUTHOR_NAME=a GIT_AUTHOR_EMAIL="$noreply" GIT_COMMITTER_NAME=c GIT_COMMITTER_EMAIL="$noreply" \
  git merge -q --no-ff -m "chore(upstream): v9.9.9 を取り込む" upstream-work
if run; then ok "(d) 本家の commit は対象外で、取り込みの merge commit だけを見る"; else ng "(d) 本家の commit で落ちた: $(cat "$work/out")"; fi

# (d2) CI の置き場所（refs/upstream/ の下）から辿れる本家の commit も対象外。
new_repo
git switch -q -c upstream-work main
commit "$other" "$other" "feat(cli): 本家の commit（CI の置き場所）"
git update-ref refs/upstream/main HEAD
git switch -q pr
GIT_AUTHOR_NAME=a GIT_AUTHOR_EMAIL="$noreply" GIT_COMMITTER_NAME=c GIT_COMMITTER_EMAIL="$noreply" \
  git merge -q --no-ff -m "chore(upstream): 本家を取り込む" upstream-work
if run; then ok "(d2) refs/upstream/ から辿れる本家の commit も対象外"; else ng "(d2) refs/upstream/ の本家の commit で落ちた: $(cat "$work/out")"; fi

# (e) 落ちたときに直し方を出す。付け直しの起点には、実際の base の SHA を出す。
new_repo
base_sha=$(git rev-parse main)
commit "$other" "$other" "fix: noreply でない"
run || true
if grep -q "git config user.email" "$work/out" && grep -q "rebase" "$work/out" && grep -q "$base_sha" "$work/out"; then
  ok "(e) 落ちたときに直し方（user.email の設定と、base の SHA を起点にした付け直し）を出す"
else
  ng "(e) 直し方か base の SHA が出ていない: $(cat "$work/out")"
fi

# (e2) 本家の取り込みを含む範囲では、rebase での付け直しではなく、取り込みのやり直しを案内する。
new_repo
git switch -q -c upstream-work main
commit "$other" "$other" "feat(cli): 本家の commit"
git update-ref refs/upstream/main HEAD
git switch -q pr
GIT_AUTHOR_NAME=a GIT_AUTHOR_EMAIL="$other" GIT_COMMITTER_NAME=c GIT_COMMITTER_EMAIL="$other" \
  git merge -q --no-ff -m "chore(upstream): 本家を取り込む" upstream-work
run || true
if grep -q "sync-upstream.sh" "$work/out" && ! grep -q "rebase --exec" "$work/out"; then
  ok "(e2) 取り込みの merge を含む範囲では、取り込みのやり直しを案内する"
else
  ng "(e2) 取り込みの範囲で rebase を案内している: $(cat "$work/out")"
fi

# (g) author と committer が両方 noreply でなければ、両方のアドレスを出す。
new_repo
commit "local-dev@example.com" "$other" "fix: 両方 noreply でない"
run || true
if grep -q "local-dev@example.com" "$work/out" && grep -q "$other" "$work/out"; then
  ok "(g) author と committer の両方のアドレスを出す"
else
  ng "(g) 片方のアドレスしか出ていない: $(cat "$work/out")"
fi

# (f) 範囲が空なら通る。
new_repo
if run; then ok "(f) 範囲が空なら通る"; else ng "(f) 範囲が空なのに落ちた: $(cat "$work/out")"; fi

echo "結果: ok $pass / FAIL $fail"
[ "$fail" -eq 0 ]
