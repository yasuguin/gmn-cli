#!/usr/bin/env bash
# 本家（google-gemini/gemini-api-cli）のリリースタグを fork の現在のブランチに取り込む。
# rebase はしない。fork 側の差分を履歴に残したまま、本家の変更を merge commit で受ける。
set -euo pipefail

# テストで本家を叩かないための差し替え口。通常は既定の本家 URL を使う。
UPSTREAM_URL=${PLUS_SYNC_UPSTREAM_URL-https://github.com/google-gemini/gemini-api-cli.git}
UPSTREAM_REMOTE=upstream
SKILL_PATH=.agents/skills/plus-upstream-sync/SKILL.md
# 印の行の数え方は、internal/plus（fork 独自の層）と、この仕組み自身のファイルを除く。
MARK='// plus:'
MARK_EXCLUDES=(':(exclude)internal/plus' ':(exclude)scripts/plus' ':(exclude).agents')

usage() {
	cat <<'USAGE'
使い方: scripts/plus/sync-upstream.sh [--dry-run] [--skip-tests] <本家のタグ>
  例: scripts/plus/sync-upstream.sh v0.5.0

本家のタグを refs/tags/upstream/<タグ> に fetch し、現在のブランチへ --no-ff で merge する。
merge 後に plus: の印の行が消えていないかを確かめ、go build と go test を回す。

  --dry-run     何をするかを表示するだけで、fetch も merge もしない
  --skip-tests  go build と go test を省く
  -h, --help    この説明を表示する

終了コード: 0 成功 / 1 失敗（衝突・印の欠落・テスト失敗など） / 2 使い方の誤り
USAGE
}

dry_run=0
skip_tests=0
tag=""
while [[ $# -gt 0 ]]; do
	case "$1" in
	--dry-run) dry_run=1 ;;
	--skip-tests) skip_tests=1 ;;
	-h | --help)
		usage
		exit 0
		;;
	-*)
		echo "不明なオプション: $1" >&2
		usage >&2
		exit 2
		;;
	*)
		if [[ -n "$tag" ]]; then
			echo "タグは 1 つだけ指定してください" >&2
			exit 2
		fi
		tag=$1
		;;
	esac
	shift
done

if [[ -z "$tag" ]]; then
	usage >&2
	exit 2
fi
# 先頭の "-" を git のオプションとして解釈させない。
if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.+-]+)?$ ]]; then
	echo "本家のタグの形式が不正です: ${tag}（例: v0.5.0）" >&2
	exit 2
fi

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$repo_root"

if git rev-parse -q --verify MERGE_HEAD >/dev/null; then
	echo "merge が途中です。衝突を直して commit（または git merge --abort）してから実行してください。" >&2
	exit 1
fi
if ! branch=$(git symbolic-ref --short -q HEAD); then
	echo "detached HEAD では取り込めません。ブランチをチェックアウトしてください。" >&2
	exit 1
fi
# 未追跡ファイルは merge の結果と無関係な限り止めない（他の作業の途中物を巻き込まない）。
if dirty=$(git status --porcelain --untracked-files=no) && [[ -n "$dirty" ]]; then
	echo "作業ツリーに未コミットの変更があります。commit か stash してから実行してください:" >&2
	echo "$dirty" | sed 's/^/  /' >&2
	exit 1
fi

ref="refs/tags/upstream/$tag"

if (( dry_run )); then
	echo "[dry-run] 何も変更しません。実行すると次を行います:"
	if git remote get-url "$UPSTREAM_REMOTE" >/dev/null 2>&1; then
		echo "  1. upstream remote は既にあるので足さない"
	else
		echo "  1. upstream remote を $UPSTREAM_URL で追加"
	fi
	echo "  2. 本家の $tag を $ref に fetch（fork の gmn-v* とは別の名前空間）"
	echo "  3. 現在のブランチ $branch に git merge --no-ff ${ref}（rebase はしない）"
	echo "  4. plus: の印の行が merge 前より減っていないか確かめる"
	if (( skip_tests )); then
		echo "  5. テストは省く（--skip-tests）"
	else
		echo "  5. go build ./... と go test ./... -count=1"
	fi
	exit 0
fi

if ! git remote get-url "$UPSTREAM_REMOTE" >/dev/null 2>&1; then
	echo "upstream remote を追加: $UPSTREAM_URL"
	git remote add "$UPSTREAM_REMOTE" "$UPSTREAM_URL"
fi

# タグは本家と同名で fork に作らない。refspec で upstream/ の名前空間に落とす。
if ! git fetch --no-tags "$UPSTREAM_REMOTE" "+refs/tags/$tag:$ref"; then
	echo "本家に $tag を fetch できませんでした（タグ名か upstream remote を確認）" >&2
	exit 1
fi

baseline=$(mktemp)
after=$(mktemp)
trap 'rm -f "$baseline" "$after"' EXIT

# 印の行を「ファイル: 行の中身」で出す。行番号は merge で動くので含めない。
plus_lines() {
	git grep -n -F -e "$MARK" -- . "${MARK_EXCLUDES[@]}" 2>/dev/null |
		sed -E 's/^([^:]+):[0-9]+:/\1: /' || true
}
plus_lines >"$baseline"

if ! git merge --no-ff -m "Merge upstream $tag

scripts/plus/sync-upstream.sh による merge。rebase しない。" "$ref"; then
	conflicts=$(git diff --name-only --diff-filter=U)
	if [[ -n "$conflicts" ]]; then
		echo "衝突したファイル:" >&2
		echo "$conflicts" | sed 's/^/  /' >&2
		echo "直し方の基準: $SKILL_PATH" >&2
		echo "merge は中断していません。直して commit してください（取り消すなら git merge --abort）。" >&2
	else
		echo "merge に失敗しました（衝突ではありません。上の git の出力を確認）" >&2
	fi
	exit 1
fi

plus_lines >"$after"
before_n=$(wc -l <"$baseline" | tr -d ' ')
after_n=$(wc -l <"$after" | tr -d ' ')
if (( after_n < before_n )); then
	echo "plus: の印の行が $((before_n - after_n)) 行減りました。欠けた場所（merge 前にあって今は無い行）:" >&2
	comm -23 <(sort "$baseline") <(sort "$after") | sed 's/^/  /' >&2
	echo "直し方の基準: $SKILL_PATH" >&2
	exit 1
fi
echo "plus: の印の行: merge 前 $before_n 行 / merge 後 $after_n 行"

if (( skip_tests )); then
	echo "テストは省きました（--skip-tests）"
	exit 0
fi

build_rc=0
test_rc=0
go build ./... || build_rc=$?
go test ./... -count=1 || test_rc=$?
if (( build_rc != 0 || test_rc != 0 )); then
	echo "go build: 終了コード $build_rc / go test: 終了コード $test_rc" >&2
	exit 1
fi
echo "go build と go test が通りました"
