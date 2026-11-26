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

# 範囲に本家の commit が入っているか（本家の取り込みの PR か）。直し方の案内を分けるのに使う。
# shellcheck disable=SC2086 # upstream_refs は ref の並びとして単語分割する。
all_count=$(git rev-list --count "${base}..${head}")
# shellcheck disable=SC2086
own_count=$(git rev-list --count "${base}..${head}" ${upstream_refs:+--not $upstream_refs})
has_upstream=0
[ "$all_count" -gt "$own_count" ] && has_upstream=1

bad=0
# 区切りは空白でない文字（0x1f）にする。tab だと空の欄が詰まり、欄がずれる。
sep=$'\x1f'
# shellcheck disable=SC2086
while IFS="$sep" read -r sha author committer subject; do
  [ -n "$sha" ] || continue
  addrs=()
  is_noreply "$author" || addrs+=("author <${author}>")
  is_noreply "$committer" || addrs+=("committer <${committer}>")
  if [ "${#addrs[@]}" -gt 0 ]; then
    echo "noreply でない: ${sha:0:7} ${addrs[*]} ${subject}"
    bad=1
  fi
done < <(git log --format="%H${sep}%ae${sep}%ce${sep}%s" "${base}..${head}" ${upstream_refs:+--not $upstream_refs})

if [ "$bad" -ne 0 ]; then
  base_sha=$(git rev-parse "$base")
  echo
  echo "直し方:"
  echo "  1. この repo のアドレスを noreply にする:"
  echo '       git config user.email "<ID>+<ユーザー名>@users.noreply.github.com"'
  echo "     （アドレスは GitHub の Settings → Emails の「Keep my email addresses private」に出る）"
  if [ "$has_upstream" -eq 1 ]; then
    # 取り込みの merge を rebase すると、merge が潰れて本家の commit の作者まで書き換わる。
    echo "  2. このブランチは本家の取り込みを含むので、rebase で付け直さない。"
    echo "     ブランチを ${base_sha} から作り直し、scripts/plus/sync-upstream.sh で取り込みをやり直す。"
    echo "  3. 強制 push して PR を更新する:"
    echo "       git push --force-with-lease"
  else
    echo "  2. ブランチの commit を付け直す:"
    echo "       git rebase --exec 'git commit --amend --no-edit --reset-author' ${base_sha}"
    echo "  3. 強制 push して PR を更新する:"
    echo "       git push --force-with-lease"
  fi
  exit 1
fi
echo "ok: ${base}..${head} の commit はすべて noreply のアドレス"
