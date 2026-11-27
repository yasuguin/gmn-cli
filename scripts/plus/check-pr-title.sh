#!/usr/bin/env bash
# PR の題が英語の conventional commit の形かを確かめる（.github/CONTRIBUTING.md「変更の入れ方」）。
# squash で PR の題が main の 1 commit になり、release-please と GitHub のリリースノートがそこから
# CHANGELOG.md と Release を作る。外の人が読むので題は英語にし、本文は日本語で書く。
set -euo pipefail

title=${1-}
types='feat|fix|chore|ci|docs|refactor|test|build|perf|revert|style'

if [[ ! "$title" =~ ^($types)(\([^\)]+\))?!?:\ [^[:space:]] ]]; then
  cat >&2 <<MSG
::error::PR の題 '${title}' が conventional commit の形ではありません。
形: <type>(<scope>): <利用者に見える変化を英語で>（例: fix(plus): publish the npm package under @yasuguin/gmn-cli）
type は ${types//|/・} のどれか。直し方: gh pr edit <番号> --title '<新しい題>'
MSG
  exit 1
fi

# 漢字・かな・全角の記号（「（」「、」など）が入っていたら止める。英語の中の é や — は通す。
if perl -CSA -e 'exit($ARGV[0] =~ /[\p{Han}\p{Hiragana}\p{Katakana}\x{3000}-\x{303F}\x{FF00}-\x{FFEF}]/ ? 0 : 1)' "$title"; then
  cat >&2 <<MSG
::error::PR の題 '${title}' に日本語が入っています。題は英語で書き、日本語の説明は本文に書いてください。
直し方: gh pr edit <番号> --title '<英語の題>'
MSG
  exit 1
fi
