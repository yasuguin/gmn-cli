#!/usr/bin/env bash
# sync-upstream.sh の振る舞いテスト。本家を叩かず、一時ディレクトリに作った偽の本家 repo と fork repo で確かめる。
# 失敗が 1 つでもあれば非 0 で終わる。
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script_src="$here/sync-upstream.sh"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# 実機の git 設定（署名・既定ブランチなど）に依存しないようにする。
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.invalid
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.invalid

pass=0
fail=0
ok() { pass=$((pass + 1)); echo "ok   - $1"; }
ng() { fail=$((fail + 1)); echo "FAIL - $1"; }

# 偽の go を PATH の先頭に置く。go の引数を記録し、FAKE_GO_FAIL=build|test で失敗させる。
mkdir -p "$work/bin"
cat >"$work/bin/go" <<'GO'
#!/usr/bin/env bash
echo "$*" >>"$FAKE_GO_LOG"
case "$1" in
build) [[ "${FAKE_GO_FAIL:-}" == build ]] && exit 3 ;;
test) [[ "${FAKE_GO_FAIL:-}" == test ]] && exit 4 ;;
esac
exit 0
GO
chmod +x "$work/bin/go"
export FAKE_GO_LOG="$work/go.log"
export PATH="$work/bin:$PATH"

# 偽の本家（upstream）と fork を作る。
#   mode=plain : fork が自前で plus: の行を足している（衝突・欠落の試験に使う）
#   mode=gone  : 取り込み済みの base に plus: の行があり、本家側が消す（欠落の試験）
# upstream の v0.4.0 が fork の起点、v0.5.0 が取り込み対象。
make_fixture() {
	local dir=$1 mode=$2
	local up="$dir/upstream" fork="$dir/fork"
	mkdir -p "$up"
	git -C "$up" init -q -b main
	if [[ "$mode" == gone ]]; then
		printf 'package main\n\nfunc main() {\n\trun() // plus: fork の差し込み\n}\n' >"$up/cli.go"
	else
		printf 'package main\n\nfunc main() {\n\trun()\n}\n' >"$up/cli.go"
	fi
	git -C "$up" add cli.go
	git -C "$up" commit -q -m "v0.4.0"
	git -C "$up" tag v0.4.0

	git clone -q --no-tags "$up" "$fork"
	git -C "$fork" tag headless-v0.1.0
	if [[ "$mode" == plain ]]; then
		printf 'package main\n\nfunc main() {\n\trun() // plus: fork の差し込み\n}\n' >"$fork/cli.go"
		git -C "$fork" commit -q -am "fork の差し込み"
	fi
	mkdir -p "$fork/scripts/plus"
	cp "$script_src" "$fork/scripts/plus/sync-upstream.sh"
}

# 本家の v0.5.0 を作る。$1 は本家で行う変更の種類（readme | conflict | drop）。
make_release() {
	local up=$1 kind=$2
	case "$kind" in
	readme) echo "変更なし" >"$up/README.md"; git -C "$up" add README.md ;;
	conflict) printf 'package main\n\nfunc main() {\n\trunAll()\n}\n' >"$up/cli.go"; git -C "$up" add cli.go ;;
	drop) printf 'package main\n\nfunc main() {\n\trunAll()\n}\n' >"$up/cli.go"; git -C "$up" add cli.go ;;
	esac
	git -C "$up" commit -q -m "v0.5.0"
	git -C "$up" tag v0.5.0
}

run_sync() {
	# 出力と終了コードを記録する。set -e の下で失敗しても止まらないようにする。
	local fork=$1 out=$2
	shift 2
	local rc=0
	# 本家の代わりに偽の upstream repo を向ける。実際の本家へは一度も接続しない。
	(cd "$fork" && PLUS_SYNC_UPSTREAM_URL="$(dirname "$fork")/upstream" bash scripts/plus/sync-upstream.sh "$@") >"$out" 2>&1 || rc=$?
	echo "$rc"
}

# (a) 衝突なしの merge 成功。go build と go test も通り、終了コード 0。
d="$work/a"; mkdir -p "$d"; make_fixture "$d" plain; make_release "$d/upstream" readme
: >"$FAKE_GO_LOG"
rc=$(run_sync "$d/fork" "$d/out" v0.5.0)
if [[ "$rc" == 0 ]] && git -C "$d/fork" rev-parse -q --verify HEAD^2 >/dev/null \
	&& grep -q '^build ./...$' "$FAKE_GO_LOG" && grep -q '^test ./... -count=1$' "$FAKE_GO_LOG"; then
	ok "(a) 衝突なしで merge され、go build と go test を回して 0 で終わる"
else
	ng "(a) 衝突なし merge が成功しない（rc=${rc}）: $(cat "$d/out")"
fi

# (b) 衝突で止まり、衝突ファイルと skill の場所を出す。merge は中断しない。
d="$work/b"; mkdir -p "$d"; make_fixture "$d" plain; make_release "$d/upstream" conflict
rc=$(run_sync "$d/fork" "$d/out" v0.5.0)
if [[ "$rc" != 0 ]] && grep -q 'cli.go' "$d/out" && grep -q '.agents/skills/plus-upstream-sync/SKILL.md' "$d/out" \
	&& git -C "$d/fork" rev-parse -q --verify MERGE_HEAD >/dev/null; then
	ok "(b) 衝突で非 0 になり、衝突ファイルと skill の場所を出し、MERGE_HEAD を残す"
