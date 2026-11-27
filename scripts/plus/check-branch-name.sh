#!/usr/bin/env bash
# PR のブランチ名が決まりの接頭辞で始まるかを確かめる（.github/CONTRIBUTING.md「変更の入れ方」）。
# 手で切るブランチは gmn/<内容>。release-please--… と upstream-sync/<tag> は workflow が、dependabot/… は Dependabot が自動で作る。
set -euo pipefail

branch=${1-}
case "$branch" in
  gmn/?* | release-please--?* | upstream-sync/?* | dependabot/?*) exit 0 ;;
esac

cat >&2 <<MSG
::error::ブランチ名 '${branch}' は決まりの接頭辞で始まっていません。
手で切るブランチは gmn/<内容>（例: gmn/next-0.1.2）にしてください。
直し方: git branch -m gmn/<内容> && git push -u origin gmn/<内容> で出し直し、この PR は閉じます。
MSG
exit 1
