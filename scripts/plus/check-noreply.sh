#!/usr/bin/env bash
# PR の範囲（BASE..HEAD）に、GitHub の noreply アドレス以外で作った commit が無いかを確かめる。
# 公開 repo の履歴に個人や会社のメールアドレスを残さないため。author と committer の両方を見る。
#
# 本家（google-gemini/gemini-api-cli）の commit は作者のアドレスを変えられないので対象外にする。
# refs/upstream/ と refs/tags/upstream/ の下の ref から辿れる commit を本家の commit とみなす。
# CI では検査の前に本家を refs/upstream/ へ fetch しておく（手元では sync-upstream.sh が
# 本家のリリースタグを refs/tags/upstream/ に入れている）。
#
# 使い方: scripts/plus/check-noreply.sh BASE HEAD
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "使い方: $0 BASE HEAD" >&2
  exit 2
fi
base="$1"
head="$2"

# 許すアドレス: <id>+<user>@users.noreply.github.com と、GitHub の web で作った commit の committer。
is_noreply() {
  case "$1" in
    *@users.noreply.github.com | noreply@github.com) return 0 ;;
    *) return 1 ;;
  esac
}

upstream_refs=$(git for-each-ref --format='%(refname)' refs/upstream refs/tags/upstream)

bad=0
# shellcheck disable=SC2086 # upstream_refs は ref の並びとして単語分割する。
while IFS=$'\t' read -r sha author committer subject; do
  [ -n "$sha" ] || continue
  for addr in "$author" "$committer"; do
    if ! is_noreply "$addr"; then
      echo "noreply でない: ${sha:0:7} <${addr}> ${subject}"
      bad=1
      break
    fi
  done
done < <(git log --format='%H%x09%ae%x09%ce%x09%s' "${base}..${head}" ${upstream_refs:+--not $upstream_refs})

if [ "$bad" -ne 0 ]; then
  cat <<'EOF'

直し方:
  1. この repo のアドレスを noreply にする:
       git config user.email "<ID>+<ユーザー名>@users.noreply.github.com"
     （アドレスは GitHub の Settings → Emails の「Keep my email addresses private」に出る）
  2. ブランチの commit を付け直す:
       git rebase --exec 'git commit --amend --no-edit --reset-author' <BASE>
  3. 強制 push して PR を更新する:
       git push --force-with-lease
EOF
  exit 1
fi
echo "ok: ${base}..${head} の commit はすべて noreply のアドレス"
