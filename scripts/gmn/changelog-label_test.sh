#!/usr/bin/env bash
# changelog-label.sh の振る舞いテスト。PR の題から、変更履歴の分類のラベルが 1 つ決まることを確かめる。
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
pass=0
fail=0

check() {
  local title=$1 want=$2 got
  got=$(bash "$here/changelog-label.sh" "$title")
  if [[ "$got" == "$want" ]]; then
    pass=$((pass + 1)); echo "ok   - '${title}' -> ${want}"
  else
    fail=$((fail + 1)); echo "FAIL - '${title}' -> got '${got}', want '${want}'"
  fi
}

check "feat(gmn): Agent Platform で music を動かす" "changelog: feat"
check "feat: 新しいコマンド" "changelog: feat"
check "feat(gmn)!: フラグの名前を変える" "changelog: feat"
check "fix(gmn): ADC を見つけられない" "changelog: fix"
check "fix!: 既定値を変える" "changelog: fix"
check "chore(upstream): 本家 v0.6.0 を取り込む" "changelog: upstream"
check "ci(gmn): CI を直す" "changelog: skip"
check "docs(gmn): README を直す" "changelog: skip"
check "chore(main): release gmn 0.1.2" "changelog: skip"
check "refactor(gmn): 名前を変える" "changelog: skip"
check "Conventional でない題" "changelog: skip"

echo "結果: ok $pass / FAIL $fail"
[ "$fail" -eq 0 ]
