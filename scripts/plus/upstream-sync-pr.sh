#!/usr/bin/env bash
# 本家（google-gemini/gemini-api-cli）のリリースタグを取り込む PR を作る。週 1 回の workflow
# （plus-upstream-watch.yaml）から呼ぶ。取り込みの中身は sync-upstream.sh（merge commit で取り込み、
# rebase しない）。追うのは本家のリリースだけで、本家の main の未リリースの commit は追わない。
#
# - origin/main に取り込み済みなら何もしない
# - 同じ版の取り込みの PR が開いていれば何もしない
# - 衝突しなければ、取り込みのブランチ upstream-sync/<タグ> を origin に push し、PR を作る
# - 衝突したら PR は作らず、issue を立てる（同じ題の issue が open/closed のどちらかにあれば立てない）
#
# go build と go test は回さない（PR の CI に任せる）。gh と git push の認証は呼び出し側で用意する。
#
# 使い方: scripts/plus/upstream-sync-pr.sh <本家のタグ>
set -euo pipefail

UPSTREAM_URL=${PLUS_SYNC_UPSTREAM_URL-https://github.com/google-gemini/gemini-api-cli.git}
LABEL=upstream-release
SKILL_PATH=.agents/skills/plus-upstream-sync/SKILL.md

if [[ $# -ne 1 ]]; then
	echo "使い方: $0 <本家のタグ>（例: v0.6.0）" >&2
	exit 2
fi
tag=$1
if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.+-]+)?$ ]]; then
	echo "本家のタグの形式が不正です: ${tag}（例: v0.6.0）" >&2
	exit 2
fi

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$repo_root"

branch="upstream-sync/$tag"
ref="refs/tags/upstream/$tag"

git fetch -q --no-tags origin +refs/heads/main:refs/remotes/origin/main
git fetch -q --no-tags "$UPSTREAM_URL" "+refs/tags/$tag:$ref"

if git merge-base --is-ancestor "${ref}^{commit}" refs/remotes/origin/main; then
	echo "本家 ${tag} は main に取り込み済み。何もしない。"
	exit 0
fi

open_prs=$(gh pr list --head "$branch" --state open --json number --jq length)
if [[ "$open_prs" != 0 ]]; then
	echo "本家 ${tag} の取り込みの PR が既に開いている。何もしない。"
	exit 0
fi

git switch -q -C "$branch" refs/remotes/origin/main

if bash scripts/plus/sync-upstream.sh --skip-tests "$tag"; then
	git push -q --force origin "$branch"
	gh label create "$LABEL" --color 0E8A16 --description "本家の新しいリリースの取り込み" --force >/dev/null
	body=$(cat <<BODY
本家 google-gemini/gemini-api-cli の ${tag} を取り込む。\`scripts/plus/sync-upstream.sh\` で merge commit を作った（rebase しない）。

- この PR は **merge commit でマージする**（squash しない）。本家の履歴を残し、次の取り込みの基点を崩さないため。
- go build と go test は、この PR の CI で回す。
- merge 後に確かめることは \`${SKILL_PATH}\` に書いてある。
BODY
	)
	gh pr create --base main --head "$branch" --title "chore(upstream): 本家 ${tag} を取り込む" --label "$LABEL" --body "$body"
	exit 0
fi

# 衝突（または plus: の印の欠落）。merge を片付け、人が直せるよう issue にする。
git merge --abort 2>/dev/null || true
git switch -q --detach refs/remotes/origin/main
title="upstream ${tag} を取り込む"
existing=$(gh issue list --label "$LABEL" --state all --limit 1000 --json title --jq '.[].title')
if printf '%s\n' "$existing" | grep -Fxq "$title"; then
	echo "${title} の issue は既にあるので立てない"
	exit 0
fi
gh label create "$LABEL" --color 0E8A16 --description "本家の新しいリリースの取り込み" --force >/dev/null
body=$(cat <<BODY
本家 google-gemini/gemini-api-cli の最新リリース ${tag} を自動で取り込めなかった（衝突、または plus: の印の欠落）。

手元の clone で取り込んで直し、PR を出す:

\`\`\`
git switch -c ${branch} origin/main
scripts/plus/sync-upstream.sh ${tag}
\`\`\`

衝突の直し方と、merge 後に確かめることは \`${SKILL_PATH}\` に書いてある。取り込みの PR は merge commit でマージする。
BODY
)
gh issue create --title "$title" --label "$LABEL" --body "$body"