else
	ng "(b) 衝突の扱いが違う（rc=${rc}）: $(cat "$d/out")"
fi

# (c) 取り込みで plus: の行が消えたら、欠けた場所を示して失敗する。
d="$work/c"; mkdir -p "$d"; make_fixture "$d" gone; make_release "$d/upstream" drop
rc=$(run_sync "$d/fork" "$d/out" --skip-tests v0.5.0)
if [[ "$rc" != 0 ]] && grep -q '減りました' "$d/out" && grep -q 'cli.go: .*plus:' "$d/out"; then
	ok "(c) plus: の行が消えたら、欠けた場所を示して非 0 になる"
else
	ng "(c) plus: の欠落を検出しない（rc=${rc}）: $(cat "$d/out")"
fi

# (d) 本家のタグは upstream/ の名前空間に入り、fork の headless-v* や同名のタグには入らない。
d="$work/d"; mkdir -p "$d"; make_fixture "$d" plain; make_release "$d/upstream" readme
rc=$(run_sync "$d/fork" "$d/out" --skip-tests v0.5.0)
plus_before=$(git -C "$d/fork" tag -l 'headless-v*')
if [[ "$rc" == 0 ]] && git -C "$d/fork" rev-parse -q --verify refs/tags/upstream/v0.5.0 >/dev/null \
	&& [[ -z "$(git -C "$d/fork" tag -l v0.5.0)" ]] && [[ "$plus_before" == "headless-v0.1.0" ]]; then
	ok "(d) 本家のタグは refs/tags/upstream/v0.5.0 に入り、fork に v0.5.0 のタグは作られない"
else
	ng "(d) タグの名前空間が分かれていない（rc=$rc, headless-v=${plus_before}）"
fi

# (e) 存在しないタグは fetch に失敗して 1 で終わる。merge は走らない。
d="$work/e"; mkdir -p "$d"; make_fixture "$d" plain
rc=$(run_sync "$d/fork" "$d/out" --skip-tests v9.9.9)
if [[ "$rc" == 1 ]] && grep -q 'fetch できません' "$d/out"; then
	ok "(e) 存在しないタグは終了コード 1 で止まる"
else
	ng "(e) 存在しないタグの扱いが違う（rc=${rc}）"
fi

# (f) 作業ツリーが汚れていたら何も変えずに止まる。
d="$work/f"; mkdir -p "$d"; make_fixture "$d" plain; make_release "$d/upstream" readme
echo "dirty" >>"$d/fork/cli.go"
rc=$(run_sync "$d/fork" "$d/out" --skip-tests v0.5.0)
if [[ "$rc" == 1 ]] && grep -q '未コミット' "$d/out" && ! git -C "$d/fork" remote get-url upstream >/dev/null 2>&1; then
	ok "(f) 作業ツリーが汚れていたら、remote も足さず 1 で止まる"
else
	ng "(f) 汚れた作業ツリーの扱いが違う（rc=${rc}）"
fi

# (g) go test が失敗したら、その終了コードを 1 で返す。
d="$work/g"; mkdir -p "$d"; make_fixture "$d" plain; make_release "$d/upstream" readme
rc=$(FAKE_GO_FAIL=test run_sync "$d/fork" "$d/out" v0.5.0)
if [[ "$rc" == 1 ]] && grep -q 'go test: 終了コード 4' "$d/out"; then
	ok "(g) go test の失敗を終了コード 1 で返す"
else
	ng "(g) テスト失敗の扱いが違う（rc=${rc}）"
fi

# (h) 引数の検査: --help は 0、タグの形式が不正なら 2。
d="$work/h"; mkdir -p "$d"; make_fixture "$d" plain
rc_help=$(run_sync "$d/fork" "$d/out" --help)
rc_bad=$(run_sync "$d/fork" "$d/out" --skip-tests -v0.5.0)
if [[ "$rc_help" == 0 ]] && [[ "$rc_bad" == 2 ]]; then
	ok "(h) --help は 0、形式の不正なタグは 2 で止まる"
else
	ng "(h) 引数検査が違う（help=$rc_help, bad=${rc_bad}）"
fi

# (i) --dry-run は何も変えない（remote を足さず、タグも fetch しない）。
d="$work/i"; mkdir -p "$d"; make_fixture "$d" plain; make_release "$d/upstream" readme
rc=$(run_sync "$d/fork" "$d/out" --dry-run v0.5.0)
if [[ "$rc" == 0 ]] && grep -q 'dry-run' "$d/out" && ! git -C "$d/fork" remote get-url upstream >/dev/null 2>&1 \
	&& [[ -z "$(git -C "$d/fork" tag -l 'upstream/*')" ]]; then
	ok "(i) --dry-run は予定だけ表示し、remote も tag も作らない"
else
	ng "(i) --dry-run が変更している（rc=${rc}）"
fi

echo "結果: ok $pass / FAIL $fail"
[[ "$fail" == 0 ]]
